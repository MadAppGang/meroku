#!/bin/bash

# Simple test script for SSM Parameter API endpoints
# Usage: ./test_ssm_api.sh

API_BASE="http://localhost:8080/api"

echo "Testing SSM Parameter API Endpoints"
echo "==================================="

# Test 1: Create a parameter
echo -e "\n1. Creating a test parameter..."
curl -X PUT "$API_BASE/ssm/parameter" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "/test/meroku/param1",
    "value": "test-value-123",
    "type": "String",
    "description": "Test parameter created by meroku"
  }' | jq .

# Test 2: Get the parameter
echo -e "\n2. Getting the test parameter..."
curl -X GET "$API_BASE/ssm/parameter?name=/test/meroku/param1" | jq .

# Test 3: Update the parameter
echo -e "\n3. Updating the test parameter..."
curl -X PUT "$API_BASE/ssm/parameter" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "/test/meroku/param1",
    "value": "updated-value-456",
    "type": "String",
    "description": "Updated test parameter",
    "overwrite": true
  }' | jq .

# Test 4: List parameters with prefix
echo -e "\n4. Listing parameters with prefix /test/meroku..."
curl -X GET "$API_BASE/ssm/parameters?prefix=/test/meroku" | jq .

# Test 5: Create a SecureString parameter
echo -e "\n5. Creating a SecureString parameter..."
curl -X PUT "$API_BASE/ssm/parameter" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "/test/meroku/secret",
    "value": "super-secret-value",
    "type": "SecureString",
    "description": "Test secure parameter"
  }' | jq .

# Test 6: Delete the first parameter
echo -e "\n6. Deleting the first test parameter..."
curl -X DELETE "$API_BASE/ssm/parameter?name=/test/meroku/param1" | jq .

# Test 7: Try to get deleted parameter (should return 404)
echo -e "\n7. Trying to get deleted parameter (should fail)..."
curl -X GET "$API_BASE/ssm/parameter?name=/test/meroku/param1" | jq .

# Test 8: Clean up - delete the secret parameter
echo -e "\n8. Cleaning up - deleting the secret parameter..."
curl -X DELETE "$API_BASE/ssm/parameter?name=/test/meroku/secret" | jq .

echo -e "\nTest completed!"