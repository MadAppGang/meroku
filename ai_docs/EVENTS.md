# Events: project bus, per-workload queues, strict PutEvents

A workload publishes events with `events:PutEvents` and receives them on its own SQS
queue. Both are declared in the YAML. Module: `modules/events`. Go: `app/events.go`.

The project bus comes in two kinds. `classic` (the default) is an EventBridge custom bus
with one rule per subscription. `enhanced` is the EventBridge V2 bus: one subscriber per
subscription, optional per-group ordering, and sharing with other AWS accounts. See
[The enhanced bus](#the-enhanced-bus). Go: `app/events_enhanced.go`. Modules:
`modules/event_bus_enhanced`, `modules/event_subscriber`.

## YAML

```yaml
event_bus: { enabled: true }            # creates the bus <project>-<env>
                                        # add type: enhanced for the V2 bus

workload:                               # the backend, keyed "backend"
  events:
    subscribes:
      - { detail_type: order.created }

services:
  - name: orders
    events:
      subscribes:
        - { detail_type: payment.completed, source: example.payments }
        - detail_type: "job.*"                                   # * is a wildcard (enhanced: prefix)
          source: example.jobs                                   # optional
          filters:                                               # optional
            - { field: detail.status, values: [done, failed] }
            - { field: detail.order.type, values: [retail] }
        - { name: users, detail_type_prefix: "user.", bus: default }
      queue: { visibility_timeout: 30, max_receive_count: 5 }   # optional
      produces:
        - { detail_types: [order.created, order.cancelled] }   # own_source_only: true by default
        - { any: true, bus: default }

scheduled_tasks:
  - name: nightly
    events:
      produces: [{ detail_types: [report.ready] }]

event_processor_tasks:
  - name: mailer
    rules:
      - { name: r1, sources: [myapp.orders], detail_types: [order.created] }   # project bus
      - { name: r2, sources: [aws.ecs], detail_types: [ECS Task State Change], bus: default }
```

| Field | Meaning |
|---|---|
| `event_bus.enabled` | `true` creates the project bus `<project>-<env>`. `EVENT_BUS_NAME` then names it in every container. |
| `event_bus.type` | `classic` (absent means classic) or `enhanced`. `enhanced` needs `enabled: true` and a supported region. See [The enhanced bus](#the-enhanced-bus). |
| `event_bus.share[]` | Enhanced only. Shares the bus with an organization, OU, account or sibling environment. See [Sharing the bus](#sharing-the-bus). |
| `bus` (any entry or rule) | `project` or `default`. Absent: the project bus when it is on, otherwise `default`. `project` with the bus off is a generate error. |
| `subscribes[]` | Backend and services only. Exactly one of `detail_type` (exact, or a wildcard pattern when it contains `*`) or `detail_type_prefix`, an optional `source`, optional `filters`, an optional `name`. |
| `subscribes[].ordered` | Enhanced project bus only. `true` delivers the entry to the consumer's FIFO queue, in order per event group. See [Ordered delivery](#ordered-delivery). |
| `subscribes[].filters[]` | Optional, at most 5. `field` is a path under the event's detail, `^detail(\.[A-Za-z0-9_-]+)+$` (`detail.status`, `detail.order.type`); `values` is 1–10 non-empty strings, any of which matches. See [Wildcards and filters](#wildcards-and-filters). |
| `queue.visibility_timeout` | 0–43200 seconds, default 30. Set it above the longest time a message takes to handle. |
| `queue.max_receive_count` | 1–1000, default 5. Receives before SQS moves a message to the application DLQ. |
| `queue.fifo_retired` | `true` keeps a deployed FIFO queue set after the consumer's last ordered subscription is gone. See [Removing the last ordered subscription](#removing-the-last-ordered-subscription). |
| `produces[]` | Exactly one of a `detail_types` list or `any: true`. `own_source_only` (default `true`) limits the entry to events whose `source` is the workload's own `EVENT_SOURCE`. |
| `event_processor_tasks[].rules[].bus` | The bus the rule is on. The legacy single-rule form (`rule_name`) follows the environment's bus. |

AWS services (`aws.ecs`, `aws.ecr`, ...) and meroku's CI events publish to the **default**
bus only. A subscription or rule for them needs `bus: default` when the project bus is on.

## What a consumer gets

A workload with at least one `subscribes` entry is a consumer. Its running copies share one
queue, so each event is handled by one copy. On an enhanced bus, a workload that keeps a
retired FIFO queue set (`queue.fifo_retired`) is also a consumer, with no subscriptions.

The table below is the classic bus, and every `bus: default` entry on either kind. On an
enhanced bus a project-bus subscription gets a subscriber instead of a rule, and the queues
stay the same; see [What an enhanced consumer gets](#what-an-enhanced-consumer-gets).

| Resource | Name (form 2 of `modules/naming`) | Settings |
|---|---|---|
| Main queue | `<project>-events-<name>-<env>` | SSE-SQS, 4-day retention, 20 s long polling, redrive to the application DLQ |
| Application DLQ | `<project>-events-<name>-dlq-<env>` | SSE-SQS, 14 days. Messages the app failed `max_receive_count` times |
| Delivery DLQ | `<project>-events-<name>-delivery-dlq-<env>` | SSE-SQS, 14 days. Events EventBridge could not deliver |
| One rule per subscription | `<project>-events-<name>-<key>-<env>` | On the subscription's bus; target retry 185 attempts within 24 h, then the delivery DLQ |
| Alarm per DLQ | `<project>-<env>-events-<name>-dlq` and `...-delivery-dlq` | `ApproximateNumberOfMessagesVisible` Maximum > 0 over 300 s, missing data not breaching |
| `EVENT_QUEUE_URL` | container variable | the main queue's URL |
| `EVENT_DLQ_URL` | container variable | the application DLQ's URL |

The rule pattern is the detail-type (exact, `{"prefix": ...}` or `{"wildcard": ...}`), the
`source` when set, the filters under `detail` when set, and `"replay-name": [{"exists": false}]`:
an archive replay is never delivered to a queue.

The main queue and the delivery DLQ each have a queue policy that lets `events.amazonaws.com`
send only for this consumer's own rules (`aws:SourceArn`). The application DLQ needs none.

The task role gets, in the inline policy `<project>_events_<name>_<env>`:

- on the main queue: `sqs:ReceiveMessage`, `DeleteMessage`, `ChangeMessageVisibility`,
  `GetQueueAttributes`, `GetQueueUrl` and `SendMessage`. `SendMessage` lets the app put a
  message back with `DelaySeconds` to retry it later;
- on the application DLQ: `sqs:SendMessage`, `GetQueueAttributes` and `GetQueueUrl`, so the app
  can dead-letter a message it can never process (an unreadable body, an invalid event) at once
  instead of failing it `max_receive_count` times;
- nothing on the delivery DLQ, which only EventBridge writes to.

These grants cover everything a consumer does with its own queues. A `policy` entry naming
a meroku queue ARN is never needed; to reach another workload, publish an event with
`produces`.

### Subscription keys

The key is the rule's `for_each` key and the last part of its name, so reordering entries
changes nothing and editing one entry touches only its own rule.

- With `name` (`^[a-z0-9][a-z0-9-]{0,31}$`): the name.
- Without: the `detail_type`, or the `detail_type_prefix` followed by `-prefix`, lower-cased,
  each run of characters outside `[a-z0-9]` replaced by `-`, trimmed and cut to 32. `-default`
  is appended when the entry is on the default bus. `payment.completed` → `payment-completed`;
  `user.` on the default bus → `user-prefix-default`; `job.*` → `job` (a `*` is replaced like
  any other character outside `[a-z0-9]`), so `job.*` and an exact `job` on one bus need a
  `name`. A `detail_type` of only `*` derives no key and always needs a `name`.

Two entries of one consumer with the same key are a generate error. Give one a `name`
(typical case: one detail-type from two sources).

### Wildcards and filters

`modules/events` builds each rule's pattern from the entry:

| Entry | `event_pattern` (keys as Terraform sorts them) |
|---|---|
| `detail_type: job.completed` | `{"detail-type":["job.completed"],"replay-name":[{"exists":false}]}` |
| `detail_type_prefix: "user."` | `{"detail-type":[{"prefix":"user."}],"replay-name":[{"exists":false}]}` |
| `detail_type: "job.*"` | `{"detail-type":[{"wildcard":"job.*"}],"replay-name":[{"exists":false}]}` |
| `job.*` + `source: example.jobs` + `filters: [{ field: detail.status, values: [done, failed] }, { field: detail.order.type, values: [retail] }]` | `{"detail":{"order":{"type":["retail"]},"status":["done","failed"]},"detail-type":[{"wildcard":"job.*"}],"replay-name":[{"exists":false}],"source":["example.jobs"]}` |

An entry without a `*` and without `filters` renders the pattern it always had, so upgrading
updates no rule. `modules/events/tests/subscriptions.tftest.hcl` pins those strings.

EventBridge's `wildcard` operator, as it applies here:

- `*` matches any run of characters, including none: `job.*` matches `job.created` and
  `job.`, not `job`; `*.created` matches `order.created`; `job.*.done` matches
  `job.export.done`. Matching is case-sensitive.
- A `detail_type` without `*` is an exact match, never a wildcard.
- A `detail_type` of only `*` matches every event on the bus, AWS services' events included on
  the default bus. Generate allows it with a warning.
- `detail_type_prefix` stays a plain prefix and cannot contain `*`; `detail_type: "user.*"` is
  the wildcard spelling of the same match, but it renders a different pattern, so switching an
  entry from one to the other updates its rule in place.

Filters:

- Each filter adds one key under `detail`, nested along the path: `detail.order.type` becomes
  `{"detail": {"order": {"type": [...]}}}`. Filters with a common parent share one object.
- A field matches when its value equals one of `values` exactly (case-sensitive). When the
  field in the event is an array, it matches when any element equals one of `values`. An event
  without the field does not match.
- Values are strings and EventBridge compares types: `values: ["3"]` does not match the number
  `3`, and `values: ["true"]` does not match the boolean `true`. Filter on string fields.
- All filters, the detail-type and the source must match together (AND); the values of one
  filter are alternatives (OR).

## What a producer gets

Nothing publishes without a `produces` entry: the owning modules no longer grant
`events:PutEvents` at all. Each entry adds one statement to the workload's inline events
policy:

```json
{ "Action": "events:PutEvents", "Resource": "<bus ARN>",
  "Condition": {
    "ForAllValues:StringEquals": { "events:detail-type": ["order.created"], "events:source": ["myapp.orders"] },
    "Null": { "events:detail-type": "false", "events:source": "false" } } }
```

`ForAllValues` is also true for a request that carries no such key, so the `Null` guard
requires it. `any: true` drops the detail-type condition (generate warns). `own_source_only:
false` drops the source condition. A batch with one entry outside the list is denied as a
whole.

A `detail_types` value may contain `*` (`order.*`). That entry's operator becomes
`ForAllValues:StringLike`, which treats `*` as any run of characters; the source value has
no `*`, so it still matches exactly. An entry without `*` keeps `StringEquals`. A value of
only `*` is refused at generate: `any: true` says the same. The web UI starts a new entry
with "Source required" off, writing `own_source_only: false`; an entry without the key keeps
the source condition.

IAM allows add up, so this holds only while nothing else on the task role grants
`events:*`. The task roles of the backend, services, pgAdmin and both task kinds no longer
carry the AWS-managed `CloudWatchFullAccess` (which includes `events:*` and `sns:*`); an
inline `cloudwatch` policy grants `cloudwatch:PutMetricData` and writes to the workload's
own log group, plus the Container Insights log groups for the backend's ADOT collector and
X-Ray writes for a service's X-Ray daemon when `xray_enabled`. Execution roles keep their
attachment; a container never sees their credentials. `app/iam_scope_test.go` refuses a
`CloudWatchFullAccess` attachment on a task role and a `events:PutEvents` grant outside
`modules/events` (except the ECR-forwarding rule role and the GitHub deploy role). Code
that used `CloudWatchFullAccess` for anything else needs a `policy` block now; the list
below says exactly what is still granted.

### What a task role has instead of `CloudWatchFullAccess`

`CloudWatchFullAccess` granted `cloudwatch:*`, `logs:*`, `sns:*`, autoscaling and
application-autoscaling `Describe*`, `iam:GetPolicy`/`GetPolicyVersion`/`GetRole` and
`oam:ListSinks` on every resource. A task role now has:

| Role | Grant | Resource |
|---|---|---|
| every task role (backend, service, pgAdmin, scheduled task, event task) | `cloudwatch:PutMetricData` | `*` |
| every task role | `logs:CreateLogStream`, `PutLogEvents`, `DescribeLogStreams` | its own log group and its streams |
| backend, `xray_enabled` (ADOT collector) | `logs:CreateLogGroup`, `CreateLogStream`, `PutLogEvents`, `DescribeLogStreams`, `DescribeLogGroups` | `/aws/ecs/containerinsights/*` (the Container Insights `performance` group) |
| backend, `xray_enabled` (ADOT collector) | `logs:CreateLogGroup`, `CreateLogStream`, `PutLogEvents`, `DescribeLogStreams` | `/aws/ecs/application/metrics` (the EMF group for metrics the app sends the collector over OTLP or StatsD) |
| backend, `AWSXrayWriteOnlyAccess` (managed) | X-Ray writes | `*` |
| service, `xray_enabled` (X-Ray daemon) | `xray:PutTraceSegments`, `PutTelemetryRecords`, `GetSamplingRules`, `GetSamplingTargets`, `GetSamplingStatisticSummaries` | `*` |
| backend, `setup_fcnsns` | `sns:CreatePlatformEndpoint`, `Publish`, `GetEndpointAttributes`, `SetEndpointAttributes`, `DeleteEndpoint` | the FCM platform application (`app/GCM/<project>-fcm-<env>`). SNS checks all five against the application ARN, endpoint calls included; a grant on `endpoint/GCM/<app>/*` authorises none of them |
| backend, `extensions.sns_topics[]` with `add_to_backend_env` | `sns:Publish` | exactly those topics (inline policy `sns-publish`) |

Everything else the code calls in these services has to be declared in the workload's
`policy` ([Workload IAM](./WORKLOAD_IAM.md)), for example:

- writing to any other log group (a library that ships to a group of its own name);
- reading logs (`logs:GetLogEvents`, `FilterLogEvents`, Logs Insights `StartQuery`);
- reading CloudWatch (`cloudwatch:GetMetricData`, `DescribeAlarms`, `ListMetrics`) or
  setting alarms (`PutMetricAlarm`);
- SNS beyond the meroku-managed topics above: publishing to a topic created outside
  meroku or by custom Terraform, `sns:Subscribe`, `CreateTopic`, SMS (`Publish` to a phone
  number needs `Resource: "*"`), or a service or task publishing to an extension topic
  (only the backend receives `add_to_backend_env` ARNs, so only it is granted);
- the autoscaling, IAM and OAM reads the managed policy carried.

**During the apply that makes the switch**, removing the managed-policy attachment and
creating the inline policies are independent resources, and Terraform may run the detach
first. For a few seconds, plus IAM propagation, a running container can get AccessDenied
on its own `PutLogEvents` or `PutMetricData`; logs shipped through stdout (`awslogs`) are
unaffected because they use the execution role. No role, task definition or service is
replaced.

`EVENT_SOURCE` per kind, pinned by `app/testdata/event_source_vectors.json`:

| Kind | `EVENT_SOURCE` |
|---|---|
| backend | `<project>.backend` |
| service | `<project>.<service>` |
| scheduled task | `<project>.task.<task>` |
| event task | `<project>.event.<task>` |

The owning modules keep `DescribeEventBus`, `ListRules`, `DescribeRule`, `ListTargetsByRule`
and `ListEventBuses` on the default bus, the project bus (only when it is on) and `rule/*`.
`event-bus/<project>-*` is gone. On an enhanced bus this statement still names the classic
ARN `event-bus/<project>-<env>`, which matches no bus. Three more statements cover the
enhanced bus ([Read grants on an enhanced bus](#read-grants-on-an-enhanced-bus)).

## Writing a consumer

- **Be idempotent.** SQS delivers at least once, and EventBridge retries. Use the event `id`
  (or a business key) to skip duplicates.
- **Delete after handling.** A message not deleted reappears after `visibility_timeout` and
  counts one more receive. After `max_receive_count` it moves to the application DLQ.
- **Long polling** is already on (`receive_wait_time_seconds = 20`); `WaitTimeSeconds` on the
  call can stay unset.
- The SQS message body is the whole EventBridge event (`version`, `id`, `detail-type`,
  `source`, `account`, `time`, `region`, `resources`, `detail`). Its `id` is the `EventId`
  that PutEvents returned. Both bus kinds deliver this body.

## Dead letters

- **Application DLQ**: the app received the message `max_receive_count` times and never
  deleted it. Fix the bug, then redrive: SQS console → the DLQ → *Start DLQ redrive* (back to
  the source queue), or `aws sqs start-message-move-task --source-arn <dlq arn>`.
- **Delivery DLQ**: EventBridge could not deliver (the queue policy refused, the queue was
  gone, or 24 h of retries ran out). The message attributes carry `ERROR_CODE` and
  `ERROR_MESSAGE`. These events never reached the main queue; replay them by publishing the
  `detail` again.
- **The 6-hour metric gap.** SQS stops publishing `ApproximateNumberOfMessagesVisible` for a
  queue that has been inactive for 6 hours, and starts again on the next message. With
  `treat_missing_data = notBreaching` the alarm sits in OK during the gap and fires on the
  first new dead letter.

## The alerts topic

The alarms publish to `<project>-<env>-alerts`, created only when the environment has at
least one consumer, so an environment without consumers gets no change. **Nothing subscribes
to it.** Add a subscription yourself, from custom Terraform:

```hcl
resource "aws_sns_topic_subscription" "oncall" {
  topic_arn = local.bridge.alerts_topic_arn
  protocol  = "email"
  endpoint  = "ops@example.com"
}
```

The topic is **unencrypted**. CloudWatch alarm actions cannot publish to a topic encrypted
with the AWS-managed SNS key (`alias/aws/sns`), whose key policy cannot be changed. The
alternative, a customer-managed key whose policy allows `cloudwatch.amazonaws.com`, was not
taken: meroku uses built-in encryption only. The messages carry the alarm name and state.

An `extensions.sns_topics` entry named `alerts` would claim the same topic name; generate
refuses it while the environment has consumers.

## SSE-SQS

All three queues use SSE-SQS, the encryption SQS manages itself (`sqs_managed_sse_enabled`),
and there is no KMS option. A queue under the AWS-managed key `alias/aws/sqs` cannot be an
EventBridge target, because that key's policy cannot grant EventBridge; SSE-SQS needs no KMS
grant. A delivery failure shows as `FailedInvocations` on the rule and as messages in the
delivery DLQ.

## Switching the bus on or off

Turning `event_bus.enabled` on:

- creates `<project>-<env>`, and every container's `EVENT_BUS_NAME` names it (one new task
  definition revision for every workload);
- moves every entry and event-task rule **without** `bus` onto it. Rules move by replacement
  (`event_bus_name` is ForceNew);
- moves PutEvents grants of entries without `bus` to the project bus.

Generate prints a warning listing every entry and rule that moves, when the bus state differs
from the one `env/<env>/main.tf` was last generated with. To keep an entry where it is, set
`bus: default` on it. Turning the bus off is the same in reverse; an entry with `bus: project`
is then a generate error.

## The enhanced bus

```yaml
region: ap-southeast-2
event_bus:
  enabled: true
  type: enhanced            # classic | enhanced; absent = classic
  share:                    # optional, see Sharing the bus
    - { env: staging, access: publish }

services:
  - name: orders
    events:
      subscribes:
        - { detail_type: payment.completed }              # standard subscriber
        - { detail_type: "job.*" }                        # prefix "job."
        - { detail_type: "*.created" }                    # suffix ".created"
        - { name: ledger, detail_type_prefix: "ledger.", ordered: true }   # FIFO queue
        - { detail_type: ECS Task State Change, source: aws.ecs, bus: default }  # classic rule
      produces:
        - { detail_types: [order.created] }
```

`event_bus.type: enhanced` makes the project bus `<project>-<env>` an EventBridge V2 bus
(`awscc_eventsv2_event_bus`, AWS Cloud Control provider `hashicorp/awscc ~> 1.110`). Absent
`type` is `classic` and renders exactly the Terraform it rendered before `type` existed.
No migration adds the key.

### Classic and enhanced compared

| | classic | enhanced |
|---|---|---|
| Delivery to a consumer queue | one EventBridge rule per subscription | one subscriber per subscription; one FIFO subscriber per consumer for its ordered subscriptions |
| Publish with | the `eventbridge` client and `EVENT_BUS_NAME` | the `eventbridgev2` client and `EVENT_BUS_ARN` |
| Ordered delivery | no | `ordered: true`, per event group |
| Share with other accounts | no | `share`, through AWS RAM |
| `detail_type` with `*` | any wildcard | one leading or trailing `*` only |
| Event-task rules on the project bus | yes | refused; use `bus: default` |
| Archive replay | `replay-name` excluded from every pattern | no archive; events stay on the bus 16 days |
| Regions | every region | the list below |
| Price | per event published | by data volume: ingress, egress per subscriber delivery (retries billed), storage beyond the free period, and evaluations for JSONata transforms |
| Quota | 100 custom buses per account and region | 5 enhanced buses per account and region (adjustable); 10,000 subscribers per bus |

Every `bus: default` subscription, and every AWS service event, stays on the default bus
with a classic rule on both kinds.

Regions where generate accepts `type: enhanced` (`enhancedEventBusRegions` in
`app/events_enhanced.go`, mirrored in `web/src/utils/workloadEvents.ts`): `ap-east-1`,
`ap-northeast-1`, `ap-south-1`, `ap-southeast-1`, `ap-southeast-2`, `ap-southeast-5`,
`ap-southeast-7`, `eu-central-1`, `eu-north-1`, `eu-south-2`, `eu-west-1`, `us-east-1`,
`us-east-2`, `us-west-2`. Add a region there when the
[EventBridge feature availability page](https://docs.aws.amazon.com/eventbridge/latest/userguide/feature-availability.html)
lists it. The 5-bus quota means at most 5 enhanced environments in one account and region
unless you raise it in Service Quotas.

### What an enhanced consumer gets

Every consumer keeps its standard queue set (main queue, application DLQ, delivery DLQ, two
alarms, `EVENT_QUEUE_URL`, `EVENT_DLQ_URL`), also when all its subscriptions are ordered.
On top of that:

| Resource | Name (form 2 of `modules/naming`) | When |
|---|---|---|
| Standard subscriber | `<project>-events-<consumer>.<key>-<env>` | each unordered project-bus subscription |
| FIFO subscriber | `<project>-events-<consumer>-fifo-<env>` | one per consumer with an ordered subscription |
| Delivery role | `<project>_events_<consumer>_delivery_<env>`, path `/meroku-events/<project>/<env>/` | a consumer with at least one subscriber |
| FIFO main queue | `<project>-events-<consumer>-<env>.fifo` | an ordered subscription, or `fifo_retired` |
| FIFO application DLQ | `<project>-events-<consumer>-dlq-<env>.fifo` | same |
| FIFO delivery DLQ | `<project>-events-<consumer>-delivery-dlq-<env>.fifo` | same |
| Alarms | `<project>-<env>-events-<consumer>-fifo-dlq`, `...-fifo-delivery-dlq` | same |
| `EVENT_FIFO_QUEUE_URL`, `EVENT_FIFO_DLQ_URL` | container variables | same |
| `EVENT_BUS_ARN` | container variable | every workload of an enhanced environment |

A subscriber name over 64 characters falls to form 3 of `modules/naming`, which starts with
`events-` and ends with an 8-character hash. The `.` in a standard subscriber's name splits
it back into one consumer and one key, so no two subscriptions share a name.

Each subscriber has:

- two filters: a `DATA` filter built from the entry (below) and the `SYSTEM_METADATA` filter
  `{"ContentType":["application/eventbridge+json"]}`, the content type of every `PutEvents`
  entry. A `PutRawEvents` payload sent with another content type reaches no meroku queue (a live
  check with `application/json` was dropped). `PutRawEvents` takes the content type from the
  caller, and a raw payload that declares `application/eventbridge+json` itself was not tested:
  do not count on the filter to keep such a payload out;
- a JSONATA transformer that writes the classic envelope, with
  `id = $events.SystemMetadata."aws:EventId"`, the `EventId` PutEvents returned. The
  other eight keys are copied from `$events.Data`;
- retry policy 185 attempts within 86,400 seconds (24 h), then the delivery DLQ;
- starting position `LATEST`: a new subscriber gets events published after it exists;
- tags `Project`, `Environment`, `ManagedBy`, `Application` (awscc has no `default_tags`).

The delivery role trusts `events.amazonaws.com` with `aws:SourceAccount` set to the
account. Its inline policy `deliver` grants `sqs:SendMessage` on every queue of its
consumer: main and delivery DLQ, plus FIFO main and FIFO delivery DLQ while the FIFO set
exists. The grant covers queues no current subscriber targets, so a subscriber that is being
replaced can still deliver until it is deleted. No queue policy is added for subscribers.

### Detail types and filters on the enhanced bus

AWS rejects the `wildcard` operator in a subscriber filter. meroku maps the two shapes it can
express and refuses every other `*`:

| `detail_type` | DATA pattern |
|---|---|
| `payment.completed` | `{"detail-type":["payment.completed"]}` |
| `"job.*"` | `{"detail-type":[{"prefix":"job."}]}` |
| `"*.created"` | `{"detail-type":[{"suffix":".created"}]}` |
| `detail_type_prefix: "user."` | `{"detail-type":[{"prefix":"user."}]}` |
| `"job.*.done"`, `"*"`, `"**"`, `"a*b"` | refused at generate |

`source` and `filters` add the same keys as on a classic rule. There is no `replay-name` key.
A FIFO subscriber's DATA pattern is the consumer's one ordered branch, or
`{"$or":[<branch>, ...]}` with every ordered subscription's branch sorted by key. An event that
matches two branches arrives once.

AWS limits each subscriber to 1,000 `$or` combinations and to 4,096 bytes of filter
configuration. AWS measures the JSON text
`{"Filters":[{"Pattern":"<DATA>","Scope":"DATA"},{"Pattern":"<SYSTEM_METADATA>","Scope":"SYSTEM_METADATA"}]}`,
with every `"` and `\` inside a pattern escaped. A pattern without backslashes therefore
has 3,960 bytes, minus one byte for each `"` it holds. Generate measures the same JSON text and
refuses a subscriber whose filters are over 4,096 bytes; `modules/event_subscriber` checks it
again at plan. Both escape `<`, `>` and `&` as `\u003c` and so on, which AWS does not, so a
pattern holding those characters is refused a little before AWS would refuse it, never after.

### What the enhanced bus cannot do

- Start ECS tasks. An `event_processor_tasks` rule on the project bus is refused; set
  `bus: default` on it, or subscribe a service instead. The legacy single-rule form follows
  the project bus, so it is refused too.
- Match a `*` in the middle of a detail type, or a lone `*`.
- Deliver `PutRawEvents` payloads to meroku queues, except a raw payload that declares the
  `PutEvents` content type itself (see the filters above).
- Accept the classic `eventbridge` client. A producer publishes with `eventbridgev2`.
- Order events across event groups, or events without an `EventGroupId`.

### Publishing to the enhanced bus

A producer's `events:PutEvents` grant keeps its conditions on `events:detail-type` and
`events:source`. Its resource is the enhanced bus ARN, so a publish to the classic name
`<project>-<env>` is denied. `PutRawEvents` is not granted.

```go
import (
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/eventbridgev2"
	"github.com/aws/aws-sdk-go-v2/service/eventbridgev2/types"
)

client := eventbridgev2.NewFromConfig(cfg)
out, err := client.PutEvents(ctx, &eventbridgev2.PutEventsInput{
	EventBusArn: aws.String(os.Getenv("EVENT_BUS_ARN")),
	Entries: []types.PutEventsRequestEntry{{
		Source:         aws.String(os.Getenv("EVENT_SOURCE")),
		DetailType:     aws.String("ledger.posted"),
		Detail:         aws.String(`{"ledgerId":"L-1042","amount":1250}`),
		SystemMetadata: &types.PutEventsSystemMetadata{EventGroupId: aws.String("L-1042")}, // order key
	}},
})
// out.Entries[0].EventId is the id the consumer reads in the message body.
```

`EVENT_BUS_ARN` is `arn:aws:events:<region>:<account>:event-busv2/<project>-<env>/<id>`.
`EVENT_BUS_NAME` keeps the value `<project>-<env>`. Leave `SystemMetadata` out for events
that need no order.

### Ordered delivery

```yaml
services:
  - name: orders
    events:
      subscribes:
        - { name: ledger, detail_type_prefix: "ledger.", ordered: true }
        - { name: refunds, detail_type: refund.issued, ordered: true }
```

The consumer gets one FIFO subscriber that matches both entries and delivers to
`EVENT_FIFO_QUEUE_URL`. Each SQS message has:

- `MessageGroupId` = `<publishing account>:<EventGroupId>`, for example
  `000000000000:L-1042`. The account is read from the event's `account` field;
- `MessageDeduplicationId` = the event id.

The guarantee: for one consumer, events that one AWS account publishes to the project bus
with the same `EventGroupId`, and that match any of the consumer's ordered subscriptions,
arrive in its FIFO queue in the order the bus accepted them. Each arrives once, however many
ordered subscriptions match it. The same group id from two accounts is two groups, with no
order between them, as far as it was tested (see Shared publishers below). Events in different
groups have no order between them.

Limits of that guarantee:

- **Without `EventGroupId`** AWS fills in a random UUID, so every such event is its own
  group and has no order.
- **`EventGroupId` length.** SQS allows 128 characters of letters, digits and punctuation in a
  `MessageGroupId`, and the account prefix takes 13. Keep `EventGroupId` at most 115
  characters. PutEvents accepts 116–128 characters, and then the delivery fails with
  `CUSTOMER_VALIDATION` and goes to the FIFO delivery DLQ. PutEvents refuses 129 or more.
- **At-least-once.** Dedupe on `id`. A subscription change can deliver an event twice for a
  short time.
- **A stuck delivery holds up the consumer.** A delivery that keeps failing on the queue's side
  holds up every group of that consumer for up to 24 hours. An internal EventBridge failure can
  hold it up longer without writing a DLQ record. Watch the FIFO delivery DLQ alarm and the
  subscriber's metrics ([Stalled subscribers](#stalled-subscribers)).
- **Shared publishers.** The per-account grouping was tested with the bus owner's account
  publishing. For an account the bus is shared with, it holds only if EventBridge writes that
  account into the event's `account` field, as a classic bus does. No second account was
  available to test it.
- Throughput is 1,500 events per second per event group (AWS quota, not adjustable).

**Retrying a message on a FIFO queue.** A FIFO queue has no per-message `DelaySeconds`, so
putting a message back with a delay is not available.

To retry, do not delete the message. SQS shows it again after `visibility_timeout`, and the rest
of its group waits meanwhile. After `max_receive_count` receives it moves to the FIFO
application DLQ (`EVENT_FIFO_DLQ_URL`).

A message you can never process goes to `EVENT_FIFO_DLQ_URL` at once, with `SendMessage` and a
`MessageGroupId` (content-based deduplication is on). The task role has the same actions on
the FIFO queues as on the standard ones (Sids `EventFifoQueue`, `EventFifoDeadLetterQueue`).

### Removing the last ordered subscription

Turning off or deleting a consumer's last ordered subscription would delete its FIFO queues and
any events still in them. Generate refuses it while the FIFO subscriber is deployed. It is two
deploys:

1. Set `fifo_retired: true` and drop the ordered entry. This works also when that entry was the
   consumer's only subscription:

   ```yaml
   services:
     - name: orders
       events:
         queue: { fifo_retired: true }
         subscribes:
           - { detail_type: payment.completed }   # or no subscribes at all
   ```

   The deploy deletes the FIFO subscriber. The FIFO queues, their alarms, the IAM grants and
   `EVENT_FIFO_*` stay.
2. Read what `EVENT_FIFO_QUEUE_URL` and `EVENT_FIFO_DLQ_URL` still hold. Then remove
   `fifo_retired`. The next deploy deletes the FIFO set.

`fifo_retired: true` together with an `ordered: true` entry is refused. `fifo_retired: true`
with no deployed FIFO set renders nothing and gives the warning `event_fifo_retired_unused`.

Removing a consumer's last subscription of any kind deletes its standard queues, as on the
classic bus; generate warns with `event_queue_deleted` on an enhanced environment.

### Subscription changes: create before delete

A subscriber's name, type and target cannot change in place, so every change that needs a new
subscriber is a new Terraform module address: `module.event_subscriber_<consumer>_<key>` for a
standard subscriber, `module.event_fifo_subscriber_<consumer>` for a FIFO one. This happens
when you rename a subscription (`name`, or a derived key), toggle `ordered`, or add or remove
ordered subscriptions. A filter change on the same address is an in-place update.

- A module that is no longer wanted stays in main.tf as a tombstone (`enabled = false`) until
  its subscriber is destroyed.
- AWS allows one subscriber create or delete per bus at a time, and awscc does not retry the
  conflict. All subscriber modules are therefore one `depends_on` chain: tombstones, then new
  modules, then modules already in state.
- `create_before_destroy` on the subscriber makes every delete wait until the creates are
  done. For a short time both the old and the new subscriber deliver, and the consumer gets
  some events twice. Dedupe on `id`.
- Generate refuses a new subscriber name that a deployed subscriber of another module still
  uses.

**Known limitation: events still being retried are lost.** A new subscriber starts at
`LATEST`. An event the old subscriber matched before the new one existed, and was still
retrying when the old one was deleted, is not delivered and not written to any DLQ. AWS gives
no signal of pending deliveries, so meroku cannot wait for them. This needs a target that is
already failing during the change.

Before you rename a subscription or toggle `ordered`,
check that the consumer's delivery DLQ alarms are OK and its delivery metrics are normal.

Other accounts on a shared bus also create and delete subscribers. One of their operations can
collide with the apply (`ConcurrentModificationException`). Run the apply again.

### Dead letters on the enhanced bus

A subscriber's delivery-DLQ record points at events. It does not contain them. In the record
of a FIFO subscriber:

- the message attributes are `ERROR_CODE`, `EXHAUSTED_RETRY_CONDITION`, `RETRY_ATTEMPTS`,
  `SUBSCRIBER_ARN`, `TARGET_ARN`, `BUS_ARN`;
- the body's `failedMessages[]` names each failed event by `eventId` and `eventGroupId`. One
  record can name several events (one failed batch);
- the record's `MessageGroupId` is the raw `EventGroupId`, without the `<account>:` prefix.

The events stay on the bus for 16 days after they are published (`retention_period_in_days =
16`). A record arrives at most 24 hours after publish, and the delivery DLQ keeps it for 14
days. So a failure record can be replayed while it is in the delivery DLQ.

To replay, create a
temporary subscriber outside meroku with starting position `POINT_IN_TIME` and a
`SYSTEM_METADATA` filter on `aws:EventId`. Delete it afterwards. Storage beyond the free period
is billed per GB-month.

### Stalled subscribers

A quiet DLQ does not prove a healthy subscriber. AWS retries some internal failures without a
limit and writes no DLQ record for them. The DLQ alarms catch deliveries that ran out of
retries, not a subscriber that stalled. A stuck FIFO subscriber holds up every group of its
consumer.

meroku creates no stall alarm, because no AWS metric counts pending deliveries. Watch these
`AWS/EventsV2` metrics, dimensions `Subscriber` = `<name>/<id>` and `EventBus` =
`<name>/<id>`:

| Signal | Meaning |
|---|---|
| `ApproximateBacklogAge` growing | deliveries are retrying. It is emitted only in minutes with delivery activity |
| `EventDeliveryAttempts` well above `EventsDelivered`, with no `OnFailureDestinationDelivered` | the target refuses and retries are not yet exhausted |
| `EventTransformationFailures` | the envelope transformer failed |
| `FilterMatched` with no `TargetInvocations` | events match but are not sent |

### Sharing the bus

```yaml
event_bus:
  enabled: true
  type: enhanced
  share:
    - { organization: true, access: publish }      # the AWS Organization of this account
    - { ou: ou-ab12-cd34ef56, access: subscribe }  # an organizational unit
    - { account: "000000000000", access: full }    # 12 digits, quoted
    - { env: staging, access: publish }            # a sibling env; meroku uses its account_id
```

| `access` | RAM managed permission | The other account may |
|---|---|---|
| `publish` | `AWSRAMEventBridgeEventBusV2PublishOnly` | publish, describe the bus |
| `subscribe` | `AWSRAMEventBridgeEventBusV2SubscribeOnly` | create its own subscribers, and so read **every** event on the bus |
| `full` | `AWSRAMEventBridgeEventBusV2FullAccess` | publish, subscribe, attach event sources |

`modules/event_bus_enhanced` creates one `aws_ram_resource_share` per access level, named
`<project>-<env>-events-<access>`, with the bus and the principals of that level.

- An `env:` entry names a sibling environment of the **same `project`**, found among the YAML
  files in this env file's own directory. It must have an `account_id`, be declared by one file
  only, and not be this env's account. `GET /api/event-bus/share-targets?env=<env>` lists
  every sibling with its status: `ok`, `ambiguous`, `no_account` or `own_account`.
- An `account` entry sets `allow_external_principals = true` on that share. An account outside
  the organization gets an invitation it has to accept. An org/OU-only share keeps it `false`.
- Each principal appears once. `access: full` covers publish and subscribe.

Preconditions of an `organization` or `ou` entry:

1. The deploying account belongs to an AWS Organization, and the deployer may call
   `organizations:DescribeOrganization`. Without an organization the plan fails with AWS's
   error from `aws_organizations_organization`.
2. RAM sharing with AWS Organizations is enabled. Only the management account can enable it
   (`aws ram enable-sharing-with-aws-organization`), and a member account cannot see whether
   it is on. When it is off, the apply fails at `aws_ram_principal_association` with
   `UnknownResourceException: Organization o-... could not be found`.
3. An `ou` id belongs to that organization.

Security notes:

- A `publish` or `full` account can publish any `source` and `detail-type`. A subscription's
  `source` filter does not prove who sent an event.
- A `subscribe` or `full` account can create a subscriber that matches everything.
- CreateSubscriber accepts a role or target that does not exist. A wrong grant shows only at
  delivery, through the delivery DLQ alarm.
- A shared account's `PutRawEvents` payloads match no meroku subscriber unless they declare
  `application/eventbridge+json` as their content type, which was not tested.

### Revoking shared subscribers

Unsharing alone does not stop subscribers another account already created. When a principal
leaves `share`, or changes to `publish`, the same apply revokes that account's subscribers on
the bus with `RevokeResource`. Generate warns with `event_share_revoke` first.

**Revocation is permanent.** The account must create new subscribers if you share again. A
revoked subscriber stops delivering. It stays in its owner's account until that account
deletes it.

How it runs: `terraform_data.revoke[0]` (the sweep) in `modules/event_bus_enhanced` runs
`scripts/revoke-subscribers.sh` after RAM stops granting access. The script lists the bus's
subscribers (`aws eventsv2 list-subscribers --event-bus-arn <exact ARN>`), skips this account's
own and those already revoked, and revokes every other account that no remaining entry entitles.
The principals it compares against come from Terraform state, so running generate twice or after
a failed apply computes the same loss.

The debt survives an apply that stops half way:

- `terraform_data.revoke_record[0]` is never replaced. It is updated in place only after the
  sweep of its round succeeded, so it still names every principal the last finished round
  granted. Terraform destroys the old sweep and a removed RAM association before it creates the
  new sweep; an apply killed in between leaves the record, and the next generate owes every
  principal it names that is no longer granted.
- Each revocation is a numbered `round` (a trigger of the sweep). The bus is settled only when
  the sweep succeeded, the record confirms its round, both were applied with today's subscribe
  principals, and RAM associates no principal outside them. Anything else starts a new round, so
  a principal that was revoked once, shared again by an apply that stopped before its sweep, and
  then removed again, is revoked again although `granted` and `lost` read as before.
- The accounting above is only as complete as the applies that wrote it. A grant that
  associates an account in RAM and stops before its sweep, then a rollback that removes the
  association and stops before its sweep, leave RAM, the sweep and the record all reading as the
  last settled round, and none of them names the account. So generate also lists the bus's
  subscribers (`eventsv2 ListSubscribers` on the bus ARN in state, with the env YAML's profile
  and region; `app/events_foreign.go`), whenever the YAML keeps the enhanced bus and the bus is
  or was shared (a share entry, the guard, a RAM association or a revoke step in state). Every
  account with a subscriber that is not revoked, is not this account and has no `account` entry
  that may subscribe is owed (`account:<id>` in `lost`). Each round also passes `judged`: the
  ARNs of the listed subscribers no `account` entry names. The record takes `judged` only after
  the sweep succeeded. A subscriber the sweep revoked drops out of the listing; one a remaining
  `organization` or `ou` entry covers is kept and stays in the record's `judged`; one
  Organizations cannot classify stops the apply (below), because `lost` is not empty. The bus is
  settled only when every listed subscriber no `account` entry names is in the record's
  `judged`. The check is per subscriber, not per account: an account a past round revoked that
  creates a new subscriber during another pair of interrupted applies is in the record's `lost`,
  but its new subscriber is not in `judged`, so a new round revokes it. A listing that fails
  refuses the render, as an unreadable state does; the refusal names the IAM action
  `events:ListSubscribers` (resource `*`) and the CLI check `aws eventsv2 list-subscribers`.

- An `account` entry still present keeps that account.
- A remaining `organization` or `ou` entry keeps an account only if Organizations says it is
  inside. That needs `organizations:DescribeAccount` and `organizations:ListParents`, which only
  the management account or a delegated administrator can call. If the deployer gets
  `AccessDenied` while a principal was lost, the script revokes what it could decide, lists the
  undecided subscribers and **stops the apply** (exit 6). It never revokes on a guess. Two ways
  out: deploy with credentials that can call both actions; or add an explicit
  `account: "<id>"` entry with `access: subscribe` for each account that is still entitled, and
  revoke the rest yourself with `aws eventsv2 revoke-resource --arn <subscriber arn>`. The next
  apply runs the step again and skips subscribers already revoked.

Not covered:

- While the environment stays enhanced, only subscribers are revoked. Event sources another
  account attached under `full` stay attached after that entry is removed or downgraded.
  Revoke one with `aws eventsv2 revoke-resource --arn <event source arn>`.
- An account that leaves the organization or an OU while an org/OU entry remains keeps its
  subscribers once a round has judged them: the YAML does not change, and an account already in
  the record's `lost` is not owed again, so the step does not run. Revoke them by hand with the
  same command.
- With an `organization` or `ou` entry and a deployer that cannot call Organizations, the first
  apply after any account without an `account` entry creates a subscriber stops (exit 6), even
  when the account is inside: nothing tells it apart from an account that only a removed entry
  entitled. Add an `account` entry for it, or deploy with Organizations access.

Script exit codes: 0 done or nothing to do, 2 bad input (including a bus ARN of another
account), 3 AWS CLI missing or not v2, 5 any other AWS error, 6 undecided after a loss.

The script and the teardown below need `/bin/sh` and **AWS CLI v2** on the deploying machine.
Only environments that share or once shared run them: at apply, when leaving enhanced, and at
`terraform destroy`.

### Leaving the enhanced bus (cleanup)

Setting `type: classic` (or removing `type`, or turning the bus off) on a deployed enhanced
environment puts it in cleanup mode. meroku reads Terraform state on every generate and deploy
(see "What was deployed" below). While state holds the enhanced bus or a
subscriber, main.tf still renders the awscc provider, `module "event_bus_enhanced"` with
`enabled = false`, and every subscriber module as a tombstone. That apply deletes the
subscribers one at a time, then the bus, in the YAML's region.

- If the bus was ever shared, `terraform_data.bus_guard[0]` runs `revoke-subscribers.sh` in
  `teardown` mode on its destroy. It revokes every subscriber and event source of other
  accounts after RAM has stopped granting access and before the bus delete. `terraform destroy`
  of the whole environment runs it too.
- A revoked resource does not block the bus delete. An own-account subscriber or event source
  created outside meroku does: the script names it, and the delete fails with AWS's error. Delete
  it or keep the bus.
- Ordered subscriptions are refused on a classic bus. Set `fifo_retired: true` on each consumer
  that had them, then remove it after the queue is read (the two deploys above). A classic
  environment can hold a retired FIFO set; its main.tf then carries the FIFO resources and
  `# meroku:events` marker lines.
- When state shows nothing enhanced, the next render is the plain classic Terraform. The awscc
  entry stays in `.terraform.lock.hcl`, which does no harm.

### Switching a deployed environment to enhanced

The type change replaces the bus: the classic bus and its rules are deleted, the enhanced bus and
its subscribers are created. Events published during the deploy are not delivered. Generate
warns with `event_bus_type_change`. Procedure:

1. Check the region is in the list and the account has an enhanced bus left in its quota.
2. Move every event-task rule on the project bus to `bus: default`, or replace it with a
   subscribed service.
3. Rewrite `detail_type` entries with a `*` in the middle as exact entries or a prefix.
4. Ship producer code that publishes with `eventbridgev2` and `EVENT_BUS_ARN` when that
   variable is set, and with the classic client and `EVENT_BUS_NAME` otherwise.
5. Deploy `type: enhanced` at a quiet time. Consumers keep their queues and
   `EVENT_QUEUE_URL`.

A classic bus and an enhanced bus of the same name can exist at once, so this is one deploy.

### Read grants on an enhanced bus

On an enhanced environment the backend, service, scheduled-task and event-task roles get three
more statements beside the unchanged read statement:

| Sid | Actions | Resource |
|---|---|---|
| `EventBridgeEnhancedBusRead` | `events:DescribeEventBus`, `events:DescribeSubscriber` | the bus ARN; `subscriber/<project>-events-*-<env>/*` |
| `EventBridgeEnhancedShortNameSubscriberRead` | `events:DescribeSubscriber` | `subscriber/events-*/*`, with `aws:ResourceTag/Project` and `aws:ResourceTag/Environment` equal to this env |
| `EventBridgeEnhancedList` | `events:ListEventBuses`, `events:ListSubscribers` | `*` |

AWS supports no resource for the two list actions. `ListSubscribers` on `*` lets a workload list
the subscribers of any bus it can name: names and ARNs, not events.

### Deployer permissions

Whoever runs `terraform apply` on an enhanced environment needs these, beyond what a classic
environment needs. `<bus>` = `arn:aws:events:<region>:<account>:event-busv2/<project>-<env>/*`;
`<subs2>` = `arn:aws:events:<region>:<account>:subscriber/<project>-events-*-<env>/*`;
`<subs3>` = `arn:aws:events:<region>:<account>:subscriber/events-*/*`, always in its own
statement with a tag condition:

- create-time (`CreateSubscriber`, and the `TagResource` it makes): `aws:RequestTag/Project`,
  `aws:RequestTag/Environment`, and `ForAllValues:StringEquals aws:TagKeys = [Project,
  Environment, ManagedBy, Application]`;
- existing resource (every other subscriber action): `aws:ResourceTag/Project`,
  `aws:ResourceTag/Environment`.

| Actions | Resource |
|---|---|
| `cloudformation:CreateResource`, `GetResource`, `UpdateResource`, `DeleteResource`, `GetResourceRequestStatus`, `ListResources`, `ListResourceRequests` (Cloud Control, used by awscc) | `*` |
| `events:ListEventBuses`, `ListSubscribers`, `ListEventSources` | `*` (`ListSubscribers` also at `meroku generate`, for an environment that shares or once shared) |
| `events:CreateEventBus`, `DescribeEventBus`, `UpdateEventBus`, `DeleteEventBus` | `<bus>` |
| `events:CreateSubscriber` | `<bus>`, `<subs2>`, `<subs3>` create-time |
| `events:UpdateSubscriber` | `<bus>`, `<subs2>`, `<subs3>` existing |
| `events:DescribeSubscriber`, `DeleteSubscriber` | `<subs2>`, `<subs3>` existing |
| `events:RevokeResource` | `<bus>` (environments that share) |
| `events:TagResource` | `<bus>`, `<subs2>`; `<subs3>` once create-time and once existing |
| `events:UntagResource`, `ListTagsForResource` | `<bus>`, `<subs2>`, `<subs3>` existing |
| `iam:PassRole` | `arn:aws:iam::<account>:role/meroku-events/<project>/<env>/*`, condition `iam:PassedToService = events.amazonaws.com` |
| `iam:CreateRole`, `DeleteRole`, `GetRole`, `TagRole`, `UntagRole`, `PutRolePolicy`, `GetRolePolicy`, `DeleteRolePolicy`, `ListRolePolicies`, `ListAttachedRolePolicies`, `ListInstanceProfilesForRole` | the same role path |
| `ram:CreateResourceShare`, `UpdateResourceShare`, `DeleteResourceShare`, `AssociateResourceShare`, `DisassociateResourceShare`, `GetResourceShares`, `GetResourceShareAssociations`, `ListResourceSharePermissions`, `AssociateResourceSharePermission`, `TagResource` | `*` |
| `organizations:DescribeOrganization` | `*`, with an org/OU entry |
| `organizations:DescribeAccount`, `ListParents` | `*`, with an org/OU entry when a principal is lost; management account or delegated administrator only |

PassRole is scoped by the IAM path, not by role tags. The subscriber, tag and PassRole rows were
run live under a role limited to them. A full apply under such a role was not run.

### Test Event on the enhanced bus

The EventBridge node's **Test Event** publishes with `eventbridgev2`. It reads the profile's
account with `sts:GetCallerIdentity`, refuses a YAML `account_id` that differs (409), and pages
`ListEventBuses` for the bus whose name and owner account both match. Buses shared in from other
accounts can have the same name and are never used. No match gives 409 ("deploy the environment
first").

An optional `eventGroupId` sets `SystemMetadata.EventGroupId`; on any other bus it is
refused (400). A failed entry gives 500 with `Event failed: <ErrorCode>: <ErrorMessage>`.

## Generate-time warnings

Each is a `ConfigWarning` (`app/config_warnings.go`); generate prints it and carries on.

| Code | Item | When |
|---|---|---|
| `event_subscription_all_types` | the workload | A `detail_type` made only of `*` (a classic rule, so a classic project bus or the default bus): the workload receives every event on that bus |
| `event_produces_any` | the workload | A `produces` entry with `any: true` |
| `event_queue_unused` | the workload | `events.queue` on a workload that subscribes to nothing and keeps no FIFO set |
| `event_bus_toggle` | `event_bus` | `event_bus.enabled` differs from the last generated main.tf; lists every entry and rule that moves |
| `event_bus_type_change` | `event_bus` | The bus kind differs from the deployed one (state, else the last main.tf); the bus is replaced |
| `event_subscription_order_change` | the workload | A subscription's `ordered` changed since the last render; its events move to the other queue. It asks you to check the old queue's delivery DLQ alarm first, because events the old subscriber is still retrying are lost |
| `event_fifo_retired_unused` | the workload | `fifo_retired: true` with no ordered subscription and no deployed FIFO set |
| `event_queue_deleted` | the workload | Enhanced or cleanup: state holds a consumer's queues and the workload is no longer a consumer |
| `event_share_revoke` | `event_bus` | A principal may no longer subscribe, or the environment leaves an enhanced bus that was shared; subscribers are revoked |

Two checks the design kept in reserve are not in the code, because live checks in
ap-southeast-2 settled them: a classic and an enhanced bus of the same name exist at once, so a
type switch is one deploy (warned with `event_bus_type_change`, never refused); and a FIFO
subscriber's message group carries the publishing account, so the owner account and another
account that send the same `EventGroupId` keep separate groups, and no shared-publisher warning
was added. That was tested with the owner account publishing only; for an account the bus is
shared with, it holds if EventBridge writes that account into the event's `account` field (see
Shared publishers under [Ordered delivery](#ordered-delivery)).

## Generate-time errors

All bus kinds:

- Not exactly one of `detail_type` / `detail_type_prefix`; not exactly one of `detail_types` /
  `any: true`.
- A `*` in `detail_type_prefix`.
- A filter whose `field` is not a path under `detail`, whose `values` is empty, longer than 10
  or holds anything but non-empty strings; two filters on one field; a field inside another
  (`detail.order` with `detail.order.type`); more than 5 filters on one entry.
- Two subscriptions of one consumer with the same key.
- `bus: project` while `event_bus.enabled` is not true; a `bus` other than `project` or `default`.
- `subscribes` or `queue` on a scheduled or event task.
- `visibility_timeout` or `max_receive_count` out of range; `ordered` or `fifo_retired` that is
  not a boolean.
- A consumer queue name equal to an `extensions.sqs_queues` queue or DLQ name (both computed
  with `awsName`, `.fifo` names included).
- An `extensions.sns_topics` entry whose name equals the alerts topic, when there are consumers.
- Two event-task rules (the legacy `rule_name` form included) with the same AWS rule name,
  `<project>_rule_<key>_<env>`, on the same bus. The same key on different buses is allowed.
- Reserved container variables in a workload's YAML env vars (`app/workload_env_reserved.go`):
  `EVENT_QUEUE_URL`, `EVENT_DLQ_URL` on a consumer; `EVENT_FIFO_QUEUE_URL`,
  `EVENT_FIFO_DLQ_URL` on a consumer with a FIFO set; `EVENT_BUS_ARN` on every workload of an
  enhanced environment.

The bus kind and sharing:

- `event_bus.type` other than `classic` or `enhanced`; `enhanced` without `enabled: true`;
  `enhanced` in a region outside the list.
- `share` on a classic bus; `share` that is not a list; an entry with zero or several of
  `organization`, `ou`, `account`, `env`; `organization` not `true`; an `ou` id not shaped like
  `ou-ab12-cd34ef56`; an `account` that is not 12 digits in quotes; `access` absent or not
  `publish`, `subscribe` or `full`.
- An entry naming this environment's own account; an `env` that no same-project sibling with an
  `account_id` declares; an `env` two sibling files declare; an `env` entry when the sibling
  files could not be read; the same principal twice.
- An environment whose render depends on the Terraform state and whose state cannot be read
  (below). meroku cannot tell which subscribers, ordered queues and shares are deployed, so it
  refuses to render. A deploy runs its AWS pre-flight, which refreshes an expired SSO session,
  and generates once more before it gives up.
- An enhanced bus that is or was shared whose subscribers cannot be listed
  (`eventsv2 ListSubscribers`), so meroku cannot tell whether another account still holds a
  subscriber that must be revoked. Retried after the pre-flight in the same way.

### What was deployed

The state in the S3 backend is shared; `env/<env>/` (main.tf, `.terraform`,
`.terraform.lock.hcl`) is build output, per machine, and says nothing about what another machine
deployed. A render that writes main.tf reads the state itself when it depends on it
(`eventsStateRequired`, `app/events_state.go`):

- the YAML sets `type: enhanced`, or a workload sets `fifo_retired`;
- the previous main.tf has a `# meroku:events` marker (it was enhanced, in cleanup, shared, or
  kept a FIFO set);
- the lock file names awscc;
- there is no previous `env/<env>/main.tf` on this machine (or it cannot be read) and the YAML
  sets `state_bucket` and `state_file`: a fresh checkout, where another machine may have deployed
  the enhanced bus. This costs one S3 read the first time an environment is generated on a
  machine.

How it reads:

- With `env/<env>/.terraform`: `terraform state pull`, with the env YAML's `aws_profile` and
  `region`. Terraform picks the workspace (`TF_WORKSPACE`, else `.terraform/environment`).
- Without it: the state object directly, with the same profile:
  `s3://<state_bucket>/<state_file>`, or `s3://<state_bucket>/env:/<workspace>/<state_file>` when
  `TF_WORKSPACE` names a workspace other than `default` (the S3 backend's default
  `workspace_key_prefix`; env/main.hbs sets none). A missing bucket or object is a verified empty
  state.
- A read that fails refuses the render, whatever made it needed. The markers record the last
  render, not the last apply, so they never stand in for the state there. Only the offline
  `/api/config-warnings` preview reads markers.

Once this machine has rendered the environment, a main.tf with no events marker is its record
that the last render had nothing of the enhanced bus; a classic environment then generates
offline.

Subscribers in state count as meroku's only with two pieces of evidence: the module address is
one meroku renders (`event_subscriber_<consumer>_<key>` or `event_fifo_subscriber_<consumer>`,
with a consumer matching `^[a-z0-9][a-z0-9-]*$` and a key matching `^[a-z0-9][a-z0-9-]{0,39}$`),
and the subscriber's `event_bus_arn` is the project bus: the one in state, or, when the state
holds none, an EventBridge V2 bus named `<project>-<env>`. A custom `awscc_eventsv2_subscriber`
on another bus changes nothing, whatever its module is called.

Ordering:

- `ordered: true` on a classic bus, or on a `bus: default` entry.
- `fifo_retired: true` beside an `ordered: true` entry.
- A deployed FIFO subscriber whose consumer is still in the YAML, has no ordered subscription
  now, and does not set `fifo_retired` (see
  [Removing the last ordered subscription](#removing-the-last-ordered-subscription)).

Patterns and names on the enhanced bus:

- A `detail_type` with a `*` that is not one leading or one trailing `*` beside other text.
- A standard subscriber whose filters, measured as AWS measures them (above), are over 4,096
  bytes; a consumer whose ordered subscriptions together make filters over 4,096 bytes or a
  pattern over 1,000 `$or` combinations.
- An event-task rule on the project bus; a legacy single-rule event task.
- A consumer name outside `^[a-z0-9][a-z0-9-]*$` (also in cleanup).
- Two subscribers, or two delivery roles, that resolve to the same AWS name.
- A new subscriber name that a deployed subscriber of another module still uses.
- In custom Terraform, while the awscc provider renders: a root module named
  `event_bus_enhanced`, or one starting with `event_subscriber_` or `event_fifo_subscriber_`.
  A classic environment may still use these names.

## Custom Terraform

`local.bridge` (env/<env>/_bridge.tf):

| Key | Value | Off |
|---|---|---|
| `event_bus_name` | the project bus name | `""` when the bus is off |
| `event_bus_arn` | the project bus ARN, classic or enhanced | `""` |
| `event_queue_urls` | map, consumer name → queue URL | `{}` when there are no consumers |
| `event_fifo_queue_urls` | map, consumer name → FIFO queue URL | the key is absent unless an environment has a FIFO set or is enhanced. Leaving enhanced drops it in the generate that deletes the bus, so the generate after that apply plans empty |
| `alerts_topic_arn` | the alerts topic ARN | `""` when there are no consumers |

Read the FIFO map as `try(local.bridge.event_fifo_queue_urls, {})`, so `_shared` custom
Terraform also works in an environment without it.

`env/<env>/main.tf` of an enhanced environment, or one with a FIFO set, carries
`# meroku:events ...` comment lines above `module "events"`. Their presence makes the next
generate read the state (a read that fails refuses it), and the offline configuration-warnings
preview reads them in place of the state. Do not edit them; main.tf is generated.

## Web UI

`GET /api/events/deployed?env=<env>` (`app/api_events_deployed.go`) tells the event editors what
the last deploy left, from the same functions generate decides with: 200
`{"known": bool, "fifoConsumers": [...], "sharedPrincipals": [...], "subscribePrincipals": [...]}`.
`fifoConsumers` are the workloads with a deployed FIFO set, active or retired; `sharedPrincipals`
the principals the bus is shared with now, any access (`account:<id>`, `ou:<id>`,
`organization`); `subscribePrincipals` the part of them whose deployed access is `subscribe` or
`full`. `known` is true only when the state (whenever generate would read it) or the previous
main.tf's markers answered; otherwise every list is empty and says nothing. It may read S3 or run `terraform state pull`, and
writes nothing. An unknown env is 404 `{"error": ...}`.

- EventBridge node: **Settings** (the bus switch) and **Test Event**, which sends to the
  default bus or the project bus with the environment's own profile and region. For a classic
  project bus it first checks that the bus exists, because PutEvents to a missing bus can
  report success and drop the event. For the enhanced bus see
  [Test Event on the enhanced bus](#test-event-on-the-enhanced-bus).
- Settings, when the bus is on: **Kind of bus** (Standard = `classic`, Enhanced =
  `enhanced`; Enhanced is disabled outside the region list) and, on Enhanced, **Share with
  other accounts**. Leaving Enhanced removes `type`, `share` and every `ordered`, and sets
  `fifo_retired: true` on consumers that had ordered entries. Turning the bus off is blocked
  while an entry names `bus: project`; one button removes those `bus` fields.
- Backend and every service: an **Events** tab (subscriptions, produces, queue settings). On
  an enhanced project-bus row, **Keep order** sets `ordered`. Turning off the last ordered row
  first reads `GET /api/events/deployed` again (the tab shows "Checking whether this …'s ordered
  queue was deployed"), then saves once: with `fifo_retired: true` unless that read is known
  and lists no FIFO set for the workload. A failed or cancelled read keeps the queue. The save
  is applied to the config as it is when the read answers, for that workload only, so an edit
  elsewhere made meanwhile (a service added) is kept. While the read is pending every events
  control of the tab is disabled (the note adds "The events settings are locked until the check
  is done"). If the workload's events changed meanwhile anyway (from another panel), the edit is
  not saved and the tab says "Ordered delivery is still on …". The save targets the environment
  file open when the edit was made (`selectedEnvironment`, not the YAML `env` field); if another
  file is open when the read answers, the edit is dropped with a toast. App computes each edit
  from the config it holds and sends it once, outside any React state updater (StrictMode runs
  updaters twice); `tests/e2e/events-ordering-save.spec.ts` counts the save requests. A read for
  the decision answers its caller even when a routine refresh started after it; an environment
  change cancels it. The queue settings card then offers **Delete the ordered queue**, which
  removes it.
- Scheduled and event tasks: a **Produces** editor on the Events tab; event-task rules have a
  bus picker.
