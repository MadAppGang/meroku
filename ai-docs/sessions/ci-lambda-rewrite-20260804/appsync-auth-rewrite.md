# AppSync Lambda authorizer rewrite

Scope: `modules/appsync/auth_lambda/`, `modules/appsync/auth_lambda.tf`,
`modules/appsync/variables.tf`. Nothing under `modules/workloads/` was read or
touched.

## Why

The authorizer as shipped was worse than having no authorizer. Two of the seven
defects were exploitable by anyone, not just by an insider.

## What changed, defect by defect

### A1 — hardcoded third-party identity provider (critical)

`index.mjs` fell back to
`https://<uuid>.hanko.io/.well-known/jwks.json` when `JWKS_URI` was unset, and
`auth_lambda.tf` never set `JWKS_URI` (it passed `EXAMPLE_VAR = "example_value"`).
Every deployment of this module therefore validated JWTs against a stranger's
Hanko tenant. Whoever controls that tenant could mint tokens accepted by every
AppSync API built from this repo.

Fix: the fallback is gone. `JWKS_URI` is read per invocation; when it is missing
or blank the handler logs `reason: "configuration_error"` at ERROR and denies.
There is no default issuer and no code path that picks one.

### A2 — `NODE_ENV=development` verification bypass (critical)

The old handler checked `process.env.NODE_ENV === 'development'` and, when set,
base64-decoded the token with a hand-rolled `parseJWT` and returned
`isAuthorized: true` for anything shaped like a JWT. Setting one environment
variable (a single `lambda:UpdateFunctionConfiguration` call) turned the API into
an open door with attacker-chosen `sub`.

Fix: deleted outright, along with `parseJWT`. No replacement escape hatch was
added. The reasoning is in `auth_lambda/README.md`: a switch that disables
signature verification is a liability regardless of naming or defaults, because
it ships inside the artifact and only an env var stands between it and
production. Local testing does not need it — the test suite generates its own
keypair and serves JWKS over loopback, exercising the real verification path.

### A3 — issuer/audience never checked

`jwt.verify` validated only the signature, so any token minted by that JWKS for
any other application was accepted.

Fix: optional `JWT_ISSUER` / `JWT_AUDIENCE` env vars, enforced when set, both
accepting a comma-separated list. `algorithms: ['RS256']` is retained (it is what
blocks `alg: none` and HS256-with-the-public-key confusion — both now have
regression tests).

### A4 — Terraform passed no real configuration

`EXAMPLE_VAR = "example_value"` replaced with `JWKS_URI` / `JWT_ISSUER` /
`JWT_AUDIENCE`, sourced from three new module inputs in `variables.tf`:

- `jwks_uri` — **required** (no default, `nullable = false`), validated to match
  `^https://[^ ]+$`. Plaintext http would let a network attacker swap the signing
  keys, and an unset value would silently disable authentication, so both fail at
  plan time.
- `jwt_issuer`, `jwt_audience` — optional, default `""`, omitted from the Lambda
  environment entirely when empty.

Also set `timeout = 10` on the function (AppSync's authorizer budget; the JWKS
fetch gives up at 3 s so the handler always answers deliberately rather than
being killed mid-flight).

### A5 — the build zipped the entire source directory into itself

`source_dir = local.auth_lambda` swept `node_modules`, `yarn.lock`,
`.yarnrc.yml`, `README.md` and `index.test.mjs` into the artifact, and
`output_path` wrote the zip back into the directory being zipped.

Fix, keeping the `null_resource` + `filemd5` trigger pattern:

- A staging build: `yarn install --immutable`, then copy **only** `index.mjs`,
  `package.json` and `node_modules` into `modules/appsync/.build/auth_lambda/`.
- `archive_file` zips the staging dir to `modules/appsync/.build/auth_lambda.zip`
  — outside the source tree.
- The build dir self-ignores from git: the recipe writes `.build/.gitignore`
  containing `*`, so no repo-root `.gitignore` change was needed and
  `git status` stays clean (verified).
- Triggers now cover **every** top-level file in the source dir via
  `fileset(local.auth_lambda, "*")` (which includes dotfiles and excludes
  directories — verified with `terraform console`), plus an md5 of the build
  recipe itself, plus a `fileexists` probe on the staged handler so a fresh CI
  clone rebuilds instead of failing or shipping a stale artifact.
- `yarn install --immutable` when `yarn.lock` exists, plain `yarn install`
  otherwise, because `auth_lambda_path` lets callers supply their own source dir.

Bug caught during verification: `path.module` is relative, so the original recipe
resolved `$stage` against the source dir after `cd "$src"` and silently staged
nothing (`cp: ./.build/auth_lambda: Not a directory`). The recipe now resolves
`$stage` to an absolute path before changing directory. Found by actually running
the rendered recipe, not by reading it.

### A6 — dead `deploy` script

`package.json` hardcoded `appSyncAuthoriser-dev`, which never matched
Terraform's `${project}-${env}-appsync-auth`. Removed; replaced with
`"test": "node --test"`. Terraform owns deployment. `node-cache` dropped from
dependencies and `yarn.lock` regenerated (17 lines removed: `node-cache`,
`clone`).

### A7 — every denial identical, no TTL, errors indistinguishable

Denials returned `{ isAuthorized: false, deniedFields: [] }` with no
`ttlOverride`, so a replayed bad token was re-verified on every request, and a
JWKS outage looked exactly like an attack in the logs.

Fix:

| Outcome                                      | `ttlOverride` | Log level / `reason`                       |
| -------------------------------------------- | ------------- | ------------------------------------------ |
| Allow                                        | ≤ 300 s, clamped to the token's remaining lifetime | none |
| Deny — no token                              | 60 s          | WARN `missing_token`                       |
| Deny — bad/expired/wrong iss/aud/unknown kid | 60 s          | WARN `invalid_token`                       |
| Deny — `JWKS_URI` unset                      | 10 s          | ERROR `configuration_error`                |
| Deny — JWKS unreachable/timeout/garbage      | 10 s          | ERROR `authorizer_internal_error`          |

Errors are classified by name: `JsonWebTokenError`, `TokenExpiredError`,
`NotBeforeError`, `SigningKeyNotFoundError` and the internal `InvalidTokenError`
mean the caller's token is bad; everything else (`JwksError`, `SyntaxError`,
socket errors, rate limiting) means the authorizer failed and gets the short TTL,
because the token may well have been valid. Logs are single-line JSON with a
stable `component` field; the token and its claims are never logged.

## Other changes in the touched files

- **`resolverContext` is now string-only.** The old code put a boolean in
  `naiveAuth`, which AppSync rejects. Values are coerced with `String()`, and
  objects/arrays (e.g. an array-valued `aud`) are dropped rather than emitted.
  A test asserts the invariant over every key. `hankoId` is retained as a
  duplicate of `sub` because `modules/appsync/vtl_templates.yaml:29` reads
  `$context.identity.resolverContext.hankoId`; `sub`, `iss`, `aud`, `email` and
  `exp` were added.
- **Missing-token path returned an HTTP shape** (`{ statusCode: 401, body }`),
  which is not the AppSync contract at all. Now a proper deny.
- **`NodeCache` layer removed.** `jwks-rsa` already memoizes per `kid`
  (`cache: true`, `cacheMaxAge`, `cacheMaxEntries`). The extra layer was not just
  redundant: its 1 hour TTL overrode the library's 10 minute TTL, so a rotated or
  revoked signing key stayed trusted for up to an hour. Rate limiting
  (10 req/min) and a 3 s request timeout added; the client is memoized per JWKS
  URI across warm invocations.
- **`Bearer ` prefix accepted.** AppSync passes the raw `Authorization` header
  value, which is usually `Bearer <jwt>`.
- **Added `aws_lambda_permission.appsync_invoke`** (scope call-out below).

## Scope call-out

`aws_lambda_permission` allowing `appsync.amazonaws.com` to invoke the function
was missing from the module entirely. AppSync cannot call a Lambda authorizer
without it, so the authorizer could never have worked as checked in. It was added
in `auth_lambda.tf` (an in-scope file) and references
`aws_appsync_graphql_api.pubsub.arn` from `main.tf` read-only. Revert it if it is
considered out of scope, but the authorizer is dead without it.

## Follow-up required outside this scope

`jwks_uri` is a required input with no default, so any environment with
`pubsub_appsync.enabled: true` will now fail at plan with "No value for required
variable" until the generator passes it. `env/main.hbs:854-855` currently passes
only `auth_lambda_path`. Needed (not done here — outside the allowed paths):

1. `env/main.hbs` — pass `jwks_uri`, and `jwt_issuer` / `jwt_audience` when set.
2. `app/model.go:54` — the `AppSync` struct needs the corresponding YAML fields
   (`AuthLambda bool` is currently the only one), plus a schema migration and web
   UI surface if the field should be editable there.

Failing loudly at plan time was chosen over a silent default: an authorizer with
no configured issuer must never be deployable by accident.

## Verification performed

- `node --test` in `modules/appsync/auth_lambda/`: **17/17 pass**, ~270 ms.
  Covers the required cases (missing token, missing `JWKS_URI`, expired, wrong
  issuer, wrong audience, valid token, JWKS fetch failure) plus blank
  `JWKS_URI`, non-JWT input, missing `kid`, unknown `kid`, foreign signing key,
  HS256 algorithm confusion, `Bearer` prefix, malformed JWKS JSON, and the
  string-only `resolverContext` invariant. No network: an RS256 keypair is
  generated in-process and JWKS is served from a throwaway `127.0.0.1` server, so
  the real `jwks-rsa` fetch path runs offline.
- `terraform fmt -check auth_lambda.tf variables.tf` → exit 0.
  `terraform fmt -check .` → exit 3, from a pre-existing trailing space in
  `main.tf:68`, a file outside this task's allowed paths. Left alone.
- `terraform validate` on a scratch copy of the module → Success.
- Build recipe executed end-to-end from `terraform console` output. Staged tree
  is exactly `index.mjs`, `package.json`, `node_modules` (25 packages). The
  produced zip is 932 KB and contains no `yarn.lock`, `.yarnrc.yml`, source
  `README.md`, `index.test.mjs` or nested zip.
- Rebuild determinism: full rebuild + re-archive produced an identical
  `output_base64sha256` (`CgYV5X94ieXfhb4OvyIOxmbUrkKw48fcIxAGnTb99AY=`), so
  rebuilds do not cause spurious Lambda redeployments.
- `git status` stays clean with `modules/appsync/.build/` present (self-ignored).

## Invariants for future edits

1. No default JWKS URI, ever. Every deployment names its own issuer.
2. No code path returns `isAuthorized: true` without a successful `jwt.verify`.
3. `algorithms: ['RS256']` stays pinned.
4. `resolverContext` values stay strings.
5. Denials keep a `ttlOverride`, and "our fault" stays distinguishable from
   "their token is bad" in the logs.
