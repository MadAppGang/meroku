# AppSync Lambda authorizer

Node.js 20 (ESM) Lambda that authorizes AppSync requests by verifying an
RS256-signed JWT against a JWKS endpoint you control.

Deployed by `modules/appsync/auth_lambda.tf` as
`${project}-${env}-appsync-auth`. Terraform owns the deployment — there is no
`yarn deploy` script.

## Configuration

All configuration comes from environment variables, set by Terraform from the
module's input variables.

| Env var        | Terraform input | Required | Effect                                                                                         |
| -------------- | --------------- | -------- | ---------------------------------------------------------------------------------------------- |
| `JWKS_URI`     | `jwks_uri`      | **Yes**  | HTTPS URL of the JWKS document holding the public signing keys. No default. Missing ⇒ deny all. |
| `JWT_ISSUER`   | `jwt_issuer`    | No       | Expected `iss` claim. Enforced only when set. Comma-separated list allowed.                     |
| `JWT_AUDIENCE` | `jwt_audience`  | No       | Expected `aud` claim. Enforced only when set. Comma-separated list allowed.                     |

```hcl
module "appsync" {
  source  = "./modules/appsync"
  project = "myapp"
  env     = "dev"

  jwks_uri     = "https://<your-idp-host>/.well-known/jwks.json"
  jwt_issuer   = "https://<your-idp-host>/"   # recommended
  jwt_audience = "https://api.example.com/"   # recommended
}
```

`jwks_uri` has no default and is validated to be an `https://` URL. Configuring
AppSync auth is always a deliberate act.

## Behaviour

Response shape (the contract AppSync expects):

```jsonc
// allow
{ "isAuthorized": true,  "resolverContext": { "sub": "...", "hankoId": "..." }, "ttlOverride": 300 }
// deny
{ "isAuthorized": false, "resolverContext": {}, "deniedFields": [], "ttlOverride": 60 }
```

- `authorizationToken` may be a bare JWT or `Bearer <jwt>`.
- Verification is always `jwt.verify(token, jwksPublicKey, { algorithms: ['RS256'], issuer?, audience? })`.
- **`resolverContext` values are always strings.** AppSync rejects non-string
  values. The handler coerces scalars with `String()` and drops objects/arrays
  (so an array-valued `aud` is omitted rather than breaking the response).
  Emitted keys: `sub`, `hankoId`, `iss`, `aud`, `email`, `exp` — each only when
  present and non-empty.
- `hankoId` duplicates `sub` purely for backwards compatibility: the bundled VTL
  templates read `$context.identity.resolverContext.hankoId`. Prefer `sub` in
  new resolvers.

### Caching (`ttlOverride`)

`ttlOverride` tells AppSync how long it may reuse a decision for the same token.

| Outcome                                      | TTL     | Why                                                                            |
| -------------------------------------------- | ------- | ------------------------------------------------------------------------------ |
| Allow                                        | ≤ 300 s | Clamped to the token's remaining lifetime, so a decision never outlives `exp`.  |
| Deny — bad/missing token                     | 60 s    | A flood of replayed bad tokens is not re-verified on every request.             |
| Deny — our fault (missing config, JWKS down) | 10 s    | The token may be valid; recover quickly once the problem is fixed.              |

### Logs

One structured JSON line per denial, on `console.warn` (caller's fault) or
`console.error` (ours). The token and its claims are never logged.

| `reason`                    | Level | Meaning                                                          |
| --------------------------- | ----- | ---------------------------------------------------------------- |
| `missing_token`             | WARN  | No `authorizationToken` on the request.                          |
| `invalid_token`             | WARN  | Bad signature, expired, wrong issuer/audience, unknown `kid`.    |
| `configuration_error`       | ERROR | `JWKS_URI` is not set — **the authorizer is denying everything.** |
| `authorizer_internal_error` | ERROR | JWKS endpoint unreachable, timed out, or returned garbage.       |

CloudWatch Logs Insights:

```
fields @timestamp, reason, errorName, message
| filter component = 'appsync-authorizer' and level = 'ERROR'
```

An `authorizer_internal_error` or `configuration_error` spike means *your*
infrastructure is broken, not that you are under attack. That distinction is the
whole point of separating the reason codes.

### JWKS caching

`jwks-rsa` is configured with `cache: true`, `cacheMaxAge: 10min`,
`cacheMaxEntries: 5`, `rateLimit: 10 req/min` and a 3 s request timeout, and one
client instance is reused across warm invocations. The previous extra `NodeCache`
layer was **removed**: it was redundant with the library's own per-`kid` memoizer
and actively harmful — its 1 hour TTL overrode the library's 10 minute TTL, so a
rotated or revoked signing key stayed trusted for up to an hour.

## Security rationale

Two defects made this authorizer worse than no authorizer at all. Do not
reintroduce them.

### No default JWKS endpoint (A1)

The old code fell back to a hardcoded third-party Hanko tenant URL when
`JWKS_URI` was unset, and Terraform never set `JWKS_URI` — it only passed
`EXAMPLE_VAR = "example_value"`. Every deployment therefore validated tokens
against a stranger's identity provider. Whoever controls that tenant could mint
tokens this API accepts, for any deployment of this module, forever.

`JWKS_URI` is now required and there is no fallback. Missing config denies every
request and logs `configuration_error`. **Never add a default issuer.** Doing so
delegates authentication of every user to whoever owns that hostname.

### No verification bypass (A2)

The old code checked `NODE_ENV === 'development'` and, if set, base64-decoded the
token without verifying anything and returned `isAuthorized: true`. Any string
that looked like a JWT authenticated as any user. `NODE_ENV` is trivially set by
a Lambda configuration change or by anything holding
`lambda:UpdateFunctionConfiguration`, so this was a one-API-call full auth bypass
sitting in production code.

The bypass was **deleted**, not made configurable. A flag that can turn signature
verification off is a liability regardless of how it is named or defaulted: it
survives in the artifact, and the only thing standing between it and production
is an environment variable. Local testing does not need it — `index.test.mjs`
generates its own keypair and serves a JWKS over loopback, which exercises the
real verification path instead of skipping it.

### Also enforced

- **Algorithm pinning** — `algorithms: ['RS256']` rejects `alg: none` and HMAC
  confusion (a token signed with HS256 using the public key as the secret).
- **Issuer / audience** (A3) — a valid signature is not enough. Without these,
  any token minted by the same identity provider for any other application is
  accepted. Set both whenever your IdP serves more than one issuer or API.

## Packaging

`terraform apply` runs `yarn install --immutable`, stages **only** `index.mjs`,
`package.json` and `node_modules` into `modules/appsync/.build/auth_lambda/`, and
zips that directory to `modules/appsync/.build/auth_lambda.zip`.

The build directory is outside this source tree on purpose: the old build zipped
the source directory in place, shipping `yarn.lock`, `.yarnrc.yml`, `README.md`,
`index.test.mjs` and the previous zip into the artifact, and wrote the zip back
into the directory it was zipping. The build directory self-ignores from git
(`.build/.gitignore` contains `*`).

Rebuilds are deterministic: `archive_file` normalises timestamps, so an unchanged
source tree produces a byte-identical zip and no Lambda redeployment. The rebuild
triggers hash **every** top-level file in this directory plus the build recipe,
and force a rebuild when the staged directory is missing (fresh CI clone).

## Tests

```bash
yarn install
node --test        # or: yarn test
```

The suite covers: missing token, missing/blank `JWKS_URI`, expired token, wrong
issuer, wrong audience, valid token (including the string-only `resolverContext`
invariant), unknown `kid`, missing `kid`, foreign signing key, HS256 algorithm
confusion, `Bearer` prefix handling, and JWKS fetch failure (HTTP 500 and
malformed JSON).

Tests never reach the public internet: an RSA keypair is generated in-process and
the JWKS document is served from a throwaway HTTP server on `127.0.0.1`, so the
real `jwks-rsa` fetch path is exercised offline.
