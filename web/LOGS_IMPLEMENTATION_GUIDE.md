# Logs Implementation Guide

This guide documents the implementation of real-time log streaming and viewing functionality for ECS services.

## Overview

The logs feature provides:
- **Historical logs retrieval** from CloudWatch Logs
- **Real-time log streaming** via WebSocket
- **Advanced filtering** by log level and search terms
- **Export functionality** for log analysis
- **Auto-scroll** and pause/resume controls

## Architecture

```
Browser ←→ WebSocket ←→ Go Backend ←→ CloudWatch Logs
         ↓
       HTTP API
```

## Backend Implementation

### 1. **HTTP Endpoint** (`GET /api/logs`)

Fetches recent logs from CloudWatch Logs:

```go
// Request parameters
env: string      // Environment name
service: string  // Service name
limit: number    // Max logs to return (default: 100, max: 1000)
nextToken: string // Pagination token

// Response
{
  "serviceName": "backend",
  "logs": [
    {
      "timestamp": "2024-01-12T10:30:45Z",
      "message": "Request processed successfully",
      "level": "info",
      "stream": "ecs/backend/abc123"
    }
  ],
  "nextToken": "..."
}
```

### 2. **WebSocket Endpoint** (`ws://localhost:8080/ws/logs`)

Streams logs in real-time:

```javascript
// Connection URL
ws://localhost:8080/ws/logs?env=dev&service=backend

// Message types
{
  "type": "connected",
  "message": "Connected to log stream"
}

{
  "type": "logs",
  "data": [
    {
      "timestamp": "...",
      "message": "...",
      "level": "...",
      "stream": "..."
    }
  ]
}

{
  "error": "Error message"
}
```

### 3. **Log Level Detection**

The backend automatically detects log levels by scanning message content:
- **error**: Contains "error" or "exception"
- **warning**: Contains "warn"
- **debug**: Contains "debug"
- **info**: Default level

## Frontend Implementation

### 1. **ServiceLogs Component**

Main features:
- **Live streaming toggle**: Start/stop real-time updates
- **Search**: Filter logs by message or stream name
- **Level filter**: Show only specific log levels
- **Auto-scroll**: Automatically scroll to newest logs
- **Export**: Download logs as text file
- **Load more**: Pagination for historical logs

### 2. **API Integration**

```typescript
// Fetch logs
const logs = await infrastructureApi.getServiceLogs(
  environment,
  serviceName,
  limit,
  nextToken
);

// Stream logs
const ws = infrastructureApi.connectToLogStream(
  environment,
  serviceName,
  (newLogs) => {
    // Handle new logs
  },
  (error) => {
    // Handle errors
  },
  () => {
    // Handle connection established
  }
);

// Close connection
ws.close();
```

### 3. **Performance Optimizations**

- **Log limit**: Keeps maximum 1000 logs in memory
- **Debounced search**: Prevents excessive filtering
- **Virtual scrolling**: Efficiently renders large log lists
- **WebSocket reconnection**: Auto-reconnects on connection loss

## Usage

### 1. **View Logs**

1. Click on a service node in the deployment canvas
2. Navigate to the "Logs" tab
3. Logs automatically load from the last hour

### 2. **Real-time Streaming**

1. Click "Start Streaming" button
2. New logs appear at the top automatically
3. Click "Stop Streaming" to pause updates

### 3. **Search and Filter**

1. Use the search box to filter by message content
2. Use the level dropdown to filter by severity
3. Combined filters work together (AND logic)

### 4. **Export Logs**

1. Apply desired filters
2. Click "Export" button
3. Logs download as timestamped text file

## CloudWatch Logs Structure

The implementation expects this log group naming convention:
- Backend service: `{project}_backend_{env}`
- Other services: `{project}_{service}_{env}`

Example:
- `myapp_backend_dev`
- `myapp_worker_dev`

## Troubleshooting

### Common Issues

1. **"Failed to get log streams"**
   - Check CloudWatch Logs permissions
   - Verify log group exists
   - Ensure service has been running and generating logs

2. **WebSocket connection fails**
   - Check if backend is running
   - Verify CORS settings allow WebSocket
   - Check browser console for errors

3. **No logs appearing**
   - Service might not be generating logs
   - Check CloudWatch Logs retention settings
   - Verify time range (defaults to last hour)

4. **Logs appear delayed**
   - CloudWatch Logs has inherent delay (5-10 seconds)
   - Increase polling frequency if needed
   - Check network latency

### Required IAM Permissions

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "logs:DescribeLogStreams",
        "logs:FilterLogEvents"
      ],
      "Resource": "arn:aws:logs:*:*:log-group:*"
    }
  ]
}
```

## Testing

### 1. **Test HTTP Endpoint**
```bash
./test_logs_endpoints.sh dev backend
```

### 2. **Test WebSocket**
```bash
# Install wscat
npm install -g wscat

# Connect to WebSocket
wscat -c 'ws://localhost:8080/ws/logs?env=dev&service=backend'
```

### 3. **Generate Test Logs**

To generate logs for testing:
```bash
# SSH into container
aws ecs execute-command --cluster <cluster> --task <task> --container backend --interactive --command "/bin/sh"

# Generate logs
echo "Test log message"
echo "ERROR: Test error message"
echo "WARNING: Test warning"
```

## Future Enhancements

1. **Advanced Filtering**
   - Regex support
   - Time range selection
   - Multiple search terms

2. **Log Analytics**
   - Error rate graphs
   - Log volume metrics
   - Pattern detection

3. **Alerts**
   - Error threshold notifications
   - Pattern-based alerts
   - Integration with monitoring

4. **Performance**
   - Log aggregation
   - Compression
   - Caching layer

5. **Export Options**
   - JSON format
   - CSV format
   - Direct S3 upload

This implementation provides a robust foundation for log management in your infrastructure visualization tool.