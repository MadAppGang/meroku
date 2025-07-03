#!/bin/bash

# Test script for the /api/account endpoint

echo "Testing /api/account endpoint..."
echo "================================"

# Make a GET request to the account endpoint
curl -s http://localhost:8080/api/account | python3 -m json.tool

echo ""
echo "================================"
echo "Test complete!"