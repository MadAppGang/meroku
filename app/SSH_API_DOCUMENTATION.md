# SSH API Documentation

This document describes the SSH API endpoints that provide interactive terminal access to ECS containers.

## Overview

The SSH API provides WebSocket-based terminal access to ECS containers using AWS ECS Execute Command. It allows frontend applications to create interactive SSH-like sessions directly in the browser.

## Prerequisites

1. **AWS ECS Execute Command** must be enabled on your ECS tasks
2. **AWS Session Manager Plugin** must be installed on the server running the backend
3. **Proper IAM permissions** for ECS Execute Command
4. **Container must have `/bin/bash`** or another shell available

## API Endpoints

### 1. Get Service Tasks

**Endpoint:** `GET /api/ecs/tasks`

**Purpose:** Retrieve all running tasks for a service to get their task ARNs.

**Query Parameters:**
- `env` (required): Environment name (e.g., "dev", "prod")
- `service` (required): Service name (e.g., "backend")

**Response:**
```json
{
  "serviceName": "backend",
  "tasks": [
    {
      "taskArn": "arn:aws:ecs:eu-central-1:123456789:task/cluster/abc123",
      "taskDefinitionArn": "arn:aws:ecs:eu-central-1:123456789:task-definition/app:1",
      "serviceName": "backend",
      "launchType": "FARGATE",
      "lastStatus": "RUNNING",
      "desiredStatus": "RUNNING",
      "healthStatus": "HEALTHY",
      "createdAt": "2024-01-12T10:00:00Z",
      "startedAt": "2024-01-12T10:01:00Z",
      "cpu": "256",
      "memory": "512",
      "availabilityZone": "eu-central-1a"
    }
  ]
}
```

### 2. Check SSH Capability

**Endpoint:** `GET /api/ssh/capability`

**Purpose:** Check if a specific task supports SSH access (ECS Execute Command enabled).

**Query Parameters:**
- `env` (required): Environment name
- `service` (required): Service name
- `taskArn` (required): Full task ARN from the tasks endpoint

**Response:**
```json
{
  "enabled": true,
  "reason": ""
}
```

Or if not enabled:
```json
{
  "enabled": false,
  "reason": "Execute command is not enabled for this task"
}
```

### 3. WebSocket SSH Connection

**Endpoint:** `ws://localhost:8080/ws/ssh`

**Purpose:** Establish an interactive SSH session to a container.

**Query Parameters:**
- `env` (required): Environment name
- `service` (required): Service name
- `taskArn` (required): Full task ARN
- `container` (optional): Container name (if not provided, uses service name pattern)

**WebSocket Message Format:**

Messages are JSON objects with the following structure:

**Incoming messages (from server):**
```json
{
  "type": "connected|output|error|disconnected",
  "data": "message content"
}
```

**Outgoing messages (to server):**
```json
{
  "type": "input",
  "data": "command to execute\n"
}
```

**Message Types:**
- `connected`: Initial connection established
- `output`: Terminal output (stdout/stderr)
- `error`: Error messages
- `disconnected`: Session ended
- `input`: User input to send to terminal

## Usage Example

### Step 1: Get Running Tasks

```bash
curl "http://localhost:8080/api/ecs/tasks?env=dev&service=backend"
```

### Step 2: Check SSH Capability

```bash
# Using task ARN from step 1
curl "http://localhost:8080/api/ssh/capability?env=dev&service=backend&taskArn=arn:aws:ecs:eu-central-1:123456789:task/cluster/abc123"
```

### Step 3: Connect via WebSocket

Using `wscat`:
```bash
# Install wscat
npm install -g wscat

# Connect to SSH session
wscat -c 'ws://localhost:8080/ws/ssh?env=dev&service=backend&taskArn=arn:aws:ecs:eu-central-1:123456789:task/cluster/abc123'

# Once connected, you'll see:
< {"type":"connected","data":"Connecting to backend container in task abc123..."}

# Send commands:
> {"type":"input","data":"ls -la\n"}
< {"type":"output","data":"total 64\ndrwxr-xr-x 1 app app 4096 Jan 12 10:00 .\n..."}

# Exit:
> {"type":"input","data":"exit\n"}
< {"type":"disconnected","data":"Session ended"}
```

### JavaScript/TypeScript Example

```typescript
// 1. Get tasks
const tasksResponse = await fetch('/api/ecs/tasks?env=dev&service=backend');
const tasks = await tasksResponse.json();
const taskArn = tasks.tasks[0]?.taskArn;

// 2. Check capability
const capabilityResponse = await fetch(`/api/ssh/capability?env=dev&service=backend&taskArn=${taskArn}`);
const capability = await capabilityResponse.json();

if (!capability.enabled) {
  console.error('SSH not available:', capability.reason);
  return;
}

// 3. Connect WebSocket
const ws = new WebSocket(`ws://localhost:8080/ws/ssh?env=dev&service=backend&taskArn=${taskArn}`);

ws.onmessage = (event) => {
  const message = JSON.parse(event.data);
  switch (message.type) {
    case 'connected':
      console.log('Connected:', message.data);
      break;
    case 'output':
      console.log(message.data); // Display in terminal UI
      break;
    case 'error':
      console.error('Error:', message.data);
      break;
    case 'disconnected':
      console.log('Disconnected:', message.data);
      break;
  }
};

// Send command
function sendCommand(command) {
  ws.send(JSON.stringify({
    type: 'input',
    data: command + '\n'
  }));
}

// Example: List files
sendCommand('ls -la');
```

## Error Handling

Common errors and their meanings:

1. **"Cluster not found"** - The ECS cluster doesn't exist or wrong environment
2. **"Service not found"** - The service name doesn't match the ECS service pattern
3. **"Execute command is not enabled"** - ECS Execute Command not enabled on the task
4. **"Failed to start ECS execute command"** - Session Manager plugin not installed or IAM permissions issue

## Security Considerations

1. **Authentication**: Ensure your API has proper authentication before exposing SSH access
2. **Authorization**: Verify users have permission to access specific environments/services
3. **Audit Logging**: All SSH sessions are logged by AWS Session Manager
4. **Network Security**: Use HTTPS/WSS in production
5. **Input Validation**: The backend validates all parameters to prevent injection attacks

## Implementation Notes

1. The backend uses `aws ecs execute-command` CLI command internally
2. Container names follow the pattern:
   - Backend: `{project}_service_{env}`
   - Other services: `{project}_service_{serviceName}_{env}`
3. The WebSocket handles bi-directional communication between browser and container
4. Session cleanup happens automatically on disconnect
5. **Important**: The backend uses a pseudo-terminal (PTY) to properly handle interactive sessions, ensuring keystrokes are properly transmitted and echoed

## Troubleshooting Input Issues

If keystrokes aren't being sent to the remote terminal:

1. **Check PTY allocation**: The backend now uses `github.com/creack/pty` to allocate a pseudo-terminal
2. **Debug with test script**: Use `./test_ssh_debug.sh dev backend` to test raw WebSocket communication
3. **Verify message format**: Input must be sent as `{"type": "input", "data": "your text"}`
4. **Character encoding**: Special characters and control sequences should be sent as-is

## Testing

Use the provided test script:
```bash
./test_ssh_endpoints.sh dev backend
```

This will:
1. Get running tasks
2. Check SSH capability
3. Show how to connect via WebSocket