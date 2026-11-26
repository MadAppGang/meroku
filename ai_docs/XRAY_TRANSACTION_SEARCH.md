# X-Ray Transaction Search

A workload can send traces to X-Ray in two ways, and both turn on X-Ray Transaction Search in the
environment's AWS account and region:

| Key (backend: `workload.`, service: `services[].`) | Sidecar | Task-role grant |
|---|---|---|
| `xray_enabled: true` | ADOT collector (backend) or X-Ray daemon (service) | `XRaySpans`, plus `XRayWrite` on a service |
| `xray_spans: true` | None. The app signs its OTLP requests and posts them itself | `XRaySpans` |

**`xray_spans` turns on an account-wide setting.** Once Transaction Search is on, every span in
the account and region is stored and billed as CloudWatch Logs (see [Cost](#cost)), not only this
environment's.

## Why

A service that exports traces with OpenTelemetry posts them over OTLP to
`https://xray.<region>.amazonaws.com/v1/traces`, signed with SigV4. That endpoint accepts spans
only while Transaction Search is on in the account and region, and the caller needs
`xray:PutSpans`. A grant alone never works: with Transaction Search off the endpoint refuses every
span, whatever the role allows. That is why a key that grants spans also turns Transaction Search
on.

## What turns it on

| Rule | Transaction Search |
|---|---|
| A rendered workload has `xray_enabled: true` | On, always. `xray.transaction_search: false` is refused |
| A rendered workload has `xray_spans: true` | On, unless `xray.transaction_search: false` |
| `xray.transaction_search: true` | On, even with no X-Ray workload |
| None of these | Off; nothing is created |

A rendered workload is the backend, or a service without `enabled: false` (a disabled service is
left out of main.tf, so it counts for nothing). Scheduled and event-processor tasks have no X-Ray
setting.

The rule exists twice:

- `local.transaction_search_on` in `modules/workloads/xray_transaction_search.tf` counts the
  resources. It is `local.xray_in_use` (sidecar) OR, when `var.xray_transaction_search` is
  `null`, `local.xray_spans_in_use`, otherwise `var.xray_transaction_search`.
- `envUsesTransactionSearch` in `app/xray_transaction_search.go` drives the warnings.

Both are checked against `app/testdata/xray_transaction_search_vectors.json`. The Go side evaluates
every vector. The Terraform side has one hand-written `run "<name>"` per vector in
`modules/workloads/tests/xray_transaction_search.tftest.hcl` (vectors with a disabled service are
`go_only`). The Go check on the tftest file matches run names only, so a vector's inputs and
expected result must be copied into its run by hand.

| sidecar (`xray_enabled`) | `xray_spans` | `xray.transaction_search` | Transaction Search |
|---|---|---|---|
| no | no | absent | off |
| no | yes | absent | **on** |
| any | any | `true` | on |
| no | yes | `false` | off (the grants stay) |
| no | no | `false` | off |
| yes | any | absent | on |
| yes | any | `false` | refused by `meroku generate`; the module still turns it on |

## What is created

In `modules/workloads/xray_transaction_search.tf`, each with
`count = local.transaction_search_on ? 1 : 0`:

| Resource | What it does |
|---|---|
| `aws_cloudwatch_log_resource_policy` `<project>-<env>-xray-transaction-search` | Lets `xray.amazonaws.com` call `logs:PutLogEvents` on `aws/spans` and `/aws/application-signals/data`, conditioned on `aws:SourceArn` (this account's X-Ray) and `aws:SourceAccount` |
| `aws_xray_trace_segment_destination` | Destination `CloudWatchLogs`. Depends on the policy: X-Ray checks it may write when the destination changes |
| `aws_xray_indexing_rule` `Default` | Probabilistic, `desired_sampling_percentage = xray.indexing_percentage` |

The policy is named per environment so that destroying one environment never removes a policy a
sibling in the same account relies on. Its name limit is 5120 characters, so it does not go
through `modules/naming`. Moving an env from `xray_enabled` to `xray_spans` keeps the same three
addresses, so nothing is replaced.

## YAML

```yaml
xray:
  transaction_search: false   # optional. absent: derive (table above); true: on; false: owned elsewhere
  indexing_percentage: 1      # optional, 0-100, default 1

workload:
  xray_spans: true            # optional, default false: XRaySpans on the backend task role, no sidecar

services:
  - name: api
    xray_spans: true          # the same, for this service
```

All optional, no migration. The template renders `xray_indexing_percentage` with
`{{default xray.indexing_percentage 1}}`, and `xray_transaction_search` and `backend_xray_spans`
only when the key exists (`{{#if (exists ...)}}`), so an env without the new keys renders the same
main.tf as before. `services[].xray_spans` travels inside the `services` list.

`xray.transaction_search`:

| Value | Meaning |
|---|---|
| absent | Derive it from the workloads (table above) |
| `true` | Turn it on here, even with no X-Ray workload: for example a scheduled task granted spans through its `policy` list |
| `false` | It is owned elsewhere: a sibling env in the same account and region, the console or another stack. Only for an env whose X-Ray use is `xray_spans`; the grants stay |

**`false` removes this env's log resource policy.** The account's segment destination stays
`CloudWatchLogs`, so X-Ray still routes spans to `aws/spans`, and only a log resource policy lets
it write there. If this env's policy was the only one (for example an env that had `xray_enabled`
switches to `xray_spans` with `false`), spans are dropped. Before writing `false`, make sure the
owner (a sibling env with Transaction Search on, the console setup, or another stack) keeps a log
resource policy of its own.

`meroku generate` refuses (`validateXRayMap`, `app/validation.go`):

- `workload.xray_spans` or any `services[].xray_spans` (a disabled service's included) that is not
  an unquoted boolean. Checked whether or not the env has an `xray` block;
- `xray` that is not a map;
- any `xray` key other than `transaction_search` and `indexing_percentage`;
- `xray.transaction_search` that is not an unquoted boolean;
- `xray.transaction_search: false` while a rendered workload has `xray_enabled: true`. The error
  names that key and suggests `xray_spans` in its place if the sidecar is not needed;
- an `indexing_percentage` that is not an unquoted number, or is below 0 or above 100.

The block sits at the top level, not under `workload`, because the setting belongs to the account
and region, not to one workload. `xray_enabled` and `xray_spans` stay on the backend and on each
service.

## IAM

`modules/workloads/cloudwatch_access.tf` holds one statement, `local.xray_spans_statement`
(`XRaySpans`: `xray:PutSpans`, `xray:PutSpansForIndexing`, `xray:PutTraceSegments`, Resource `*`).
It is added to the backend's task-role CloudWatch policy when `xray_enabled` or `xray_spans` is on
(`local.backend_gets_xray_spans`), and to each service's when that service has either
(`local.service_gets_xray_spans`). Both set gives one statement, not two. Any later role that sends
spans references the same local and feeds `local.xray_spans_in_use`.

Unchanged: `AWSXrayWriteOnlyAccess` on the backend task role (always attached), the `XRayWrite`
statement on a service with `xray_enabled`, and the daemon and collector sidecars, which come only
with `xray_enabled`.

## Sending spans without a sidecar

With `xray_spans: true` the app exports OTLP traces itself:

- Endpoint: `https://xray.<region>.amazonaws.com/v1/traces` (HTTP, protobuf).
- Signing: SigV4, service name `xray`, with the task role's credentials (the ECS container
  credentials the AWS SDK picks up on its own).
- No sidecar, no container change and no environment variable is added; the app is configured
  as it would be for any OTLP exporter.

Because there is no sidecar, `xray_spans` works on a bridge-mode EC2 compute pool, where
`xray_enabled` is refused. The task posts to the public regional endpoint.

Scheduled and event-processor tasks have no `xray_spans`. Grant such a task the three `XRaySpans`
actions through its `policy` list and turn Transaction Search on with
`xray.transaction_search: true`.

## Cost

Once the segment destination is CloudWatch Logs, every span in the account and region, including
the segments the X-Ray daemon sends and spans from other applications, is stored in the
`aws/spans` log group and billed as CloudWatch Logs ingestion and storage, not as X-Ray traces.
Indexing up to 1% of spans as trace summaries is free; above that, indexing is billed per span.
Keep the default 1 in production; a development environment may set 100 to find every trace.
This applies to `xray_spans` exactly as to `xray_enabled`.

## Account-wide behaviour

The segment destination and the `Default` indexing rule are singletons: one per account and
region, existing in every account before Terraform touches them.

- **The next apply flips it.** An environment that turns Transaction Search on switches its
  account and region to CloudWatch Logs on its next apply. Other applications in the same account
  and region that send X-Ray segments are moved too.
- **Two environments in one account.** Both hold the two X-Ray resources. Creating one updates
  the existing singleton, so neither apply fails. If their `indexing_percentage` values differ,
  each apply overwrites the other's. `meroku generate` and the web UI warn about it
  (`xray_indexing_conflict`, env level) when an env YAML next to this one has the same account
  (`account_id`, or `aws_profile`) and region, turns on Transaction Search
  (`envUsesTransactionSearch`), and has a different effective percentage. Fix it by setting the
  same value in all of them, or let one env own it and give the others
  `xray.transaction_search: false` (see the policy caveat above).
- **Destroy does not revert.** Deleting either X-Ray resource does nothing in AWS. Removing the
  X-Ray keys, setting `transaction_search: false` or destroying the environment leaves
  Transaction Search on and the indexing rule at its last value. Only the per-environment log
  resource policy is deleted. To turn Transaction Search off, run
  `aws xray update-trace-segment-destination --destination XRay` in that account and region once
  no environment there turns it on.

## Warnings

| Code | When |
|---|---|
| `xray_indexing_conflict` | This env and a sibling in the same account and region both turn on Transaction Search with different effective percentages |
| `xray_indexing_unused` | `xray.indexing_percentage` is set but this env does not turn on Transaction Search, so no rule is created from it. With `transaction_search: false`, set the percentage in the env that owns it |

Both are env-level configuration warnings: `meroku generate` prints them and carries on
([Config Warnings](./CONFIG_WARNINGS.md)).

## Provider floor

`aws_xray_trace_segment_destination` and `aws_xray_indexing_rule` are new in hashicorp/aws
6.47.0. The constraint is `~> 6.47` in `env/main.hbs` and `modules/workloads/versions.tf`.

An environment whose `.terraform.lock.hcl` selects an older 6.x would fail with "locked provider
... does not match configured version constraint". `lockedProviderMajorBehind`
(`app/terraform_lock_upgrade.go`) compares major.minor when the constraint names a minor, so
meroku runs `terraform init -upgrade` itself before the plan and prints:

```
🔄 The AWS provider lock is on version 6.30 and this meroku generates for version 6.47 or newer.
   Running terraform init -upgrade to move the lock to the new version.
```

`init -upgrade` selects the newest 6.x, so every environment on an older lock moves to the newest
6.x release on its next deploy, whether or not it uses X-Ray.

## Not covered

- No web UI or TUI field for `xray.indexing_percentage`, `xray.transaction_search` or
  `xray_spans`; they are set in the YAML. The web editor keeps them through a save.
- No check that another log resource policy exists when an env sets `transaction_search: false`.
- No `xray_spans` on scheduled or event-processor tasks (use `policy` and
  `transaction_search: true`).
- A project that already manages these singletons in its own custom Terraform should remove that
  copy, or set `transaction_search: false` with the caveat above. Two addresses then manage the
  same settings, which is harmless while the values match.
