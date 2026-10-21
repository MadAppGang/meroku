# Meroku V5 Architecture Plan

V5 is the next version of the meroku reference architecture. Its main change is
the event layer: application events move from the shared `default` EventBridge
bus to the **new EventBridge Custom Event Bus** (launched 24 September 2026),
one per project and environment. V5 also closes the defects and takes up the
limitations recorded in `ai_docs/ARCHITECTURE_V4_DECISIONS.md`.

Status: plan. Nothing in this document is implemented. Research date:
2026-09-30. AWS facts come from AWS documentation fetched that day; the sources
are listed at the end. Items marked **to verify** need a spike or a second
source before we build on them.

---

## 1. V4 to V5 at a glance

| Area | V4 (today) | V5 (target) | Gate |
|---|---|---|---|
| Application events | `default` bus, rules and targets | Custom Event Bus per project and environment, subscribers | Terraform path, region, quota (section 2.6) |
| Event retention and replay | None | 1 to 365 days on the bus, replay by subscriber start position | Same |
| Exactly-once ingestion | None; a duplicate publish runs a task twice | Publish-time deduplication, 5-minute window | Same |
| Event consumers (event tasks) | Classic rule, ECS target | Subscriber, universal target `ecs:runTask`, DLQ per subscriber | Spike on the `runTask` input |
| Deploy triggers | Classic rules on `default` | Unchanged in V5.0; optional move through event sources in V5.x | None |
| Ingress | API Gateway or ALB, `alb.enabled`, prefix bug | `ingress: api_gateway \| alb`, one path layout in both modes | Your decision on the prefix (option A or B) |
| Service exposure | Every service public | `expose: true \| false` per service | None |
| Service discovery | `BACKEND_INTERNAL_URL`, own `SERVICE_INTERNAL_URL` | `<NAME>_INTERNAL_URL` for every service | None |
| EC2 `awsvpc` | Refused unless `assume_egress` | Allowed under NAT egress | None |
| Tracing on EC2 bridge pools | Refused | Sidecar reached by container link | Spike |
| Edge protection | None | WAF on CloudFront, ALB and AppSync; ALB locked to CloudFront | Cost decision |
| Frontend | Amplify, platform `WEB`, plaintext env vars | `WEB` or `WEB_COMPUTE`, secrets from SSM | To verify: Amplify secrets |
| Egress | `public_ip`, `nat_gateway`, `nat_gateway_ha` | Adds IPv6-only per project | AAAA check per project |
| Deploy safety | Pinned revisions, manual rollback | ECS circuit breaker with automatic rollback | None |
| AI workloads on GPU | GPU pools start, but no task reserves a GPU; training tasks are Fargate-only | GPU count per service and task; serving on a warm autoscaled pool, training on a pool that scales from zero | None |
| EC2 patching | AMI read at plan time; instances never replaced on a schedule | Instances replaced within 7 days on the newest AMI | To verify: mechanism (section 3.4) |
| Alarms | None | Alarms on DLQ depth and dropped events | None |

---

## 2. The event layer: Custom Event Bus

### 2.1 What the new bus is

EventBridge now has four kinds of bus. The new Custom Event Bus is a separate
product from the Classic bus that meroku could use today.

| | Default bus | Custom bus – Classic | **Custom Event Bus (new)** |
|---|---|---|---|
| Launched | always present | July 2019 | September 2026 |
| Routing | rules, up to 5 targets each | rules, up to 5 targets each | subscribers, one target each |
| Event storage | none | none; add an archive | built in, 1 to 365 days |
| Replay | none | from an archive | a subscriber starts at a past time |
| Ordering | none | none | FIFO per event group, per subscriber |
| Deduplication | none | none | at publish, by content or by key, 5-minute window |
| Pause and resume | disable a rule; events are lost | same | subscriber `STOPPED`, resumes with its backlog |
| Payload formats | JSON | JSON | JSON, Avro, Protobuf, raw bytes |
| Receives AWS service events | yes, the only bus that does | through a forwarding rule | through an **event source** (a managed forwarding rule) |
| Cross-account | resource policy | resource policy, or forward to another bus | AWS RAM share; each consumer account owns its subscribers |
| API and SDK | `events`, `EventBridge` | same | `eventsv2`, `EventBridgeV2`, own endpoint |
| IAM prefix | `events:` | `events:` | `events:`, shared. A policy that names `eventsv2:` grants nothing. |
| ARN | `event-bus/default` | `event-bus/<name>` | `event-busv2/<name>/<generated id>`; cannot be built from the name |
| Price model | per event | per event | per GB in, per GB out, storage beyond 24 hours |
| Buses per account (default quota) | 1 | 100 | **5**, adjustable |
| Terraform (`hashicorp/aws`) | yes | yes | **no** (checked v6.66.0, 2026-09-21) |
| CloudFormation | yes | yes | yes, `AWS::EventsV2::*` |
| Regions | all | all | 14 at launch (section 2.6) |

The two products share the IAM namespace and the `events.amazonaws.com` service
principal. Existing identity policies and delivery roles carry over.

### 2.2 Why V5 uses the new bus

1. **Retention replaces archives.** Every event stays on the bus for the period
   we set. A consumer added today can read yesterday's events. A consumer that
   was down resumes without loss. With Classic we would need an archive and a
   separate replay process.
2. **Exactly-once ingestion solves our double-run risk.** Every meroku consumer
   is an ECS task started per event. A producer that retries a publish today runs
   the task twice. On the new bus, a publish with a `DeduplicationId` is
   suppressed if it repeats within 5 minutes. A FIFO subscriber then hands each
   accepted event to its target once, in order, within its group.
3. **Ordered processing when a workload needs it.** A subscriber of type `FIFO`
   delivers in publish order per `EventGroupId`, for example per customer or per
   order. Classic has no ordering.
4. **Safe migrations and reprocessing.** A new subscriber can start at a past
   time. Section 2.8 uses this to cut over without losing or duplicating events.
5. **Cross-account by design.** A bus shared through AWS RAM lets a dev account
   and a prod account take part in one event flow, each owning its own
   subscribers. This fits meroku's multi-account setup (DNS delegation,
   cross-account ECR).
6. **Cheaper at meroku's event sizes** (section 2.3).
7. **Better failure signals.** Per-subscriber metrics (`FilterEvaluated`,
   `FilterMatched`, `EventsDelivered`, `EventsDropped`,
   `OnFailureDestinationDelivered`) show exactly where an event stopped.

### 2.3 Cost

**Rates** (from the worked example on the AWS pricing page; the rate table
itself loads by JavaScript and was not read directly):

| Item | New bus | Classic custom bus |
|---|---|---|
| Publish | $0.18 per GB, first 5 TB per month | $1.00 per million events, per 64 KB chunk |
| Delivery to a target in the same account | $0.05 per GB per subscriber | free |
| Storage | first 24 hours included; $0.08 per GB-month beyond | archive: $0.10 per GB processed, $0.023 per GB-month |
| Content-based deduplication | $0.15 per million events | not available |
| JSONata transform | $0.15 per million events | not charged |
| Deduplication by `DeduplicationId` | not charged as an evaluation | not available |
| Fixed monthly fee | none | none |

Events are rounded up to 1 KB. With 1 TB counted as 1,000 GB, one million 1 KB
events is 1 GB.

**Comparison: one million events per month, one subscriber, no transform, 1-day
retention:**

| Event size | New bus | Classic |
|---|---|---|
| 1 KB | $0.18 + $0.05 = **$0.23** | $1.00 |
| 2 KB | $0.36 + $0.10 = **$0.46** | $1.00 |
| 4 KB | $0.72 + $0.20 = **$0.92** | $1.00 |
| 8 KB | $1.44 + $0.40 = $1.84 | **$1.00** |

Break-even is about 4.3 KB per event with one subscriber. Each extra subscriber
adds $0.05 per GB. meroku's application events are small JSON documents, so the
new bus is cheaper. At idle, both cost $0.

Not in the table, **to verify**: whether a universal target such as
`ecs:runTask` has its own charge.

### 2.4 Target design

```
                        producers (backend, services, tasks)
                              │  PutEvents to EVENT_BUS_ARN
                              │  SystemMetadata.DeduplicationId = idempotency key
                              ▼
┌──────────────────────── Custom Event Bus <project>-<env> ────────────────────────┐
│  retention: 7 days (default in meroku)                                             │
│  subscribers (one per event task rule):                                            │
│    filter DATA  {"source": [...], "detail-type": [...]}                            │
│    target       arn:aws:events:::aws-sdk:ecs:runTask   (universal target)          │
│    retry        86,400 s / 185 attempts (Classic parity)                            │
│    on failure   SQS DLQ per subscriber                                              │
└────────────────────────────────────────────────────────────────────────────────────┘
          ▲ event source (V5.x, optional)             ▲ Classic rule on default (migration only)
          │ aws.ecr / aws.ecs / aws.ssm               │ forwards <project>.* events
     default bus ─────────────────────────────────────┘
          │ Classic rules (unchanged in V5.0)
          ▼
     deploy coordinator Lambda
```

| Element | V5 choice | Reason |
|---|---|---|
| Bus granularity | One bus per project and environment | Isolation between environments; matches meroku's account layout. |
| Bus name | `<project>-<env>` | Name limit is 256 characters, above the 80-character cutoff for `modules/naming`. |
| Retention | 7 days by default, YAML override 1 to 365 | Covers a weekend outage and a replay after a bad deploy. First 24 hours are free; 7 days of 1 KB events at 1 million per month is well under 1 GB stored. |
| Publish API | `PutEvents` | Keeps the Classic envelope, so existing event patterns work unchanged as `DATA` filters. `PutRawEvents` puts fields at a different path and silently matches nothing with a Classic pattern. |
| Deduplication | By `DeduplicationId`, supplied by the producer | Free; content-based deduplication costs $0.15 per million. |
| Subscriber type | `UNORDERED` by default, `FIFO` per event task on request | Unordered gives the highest throughput. |
| Event task target | Universal target `arn:aws:events:::aws-sdk:ecs:runTask`, `MaxBatchSize: 1` | The new bus has no bespoke ECS target. Batch size 1 keeps one task per event. |
| Retry | `MaxEventAgeInSeconds: 86400`, `MaxRetryAttempts: 185` | New-bus default is 300 seconds and 5 attempts. Without this, an outage over 5 minutes dead-letters events that Classic would have delivered late. |
| Dead-letter queue | One standard SQS queue per subscriber | FIFO queues are rejected. The delivery role needs `sqs:SendMessage` on it, or failed events vanish with no signal. |
| Delivery role | One per subscriber, trusts `events.amazonaws.com` | Needs `ecs:RunTask`, `iam:PassRole` for the task roles, and the DLQ grant. |
| Producer IAM | `events:PutEvents`, `events:PutRawEvents` on the exact bus ARN | Replaces today's wildcard `event-bus/<project>-*`, which lets one environment publish to another. |
| App configuration | `EVENT_BUS_ARN` (new), `EVENT_BUS_NAME` kept | The new SDK client addresses the bus by ARN. |
| Alarms | `EventsDropped` > 0, `OnFailureDestinationFailed` > 0, DLQ depth > 0 | First alarms in meroku; see Defect 4 in `ARCHITECTURE_V4_DECISIONS.md`. |

### 2.5 What stays on the default bus

| Events | Why |
|---|---|
| ECR push, ECS deployment state, SSM parameter change | AWS emits service events to the default bus only. |
| S3 `.env` changes through CloudTrail | Same; and still depends on a CloudTrail trail (Open defect 4). |
| Manual deploy events from CI | Pipelines already in use publish to `default` by name. |

**V5.x option:** create **event sources** for `aws.ecr`, `aws.ecs` and `aws.ssm`
on the project bus and make the deploy coordinator a subscriber. Gains: 7 days
of deploy history, replay of a missed deploy, one place to observe all events.
Cost: forwarded AWS events are billed at the ingress rate (S3 and partner events
are excluded). Match these events on `SYSTEM_METADATA` `aws:Source`, because no
publisher can set a source that starts with `aws.`, so it proves the origin.

### 2.6 Gates before we build

| Gate | Status | How to pass |
|---|---|---|
| **Terraform support** | `hashicorp/aws` and `hashicorp/awscc` have no `eventsv2` resources. CloudFormation has `AWS::EventsV2::EventBus`, `Subscriber`, `EventSource`, `ResourcePolicy`. | Option A below now, native resources later. |
| **Region** | Available in us-east-1, us-east-2, us-west-2, eu-west-1, eu-central-1, eu-north-1, eu-south-2, ap-east-1, ap-southeast-5, ap-south-1, ap-southeast-1, ap-southeast-2, ap-southeast-7, ap-northeast-1 | Generate-time check. Other regions stay on the V4 path. **To verify**: the region codes for Spain, Malaysia and Thailand. |
| **Quota: 5 buses per account** | Adjustable | Most meroku accounts hold one environment of one project: 1 bus. A preflight check counts buses (`ResourceCount` in `AWS/Usage`) and names the quota to raise. |
| **Application SDK** | Producers need the `EventBridgeV2` client | **To verify**: minimum SDK versions for Go and Node.js. meroku ships a code sample per language. |

**Terraform delivery options:**

| Option | How | For | Against |
|---|---|---|---|
| **A. `aws_cloudformation_stack` inside the module** (recommended now) | The module renders a CloudFormation template with the `AWS::EventsV2` resources and reads the bus ARN from stack outputs. | Works today. Terraform still owns the lifecycle. | Two layers; drift inside the stack is less visible. |
| B. `awscc` provider | Wait for `awscc` to generate resources from the CloudFormation registry. | Native Terraform. | Not available; timing unknown. |
| C. `hashicorp/aws` resources | Wait for native resources. | Best long-term. | Timing unknown. |

Keep the module's inputs and outputs (`event_bus_arn`, `event_bus_name`) the
same across A, B and C, so the switch from A later is internal to one module.
Note the switch itself will need a state move or a planned replacement.

### 2.7 Lifecycle traps

1. **Subscriber replacement drops events.** `Name`, `EventBusArn`,
   `InvokeConfiguration.TargetArn`, `Type`, `StartingPosition` and
   `PointInTimeConfiguration` are create-only. CloudFormation deletes the old
   subscriber before it creates the new one, and a new `LATEST` subscriber never
   sees events from the gap. meroku must give a replacement subscriber
   `POINT_IN_TIME` with a start time from before the change.
2. **Bus deletion needs every subscriber gone.** `DeleteEventBus` fails into
   `DELETE_FAILED` while any subscriber or event source exists, including one
   another account created. `terraform destroy` must delete subscribers first.
3. **The bus is not usable at once.** It starts in `CREATING`; publishing and
   creating subscribers wait for `ACTIVE`.
4. **A wrong-shape filter fails silently.** A pattern written for `PutEvents`
   matches nothing on `PutRawEvents` events, and the reverse. meroku must fix the
   publish API per bus (V5: `PutEvents` only).
5. **KMS key changes.** If a key update fails, do not disable the old key until
   the bus is `ACTIVE` on the new one, or the bus can only be deleted.
6. **Filter changes are limited** to 24 per subscriber in any 24 hours.
7. **One subscriber create or delete at a time per bus.** Parallel Terraform
   operations hit `ConcurrentModificationException` and must retry.
8. **`DEDUPLICATED` is a success code.** Producer code that counts successes as
   new events overstates what was published.

### 2.8 Migration plan

AWS's own guide says to publish to both buses during a move. In meroku that
runs every event task twice. This plan uses a forwarding rule and the bus's
retention instead, so each event runs its task once.

| Step | Change | Runtime effect | Rollback |
|---|---|---|---|
| 0. Spike | Scratch account: bus through Option A, one `runTask` subscriber, forwarding rule, a replay from a past time. Confirm the `runTask` input, the delivery role, `MaxBatchSize: 1`, and the step 3 cutover. | None | n/a |
| 1. Add the bus | Create the bus, DLQs, delivery roles, alarms. No subscribers. | None | Delete the bus |
| 2. Forward | Add a Classic rule on `default` matching `<project>.*` sources, target the new bus. Events are stored, not delivered. | Forwarding cost only ($1 per million plus new-bus ingress) | Delete the rule |
| 3. Cut over consumers | Apply 1: delete the old event-task rules and record the time T. Apply 2: create the subscribers with `POINT_IN_TIME` start T. | Events before T ran through old rules. Events from T replay from retention. No loss, no duplicate. **To verify** in the spike: ingestion timestamps against rule matching at T. | Recreate the old rules; delete the subscribers |
| 4. Move producers | Set `EVENT_BUS_ARN`; apps switch to the `EventBridgeV2` client and send a `DeduplicationId`. Services roll. | Apps that did not switch keep working through the forwarding rule. | Point apps back to `default` |
| 5. Drain | When the forwarding rule's matched-event count stays at zero for an agreed window, delete it. Narrow producer IAM to the bus ARN only. | None | Recreate the rule |
| 6. Optional | Event sources for deploy events (section 2.5), FIFO subscribers, RAM sharing. | Per feature | Per feature |

**Schema:** a migration writes `event_bus.type: default` into existing project
files, so no existing routing changes without a YAML edit. New projects get
`event_bus.type: custom`. A third value, `classic`, is the fallback for regions
without the new bus. This follows the precedent of the v6 VPC migration.

Proposed YAML:

```yaml
event_bus:
  type: custom            # default | classic | custom
  retention_days: 7       # custom only, 1 to 365
  kms_key_arn: ""         # optional customer managed key

event_processor_tasks:
  - name: order-sync
    rules:
      - sources: ["myapp.backend"]
        detail_types: ["OrderPlaced"]
    ordering: unordered   # unordered | fifo (custom only)
```

### 2.9 Risks

| Risk | Mitigation |
|---|---|
| The product is six days old | Spike first; keep `classic` and `default` as supported types. |
| No native Terraform resources | Option A behind a stable module interface. |
| Apps that ignore `EVENT_BUS_ARN` | The forwarding rule keeps them working; step 5 waits on its metric. |
| Retry defaults far shorter than Classic | meroku sets Classic-parity retry on every subscriber. |
| Silent filter mismatch | `PutEvents` only; a generated diagnostic subscriber is an option for debugging. |
| Quota of 5 buses | Preflight check; one bus per project and environment. |

### 2.10 Fallback: Custom Event Bus – Classic

Where the new bus is not available (region, or a gate that does not pass),
V5 uses a Classic custom bus `<project>-<env>` instead (`event_bus.type:
classic`).

- It has native Terraform support (`aws_cloudwatch_event_bus`), rules and
  targets like today, and costs $1 per million events, with same-account
  delivery free.
- It gives isolation per environment, exact-ARN IAM, and optional archive,
  logging and a KMS key. It gives no retention, deduplication or ordering.
- A rule cannot move between buses in place: Terraform replaces it. Migration
  therefore copies the rules: (1) add copies of the event-task rules on the new
  bus; (2) switch `EVENT_BUS_NAME` for producers; (3) delete the old rules when
  their matched-event count stays at zero. Consumers listen on both buses during
  the move and each producer publishes to one, so no task runs twice.

---

## 3. AI compute on GPU pools

V4 has GPU pools but cannot yet run AI workloads properly (V4 decisions,
Decision 1 limitations). V5 closes the gaps for both workload shapes.

| Workload | V5 setup |
|---|---|
| Model serving | A service on a GPU pool, `capacity_type: spot_with_base`, `on_demand_base` ≥ 1, `min_size` ≥ 1, autoscaling by ECS managed scaling. The on-demand base keeps the model available during a spot reclaim. |
| Training and fine-tuning | A scheduled or event-triggered task on a GPU pool, `capacity_type: spot`, `min_size: 0`. The pool starts instances when the job arrives and returns to zero after. The job checkpoints to S3 or EFS, so a reclaim costs one checkpoint interval, not the whole run. |

### 3.1 GPU reservation per task

- Add `gpu: <count>` to services and tasks. meroku renders it as the task
  definition's GPU resource requirement.
- ECS then places a task only on an instance with free GPUs and gives each task
  its own GPUs.
- Validation: `gpu` > 0 requires a pool with `ami_family: al2023_gpu` and GPU
  instance types; `gpu` on Fargate is refused.

### 3.2 Tasks on compute pools

- Add `runtime` and `compute_pool` to `scheduled_tasks[]` and
  `event_processor_tasks[]`, the same fields services have.
- With the V5 event bus, a training job can be started by an event
  (subscriber → `ecs:runTask` with the pool's capacity provider) or by a
  schedule.

### 3.3 Spot for AI workloads

Pools already support spot (V4 decisions, Decision 1). V5 adds guidance and
checks for GPU pools:

- List several GPU instance types of similar capacity per pool, because GPU
  spot capacity is scarce.
- Warn at generate time when a GPU service pool uses `capacity_type: spot`
  with no on-demand base.
- *Approximate*: AWS advertises spot discounts of up to 90 percent; the real
  GPU discount varies by type, region and time.

### 3.4 Weekly instance replacement

**Goal:** no EC2 instance is more than one week behind on patches, with no human
action. This removes the main maintenance argument against EC2.

The earlier discussion of this topic is not recorded in the repo, and the
mechanisms below were not checked against AWS documentation on 2026-09-30.
**To verify** before building.

| Mechanism | How it works | Fit |
|---|---|---|
| ASG maximum instance lifetime + AMI resolved at launch | Set the Auto Scaling group's maximum instance lifetime to 7 days. Set the launch template image to `resolve:ssm:<the ECS AMI parameter>`, so EC2 reads the newest AMI when each instance starts, not when Terraform runs. Each instance is replaced before it turns 7 days old, on the current AMI. | Works with the current pools. Managed draining moves tasks first. |
| Amazon ECS Managed Instances | AWS runs the EC2 instances for the cluster, including patching and periodic replacement. We choose instance attributes. | Closest to Fargate. To verify: GPU support, replacement interval, network modes, price, Terraform support. |

---

## 4. Networking, ingress and other items

Each item closes a limitation or defect in `ai_docs/ARCHITECTURE_V4_DECISIONS.md`.

| # | Item | Summary |
|---|---|---|
| 1 | Ingress setting and path fix | Rename `alb.enabled` to `ingress: api_gateway \| alb`. Make the `/<service>` path identical in both modes (Defect 1, pending your choice). Remove the duplicate API Gateway service routes in ALB mode (Defect 2). |
| 2 | `expose` per service | Internal services get no public route. Outbound access is unchanged. |
| 3 | `<NAME>_INTERNAL_URL` for every service | Tasks stop building Cloud Map names by hand. |
| 4 | `awsvpc` under NAT | Rule 11 reads `egress_strategy`; error text and docs corrected (Defect 3). |
| 5 | Tracing on bridge pools | Drop the sidecar's host ports, link the app container to it, point the collector address at the link name. Spike needed. |
| 6 | WAF and CloudFront lock | WAF on CloudFront, the ALB and AppSync. Restrict the ALB security group to CloudFront. **To verify**: WAF support for API Gateway HTTP APIs. |
| 7 | Amplify `WEB_COMPUTE` and SSM secrets | Enables SSR (Defect 5); build secrets from SSM Parameter Store, names only in YAML. **To verify** against Amplify docs. |
| 8 | IPv6-only workloads | Removes the $3.65 public IPv4 per task. Works only as IPv6-only subnets, not dual-stack. See section 4.1. |
| 9 | Deploy circuit breaker | Automatic rollback to the last pinned revision. |
| 10 | Alarms | DLQ depth and dropped events (section 2.4), plus a warning when the S3 `.env` trigger has no CloudTrail trail (Defect 4). |
| 11 | IAM tightening | Defects 8, 9 and 10. |
| 12 | SNS webhooks to `api_domain` | Defect 6. |

### 4.1 IPv6-only workloads (research 2026-09-30)

**The idea.** A service that needs no third-party API still needs a public IPv4
today. Give it IPv6 instead, because IPv6 addresses cost nothing.

**Verdict: partly correct.**

| Claim | Finding |
|---|---|
| "Without a public IP the task loses all internal infrastructure" | Wrong. RDS, Cloud Map, EFS, the ALB, the API Gateway VPC link and S3 (gateway endpoint) work over private IPv4. The task loses the AWS service endpoints (ECR, CloudWatch Logs, SSM, SQS, SES, EventBridge) and the internet, and that is enough to stop it starting. |
| "So give it IPv6" | Only as **IPv6-only** subnets. A dual-stack task with no public IPv4 fails: AWS docs say Fargate reaches ECR, SSM and Secrets Manager over IPv4 at startup in dual-stack mode, and the ECS agent source switches to IPv6 endpoints only on IPv6-only hosts. |
| "All AWS services support IPv6" | Mostly, under different host names. 16 of 19 default SDK host names have no IPv6 address; apps need `AWS_USE_DUALSTACK_ENDPOINT=true` (Java SDK v1 cannot). ECS Exec and AppSync clients do not support IPv6. RDS can only be dual-stack. |
| "IPv6 routing is free" | Partly. Addresses and the egress-only gateway are free. Data is billed per GB as for IPv4. |
| "The only risk is external services" | No. See the blockers below. External risk is also large: 38 of 89 third-party hosts checked have no IPv6 address. |

**Third-party IPv6 (DNS check, 2026-10-01, resolvers 1.1.1.1 and 8.8.8.8):**

| Has IPv6 | No IPv6 |
|---|---|
| Google APIs (`googleapis.com`, FCM, OAuth, Gemini), OpenAI, Anthropic, GitLab, Bitbucket, npm, PyPI, Go proxy, Docker Hub, `raw.githubusercontent.com` | `github.com`, `api.github.com`, `ghcr.io`, Stripe, Twilio, SendGrid, Slack |

**Blockers inside meroku:**

- ECR images need the IPv6 URI `<account>.dkr-ecr.<region>.on.aws`. The CI Lambda hard-codes the IPv4 URI (`ci_lambda/internal/handler/ecr.go:89-90`), so every deploy would break the task while CI stays green.
- The OTEL sidecar comes from `public.ecr.aws` (IPv4 only); it must use `ecr-public.aws.com`.
- Cloud Map cannot create A records in IPv6-only subnets. Changing the record type replaces the Cloud Map service, and IPv4-only callers lose the service.
- API Gateway VPC link to IPv6-only targets is undocumented: a blocker for the default ingress until tested.
- The ALB must become dual-stack with a new IPv6 target group.
- Task subnets must route `::/0` only to the egress-only gateway. Named-service security groups allow `::/0` inbound, and IPv6 addresses are public.
- Not available in Ireland, Stockholm or Zurich.
- Provider pin `~> 5.0`: EFS IPv6 needs 6.12, the `dualStackIPv6` account setting needs 6.13.

**Cost (fixed, per month):**

| Option | Starts? | 1 task | 5 tasks | 20 tasks |
|---|---|---|---|---|
| Public IPv4 per task (today) | yes | $3.65 | $18.25 | $73.00 |
| Dual-stack, no public IPv4 | no | n/a | n/a | n/a |
| IPv6-only + egress-only gateway | yes, if every dependency has IPv6 | $0 | $0 | $0 |
| IPv6-only + NAT64 | yes | $36.50 | $36.50 | $36.50 |
| NAT Gateway | yes | $36.50 | $36.50 | $36.50 |

**Proposed design.**

- Environment: `ipv6: true` adds an IPv6 range to the VPC and two IPv6-only task
  subnets behind an egress-only gateway. No fixed cost. In NAT environments,
  optional `ipv6_nat64: true` reaches IPv4-only hosts through the existing NAT.
- Workload: `network: ipv6_only`, first for Fargate workers, queue consumers and
  scheduled or event tasks. meroku rewrites the image URIs, sets
  `AWS_USE_DUALSTACK_ENDPOINT=true` and registers AAAA records.
- Generate-time refusal when: default VPC, unsupported region, ECS Exec on, EFS
  mounts, API Gateway in front, IPv4-only callers, IPv4-only image host, or
  undeclared external hosts.

**Spike before building** (sandbox account, us-east-1):

1. IPv6-only task with dual-stack ECR URI, awslogs, an SSM secret, a Secrets
   Manager secret and an S3 env file. Pass: RUNNING, logs and secrets present.
2. Negative control: dual-stack subnet, `assign_public_ip=false`. Expect
   `ResourceInitializationError`.
3. API Gateway HTTP API through the VPC link to an IPv6-only Cloud Map service.
4. Repeat 1 with `dualStackIPv6` disabled (the AWS doc contradicts itself).
5. RDS in `DUAL` mode reached from the IPv6-only task.

Confidence: medium-high. Nothing was deployed. Unverified: VPC link behaviour,
the Fargate agent's IPv6 behaviour, the `dualStackIPv6` requirement.

---

## 5. V5 diagram changes

Compared with the V4.2 diagram:

- **Event broker layer:** replace "Amazon EventBridge" with "EventBridge Custom
  Event Bus `<project>-<env>`", with notes "retention 7 days", "dedup",
  "FIFO optional". Draw subscribers as the edges to event tasks, labelled
  "subscriber → ecs:runTask". Add a DLQ per subscriber.
- **Default bus:** keep it as a separate node that feeds only the deploy
  coordinator. Add a dashed "event source (V5.x)" edge from it to the custom bus.
- **Scheduler:** unchanged, a separate service.
- **Producers:** backend, services and tasks → custom bus, labelled "PutEvents".
- **Cross-account:** optional dashed "RAM share" edge to another account's bus.
- **Ingress:** one path layout; WAF badges on CloudFront, ALB and AppSync
  marked planned.
- **Legend:** solid for V5 deployed, dashed for V5.x options.

---

## 6. Order of work

| Phase | Content | Depends on |
|---|---|---|
| 5.0-a | Fixes with no design question: Open defects 2, 3 (rule 11), 4 (CloudTrail warning), 6, 7, and IAM tightening | Nothing |
| 5.0-b | Event bus spike (step 0) | Nothing |
| 5.0-c | Event bus steps 1 to 3, schema migration, module (Option A) | 5.0-b passes |
| 5.0-d | Ingress path fix and `expose` | Your prefix decision |
| 5.1 | Producers move (step 4), alarms, internal URLs, circuit breaker | 5.0-c |
| 5.1-ai | GPU reservation, tasks on pools, spot checks, weekly instance replacement (section 3) | Patching mechanism verified |
| 5.2 | Tracing on bridge, Amplify SSR and secrets, WAF | Spikes and cost decision |
| 5.x | Event sources for deploy events, FIFO consumers, RAM sharing, IPv6, native Terraform resources | AWS and provider releases |

---

## 7. Open questions

1. Prefix behaviour: strip in both modes (A) or keep in both (B)?
2. Default retention: 7 days, or 1 day (free) with opt-in longer?
3. Does WAF ship in V5 or stay planned? It adds a fixed monthly charge per web ACL.
4. Which languages get an `EventBridgeV2` producer sample first?

---

## Sources

AWS documentation, fetched 2026-09-30:

- [Amazon EventBridge event bus (Custom)](https://docs.aws.amazon.com/eventbridge/latest/userguide/eb-custom-bus.html)
- [What is the EventBridge Custom Event Bus?](https://docs.aws.amazon.com/eventbridge/latest/userguide/eb-custom-bus-what-is.html)
- [Creating a bus](https://docs.aws.amazon.com/eventbridge/latest/userguide/eb-custom-bus-create.html)
- [Publishing events](https://docs.aws.amazon.com/eventbridge/latest/userguide/eb-custom-bus-publish.html)
- [Event structure](https://docs.aws.amazon.com/eventbridge/latest/userguide/eb-custom-bus-addressing.html)
- [Subscribing to events](https://docs.aws.amazon.com/eventbridge/latest/userguide/eb-custom-bus-subscribers.html)
- [Ordering and deduplicating events](https://docs.aws.amazon.com/eventbridge/latest/userguide/eb-custom-bus-ordering.html)
- [Replaying retained events](https://docs.aws.amazon.com/eventbridge/latest/userguide/eb-custom-bus-replay.html)
- [Event sources](https://docs.aws.amazon.com/eventbridge/latest/userguide/eb-custom-bus-event-sources.html)
- [Migrating from Custom Event Bus - Classic](https://docs.aws.amazon.com/eventbridge/latest/userguide/eb-custom-bus-migrate.html)
- [Names, endpoints, and IAM permissions](https://docs.aws.amazon.com/eventbridge/latest/userguide/eb-custom-bus-names.html)
- [CloudFormation resources](https://docs.aws.amazon.com/eventbridge/latest/userguide/eb-custom-bus-cloudformation.html)
- [EventBridge quotas](https://docs.aws.amazon.com/eventbridge/latest/userguide/eb-quota.html)
- [Events from AWS services](https://docs.aws.amazon.com/eventbridge/latest/userguide/eb-events.html)
- [EventBridge pricing](https://aws.amazon.com/eventbridge/pricing/)
- AWS News Blog, [Introducing enhanced custom event buses in Amazon EventBridge (2026-09-24)](https://aws.amazon.com/blogs/aws/introducing-enhanced-custom-event-buses-in-amazon-eventbridge-for-enterprise-scale-event-driven-applications/)
- [hashicorp/terraform-provider-aws](https://github.com/hashicorp/terraform-provider-aws) and [hashicorp/terraform-provider-awscc](https://github.com/hashicorp/terraform-provider-awscc): searched for `eventsv2`, none found.

Related meroku documents:

- `ai_docs/ARCHITECTURE_V4_DECISIONS.md`: the decisions, limitations and defects that V5 builds on.
- `ai_docs/EGRESS_COST_MODEL.md`, `ai_docs/EC2_COMPUTE_POOLS.md`, `ai_docs/MIGRATIONS.md`.
