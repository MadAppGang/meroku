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
 * @throws {ConfigurationError} when JWKS_URI is missing. Callers must deny.
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
  };
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
 */
function buildResolverContext(payload) {
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

  return {
    isAuthorized: true,
    resolverContext: buildResolverContext(payload),
    ttlOverride: successTtlSeconds(payload),
  };
};
