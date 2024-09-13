import jwt from 'jsonwebtoken';
import jwksClient from 'jwks-rsa';
import NodeCache from 'node-cache';

const cache = new NodeCache({ stdTTL: 3600 }); // Cache for 1 hour

const client = jwksClient({
  jwksUri: process.env.JWKS_URI || 'https://302ef659-5cff-4f61-8db7-a3eb4bbf5113.hanko.io/.well-known/jwks.json',
  cache: true,
  cacheMaxEntries: 5,
  cacheMaxAge: 600000, // 10 minutes
});

async function getKey(kid) {
  const cachedKey = cache.get(kid);
  if (cachedKey) {
    return cachedKey;
  }

  return new Promise((resolve, reject) => {
    client.getSigningKey(kid, (err, key) => {
      if (err) {
        reject(err);
      } else {
        const publicKey = key.getPublicKey();
        cache.set(kid, publicKey);
        resolve(publicKey);
      }
    });
  });
}

function parseJWT(token) {
  const base64Url = token.split('.')[1];
  const base64 = base64Url.replace(/-/g, '+').replace(/_/g, '/');
  const jsonPayload = decodeURIComponent(
    atob(base64)
      .split('')
      .map((c) => `%${`00${c.charCodeAt(0).toString(16)}`.slice(-2)}`)
      .join(''),
  );
  return JSON.parse(jsonPayload);
}

export const handler = async (event, context) => {
  const token = event.authorizationToken;
  if (!token) {
    return {
      statusCode: 401,
      body: JSON.stringify({ message: 'No token provided' }),
    };
  }

  if (process.env.NODE_ENV === 'development') {
    console.warn('Skipping JWT verification in development mode');
    const payload = parseJWT(token);
    console.warn(`Naive authentication result: ${payload.sub}`);
    return {
      isAuthorized: true,
      resolverContext: {
        hankoId: payload.sub,
        naiveAuth: true,
      },
    };
  }

  try {
    const decodedToken = jwt.decode(token, { complete: true });
    if (!decodedToken) {
      throw new Error('Invalid token');
    }

    const kid = decodedToken.header.kid;
    if (!kid) {
      throw new Error('No "kid" in token header');
    }

    const publicKey = await getKey(kid);
    const payload = jwt.verify(token, publicKey, { algorithms: ['RS256'] });

    return {
      isAuthorized: true,
      resolverContext: {
        hankoId: payload.sub,
        naiveAuth: false,
      },
    };
  } catch (error) {
    console.error('Error:', error);
  }

  return {
    isAuthorized: false,
    deniedFields: [],
  };
};
