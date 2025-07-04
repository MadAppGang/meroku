#!/bin/bash

# Test script for logs API endpoints

echo "Testing Logs API endpoints..."
echo "================================"

ENV="${1:-dev}"
SERVICE="${2:-backend}"
echo "Using environment: $ENV"
echo "Using service: $SERVICE"
echo ""

echo "1. Testing /api/logs endpoint (fetch logs)..."
curl -s "http://localhost:8080/api/logs?env=$ENV&service=$SERVICE&limit=10" | python3 -m json.tool

echo ""
echo "================================"
echo ""
echo "2. Testing WebSocket connection (requires wscat)..."
echo "To test WebSocket streaming, install wscat:"
echo "  npm install -g wscat"
echo ""
echo "Then run:"
echo "  wscat -c 'ws://localhost:8080/ws/logs?env=$ENV&service=$SERVICE'"
echo ""
echo "You should see:"
echo "  - Connection message"
echo "  - Real-time log entries as they arrive"
echo ""
echo "================================"
echo "Test complete!"