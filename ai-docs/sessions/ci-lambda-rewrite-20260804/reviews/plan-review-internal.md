# Adversarial review — CI Lambda rewrite plan

Reviewed: `ai-docs/sessions/ci-lambda-rewrite-20260804/requirements.md`,
`ai-docs/sessions/ci-lambda-rewrite-20260804/architecture.md`.
Cross-checked against `modules/workloads/lambda.tf`, `modules/workloads/ecr.tf`,
`modules/workloads/variables.tf`, `modules/workloads/env.tf`,
`modules/workloads/env_services.tf`, `modules/ecs_task/{main,env}.tf`,
`modules/appsync/auth_lambda.tf`, `app/terrafrom.go`,
`app/terraform_destroy_progress_tui.go`, `app/deploy.go`, `env/main.hbs`,
`web/src/components/{Sidebar,ServiceCICDConfiguration}.tsx`,
`modules/workloads/ci_lambda/**`, `.github/workflows/`, `Taskfile.yml`,
`project/Taskfile.yml`.

Everything below was reproduced or read, not recalled. Tool versions used for
reproduction: **Terraform v1.15.8 (darwin_arm64)**, **Go 1.26.5**.

Two claims in the plan were checked and found **correct**, so they generate no findings:
EventBridge does require every `detail` key in a pattern to be present in the event
(AWS EventBridge User Guide, *Event patterns*: "For an event pattern to match an event,
the event must contain all the field names listed in the event pattern"), so the
one-rule → four-rules split is necessary; and a provider-free Terraform module really
does `init -backend=false` + `apply` with all outbound network blackholed (reproduced
with `HTTPS_PROXY=http://127.0.0.1:9`).

---

## 1. CRITICAL — `null_resource` + `data.archive_file` breaks `plan`, `apply` **and** `destroy` on every checkout that has no prebuilt `bootstrap`

**Where:** `architecture.md` §7.1b (`null_resource.build_ci_lambda` / `data.archive_file.lambda`),
`requirements.md` §5 decision 4, `architecture.md` §7.2 (deleting `ensureLambdaBootstrapExists`).

**Why it is wrong.** `triggers = { src = sha1(...source files...) }` is a pure function of
committed source. On a fresh clone or a CI runner, the sources are unchanged, so the
`null_resource` is *not* replaced, so the provisioner never runs, so `bootstrap` never
exists — but `data.archive_file` is still read during the plan walk, because a
`depends_on` on an unchanged resource does not defer anything. Reproduced verbatim:

```
$ terraform plan            # state present, triggers unchanged, bootstrap deleted
data.archive_file.lambda: Reading...
Planning failed. Terraform encountered an error while generating this plan.
Error: Archive creation error
  error creating archive: error archiving file: could not archive missing file: ./bootstrap
```

The same error was reproduced for `terraform apply` and for `terraform destroy`.
That is not a corner case: it is the normal state of every CI runner, every fresh
`git clone`, and every customer laptop, because `bootstrap` is gitignored
(`.gitignore:32`).

The plan says to "mirror `modules/appsync/auth_lambda.tf`", but it mirrored the wrong
half. That file already contains the fix, with a comment naming this exact scenario
(`modules/appsync/auth_lambda.tf:56-64`):

```hcl
      # A fresh clone (CI) has no build directory even though the source hashes
      # are unchanged. Without this the archive step would fail or package a
      # stale artifact.
      staged = fileexists(local.auth_lambda_stage_probe) ? "present" : "absent"
```

I verified that adding that probe does fix `plan`/`apply`/`destroy` once the state has
stabilised at `staged = "present"` (destroy mode does not re-read a data source whose
dependency is being destroyed). Without it, all three fail.

Compounding it: `architecture.md` §7.2 deletes `ensureLambdaBootstrapExists`, which is
called at `app/terraform_destroy_progress_tui.go:153` *specifically* to keep destroy
working — the in-tree comment says "This prevents errors when the archive_file data
source tries to archive a missing file". Removing the workaround without adding the
replacement is a straight regression.

**Concrete alternative (preferred):** delete `data.archive_file` from this path entirely.
Have the provisioner emit the zip, and take the hash from the trigger:

```hcl
resource "null_resource" "build_ci_lambda" {
  triggers = { src = local.ci_lambda_src_sha, arch = "arm64", cmd = md5(local.build_cmd) }
  provisioner "local-exec" { working_dir = ..., command = local.build_cmd }  # go build && zip
}

resource "aws_lambda_function" "lambda_deploy" {
  filename         = "${local.build_dir}/ci_lambda.zip"
  source_code_hash = null_resource.build_ci_lambda.triggers.src
  ...
}
```

`filename` is only read by the provider during Create/Update, never during plan or
destroy, so the failure mode disappears instead of being papered over. If you keep
`archive_file`, you **must** add the `fileexists` probe trigger.

---

## 2. CRITICAL — the new `manual_deploy` rule kills every manual deploy, including the one that works today

**Where:** `architecture.md` §7.1c (`aws_cloudwatch_event_rule.manual_deploy`), §7.2 last row
("Only if Q1 is answered…"), §9 step 11 ("Optional per Q1"); `requirements.md` Q1.

**Why it is wrong.** The rule as written is unconditional:

```hcl
detail = { project = [var.project], env = [var.env] }      # Q1
```

Combined with the EventBridge semantics the plan itself cites, that requires both
`detail.project` and `detail.env` to be present. Neither generator emits them
(read in-tree, not recalled):

- `web/src/components/Sidebar.tsx:769` and `:878` →
  `Source=action.${env}, DetailType=DEPLOY, Detail="{\"service\":\"backend\"}"`
- `web/src/components/ServiceCICDConfiguration.tsx:123-127` →
  `"Source": "github.actions.${env}", "DetailType": "SERVICE_DEPLOY", "Detail": {service, env, trigger, commit, branch}` — no `project`.

Today, `env == "production"` **does** work: `action.production` + `DEPLOY` matches the
current rule (`lambda.tf:203-218`) and `handlers/handler.go:141-146` routes it. Under
the plan's rule it stops matching. Because §7.2/§9 mark the web-side change "optional",
the plan can legitimately ship in a state strictly worse than the status quo: D5 grows
from "non-prod manual deploys are dead" to "all manual deploys are dead", which is also
the only remaining trigger for cross-account environments (`requirements.md` §9) and for
non-dev scheduled tasks (see finding 12).

The `github.actions.{env}` / `SERVICE_DEPLOY` contract is also absent from the new rule's
`source`/`detail-type` lists *and* from the handler routing in `architecture.md` §5.6, so
the per-service workflow stays dead either way.

**Concrete alternative:** make the rule and the generators one atomic change — remove the
"optional" framing from §7.2 and §9 step 11 and fold web + `app/webapp` rebuild into the
same PR as `lambda.tf`. If Q1 cannot be answered in time, ship the rule *without* the
`detail` filter and rely on `handler.manual`'s project/env check (already specified in
§5.6 as "defence in depth") as the primary scoping, gated on a variable
`strict_manual_deploy_scope` that defaults to `false` for existing projects and `true`
for new generations. Add `"action.${var.env}"`, `"github.actions.${var.env}"` to `source`
and `"SERVICE_DEPLOY"` to `detail-type`, or explicitly declare the per-service workflow
unsupported and delete its generator.

---

## 3. HIGH — D8 is only half fixed: per-service S3 env files still get no EventBridge rule

**Where:** `architecture.md` §7.1a ("Rebuild `s3_to_service_map` … with raw bucket names"),
`requirements.md` FR-6 / D8.

**Why it is wrong.** D8 has two halves: the map is wrong *and* the rules do not exist.
The architecture fixes only the map. All three S3 resources still iterate the
backend-only list, and nothing in §7 changes them:

- `lambda.tf:246` `aws_cloudwatch_event_rule.s3_env_file_change_rule` — `for_each = { for file in local.env_files_s3 ... }`
- `lambda.tf:276` `aws_cloudwatch_event_target.lambda_target` — same
- `lambda.tf:284` `aws_lambda_permission.allow_eventbridge` — same

`local.env_files_s3` is derived only from `var.env_files_s3` (`variables.tf:209-217`);
per-service files live in `local.services_env_files_s3` (`variables.tf:328-338`). With
the map fixed and no rule added, a per-service S3 write still produces zero invocations,
so **FR-6 is not met**.

**Concrete alternative:** introduce one local and drive all three resources from it:

```hcl
all_env_files_s3 = distinct(concat(
  local.env_files_s3,
  flatten([for _, files in local.services_env_files_s3 : files]),
))
```

Then `for_each = { for f in local.all_env_files_s3 : "${f.bucket}-${f.key}" => f }`.
The existing `s3_event_rule_names` md5 suffix (`lambda.tf:301-318`) already keeps the
64-char rule-name limit safe; reuse it unchanged.

---

## 4. HIGH — `contract.json` is a *fourth* derivation of the name formats, not the single one; FR-2 is not achieved

**Where:** `architecture.md` §2 L1 ("ONE definition"), §2 L2 (`ci_identifiers/main.tf`),
§2 L4 static guard; `requirements.md` FR-2.

**Why it is wrong.** The plan routes only `lambda.tf` through the contract. The strings
the contract claims to own are still produced independently, from their own literals, by
the resources that actually create the objects:

| Contract key | Independent producer still in tree |
|---|---|
| `repo_backend_fmt` `"%s_backend"` | `ecr.tf:5` `name = "${var.project}_backend"` |
| `repo_service_fmt` `"%s_service_%s"` | `ecr.tf:179` `name = "${var.project}_service_${each.value.name}"` |
| `repo_task_fmt` `"%s_task_%s"` | `modules/ecs_task/main.tf:44` |
| `ssm_backend_fmt` | `env.tf:2` |
| `ssm_service_fmt` | `env_services.tf:5` |
| `ssm_task_fmt` | `modules/ecs_task/env.tf:2` |

`ci_identifiers` re-derives all six from `contract.json` and then *compares nothing*.
The static guard in §2 L4 reads `../../lambda.tf` only, so none of the six files above is
checked. Editing `contract.json` therefore silently desynchronises the Lambda's ECR
allow-list and SSM prefix map from the repositories and parameters Terraform actually
creates — the same failure shape as D1, one level up.

**Concrete alternative:** delete `repo_*_fmt` and `ssm_*_fmt` from `contract.json`
entirely and pass the *actual resource attributes* into `ci_identifiers`:

```hcl
module "ci_identifiers" {
  source              = "./ci_identifiers"
  backend_repo        = try(aws_ecr_repository.backend[0].name, "")
  service_repos       = { for n, r in aws_ecr_repository.services : n => r.name }
  service_repo_override = local.manual_or_shared_repo_names
  backend_ssm_prefix  = trimsuffix(aws_ssm_parameter.backend_env.name, "/env")
  service_ssm_prefixes = { for n, p in aws_ssm_parameter.services_env : n => trimsuffix(p.name, "/env") }
  ...
}
```

Task repo/param names need two new outputs from `modules/ecs_task`. What is left in
`contract.json` is `backend_id` and `task_id_prefix` — the only two values that genuinely
must agree across the Terraform↔Go boundary, and neither is a format string. That also
deletes findings 5 and 7 outright.

---

## 5. HIGH — the boundary test (the plan's central verification mechanism) does not work as written, and its CI gate is optional

**Where:** `architecture.md` §2 L4 `tfboundary_test.go`, §8 "Boundary" row, §9 step 3;
`requirements.md` NFR-8, acceptance criterion 1.

**Why it is wrong.** Two independent defects.

(a) **The module source path is wrong.** `writeScratchModule(t, ...)` writes into a temp
dir and the scratch module says `source = "../../ci_identifiers"`. Module sources are
resolved relative to *the file that declares them*, i.e. the scratch dir. On macOS
`t.TempDir()` is `/var/folders/…/T/TestX/001`, so this resolves to
`/var/folders/…/T/ci_identifiers` and `terraform init` fails with "Unreadable module
directory". The test can never have been run.

(b) **`t.Skip` makes a mandatory gate silently optional.** NFR-8 and acceptance criterion
1 rest entirely on this test. If a CI job forgets `hashicorp/setup-terraform`, or the
action is bumped and fails soft, the test skips and the pipeline is green while the
boundary is unchecked. That is exactly the failure mode that let D1 survive.

What does **not** break it, contrary to the concerns worth checking: no network is needed
(verified — `terraform init -backend=false` and `terraform apply` on the provider-free
module both succeed with `HTTPS_PROXY=http://127.0.0.1:9 HTTP_PROXY=http://127.0.0.1:9`,
exit 0), and parallel `go test` is safe because `t.TempDir()` is per-test and tests in one
package are serial unless `t.Parallel()` is called.

**Concrete alternative.** Fix (a) with an absolute path:

```go
_, thisFile, _, _ := runtime.Caller(0)
src := filepath.Join(filepath.Dir(thisFile), "..", "..", "ci_identifiers")
```

Fix (b) by making absence fatal in CI: `if os.Getenv("CI") != "" { t.Fatalf("terraform
required") } else { t.Skip(...) }`, and run with `CHECKPOINT_DISABLE=1 TF_IN_AUTOMATION=1
-input=false`.

**Cheapest alternative that still spans the boundary genuinely, without a terraform
binary on every developer machine:** add a `task lambda:contract:golden` target that runs
the real `ci_identifiers` module for the synthetic project and writes
`contract/testdata/tf_identifiers.golden.json`, committed. An always-on Go test (no build
tag, no terraform) asserts that Go's derivation and `config.Load` + `SelfCheck` fully
resolve that golden file; a CI job regenerates it and fails on `git diff --exit-code`.
Both sides are still checked against real Terraform output, the check runs on every
`go test`, and drift in either direction is a hard failure. Keep the
terraform-executing test as the regeneration mechanism rather than as the gate.

---

## 6. HIGH — build sequence steps 9 and 10 are in the wrong order; the intermediate commit deploys a permanently broken Lambda

**Where:** `architecture.md` §9 steps 9 and 10; §4 row `bootstrap`; `requirements.md` D12.

**Why it is wrong.** §9 claims "tree compiles and tests pass at every step". Step 9 lands
the `null_resource` build; step 10 deletes `ensureLambdaBootstrapExists`. Between those
two commits, `app/terrafrom.go:78` (`terraformInitIfNeeded`) still runs
`ensureLambdaBootstrapExists()` **before** terraform, which writes a 100-byte text file
named `bootstrap` when it is missing. The build resource's own artifact probe then sees
the artifact as present, does not rebuild, and `archive_file` zips the dummy. Result: a
green apply and `Runtime.InvalidEntrypoint` on every invocation — D12 exactly, shipped by
the change that is supposed to fix it.

**Concrete alternative:** swap the order (delete `ensureLambdaBootstrapExists` and
`buildDeploymentLambda` *before* the Terraform change) or merge steps 9 and 10 into one
commit. Note also that `app/deploy.go:475-476` sets `GOARCH=amd64` with `os.Setenv` on
the meroku process and never unsets it, and `app/deploy.go:472` `os.Chdir(
"infrastructure/modules/workloads/ci_lambda")` is unchecked with the error discarded at
`app/deploy.go:76` — so while it survives, it can also leak `GOARCH=amd64` into the
`local-exec` child. Deleting it first removes both hazards.

---

## 7. HIGH — `Config.SelfCheck` item 2 (layer L3) is tautological and cannot catch a D1-class divergence

**Where:** `architecture.md` §2 L3 item 2; §5.1 `ParseRepo` / `ExpectedRepo`;
`requirements.md` Q5.

**Why it is wrong.** Item 2 asserts
`spec.ExpectedRepo(project, id)` → `spec.ParseRepo(project, repo)` returns `id`. Both
functions are Go, both derive from the same embedded `contract.json`, and **no Terraform
data enters the round trip** — the `repo` string is manufactured by `ExpectedRepo`, not
read from `ECR_REPO_MAP`. It can only catch a typo inside `contract.go`, which
`contract_test.go` already covers per §8 row 1.

Worse, it is vacuous precisely for the two cases it is advertised to protect. For the
plan's own fan-out example (`reporting` sharing `acme_service_api`),
`ExpectedRepo("acme","reporting")` returns `acme_service_reporting`, which round-trips
fine and says nothing about the real repo. For `manual_repo` it returns a name that is
not an ECR repo at all. Only items 1 and 3 touch Terraform-produced data.

**Concrete alternative:** delete item 2 and the "independent second derivation" framing.
`SelfCheck` reduces to: every value in `ECR_REPO_MAP` and `SSM_SERVICE_MAP` and every key
of `S3_SERVICE_MAP` is a key of `Targets`; `backendID` is a key of `Targets`. That is the
whole of the runtime backstop. `ParseRepo` then has no production or self-check consumer
and should be dropped with `utils/service_name_extractor.go` rather than ported (see
finding 18).

---

## 8. MEDIUM — `format()` and `fmt.Sprintf` disagree on arity and on non-string args, and `contract.json` carries no arity metadata

**Where:** `architecture.md` §2 L1 ("Same verbs, same format strings, one file.
`format()` and `Sprintf` agree on `%s`").

**Why it is incomplete.** For the eight strings as written — all `%s`, all string
arguments — the two do agree; I verified each of the eight against both engines,
including that a format string obtained from `jsondecode(file(...))` is accepted by
`format()` at plan time in every position the plan uses it
(`format(jsondecode("{\"f\":\"%s_backend\"}").f, "acme")` → `"acme_backend"`). So this is
latent, not live. But the plan states the equivalence as a settled property of a file
that is unversioned, unschema'd, and checked by no compiler on either side, and the two
engines fail in opposite directions:

| case | Terraform 1.15.8 `format()` | Go 1.26.5 `fmt.Sprintf` |
|---|---|---|
| extra arg | `Error: Invalid function argument` | `"acme_backend%!(EXTRA string=extra)"` |
| missing arg | `Error: Error in function call` | `"acme_service_%!s(MISSING)"` |
| `%s` + number | `"5_backend"` | `"%!s(int=5)_backend"` |
| `%s` + bool | `"true_backend"` | `"%!s(bool=true)_backend"` |
| `%s` + list | `Error` | `"[a]_backend"` |
| `%%` | `"100% x"` | `"100% x"` |

Terraform hard-fails the apply; Go silently emits a corrupted identifier. Adding or
removing one `%s` in `contract.json` is a one-character edit.

**Concrete alternative:** adopt finding 4 and the question disappears. If you keep format
strings, drop `%`-verbs for named placeholders — `"{project}_service_{service}"` —
applied with chained `replace()` in HCL and `strings.NewReplacer` in Go. Arity-free,
verb-free, and (unlike `%s`) mechanically convertible into the `ParseRepo` regex. Add a
`contract_test.go` case pinning the exact placeholder set for each key.

---

## 9. MEDIUM — `service_repo_names` derivation is wrong for `manual_repo` and namespaced ECR repos, and needlessly makes the ECR pattern unknown at plan

**Where:** `architecture.md` §7.1a:
`{ for n, url in local.service_ecr_urls : n => element(split("/", url), length(split("/", url)) - 1) if url != "" }`.

**Why it is wrong.** Three problems, all visible against `ecr.tf:153-172` and
`variables.tf:317-322`:

1. ECR repository names may contain `/` (`team/legacy-api`). Taking only the last
   segment yields `legacy-api`, which matches neither `detail.repository-name` nor any
   real map key.
2. `ecr_config.repository_uri` is a URI and commonly carries a tag —
   `myregistry.io/team/legacy-api:v1` yields `legacy-api:v1`.
3. For `create_ecr` (the common case) the expression re-derives from
   `aws_ecr_repository.services[*].repository_url`, a computed attribute, a string that is
   already statically known as `"${var.project}_service_${name}"` (`ecr.tf:179`). That
   makes `keys(module.ci_identifiers.ecr_repo_ids)` `(known after apply)` and the ECR
   rule's `event_pattern` churn on first apply, for no benefit. (Terraform does tolerate
   unknown map keys here — verified, it yields an unknown map rather than an error — so
   this is churn, not breakage.)

**Concrete alternative:** take names from `aws_ecr_repository.*.name` per finding 4. If
you must parse a URI, strip the registry host and the tag/digest explicitly:
`replace(replace(url, "/^[^/]+\\//", ""), "/[:@].*$/", "")`.

---

## 10. MEDIUM — the exhaustive ECR repo list is bounded by a 2,048-character EventBridge quota

**Where:** `architecture.md` §7.1c `aws_cloudwatch_event_rule.ecr_push`,
`repository-name = keys(module.ci_identifiers.ecr_repo_ids)`; `requirements.md` FR-15.

**Why it matters.** The `PutRule` API string maximum is 4,096 (botocore
`events/2015-10-07/service-2.json`, shape `EventPattern` `max: 4096`), but the *service
quota* is lower: EventBridge quotas, "Event pattern size — Each supported Region: 2,048 —
Maximum size of an event pattern, in characters". The fixed part of this pattern is ~125
characters, leaving ~1,900 for the list. At ~30 characters per
`"{project}_service_{name}",` that is roughly 60 repositories, and materially fewer with
long project/service names. Past that, `terraform apply` fails with
`InvalidEventPatternException` and no obvious cause, and the quota is a support-ticket
increase rather than a code change.

Empty lists are not a live risk — `repo_pairs` always contains the backend entry, so
`keys()` is never `[]` even for a project with zero services and zero tasks (verified by
executing the module). But that is accidental: it holds only because the backend repo is
appended unconditionally, including in `ecr_strategy = "cross_account"` where
`aws_ecr_repository.backend` has `count = 0` (`ecr.tf:6`).

**Concrete alternative:** `repository-name = [{ prefix = "${var.project}_" }]`. Constant
size, still project-scoped, and anything that slips through (e.g. project `acme` also
matching `acme_two_backend`) is rejected by the `ECR_REPO_MAP` lookup as `ignored` per
FR-9 — the map remains the authority, which is the plan's own stated design. If you keep
the explicit list, add a `precondition` on `length(join(",", keys(...))) < 1800`.

---

## 11. MEDIUM — Q5 (fail initialization on SelfCheck) re-creates D14

**Where:** `architecture.md` §2 L3 ("Failure returns an error from `Initialize`, the Lambda
fails to start"), §5.7 `fatal(log, cfg.SelfCheck(spec))`; `requirements.md` Q5, FR-9, D14.

**Why it is wrong.** EventBridge invokes this Lambda **asynchronously**. An init failure
is an invocation failure, so every event in the account that matches any of the four
rules is queued and retried by the async invoke path, then held on the event age budget.
That is precisely the behaviour FR-9/D14 exist to remove — "an unknown event source
returns an error → EventBridge retries garbage" — reintroduced with a larger blast radius
(all events, not one). It also guarantees that the diagnostic the operator most needs
(the structured `ignored`/`selfcheck_failed` log with the offending key) is emitted once
per retry rather than once.

**Concrete alternative:** log the failure at `error` with a stable
`event=contract_selfcheck_failed` attribute and the offending keys, start normally, and
return `Response{Status:"ignored"}, nil` for any event whose identifier does not resolve.
Get the loudness from a CloudWatch metric filter + alarm on that attribute (and from the
CI boundary test, which is where a contract violation should be caught anyway). Q4's DLQ
becomes the safety net for genuine failures. Note also that the architecture's own risk
table says "retrying for 24h" while the async-invoke maximum event age is 6 hours by
default — worth correcting so the DLQ sizing discussion starts from the right number.

---

## 12. MEDIUM — FR-4's scheduled-task ECR path is structurally impossible outside `dev`, and the architecture does not say so

**Where:** `architecture.md` §2 L2 (`repo = format(local.c.repo_task_fmt, var.project, n)`,
unconditional), §7.1c ECR rule; `requirements.md` FR-4, §8 table.

**Why it is wrong.** `modules/ecs_task/main.tf:44-46` creates `{project}_task_{name}` only
when `var.env == "dev"`; for every other environment `local.ecr_image = var.ecr_url`, and
`env/main.hbs:688,741` sets that to a **cross-account** URL. So in staging/prod the ECR
rule lists a repository that does not exist in the account, no `ECR Image Action` event is
ever emitted locally, and `ECR_REPO_MAP` contains an unreachable key. `requirements.md`
§8 notes "(dev only)" in a table cell, but §6 FR-4 states the behaviour unconditionally and
the architecture implements it unconditionally.

Combined with finding 2, non-dev scheduled tasks end up with **no** working deploy
trigger: not ECR (no event), not manual (rule broken), not SSM for the task path unless the
operator edits SSM directly.

**Concrete alternative:** gate the task repo entries on `var.env == "dev"` in
`ci_identifiers`, state the non-dev trigger explicitly in `architecture.md` §7.1c, and
treat finding 2 (manual path) as a blocker rather than as Q1-optional, since it is the
only non-dev trigger for tasks.

---

## 13. MEDIUM — build artifacts are shared between environments; concurrent applies clobber each other

**Where:** `architecture.md` §7.1b `locals { ci_lambda_dir = "${path.module}/ci_lambda" }`,
`source_file`/`output_path` both under that directory.

**Why it is wrong.** `env/dev` and `env/prod` both source `modules/workloads`, so
`path.module` resolves to the *same* directory for both. `go build -o bootstrap` truncates
that file in place; a concurrent `task infra-apply env=prod` can read a half-written
`bootstrap` into its zip, and the two environments cannot hold different architectures or
Go versions. The current code writes `ci_lambda.zip` relative to the *working* directory
(`lambda.tf:5` `output_path = "ci_lambda.zip"`), i.e. per-env — the plan removes that
isolation.

**Concrete alternative:** build to `${path.module}/ci_lambda/.build/${var.env}/bootstrap`
and zip to the same subdirectory; add `modules/workloads/ci_lambda/.build/` to
`.gitignore` (this is what `modules/appsync/auth_lambda.tf:5-7` does with `.build/`,
including writing a `.gitignore` into it from the provisioner).

---

## 14. MEDIUM — `triggers` omits the inputs that actually determine the artifact, so the `arm64` switch can deploy an x86_64 binary

**Where:** `architecture.md` §7.1b `triggers = { src = sha1(...) }`; `requirements.md` Q3.

**Why it is wrong.** The glob itself is fine — I verified `fileset(dir,
"**/*.{go,json,mod,sum,tmpl}")` matches `main.go`, `sub/x.go`, `go.mod`, `go.sum`,
`a.json`, `t.tmpl` on Terraform 1.15.8, so brace alternation and zero-depth `**` both
work. The problem is what is *not* hashed: `GOARCH`, `GOOS`, the `-trimpath -ldflags`
string, and the Go toolchain version. Once the resource exists with a given `src`, none of
those changes causes a rebuild.

`architectures = ["arm64"]` is an in-place update on `aws_lambda_function` (not a
replacement), so the sequence "flip Q3 to arm64, source unchanged" updates the function's
architecture while shipping whatever `bootstrap` is already on disk — the previously built
x86_64 one — and every invocation dies with an exec-format failure. The precedent file
already guards against this: `modules/appsync/auth_lambda.tf:56` includes
`build_command = md5(local.auth_lambda_build_command)`.

**Concrete alternative:** `triggers = { src = ..., goos = "linux", goarch = "arm64",
build_cmd = md5(local.build_command), go_version = ... }` — plus the artifact-presence
probe from finding 1.

---

## 15. MEDIUM — the plan is over-engineered relative to the defect it is fixing

**Where:** `architecture.md` §2 "The D1 firewall — four layers".

Three of the four layers restate one side's assumptions rather than comparing the two
sides: L1's format strings are a fourth copy of formats that live in `ecr.tf`/`env*.tf`
(finding 4), L3's round-trip is Go-against-Go (finding 7), and L4 skips whenever
`terraform` is off `PATH` (finding 5). The single mechanism that actually makes D1
un-reintroducible is much smaller and is already in the plan: **Go stops deriving
identifiers at all**, Terraform emits `ECR_REPO_MAP`/`SSM_SERVICE_MAP`, and an unmapped
key becomes `ignored`. Everything else is machinery that must be maintained and can itself
drift — and `ParseRepo`/`ExpectedRepo` in particular exist solely to feed a check that
proves nothing.

**Concrete alternative:** keep `contract.json` at `backend_id` + `task_id_prefix`; keep
the boundary check (as the golden-file form in finding 5); drop `ParseRepo`,
`ExpectedRepo`, and `SelfCheck` item 2. That removes findings 8 and 9 as well and cuts the
`contract` package to roughly three functions.

---

## 16. LOW — the `ci_identifiers` HCL in §2 L2 does not parse

**Where:** `architecture.md` §2 L2, the three single-line `variable` blocks.

`variable "service_names" { type = list(string), default = [] }` (and the two like it):

```
Error: Invalid single-argument block definition
  Single-line block syntax can include only one argument definition. To define
  multiple arguments, use the multi-line block syntax with one argument
  definition per line.
```

Expanded to multi-line, the module runs and produces exactly the outputs the plan claims
(I executed it: `ecr_repo_ids = {"acme_backend":["backend"], "acme_service_api":["api","reporting"],
"acme_service_payment-worker":["payment-worker"], "acme_task_cleanup":["task:cleanup"]}`,
`ssm_prefix_ids` including `/dev/acme/task/cleanup → task:cleanup`). So this is a document
defect, but it means the snippet was never run through the `terraform validate` that §9
step 2 requires.

**Fix:** multi-line blocks; run `terraform fmt -check && terraform validate` on the
snippet before it lands in the doc.

---

## 17. LOW — `ecs_service_map` cannot take `module.ci_identifiers.backend_id` as a bare object key

**Where:** `architecture.md` §7.1a, "`ecs_service_map` / `scheduled_task_map` keep their
current shape but take keys from `module.ci_identifiers.backend_id`, `.service_ids[k]`,
`.task_ids[n]`".

A dotted reference used as an object key is rejected:

```
Error: Ambiguous attribute key
  If this expression is intended to be a reference, wrap it in parentheses.
```

**Fix:** `{ (module.ci_identifiers.backend_id) = { ... } }` — and note the same applies to
`(module.ci_identifiers.task_ids[n])` inside the `for` expression's key position.

---

## 18. LOW — the SSM rule does not filter `operation`, so parameter deletion and Terraform's own parameter creation trigger deployments

**Where:** `architecture.md` §7.1c `aws_cloudwatch_event_rule.ssm_change`.

AWS's documented pattern for this event includes `operation`, whose values are `Create`,
`Update`, `Delete`, `LabelParameterVersion` (SSM User Guide, *Setting up notifications or
trigger actions based on Parameter Store events*). The plan filters on `detail.name`
prefix only. `aws_ssm_parameter.backend_env` (`env.tf:1`) and
`aws_ssm_parameter.services_env` (`env_services.tf:2`) are created by the same
`terraform apply` that creates the rule, so a fresh environment fires a deploy per
parameter at creation time, and any parameter deletion fires a deploy against a service
whose env has just vanished.

Confirmed while checking: `detail.name` does carry the leading `/` for hierarchical
parameters (the AWS example uses `"/Oncall"`, `"/Project/Teamlead"`), so the
`prefix = "/${var.env}/${var.project}/"` filter is correct as written.

**Fix:** `detail = { name = [{ prefix = "/${var.env}/${var.project}/" }], operation = ["Update"] }`.

---

## 19. LOW — duplicate `source` value when `var.env == "production"`

**Where:** `architecture.md` §7.1c,
`source = ["action.deploy", "action.production", "action.${var.env}"]`.

For `env = "production"` this emits `"action.production"` twice. Harmless to EventBridge
but it round-trips through `jsonencode` into state and is a diff-noise source.

**Fix:** `distinct([...])`.

---

## 20. LOW — §9 step 3 and step 9 disagree about when the static guard can pass

**Where:** `architecture.md` §9 step 3 ("Guard fails against current `lambda.tf` → keep it
skipped-with-TODO or land it in step 8") vs step 9 (`lambda.tf` is rewritten here, and
"Enable the static guard").

`lambda.tf` is not touched until step 9, so the guard cannot pass at step 8.

**Fix:** land the guard in step 9 only; delete the step-8 alternative from step 3.

---

## Defect coverage summary (requirements.md D1–D17)

| ID | Fixed by the architecture? |
|---|---|
| D1, D2, D3, D4, D6, D7, D9, D10, D11, D13, D15, D16, D17 | yes |
| **D5** | **no** — deferred to Q1, and finding 2 makes it strictly worse than today |
| **D8** | **half** — map fixed, rules not created (finding 3) |
| **D12** | **regressed** — dummy artifact removed, but findings 1 and 6 introduce a hard `plan`/`destroy` failure and a shippable dummy-artifact window |
| **D14** | **contradicted** by Q5's fail-init (finding 11) |

---

**Verdict: PROCEED WITH CHANGES** — the direction (Terraform-emitted lookup maps, four
scoped rules, SDK v2, build-at-apply) is right and the requirements are sound, but
§2 (L1/L3/L4), §7.1b, §7.1c and §9 steps 9–10 must be re-drafted and the Q1 web change
made non-optional before any code lands.
