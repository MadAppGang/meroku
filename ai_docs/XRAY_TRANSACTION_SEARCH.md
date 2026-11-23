# X-Ray Transaction Search

Every environment that uses X-Ray gets X-Ray Transaction Search turned on in its AWS account
and region, and its X-Ray task roles may send OpenTelemetry spans to the X-Ray OTLP endpoint.

## What "uses X-Ray" means

The backend has `workload.xray_enabled: true`, or any rendered service (not `enabled: false`) has
`services[].xray_enabled: true`. Scheduled and event-processor tasks have no X-Ray setting.
The same condition is `local.xray_in_use` in `modules/workloads/xray_transaction_search.tf` and
`envUsesXRay` in `app/xray_transaction_search.go`.

## Why

A service that exports traces with OpenTelemetry posts them over OTLP to
`https://xray.<region>.amazonaws.com/v1/traces`, signed with SigV4. That endpoint accepts spans
only while Transaction Search is on in the account and region, and the caller needs
`xray:PutSpans`. Before this, `xray_enabled` gave the X-Ray daemon sidecar (services) or the ADOT
collector (backend) and the segment-write IAM, but nothing carried `PutSpans` and nothing turned
Transaction Search on, so OTLP spans were refused.

## What is created

In `modules/workloads/xray_transaction_search.tf`, each with `count = local.xray_in_use ? 1 : 0`:

| Resource | What it does |
|---|---|
| `aws_cloudwatch_log_resource_policy` `<project>-<env>-xray-transaction-search` | Lets `xray.amazonaws.com` call `logs:PutLogEvents` on `aws/spans` and `/aws/application-signals/data`, conditioned on `aws:SourceArn` (this account's X-Ray) and `aws:SourceAccount` |
| `aws_xray_trace_segment_destination` | Destination `CloudWatchLogs`. Depends on the policy: X-Ray checks it may write when the destination changes |
| `aws_xray_indexing_rule` `Default` | Probabilistic, `desired_sampling_percentage = xray.indexing_percentage` |

The policy is named per environment so that destroying one environment never removes a policy a
sibling in the same account relies on. Its name limit is 5120 characters, so it does not go
through `modules/naming`.

## YAML

```yaml
xray:
  indexing_percentage: 1   # 0-100, default 1
```

Optional, top level, no migration. Absent renders `xray_indexing_percentage = 1`
(`{{default xray.indexing_percentage 1}}` in `env/main.hbs`). `meroku generate` refuses
(`validateXRayMap`, `app/validation.go`):

- `xray` that is not a map;
- any key other than `indexing_percentage`;
- a value that is not an unquoted number, or is below 0 or above 100.

The block sits at the top level, not under `workload`, because the setting belongs to the account
and region, not to one workload. `xray_enabled` stays on the backend and on each service.

## IAM

`modules/workloads/cloudwatch_access.tf` adds a statement `XRaySpans`
(`xray:PutSpans`, `xray:PutSpansForIndexing`, `xray:PutTraceSegments`, Resource `*`) to the
backend's task-role CloudWatch policy when `xray_enabled` is on, and to each service's when that
service's `xray_enabled` is on. Unchanged: `AWSXrayWriteOnlyAccess` on the backend task role, the
`XRayWrite` statement on services, the daemon and collector sidecars.

## Cost

Once the segment destination is CloudWatch Logs, every span, including the segments the X-Ray
daemon sends, is stored in the `aws/spans` log group and billed as CloudWatch Logs ingestion and
storage, not as X-Ray traces. Indexing up to 1% of spans as trace summaries is free; above that,
indexing is billed per span. Keep the default 1 in production; a development environment may
set 100 to find every trace.

## Account-wide behaviour

The segment destination and the `Default` indexing rule are singletons: one per account and
region, existing in every account before Terraform touches them.

- **The next apply flips it.** Every environment that already has X-Ray on switches its account
  and region to CloudWatch Logs on its next apply after upgrading meroku. Other applications in
  the same account and region that send X-Ray segments are moved too.
- **Two environments in one account.** Both hold the two X-Ray resources. Creating one updates
  the existing singleton, so neither apply fails. If their `indexing_percentage` values differ,
  each apply overwrites the other's. `meroku generate` and the web UI warn about it
  (`xray_indexing_conflict`, env level) when an env YAML next to this one has the same account
  (`account_id`, or `aws_profile`) and region, uses X-Ray, and has a different effective
  percentage. Fix it by setting the same value in all of them.
- **Destroy does not revert.** Deleting either X-Ray resource does nothing in AWS. Turning X-Ray
  off, removing it from the YAML or destroying the environment leaves Transaction Search on and
  the indexing rule at its last value. Only the per-environment log resource policy is deleted.
  To turn Transaction Search off, run
  `aws xray update-trace-segment-destination --destination XRay` in that account and region
  once no environment there uses X-Ray.

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

- No web UI or TUI field for `xray.indexing_percentage`; it is set in the YAML.
- A project that already manages these singletons in its own custom Terraform should remove that
  copy. Two addresses then manage the same settings, which is harmless while the values match.
