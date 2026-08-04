# CI Lambda Rewrite — Requirements

Subject: `modules/workloads/ci_lambda/` (Go Lambda) + `modules/workloads/lambda.tf` (its wiring).
Date: 2026-08-04. Status: planning, no code written.

---

## 1. Scope

The Lambda is the deployment trigger for every meroku project: an EventBridge event
(ECR push, S3 env-file write, SSM parameter change, manual `DEPLOY`, ECS deployment state
change) must result in either an ECS `UpdateService` (long-running services) or a new
task-definition revision (scheduled/cron tasks), plus a Slack post. It is fire-and-forget:
no deployment waiter, `timeout = 60` in `lambda.tf` depends on that.

In scope: the Go module, `lambda.tf`, the identifier contract between them, the build
pipeline, and the CLI/Taskfile touchpoints that feed it.

---

## 2. Confirmed defects (given, verified against live AWS — not re-litigated)

| ID | Defect | Location |
|----|--------|----------|
| **D1** | Backend deploy path 100% broken. Terraform emits key `"backend"`; Go returns `""` for the backend repo, so `GetServiceMapping("")` misses. Every unit test passes because the Go tests build maps with the key the Go code expects. | `utils/service_name_extractor.go:18`, `handlers/handler.go:537`, `lambda.tf:334` |
| **D2** | Cross-project mis-deployment. The ECR rule filters only `source`/`detail-type`; the service regex `\w+_service_(...)` is unanchored and project-blind. Two projects in one account cross-fire. | `lambda.tf:200-218`, `utils/service_name_extractor.go:23` |
| **D3** | Wrong task-definition revision: re-sorts ARN strings lexicographically (`:9` > `:11`), `FamilyPrefix` prefix-matches sibling families, `MaxResults 10`. | `services/ecs.go:173-205` |
| **D4** | `context.Context` accepted then discarded; SDK v1 calls take no ctx. Lambda deadline never propagates. | `handlers/handler.go` (all), `deployer/deployer.go:56`, `services/ecs.go` |

---

## 3. Additional defects found while reading (all verified in-tree)

| ID | Defect | Evidence |
|----|--------|----------|
| **D5** | The manual-deploy contract the product emits does not match the rule that receives it. Generated GH workflows emit `Source=action.{env}` / `DetailType=DEPLOY`, and the per-service generator emits `Source=github.actions.{env}` / `DetailType=SERVICE_DEPLOY`. The rule accepts only sources `action.production`, `action.deploy` and detail-types `DEPLOY`. So *every* generated manual-deploy workflow is a no-op unless `env == "production"`. | `web/src/components/Sidebar.tsx:769,878`; `web/src/components/ServiceCICDConfiguration.tsx:123-127`; `lambda.tf:203-218`; `handlers/handler.go:141-146` |
| **D6** | `RegisterTaskDefinition` clone silently drops `PidMode`, `IpcMode`, `InferenceAccelerators`. Every scheduled-task deploy loses those settings permanently. | `services/ecs.go:270-284` |
| **D7** | Backend S3 env-file changes are a guaranteed no-op. Rules are created from `local.env_files_s3` (backend list), but `S3_SERVICE_MAP` is built only from `local.services_env_files_s3`. The Lambda is invoked, finds no service, logs, exits. | `lambda.tf:246` vs `lambda.tf:349-356` |
| **D8** | Per-service S3 env files get **no EventBridge rule at all** (rules loop over the backend list only), and the map rewrites the bucket to `"{project}-{bucket}-{env}"` while the task definitions use the bucket verbatim. The S3 trigger is broken in both directions. | `lambda.tf:246,352` vs `services.tf:156`, `backend.tf:130`, `variables.tf:209-217` |
| **D9** | `ecr_config.mode = use_existing` lets N services share one ECR repo, and `manual_repo` points at an arbitrarily-named repo. Repo-name **parsing** structurally cannot express either: it yields exactly one identifier, always the repo's "owner". | `ecr.tf:138-172` |
| **D10** | `slack.message.{success,error,info}.json.tmpl` are tracked, never read (templates are inline in `slack.go`), reference `{{.Env}}` which is not a field of `NotificationData`, and contain trailing commas (invalid JSON). Dead and broken. | `ci_lambda/*.tmpl`, `services/slack.go:186-274` |
| **D11** | Nothing runs `go test ./modules/workloads/ci_lambda/...`. The only workflow is `release.yml`. The "all tests pass while prod is broken" situation was never even observed by CI. | `.github/workflows/` |
| **D12** | The bootstrap binary is **not** committed and **is** already gitignored (`.gitignore:32`; `git log --all --diff-filter=A` finds no add). The real problem is worse: `ensureLambdaBootstrapExists` writes a **100-byte text file named `bootstrap`** when it is missing; `archive_file` zips it; the Lambda deploys green and every invocation dies with `Runtime.InvalidEntrypoint`. In a project copy with no Go build step this is the default outcome. | `app/terrafrom.go:39-78`, `app/terraform_destroy_progress_tui.go:153`, `lambda.tf:2-6` |
| **D13** | The config lookup is *inside* the retry loop, so unrecoverable configuration errors burn all attempts (exactly what D1 produced: 3× `service '' not found`). No retryable/non-retryable classification. | `deployer/deployer.go:88-122`, `services/ecs.go:65-72` |
| **D14** | An unknown event source returns an error → EventBridge retries garbage for up to 24h. Same for unmapped repos and unparsable details. | `handlers/handler.go:152` |
| **D15** | `deployer_test.go` does not test `DeployerV2`. It defines `testableDeployer`, a hand-copied reimplementation of the branching logic, and asserts against the copy. Retries, notification behaviour, and error paths of the real type are untested. | `deployer/deployer_test.go:44-83` |
| **D16** | Scheduled tasks store env at `/{env}/{project}/task/{task}/env` (4 segments). The SSM regex requires exactly `/{env}/{project}/{x}/{y}`, so scheduled-task env changes never redeploy anything. | `modules/ecs_task/env.tf:2` vs `handlers/handler.go:522` |
| **D17** | The SSM service segment is captured with `(\w+)`. Service names come from user YAML and may contain hyphens; `my-api` never matches. Same class of bug as D2. | `handlers/handler.go:522` |

---

## 4. Cleanliness targets (given)

- `aws-sdk-go v1.44.270` (EOS July 2025) → `aws-sdk-go-v2`; `go 1.20` → `go 1.22+`. Rest of meroku is already v2.
- Delete every meaningless `V2` suffix (`ECSServiceV2`, `DeployerV2`, `EventHandlerV2`, `NewECSServiceV2`) and the `"architecture": "v2-direct-naming"` log field. The README's V1/V2 narrative is fiction and goes with it.
- Hand-rolled logger passing `map[string]interface{}` → stdlib `log/slog` JSON handler; output must stay CloudWatch-parseable.
- Build `bootstrap` at apply time; keep it out of git.
- Dead config: `SERVICE_CONFIG` (never read), `DEPLOYMENT_TIMEOUT_SECONDS` (validated, never used), `local.service_config`.
- Retry comment says exponential, code is linear (`attempt * 5s`).
- Slack errors discarded with `_ =` in four places, yet a Slack failure in `handleECSEvent` fails the invocation → EventBridge retries → duplicate notifications. Notification failure must never fail a deployment.
- Backend identified by an empty-string sentinel threaded everywhere, with `"(backend)"` / `"(unknown)"` display hacks.

---

## 5. Decisions taken as given

1. `"backend"` is the backend identifier. Never `""`.
2. Flat string keys in the env-var JSON; keep the `task:` prefix; parsing must be anchored and project-scoped with `regexp.QuoteMeta`.
3. Add `detail.repository-name` to the ECR event pattern, listing exactly this project's repos.
4. Un-commit `bootstrap`; build at apply time with `null_resource` + `local-exec` (mirror `modules/appsync/auth_lambda.tf`); gitignore it; document the Go-on-apply-machine requirement.
5. Lambda stays fire-and-forget; `timeout = 60`.

### 5.1 Where those decisions break something — stated plainly

**Decision 3 as literally written will break the Lambda.** There is *one* rule
(`aws_cloudwatch_event_rule.ecr_event`) carrying the union of five sources. EventBridge
requires every key in the `detail` object to be present and matching. Adding
`detail.repository-name` to that rule silently drops all ECS state-change, SSM, and manual
`DEPLOY` events, because none of them carry that field. The rule **must** be split into four
rules (ECR / ECS-state / SSM / manual), each with its own pattern, target, and
`aws_lambda_permission` (distinct `statement_id` — the current one is hardcoded to
`AllowExecutionFromCloudWatch`). Requirement **FR-14** captures this.

**Decision 2 cannot fix D9, D16, or D17 by itself.** Anchoring the parser fixes D2, but no
parser can resolve `use_existing` (one repo → many identifiers) or `manual_repo` (arbitrary
repo name), and the SSM path shape differs for scheduled tasks. The plan therefore keeps the
decision's *output format* verbatim (flat keys, `task:` prefix) but moves *resolution* to
Terraform-emitted lookup maps — `ECR_REPO_MAP` (repo → [ids]) and `SSM_SERVICE_MAP`
(prefix → id) — exactly as `S3_SERVICE_MAP` and `ECS_SERVICE_MAP` already do. The anchored,
`QuoteMeta`-scoped parser is still written, and becomes the *independent second derivation*
that the boundary test and the cold-start self-check compare against. That is what makes D1
un-reintroducible; using the parser as the resolver would leave D9 unfixed.

**Decision 4 has four consequences that need accepting:**
- `data.archive_file` is a data source. Making it `depends_on` the build resource defers its
  read to apply time; the first plan after the change will show churn (same as
  `modules/appsync/auth_lambda.tf` today).
- Go becomes a hard requirement on every machine that runs `terraform apply`, including CI
  runners and any customer laptop running the prebuilt `meroku` binary. The provisioner must
  fail with a readable message when `go` is absent, not with an exec error.
- `app/terrafrom.go:ensureLambdaBootstrapExists` **must be deleted** in the same change. If it
  survives, any path that skips the provisioner (`-target`, `terraform destroy`, provisioner
  failure) hands a dummy text file to the zip and deploys a permanently broken Lambda (D12).
- `var.lambda_path` becomes unused but **cannot be deleted**: already-generated
  `project/env/*/main.tf` pass it, and passing an undeclared variable is a hard Terraform
  error. Keep it declared and deprecated; remove it from `env/main.hbs` so new generations omit it.

**Decision 1 has no conflicts.** `"backend"` is already what Terraform emits
(`lambda.tf:334`), what the generated GH workflows send (`{"service":"backend"}`), and what
`docs/ECR_STRATEGY.md` documents. Only the Go side is wrong.

**Decision 5 is fine**, with one constraint: the whole retry budget plus Slack must fit inside
60s. Today's linear `5s, 10s` backoff plus two 10s Slack timeouts can consume 35s+ of it.

---

## 6. Functional requirements

**Identifiers**
- **FR-1** The backend identifier is the string `backend` everywhere: Terraform map key, Go
  constant, log fields, Slack messages, manual-deploy payloads. No empty-string sentinel, no
  `(backend)` / `(unknown)` display fallbacks.
- **FR-2** Identifier formats (`backend`, `{service}`, `task:{name}`) and the ECR/SSM name
  formats derived from them are defined in exactly one artifact, consumed by both Terraform
  and Go. Neither side may contain an identifier string literal.
- **FR-3** Every identifier the Lambda can derive from any supported event must resolve to an
  entry in the merged target map, or be reported as `ignored` — never as a retryable failure.

**Event handling**
- **FR-4** ECR push → resolve repository name to *all* identifiers that use it (fan-out,
  fixes D9) → deploy each. Services use `UpdateService`; `task:` identifiers register a new
  task-definition revision with the pushed image URI.
- **FR-5** SSM parameter change → resolve by longest-prefix match against a Terraform-emitted
  prefix map, covering `/{env}/{project}/backend/…`, `/{env}/{project}/{service}/…` and
  `/{env}/{project}/task/{name}/…` (fixes D16, D17).
- **FR-6** S3 env-file write → deploy every identifier bound to that exact `bucket`+`key`.
  The map must contain backend files *and* per-service files, keyed on the same bucket string
  the task definitions use (fixes D7, D8).
- **FR-7** Manual `DEPLOY` → deploy the named identifier, optionally pinned to a supplied task
  definition. The event must be project- and environment-scoped so a manual deploy in project
  A cannot deploy project B.
- **FR-8** ECS deployment state change → Slack notification only. Never deploys. Never returns
  an error. `SERVICE_STEADY_STATE` stays suppressed.
- **FR-9** Unknown source, unmapped repo, unparsable detail, or feature-flag-disabled path →
  structured `ignored` response with `nil` error (fixes D14).

**Deployment**
- **FR-10** Service deploys pass the task-definition *family* to `UpdateService` and let ECS
  resolve the latest ACTIVE revision; the resolved ARN is read from the response. No
  `ListTaskDefinitions`, no client-side sorting (fixes D3 at the root and removes an IAM
  permission).
- **FR-11** Scheduled-task deploys clone the current definition and carry forward **every**
  field the API returns, including `PidMode`, `IpcMode`, `InferenceAccelerators` (fixes D6).
  Image matching must handle digest refs (`@sha256:…`) and registry ports.
- **FR-12** If a revision registration would update zero containers, it is an error, not a
  silent no-op re-registration.
- **FR-13** Retries: real exponential backoff with jitter, ctx-aware, and only for retryable
  AWS errors. Configuration errors (unknown identifier, unknown service) fail on the first
  attempt with a clear message (fixes D13).

**Terraform**
- **FR-14** Four separate event rules: ECR (filtered to this project's repos, `action-type=PUSH`,
  `result=SUCCESS`), ECS state (filtered to this cluster's service ARNs by prefix), SSM
  (filtered to `/{env}/{project}/` prefix), manual (filtered to this project + env). Each with
  its own target, permission, and unique `statement_id`.
- **FR-15** The repo list in the ECR pattern and the keys of `ECR_REPO_MAP` are produced by the
  same expression — they cannot drift.
- **FR-16** `SERVICE_CONFIG`, `DEPLOYMENT_TIMEOUT_SECONDS`, and `local.service_config` are removed.
- **FR-17** `bootstrap` is built by Terraform at apply, from source, with a content-hash
  trigger; `ci_lambda.zip` and `bootstrap` are gitignored.
- **FR-18** `iam:PassRole` gains a `iam:PassedToService = ecs-tasks.amazonaws.com` condition;
  `ecs:UpdateService` is scoped to this module's service ARNs; `ecs:ListTaskDefinitions` is dropped.

---

## 7. Non-functional requirements

- **NFR-1** Go 1.22+, `aws-sdk-go-v2` only. `aws-sdk-go` v1 absent from `go.mod` at the end.
- **NFR-2** All AWS and HTTP calls take a `context.Context` derived from the Lambda handler ctx.
- **NFR-3** `log/slog` JSON handler on stdout. Keys `timestamp`, `level` (lowercase), `message`
  preserved; `project`/`env` as base attributes. Attributes are flat (the current nested
  `fields.*` shape goes away — a documented, deliberate break of any existing Logs Insights
  query on `fields.x`).
- **NFR-4** Notification delivery can never influence deployment outcome or invocation result.
  Enforced by type: the notifier's method returns nothing.
- **NFR-5** No type carries a `V*` suffix. No log field describes an "architecture".
- **NFR-6** Cold start + one deploy stays well inside 60s: default 2 retries, 1s base backoff,
  Slack timeout 5s, Slack calls bounded by the remaining ctx deadline.
- **NFR-7** Unit tests run with no AWS credentials and no network. AWS access is behind a
  narrow interface with a fake.
- **NFR-8** CI runs `go vet` + `go test ./...` for the module, plus the Terraform↔Go boundary
  test, on every PR touching `modules/workloads/**` (fixes D11).
- **NFR-9** Public-repo hygiene: no account IDs, ARNs, webhook URLs, or real project names in
  fixtures. Synthetic values only.

---

## 8. Authoritative conventions (verified in Terraform, must be matched)

| Thing | Format | Source |
|---|---|---|
| Backend ECR repo | `{project}_backend` | `ecr.tf:5` |
| Service ECR repo | `{project}_service_{name}` | `ecr.tf:179` |
| Task ECR repo | `{project}_task_{name}` (dev only; other envs pull `var.ecr_url`) | `modules/ecs_task/main.tf:44,46` |
| Backend SSM env | `/{env}/{project}/backend/env` | `env.tf:2` |
| Service SSM env | `/{env}/{project}/{service}/env` | `env_services.tf:5` |
| Task SSM env | `/{env}/{project}/task/{task}/env` | `modules/ecs_task/env.tf:2` |
| `ECS_SERVICE_MAP` keys | `backend`, then each service name | `lambda.tf:334,341` |
| `SCHEDULED_TASK_MAP` keys | `task:{name}` | `lambda.tf:363` |
| ECS cluster | `{project}_cluster_{env}` | `CLAUDE.md`, `ecs.tf` |
| Service names | user YAML — **may contain hyphens**; `\w+` is wrong | `var.services[].name` |

---

## 9. Out of scope

- Deployment waiters / stability polling (decision 5).
- The `github.actions` / "Deployment Ready" design in `docs/CI_CD_EVENTBRIDGE_PATTERN.md` — it
  is a design doc, not implemented here.
- Cross-account ECR *pull* mechanics (unchanged); note only that in `cross_account` mode no
  local ECR event is ever emitted, so those environments deploy via manual `DEPLOY` events.
- Rewriting the web UI's CI/CD generator beyond the minimum needed by FR-7 (see §11).

---

## 10. Acceptance criteria

1. A synthetic project with a backend, two services (one hyphenated, one using
   `ecr_config.mode=use_existing`), and one scheduled task produces a Terraform identifier set
   that the Go binary resolves 100% — asserted by a test that *executes Terraform* and feeds
   its real output to the real Go code.
2. Deleting or altering the backend identifier on either side fails that test.
3. Pushing to `otherproject_service_api` in the same account produces no deployment.
4. Task family with 12 revisions deploys revision 12.
5. A Slack outage produces a successful deployment, a logged notification error, and no
   EventBridge retry.
6. `grep -rn '"V2"\|V2\b\|aws-sdk-go/aws' modules/workloads/ci_lambda` returns nothing.
7. `terraform apply` on a machine without a prebuilt binary produces a working Lambda; on a
   machine without Go it fails with an actionable message, never with a dummy artifact.
8. `go test ./...` fails if `lambda.tf` hardcodes an identifier literal.

---

## 11. Open questions (need a human decision)

- **Q1 — manual-deploy contract (D5).** Recommended: rule requires `detail.project` and
  `detail.env`, sources `["action.deploy", "action.production", "action.{env}"]`,
  detail-type `["DEPLOY"]`; update `web/src/components/Sidebar.tsx`,
  `web/src/components/ServiceCICDConfiguration.tsx`, `docs/ECR_STRATEGY.md`, and rebuild
  `app/webapp/` in the same PR. Alternative: keep an unscoped legacy rule behind a variable
  defaulting to `true` for existing projects. Doing nothing leaves every generated non-prod
  deploy workflow silently dead **and** leaves manual deploys cross-project-unsafe.
- **Q2 — S3 bucket naming (D8).** `s3_to_service_map` prefixes `{project}-` and suffixes
  `-{env}`; task definitions use the raw bucket. One of them is wrong. Recommended: raw
  (matches `variables.tf:210`'s stated intent, the backend path, and the actual ARNs).
  This changes behaviour for anyone who relied on the prefixing — believed to be nobody, since
  the path never fired.
- **Q3 — Lambda architecture.** Recommended `arm64` + `architectures = ["arm64"]` (cheaper,
  faster cold start; cross-compile is a `GOARCH` flag). Say no and it stays `x86_64`.
- **Q4 — EventBridge target retry policy / DLQ.** Recommended: `retry_policy` with
  `maximum_retry_attempts = 2`, `maximum_event_age_in_seconds = 300`, plus an SQS DLQ, so a
  hard failure surfaces instead of retrying for 24h. Adds one queue per project/env.
- **Q5 — cold-start self-check severity.** Recommended: fail initialization (loud, immediate,
  impossible to ignore). The alternative — log-and-continue — is how D1 survived for months.
  Risk accepted: a contract violation blocks all deploys for that env until fixed.
- **Q6 — doc location.** These files were requested at `ai-docs/sessions/…`; `CLAUDE.md`
  mandates `ai_docs/`. Both directories exist in-tree. Flagging, not deciding.
