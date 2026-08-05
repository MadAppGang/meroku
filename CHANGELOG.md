# Changelog

## v3.25.0

Same code as v3.24.0, released from `main`.

v3.24.0 was tagged on the feature branch before it merged. The binaries it
published are correct and identical to these; this release exists so the
published version is one anybody can reach from `main`.

Everything below applies to both.

## v3.24.0

The CI/CD Lambda is rewritten. Its main path — redeploy the backend when a new
image is pushed — had never worked, and two authentication bypasses in the
AppSync authorizer are closed. A fresh checkout of a deployed project now
connects itself instead of reporting that nothing is deployed.

### Choosing how AppSync authenticates

The API hardcoded a Lambda authorizer and attached an API key beside it,
unconditionally. Anyone with that key skipped JWT verification altogether, which
made the authorizer decorative. Authorization is now chosen per environment:

```yaml
pubsub_appsync:
  enabled: true
  auth_mode: cognito        # or: oidc, lambda
  api_key_enabled: false
```

- **`cognito`** — validated by AppSync against the user pool meroku already
  creates. `modules/cognito` gained the `outputs.tf` it never had, which is why
  the pool could not be wired before.
- **`oidc`** — validated by AppSync against a configured issuer. `oidc_client_id`
  is matched against the token's `aud` (or `azp`) claim, and accepts several
  client ids separated by `|`.
- **`lambda`** — the bundled authorizer. Needed only when you must check claims
  beyond issuer and audience; it now supports required-claim policies, failing
  closed with a denial reason distinct from a bad signature or a JWKS outage.

The first two are verified by AWS, so no Lambda is built, deployed or invoked.

Cognito mode previously had **no audience check at all** — it accepted a token
minted for any app client in the pool, and meroku creates three.
`cognito_app_id_client_regex` now filters them; leaving it unset still accepts
all of them, and the docs say so.

The API key defaults off. Existing AppSync environments keep theirs — the
migration writes `api_key_enabled: true` where AppSync is already enabled, since
withdrawing a live credential on upgrade would break its clients.

Note for single-mode OIDC APIs: AppSync skips comparing the `iss` claim against
the configured issuer unless a second authorization mode is present. Signature
verification against that issuer's JWKS still applies. Use `lambda` mode if you
need `iss` asserted.

### Two defects found by rendering a config with Cognito enabled

- `dashboard_callback_urls` was tagged `dashboard_callback_ur_ls` in the Go
  struct — an acronym split by a camelCase conversion. Because the struct both
  reads and writes the file, every load dropped configured URLs and every save
  wrote them back misspelt. Fixed at the tag, with a migration repairing configs
  on disk.
- The `array` template helper panicked on a missing key. All twenty of its call
  sites read optional fields, so any config omitting one crashed generation with
  a Go stack trace. An absent list now renders `[]`.

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

2. **State how AppSync authenticates, on every environment with it enabled.**
   There is no default, and generation refuses to proceed without it. Pick the
   mode that matches your identity provider:

   ```yaml
   # Cognito issues the tokens — AWS verifies them, no Lambda runs
   pubsub_appsync:
     enabled: true
     auth_mode: cognito
     cognito_app_id_client_regex: "1F4G9H|1J6L4B"   # which app clients; unset accepts all

   # An OIDC provider issues them — AWS verifies them, no Lambda runs
   pubsub_appsync:
     enabled: true
     auth_mode: oidc
     oidc_issuer: https://your-idp.example.com
     oidc_client_id: "your-client-id"               # matched against aud / azp

   # Anything else, or you need to check custom claims
   pubsub_appsync:
     enabled: true
     auth_mode: lambda
     jwks_uri: https://your-idp.example.com/.well-known/jwks.json
     jwt_issuer: https://your-idp.example.com/      # optional, enforced when set
     jwt_audience: your-api                          # optional, enforced when set
   ```

3. **Run `meroku migrate all`.** Schema v24. v22 adds `auto_deploy` (production
   environments get `false`; everything else keeps auto-deploying). v23 adds the
   AppSync `auth_mode` and `api_key_enabled`. v24 repairs the misspelt
   `dashboard_callback_urls` key.

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
