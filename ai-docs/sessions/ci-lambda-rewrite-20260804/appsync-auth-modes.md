# AppSync authorization modes

Scope: `modules/appsync/`, `modules/cognito/` (outputs only, plus two `terraform
fmt` fixes), `env/main.hbs`, `app/{model,migrations,validation,raymond}.go`,
`web/src/types/yamlConfig.ts`, `web/src/utils/hiddenComponentsNodes.ts`, docs.

## Why

`modules/appsync/main.tf` hardcoded `authentication_type = "AWS_LAMBDA"` and
unconditionally attached a second provider:

```hcl
additional_authentication_provider { authentication_type = "API_KEY" }
resource "aws_appsync_api_key" "pubsub" { expires = timeadd(timestamp(), "8760h") }
```

Three faults:

1. **The API key defeated the authorizer.** Anyone holding it skipped JWT
   verification entirely, so every hardening on the Lambda path was decorative.
   `output.tf` exported it.
2. **`timestamp()` in `expires`** re-planned on every apply — permanent drift.
3. **The mode was not configurable**, and `modules/cognito/` (user pool,
   web/mobile/dashboard clients, groups) was unwired: it had **no `outputs.tf`**,
   so nothing could reference the pool.

Meanwhile AppSync natively supports `AMAZON_COGNITO_USER_POOLS` and
`OPENID_CONNECT`, where AWS validates the token itself.

## What it is now

`pubsub_appsync.auth_mode` is `cognito`, `oidc` or `lambda`, per environment.

| | `cognito` | `oidc` | `lambda` |
|---|---|---|---|
| AppSync type | `AMAZON_COGNITO_USER_POOLS` | `OPENID_CONNECT` | `AWS_LAMBDA` |
| Signature | AWS | AWS, against the configured issuer's JWKS | authorizer, `RS256` only |
| Issuer (`iss`) | implicit — pool named directly | **not compared in single-mode OIDC** | `jwt_issuer` |
| Audience | `cognito_app_id_client_regex` (`aud` in an ID token, `client_id` in an access token) | `oidc_client_id` (`aud`, then `azp`) | `jwt_audience` |
| Several audiences | yes, `1F4G9H\|1J6L4B` | yes, `1F4G9H\|1J6L4B` | yes, comma-separated |
| Any other claim | no | no | **`required_claims`** |
| Lambda on the request path | no | no | yes |

The two shaping facts: **needing several audiences does not select `lambda`**
(both native modes take a pipe-separated list), and **needing to check any other
claim does** — that is the only thing neither native mode can do.

- `cognito` requires `cognito.enabled` in the same environment. `modules/cognito`
  gained `outputs.tf` (pool id/ARN/endpoint/issuer/domain and the three client
  ids). The dashboard client's `client_secret` is deliberately **not** exported:
  nothing consumes it and an output lands in plaintext state.
- `oidc` takes `oidc_issuer` and optional `oidc_client_id`. No Passflow URL or
  tenant format is baked in anywhere — issuer and audience are configuration,
  exactly as `lambda` mode already treated `jwks_uri`.
- `lambda` keeps everything previously fixed: required `JWKS_URI`, enforced
  issuer/audience, fail-closed, no dev bypass, the `aws_lambda_permission`. In
  `cognito` and `oidc` mode every authorizer resource is `count = 0`, so there is
  no function, no IAM role, and no `yarn install` during apply.
- `api_key_enabled` defaults to `false`. `output.tf` now exports
  `one(aws_appsync_api_key.pubsub[*].key)`, which is null when there is no key.
- The expiry is derived from `time_rotating.api_key.rfc3339` (held in state),
  rotating at half of `api_key_expiration_days`. Terraform extends the expiry in
  place before it lapses.

### Claim verification (`required_claims`)

Claim name to accepted values; an empty list means "must be present". Reaches the
function as `REQUIRED_CLAIMS`, a `jsonencode`d `map(list(string))`.

- Checked **after** `jwt.verify`, so it can only subtract acceptance. A forged
  token still fails as `invalid_token`.
- Array-valued claims match on any entry. Values compared as strings. Absent,
  `null`, `""`, `[]` and object-valued claims all count as missing — fail closed.
- A present-but-malformed policy **denies everything** and logs
  `configuration_error`. It is never read as "no policy": this module has already
  shipped two checks that silently did nothing.
- Denials get their own reason, `claim_denied`, with `claim` and `detail`
  (`missing` / `not_allowed`) — distinguishable from `invalid_token` (forged) and
  `authorizer_internal_error` (JWKS outage). The claim name is configuration and
  is logged; the value never is.
- Verified claims are added to `resolverContext` so resolvers need not re-decode.
- Refused at generate time in any non-lambda mode, and by a module precondition.
  Believing a rule is enforced when nothing reads it is worse than not asking.
- **Not for identity.** `sub` names an individual, so pinning it to fixed values
  is a user allowlist in Terraform, not authorization — meroku refuses it and
  points at `resolverContext.sub`. Requiring `sub` to be present is fine.

### The Cognito audience gap

`user_pool_config` had no `app_id_client_regex`, so a token minted for **any**
app client in the pool was accepted — and `modules/cognito` creates three. Same
class as the API key: a credential the operator did not intend to grant reaching
the API. It is now configuration, taken verbatim, left null when unset, with the
docs stating plainly that unset accepts every client.

### Single-mode OIDC and `iss`

From the [AppSync authorization
docs](https://docs.aws.amazon.com/appsync/latest/devguide/security-authz.html):

> If an API is configured with multiple authorization types, AWS AppSync
> validates the issuer (iss claim) present in the JWT token from request headers
> by comparing it against the issuer URL specified in the API configuration.
> However, when an API is configured with only OPENID_CONNECT authorization, AWS
> AppSync skips this issuer URL validation step.

Stated in the `oidc_issuer` variable description, in `modules/appsync/README.md`
and in the comparison tables. Accurately: the signature is still verified against
**that** issuer's JWKS, so a token from an unrelated provider is rejected — this
is not an open door. But `iss` itself is not asserted. The docs explicitly say
not to work around it by attaching a second authorization type; adding an API key
to gain a stricter issuer check would trade a narrow gap for an unauthenticated
bypass. The honest answer is `lambda` mode, which enforces `jwt_issuer`.

## Migration: schema v23

`auth_mode` is inferred as `lambda` for every existing config — including where
`auth_lambda: false`, because that flag only ever chose whose authorizer *source*
was packaged, never whether a Lambda authorizer ran. `authentication_type` was
hardcoded, so anything else would change what the next apply deploys.

### The call I made, flagged so it can be overridden

**`api_key_enabled` is written `true` for environments where AppSync is
enabled**, and `false` where it is disabled.

An enabled environment has a live API key in AWS right now, because the module
created one for every deployment. Writing `false` would delete that key on the
next apply and take down any client holding it. Breaking a working API to close a
weakness nobody reported that day is a worse failure than the weakness. So the
migration preserves the credential and prints a loud notice: what the key is,
that it bypasses whatever `auth_mode` verifies, that new environments default to
`false`, and the exact two lines to turn it off — with
`aws appsync list-api-keys --api-id <id>` to check first.

"Default off" holds for every **new** environment (`createEnv` sets `false`) and
for every environment with no key to lose. If you would rather existing
environments be migrated to `false` and take the breakage, that is a one-line
change in `migrateToV23`.

Mode-specific optional fields (`oidc_issuer`, `oidc_client_id`,
`cognito_app_id_client_regex`, `required_claims`) are **not** written by the
migration. Per CLAUDE.md, optional fields with fallbacks are the `default`
helper's job; a migration writes core policy fields that should always be
explicit. An empty `oidc_issuer` in every lambda-mode config is noise nothing
reads. This is also a decision that can be reversed cheaply.

## Web UI

`web/src/types/yamlConfig.ts` carries all the new fields with the caveats inline.

**AppSync still has no editable panel.** It appears only as a read-only node from
`hiddenComponentsNodes.ts`, and `configMerge.ts` already lists `pubsub_appsync`
among the sections preserved verbatim on save precisely because no editor
surfaces them. I did not invent a panel. I did add `authMode` and
`apiKeyEnabled` to the read-only node's `configProperties`: the canvas is the
only place a reviewer would notice an API key is attached.

## Files

| File | Change |
|---|---|
| `modules/appsync/main.tf` | `local.authentication_type`; dynamic `lambda_authorizer_config` / `user_pool_config` / `openid_connect_config`; API key + `time_rotating` behind `api_key_enabled`; four preconditions |
| `modules/appsync/variables.tf` | `auth_mode`, `cognito_*`, `oidc_*`, `api_key_*`, `required_claims`; `jwks_uri` no longer unconditionally required |
| `modules/appsync/auth_lambda.tf` | every authorizer resource `count`-gated on lambda mode; `REQUIRED_CLAIMS` env var |
| `modules/appsync/auth_lambda/index.mjs` | `parseRequiredClaims`, `assertClaims`, `ClaimDeniedError`, `claim_denied` reason, claims in `resolverContext` |
| `modules/appsync/auth_lambda/index.test.mjs` | 9 new tests (26 total) |
| `modules/appsync/output.tf` | `api_key` is null when disabled; added `api_id`, `auth_mode` |
| `modules/appsync/README.md` | new — mode comparison, the OIDC `iss` note, API key rationale |
| `modules/appsync/auth_lambda/README.md` | mode table, `REQUIRED_CLAIMS`, `claim_denied`, A5 |
| `modules/cognito/outputs.tf` | new — pool id/ARN/endpoint/issuer/domain, three client ids |
| `modules/cognito/{main,variable}.tf` | `terraform fmt` only (pre-existing, 2 lines) |
| `env/main.hbs` | per-mode arguments, gated on `(eq (default … "lambda") …)` |
| `app/model.go` | mode constants, `normalizeAppSyncAuthMode`, 6 new `AppSync` fields, `createEnv` defaults |
| `app/migrations.go` | `migrateToV23`, `CurrentSchemaVersion = 23` |
| `app/validation.go` | `validateAppSyncAuthFull` dispatching per mode; `required_claims` rules; `yamlSubMap`, `yamlClaimMap` |
| `app/raymond.go` | `stringListMap` helper (sorted, for reproducible generation) |
| `app/appsync_auth_modes_test.go` | new |
| `app/migrations_test.go` | v23 tests |
| `web/src/types/yamlConfig.ts`, `hiddenComponentsNodes.ts` | new fields |
| `docs/ENVIRONMENT_CONFIGURATION.md`, `ai_docs/YAML_SPECIFICATION.md`, `ai_docs/MIGRATIONS.md` | all three modes |

## Verification

### Go

```
$ go build ./... && go vet ./... && gofmt -l .
BUILD OK
VET OK
GOFMT CLEAN

$ go test ./...
ok  	madappgang.com/meroku	0.412s
ok  	madappgang.com/meroku/pricing	(cached)
```

### Authorizer

```
$ node --test
1..26
# tests 26
# pass 26
# fail 0
```

### Terraform

```
$ cd modules/appsync && terraform fmt -check -recursive . && terraform validate
FMT OK
Success! The configuration is valid.

$ cd modules/workloads && terraform fmt -check -recursive . && terraform validate
WORKLOADS FMT OK
Success! The configuration is valid, but there were some validation warnings
```

The workloads warnings are pre-existing `data.aws_region.current.name`
deprecations, raised only because a standalone `terraform init` resolves an
unconstrained `aws` v6; the generated environment pins `~> 5.0`, where `.name` is
current. `modules/cognito` and `modules/workloads` already use `.name`
throughout, and `modules/appsync` follows them.

### Web

```
$ npx tsc --noEmit -p tsconfig.app.json
TSC EXIT: 0

$ bun test
 5 pass
 0 fail
Ran 5 tests across 1 file.
```

### `aws_appsync_graphql_api`, per mode, API key off

Real `terraform plan` against the module, fake credentials, no AWS calls:

```
================= auth_mode = cognito =================
  + resource "aws_appsync_graphql_api" "pubsub" {
      + authentication_type  = "AMAZON_COGNITO_USER_POOLS"
      + user_pool_config {
          + app_id_client_regex = "1F4G9H|1J6L4B"
          + aws_region          = "us-east-1"
          + default_action      = "ALLOW"
          + user_pool_id        = "us-east-1_EXAMPLE00"
        }
    }
--- API key resources in this plan:
                                            (none)

================= auth_mode = oidc =================
  + resource "aws_appsync_graphql_api" "pubsub" {
      + authentication_type  = "OPENID_CONNECT"
      + openid_connect_config {
          + client_id = "1F4G9H|1J6L4B"
          + issuer    = "https://your-tenant.example.com"
        }
    }
--- API key resources in this plan:
                                            (none)

================= auth_mode = lambda =================
  + resource "aws_appsync_graphql_api" "pubsub" {
      + authentication_type  = "AWS_LAMBDA"
      + lambda_authorizer_config {
          + authorizer_result_ttl_in_seconds = 300
          + authorizer_uri                   = (known after apply)
        }
    }
--- API key resources in this plan:
# module.appsync.aws_lambda_function.function[0] will be created
# module.appsync.aws_lambda_permission.appsync_invoke[0] will be created
```

No `aws_appsync_api_key` and no `time_rotating` in any of the three. The Lambda
exists only in lambda mode.

With `api_key_enabled = true`:

```
  # module.appsync.aws_appsync_api_key.pubsub[0] will be created
      + description = "testproject-dev pubsub API key. Bypasses auth_mode = cognito."
  # module.appsync.time_rotating.api_key[0] will be created
      + rotation_days    = 182
      + additional_authentication_provider {
          + authentication_type = "API_KEY"
        }
```

### `timestamp()` drift, before and after

Both expressions in one config, applied, then planned again with nothing changed:

```
  # terraform_data.old_expires will be updated in-place
  ~ resource "terraform_data" "old_expires" {
      ~ input  = "2027-08-05T00:52:24Z" -> (known after apply)   # timeadd(timestamp(), "8760h")
    }
Plan: 0 to add, 1 to change, 0 to destroy.
```

The new expression (`timeadd(time_rotating.api_key.rfc3339, …)`) does not appear
in the plan at all — no change proposed.

### Module preconditions, one per mode

```
auth_mode = cognito:  condition = !local.use_cognito_auth || var.cognito_user_pool_id != ""
                      │ local.use_cognito_auth is true
                      │ var.cognito_user_pool_id is ""
auth_mode = oidc:     condition = !local.use_oidc_auth || var.oidc_issuer != ""
auth_mode = lambda:   condition = !local.use_lambda_auth || var.jwks_uri != ""
required_claims in cognito mode:
                      condition = length(var.required_claims) == 0 || local.use_lambda_auth
auth_mode = passflow: auth_mode must be "cognito", "oidc" or "lambda".
```

### Validation, from the real CLI

`meroku generate dev` on a project whose `cognito.enabled` is false:

```
❌ Invalid configuration in dev.yaml:

pubsub_appsync.auth_mode is "cognito" but cognito.enabled is false

Cognito mode points AppSync at the user pool this environment creates, so the
pool has to exist. Either enable it in your <env>.yaml:

  cognito:
    enabled: true

or pick a mode that does not need one:

  pubsub_appsync:
    auth_mode: "oidc"     # AWS verifies tokens against your issuer's JWKS
    oidc_issuer: "https://<your-idp-host>"

  pubsub_appsync:
    auth_mode: "lambda"   # runs the bundled authorizer against jwks_uri
    jwks_uri: "https://<your-idp-host>/.well-known/jwks.json"
```

`auth_mode: oidc` with no issuer:

```
pubsub_appsync.oidc_issuer is required when pubsub_appsync.auth_mode is "oidc"

AppSync fetches this issuer's OIDC discovery document and its signing keys, and
accepts every token they validate. There is no default: whoever controls that
URL can mint tokens your API accepts, so meroku will not pick one for you.
…
```

`auth_mode: lambda` with no `jwks_uri` — the message now also steers away from
lambda mode when it is not needed:

```
pubsub_appsync.jwks_uri is required when pubsub_appsync.enabled is true and pubsub_appsync.auth_mode is "lambda"
…
Before you do: lambda mode exists for providers the native modes cannot express,
and it puts a Lambda invocation on the request path. If your provider publishes
an OIDC discovery document, prefer

  pubsub_appsync:
    auth_mode: "oidc"
    oidc_issuer: "https://<your-idp-host>"

and if this environment already has a Cognito user pool, prefer

  pubsub_appsync:
    auth_mode: "cognito"
```

`required_claims` in a mode that cannot enforce it:

```
pubsub_appsync.required_claims is set but pubsub_appsync.auth_mode is "cognito", which cannot enforce it

Only the Lambda authorizer checks claims beyond the issuer and the audience.
AMAZON_COGNITO_USER_POOLS verifies the signature, the issuer and the audience and nothing else, so
these requirements would be silently ignored — you would believe a rule is in
force that is not.
…
If all you need is to accept several audiences, you do not need lambda mode:
  cognito -> cognito_app_id_client_regex: "1F4G9H|1J6L4B"
  oidc    -> oidc_client_id: "1F4G9H|1J6L4B"
```

Pinning `sub`:

```
pubsub_appsync.required_claims must not pin "sub" to a fixed set of values

"sub" identifies the individual caller, so listing accepted values here is a
deployment-wide allowlist of user ids, redeployed on every change — not
authorization.
…
and make per-user decisions in your resolvers, which read the verified "sub"
from $context.identity.resolverContext.sub. Requiring "sub" to merely be
present is fine: sub: []
```

### Migration, end to end

The repo's own `project/dev.yaml` (schema v18) migrated with `meroku migrate`.

AppSync disabled:

```
  → Migrating to v23: Adding AppSync auth_mode and api_key_enabled
    ✓ Added auth_mode = "lambda" (what this module deploys today; nothing changes)
    ✓ Added api_key_enabled = false (AppSync is disabled here, so there is no key to keep)
✓ Successfully migrated to v23
```

Same file with AppSync enabled:

```
    ✓ Added auth_mode = "lambda" (what this module deploys today; nothing changes)
       AppSync now also supports:
         auth_mode: "cognito"  — AWS validates tokens against this environment's user pool
         auth_mode: "oidc"     — AWS validates tokens against your OIDC issuer
       Both drop the authorizer Lambda entirely: no cold start, no invocation cost.
    ✓ Added api_key_enabled = true

    ⚠️  IMPORTANT — this environment has a live AppSync API key, and this
        migration keeps it.
        …
        It is preserved rather than removed because setting it to false here
        would delete the key on your next apply and break any client using it.
        New environments default to false.

        To turn it off once you have confirmed nothing depends on it:

          pubsub_appsync:
            api_key_enabled: false
```

New environment (`createEnv`) serializes `api_key_enabled: false` and
`auth_mode: lambda`; asserted by `TestNewEnvironmentSerializesAPIKeyOff`.

### Generated HCL, end to end

`meroku generate dev` on the repo's sample project, lambda mode with a claim
policy, after `terraform fmt`:

```hcl
module "appsync" {
  source  = "../../infrastructure/modules/appsync"
  project = "instagram"
  env     = "dev"
  auth_mode = "lambda"
  api_key_enabled = false
  auth_lambda_path = "../../custom/appsync/auth_lambda"
  jwks_uri   = "https://your-tenant.example.com/.well-known/jwks.json"
  jwt_issuer = "https://your-tenant.example.com"
  jwt_audience = "your-api-audience"
  required_claims = {
    "role"      = ["admin", "ops"]
    "tenant_id" = []
  }
}
```

Cognito mode emits `cognito_user_pool_id = module.cognito.user_pool_id` and
`cognito_app_id_client_regex`, and no lambda-mode arguments; oidc mode emits
`oidc_issuer` / `oidc_client_id` and neither of the others.

### Binary

`bin/meroku` rebuilt last, after every edit:

```
$ ./bin/meroku migrate
Current schema version: v23

$ strings bin/meroku | grep -c 'auth_mode is "cognito" but cognito.enabled is false'
1
```

All six new user-facing strings checked into the binary: the three per-mode
validation messages, the `required_claims` mode error, the `sub` refusal, and the
v23 API-key notice.

## Not done, and why

- **No AppSync editor panel.** There was none before and the brief said to say so
  rather than invent one. `configMerge.ts` preserves `pubsub_appsync` verbatim on
  save, so the new fields survive a round trip through the web UI untouched.
- **No `terraform apply`.** Every plan here ran with fake credentials against no
  AWS account. The drift fix is demonstrated on an isolated config using the
  `time` provider, which needs no cloud.
- **`data.aws_region.current.name` not migrated to `.region`.** It is deprecated
  only under `aws` v6; environments pin `~> 5.0`, and changing it here would
  leave `modules/cognito` and `modules/workloads` inconsistent. That is a
  repo-wide change, not this one.
- **`modules/cognito` formatting.** `terraform fmt` rewrote two pre-existing
  lines (`"${var.backend_task_role_name}"` → `var.backend_task_role_name`, one
  alignment). Disclosed rather than reverted.
