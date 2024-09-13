// Use 'import' instead of 'require'
import { handler } from './index.mjs';

// Mock event payload
const createEvent = (token) => ({
  authorizationToken: token,
  requestContext: {
    apiId: 'test-api-id',
    accountId: '123456789012',
    requestId: 'test-request-id',
    queryString: 'query { someField }',
    operationName: 'SomeOperation',
  },
});

// sub = 1234567890
const naiveToken = 'eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCIsImtpZCI6Ijk2ZTc5YjgwLWYxNWYtNDU3NC1iYWRlLWIzOWFiMmY1MzE5MSJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.jGZi8g0OB1wZncZqpjdWZjSczNvgp8NodU7chS7gl64aXvL2LSdbVaPmKeTDQiH_0gWywSPffaV2IpqfoiHF0G3ShzeHffjEUIlp7aIXwI3gUBVGixvlHAIcnSjc02gyquzy7Qpm_LMOZF7vryXTCzk50EitD66lHCKPPAyQe8K_lpUQMvY54rBtfrrgvdICbqRg6tfZdIqHcTK2TZx1zXPBPR__mVwC7UiZ9pVg2U1pyX83MLglWrvHcyDeX2iPBmY3RUiUMiwHiR9pmd5SpiHgZFZZymxBPKeeo9TGZLNPd_LIWJRzu93xwkStl0RTNo2U2xh2-Gp1jZzmTatbQQ'


// Test function
async function testAuthorizer() {
  console.log('Testing authorizer function...');

  // Test case 1: Valid token
  process.env.NODE_ENV = 'development';
  const validEvent = createEvent(naiveToken);
  console.log('Test case 1: Valid token');
  const validResult = await handler(validEvent);
  console.log('Result:', JSON.stringify(validResult, null, 2));

  // Test case 2: Invalid token
  process.env.NODE_ENV = 'production';
  const invalidEvent = createEvent(naiveToken);
  console.log('\nTest case 2: Invalid token');
  const invalidResult = await handler(invalidEvent);
  console.log('Result:', JSON.stringify(invalidResult, null, 2));
}

// Run the test
testAuthorizer().catch((error) => console.error('Test error:', error))
