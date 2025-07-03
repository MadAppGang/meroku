#!/bin/bash

# Test script for ECS endpoints

echo "Testing ECS API endpoints..."
echo "================================"

ENV="${1:-dev}"
echo "Using environment: $ENV"
echo ""

echo "1. Testing /api/ecs/cluster endpoint..."
curl -s "http://localhost:8080/api/ecs/cluster?env=$ENV" | python3 -m json.tool

echo ""
echo "2. Testing /api/ecs/network endpoint..."
curl -s "http://localhost:8080/api/ecs/network?env=$ENV" | python3 -m json.tool

echo ""
echo "3. Testing /api/ecs/services endpoint..."
curl -s "http://localhost:8080/api/ecs/services?env=$ENV" | python3 -m json.tool

echo ""
echo "================================"
echo "Test complete!"