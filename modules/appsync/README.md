# AppSync module

A GraphQL API with pub/sub subscriptions, and a per-environment choice of how
callers are authenticated.

## Choosing a mode

`auth_mode` is one of `cognito`, `oidc` or `lambda`. What each one actually
verifies, and what it costs:

| | `cognito`<br>`AMAZON_COGNITO_USER_POOLS` | `oidc`<br>`OPENID_CONNECT` | `lambda`<br>`AWS_LAMBDA` |
|---|---|---|---|
| Signature | Yes, by AWS | Yes, by AWS, against the configured issuer's JWKS | Yes, in the authorizer, `RS256` only |
| Issuer (`iss`) | Implicit — the pool is named directly | **Not compared when OIDC is the only mode.** See below | Yes, when `jwt_issuer` is set |
| Audience | `cognito_app_id_client_regex`, matched against `aud` (ID token) or `client_id` (access token). **Unset accepts every app client in the pool** | `oidc_client_id`, matched against `aud` falling back to `azp`. Unset accepts any audience from that issuer | Yes, when `jwt_audience` is set |
| Several audiences | Yes — pipe-separated, `1F4G9H\|1J6L4B` | Yes — pipe-separated, `1F4G9H\|1J6L4B` | Yes — comma-separated |
| Any other claim (`role`, `tenant_id`, `scope`) | No | No | **Yes — `required_claims`** |
| Runs a Lambda | No | No | Yes, on every uncached request |
| Cold starts | None | None | Yes |
| Cost | AppSync request pricing only | AppSync request pricing only | Plus Lambda invocations and duration |

Read it this way:

- **Needing several audiences does not select `lambda`.** Both native modes take
  a pipe-separated list of client identifiers.
- **Needing to check a claim other than issuer and audience does select
  `lambda`.** That is the one thing neither native mode can do.
- If this environment already has a Cognito user pool, `cognito` is the natural
  choice and costs nothing to run.
- For an external provider that publishes an OIDC discovery document, `oidc` is
  the natural choice — again with no Lambda.

An absent `auth_mode` means `lambda`, because that is what this module hardcoded
before the setting existed.

## Issuer validation in single-mode OIDC

From the AppSync
[authorization and authentication docs](https://docs.aws.amazon.com/appsync/latest/devguide/security-authz.html):

> If an API is configured with multiple authorization types, AWS AppSync
> validates the issuer (iss claim) present in the JWT token from request headers
> by comparing it against the issuer URL specified in the API configuration.
> However, when an API is configured with only OPENID_CONNECT authorization, AWS
> AppSync skips this issuer URL validation step.

So on an API whose only authorization type is `OPENID_CONNECT`, the `iss` claim
is not compared against `oidc_issuer`.

What still holds: AppSync discovers the signing keys from **that** issuer, so a
token signed by an unrelated provider fails signature verification and is
rejected. This is not an open door.

What does not hold: `iss` itself is not asserted. If your provider mints tokens
carrying different `iss` values from the same signing keys, those are not
rejected on that basis.

Do not work around this by attaching a second authorization type to trip the
multi-mode path. Adding an API key to gain a stricter issuer check would trade a
narrow gap for an unauthenticated bypass — the exact mistake this module already
made once. If your threat model needs `iss` asserted, use `auth_mode = "lambda"`,
whose authorizer enforces `jwt_issuer` directly.

## The API key

`api_key_enabled` defaults to `false`.

An API key is an unattributable bearer credential that **bypasses `auth_mode`
entirely**: whoever holds it reaches every resolver without presenting a token,
so no issuer, audience, signature or claim check applies to them. This module
used to create one for every deployment and export it, which made the authorizer
decorative.

Enable it only for an API that is genuinely public, or for the length of a
migration, and shorten `api_key_expiration_days` when you do.

The expiry is derived from a rotation timestamp held in state
(`time_rotating.api_key`), not from `timestamp()`. The old expression
re-evaluated on every plan and proposed a change on every apply — permanent
drift that trains people to ignore diffs. Terraform now extends the expiry in
place once the key is halfway through its life.

## Examples

Cognito, accepting only two of the pool's app clients:

```hcl
module "appsync" {
  source  = "./modules/appsync"
  project = "myapp"
  env     = "dev"

  auth_mode                   = "cognito"
  cognito_user_pool_id        = module.cognito.user_pool_id
  cognito_app_id_client_regex = "1F4G9H|1J6L4B"
}
```

OIDC:

```hcl
module "appsync" {
  source  = "./modules/appsync"
  project = "myapp"
  env     = "dev"

  auth_mode      = "oidc"
  oidc_issuer    = "https://your-tenant.example.com"
  oidc_client_id = "1F4G9H|1J6L4B"
}
```

Lambda authorizer with a claim policy — the case the native modes cannot express:

```hcl
module "appsync" {
  source  = "./modules/appsync"
  project = "myapp"
  env     = "dev"

  auth_mode    = "lambda"
  jwks_uri     = "https://your-tenant.example.com/.well-known/jwks.json"
  jwt_issuer   = "https://your-tenant.example.com"
  jwt_audience = "https://api.example.com/"

  required_claims = {
    tenant_id = []                 # must be present
    role      = ["admin", "ops"]   # must be one of these
  }
}
```

See `auth_lambda/README.md` for the authorizer's behaviour, logging and security
rationale.

## Outputs

| Output | Notes |
|---|---|
| `api_url` | GraphQL endpoint |
| `api_id` | AppSync API id |
| `auth_mode` | The mode actually deployed |
| `api_key` | **Null unless `api_key_enabled` is true.** Sensitive |

## What is created per mode

`cognito` and `oidc` create no Lambda, no IAM execution role, no CloudWatch log
group for an authorizer and run no `yarn install` during `terraform apply`. Only
`lambda` mode builds and deploys `auth_lambda/`.
