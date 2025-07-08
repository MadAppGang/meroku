#!/bin/bash

# Test script to verify the ECS tasks API fix
echo "Testing ECS tasks API endpoint..."

# Start the server in background
cd /Users/jack/mag/infrastructure
export AWS_PROFILE=default
./app/meroku web -p 3001 &
SERVER_PID=$!

# Wait for server to start
sleep 3

# Test the endpoint
echo "Calling /api/ecs/tasks?env=dev&service=aichat"
curl -s "http://localhost:3001/api/ecs/tasks?env=dev&service=aichat" | jq .

# Kill the server
kill $SERVER_PID 2>/dev/null

echo "Test complete"