# AppSync Lambda authorizer

Node.js 20 (ESM) Lambda that authorizes AppSync requests by verifying an
RS256-signed JWT against a JWKS endpoint you control.

Deployed by `modules/appsync/auth_lambda.tf` as
`${project}-${env}-appsync-auth`, **only when `auth_mode = "lambda"`**.
Terraform owns the deployment — there is no `yarn deploy` script.

## Do you need this authorizer?

`modules/appsync` supports three authorization modes. Two of them are verified by
AWS itself and run no Lambda at all:

| | `cognito`<br>`AMAZON_COGNITO_USER_POOLS` | `oidc`<br>`OPENID_CONNECT` | `lambda`<br>this authorizer |
|---|---|---|---|
| Signature | Yes, by AWS | Yes, by AWS, against the configured issuer's JWKS | Yes, `RS256` only |
| Issuer (`iss`) | Implicit — the pool is named directly | **Not compared when OIDC is the only authorization type** | Yes, when `JWT_ISSUER` is set |
| Audience | `cognito_app_id_client_regex`, matched against `aud` (ID token) or `client_id` (access token). **Unset accepts every app client in the pool** | `oidc_client_id`, matched against `aud` falling back to `azp`. Unset accepts any audience from that issuer | Yes, when `JWT_AUDIENCE` is set |
| Several audiences | Yes — pipe-separated, `1F4G9H\|1J6L4B` | Yes — pipe-separated, `1F4G9H\|1J6L4B` | Yes — comma-separated |
| Any other claim (`role`, `tenant_id`, `scope`) | No | No | **Yes — `REQUIRED_CLAIMS`** |
| Runs a Lambda | No | No | Yes, on every uncached request |
| Cold starts | None | None | Yes |
| Cost | AppSync request pricing only | AppSync request pricing only | Plus Lambda invocations and duration |

**Needing several audiences is not a reason to be here** — both native modes take
a pipe-separated list. **Needing to check a claim beyond issuer and audience
is**, and so is needing `iss` asserted on an OIDC-only API (see
`../README.md`). Otherwise prefer `cognito` when the environment already has a
user pool, or `oidc` when your provider publishes a discovery document.

## Configuration

All configuration comes from environment variables, set by Terraform from the
module's input variables.

| Env var           | Terraform input   | Required | Effect                                                                                          |
| ----------------- | ----------------- | -------- | ----------------------------------------------------------------------------------------------- |
| `JWKS_URI`        | `jwks_uri`        | **Yes**  | HTTPS URL of the JWKS document holding the public signing keys. No default. Missing ⇒ deny all.  |
| `JWT_ISSUER`      | `jwt_issuer`      | No       | Expected `iss` claim. Enforced only when set. Comma-separated list allowed.                      |
| `JWT_AUDIENCE`    | `jwt_audience`    | No       | Expected `aud` claim. Enforced only when set. Comma-separated list allowed.                      |
| `REQUIRED_CLAIMS` | `required_claims` | No       | Claim policy, checked after verification. Present but unparseable ⇒ deny all. See below.         |

```hcl
module "appsync" {
  source  = "./modules/appsync"
  project = "myapp"
  env     = "dev"

  auth_mode    = "lambda"
  jwks_uri     = "https://<your-idp-host>/.well-known/jwks.json"
  jwt_issuer   = "https://<your-idp-host>/"   # recommended
  jwt_audience = "https://api.example.com/"   # recommended

  required_claims = {
    tenant_id = []                 # must be present
    role      = ["admin", "ops"]   # must be one of these
  }
}
```

`jwks_uri` has no default and is validated to be an `https://` URL. Configuring
AppSync auth is always a deliberate act.

## Claim policy (`REQUIRED_CLAIMS`)

The reason to run this authorizer rather than a native mode.

Wire format is a JSON object of claim name to accepted values, produced by
Terraform's `jsonencode` from `map(list(string))`:

```jsonc
{"tenant_id": [], "role": ["admin", "ops"]}
```

- **Empty list** — the claim must be present.
- **Non-empty list** — the claim must hold one of these values.
- **Array-valued claims** (`cognito:groups`, a multi-valued `aud`) match when any
  entry is accepted, which is the useful reading of "the user is in one of these
  roles".
- Values are compared **as strings**: `tier: 2` in a token matches `["2"]`.
- Absent, `null`, `""`, `[]` and object-valued claims all count as **missing**.
  Fail closed.
- Checked **after** `jwt.verify`, so it can only ever remove acceptance, never
  grant it. A forged token still fails as `invalid_token`.
- Every claim named in the policy is added to `resolverContext` once verified
  (array values joined with `,`), so resolvers can read it without re-decoding
  the token.

A policy that is present but malformed — not JSON, not an object, a value that is
not an array of strings, an empty claim name — **denies every request** and logs
`configuration_error`. It is never treated as "no policy". This module has twice
shipped a check that silently did nothing (a hardcoded fallback issuer, then an
API key that bypassed the authorizer); a typo must not become the third.

`meroku` rejects `required_claims` at generate time in any mode other than
`lambda`, and the module carries the same rule as a plan-time precondition,
because believing a rule is enforced when nothing reads it is worse than not
having asked for it.

### Not for identity

This is for **policy** claims — `role`, `scope`, `tenant_id`, a plan tier. It is
not where you check *who* the caller is: `sub` identifies an individual, so a
list of accepted values is a deployment-wide user allowlist redeployed on every
change, not authorization. `meroku` refuses to pin `sub` to fixed values for that
reason (requiring it merely to be present is fine). Per-user decisions belong in
your resolvers, which read the verified `sub` from
`$context.identity.resolverContext.sub`.

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
  present and non-empty — plus every claim named in `REQUIRED_CLAIMS`, with
  array values joined by `,`. A required claim never overwrites one of the fixed
  keys.
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

| `reason`                    | Level | Meaning                                                                                    |
| --------------------------- | ----- | ------------------------------------------------------------------------------------------ |
| `missing_token`             | WARN  | No `authorizationToken` on the request.                                                    |
| `invalid_token`             | WARN  | Bad signature, expired, wrong issuer/audience, unknown `kid`.                              |
| `claim_denied`              | WARN  | Token verified, but `REQUIRED_CLAIMS` not satisfied. Carries `claim` and `detail`.         |
| `configuration_error`       | ERROR | `JWKS_URI` unset or `REQUIRED_CLAIMS` malformed — **the authorizer is denying everything.** |
| `authorizer_internal_error` | ERROR | JWKS endpoint unreachable, timed out, or returned garbage.                                 |

`claim_denied` is deliberately its own reason. "A valid user is not allowed here"
is an access-control signal; "someone is presenting forged tokens" is an attack
signal; "our JWKS endpoint is down" is an outage. A single denial code would
merge all three. `detail` is `missing` or `not_allowed`, and `claim` names the
claim — the claim NAME is configuration, so it is safe to log. The claim's
**value** never is.

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
- **Claim policy** — `REQUIRED_CLAIMS`, above. A malformed policy denies rather
  than being ignored, for the same reason A1 has no fallback issuer: a check that
  silently does nothing is worse than no check, because it is believed.

### The API key is no longer created for you (A5)

`modules/appsync` used to attach an `API_KEY` additional authentication provider
to every deployment and export the key. Everything on this page could be
configured perfectly and still be skipped by anyone holding it. The key is now
opt-in (`api_key_enabled`, default `false`) — but note that existing environments
were migrated to `true` to preserve a key their clients may already hold. Turn it
off deliberately; see `../README.md`.

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

For the claim policy: satisfied policy, absent claim, unaccepted value,
array-valued claim (matching and not), empty/object-valued claims treated as
missing, non-string claim values compared as strings, seven shapes of malformed
`REQUIRED_CLAIMS` each denying with `configuration_error`, unset/empty policy as
a no-op, and a forged token still failing as `invalid_token` rather than being
rescued by a satisfied policy.

Tests never reach the public internet: an RSA keypair is generated in-process and
the JWKS document is served from a throwaway HTTP server on `127.0.0.1`, so the
real `jwks-rsa` fetch path is exercised offline.
