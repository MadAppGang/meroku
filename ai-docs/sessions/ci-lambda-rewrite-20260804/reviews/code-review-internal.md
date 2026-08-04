# Code review — CI Lambda rewrite

Scope: `modules/workloads/ci_lambda/**`, `modules/workloads/lambda.tf`, and the
`app/` + Terraform touchpoints that feed them.

Everything below was reproduced against the tree, not recalled.

Commands run:

```
$ go version
go version go1.26.5 darwin/arm64

$ cd modules/workloads/ci_lambda && go vet ./...        # clean, no output
$ go test -race ./...
?     madappgang.com/infrastructure/ci_lambda            [no test files]
ok    madappgang.com/infrastructure/ci_lambda/contract            1.410s
ok    madappgang.com/infrastructure/ci_lambda/internal/awsecs     1.687s
ok    madappgang.com/infrastructure/ci_lambda/internal/boundary   2.435s
ok    madappgang.com/infrastructure/ci_lambda/internal/config     2.042s
ok    madappgang.com/infrastructure/ci_lambda/internal/deploy     2.708s
ok    madappgang.com/infrastructure/ci_lambda/internal/handler    3.077s
ok    madappgang.com/infrastructure/ci_lambda/internal/slack      3.461s
?     madappgang.com/infrastructure/ci_lambda/internal/testsupport [no test files]
?     madappgang.com/infrastructure/ci_lambda/tools/mkzip          [no test files]

$ gofmt -l .                                            # nothing
$ terraform fmt -check -recursive lambda.tf ci_lambda/tf_identifiers   # exit 0
$ terraform init -backend=false && terraform validate    # (modules/workloads)
Success! The configuration is valid, but there were some validation warnings
valid: True errors: 0
```

The race detector reports nothing. There is no shared mutable state to race on:
`Handler`, `Deployer`, `awsecs.Client` and `slack.Client` are all built once in
`main.go:55-58` and hold only immutable configuration plus an `*http.Client` and
an SDK client, both of which are safe for concurrent use. `contract.Load`
(`contract/contract.go:45-60`) is `sync.Once`-guarded. `defaultJitter`
(`retry.go:59`) uses the global `math/rand` source, which is internally locked.
Nothing is memoised at cold start that should not be.

---

## Verdict: CONDITIONAL

D1, D2 (event paths), D3, D4, D6, D7, D8, D9, D10, D13, D15, D16, D17 are fixed
and I could not break them. D5 is fixed for the web-generated workflows only,
D11 is not fixed, D12 is fixed on the Terraform side and unfixed on the `app/`
side, and D14 has a residual hole in the retry classification.

| ID | Status | Evidence |
|---|---|---|
| D1 backend `""` vs `"backend"` | fixed | `contract/contract.json:2`, `lambda.tf:655`, `boundary_test.go:39` |
| D2 cross-project ECR | fixed | `lambda.tf:376-396`, `config.go:210-214`, `boundary_test.go:92-103` |
| D2 cross-project **manual** | **not fixed** | finding H4 |
| D3 lexicographic revision | fixed at root | `awsecs/client.go:22-25,136-139`; `ecs:ListTaskDefinitions` gone (`lambda.tf:270`) |
| D4 ctx discarded | fixed | ctx reaches every SDK call and every HTTP call; verified below |
| D5 manual contract | half | web generators now emit `project`; `receipts/`, `docs/` not updated — finding H4 |
| D6 dropped TD fields | fixed | all 18 `RegisterTaskDefinitionInput` fields copied (`client.go:176-197`), checked against the SDK struct |
| D7 backend S3 no-op | fixed | `lambda.tf:672-674` |
| D8 per-service S3 rules + bucket rewriting | fixed | `lambda.tf:569,599,607,623-626`; buckets verbatim (`variables.tf:211-217`) |
| D9 `use_existing` / `manual_repo` | fixed | fan-out map + URI stripping, verified in `terraform apply` below |
| D10 dead templates | fixed | embedded + `TestEveryTemplateRendersValidJSON` |
| D11 nothing runs the tests | **not fixed** | finding H2 |
| D12 dummy bootstrap | half | Terraform side fixed; `app/` side still live — finding M3 |
| D13 config errors burn retries | fixed | `deployer.go:92-98` resolves outside the loop |
| D14 unknown events retried | mostly | residual hole — finding H1 |
| D15 tests test a copy | fixed | `deployer_test.go` drives the real `Deployer` |
| D16 4-segment task SSM path | fixed | `config.go:222-239`, `boundary_test.go:125-147` |
| D17 hyphenated names | fixed | no regex left |

**Context and timeouts (item 5 of the brief) are correct.** `Handle` threads
`ctx` into `ecr/ssm/s3/manual/ecsState`, into `Deployer.Deploy` →
`attempt` → `call` → `awsecs.Client.{UpdateService,DescribeTaskDefinition,
RegisterTaskDefinition}` and into `slack.Client.Notify`, which derives a 5s
child (`slack/client.go:143`) so the Lambda deadline always wins. `sleepCtx`
(`retry.go:45-57`) selects on `ctx.Done()`, and `fitsDeadline`
(`deployer.go:231-238`) refuses a sleep that would not leave 2s for the call.
`context.Canceled`/`DeadlineExceeded` are classified non-retryable
(`retry.go:87-89`), so a timeout does not become an EventBridge retry. No
`time.Sleep` remains anywhere in the module.

**Regressions vs the old code: none found on the paths the old code actually
served.** ECR→service, ECR→scheduled task, SSM→service, manual `DEPLOY`
(production), and ECS-state→Slack all still work and are each covered by a test
driven off the real Terraform capture. Slack volume is unchanged (the old
`DeployerV2` also sent `DEPLOYMENT_INITIATING` plus a success/failure post —
`git show HEAD:.../deployer/deployer.go:74-140`).

---

## HIGH

### H1 — permanent ECS errors are classified retryable, producing the retry storm + duplicate Slack posts the rewrite exists to remove

`internal/deploy/retry.go:140-141` (`// Unclassified: one more attempt is cheap
and bounded. return true`) combined with the bare `fmt.Errorf` sentinels in
`internal/awsecs/client.go:66-68,121-125,143-145,170-174,203-205`.

Reproduced:

```
$ go test ./internal/deploy/ -run TestScratchNoContainerIsRetryable -v
    Retryable(no-container-match)   = true
    Retryable(no-definition)        = true
    Retryable(service-name-required)= true
```

Concrete failure. A scheduled task whose container image does not come from the
repository that was pushed — a task with `docker_image` set explicitly in YAML
(`modules/ecs_task/main.tf:60`), a task whose ECR repo was recreated in another
registry, or a multi-container task where the app container was renamed — hits
`client.go:170-174`, "no container in family %q uses repository %q". That is a
permanent configuration fact. It is classified retryable, so:

1. `attempt` (`deployer.go:153-183`) burns all 3 attempts with 1s + 2s backoff;
2. the wrapped error at `deployer.go:185` still satisfies `Retryable` (`%w`
   preserves the unclassified leaf), so `deployMany` returns it as an
   invocation error (`handler.go:130-132`);
3. EventBridge/Lambda async invoke retries the event twice more;
4. each of the 3 invocations posts `DEPLOYMENT_INITIATING` **and**
   `DEPLOYMENT_FAILED` to Slack (`deployer.go:105-111,122-128`) → 6 posts for
   one push, on a condition that will never clear.

This is the same shape as D13/D14 and contradicts FR-12/FR-13 ("configuration
errors fail on the first attempt"). `TestRegisterRevisionRefusesToRegisterAnIdenticalCopy`
(`awsecs/client_test.go:162`) asserts *that* it errors but never asserts the
error class, and `TestRetryableClassification` (`deployer_test.go:283`) pins
`"unclassified" → true` as intended behaviour.

Fix. Give `awsecs` its own `ErrPermanent` sentinel (or reuse
`deploy.ErrInvalidRequest`) and wrap the five non-AWS errors in `client.go` with
it, then add it to the non-retryable list at `retry.go:84-86`. Add a test that
asserts `deploy.Retryable(err) == false` for the zero-container case. Consider
inverting the default at `retry.go:141` to `false` — with `net.Error`,
`smithy.APIError` fault/throttle and `types.ServerException` already handled
above it, "unclassified" now only reaches errors this module produced itself.

### H2 — D11 is not fixed: nothing runs `go test`, `go vet` or the boundary test

```
$ ls .github/workflows/
release.yml
$ grep -rn "ci_lambda\|go test\|go vet" .github/workflows/     # no output
```

NFR-8 requires `go vet` + `go test ./...` + the boundary test on every PR
touching `modules/workloads/**`. Acceptance criterion 8 ("`go test ./...` fails
if `lambda.tf` hardcodes an identifier literal") is only true if someone runs
`go test`. `Taskfile.yml:123` has a `lambda:test` target, which nothing invokes
automatically, and `Taskfile.yml:31` `task test` runs `go test ./...` from the
repo root — a different Go module, so it does not reach `ci_lambda` at all
(verified: `ci_lambda/go.mod` declares its own module).

Concrete failure. The entire D1 firewall — `boundary_test.go`,
`lambdatf_guard_test.go`, `contract_test.go` — is opt-in. D1 survived for months
precisely because "all unit tests pass" was never checked by anything. The
rewrite reproduces that property: a PR that renames `ECR_REPO_MAP` in
`lambda.tf` merges green.

The implementation log (§5.1) states the workflow is out of scope and gives the
YAML for it. It needs to land, not be described.

Fix. Add `.github/workflows/ci-lambda.yml` with `paths: ['modules/workloads/**']`
running, in `modules/workloads/ci_lambda`: `go build ./... && go vet ./... &&
go test -race ./... -count=1`, `test -z "$(gofmt -l .)"`, and in
`modules/workloads`: `terraform fmt -check -recursive lambda.tf ci_lambda/ &&
terraform init -backend=false && terraform validate`. Add the
`-tags tfgolden` drift check on a runner that has `hashicorp/setup-terraform`.

### H3 — the boundary test does not cover the env-var names, so a D1-class divergence can still be 100% silent

`internal/boundary/testdata/tfgolden/main.tf:92-101` builds the golden `env`
map by hand. It is a *mirror* of `lambda.tf:222-255`, not that file.
`lambdatf_guard_test.go` checks only that `module.ci_identifiers.*` is
referenced (`:45-54`), that two literals are absent (`:59-74`), and that four
dead settings are gone (`:129-136`). Nothing anywhere compares the set of
variable **names** `lambda.tf` emits against the set `config.Load` reads.

Concrete failure. Rename `ECR_REPO_MAP` at `lambda.tf:239` to anything (a typo,
a "consistency" rename to `ECR_REPOSITORY_MAP`):

- `config.go:136` reads `""` → `decodeMap` substitutes `"{}"` (`config.go:317-319`)
  → `cfg.ECRRepos` is an empty map;
- `Validate()` (`config.go:155-192`) does not look at `ECRRepos`, so init succeeds;
- `SelfCheck()` (`config.go:272-279`) iterates an empty map, so it reports nothing;
- every ECR push then resolves to zero identifiers (`config.go:210-214`) and is
  logged as `ECR repository is not mapped to any target in this project`
  (`handler/ecr.go:44`) with `status: ignored`, `error: nil`.

Result: auto-deploy is 100% dead, every log line says "working as designed", no
alarm fires, and every test in the module still passes. That is D1 exactly, one
layer up. The same is true for `SSM_SERVICE_MAP`, `S3_SERVICE_MAP` and
`SCHEDULED_TASK_MAP`. Only `ECS_SERVICE_MAP`, `PROJECT_NAME`, `PROJECT_ENV` and
`ECS_CLUSTER_NAME` fail closed, via `Validate()`.

Four further gaps in the same mechanism, all verified by reading the fixture:

1. **Map shape.** The JSON field names `service_name`, `task_family`, `type`,
   `bucket`, `key` are produced at `lambda.tf:653-689` and consumed by the tags
   in `config.go:32-46`. The fixture re-types them by hand
   (`tfgolden/main.tf:52-80`). `service_name`/`task_family` fail closed via
   `Validate`; `bucket`, `key` and `type` do not — renaming `bucket` makes
   every S3 trigger silently resolve to nothing.
2. **Module inputs.** `local.ci_backend_repo`, `ci_service_repos`,
   `ci_task_repos`, `ci_task_ssm_prefixes` (`lambda.tf:78-119`) are what feed
   `module.ci_identifiers`. The fixture hands the module hand-written correct
   values instead. Every one of those locals is `try(..., "")`-guarded and
   `tf_identifiers/main.tf:26-36,46-56` drops empty entries, so a wrong resource
   key silently removes a repository from both `ECR_REPO_MAP` *and* the ECR
   rule's allow-list, and `SelfCheck` stays clean because fewer entries is still
   self-consistent.
3. **Event patterns.** No test evaluates any `event_pattern`. The ECR
   `action-type`/`result` values (`lambda.tf:380-381` vs `handler/ecr.go:37`),
   the SSM `operation = ["Update"]` (`lambda.tf:493` vs `handler/ssm.go:36`),
   the S3 `eventName` list (`lambda.tf:581`) and the ECS `resources` prefix
   (`lambda.tf:457`) are asserted only against hand-written events in
   `handler_test.go`. A pattern change that stops matching what the handler
   parses is invisible on both sides.
4. **Init failure is a retry storm.** `main.go:28-34` `os.Exit(1)` on a
   `Validate` failure. EventBridge invokes asynchronously, so an init failure
   queues and retries *every* matching event in the account — the exact
   behaviour the `SelfCheck` comment at `main.go:38-41` deliberately avoids.
   The two policies should agree.

Fix. Extend `lambdatf_guard_test.go` with a test that extracts the keys of the
`environment { variables { ... } }` block from `lambda.tf` and requires that
every key the golden `env` map contains is present there, and vice versa for the
map-valued ones. That is ~20 lines of regexp and closes the silent half. For (2),
have the golden fixture take its `service_repos`/`task_repos` from the same
expressions `lambda.tf` uses, or add `precondition` blocks on
`module.ci_identifiers` asserting no input value is `""` for a service that
exists.

### H4 — manual deploys are still not project- or environment-scoped for the emitters that ship in this repo (FR-7)

`lambda.tf:403-408` puts `"action.production"` in **every** environment's
source list, and `lambda.tf:529-535` deliberately carries no `detail` filter.
Scoping is delegated to `handler/manual.go:38-45`, which only applies when the
payload carries the fields:

```go
if d.Project != "" && d.Project != h.cfg.Project { ... }
if d.Env != "" && d.Env != h.cfg.Env { ... }
```

The web generators were updated to send `project`
(`web/src/components/Sidebar.tsx:769,878`,
`web/src/components/ServiceCICDConfiguration.tsx:127`), but the two emitters
that ship in-tree were not:

- `receipts/github/prod-deploy.yml:28` —
  `Source=action.production,DetailType=DEPLOY,Detail="{\"service\":\"backend\"}"`
- `docs/ECR_STRATEGY.md:317,339` — same, plus a `tag` field the handler ignores.

Concrete failures, both from a single production deploy in project `acme`:

1. **Cross-environment.** `acme`'s dev and staging rules both list
   `action.production` (`lambda.tf:405`), so their Lambdas are invoked. `d.Env`
   is `""`, the check is skipped, and dev and staging redeploy their own
   backends with whatever `:latest` is in their registries. A production release
   silently restarts dev.
2. **Cross-project.** Every other meroku project in the account also matches
   (`action.production`, `DEPLOY`, no detail filter). `d.Project` is `""`, so
   each one redeploys its own backend.

Neither is a regression — the old single rule
(`git show HEAD:modules/workloads/lambda.tf:200-218`) had the same sources and
the old handler had no project/env check at all — but FR-7 explicitly required
this to be fixed, and the fix was applied to one of the three emitters.

Fix. Update `receipts/github/prod-deploy.yml` and `docs/ECR_STRATEGY.md` to send
`{"service":"backend","project":"<project>","env":"<env>"}` in the same change,
and drop the unconditional `"action.production"` from `ci_manual_sources` for
non-production environments (`"action.${var.env}"` already covers production).
Add a `handler_test.go` case that a `{"service":"backend"}` payload with no
`project` on an `action.production` source is *ignored* once the emitters are
fixed, so the loose path cannot come back.

---

## MEDIUM

### M1 — `aws_lambda_permission` statement IDs for S3 rules are unbounded; AWS caps them at 100 characters

`lambda.tf:609`:

```hcl
statement_id = "AllowExecutionFromEventBridge_${replace(each.key, "/[^a-zA-Z0-9_-]/", "_")}"
```

`each.key` is `"${bucket}-${key}"`. `AddPermission`'s `StatementId` has a
maximum length of 100. The prefix is 30 characters, leaving 70 for
bucket + `-` + key. A bucket like `acme-production-config-eu-central-1` (35) and
a key like `envs/production/services/payment-worker/.env` (44) gives 110 →
`terraform apply` fails with `ValidationException` and no obvious cause.

This string is carried over unchanged from the old file, but `for_each` now runs
over `local.all_env_files_s3` (`lambda.tf:607`) instead of the backend-only list,
so per-service keys — which are the deeper ones — are newly exposed. The rule
*name* two resources above already solves this with
`local.s3_event_rule_names[each.key]` (`lambda.tf:574`, ≤21 chars, md5-unique).

Fix. `statement_id = "AllowEventBridgeS3_${local.s3_event_rule_names[each.key]}"`.
Note this renames existing permissions, i.e. one destroy/create per env file.

### M2 — scheduled tasks outside `dev` have no reachable deploy trigger, and nothing says so

- ECR: `lambda.tf:109-111` correctly emits `ci_task_repos = {}` when
  `var.env != "dev"`, because `modules/ecs_task/main.tf:43-46` only creates the
  repository in dev. No local ECR event exists.
- SSM: `handler/ssm.go:49-52` deliberately ignores scheduled tasks.
- S3: `lambda.tf:671-680` builds `S3_SERVICE_MAP` from the backend and services
  only; there is no task entry.
- Manual: `deployer.go:189-193` rejects a scheduled-task request without
  `ImageURI` as `ErrInvalidRequest` → non-retryable → `ignored`. No generator
  emits `image_uri` (`Sidebar.tsx:769`, `ServiceCICDConfiguration.tsx:127`).

`SCHEDULED_TASK_MAP` (`lambda.tf:684-689`) is still populated unconditionally, so
staging and prod ship a map of targets that no event can ever reach. The README's
event table (`ci_lambda/README.md:46-52`) does not mention it.

Fix. Either document it in the README and in `lambda.tf` next to
`ci_task_repos`, or make `SCHEDULED_TASK_MAP` conditional so `SelfCheck` and the
maps agree with what is reachable, and extend the manual-deploy generator to
send `image_uri` for `task:` identifiers.

### M3 — `app/` still builds, verifies and placeholder-writes an artifact Terraform no longer uses (D12, app half)

- `app/deploy.go:494-531` `buildDeploymentLambda` compiles
  `modules/workloads/ci_lambda/bootstrap` with `GOOS=linux GOARCH=amd64`
  (`app/deploy.go:538-545`).
- `lambda.tf:48-50` builds `arm64` into `.build/${var.env}/bootstrap` and zips it
  there. The file at the module root is never read by Terraform — confirmed: the
  build hash glob is `**/*.{go,json,mod,sum,tmpl}` (`lambda.tf:27`), and I
  enumerated it with `terraform apply` (35 files, no `bootstrap`).
- `app/terrafrom.go:191-199` `terraformInitIfNeeded` still calls
  `verifyLambdaBootstrapForApply()` and `os.Exit(1)`s when that dead artifact is
  missing.
- `app/terraform_destroy_progress_tui.go:153-161` still writes a placeholder "so
  `archive_file` can resolve during the destroy plan" — `data "archive_file"` no
  longer exists in `lambda.tf` (asserted by
  `lambdatf_guard_test.go:83-89`).

Concrete failures: every meroku deploy compiles the Lambda twice, once for an
architecture that is never deployed, so a project needs Go for two independent
reasons and a Go failure surfaces from the wrong place; and the apply gate at
`app/terrafrom.go:194` can stop a deploy over a file that has no bearing on what
ships. The implementation log §5.2 acknowledges this and defers it.

Fix. Delete `buildDeploymentLambda`, `setLambdaBuildEnv`,
`verifyLambdaBootstrapForApply`, `writeLambdaBootstrapPlaceholder`,
`removeLambdaBootstrapPlaceholder` and their call sites, and drop
`lambda_path` from `env/main.hbs:507` (keep the variable declared per the
requirements). Update `Taskfile.yml:118-137`, which still describes the old
amd64 single-artifact flow.

### M4 — `local.ci_service_repos` is a second, independent derivation of `local.service_ecr_urls` and diverges for `use_existing`

`lambda.tf:94-105` resolves the `use_existing` case as
`aws_ecr_repository.services[svc.ecr_config.source_service_name].name`.
`ecr.tf:161-171` resolves the same case as
`lookup(local.ecr_repository_map, "${source_service_type}-${source_service_name}", "")`,
where `ecr_repository_map` is keyed `"services-${name}"` (`ecr.tf:146-152`).

The two disagree whenever `source_service_type` is anything other than
`"services"`. Both are wrapped in `try(..., "")` / `lookup(..., "")`, and
`tf_identifiers/main.tf:26-30` drops empty entries, so the divergence is silent:
the service disappears from `ECR_REPO_MAP` and from the ECR rule's allow-list,
and every push to its repository is logged `ECR repository is not mapped to any
target in this project` (`handler/ecr.go:44`) forever.

FR-2 asks that a name be defined once. It is defined twice, and the boundary
test cannot see it (H3, gap 2).

Fix. Derive `ci_service_repos` from `local.service_ecr_urls` with the same
URI-stripping already written at `lambda.tf:87-92` (which I verified works —
`…/team/legacy-api:v1 → team/legacy-api`, `host/repo@sha256:abc → repo`), so
there is one resolution path. Alternatively add a `precondition` that every
service in `var.services` has a non-empty entry.

### M5 — the >2,048-character fallback ECR pattern loses `manual_repo` and namespaced repositories

`lambda.tf:396` swaps the explicit repository list for
`repository-name = [{ prefix = "${var.project}_" }]` (`lambda.tf:392`) once the
explicit pattern exceeds 2,048 characters.

Repositories reached through `ecr_config.mode = manual_repo` do not start with
`${var.project}_` — I verified the stripping produces names like
`team/legacy-api` and `repo`. A project that crosses the ~60-repository
threshold therefore silently stops receiving ECR events for exactly those
services, while everything else keeps working. The comment at
`lambda.tf:369-375` claims the map "stays the authority either way", which is
true for false positives but not for the false negatives this introduces.

Fix. Build the fallback as `distinct(concat([{prefix = "${var.project}_"}],
[for r in local.ci_ecr_repos : r if !startswith(r, "${var.project}_")]))` so
off-prefix repositories are always listed explicitly, or add a `precondition`
that fails the apply with an actionable message instead of degrading silently.

### M6 — `null_resource.build_ci_lambda` has no artifact-presence trigger

`lambda.tf:55-60` triggers on `src`, `goos`, `goarch`, `build_cmd`. The sibling
implementation this was modelled on has one more,
`modules/appsync/auth_lambda.tf:56-64`:

```hcl
# A fresh clone (CI) has no build directory even though the source hashes
# are unchanged.
staged = fileexists(local.auth_lambda_stage_probe) ? "present" : "absent"
```

Concrete failure. `aws_lambda_function.lambda_deploy` reads
`filename = local.ci_lambda_zip` (`lambda.tf:199`) whenever the provider needs
a code update — which includes **Create**. On a machine where
`.build/${var.env}/ci_lambda.zip` does not exist (every fresh clone; the
directory is gitignored) and the source hash is unchanged (so the provisioner
does not re-run), any path that recreates the function — state loss,
`terraform state rm`, the function deleted out of band, `-replace=` — fails with
`unable to load .../ci_lambda.zip: no such file or directory`.

The design is otherwise sound: `plan` and `destroy` never read `filename`, and
a source change always changes `triggers.src`, so the ordinary paths are safe.
The implementation log §5.8 judged this not worth two churning applies. Adding
the probe costs one extra trigger and removes the class entirely.

Fix. `staged = fileexists(local.ci_lambda_zip) ? "present" : "absent"`.

---

## LOW

Clustered; none of these changes behaviour on a working path.

1. `internal/deploy/deployer.go:185` reports `"failed after %d attempt(s)"` using
   `d.cfg.MaxRetries+1` even when the loop broke early at `deployer.go:156-160`
   (deadline) or `:162-165` (ctx cancelled). The log line and the Slack
   `DEPLOYMENT_FAILED` reason overstate what was tried. Count the iterations.
2. `internal/handler/handler.go:130-132`: when `DeployAll` returns a retryable
   joined error, the IDs that *did* deploy are dropped from the response and the
   EventBridge retry redeploys them. Harmless (force-new-deployment is
   idempotent) but the response is misleading; keep `Deployed` populated.
3. `lambda.tf:27` hashes `**/*.go`, which includes `*_test.go`. Editing a test
   file changes `source_code_hash` and triggers a Lambda code update with a
   byte-identical binary. Exclude `_test.go` from the fileset.
4. `env/main.hbs:507` still emits `lambda_path`, now unused
   (`modules/workloads/variables.tf:22` correctly keeps the variable declared).
5. `Taskfile.yml:118-137` — `lambda:build` (`GOARCH=amd64 ... -o bootstrap
   main.go`), `lambda:package` (`zip ci_lambda.zip bootstrap`) and
   `lambda:clean` all describe the pre-rewrite artifact layout.
6. `internal/handler/handler_test.go:325` pins the *legacy* Sidebar payload
   `{"service":"backend"}`. The generator now emits `project` as well
   (`Sidebar.tsx:769`); the case labelled "Sidebar backend workflow" no longer
   matches the Sidebar. Rename it to "legacy payload, no project" and add the
   current one.
7. `handler/ecsstate.go:63-70` puts the raw service ARN in the Slack `Service`
   field. Every ECS-state notification now reads
   `arn:aws:ecs:…:service/acme_cluster_dev/acme_service_dev` instead of a name.
   `basename` of the ARN would restore the old message.
8. Neither `aws_ecs_service.backend` (`backend.tf:10`) nor
   `aws_ecs_service.services` (`services.tf`) has
   `lifecycle { ignore_changes = [task_definition] }`, so the next
   `terraform apply` reverts whatever revision the Lambda deployed. Pre-existing
   and outside this change, but it caps the usefulness of the whole component.

---

## Things I checked and found correct (no action)

- Every `RegisterTaskDefinitionInput` field the SDK declares is carried forward
  (`awsecs/client.go:176-197` against
  `aws-sdk-go-v2/service/ecs@v1.58.1/api_op_RegisterTaskDefinition.go`): 18/18.
- `SplitImageRef` (`awsecs/image.go:15-28`) handles `@sha256:`, `host:port/repo`
  and bare refs; `TestSplitImageRef` covers all three.
- Longest-prefix SSM matching is segment-aware (`config.go:228`), so
  `/dev/acme/api` does not swallow `/dev/acme/api-v2/env`.
- The URI-stripping regexes at `lambda.tf:87-92` behave as intended — verified by
  running them through `terraform apply` on six inputs including a namespaced
  repo with a tag and a digest ref.
- Slack cannot fail a deployment or an invocation: `Notifier.Notify` returns
  nothing (`slack/client.go:50-52`), `New` degrades to `noop` on a missing URL or
  a template parse failure (`slack/client.go:69-89`), and every send path exits
  through a log line (`slack/client.go:134-169`). Acceptance criterion 5 holds.
- `ecs:ListTaskDefinitions` is gone and the guard test enforces it
  (`lambdatf_guard_test.go:135`); `iam:PassRole` is conditioned on
  `iam:PassedToService = ecs-tasks.amazonaws.com` (`lambda.tf:299-303`);
  `ecs:UpdateService` is scoped to this module's service ARNs
  (`lambda.tf:284-289`). FR-18 met.
- `SERVICE_CONFIG`, `DEPLOYMENT_TIMEOUT_SECONDS`, `local.service_config` are gone
  and guarded (FR-16).
- No `V2` suffix, no `aws-sdk-go` v1 in `go.mod`, `gofmt -l` clean
  (acceptance criterion 6, NFR-1, NFR-5).
- `terraform validate` on `modules/workloads` passes with the new module wired
  in, and `terraform fmt -check` is clean.
