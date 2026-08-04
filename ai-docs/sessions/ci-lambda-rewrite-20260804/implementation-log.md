# CI Lambda rewrite — implementation log

Date: 2026-08-04.
Scope actually touched: `modules/workloads/ci_lambda/**` and `modules/workloads/lambda.tf`. Nothing else.
Tool versions used: Go 1.26.5 (darwin/arm64), Terraform v1.15.8 (darwin_arm64).

Sources: `requirements.md` (D1–D17, FR/NFR), `architecture.md`, and
`reviews/plan-review-internal.md`. The review overrides the architecture; corrections C1–C8
from the task brief were implemented as decided, not re-litigated.

---

## 1. What shipped

### Go module — rewritten from scratch

Deleted: `cmd/`, `config/`, `deployer/`, `handlers/`, `services/`, `utils/`, `Makefile`,
`slack.message.{success,error,info}.json.tmpl`, and the untracked `bootstrap` file.

New layout:

```
main.go                       cold start, wiring, lambda.Start
contract/                     backend_id + task_id_prefix; read by HCL, embedded by Go
tf_identifiers/               provider-free Terraform module: identifier fan-out
tools/mkzip/                  packages the bootstrap binary (replaces the `zip` CLI)
internal/config/              env vars -> Config, resolvers, SelfCheck
internal/awsecs/              the only code that talks to ECS (SDK v2)
internal/slack/               notifications; Notify returns nothing
internal/deploy/              retry policy, notification policy
internal/handler/             one file per event source
internal/boundary/            Terraform <-> Go boundary check + lambda.tf guards
internal/testsupport/         the shared fixture every test loads
```

`go.mod`: `go 1.22`; `aws-sdk-go-v2` + `aws-sdk-go-v2/config` + `service/ecs` + `smithy-go`;
`aws-lambda-go v1.46.0`; `testify v1.9.0`. **`aws-sdk-go` v1 is absent** (NFR-1).

### `modules/workloads/lambda.tf` — rewritten

Build (C1), identifiers module, four scoped event rules + the S3 rules, tightened IAM,
new env vars, dead config removed.

---

## 2. The corrections, one by one

### C1 — no `data.archive_file`; the provisioner emits the zip

`null_resource.build_ci_lambda` runs `go build` **and** `go run ./tools/mkzip`;
`aws_lambda_function.lambda_deploy` takes `filename = <zip path>` and
`source_code_hash = null_resource.build_ci_lambda.triggers.src`.

Reproduced both designs in an isolated scratch module (no AWS), simulating a fresh clone —
artifact deleted, sources unchanged:

```
### OLD (null_resource + data.archive_file)
--- terraform plan ---
  with data.archive_file.lambda,
error creating archive: error archiving file: could not archive missing file: ./src/bootstrap
--- terraform destroy ---
  with data.archive_file.lambda,
error creating archive: error archiving file: could not archive missing file: ./src/bootstrap

### NEW (filename + trigger hash)
--- terraform plan ---
Terraform has compared your real infrastructure against your configuration
and found no differences, so no changes are needed.
--- terraform destroy ---
Destroy complete! Resources: 1 destroyed.
```

A source change still rebuilds (`source_code_hash` moved from `5d1c8cd…` to `69c6e9e…` and
both artifacts reappeared on disk).

Also verified in scratch: `null_resource.triggers.src` is **known at plan time** even for a
resource being created, so `source_code_hash` never shows `(known after apply)` and produces
no churn.

Other decisions inside C1:

- **Pre-flight.** `command -v go` with a message naming the toolchain, the version, the
  download URL and why no placeholder is substituted. Verified by running the exact
  provisioner script under `env -i` (no `go` on PATH): exit 1 with the intended text.
- **arm64.** `architectures = ["arm64"]`, and `goos`/`goarch`/`build_cmd` are in `triggers`
  (finding 14), so flipping architecture actually rebuilds.
- **No `zip` CLI.** `tools/mkzip` uses `archive/zip` and sets mode 0755 on the entry. Go is
  already a hard requirement; `zip` is missing from many build images, and an artifact
  without the exec bit fails with `Runtime.InvalidEntrypoint`. Verified:
  `bootstrap mode=100755` inside the archive.
- **Per-environment build dir** (finding 13): `ci_lambda/.build/${var.env}/`. `env/dev` and
  `env/prod` both resolve `path.module` to the same directory; a shared `bootstrap` meant a
  concurrent apply could zip a half-written binary. mkzip also writes to a temp file and
  renames.
- **Source hash filter.** `fileset(dir, "**/*.{go,json,mod,sum,tmpl}")` matches build output
  (`.build/**/*.json`) and any local `.terraform/` — a feedback loop. Filtered with
  `regexall("(^|/)[.](build|terraform)/", f)`; verified in scratch that the filter removes
  exactly those and keeps `main.go`, `sub/a.go`, `go.mod`, `go.sum`, `c.json`, `t.tmpl`.

### C2 — manual rule carries no `detail` filter

```hcl
source      = local.ci_manual_sources   # distinct(action.deploy, action.production,
                                        #          action.${env}, github.actions.${env})
detail-type = ["DEPLOY", "SERVICE_DEPLOY"]
```

No `detail` block. `internal/handler/manual.go` does the check: if the payload carries
`project` or `env` and they disagree with ours, the event is `ignored`; absent fields are
allowed, because no generator emits `project` today.

`distinct()` collapses the duplicate `action.production` when `env == "production"`
(finding 19) — verified: `["action.deploy", "action.production", "github.actions.production"]`.

A guard test fails the build if `detail =` reappears inside that rule.

### C3 — S3 rules cover per-service env files

New `local.all_env_files_s3 = distinct(concat(local.env_files_s3, flatten([... services ...])))`
drives all three S3 resources (rule, target, permission) and the `s3_event_rule_keys`/`names`
locals. `S3_SERVICE_MAP` now carries backend files **and** per-service files, with bucket
names verbatim (Q2 — confirmed against `backend.tf:129`, `services.tf:155` and both S3 IAM
policies, all of which use `arn:aws:s3:::${file.bucket}/${file.key}` raw).

### C4 — the over-engineering is gone

`contract.json` is two keys:

```json
{ "backend_id": "backend", "task_id_prefix": "task:" }
```

No `repo_*_fmt`, no `ssm_*_fmt`. No `ParseRepo`, no `ExpectedRepo`, no SelfCheck item 2.
There is no repository-name parser left anywhere in the Go module.

Repository names and SSM prefixes now come from the resources that create them
(review finding 4):

| Input to `tf_identifiers` | Source |
|---|---|
| `backend_repo` | `try(aws_ecr_repository.backend[0].name, "")` |
| `service_repos` (create_ecr) | `aws_ecr_repository.services[name].name` |
| `service_repos` (use_existing) | `aws_ecr_repository.services[source_service_name].name` |
| `service_repos` (manual_repo) | `repository_uri` with registry host and `:tag`/`@digest` stripped |
| `backend_ssm_prefix` | `trimsuffix(aws_ssm_parameter.backend_env.name, "/env")` |
| `service_ssm_prefixes` | `trimsuffix(aws_ssm_parameter.services_env[n].name, "/env")` |

Verified the `manual_repo` URI stripping in scratch — it handles namespaced repos, `host:port`
registries and digests (finding 9):

```
000000000000.dkr.ecr.us-east-1.amazonaws.com/team/legacy-api:v1  -> team/legacy-api
registry.example.com:5000/team/legacy-api                        -> team/legacy-api
000000000000.dkr.ecr.us-east-1.amazonaws.com/acme_svc@sha256:aaaa -> acme_svc
""                                                               -> ""
```

The mechanism that makes D1 un-reintroducible: **Go never derives an identifier.**
`ECR_REPO_MAP` (repo -> [ids]) and `SSM_SERVICE_MAP` (prefix -> id) join the existing maps;
`config.IdentifiersForRepo` / `IdentifierForSSMPath` / `IdentifiersForS3` are pure lookups.
An unmapped key logs at INFO and returns `{"status":"ignored"}` with a nil error. D2 dies with
it — a foreign project's repository is simply absent from the map.

### C5 — golden-file boundary check, no `terraform` on the gate

- `internal/boundary/testdata/tfgolden/main.tf` — synthetic project wired to the real
  `tf_identifiers` module: hyphenated service (`payment-worker`), two services sharing one
  repository (`reporting` -> `acme_service_api`, the `use_existing` fan-out), a service
  literally named `task`, and a scheduled task (`cleanup`).
- `internal/boundary/testdata/tf_identifiers.golden.json` — the committed capture, including
  the exact `ECS_SERVICE_MAP` / `SCHEDULED_TASK_MAP` / `S3_SERVICE_MAP` / `ECR_REPO_MAP` /
  `SSM_SERVICE_MAP` strings.
- `boundary_test.go` — **always on**, no build tag, no terraform, no network, no credentials.
  Feeds the golden file to the real `config.Load` and asserts SelfCheck is clean, every ECR
  repository resolves to exactly the identifiers Terraform expects, every SSM prefix resolves,
  every S3 entry resolves, the shared repository fans out to more than one target, another
  project's repository resolves to nothing, and `/dev/acme/task/cleanup/env` beats
  `/dev/acme/task/env` on longest prefix.
- `regen_test.go` (`-tags tfgolden`) — the only test that shells out to terraform. Runs the
  fixture **in place** (a copy would break the relative module source — review finding 5a) and
  fails on any byte of drift; `-update` rewrites the file. Cleans up its own state files;
  `TF_DATA_DIR` points at a temp dir so no `.terraform` lands in the tree.
- `lambdatf_guard_test.go` — always on. Reads `../../../lambda.tf` (comments stripped) and
  fails if an identifier literal appears in a key position, if the module references
  disappear, if `data.archive_file` returns, if a build trigger goes missing, if the manual
  rule grows a `detail` filter, or if `SERVICE_CONFIG` / `DEPLOYMENT_TIMEOUT_SECONDS` /
  `service_config` / `ecs:ListTaskDefinitions` come back.

`internal/testsupport` loads the same golden file for **every** unit test, so no test can
invent a map key. That is the specific habit that let the old suite pass green while the
backend path was dead.

### C6 — one atomic change

Everything landed together: Terraform env vars and the Go code that reads them are in the
same change; there is no intermediate state.

Note on the dummy-artifact hazard: `app/terrafrom.go:ensureLambdaBootstrapExists` is outside
this scope and is being removed by a concurrent agent. **It is already harmless here** — the
build writes `ci_lambda/.build/<env>/bootstrap` and zips from there, so a stray 100-byte text
file at `ci_lambda/bootstrap` is read by nothing and (having no matching extension) does not
enter the source hash either.

### C7 — SelfCheck reports, it does not gate

`Config.SelfCheck(backendID) []string` returns problems. `main.go` logs them at `error` with
`event=contract_selfcheck_failed` and starts normally. Alarm on that attribute. Failing
initialization would queue and retry every matching event in the account through the async
invoke path — the exact behaviour FR-9/D14 exist to remove.

### C8 — quotas, SSM operation, duplicate source

- **ECR pattern size.** `local.ci_ecr_pattern` picks the exhaustive repository list when the
  encoded pattern is `<= 2048` characters and falls back to
  `[{ prefix = "${var.project}_" }]` otherwise, rather than emitting a rule AWS rejects with
  `InvalidEventPatternException`. `ECR_REPO_MAP` stays the authority, so anything the looser
  filter lets through is `ignored`. Verified in scratch: 2 repositories -> explicit list;
  120 long-named repositories -> 5,542 characters -> prefix fallback.
- **Empty list.** The ECR rule, target and permission take
  `count = length(local.ci_ecr_repos) > 0 ? 1 : 0`. With `ecr_strategy = "cross_account"` and
  no locally built services there is no repository, no possible event, and
  `repository-name = []` would be invalid.
- **SSM operation.** `operation = ["Update"]` (finding 18) plus a defensive check in the
  handler. Terraform creates these parameters itself, and a delete removes the configuration
  a service needs.
- **Duplicate source.** `distinct()` (finding 19).

---

## 3. The uncontested fixes from requirements.md

| Item | What was done |
|---|---|
| SDK v1 -> v2, Go 1.22+ | Done. `grep -rn '"V2"\|V2\b\|aws-sdk-go/aws\|v2-direct-naming'` over the module returns nothing. |
| ctx end to end | Every handler, every deploy call and every SDK call takes the handler ctx. `Deploy(ctx, req)` — the old `deployer.Deploy(opts)` dropped it. Slack calls are bounded by `min(5s, remaining deadline)`. |
| `log/slog` JSON | `main.go:newLogger`. `ReplaceAttr` maps `time`->`timestamp`, `msg`->`message`, lowercases `level`. Attributes are flat (documented break of `fields.*` Logs Insights queries). |
| `V2` suffixes and the `architecture` log field | Gone. |
| `getLatestTaskDefinition` | Deleted. `UpdateService` receives the bare **family** and ECS resolves the latest ACTIVE revision; the resolved ARN is read from the response. `DescribeTaskDefinition` uses an exact family name, not `FamilyPrefix`. `ecs:ListTaskDefinitions` dropped from IAM. |
| Slack must never fail a deployment | `Notifier.Notify(ctx, Message)` returns nothing — enforced by type. `handleECSEvent`'s equivalent returns `{"status":"notified"}, nil` always. |
| Retry backoff | Genuinely exponential: `base * 2^(attempt-1)`, ±20% jitter, capped at 10s, ctx-aware, stops early when the remaining deadline cannot fit another attempt. Asserted: `100ms, 200ms, 400ms`. |
| Retry classification | Configuration errors (`ErrUnknownTarget`, `ErrInvalidRequest`) resolve **outside** the loop and never reach AWS. Non-retryable ECS errors fail on attempt 1. |
| `SERVICE_CONFIG`, `DEPLOYMENT_TIMEOUT_SECONDS`, `local.service_config` | Removed; a guard test keeps them out. |
| Empty-string sentinel, `(backend)` / `(unknown)` | Gone. `backend` is a real string everywhere. |
| D6 field carry-forward | `RegisterRevisionWithImage` carries `PidMode`, `IpcMode`, `InferenceAccelerators`, `Volumes`, `PlacementConstraints`, `RequiresCompatibilities`, `Cpu`, `Memory`, `ProxyConfiguration`, `EphemeralStorage`, `RuntimePlatform`, `EnableFaultInjection`, `TaskRoleArn`, `ExecutionRoleArn`, `NetworkMode` and tags. Asserted field by field. |
| D6 image parsing | `SplitImageRef` splits on `@` first, then on the last `:` only when it follows the last `/`. Handles digests and `host:port`. |
| FR-12 | Zero matching containers is an error, not a silent identical re-registration. |
| D10 templates | Recreated under `internal/slack/templates/`, embedded, every interpolation through a `json` template func. A test renders every template against every message shape (including quotes, newlines and backslashes) and asserts `json.Valid`. |
| D16 / D17 | Longest path-segment prefix lookup. `/dev/acme/task/cleanup/env` and `/dev/acme/payment-worker/env` both resolve; `/dev/acme/backendish/env` does not. |
| FR-18 IAM | `ecs:UpdateService` scoped to `concat([aws_ecs_service.backend.id], values(aws_ecs_service.services)[*].id)`; `iam:PassRole` gains `iam:PassedToService = ecs-tasks.amazonaws.com`; `ecs:ListTaskDefinitions` dropped; Describe/Register/TagResource stay on `*` (no resource-level support). |
| ECS-state cross-project noise | The ECS rule filters `resources` by the service-ARN prefix derived from the cluster ARN. Verified: `arn:...:cluster/acme_cluster_dev` -> `arn:...:service/acme_cluster_dev/`. |

Also adopted from the review, outside C1–C8:

- **Finding 12** — task ECR repositories are gated on `var.env == "dev"`, matching
  `modules/ecs_task/main.tf:45`. Elsewhere the task pulls from a cross-account URL, so listing
  the repository would only put an unreachable key in the map and in the event pattern.
  Scheduled tasks remain in `SCHEDULED_TASK_MAP` and `SSM_SERVICE_MAP` in every environment,
  so the manual path (now fixed by C2) works for non-dev tasks.
- **Findings 16, 17** — the module's HCL is real, `terraform validate`-clean HCL; dotted
  references in object-key position are parenthesised, e.g.
  `(module.ci_identifiers.backend_id) = { ... }`.
- **SSM change on a scheduled task** returns `ignored` with a clear reason rather than
  attempting a revision: ECS resolves `secrets`/`valueFrom` when the task starts, so the next
  scheduled run picks the value up on its own.

---

## 4. Verification — actual output

All run from a clean state.

```
$ cd modules/workloads/ci_lambda

$ go build ./...
exit=0

$ go vet ./...
exit=0

$ gofmt -l .
(no files -- all formatted)

$ go test ./... -count=1
?   	madappgang.com/infrastructure/ci_lambda	[no test files]
ok  	madappgang.com/infrastructure/ci_lambda/contract	0.494s
ok  	madappgang.com/infrastructure/ci_lambda/internal/awsecs	0.605s
ok  	madappgang.com/infrastructure/ci_lambda/internal/boundary	0.716s
ok  	madappgang.com/infrastructure/ci_lambda/internal/config	0.161s
ok  	madappgang.com/infrastructure/ci_lambda/internal/deploy	1.072s
ok  	madappgang.com/infrastructure/ci_lambda/internal/handler	1.410s
ok  	madappgang.com/infrastructure/ci_lambda/internal/slack	1.553s
?   	madappgang.com/infrastructure/ci_lambda/internal/testsupport	[no test files]
?   	madappgang.com/infrastructure/ci_lambda/tools/mkzip	[no test files]
exit=0

$ go test -tags tfgolden ./internal/boundary/ -count=1
=== RUN   TestGoldenMatchesTerraform
--- PASS: TestGoldenMatchesTerraform (0.09s)
ok  	madappgang.com/infrastructure/ci_lambda/internal/boundary	0.439s
```

```
$ cd modules/workloads

$ terraform fmt -check -recursive lambda.tf ci_lambda/
exit=0 (no filenames listed = formatted)

$ terraform fmt -check -recursive .
services.tf
exit=3

$ terraform init -backend=false -input=false
Terraform has been successfully initialized!

$ terraform validate
Success! The configuration is valid, but there were some validation warnings
as shown above.
```

**`terraform fmt -check` over the whole of `modules/workloads` fails on `services.tf`.**
That is pre-existing and not mine: `git diff --stat -- modules/workloads/services.tf` is empty,
and `services.tf` is outside the touch scope for this change. The offending hunk is a list
indentation at `services.tf:146-149`. Everything this change owns is clean.

The `terraform validate` warnings are all pre-existing and all in `backend.tf` /
`pgadmin.tf` / `services.tf`: `failure_threshold` deprecation on
`aws_service_discovery_service`, and `data.aws_region.current.name` deprecated in favour of
`.region`. None originates in `lambda.tf` or `ci_lambda/`.

### The artifact

Built with the exact command the provisioner runs:

```
$ file ci_lambda/.build/dev/bootstrap
ci_lambda/.build/dev/bootstrap: ELF 64-bit LSB executable, ARM aarch64, version 1 (SYSV),
statically linked, Go BuildID=..., stripped

$ ls -l ci_lambda/.build/dev/
-rwxr-xr-x  13107362  bootstrap
-rw-r--r--   4569916  ci_lambda.zip

$ python3 -c "import zipfile; ..."
bootstrap mode=100755
```

A real Linux arm64 ELF, statically linked, stripped — not a text file — and the zip entry
carries the executable bit.

### Not run, and why

**`terraform plan` / `apply` / `destroy` against `modules/workloads` itself.** The AWS
provider validates credentials before evaluating anything:

```
Error: Retrieving AWS account details: validating provider credentials: retrieving caller
identity from STS: ... InvalidClientTokenId: The security token included in the request is
invalid.
```

No AWS account is available here, so the end-to-end plan is unverified. The pieces that do not
need AWS were each verified in isolation, as described above; the pieces that do — a real
apply producing a working Lambda, an actual ECR push deploying, EventBridge accepting the
patterns — remain to be exercised against a scratch project (architecture §9 step 12).

---

## 5. Left undone, clearly marked

1. **CI workflow (`.github/workflows/ci.yml`) — NOT created.** It is outside the
   "touch only `modules/workloads/ci_lambda/**` and `modules/workloads/lambda.tf`" scope for
   this change. The gate itself is real and unconditional — the boundary check is a plain
   `go test` with no build tag and no terraform dependency, so any job running
   `go test ./...` enforces it. What is still needed is a job that runs, on every PR touching
   `modules/workloads/**`:

   ```yaml
   - run: go build ./... && go vet ./... && go test ./... -count=1
     working-directory: modules/workloads/ci_lambda
   - run: test -z "$(gofmt -l .)"
     working-directory: modules/workloads/ci_lambda
   - run: go test -tags tfgolden ./internal/boundary/ -count=1   # golden drift, needs terraform
     working-directory: modules/workloads/ci_lambda
   - run: terraform fmt -check -recursive lambda.tf ci_lambda/ && terraform init -backend=false && terraform validate
     working-directory: modules/workloads
   ```

   Until that exists, D11 ("nothing runs the tests") is fixed in capability but not in
   enforcement.

2. **`app/terrafrom.go:ensureLambdaBootstrapExists`, `app/terraform_destroy_progress_tui.go:153`
   and `app/deploy.go:buildDeploymentLambda` are not deleted here** — `app/` is being edited
   concurrently by another agent and is out of scope. As noted under C6, the new build path
   makes them inert rather than dangerous, but they are dead code and the `GOARCH=amd64`
   `os.Setenv` leak in `app/deploy.go:475` should still go.

3. **`Taskfile.yml`, `project/Taskfile.yml`, root `.gitignore`, `env/main.hbs` — not updated.**
   Out of scope. `modules/workloads/ci_lambda/.gitignore` was added instead and covers
   `.build/`, `bootstrap`, `*.zip` and the fixture's local terraform state. The Taskfile's
   `lambda:build` target still describes the old single-artifact build; it should become
   `go build ... -o .build/<env>/bootstrap` plus `lambda:test` / `lambda:test:golden`.

4. **`var.lambda_path` in `modules/workloads/variables.tf` is now unused.** It is deliberately
   left declared — already-generated `project/env/*/main.tf` pass it, and passing an undeclared
   variable is a hard Terraform error. It cannot be marked deprecated from here (`variables.tf`
   is out of scope). `env/main.hbs` should stop emitting it for new generations.

5. **Scheduled-task name formats are still duplicated.** `local.ci_task_repos`
   (`{project}_task_{name}`) and `local.ci_task_ssm_prefixes`
   (`/{env}/{project}/task/{name}`) are written in `lambda.tf` because the real resources live
   in `modules/ecs_task`, which is out of scope. Each carries a comment naming the file it
   mirrors. Closing this properly needs two new outputs from `modules/ecs_task` (the
   repository name and the parameter name) plumbed through `env/main.hbs`.

6. **The web CI/CD generators are unchanged.** They are out of scope, and C2 was designed so
   they do not need changing: the rule accepts `action.{env}` / `DEPLOY` and
   `github.actions.{env}` / `SERVICE_DEPLOY` exactly as emitted today, and the handler accepts
   payloads with or without `project`/`env`. Tests pin all four payload shapes the generators
   produce. If they are ever updated to send `{"project", "env", "service"}`, the handler
   already enforces both.

7. **Residual manual-deploy scoping gap.** With no `detail` filter, two meroku projects in one
   AWS account both receive an unscoped `action.deploy` / `DEPLOY` / `{"service":"backend"}`
   event and each deploys *its own* backend. That is the status quo, and it is not a
   mis-deployment (nothing deploys another project's image), but it is not isolation either.
   Closing it needs the generators to emit `detail.project`, at which point the handler's
   existing check makes it exact.

8. **One drift case the build triggers do not cover.** If the artifact is deleted *and* the
   function needs to be created from scratch (state removed, or the function deleted out of
   band) while the source hash is unchanged, `filename` points at a zip that is not there.
   A `fileexists` probe trigger would cover it, at the cost of two churning applies before the
   state stabilises. Judged not worth it: the ordinary paths (fresh clone + unchanged source
   -> no code update needed; source change -> provisioner runs) are all safe, as reproduced
   above.

---

## 6. Defect coverage

| ID | Status |
|---|---|
| D1 backend deploy broken | Fixed. Go never derives an identifier; `backend` is a real string; the boundary test fails if either side changes it. |
| D2 cross-project mis-deployment | Fixed. Lookup-only resolution + repository allow-list in the ECR pattern + ECS rule scoped by service-ARN prefix. |
| D3 wrong revision | Fixed at the root: `ListTaskDefinitions` deleted, family handed to ECS, exact-family describe. |
| D4 ctx dropped | Fixed end to end. |
| D5 manual-deploy contract | Fixed. Rule accepts every source/detail-type the generators emit; no `detail` filter, so nothing that works today breaks. |
| D6 dropped task-definition fields | Fixed, asserted field by field. |
| D7 backend S3 no-op | Fixed. `S3_SERVICE_MAP` includes backend files. |
| D8 per-service S3 rules missing + bucket rewriting | Fixed. Rules driven by `local.all_env_files_s3`; buckets verbatim. |
| D9 `use_existing` / `manual_repo` | Fixed. Fan-out map; repository names from resource attributes. |
| D10 dead, invalid templates | Fixed. Embedded, JSON-escaped, tested. |
| D11 no CI | **Partial.** Tests exist and are unconditional; the workflow file is out of scope (see §5.1). |
| D12 dummy bootstrap | Fixed on the Terraform side; the `app/` writer is inert against the new path but still needs deleting (§5.2). |
| D13 config errors burn retries | Fixed. Resolution is outside the loop; classification via `errors.As`. |
| D14 unknown events retried | Fixed. `ignored` + nil error. SelfCheck reports rather than failing init (C7). |
| D15 deployer tests test a copy | Fixed. Tests drive the real `Deployer` through a fake ECS. |
| D16 scheduled-task SSM path | Fixed by longest-prefix lookup. |
| D17 hyphenated service names | Fixed; no regex left. |

---

# Review remediation — 2026-08-04

Response to `reviews/code-review-internal.md` (verdict CONDITIONAL, 4 HIGH).
All four are fixed. Every command quoted below was run against this tree.

## H4 — manual deploys were not environment- or project-scoped

`lambda.tf` put `action.production` in **every** environment's manual rule, so a
production deploy event matched the dev rule, the staging rule, and every other
meroku project's rule in the account. The shipped emitters sent no scoping at
all, so the handler's project/env check never engaged. Reproduced live before
the fix: an event with `source: action.production` deployed the **dev** backend.

The rule is now split in two, because the sources fall into two kinds and only
one of them can carry a filter.

**`ci_manual_deploy` — environment-scoped sources, no detail filter.**
`local.ci_manual_sources_scoped` is `["action.${var.env}",
"github.actions.${var.env}"]`, plus `"action.production"` only when
`contains(local.ci_production_envs, var.env)` (`["prod", "production"]` — the
shipped receipt hardcodes that source with `ENV: prod`). Another environment's
event can no longer arrive at all, which is what makes it safe to leave
unfiltered — and it must stay unfiltered, because EventBridge requires every key
named in a pattern to be present and payloads already in the wild send only
`{"service": "..."}`.

**`ci_manual_deploy_global` — environment-agnostic sources, detail filter
required.** `action.deploy` names no environment, so no source list can scope
it; this rule requires `detail.project = [var.project]` and
`detail.env = [var.env]`. A payload omitting them now matches nothing instead of
matching everything.

Backward compatibility was checked, not assumed: `git log -S'action.deploy' --
web/ receipts/ docs/ env/ templates/` is **empty**. No meroku generator, receipt,
doc or template has ever emitted that source — it only ever appeared in the
rule's accept list. Anything hand-written that emits it was, by construction,
deploying every project in the account. Legacy payloads on the *scoped* sources
keep working untouched; that is the path the "both paths exist" comment in
`lambda.tf` and `handler/manual.go` explains.

Rendered patterns (real `terraform apply` on the same expressions):

```
dev.ci_manual_deploy         source = ["action.dev", "github.actions.dev"]
staging.ci_manual_deploy     source = ["action.staging", "github.actions.staging"]
prod.ci_manual_deploy        source = ["action.prod", "github.actions.prod", "action.production"]
production.ci_manual_deploy  source = ["action.production", "github.actions.production"]

*.ci_manual_deploy_global    source = ["action.deploy"]
                             detail = { project = ["acme"], env = ["<env>"] }
```

`action.production` is gone from dev and staging. Handler behaviour, printed
from the real `Handler` with `PROJECT_NAME=acme PROJECT_ENV=production`:

```
detail={"env":"production","project":"otherproj","service":"backend"}
  -> {"status":"ignored","detail":"manual deploy targets project otherproj"}  err=<nil>  deployed=[]
detail={"env":"dev","project":"acme","service":"backend"}
  -> {"status":"ignored","detail":"manual deploy targets environment dev"}    err=<nil>  deployed=[]
detail={"env":"production","project":"acme","service":"backend"}
  -> {"status":"deployed","deployed":["backend"]}                             err=<nil>  deployed=[backend]
```

Emitters updated to send `project` and `env`, matching the shape
`web/src/components/Sidebar.tsx` and `ServiceCICDConfiguration.tsx` already emit
(`web/` not touched):

- `receipts/github/prod-deploy.yml` — now `Source=action.${{ env.ENV }}`,
  `Detail={"service":"backend","project":"...","env":"..."}`.
- `docs/ECR_STRATEGY.md` — both examples, plus a paragraph on why both fields
  are required and an `action.deploy` example showing the filtered path.

Also in that receipt: `AWS_ACCOUNT_ID` carried a **real 12-digit AWS account
ID** in a public repository. Replaced with `000000000000`. Pre-existing,
unrelated to the review, fixed because the file was open and `CLAUDE.md` forbids
it absolutely.

Tests: `TestManualDeployFromAnotherProjectOnAProductionSourceIsIgnored`,
`TestManualDeployFromAnotherEnvironmentIsIgnored`, the generator-contract table
extended with the current Sidebar / per-service / receipt payloads and the
legacy one relabelled (review LOW #6). Terraform side:
`TestLambdaTFManualSourcesAreEnvironmentScoped` and
`TestLambdaTFManualRulesAreScopedTwoWays`.

## H1 — permanent ECS errors were classified retryable

Two changes.

`awsecs.ErrPermanent` is a new sentinel wrapping all six errors the package
raises itself (missing service name, missing family, missing image URI, empty
describe response, zero containers matched, empty register response). They were
bare `fmt.Errorf`, indistinguishable to `deploy.Retryable` from a transient AWS
fault. `Retryable` matches it with `errors.Is` alongside `ErrUnknownTarget` and
`ErrInvalidRequest`.

The unclassified default at `retry.go` is **inverted to `false`**, deliberately.
Everything that can plausibly succeed on a second attempt is classified
explicitly above it — `net.Error`, smithy server faults, the throttling family,
`types.ServerException` — and `aws-sdk-go-v2` has already retried transient
transport failures inside the call before we see the error. What reached the
default in practice was this module's own errors. Retrying them is not cheap: a
`DEPLOYMENT_INITIATING` + `DEPLOYMENT_FAILED` Slack pair per attempt, *and* a
retryable verdict propagates out of the handler as an invocation error, so
EventBridge redelivers and repeats the whole thing — six posts for one push on a
condition that never clears. The cost of being wrong the new way is one missed
deployment, logged at ERROR and reported as `ignored`. The reasoning is written
out at the return statement.

Tests: `TestPermanentECSErrorsAreNotRetryable` (one register call, not four; no
sleeps; exactly one Slack pair), `TestPermanentECSErrorFromTheRealClientIsNotRetryable`
(drives the real `awsecs.Client`, not a hand-written error),
`awsecs.TestPermanentErrorsAreTyped` covering all six sites, and
`TestRegisterRevisionRefusesToRegisterAnIdenticalCopy` now asserts the error
*class*, which is what it was missing. `TestRetryableClassification` gains
throttling, modelled-client-error, network and permanent rows.

## H3 — the boundary test did not cover the names

New file `internal/boundary/lambdatf_names_test.go`. Everything it compares is
*derived* from the real artifacts, never restated:

1. **Env var names.** `configGetenvNames` walks the AST of `internal/config/config.go`
   for `getenv("X")` literals and, separately, for `decodeMap(getenv("X"), ...)`
   — which yields exactly the fail-open JSON maps with nothing hardcoded.
   `lambdaTFEnvNames` parses the `environment { variables { ... } }` block.
   `TestEnvironmentVariableNamesAgree` requires set equality both ways, with
   `AWS_REGION` allow-listed as runtime-provided (and asserted *absent* from
   `lambda.tf`, since Lambda reserves it). `TestGoldenFixtureCoversEveryFailOpenMap`
   requires the golden capture to carry every map variable and to invent none.
2. **Map field names.** `jsonTagsOf` reflects the tags off `config.Target` and
   `config.S3File` (and fails if a field has no tag). Target attributes must be
   written in `lambda.tf`; S3 attributes must be *read* there as
   `each.value.<tag>`; and the shipped maps in the golden capture must contain no
   field the Go struct does not read — `encoding/json` drops an unknown field
   without a word, which is how `bucket` or `type` would vanish silently.
3. **`module.ci_identifiers` inputs.** `TestIdentifierModuleInputsAgree` requires
   the argument set of the real call, the fixture's call, and
   `tf_identifiers/variables.tf` to be the same three-way. The fixture can no
   longer stop mirroring the call it is supposed to prove things about.
4. **Event patterns.** `handler.PatternContracts()` (new `eventpattern.go`)
   describes each rule from the constants the router uses and the struct tags of
   the types that parse those events; the literals in `handler.go`, `ecr.go` and
   `ssm.go` now reference those constants. `TestEventPatternsMatchWhatTheHandlerParses`
   extracts each rule's pattern from `lambda.tf` with a brace-balanced scanner —
   including **both** ECR candidates, the explicit list and the >2,048-character
   prefix fallback — and asserts source, detail field names and required values.
   `TestManualDetailTypesAgree` pins the manual detail-types on both manual rules.
5. **Init failure.** `main.go` no longer calls `os.Exit` anywhere. `startInert`
   serves every event as `ignored` and logs `event=configuration_invalid` at
   ERROR on every invocation. An init failure fails *every* async invocation and
   EventBridge redelivers each twice more, so `os.Exit(1)` turned one bad
   configuration into a permanent storm across every rule the project owns —
   exactly what the `SelfCheck` comment already refuses for a bad map entry. The
   two policies now agree. `main_test.go` covers the behaviour and guards the
   source against a re-added `os.Exit`.

Verified by breaking things on purpose and reverting:

```
$ sed -i '' 's/ECR_REPO_MAP    =/ECR_REPOSITORY_MAP =/' lambda.tf
$ go test ./internal/boundary/
--- FAIL: TestEnvironmentVariableNamesAgree
    config.Load reads ECR_REPO_MAP but lambda.tf does not emit it: the Lambda will
    read "" and either fail closed at startup or, for a map, silently resolve
    nothing forever
--- FAIL: TestGoldenFixtureCoversEveryFailOpenMap
    the golden capture carries ECR_REPO_MAP, which lambda.tf does not emit: the
    fixture has stopped mirroring the real file
FAIL

$ git checkout lambda.tf && go test ./internal/boundary/
--- PASS: TestEnvironmentVariableNamesAgree
--- PASS: TestGoldenFixtureCoversEveryFailOpenMap
ok
```

and, for the pattern half:

```
$ sed -i '' 's/operation = ["Update"]/operation = ["Set"]/;
             s/"bucketName" : \[each.value.bucket\]/"bucket_name" : [each.value.bucket]/' lambda.tf
--- FAIL: TestEventPatternsMatchWhatTheHandlerParses/ci_ssm_change
    rule "ci_ssm_change" (pattern 0) must select operation = "Update"; ...
--- FAIL: TestEventPatternsMatchWhatTheHandlerParses/s3_env_file_change_rule
    rule "s3_env_file_change_rule" (pattern 0) does not filter on detail field
    "bucketName", which is the JSON tag the handler parses
```

Both reverted; suite green.

## H2 — nothing ran the tests

`.github/workflows/ci.yml` added. Runs on every push to `main` and every pull
request, **with no `paths:` filter** — a boundary test whose purpose is to catch
a change on one side of a boundary must not be skipped because the change landed
on the other side.

- `go` job, matrix over `app` and `modules/workloads/ci_lambda`: `go build ./...`,
  `go vet ./...`, `gofmt -l` (exit code derived from the output, since `gofmt -l`
  exits 0), `go test -race ./... -count=1`.
- `terraform` job: `terraform fmt -check -recursive -diff modules/workloads`,
  then `init -backend=false` + `validate` in `modules/workloads`.
- `boundary-golden` job: the `-tags tfgolden` drift check, on a runner with
  `hashicorp/setup-terraform`.

`task ci` mirrors the workflow locally.

**`terraform fmt` scope — the choice the review asked for.** Scoped to
`modules/workloads`, recursive, **with no exemptions inside it**:
`modules/workloads/services.tf` was reformatted rather than excluded (one
indentation character, zero semantic change). Not widened to the repository,
because 18 files across the other modules are unformatted today and
`project/env/dev/main.tf` is a committed generated artifact that `terraform fmt`
cannot even parse ("Invalid expression"). Widening is a real change with a real
diff, not something to smuggle in here; excluding a file inside the scope would
have reproduced the opt-in property the review is objecting to.

Two pre-existing `gofmt` violations in `app/` (`apply_dryrun_test.go`,
`apply_progress_test.go`, whitespace only) were fixed for the same reason: the
gate has no carve-outs.

## Not fixed (unchanged from the review)

M1–M6 and LOW 1–5, 7, 8 are untouched and remain open, except LOW #6 (the
mislabelled Sidebar test case), which was in the path of H4 and is done.

## Status of the defect table

`D11` moves from **Partial** to **Fixed**: the workflow file exists and runs
both modules plus Terraform. `D14`'s residual hole (the unclassified retry
default) is closed. `D5` is now fixed for all three emitters, not just the
web-generated ones.

## Verification

```
modules/workloads/ci_lambda$ go build ./...            (clean)
modules/workloads/ci_lambda$ go vet ./...              (clean)
modules/workloads/ci_lambda$ gofmt -l .                (empty)
modules/workloads/ci_lambda$ go test -race ./... -count=1
ok  madappgang.com/infrastructure/ci_lambda                    1.222s
ok  madappgang.com/infrastructure/ci_lambda/contract           1.469s
ok  madappgang.com/infrastructure/ci_lambda/internal/awsecs    1.348s
ok  madappgang.com/infrastructure/ci_lambda/internal/boundary  1.884s
ok  madappgang.com/infrastructure/ci_lambda/internal/config    2.163s
ok  madappgang.com/infrastructure/ci_lambda/internal/deploy    1.622s
ok  madappgang.com/infrastructure/ci_lambda/internal/handler   2.045s
ok  madappgang.com/infrastructure/ci_lambda/internal/slack     1.897s
modules/workloads/ci_lambda$ go test -tags tfgolden ./internal/boundary/ -count=1
ok  madappgang.com/infrastructure/ci_lambda/internal/boundary  0.652s

app$ go build ./...   (clean)
app$ go vet ./...     (clean)
app$ gofmt -l .       (empty)
app$ go test ./...
ok  madappgang.com/meroku          0.426s
ok  madappgang.com/meroku/pricing  0.150s

modules/workloads$ terraform fmt -check -recursive .   (exit 0, no files listed)
modules/workloads$ terraform init -backend=false && terraform validate
Success! The configuration is valid, but there were some validation warnings
(pre-existing: deprecated failure_threshold, deprecated data.aws_region.name)
```

---

# M3 — the `app/` half of the dead build path, removed

Closes finding M3 of `reviews/code-review-internal.md` and deferred items §5.2,
§5.3 and §5.4 of this log. Terraform builds and zips the CI Lambda itself;
`app/` was building a second, `linux/amd64` copy at the module root that nothing
read. Both artifacts were observed side by side in one real run:

```
ci_lambda/bootstrap             20 MB  amd64  11:32   built by meroku, deployed nowhere
ci_lambda/.build/dev/bootstrap  13 MB  arm64  11:32   built by terraform, actually deployed
```

## What was removed

| Where | What |
|---|---|
| `app/deploy.go` | `buildDeploymentLambda`, its call site in `runCommandToDeploy`, `setLambdaBuildEnv`, `restoreEnvVar`, `lambdaBuildError`, `lambdaBuildCommand`, `lambdaBuildTempName` |
| `app/terrafrom.go` | `lambdaBootstrapPlaceholder{,Marker}`, `findLambdaBootstrapPath`, `isLambdaBootstrapPlaceholder`, `writeLambdaBootstrapPlaceholder`, `removeLambdaBootstrapPlaceholder`, `verifyLambdaBootstrapForApply`, `lambdaBootstrapBuildHint`, and the three call sites in `terraformInitIfNeeded`, `runTerraformApply` and `runTerraformDestroy` |
| `app/terraform_destroy_progress_tui.go` | the placeholder write/remove pair at the top of the destroy command |
| `app/lambda_bootstrap_test.go` | deleted (see below) |
| `env/main.hbs` | the `lambda_path` line; a template comment records why it is absent |
| `Taskfile.yml` | `lambda:build` / `lambda:package` / `lambda:clean` now mirror what `lambda.tf` does — `GOARCH=arm64`, `-trimpath -ldflags="-s -w"`, output to `.build/<env>/`, zipped by `go run ./tools/mkzip`. Added `lambda:test:golden` |
| `project/Taskfile.yml` | the `buildlambda` target (same dead amd64 flow, shipped into user projects) replaced by a comment saying terraform does it |

`var.lambda_path` **stays declared** in `modules/workloads/variables.tf`, now with
a description and a comment saying why: every `env/*/main.tf` generated before
today still passes it, and passing an undeclared variable is a hard Terraform
error. Verified against a real pre-change file below.

## The four things that had to keep working

### 1. `terraform destroy` with no artifact present — verified, no placeholder needed

The placeholder existed so `data.archive_file` could resolve during a destroy
plan. That data source is gone (`lambdatf_guard_test.go`), and nothing else in
`lambda.tf` reads the artifact at plan time: `filename` is only touched by the
provider on Create/Update, `source_code_hash` comes off
`null_resource.build_ci_lambda.triggers.src`, and the only file function pointed
at the build directory is `fileexists()`, which returns `false` rather than
failing.

Verified for real, not by inspection. A scratch copy of a **deployed**
environment's generated terraform (67 live resources, state serial 8, backend
overridden to local so nothing could touch the real S3 state), with **no**
`.build/` directory and no module-root `bootstrap` anywhere:

```
$ ls modules/workloads/ci_lambda/
contract  go.mod  go.sum  internal  main_test.go  main.go  README.md  tf_identifiers  tools

$ terraform plan -destroy -refresh=false     # the flags meroku itself uses
  # module.workloads.aws_lambda_function.lambda_deploy will be destroyed
  # module.workloads.aws_cloudwatch_event_target.lambda will be destroyed
  # module.workloads.aws_lambda_permission.ecr_event_call_deploy_lambda will be destroyed
  ... (10 lambda-related objects in all)
Plan: 0 to add, 0 to change, 67 to destroy.

$ terraform plan -destroy                    # harsher: every data source is read
module.workloads.data.aws_iam_policy_document.lambda_deploy_assume_role: Reading...
module.workloads.data.aws_ssm_parameters_by_path.backend: Reading...
... (17 data sources read)
Plan: 0 to add, 0 to change, 67 to destroy.
```

That state was written by the **pre-rewrite** config and still contains
`module.workloads.data.archive_file.lambda`, which is the worst case for this
check: the destroy plan drops it without ever looking for a file. No apply was
run at any point.

### 2. A missing Go toolchain still fails loudly, now in exactly one place

The provisioner's pre-flight is the only remaining check, so it now carries the
whole explanation. It gained the `brew install go` route that only the deleted
`app/` message had, and states that the build is retried on the next run:

```
$ env -i PATH=/usr/bin:/bin sh -c '<the pre-flight from local.ci_lambda_build_command>'
ERROR: building the CI/CD Lambda needs a Go toolchain (1.22+) on the machine running 'terraform apply'.
       'go' was not found in PATH. Install it with 'brew install go' on macOS, or from
       https://go.dev/dl/ , then re-run the deploy -- the build is retried automatically.
       Nothing is deployed without it: meroku will not substitute a placeholder artifact,
       because a placeholder deploys green and then fails every invocation with
       Runtime.InvalidEntrypoint.
exit code: 1
```

Cost, stated plainly: the text is part of `triggers.build_cmd`, so the next apply
in an existing environment replaces `null_resource.build_ci_lambda` once. The
function itself does not change — `source_code_hash` reads `triggers.src`, which
is unaffected.

### 3. `app/lambda_bootstrap_test.go` — deleted, with the surviving property moved

All eleven tests drove `buildDeploymentLambda`, `verifyLambdaBootstrapForApply`
or the placeholder pair. Nine are obsolete by construction (missing toolchain,
compiler failure, binary installed on success, `GOOS`/`GOARCH` restored, apply
rejects placeholder / missing / accepts real / ignores projects without the
module, placeholder is not a viable handler): the code they describe no longer
exists, and the behaviour they asserted — meroku building an amd64 binary and
gating the apply on it — is the defect, not a requirement.

Two described a property that **does** survive, so it was moved rather than
dropped:

- `TestDestroyWorksWithNoBinaryPresent` and
  `TestPlaceholderCleanupNeverDeletesARealBinary` both existed to keep destroy
  working without an artifact. That requirement is now enforced where it is
  actually decided, in `lambdatf_guard_test.go::TestLambdaTFNeverReadsTheArtifactAtPlanTime`:
  no `file*()` function may point at `ci_lambda_zip` / `ci_lambda_build_dir`,
  `source_code_hash` must come from the build resource, and the `fileexists`
  staging probe must stay.

The new guard was mutation-checked rather than assumed — it fires on
`filebase64sha256(local.ci_lambda_zip)`, `filemd5(local.ci_lambda_zip)` and
`file("${local.ci_lambda_build_dir}/bootstrap")`, and stays quiet on the two
legitimate uses (`fileexists(local.ci_lambda_zip)` and `filesha1` over the
sources).

Net: `app/` loses tests for code that no longer exists; the Terraform side gains
a test for the invariant those tests were really protecting.

### 4. The stale `modules/workloads/ci_lambda/bootstrap`

Already ignored twice over (`.gitignore:32` at the repo root and
`ci_lambda/.gitignore:4`) and never tracked by git. Removed locally, together
with `.build/`. `task lambda:clean` now removes both the new `.build/` tree and
any module-root `bootstrap` / `ci_lambda.zip` left over from the old flow.

## Verification

```
app$ go build ./...   (clean)
app$ go vet ./...     (clean)
app$ gofmt -l .       (empty)
app$ go test ./... -count=1
ok  madappgang.com/meroku          0.738s
ok  madappgang.com/meroku/pricing  0.755s

modules/workloads/ci_lambda$ go build ./...   (clean)
modules/workloads/ci_lambda$ gofmt -l .       (empty)
modules/workloads/ci_lambda$ go test -race ./... -count=1
ok  madappgang.com/infrastructure/ci_lambda                    1.507s
ok  madappgang.com/infrastructure/ci_lambda/contract           1.674s
ok  madappgang.com/infrastructure/ci_lambda/internal/awsecs    1.215s
ok  madappgang.com/infrastructure/ci_lambda/internal/boundary  3.118s
ok  madappgang.com/infrastructure/ci_lambda/internal/config    2.617s
ok  madappgang.com/infrastructure/ci_lambda/internal/deploy    2.378s
ok  madappgang.com/infrastructure/ci_lambda/internal/handler   2.129s
ok  madappgang.com/infrastructure/ci_lambda/internal/slack     1.905s
modules/workloads/ci_lambda$ go test -tags tfgolden ./internal/boundary/ -count=1
ok  madappgang.com/infrastructure/ci_lambda/internal/boundary  0.568s

modules/workloads$ terraform fmt -check -recursive .   (exit 0, no files listed)
modules/workloads$ terraform validate
Success! The configuration is valid, but there were some validation warnings
(pre-existing: deprecated failure_threshold, deprecated data.aws_region.name)
```

### One binary, and it is arm64

```
$ find modules/workloads/ci_lambda \( -name bootstrap -o -name '*.zip' \) | wc -l
0
$ task lambda:package env=dev
task: [lambda:build] mkdir -p .build/dev
task: [lambda:build] GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o .build/dev/bootstrap .
task: [lambda:package] go run ./tools/mkzip -in .build/dev/bootstrap -out .build/dev/ci_lambda.zip
$ find modules/workloads/ci_lambda \( -name bootstrap -o -name '*.zip' \)
modules/workloads/ci_lambda/.build/dev/bootstrap
modules/workloads/ci_lambda/.build/dev/ci_lambda.zip
$ file modules/workloads/ci_lambda/.build/dev/bootstrap
ELF 64-bit LSB executable, ARM aarch64, version 1 (SYSV), statically linked, ... stripped
```

### End to end: regenerate and plan

`meroku generate dev` then `terraform plan` (local-backend scratch copy, real
credentials, **no apply**), run with `.build/` deleted so nothing was staged:

```
$ ls modules/workloads/ci_lambda/            # no bootstrap, no .build/
contract  go.mod  go.sum  internal  main_test.go  main.go  README.md  tf_identifiers  tools

$ grep -n lambda_path env/dev/main.tf
(none)

  # module.workloads.aws_lambda_function.lambda_deploy will be created
  + architectures    = ["arm64"]
  + filename         = ".../modules/workloads/ci_lambda/.build/dev/ci_lambda.zip"
  + function_name    = "<project>_ci_lambda_dev"
  + handler          = "bootstrap"
  + source_code_hash = "aa6e6dc4da3f388385a06a5a50befbcd89c6f846"

  # module.workloads.null_resource.build_ci_lambda will be created
  + triggers = {
      + "build_cmd" = "267a889a83d63aa219fb6e9bfbe4a717"
      + "goarch"    = "arm64"
      + "goos"      = "linux"
      + "src"       = "aa6e6dc4da3f388385a06a5a50befbcd89c6f846"
      + "staged"    = "absent"
    }

Plan: 80 to add, 0 to change, 0 to destroy.
```

**Caveat, stated because it changes what this proves.** The intended target
project was torn down (`./reset --greenfield`) by a concurrent session partway
through this work — config, generated terraform and state bucket all removed —
so there is no deployed state left to diff against and "clean plan, no diff on
the Lambda" could not be produced for it without an apply, which was out of
bounds. Its config was restored from its own backup for the run above and the
directory was returned to the post-reset state afterwards. What the run does
prove: generation drops `lambda_path` and nothing else, the module plans, and the
function is planned onto the arm64 zip that terraform builds, from a checkout
with no artifact.

The "no unintended template change" half was proved separately, by regenerating
a `main.tf` that had been generated before this change and diffing:

```
@@ -175,7 +175,6 @@
   subnet_ids = local.subnet_ids
-  lambda_path = "../../infrastructure/modules/workloads/ci_lambda/bootstrap"
   slack_deployment_webhook = ""
```

(The only other hunk is `backend_auto_deploy`, from this session's auto-deploy
work, not from this change.)

And the backward-compatibility half was proved against a **real deployed
environment** whose `env/dev/main.tf` still passes the variable:

```
$ grep -n lambda_path main.tf
178:  lambda_path = "../../infrastructure/modules/workloads/ci_lambda/bootstrap"
$ terraform validate
Success! The configuration is valid.
$ terraform plan          # real state, no artifact on disk
  # module.workloads.aws_lambda_function.lambda_deploy will be updated in-place
  # module.workloads.null_resource.build_ci_lambda will be created
Plan: 16 to add, 2 to change, 3 to destroy.
```

Those 21 changes are this session's CI-Lambda rewrite reaching an environment
that predates it, not this removal: the state still holds `data.archive_file.lambda`.
Nothing was applied.

## Residual

- `modules/appsync/auth_lambda.tf` still uses `data "archive_file"` and
  `filemd5()` over its source tree — a genuine plan-time read, for a different
  module, with its own staging probe. Out of scope here; the deleted placeholder
  never covered it (it only ever wrote `ci_lambda/bootstrap`).
- `var.lambda_path` can be deleted once every `env/*/main.tf` in the wild has
  been regenerated. The comment above the variable says so.
