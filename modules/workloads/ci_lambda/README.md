# CI/CD Lambda

The deployment trigger for a meroku project. An EventBridge event becomes either
an ECS `UpdateService` (long-running services) or a new task-definition revision
(scheduled tasks), plus a Slack post.

It is fire-and-forget: there is no deployment waiter, which is why a 60-second
function timeout is enough.

## The one rule

**The Lambda never derives an identifier. It only ever looks one up.**

Terraform emits the maps; a key that is not in a map is not ours and the event is
`ignored`. That is what makes cross-project mis-deployment impossible in a shared
account, what lets one ECR repository fan out to several services
(`ecr_config mode = use_existing`), and what stops a repository-name parser from
inventing a service that does not exist.

| Map | Shape | Answers |
|---|---|---|
| `ECS_SERVICE_MAP` | `{"backend": {"service_name", "task_family"}, "{svc}": {…}}` | what to deploy |
| `SCHEDULED_TASK_MAP` | `{"task:{name}": {"task_family", "type"}}` | what to deploy |
| `ECR_REPO_MAP` | `{"acme_service_api": ["api", "reporting"]}` | which push deploys what |
| `SSM_SERVICE_MAP` | `{"/dev/acme/task/cleanup": "task:cleanup"}` | which parameter deploys what |
| `S3_SERVICE_MAP` | `{"backend": [{"bucket", "key"}], "{svc}": […]}` | which env file deploys what |
| `AUTO_DEPLOY_MAP` | `{"backend": true, "api": false, "task:cleanup": true}` | may an event deploy it at all |

Identifiers are `backend`, the service name, or `task:{name}`. `backend` is a real
string everywhere: Terraform key, Go constant, log field, Slack message, manual
deploy payload.

Each map answers exactly one question, which is why `auto_deploy` is a map of its
own rather than a field inside `ECS_SERVICE_MAP`: a policy edit should not rewrite
a map that describes resource identity, and the whole policy is then readable in
one value from `aws lambda get-function-configuration`. Its key set must equal the
union of the two target maps' key sets — asserted in the boundary test and again
at runtime by `config.SelfCheck`.

## Layout

```
main.go                       cold start, wiring, lambda.Start
contract/                     backend_id + task_id_prefix, read by HCL and embedded by Go
tf_identifiers/               provider-free Terraform module: identifier fan-out
tools/mkzip/                  packages the bootstrap binary (replaces the `zip` CLI)
internal/config/              env vars -> Config, plus the resolvers and SelfCheck
internal/awsecs/              the only code that talks to ECS (SDK v2)
internal/slack/               notifications; Notify returns nothing, by design
internal/deploy/              retry policy, notification policy
internal/handler/             one file per event source
internal/boundary/            the Terraform <-> Go boundary check + lambda.tf guards
internal/testsupport/         the shared fixture every test loads
```

## Events

| Source | Detail type | Effect | Honours `auto_deploy` |
|---|---|---|---|
| `aws.ecr` | `ECR Image Action` | deploy every identifier bound to the repository | yes |
| `aws.ssm` | `Parameter Store Change` (`operation = Create` or `Update`) | deploy the identifier owning the longest matching path prefix | yes |
| `aws.s3` | `AWS API Call via CloudTrail` | deploy every identifier bound to that bucket + key | yes |
| `action.{env}`, `github.actions.{env}` | `DEPLOY`, `SERVICE_DEPLOY` | deploy the named identifier | **no** — asked for explicitly |
| `action.deploy` | `DEPLOY`, `SERVICE_DEPLOY` | deploy the named identifier, only with `detail.project` + `detail.env` | **no** — asked for explicitly |
| `terraform.{env}` | `SERVICE_DEPLOY` | deploy the service whose task definition Terraform just registered | yes — the resource is not created when it is off |
| `aws.ecs` | `ECS Deployment State Change`, `ECS Service Action` | Slack only; never deploys, never errors | n/a |

### The two SSM operations, and the one that is excluded

`Create` and `Update` are the same operator action: `PutParameter` with and
without `Overwrite`, with Parameter Store choosing between them purely on whether
the name already existed. meroku writes parameters that way itself
(`app/api_ssm.go`), so **adding** a variable emits `Create` and only ever
`Create`. Both therefore deploy.

Two exclusions ride on top:

- **`Delete` is not in the rule**, and the absence is load-bearing. A deployment
  cannot restore a deleted parameter; it makes the loss fatal. The revision the
  service is running still lists the parameter in `secrets`, so every task
  launched after the delete fails on `ResourceInitializationError` — a redeploy
  replaces the tasks that were still working with ones that cannot start.
  Terraform's next apply removes the entry from the list, and it is the only
  thing that can.
- **Terraform's own `{prefix}/env` placeholders are ignored on `Create`.**
  `env.tf`, `env_services.tf` and `modules/ecs_task/env.tf` each create one
  holding a single space, so the path exists for somebody to fill in; creating it
  says nothing about the configuration, and on a first apply the ECS service it
  names does not exist yet. The exclusion is `config.IsTerraformOwnedSSMPath`, an
  equality test against the prefixes in `SSM_SERVICE_MAP` — *not*
  `strings.HasSuffix(name, "/env")`, which would exclude an operator's own
  `/{env}/{project}/backend/nested/env`. An **Update** of `{prefix}/env` is the
  main configuration path here and always deploys.

### Why Terraform invokes the Lambda directly

`aws_ecs_service` carries `ignore_changes = [task_definition]`: Terraform owns
the service's shape, CI owns which revision runs in it. That split has a hole,
and `aws_lambda_invocation.backend_revision` / `.services_revision` are what
close it.

Adding an SSM parameter changes the `secrets` **list**, which lives in the task
definition. Only Terraform can render a revision that lists it — this Lambda's
one revision-producing call, `RegisterRevisionWithImage`, *clones* the latest
ACTIVE revision and substitutes an image, carrying the old list forward. So
Terraform registered the revision and deliberately did not deploy it, nothing
told the Lambda it existed, and every component reported success while the
container kept running the old configuration. The `Create` event cannot cover it
either: Parameter Store emits it at `PutParameter` time, *before* the apply that
registers the revision, so the deploy it triggers lands on the revision already
running.

One consequence, accepted deliberately: the **first** apply after this shipped
deploys every auto-deployable service in the environment at once, because each
was stranded on whatever revision it was last pushed. That is the repair — and
the only thing that rescues a service whose stranded revision has since been
deregistered — but it is a fleet-wide rolling restart triggered by an apply.

#### A green apply does not mean the deployment succeeded

The invoke is synchronous, so the apply waits for the Lambda to answer. It does
**not** wait for the deployment: this Lambda calls `ecs:UpdateService` and
returns, and there is no stabilisation waiter anywhere in it. ECS *accepting* the
request is where its knowledge ends.

The apply **fails** for two classes of problem: an error `deploy.Retryable()`
calls retryable that is still failing after `MAX_DEPLOYMENT_RETRIES`
(`ServerException`, 5xx server faults, the throttling family, network failures),
and an exhausted permission-propagation poll — see below.

The apply stays **green** for:

- every configuration-shaped failure. `ServiceNotFoundException`,
  `ClusterNotFoundException`, `InvalidParameterException` and an unknown
  identifier are all non-retryable (`internal/deploy/retry.go`), and the handler
  answers them with `{"status":"ignored"}` and a nil error. That is deliberate:
  EventBridge invokes this function asynchronously as well, and returning an
  error for an event that can never succeed means retrying it for hours.
- **everything downstream of an accepted `UpdateService`** — tasks that
  crashloop, an image that cannot be pulled, a secret in the new revision that
  cannot be resolved. ECS returns 200, the Lambda returns success, and the apply
  goes green while the service rolls back or sits stuck.

So read a green apply as "ECS accepted the deployment request". The outcome
arrives separately, through the `aws.ecs` state-change notifications the
`ci_ecs_state` rule exists to deliver, and in the service's own event log.

#### `AccessDeniedException` on the first apply, and why it is the exception

`AccessDeniedException` used to be on the green list, and leaving it there cost a
whole first apply. Measured on a real run: `aws_iam_role_policy_attachment.lambda_ecs`
completed at `06:01:17.0126` and the three
`aws_lambda_invocation.{backend,services}_revision` resources ran at
`06:01:23.38` — 6.4s later, before IAM had propagated. All three answered

```
AccessDeniedException: User: .../scratchpl_ci_lambda_dev is not authorized to
perform: ecs:UpdateService ... because no identity-based policy allows the
ecs:UpdateService action
```

against a policy that was **correct** — every later invocation succeeded. Read as
permanent, that produced `{"status":"ignored"}`, a nil error and a green apply in
which the repair deployment simply never happened.

The Lambda now **polls** for the permission rather than sleeping a guessed
interval: `deploy.awaitingPropagation` and the poll in `Deployer.attempt`. Three
properties matter.

- **No cost on the common path.** Every apply of an environment that already
  exists succeeds on the first ECS call and never polls at all.
- **Bounded by the invocation deadline**, not by a constant. The waits ramp
  1s → 2s → 4s and hold, and `fitsDeadline` refuses one that will not fit inside
  the function's 60s timeout with a call budget to spare. Reclassifying
  `AccessDenied` as an ordinary retry would not have worked: `MAX_DEPLOYMENT_RETRIES=2`
  over a 1s base is about three seconds, and the race above was still live at 6.4s.
- **Exhausting it fails loudly.** The error carries `deploy.ErrPropagationTimeout`,
  which `Retryable` reports true for, so `deployOne` returns a non-nil error, the
  Lambda reports a `FunctionError`, and the apply **fails** with the AccessDenied
  message attached. A genuine policy mistake is now a failed apply naming the
  real reason instead of a green no-op.

It is **scoped to Terraform's own invocations** and nothing else. The
`aws_lambda_invocation` resources send `source = "terraform.{env}"`, no
EventBridge rule accepts a `terraform.*` source, and `handler/manual.go` promotes
only that source to `deploy.SourceTerraform`. The ECR, SSM and S3 paths — and a
human clicking deploy in a workflow or the web UI — keep treating `AccessDenied`
as permanent, because those are asynchronous: a retryable verdict there means
EventBridge redelivers, and a `DEPLOYMENT_INITIATING` + `DEPLOYMENT_FAILED` pair
goes to Slack on every attempt of every redelivery, for a condition that will
never clear.

Scheduled tasks need no such edge: `modules/ecs_task/main.tf` targets
`arn_without_revision`, resolved at run time, and `modules/event_bridge_task`
pins `.arn` but is rewritten by the same apply that registered the revision.

### Which triggers actually exist, per environment

The table above lists what the Lambda does with an event it receives. Which
events can be *emitted* at all depends on the environment, and it is not uniform.
`SCHEDULED_TASK_MAP` is populated everywhere, which used to read as a promise that
every environment auto-deploys its scheduled tasks. It never did.

| Target | ECR push | SSM `Create`/`Update` | S3 env file | Manual `DEPLOY` |
|---|---|---|---|---|
| backend, `dev` | yes | yes | yes | yes |
| backend, other env | only with `ecr_strategy = local` | yes | yes | yes |
| service, `dev` | yes | yes | yes | yes |
| service, other env | only with `ecr_strategy = local` | yes | yes | yes |
| scheduled task, `dev` | yes | **never** | **never** | needs `image_uri` |
| scheduled task, other env | **never** | **never** | **never** | needs `image_uri` |

Why the blanks:

- **Scheduled task ECR, outside `dev`.** `modules/ecs_task/main.tf` creates
  `{project}_task_{name}` only when `env == "dev"`; elsewhere the task pulls from
  a cross-account URL, so no push into this account concerns it and
  `local.ci_task_repos` is empty. Nothing is dropped from `ECR_REPO_MAP` by
  policy here — the repository does not exist.
- **Scheduled task SSM, everywhere.** `internal/handler/ssm.go` skips scheduled
  tasks deliberately, because there is nothing useful this Lambda could do. A
  changed *value* is re-read when the task next starts; an *added* parameter
  changes the `secrets` list in the task definition, and
  `RegisterRevisionWithImage` clones the current revision, so the Lambda cannot
  add the entry either. Only Terraform can — and once it has, the scheduler
  reaches the new revision unaided, which is why the `aws_lambda_invocation`
  edge exists for services alone.
- **Scheduled task S3.** `S3_SERVICE_MAP` is built from the backend's env files
  and each service's; a scheduled task has no `env_files_s3` input.
- **Scheduled task manual deploy.** There is no ECS service to update, so a
  deploy means registering a revision with a specific image, and the request must
  carry `image_uri`. No meroku generator emits one, so in practice this is a
  hand-written event.

So on a scheduled task outside `dev`, `auto_deploy: true` enables nothing
automatic — only the hand-written manual path. `auto_deploy: false` there (the
migration's default for any non-dev environment) is the accurate statement of
what was already true.

### `auto_deploy`

Per-target, in YAML, on the backend (`workload.backend_auto_deploy`), on each
service and on each scheduled task. The meroku migration writes it explicitly:
**`true` in `dev`, `false` in every other environment.** An *absent* value means
`true` — that is what a project from before the setting does, and the upgrade
must not change it.

It is a **flag, not a filter**. A disabled target stays in every map above and
its repository stays in the ECR event rule, so the push still invokes the Lambda
and the log still gets written:

    "auto_deploy is disabled for api"          ← what actually happened
    "no target uses repository acme_service_api" ← what an omission would have said

The second sentence is untrue and indistinguishable from a typo'd repository
name. One extra invocation per push costs nothing; a silence nobody can explain
costs an afternoon. `ENABLE_ECR_MONITORING` and friends are the blunt version of
the same idea — whole event source off, no per-target detail.

What `auto_deploy: false` does **not** do:

- it does not undeploy anything, and does not remove the service, task, ECR
  repository, SSM parameter or event rule — that is `enabled: false`;
- it does not block a manual deploy. `auto_deploy` answers "may an event redeploy
  this on its own?", and a `DEPLOY` / `SERVICE_DEPLOY` event is somebody asking
  for exactly this deployment. Turning off automatic deploys in prod must not
  also take away the button that deploys prod.

### Scoping a manual deploy

There are two manual rules, because the sources fall into two kinds and only one
of them can be filtered.

**`ci_manual_deploy`** accepts sources whose *name* carries the environment:
`action.{env}`, `github.actions.{env}`, plus `action.production` when this
environment is a production one (the shipped deploy receipt hardcodes that
source). It carries **no `detail` filter** — EventBridge requires every key named
in a pattern to be present, and payloads already in the wild send only
`{"service": "..."}`, so a filter would kill them. It is safe unfiltered because
another environment's event cannot reach it: dev's rule does not list
production's sources.

**`ci_manual_deploy_global`** accepts `action.deploy`, which names no
environment. Nothing about such an event says which environment it means, so
this rule **requires `detail.project` and `detail.env`** and matches nothing
without them.

Cross-*project* separation is `internal/handler/manual.go`, which ignores an
event whose `project` or `env` does not match this Lambda's own. It can only act
on fields the payload carries, so **send `project` and `env`**: two projects in
one account both have a production environment, and no source list can tell them
apart.

    Detail="{\"service\":\"backend\",\"project\":\"<project>\",\"env\":\"<env>\"}"

Before this split, every environment's rule listed `action.production`
unconditionally. One production deploy invoked the dev Lambda, the staging
Lambda, and every other meroku project's Lambda in the account, and each
redeployed its own backend from whatever `:latest` was in its registry.

### The ECR rule's repository list

`aws_cloudwatch_event_rule.ci_ecr_push` normally lists every repository the
project deploys from. An EventBridge event pattern is capped at **2,048
characters**, so past roughly 60 repositories the exhaustive list stops fitting
and `lambda.tf` falls back to a prefix filter.

The fallback is `{"prefix": "{project}_"}` **plus every off-prefix repository in
full**. `ecr_config mode = manual_repo` points a service at an arbitrary registry
URI, which strips down to a name like `team/legacy-api` and carries no project
prefix; `use_existing` pointed at such a service inherits it. A bare prefix filter
therefore used to drop exactly those services the moment a project outgrew the
limit — around 92 repositories at 18-character names, 69 at 25, 51 at 35 — while
everything project-prefixed kept working. Nothing failed and nothing logged.

If the off-prefix repositories alone overflow the quota there is no third
fallback that keeps the rule complete, so a `precondition` fails the apply with
the count and the options rather than shipping a rule that ignores some of them.

`ECR_REPO_MAP` stays the authority in every case: whatever the looser filter lets
through resolves to no target and is ignored.

### Error policy

An error is returned **only** when a retry could plausibly succeed. Unknown
source, unmapped repository, unparsable detail, disabled feature flag, unknown
identifier, non-`PUSH` action → `{"status": "ignored"}` with a nil error.
EventBridge invokes asynchronously, so returning an error for an event that can
never be handled means retrying it for hours.

`deploy.Retryable` classifies **unrecognised errors as non-retryable**. Every
error that could succeed on a second attempt is recognised explicitly —
`net.Error`, smithy server faults, the throttling family, `ServerException` —
and the SDK has already retried transient transport failures inside the call.
Errors this module raises itself carry `awsecs.ErrPermanent` or
`deploy.ErrUnknownTarget` / `ErrInvalidRequest`. Retrying anything else costs a
`DEPLOYMENT_INITIATING` + `DEPLOYMENT_FAILED` Slack pair per attempt *and* an
EventBridge redelivery of the whole event: six posts for one push on a condition
that will never clear. One missed deployment, logged at ERROR, is the cheaper
mistake.

Initialization follows the same rule and **never calls `os.Exit`**. A Lambda
that fails to start fails every async invocation, and EventBridge redelivers
each of them twice more — an unbootable Lambda is a permanent retry storm across
every rule the project owns. A bad configuration serves `ignored` and logs
`event=configuration_invalid` at ERROR on every invocation instead.

## Build

Terraform builds the binary at apply time; it is never committed.

```
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o .build/<env>/bootstrap .
go run ./tools/mkzip -in .build/<env>/bootstrap -out .build/<env>/ci_lambda.zip
```

- **A Go toolchain (1.22+) is required on every machine that runs
  `terraform apply`**, including CI runners. The provisioner pre-flights `go` and
  fails with an actionable message if it is missing. It will never substitute a
  placeholder: a placeholder deploys green and then fails every invocation with
  `Runtime.InvalidEntrypoint`.
- The artifact lives under `.build/<env>/`, so two environments applying
  concurrently cannot clobber each other's binary.
- There is deliberately no `data.archive_file`. A data source is read during the
  plan walk, so on any checkout without a prebuilt artifact — every fresh clone,
  every CI runner — `plan`, `apply` and `destroy` would all fail before starting.
  `filename` is only read by the provider during Create/Update.
- The build trigger hashes the sources **and** `GOOS`, `GOARCH` and the build
  command, so switching architecture actually rebuilds.
- `task lambda:build env=<env>` and `task lambda:package env=<env>` run exactly
  the two commands above, to the same paths, for inspecting the artifact locally.
  Nothing else builds this binary: meroku itself does not, and there is no
  module-root `bootstrap`.

## Tests

```
go build ./...
go vet ./...
go test ./...          # includes the Terraform <-> Go boundary check
gofmt -l .             # must print nothing
```

Everything runs with no AWS credentials, no network and no `terraform` binary.

### The boundary check

`internal/boundary` holds the mechanism that makes a Terraform/Go identifier
split impossible to reintroduce:

1. `internal/boundary/testdata/tfgolden/` is a synthetic project — hyphenated
   service, two services sharing one repository, a service literally named
   `task`, a scheduled task, an off-prefix `manual_repo` repository, and one
   service and one scheduled task with `auto_deploy = false` — wired to the real
   `tf_identifiers` module. Its own inputs are captured alongside its output, so
   the disabled-target assertions are derived from what Terraform was given
   rather than from a hardcoded name that can quietly stop matching.
2. `internal/boundary/testdata/tf_identifiers.golden.json` is the committed
   capture of what Terraform emits for it.
3. `boundary_test.go` runs on every `go test`, with no build tag: it feeds the
   golden file to the real `config.Load` and asserts that every repository,
   every parameter prefix and every env file resolves to a real target.
4. `lambdatf_guard_test.go` reads `../../lambda.tf` and fails if an identifier is
   hardcoded, if `data.archive_file` returns, if the environment-scoped manual
   rule grows a `detail` filter or the global one loses it, if the manual source
   list stops being environment-scoped, if dead configuration comes back, if the
   over-2,048-character ECR fallback stops listing the off-prefix repositories or
   loses its overflow `precondition`, or if `auto_deploy` turns from a flag into
   a filter on any of the target maps.
5. `lambdatf_names_test.go` covers the channels the golden capture cannot see,
   because a golden file only proves the *contents* agree, not the names they
   travel under:
   - the environment variable **names**, taken from the AST of `config.go` and
     compared against the `environment { variables { ... } }` block, in both
     directions. Renaming `ECR_REPO_MAP` on one side used to leave `Validate`,
     `SelfCheck` and every test green while auto-deploy was 100% dead and every
     log line read "not mapped to any target in this project";
   - the **JSON field names inside** those maps, taken from the struct tags of
     `config.Target` and `config.S3File`. `bucket`, `key` and `type` fail open;
     `encoding/json` drops an unknown field without a word;
   - the **arguments `module.ci_identifiers` is called with**, required to be the
     same set in `lambda.tf`, in the golden fixture, and in the module's own
     `variables.tf`, so the fixture cannot stop mirroring the real call;
   - the **event patterns**, compared against `handler.PatternContracts()`, whose
     field names come from the tags of the types that parse those events. A
     pattern that stops matching the parser is otherwise invisible from both
     sides: EventBridge simply never invokes the Lambda.

Regenerate the capture after changing `tf_identifiers`:

```
go test -tags tfgolden ./internal/boundary/ -update
```

Check it for drift (this is the only test that needs `terraform`):

```
go test -tags tfgolden ./internal/boundary/
```

CI must run `go test ./...` and the tagged drift check on every change under
`modules/workloads/**`.

## Logging

`log/slog` with a JSON handler on stdout. Keys are `timestamp`, `level`
(lowercase) and `message`, with `project` and `env` as base attributes.

Attributes are **flat**. The previous logger nested everything under `fields.*`;
any saved Logs Insights query on `fields.x` needs updating to `x`.

A failed contract self-check logs at `error` with `event=contract_selfcheck_failed`
and carries on. Alarm on that attribute — refusing to start would queue and retry
every matching event in the account.

## Manual smoke test

```bash
FN=<project>_ci_lambda_<env>

# backend push
aws lambda invoke --function-name "$FN" --payload '{
  "id":"1","source":"aws.ecr","detail-type":"ECR Image Action",
  "account":"000000000000","region":"us-east-1",
  "detail":{"repository-name":"<project>_backend","image-tag":"smoke","action-type":"PUSH","result":"SUCCESS"}
}' --cli-binary-format raw-in-base64-out /dev/stdout

# service push (fans out to every consumer of the repository)
aws lambda invoke --function-name "$FN" --payload '{
  "id":"2","source":"aws.ecr","detail-type":"ECR Image Action",
  "account":"000000000000","region":"us-east-1",
  "detail":{"repository-name":"<project>_service_<name>","image-tag":"smoke","action-type":"PUSH","result":"SUCCESS"}
}' --cli-binary-format raw-in-base64-out /dev/stdout

# scheduled task push (registers a new revision)
aws lambda invoke --function-name "$FN" --payload '{
  "id":"3","source":"aws.ecr","detail-type":"ECR Image Action",
  "account":"000000000000","region":"us-east-1",
  "detail":{"repository-name":"<project>_task_<name>","image-tag":"smoke","action-type":"PUSH","result":"SUCCESS"}
}' --cli-binary-format raw-in-base64-out /dev/stdout

# SSM env change (an Update of the env parameter: the main configuration path)
aws lambda invoke --function-name "$FN" --payload '{
  "id":"4","source":"aws.ssm","detail-type":"Parameter Store Change",
  "detail":{"operation":"Update","name":"/<env>/<project>/backend/env","type":"SecureString"}
}' --cli-binary-format raw-in-base64-out /dev/stdout

# a newly ADDED parameter, which Parameter Store reports as Create
aws lambda invoke --function-name "$FN" --payload '{
  "id":"4b","source":"aws.ssm","detail-type":"Parameter Store Change",
  "detail":{"operation":"Create","name":"/<env>/<project>/backend/DATABASE_URL","type":"SecureString"}
}' --cli-binary-format raw-in-base64-out /dev/stdout

# Terraform's own placeholder: must answer "ignored" on Create, and deploy on Update
aws lambda invoke --function-name "$FN" --payload '{
  "id":"4c","source":"aws.ssm","detail-type":"Parameter Store Change",
  "detail":{"operation":"Create","name":"/<env>/<project>/backend/env","type":"SecureString"}
}' --cli-binary-format raw-in-base64-out /dev/stdout

# S3 env file write
aws lambda invoke --function-name "$FN" --payload '{
  "id":"5","source":"aws.s3","detail-type":"AWS API Call via CloudTrail",
  "detail":{"eventName":"PutObject","requestParameters":{"bucketName":"<bucket>","key":"<key>"}}
}' --cli-binary-format raw-in-base64-out /dev/stdout

# manual deploy
aws lambda invoke --function-name "$FN" --payload '{
  "id":"6","source":"action.deploy","detail-type":"DEPLOY",
  "detail":{"service":"backend","project":"<project>","env":"<env>"}
}' --cli-binary-format raw-in-base64-out /dev/stdout

# another project's repository: must answer "ignored" and deploy nothing
aws lambda invoke --function-name "$FN" --payload '{
  "id":"7","source":"aws.ecr","detail-type":"ECR Image Action",
  "account":"000000000000","region":"us-east-1",
  "detail":{"repository-name":"someoneelse_backend","image-tag":"x","action-type":"PUSH","result":"SUCCESS"}
}' --cli-binary-format raw-in-base64-out /dev/stdout

# a target with auto_deploy: false — note the reason names the target, and does
# NOT claim the repository is unmapped
aws lambda invoke --function-name "$FN" --payload '{
  "id":"8","source":"aws.ecr","detail-type":"ECR Image Action",
  "account":"000000000000","region":"us-east-1",
  "detail":{"repository-name":"<project>_service_<name>","image-tag":"smoke","action-type":"PUSH","result":"SUCCESS"}
}' --cli-binary-format raw-in-base64-out /dev/stdout
# {"status":"ignored","detail":"auto_deploy is disabled for <name>"}

# the current policy, in one value
aws lambda get-function-configuration --function-name "$FN" \
  --query 'Environment.Variables.AUTO_DEPLOY_MAP' --output text
```

Set `DRY_RUN=true` on the function to exercise the routing without touching ECS.
