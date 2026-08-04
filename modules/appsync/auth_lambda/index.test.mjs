/**
 * Tests for the AppSync Lambda authorizer.
 *
 * Run with: node --test
 *
 * These tests never touch the public internet. An RS256 keypair is generated
 * in-process and the JWKS document is served by a throwaway HTTP server bound
 * to 127.0.0.1 on an ephemeral port, so the real `jwks-rsa` fetch/cache path is
 * exercised without any external dependency.
 */

import test from 'node:test';
import assert from 'node:assert/strict';
import http from 'node:http';
import { generateKeyPairSync } from 'node:crypto';
import jwt from 'jsonwebtoken';

import { handler } from './index.mjs';

// --- Fixtures ---------------------------------------------------------------

const KID = 'test-signing-key';
const OTHER_KID = 'unknown-signing-key';
const ISSUER = 'https://issuer.example.com/';
const AUDIENCE = 'https://api.example.com/';

const keyPair = generateKeyPairSync('rsa', { modulusLength: 2048 });
const privatePem = keyPair.privateKey.export({ type: 'pkcs8', format: 'pem' });
const jwk = { ...keyPair.publicKey.export({ format: 'jwk' }), kid: KID, alg: 'RS256', use: 'sig' };

// A second, unrelated key used to forge tokens the JWKS does not vouch for.
const foreignKeyPair = generateKeyPairSync('rsa', { modulusLength: 2048 });
const foreignPrivatePem = foreignKeyPair.privateKey.export({ type: 'pkcs8', format: 'pem' });

const nowSeconds = () => Math.floor(Date.now() / 1000);

function signToken({ claims = {}, options = {}, key = privatePem } = {}) {
  const payload = { sub: 'user-abc-123', iss: ISSUER, aud: AUDIENCE, exp: nowSeconds() + 300, ...claims };
  return jwt.sign(payload, key, { algorithm: 'RS256', keyid: KID, ...options });
}

// --- Harness ----------------------------------------------------------------

/** Serves a JWKS document (or a failure) on 127.0.0.1. */
async function startJwksServer({ status = 200, body = JSON.stringify({ keys: [jwk] }) } = {}) {
  const requests = [];
  const server = http.createServer((req, res) => {
    requests.push(req.url);
    res.writeHead(status, { 'content-type': 'application/json' });
    res.end(body);
  });

  await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve));

  return {
    uri: `http://127.0.0.1:${server.address().port}/.well-known/jwks.json`,
    requests,
    async close() {
      server.closeAllConnections();
      await new Promise((resolve) => server.close(resolve));
    },
  };
}

const ENV_KEYS = ['JWKS_URI', 'JWT_ISSUER', 'JWT_AUDIENCE'];

function setEnv(vars = {}) {
  for (const key of ENV_KEYS) {
    if (vars[key] === undefined) {
      delete process.env[key];
    } else {
      process.env[key] = vars[key];
    }
  }
}

/** Runs `fn` with console.warn/console.error captured and parsed. */
async function captureLogs(fn) {
  const lines = [];
  const originalWarn = console.warn;
  const originalError = console.error;
  console.warn = (line) => lines.push(line);
  console.error = (line) => lines.push(line);

  try {
    const result = await fn();
    return { result, logs: lines.map((line) => JSON.parse(line)) };
  } finally {
    console.warn = originalWarn;
    console.error = originalError;
  }
}

function createEvent(token) {
  return {
    authorizationToken: token,
    requestContext: {
      apiId: 'test-api-id',
      accountId: '000000000000',
      requestId: 'test-request-id',
      queryString: 'query { someField }',
      operationName: 'SomeOperation',
    },
  };
}

function assertDenied(result) {
  assert.equal(result.isAuthorized, false, 'must not authorize');
  assert.deepEqual(result.deniedFields, []);
  assert.ok(Number.isInteger(result.ttlOverride), 'ttlOverride must be an integer');
  assert.ok(result.ttlOverride > 0, 'denials must set a non-zero ttlOverride so floods are cached');
}

function reasonsOf(logs) {
  return logs.map((entry) => entry.reason);
}

/** Guards against the token or its claims leaking into CloudWatch. */
function assertNoTokenInLogs(logs, token) {
  for (const entry of logs) {
    assert.ok(!JSON.stringify(entry).includes(token), 'token must never be logged');
  }
}

test.afterEach(() => setEnv({}));

// --- Tests ------------------------------------------------------------------

test('denies when the request carries no token', async () => {
  const jwks = await startJwksServer();
  try {
    setEnv({ JWKS_URI: jwks.uri });

    const { result, logs } = await captureLogs(() => handler(createEvent(undefined)));

    assertDenied(result);
    assert.deepEqual(reasonsOf(logs), ['missing_token']);
    assert.equal(jwks.requests.length, 0, 'must not hit the JWKS endpoint without a token');
  } finally {
    await jwks.close();
  }
});

test('denies when the token is an empty/whitespace string', async () => {
  setEnv({ JWKS_URI: 'https://issuer.example.com/.well-known/jwks.json' });

  const { result, logs } = await captureLogs(() => handler(createEvent('   ')));

  assertDenied(result);
  assert.deepEqual(reasonsOf(logs), ['missing_token']);
});

test('denies and reports a configuration error when JWKS_URI is missing', async () => {
  setEnv({}); // JWKS_URI deliberately unset

  const token = signToken();
  const { result, logs } = await captureLogs(() => handler(createEvent(token)));

  assertDenied(result);
  assert.deepEqual(reasonsOf(logs), ['configuration_error']);
  assert.equal(logs[0].level, 'ERROR');
  assert.match(logs[0].message, /JWKS_URI/);
  // Distinguishable from a bad token, and short so it recovers once fixed.
  assert.equal(result.ttlOverride, 10);
});

test('denies when JWKS_URI is set to an empty string', async () => {
  setEnv({ JWKS_URI: '   ' });

  const { result, logs } = await captureLogs(() => handler(createEvent(signToken())));

  assertDenied(result);
  assert.deepEqual(reasonsOf(logs), ['configuration_error']);
});

test('authorizes a valid token and returns a string-only resolverContext', async () => {
  const jwks = await startJwksServer();
  try {
    setEnv({ JWKS_URI: jwks.uri, JWT_ISSUER: ISSUER, JWT_AUDIENCE: AUDIENCE });

    const token = signToken({ claims: { email: 'user@example.com' } });
    const { result, logs } = await captureLogs(() => handler(createEvent(token)));

    assert.equal(result.isAuthorized, true);
    assert.equal(result.deniedFields, undefined);
    assert.deepEqual(logs, [], 'a successful authorization should not log');

    // AppSync rejects resolverContext entries that are not strings.
    for (const [key, value] of Object.entries(result.resolverContext)) {
      assert.equal(typeof value, 'string', `resolverContext.${key} must be a string`);
    }
    assert.equal(result.resolverContext.sub, 'user-abc-123');
    assert.equal(result.resolverContext.hankoId, 'user-abc-123');
    assert.equal(result.resolverContext.iss, ISSUER);
    assert.equal(result.resolverContext.aud, AUDIENCE);
    assert.equal(result.resolverContext.email, 'user@example.com');

    // Never cached past the token's own expiry, never longer than the cap.
    assert.ok(Number.isInteger(result.ttlOverride));
    assert.ok(result.ttlOverride > 0 && result.ttlOverride <= 300);
    assert.equal(jwks.requests.length, 1);
  } finally {
    await jwks.close();
  }
});

test('accepts a token sent with a Bearer prefix', async () => {
  const jwks = await startJwksServer();
  try {
    setEnv({ JWKS_URI: jwks.uri });

    const result = await handler(createEvent(`Bearer ${signToken()}`));

    assert.equal(result.isAuthorized, true);
    assert.equal(result.resolverContext.sub, 'user-abc-123');
  } finally {
    await jwks.close();
  }
});

test('denies an expired token', async () => {
  const jwks = await startJwksServer();
  try {
    setEnv({ JWKS_URI: jwks.uri });

    const token = signToken({ claims: { exp: nowSeconds() - 60 } });
    const { result, logs } = await captureLogs(() => handler(createEvent(token)));

    assertDenied(result);
    assert.deepEqual(reasonsOf(logs), ['invalid_token']);
    assert.equal(logs[0].errorName, 'TokenExpiredError');
    assert.equal(result.ttlOverride, 60);
    assertNoTokenInLogs(logs, token);
  } finally {
    await jwks.close();
  }
});

test('denies a token minted by a different issuer', async () => {
  const jwks = await startJwksServer();
  try {
    setEnv({ JWKS_URI: jwks.uri, JWT_ISSUER: ISSUER });

    const token = signToken({ claims: { iss: 'https://attacker.example.net/' } });
    const { result, logs } = await captureLogs(() => handler(createEvent(token)));

    assertDenied(result);
    assert.deepEqual(reasonsOf(logs), ['invalid_token']);
    assert.match(logs[0].message, /issuer/i);
  } finally {
    await jwks.close();
  }
});

test('denies a token minted for a different audience', async () => {
  const jwks = await startJwksServer();
  try {
    setEnv({ JWKS_URI: jwks.uri, JWT_AUDIENCE: AUDIENCE });

    const token = signToken({ claims: { aud: 'https://someone-elses-api.example.net/' } });
    const { result, logs } = await captureLogs(() => handler(createEvent(token)));

    assertDenied(result);
    assert.deepEqual(reasonsOf(logs), ['invalid_token']);
    assert.match(logs[0].message, /audience/i);
  } finally {
    await jwks.close();
  }
});

test('accepts any issuer/audience when they are not configured', async () => {
  const jwks = await startJwksServer();
  try {
    setEnv({ JWKS_URI: jwks.uri });

    const token = signToken({ claims: { iss: 'https://anything.example.net/', aud: 'anything' } });
    const result = await handler(createEvent(token));

    assert.equal(result.isAuthorized, true);
  } finally {
    await jwks.close();
  }
});

test('denies a token signed by a key the JWKS does not vouch for', async () => {
  const jwks = await startJwksServer();
  try {
    setEnv({ JWKS_URI: jwks.uri });

    const token = signToken({ key: foreignPrivatePem });
    const { result, logs } = await captureLogs(() => handler(createEvent(token)));

    assertDenied(result);
    assert.deepEqual(reasonsOf(logs), ['invalid_token']);
    assert.equal(logs[0].errorName, 'JsonWebTokenError');
  } finally {
    await jwks.close();
  }
});

test('denies a token whose kid is not in the JWKS', async () => {
  const jwks = await startJwksServer();
  try {
    setEnv({ JWKS_URI: jwks.uri });

    const token = signToken({ options: { keyid: OTHER_KID } });
    const { result, logs } = await captureLogs(() => handler(createEvent(token)));

    assertDenied(result);
    assert.deepEqual(reasonsOf(logs), ['invalid_token']);
    assert.equal(logs[0].errorName, 'SigningKeyNotFoundError');
  } finally {
    await jwks.close();
  }
});

test('denies a token with no kid header', async () => {
  const jwks = await startJwksServer();
  try {
    setEnv({ JWKS_URI: jwks.uri });

    const token = jwt.sign({ sub: 'user-abc-123' }, privatePem, { algorithm: 'RS256' });
    const { result, logs } = await captureLogs(() => handler(createEvent(token)));

    assertDenied(result);
    assert.deepEqual(reasonsOf(logs), ['invalid_token']);
    assert.equal(jwks.requests.length, 0, 'must reject before fetching the JWKS');
  } finally {
    await jwks.close();
  }
});

test('denies a token that is not a JWT at all', async () => {
  const jwks = await startJwksServer();
  try {
    setEnv({ JWKS_URI: jwks.uri });

    const { result, logs } = await captureLogs(() => handler(createEvent('not-a-jwt')));

    assertDenied(result);
    assert.deepEqual(reasonsOf(logs), ['invalid_token']);
  } finally {
    await jwks.close();
  }
});

test('denies an HS256 token signed with the public key (algorithm confusion)', async () => {
  const jwks = await startJwksServer();
  try {
    setEnv({ JWKS_URI: jwks.uri });

    const publicPem = keyPair.publicKey.export({ type: 'spki', format: 'pem' });
    const token = jwt.sign({ sub: 'attacker', exp: nowSeconds() + 300 }, publicPem, {
      algorithm: 'HS256',
      keyid: KID,
    });

    const { result, logs } = await captureLogs(() => handler(createEvent(token)));

    assertDenied(result);
    assert.deepEqual(reasonsOf(logs), ['invalid_token']);
    assert.match(logs[0].message, /algorithm/i);
  } finally {
    await jwks.close();
  }
});

test('denies with an internal error when the JWKS endpoint fails', async () => {
  const jwks = await startJwksServer({ status: 500, body: '{"message":"boom"}' });
  try {
    setEnv({ JWKS_URI: jwks.uri });

    const { result, logs } = await captureLogs(() => handler(createEvent(signToken())));

    assertDenied(result);
    // Distinguishable in CloudWatch from a genuinely invalid token.
    assert.deepEqual(reasonsOf(logs), ['authorizer_internal_error']);
    assert.equal(logs[0].level, 'ERROR');
    assert.equal(logs[0].jwksUri, jwks.uri);
    // Short TTL: the token may well be valid, so recover quickly.
    assert.equal(result.ttlOverride, 10);
  } finally {
    await jwks.close();
  }
});

test('denies with an internal error when the JWKS document is malformed', async () => {
  const jwks = await startJwksServer({ body: 'this is not json' });
  try {
    setEnv({ JWKS_URI: jwks.uri });

    const { result, logs } = await captureLogs(() => handler(createEvent(signToken())));

    assertDenied(result);
    assert.deepEqual(reasonsOf(logs), ['authorizer_internal_error']);
  } finally {
    await jwks.close();
  }
});
