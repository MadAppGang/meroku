# Events: project bus, per-workload queues, strict PutEvents

A workload publishes events with `events:PutEvents` and receives them on its own SQS
queue. Both are declared in the YAML. Module: `modules/events`. Go: `app/events.go`.

## YAML

```yaml
event_bus: { enabled: true }            # creates the bus <project>-<env>

workload:                               # the backend, keyed "backend"
  events:
    subscribes:
      - { detail_type: order.created }

services:
  - name: orders
    events:
      subscribes:
        - { detail_type: payment.completed, source: example.payments }
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
| `bus` (any entry or rule) | `project` or `default`. Absent: the project bus when it is on, otherwise `default`. `project` with the bus off is a generate error. |
| `subscribes[]` | Backend and services only. Exactly one of `detail_type` (exact) or `detail_type_prefix`, an optional `source`, an optional `name`. |
| `queue.visibility_timeout` | 0–43200 seconds, default 30. Set it above the longest time a message takes to handle. |
| `queue.max_receive_count` | 1–1000, default 5. Receives before SQS moves a message to the application DLQ. |
| `produces[]` | Exactly one of a `detail_types` list or `any: true`. `own_source_only` (default `true`) limits the entry to events whose `source` is the workload's own `EVENT_SOURCE`. |
| `event_processor_tasks[].rules[].bus` | The bus the rule is on. The legacy single-rule form (`rule_name`) follows the environment's bus. |

AWS services (`aws.ecs`, `aws.ecr`, ...) and meroku's CI events publish to the **default**
bus only. A subscription or rule for them needs `bus: default` when the project bus is on.

## What a consumer gets

A workload with at least one `subscribes` entry is a consumer. Its running copies share one
queue, so each event is handled by one copy.

| Resource | Name (form 2 of `modules/naming`) | Settings |
|---|---|---|
| Main queue | `<project>-events-<name>-<env>` | SSE-SQS, 4-day retention, 20 s long polling, redrive to the application DLQ |
| Application DLQ | `<project>-events-<name>-dlq-<env>` | SSE-SQS, 14 days. Messages the app failed `max_receive_count` times |
| Delivery DLQ | `<project>-events-<name>-delivery-dlq-<env>` | SSE-SQS, 14 days. Events EventBridge could not deliver |
| One rule per subscription | `<project>-events-<name>-<key>-<env>` | On the subscription's bus; target retry 185 attempts within 24 h, then the delivery DLQ |
| Alarm per DLQ | `<project>-<env>-events-<name>-dlq` and `...-delivery-dlq` | `ApproximateNumberOfMessagesVisible` Maximum > 0 over 300 s, missing data not breaching |
| `EVENT_QUEUE_URL` | container variable | the main queue's URL |

The rule pattern is the detail-type (exact, or `{"prefix": ...}`), the `source` when set, and
`"replay-name": [{"exists": false}]`: an archive replay is never delivered to a queue.

The main queue and the delivery DLQ each have a queue policy that lets `events.amazonaws.com`
send only for this consumer's own rules (`aws:SourceArn`). The application DLQ needs none.

The task role gets `sqs:ReceiveMessage`, `DeleteMessage`, `ChangeMessageVisibility`,
`GetQueueAttributes` and `GetQueueUrl` on the main queue, in the inline policy
`<project>_events_<name>_<env>`.

### Subscription keys

The key is the rule's `for_each` key and the last part of its name, so reordering entries
changes nothing and editing one entry touches only its own rule.

- With `name` (`^[a-z0-9][a-z0-9-]{0,31}$`): the name.
- Without: the `detail_type`, or the `detail_type_prefix` followed by `-prefix`, lower-cased,
  each run of characters outside `[a-z0-9]` replaced by `-`, trimmed and cut to 32. `-default`
  is appended when the entry is on the default bus. `payment.completed` → `payment-completed`;
  `user.` on the default bus → `user-prefix-default`.

Two entries of one consumer with the same key are a generate error. Give one a `name`
(typical case: one detail-type from two sources).

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
| backend, `setup_fcnsns` | `sns:CreatePlatformEndpoint` | the FCM platform application (`app/GCM/<project>-fcm-<env>`) |
| backend, `setup_fcnsns` | `sns:Publish`, `GetEndpointAttributes`, `SetEndpointAttributes`, `DeleteEndpoint` | that application's endpoints (`endpoint/GCM/<project>-fcm-<env>/*`) |
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
`event-bus/<project>-*` is gone.

## Writing a consumer

- **Be idempotent.** SQS delivers at least once, and EventBridge retries. Use the event `id`
  (or a business key) to skip duplicates.
- **Delete after handling.** A message not deleted reappears after `visibility_timeout` and
  counts one more receive. After `max_receive_count` it moves to the application DLQ.
- **Long polling** is already on (`receive_wait_time_seconds = 20`); `WaitTimeSeconds` on the
  call can stay unset.
- The SQS message body is the whole EventBridge event (`detail-type`, `source`, `detail`, ...).

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

## Generate-time errors

- Not exactly one of `detail_type` / `detail_type_prefix`; not exactly one of `detail_types` /
  `any: true`.
- Two subscriptions of one consumer with the same key.
- `bus: project` while `event_bus.enabled` is not true; a `bus` other than `project` or `default`.
- `subscribes` or `queue` on a scheduled or event task.
- `visibility_timeout` or `max_receive_count` out of range.
- A consumer queue name equal to an `extensions.sqs_queues` queue or DLQ name (both computed
  with `awsName`).
- An `extensions.sns_topics` entry whose name equals the alerts topic, when there are consumers.
- Two event-task rules (the legacy `rule_name` form included) with the same AWS rule name,
  `<project>_rule_<key>_<env>`, on the same bus. The same key on different buses is allowed.
- `EVENT_QUEUE_URL` in a consumer's YAML env vars (`app/workload_env_reserved.go`).

## Custom Terraform

`local.bridge` (env/<env>/_bridge.tf):

| Key | Value | Off |
|---|---|---|
| `event_bus_name` | the project bus name | `""` when the bus is off |
| `event_bus_arn` | the project bus ARN | `""` |
| `event_queue_urls` | map, consumer name → queue URL | `{}` when there are no consumers |
| `alerts_topic_arn` | the alerts topic ARN | `""` when there are no consumers |

Ordered (FIFO) events are not offered; write those in custom Terraform.

## Web UI

- EventBridge node: **Settings** (the bus switch) and **Test Event**, which sends to the
  default bus or the project bus with the environment's own profile and region. For the
  project bus it first checks that the bus exists, because PutEvents to a missing bus can
  report success and drop the event.
- Backend and every service: an **Events** tab (subscriptions, produces, queue settings).
- Scheduled and event tasks: a **Produces** editor on the Events tab; event-task rules have a
  bus picker.
