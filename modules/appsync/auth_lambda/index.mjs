/**
 * AppSync Lambda authorizer.
 *
 * Verifies an RS256-signed JWT against a JWKS endpoint and returns the
 * response shape AppSync expects:
 *
 *   { isAuthorized, resolverContext, deniedFields?, ttlOverride? }
 *
 * Security rules this file must keep:
 *   - There is NO default JWKS endpoint. `JWKS_URI` is required; when it is
 *     missing the authorizer denies and logs a configuration error. Falling
 *     back to a built-in issuer would let whoever controls that issuer mint
 *     tokens this API accepts.
 *   - There is NO verification bypass. Every request goes through
 *     `jwt.verify` with `algorithms: ['RS256']` (pinning the algorithm is what
 *     blocks `alg: none` / HMAC confusion attacks).
 *   - `issuer` / `audience` are enforced whenever they are configured.
 *   - `REQUIRED_CLAIMS` is enforced after verification, and a configuration that
 *     cannot be parsed denies everything rather than being ignored. An
 *     unparseable policy that fell through to "no requirements" would silently
 *     drop the check the operator asked for.
 *
 * Every failure path denies. Never add a path that returns
 * `isAuthorized: true` without a successful `jwt.verify`.
 */

import jwt from 'jsonwebtoken';
import jwksClient from 'jwks-rsa';

// --- Tunables ---------------------------------------------------------------

/** Give up on the JWKS endpoint well before the Lambda timeout so we can deny
 *  deliberately (with a ttlOverride) instead of being killed by the runtime. */
const JWKS_REQUEST_TIMEOUT_MS = 3_000;
/** How long a signing key stays cached. Also the upper bound on how long a
 *  revoked/rotated key keeps being trusted, so do not make this large. */
const JWKS_CACHE_MAX_AGE_MS = 600_000; // 10 minutes
const JWKS_CACHE_MAX_ENTRIES = 5;
const JWKS_REQUESTS_PER_MINUTE = 10;

/** Upper bound for how long AppSync may cache a successful authorization.
 *  Also clamped to the token's own remaining lifetime. */
const MAX_AUTH_TTL_SECONDS = 300;
/** Denials for a bad token are cached briefly so a replayed token is not
 *  re-verified on every single request. */
const DENY_TTL_SECONDS = 60;
/** Denials caused by our own problems (missing config, JWKS unreachable) get a
 *  much shorter TTL: the token may well be valid, so recover fast. */
const ERROR_TTL_SECONDS = 10;

// --- Errors -----------------------------------------------------------------

class ConfigurationError extends Error {
  constructor(message) {
    super(message);
    this.name = 'ConfigurationError';
  }
}

class InvalidTokenError extends Error {
  constructor(message) {
    super(message);
    this.name = 'InvalidTokenError';
  }
}

/**
 * A token whose signature, issuer and audience are all fine, but which does not
 * satisfy the configured claim policy.
 *
 * Kept separate from InvalidTokenError so the denial is distinguishable in the
 * logs: "this token is not for you" is a different operational signal from
 * "someone is presenting forged tokens", and both differ from "our JWKS endpoint
 * is down". `claim` and `detail` are safe to log — the claim NAME is
 * configuration, not user data. The claim's value is never logged.
 */
class ClaimDeniedError extends Error {
  constructor(claim, detail) {
    super(`Claim "${claim}" ${detail === 'missing' ? 'is missing' : 'does not hold an accepted value'}`);
    this.name = 'ClaimDeniedError';
    this.claim = claim;
    this.detail = detail;
  }
}

/**
 * Errors that mean "the caller's token is bad" as opposed to "the authorizer
 * could not do its job". Both deny; they are logged differently and get
 * different cache TTLs.
 *
 * `SigningKeyNotFoundError` counts as a bad token: the JWKS was fetched fine,
 * it simply does not contain the `kid` the token claims.
 */
const INVALID_TOKEN_ERRORS = new Set([
  'InvalidTokenError',
  'JsonWebTokenError',
  'TokenExpiredError',
  'NotBeforeError',
  'SigningKeyNotFoundError',
]);

// --- Logging ----------------------------------------------------------------

/**
 * Structured single-line logs so CloudWatch Logs Insights can filter on
 * `reason`. Never log the token or the decoded payload.
 */
function log(level, fields) {
  const line = JSON.stringify({ level, component: 'appsync-authorizer', ...fields });
  if (level === 'ERROR') {
    console.error(line);
  } else {
    console.warn(line);
  }
}

// --- Configuration ----------------------------------------------------------

function parseList(value) {
  if (typeof value !== 'string') return undefined;
  const items = value
    .split(',')
    .map((item) => item.trim())
    .filter((item) => item !== '');
  if (items.length === 0) return undefined;
  return items.length === 1 ? items[0] : items;
}

/**
 * Parses REQUIRED_CLAIMS, the claim policy.
 *
 * Wire format is a JSON object of claim name -> array of accepted values:
 *
 *   {"tenant_id": [], "role": ["admin", "ops"]}
 *
 * An empty array means "the claim must be present"; a non-empty array means
 * "the claim must hold one of these values". Terraform builds this with
 * jsonencode() from a map(list(string)), so the shape is fixed at plan time.
 *
 * Anything that does not fit that shape is a ConfigurationError, which denies
 * every request. That is the point: this module has twice shipped a check that
 * silently did nothing (a hardcoded fallback issuer, then an API key that
 * bypassed the authorizer). A typo in a claim policy must not become a third.
 *
 * @throws {ConfigurationError}
 */
function parseRequiredClaims(raw) {
  if (raw === undefined || raw === null) return [];
  if (typeof raw !== 'string' || raw.trim() === '') return [];

  let parsed;
  try {
    parsed = JSON.parse(raw);
  } catch (error) {
    throw new ConfigurationError(
      `REQUIRED_CLAIMS is not valid JSON (${error.message}). The authorizer denies every request ` +
        'until it is fixed, rather than ignoring the claim policy it was given.',
    );
  }

  if (parsed === null || typeof parsed !== 'object' || Array.isArray(parsed)) {
    throw new ConfigurationError(
      'REQUIRED_CLAIMS must be a JSON object of claim name to array of accepted values, ' +
        'e.g. {"role":["admin"]}.',
    );
  }

  return Object.entries(parsed).map(([claim, accepted]) => {
    if (claim.trim() === '') {
      throw new ConfigurationError('REQUIRED_CLAIMS contains an empty claim name.');
    }
    if (!Array.isArray(accepted)) {
      throw new ConfigurationError(
        `REQUIRED_CLAIMS["${claim}"] must be an array of accepted values ` +
          '(use [] to require only that the claim is present).',
      );
    }
    for (const value of accepted) {
      if (typeof value !== 'string') {
        throw new ConfigurationError(
          `REQUIRED_CLAIMS["${claim}"] must contain only strings; JWT claim values are compared as strings.`,
        );
      }
    }
    return { claim, accepted };
  });
}

/**
 * @throws {ConfigurationError} when JWKS_URI is missing or the claim policy is
 * malformed. Callers must deny.
 */
function readConfig(env) {
  const jwksUri = typeof env.JWKS_URI === 'string' ? env.JWKS_URI.trim() : '';
  if (jwksUri === '') {
    throw new ConfigurationError(
      'JWKS_URI is not set. The authorizer denies every request until it is configured; ' +
        'set the jwks_uri input on the appsync Terraform module.',
    );
  }

  return {
    jwksUri,
    issuer: parseList(env.JWT_ISSUER),
    audience: parseList(env.JWT_AUDIENCE),
    requiredClaims: parseRequiredClaims(env.REQUIRED_CLAIMS),
  };
}

// --- Claim policy -----------------------------------------------------------

/**
 * Values a claim can be compared by. A claim holding an array (`cognito:groups`,
 * a multi-valued `aud`) satisfies the policy if ANY of its entries is accepted,
 * which is the useful reading of "the user is in one of these roles".
 *
 * Objects are not comparable and yield no values, so a claim holding one is
 * treated as unsatisfied rather than as a match.
 */
function comparableValues(value) {
  const scalar = (v) => (v === null || v === undefined || typeof v === 'object' ? null : String(v));

  if (Array.isArray(value)) {
    return value.map(scalar).filter((v) => v !== null && v !== '');
  }
  const single = scalar(value);
  return single === null || single === '' ? [] : [single];
}

/**
 * Enforces the claim policy against an already-verified payload.
 *
 * Runs only after jwt.verify has succeeded, so it can never be the thing that
 * makes an unverified token acceptable — it only ever removes acceptance.
 *
 * @throws {ClaimDeniedError}
 */
function assertClaims(payload, requiredClaims) {
  for (const { claim, accepted } of requiredClaims) {
    const values = comparableValues(payload[claim]);

    // Absent, null, empty string, empty array, or an object: nothing to match
    // against, so the requirement is not met. Fail closed.
    if (values.length === 0) {
      throw new ClaimDeniedError(claim, 'missing');
    }
    if (accepted.length === 0) {
      continue; // presence was the whole requirement
    }
    if (!values.some((value) => accepted.includes(value))) {
      throw new ClaimDeniedError(claim, 'not_allowed');
    }
  }
}

// --- JWKS -------------------------------------------------------------------

/**
 * One client per JWKS URI, kept across warm invocations. `jwks-rsa` already
 * memoizes signing keys per `kid` (see `cache*` options below), so no second
 * cache layer is needed here.
 */
let clientCache = { jwksUri: null, client: null };

function getClient(jwksUri) {
  if (clientCache.client !== null && clientCache.jwksUri === jwksUri) {
    return clientCache.client;
  }

  const client = jwksClient({
    jwksUri,
    cache: true,
    cacheMaxEntries: JWKS_CACHE_MAX_ENTRIES,
    cacheMaxAge: JWKS_CACHE_MAX_AGE_MS,
    rateLimit: true,
    jwksRequestsPerMinute: JWKS_REQUESTS_PER_MINUTE,
    timeout: JWKS_REQUEST_TIMEOUT_MS,
  });

  clientCache = { jwksUri, client };
  return client;
}

async function getSigningKey(jwksUri, kid) {
  const key = await getClient(jwksUri).getSigningKey(kid);
  return key.getPublicKey();
}

// --- Token handling ---------------------------------------------------------

/**
 * AppSync passes the raw `Authorization` header value, which is usually
 * `Bearer <jwt>` but may be a bare JWT.
 */
function extractToken(event) {
  const raw = event && event.authorizationToken;
  if (typeof raw !== 'string') return null;

  const trimmed = raw.trim();
  if (trimmed === '') return null;

  const bearer = /^Bearer\s+(.+)$/i.exec(trimmed);
  const token = bearer ? bearer[1].trim() : trimmed;
  return token === '' ? null : token;
}

async function verifyToken(token, config) {
  const decoded = jwt.decode(token, { complete: true });
  if (!decoded) {
    throw new InvalidTokenError('Token is not a well-formed JWT');
  }

  const kid = decoded.header && decoded.header.kid;
  if (!kid) {
    throw new InvalidTokenError('Token header does not contain a "kid"');
  }

  const publicKey = await getSigningKey(config.jwksUri, kid);

  // `algorithms` is the alg-confusion guard; issuer/audience are enforced only
  // when configured, and jsonwebtoken throws JsonWebTokenError when they differ.
  return jwt.verify(token, publicKey, {
    algorithms: ['RS256'],
    ...(config.issuer === undefined ? {} : { issuer: config.issuer }),
    ...(config.audience === undefined ? {} : { audience: config.audience }),
  });
}

// --- Responses --------------------------------------------------------------

function deny(ttlOverride) {
  return {
    isAuthorized: false,
    resolverContext: {},
    deniedFields: [],
    ttlOverride,
  };
}

/**
 * AppSync rejects a resolverContext containing anything other than strings, so
 * everything is coerced and non-scalars are dropped.
 *
 * Claims named in REQUIRED_CLAIMS are included: a resolver that is only reachable
 * with `tenant_id` present almost always needs to read it, and making the caller
 * re-decode the token to get a value the authorizer already verified would be
 * silly. Array-valued claims are joined with "," so the string-only invariant
 * holds. The fixed keys keep their existing behaviour so the documented contract
 * does not move.
 */
function buildResolverContext(payload, requiredClaims = []) {
  const context = {};

  const put = (key, value) => {
    if (value === undefined || value === null) return;
    if (typeof value === 'object') return; // e.g. `aud` as an array
    const asString = String(value);
    if (asString !== '') context[key] = asString;
  };

  put('sub', payload.sub);
  // Kept for backwards compatibility: the bundled VTL templates read
  // `$context.identity.resolverContext.hankoId`.
  put('hankoId', payload.sub);
  put('iss', payload.iss);
  put('aud', payload.aud);
  put('email', payload.email);
  put('exp', payload.exp);

  for (const { claim } of requiredClaims) {
    if (claim in context) continue; // never overwrite a fixed key
    const values = comparableValues(payload[claim]);
    if (values.length > 0) context[claim] = values.join(',');
  }

  return context;
}

/** Never let AppSync cache an authorization past the token's own expiry. */
function successTtlSeconds(payload, nowSeconds = Math.floor(Date.now() / 1000)) {
  if (typeof payload.exp !== 'number') return MAX_AUTH_TTL_SECONDS;
  const remaining = Math.floor(payload.exp - nowSeconds);
  if (remaining <= 0) return 0;
  return Math.min(MAX_AUTH_TTL_SECONDS, remaining);
}

// --- Handler ----------------------------------------------------------------

export const handler = async (event) => {
  let config;
  try {
    config = readConfig(process.env);
  } catch (error) {
    log('ERROR', { reason: 'configuration_error', errorName: error.name, message: error.message });
    return deny(ERROR_TTL_SECONDS);
  }

  const token = extractToken(event);
  if (token === null) {
    log('WARN', { reason: 'missing_token', message: 'Request carried no authorization token' });
    return deny(DENY_TTL_SECONDS);
  }

  let payload;
  try {
    payload = await verifyToken(token, config);
  } catch (error) {
    if (INVALID_TOKEN_ERRORS.has(error.name)) {
      log('WARN', { reason: 'invalid_token', errorName: error.name, message: error.message });
      return deny(DENY_TTL_SECONDS);
    }
    // JWKS unreachable, timed out, rate limited or returned garbage. The token
    // might be perfectly valid, so this is our failure, not the caller's.
    log('ERROR', {
      reason: 'authorizer_internal_error',
      errorName: error.name,
      message: error.message,
      jwksUri: config.jwksUri,
    });
    return deny(ERROR_TTL_SECONDS);
  }

  // Signature, issuer and audience are settled; the remaining question is
  // whether this verified identity is allowed here.
  try {
    assertClaims(payload, config.requiredClaims);
  } catch (error) {
    log('WARN', {
      reason: 'claim_denied',
      errorName: error.name,
      claim: error.claim,
      detail: error.detail,
      message: error.message,
    });
    return deny(DENY_TTL_SECONDS);
  }

  return {
    isAuthorized: true,
    resolverContext: buildResolverContext(payload, config.requiredClaims),
    ttlOverride: successTtlSeconds(payload),
  };
};
