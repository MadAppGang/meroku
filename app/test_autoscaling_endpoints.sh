#!/bin/bash

# Test script for autoscaling API endpoints

echo "Testing Autoscaling API endpoints..."
echo "================================"

ENV="${1:-dev}"
SERVICE="${2:-backend}"
echo "Using environment: $ENV"
echo "Using service: $SERVICE"
echo ""

echo "1. Testing /api/ecs/autoscaling endpoint..."
curl -s "http://localhost:8080/api/ecs/autoscaling?env=$ENV&service=$SERVICE" | python3 -m json.tool

echo ""
echo "2. Testing /api/ecs/scaling-history endpoint..."
curl -s "http://localhost:8080/api/ecs/scaling-history?env=$ENV&service=$SERVICE&hours=24" | python3 -m json.tool

echo ""
echo "3. Testing /api/ecs/metrics endpoint..."
curl -s "http://localhost:8080/api/ecs/metrics?env=$ENV&service=$SERVICE" | python3 -m json.tool

echo ""
echo "================================"
echo "Test complete!"