#!/bin/bash

# Test script for tasks API endpoint

echo "Testing Tasks API endpoint..."
echo "==============================="

ENV="${1:-dev}"
SERVICE="${2:-backend}"
echo "Using environment: $ENV"
echo "Using service: $SERVICE"
echo ""

echo "1. Testing /api/ecs/tasks endpoint (get task ARNs)..."
echo "Note: Frontend service name '$SERVICE' will be mapped to actual ECS service name:"
if [ "$SERVICE" = "backend" ]; then
    echo "  - Backend service: {project}_service_{env}"
else
    echo "  - Other service: {project}_service_${SERVICE}_{env}"
fi
echo ""

curl -s "http://localhost:8080/api/ecs/tasks?env=$ENV&service=$SERVICE" | python3 -m json.tool

echo ""
echo "==============================="
echo "Test complete!"
echo ""
echo "This endpoint returns:"
echo "- Task ARNs for running tasks"
echo "- Task definition ARNs"
echo "- Task status and health information"
echo "- CPU and memory allocation"
echo "- Availability zone"
echo "- Launch timestamps"
echo ""
echo "Service name mapping:"
echo "- 'backend' → {project}_service_{env}"
echo "- 'other' → {project}_service_other_{env}"