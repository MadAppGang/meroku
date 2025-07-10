# Amplify API Endpoints

## Overview
The backend now provides API endpoints to interact with AWS Amplify apps, allowing you to:
- Get list of Amplify apps with their default domains and build status
- View build logs for specific jobs
- Trigger new builds for branches

## Endpoints

### 1. Get Amplify Apps
**GET** `/api/amplify/apps`

Retrieves all Amplify apps for a specific environment with branch and build information.

#### Query Parameters
- `environment` (required): The environment to filter apps by (e.g., "dev", "prod")
- `profile` (optional): AWS profile to use for authentication

#### Response
```json
{
  "apps": [
    {
      "appId": "d1234abcd",
      "name": "main-web",
      "defaultDomain": "d1234abcd.amplifyapp.com",
      "customDomain": "example.com",
      "repository": "https://github.com/username/repo",
      "createTime": "2024-01-15T10:30:00Z",
      "lastUpdateTime": "2024-01-20T15:45:00Z",
      "branches": [
        {
          "branchName": "main",
          "stage": "PRODUCTION",
          "displayName": "main",
          "enableAutoBuild": true,
          "enablePullRequestPreview": false,
          "branchUrl": "https://main.d1234abcd.amplifyapp.com",
          "lastBuildStatus": "SUCCEED",
          "lastBuildTime": "2024-01-20T15:30:00Z",
          "lastBuildDuration": 180,
          "lastCommitId": "abc123def",
          "lastCommitMessage": "Update feature X",
          "lastCommitTime": "2024-01-20T15:25:00Z",
          "createTime": "2024-01-15T10:35:00Z",
          "updateTime": "2024-01-20T15:45:00Z"
        }
      ]
    }
  ]
}
```

### 2. Get Build Logs
**GET** `/api/amplify/build-logs`

Retrieves build logs for a specific job.

#### Query Parameters
- `appId` (required): The Amplify app ID
- `branchName` (required): The branch name
- `jobId` (required): The job ID
- `profile` (optional): AWS profile to use

#### Response
```json
{
  "logUrl": "https://logs.amplify.com/...",
  "job": {
    // Full job details including steps and status
  }
}
```

### 3. Trigger Build
**POST** `/api/amplify/trigger-build`

Triggers a new build for a specific branch.

#### Request Body
```json
{
  "appId": "d1234abcd",
  "branchName": "main",
  "profile": "default" // optional
}
```

#### Response
```json
{
  "jobId": "123",
  "status": "PENDING",
  "message": "Build triggered successfully"
}
```

## Frontend Integration

### TypeScript Types
The frontend types are available in `web/src/types/amplify.ts`:
- `AmplifyAppInfo`: Main app information
- `AmplifyBranchInfo`: Branch details including build status
- `AmplifyAppsResponse`: API response type
- `TriggerBuildRequest/Response`: Build trigger types

### API Client
Use the API client in `web/src/api/amplify.ts`:

```typescript
import { amplifyApi } from '../api/amplify';

// Get all apps for an environment
const response = await amplifyApi.getApps('dev', 'default');

// Get build logs
const logs = await amplifyApi.getBuildLogs(appId, branchName, jobId);

// Trigger a new build
const result = await amplifyApi.triggerBuild({
  appId: 'd1234abcd',
  branchName: 'main',
  profile: 'default'
});
```

## Build Status Values
- `PENDING`: Build is queued
- `PROVISIONING`: Build environment is being set up
- `RUNNING`: Build is in progress
- `FAILED`: Build failed
- `SUCCEED`: Build completed successfully
- `CANCELLING`: Build is being cancelled
- `CANCELLED`: Build was cancelled

## Stage Values
- `PRODUCTION`: Production environment
- `BETA`: Beta/staging environment
- `DEVELOPMENT`: Development environment
- `EXPERIMENTAL`: Experimental features
- `PULL_REQUEST`: Pull request preview

## Error Handling
All endpoints return appropriate HTTP status codes:
- `200 OK`: Success
- `400 Bad Request`: Missing or invalid parameters
- `500 Internal Server Error`: AWS API errors or server issues

Error responses include a descriptive message:
```json
{
  "error": "Failed to list Amplify apps: AccessDeniedException"
}
```

## Security Considerations
1. The API filters apps by environment tags to ensure proper isolation
2. AWS credentials are managed through profiles or IAM roles
3. No sensitive build information is exposed in the responses
4. Build logs URLs are temporary and expire after a period

## Usage Examples

### Display Amplify Apps in UI
```typescript
const [apps, setApps] = useState<AmplifyAppInfo[]>([]);

useEffect(() => {
  const fetchApps = async () => {
    try {
      const response = await amplifyApi.getApps(currentEnvironment);
      setApps(response.apps);
    } catch (error) {
      console.error('Failed to fetch Amplify apps:', error);
    }
  };
  
  fetchApps();
}, [currentEnvironment]);
```

### Show Build Status Badge
```typescript
const getBuildStatusColor = (status?: string) => {
  switch (status) {
    case 'SUCCEED': return 'green';
    case 'FAILED': return 'red';
    case 'RUNNING': return 'blue';
    case 'PENDING': return 'yellow';
    default: return 'gray';
  }
};
```

### Auto-refresh Build Status
```typescript
const [refreshInterval, setRefreshInterval] = useState<NodeJS.Timeout>();

const startAutoRefresh = () => {
  const interval = setInterval(async () => {
    const response = await amplifyApi.getApps(environment);
    updateAppsState(response.apps);
  }, 30000); // Refresh every 30 seconds
  
  setRefreshInterval(interval);
};

// Clean up on unmount
useEffect(() => {
  return () => {
    if (refreshInterval) clearInterval(refreshInterval);
  };
}, [refreshInterval]);
```