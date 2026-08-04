# Changelog

## v3.24.0

The CI/CD Lambda is rewritten. Its main path — redeploy the backend when a new
image is pushed — had never worked, and two authentication bypasses in the
AppSync authorizer are closed. A fresh checkout of a deployed project now
connects itself instead of reporting that nothing is deployed.

### Syncing a checkout to what is deployed

`env/` is generated and gitignored, so a fresh clone never has one. meroku used
to read that absence as "nothing is deployed" — while the resources sat in the
S3 state, untouched — and every terraform command then failed with `Backend
initialization required`.

It now asks the only thing that knows. When `env/<env>/.terraform` is missing,
meroku reads the state backend named in `<env>.yaml`: resources there mean the
environment is deployed, and it offers to regenerate `env/<env>/`, connect it,
and run a plan so you can see whether your checkout matches what is running. No
state means a genuinely new project, and it stays quiet.

    meroku sync [env]

does the same on demand and reports whatever it finds. The automatic path asks
before writing anything, and a refusal is not remembered — it asks again next
time. `meroku sync` never asks, since running it is the consent, and neither
does a non-interactive shell.

Nothing on this path applies, destroys, or migrates state.

Also fixed: `meroku generate` rendered Terraform through a loader that never ran
migrations, so a config two schema versions behind produced output with defaults
silently substituted for every missing field. And environment discovery matched
any root YAML by filename, offering `Taskfile` as a deployable environment; an
environment is now identified by declaring both `project` and `env`.

### Upgrade actions

Do these before deploying.

1. **Install Go 1.22+ on whatever machine runs `terraform apply`.** The Lambda is
   now built at apply time instead of shipping a prebuilt binary. The apply fails
   with an actionable message if `go` is missing; nothing is substituted for it.

2. **Set `jwks_uri` on every environment with AppSync enabled.** It is required
   and has no default. See "AppSync authorizer" below for why the previous
   behaviour was not safe to keep.

   ```yaml
   pubsub_appsync:
     enabled: true
     jwks_uri: https://your-idp.example.com/.well-known/jwks.json
     jwt_issuer: https://your-idp.example.com/      # optional, enforced when set
     jwt_audience: your-api                          # optional, enforced when set
   ```

3. **Run `meroku migrate all`.** Schema v22 adds `auto_deploy`. Production
   environments get `auto_deploy: false`; every other environment keeps
   auto-deploying. Set it back to `true` in prod if you want push-to-deploy there.

4. **Expect one apply to replace the EventBridge rules.** The single CI rule
   becomes four. There is a brief window during the apply where no rule is
   attached, so deploy triggers fired in that window are lost. Apply during a
   quiet period if that matters.

### Fixed

- **Backend auto-deploy never worked.** Terraform emitted the service key
  `"backend"` while the Lambda looked up the empty string, so every backend ECR
  push failed with `service '' not found in ECS_SERVICE_MAP`. All six test files
  passed throughout, because each side tested its own assumption.

- **Two projects sharing an AWS account cross-deployed each other.** The event
  rule filtered only source and detail-type, and the repository regex ignored the
  project name, so a push to `projectA_service_api` could redeploy project B's
  `api`. Identifier resolution is now pure lookup against Terraform-emitted maps,
  so another project's repository is simply absent and no code path can act on it.

- **The wrong task-definition revision was deployed past `:9`.** ARNs were
  re-sorted as strings after AWS had already sorted them newest-first, so `:9`
  ranked above `:11`.

- **A failed Lambda build deployed a 97-byte text file.** The build error was
  discarded, and a placeholder was written so `archive_file` would not complain.
  The result deployed green and failed every invocation with
  `Runtime.InvalidEntrypoint`.

- **`terraform apply` silently rolled back CI deployments.** ECS services now
  carry `ignore_changes = [task_definition]`: Terraform owns the service's shape,
  CI owns the revision running in it.

- **Slack failures could fail a deployment**, causing EventBridge to retry and
  post duplicates. Notification can no longer fail an invocation, and permanent
  errors are no longer classified as retryable.

- **`context.Context` was accepted by every handler and then discarded** at the
  deployer boundary. It now reaches every AWS SDK call, and retries respect
  cancellation instead of sleeping past the Lambda deadline.

- **Two template conditions never resolved** — `appsync.resolvers` and
  `pubsub_appsync.appsync.auth_lambda` — so `vtl_templates_yaml` and
  `auth_lambda_path` had never reached the module. A false `{{#if}}` is
  indistinguishable from a disabled feature.

- **The AppSync authorizer had no `aws_lambda_permission`**, so AppSync could not
  invoke it at all.

### AppSync authorizer

`JWKS_URI` fell back to a hardcoded third-party endpoint when unset, and
Terraform set only `EXAMPLE_VAR` — so the fallback was always the path taken, and
whoever controlled that tenant could mint tokens every deployment accepted.
Verification was also skipped entirely when `NODE_ENV=development`, trusting a
base64-decoded payload.

Both are removed. `JWKS_URI` is required and fails closed, issuer and audience
are enforced when set, `resolverContext` values are strings as AppSync requires,
and denials carry reason codes with TTLs that distinguish an invalid token from a
JWKS outage.

### Added

- **`auto_deploy` per target** (backend, services, scheduled tasks). CI policy,
  distinct from `enabled`: whether an ECR push, SSM change or S3 env-file write
  may redeploy without anyone asking. Manual deploys ignore it. Absent means
  `true`, so projects predating the field are unaffected.

- **A boundary test spanning Terraform and Go.** A golden fixture of the
  identifiers Terraform emits, asserted against the real resolver, covering
  environment variable names as well as their contents. This is the mechanism
  that keeps the backend bug fixed; corrupting either side fails the build.

- **CI that actually runs it** (`.github/workflows/ci.yml`): build, vet, gofmt
  and `go test -race` for both Go modules, plus `terraform fmt` and `validate`.
  There was previously no workflow running any test.

### Changed

- AWS SDK v1 (end of support July 2025) → v2; Go 1.20 → 1.22.
- Hand-rolled logging → `log/slog`. Attributes are top-level rather than nested
  under `fields`, so CloudWatch Logs Insights queries need updating.
- The Lambda is built for **arm64** and packaged by Terraform.
- Manual-deploy snippets emit `project` so deploys can be scoped per project.
- Removed: `SERVICE_CONFIG` (passed but never read) and the CLI's separate amd64
  Lambda build (produced an artifact nothing deployed).
