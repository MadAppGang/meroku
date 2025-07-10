# Amplify Frontend Implementation Guide

## Overview
This guide provides step-by-step instructions for integrating AWS Amplify information into the web UI, including displaying apps, build status, and managing deployments.

## Prerequisites
- The Amplify API types are already available at `web/src/types/amplify.ts`
- The API client is ready at `web/src/api/amplify.ts`
- Backend endpoints are implemented and running

## Implementation Steps

### 1. Create Amplify Apps Component

Create a new component to display Amplify apps:

**File:** `web/src/components/AmplifyApps.tsx`

```typescript
import React, { useState, useEffect } from 'react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from './ui/card';
import { Badge } from './ui/badge';
import { Button } from './ui/button';
import { RefreshCw, ExternalLink, PlayCircle, Clock, GitCommit } from 'lucide-react';
import { amplifyApi } from '../api/amplify';
import { AmplifyAppInfo, AmplifyBranchInfo } from '../types/amplify';
import { formatDistanceToNow } from 'date-fns';

interface AmplifyAppsProps {
  environment: string;
  profile?: string;
}

export const AmplifyApps: React.FC<AmplifyAppsProps> = ({ environment, profile }) => {
  const [apps, setApps] = useState<AmplifyAppInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [refreshing, setRefreshing] = useState(false);

  const fetchApps = async () => {
    try {
      setError(null);
      const response = await amplifyApi.getApps(environment, profile);
      setApps(response.apps);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch apps');
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  };

  useEffect(() => {
    fetchApps();
    
    // Auto-refresh every 30 seconds
    const interval = setInterval(fetchApps, 30000);
    return () => clearInterval(interval);
  }, [environment, profile]);

  const handleRefresh = () => {
    setRefreshing(true);
    fetchApps();
  };

  const getBuildStatusColor = (status?: string): string => {
    switch (status) {
      case 'SUCCEED': return 'bg-green-500';
      case 'FAILED': return 'bg-red-500';
      case 'RUNNING': return 'bg-blue-500';
      case 'PENDING': return 'bg-yellow-500';
      case 'CANCELLED': return 'bg-gray-500';
      default: return 'bg-gray-400';
    }
  };

  const getStageColor = (stage: string): string => {
    switch (stage) {
      case 'PRODUCTION': return 'bg-green-600';
      case 'BETA': return 'bg-orange-500';
      case 'DEVELOPMENT': return 'bg-blue-600';
      case 'EXPERIMENTAL': return 'bg-purple-600';
      default: return 'bg-gray-500';
    }
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center p-8">
        <RefreshCw className="w-6 h-6 animate-spin" />
        <span className="ml-2">Loading Amplify apps...</span>
      </div>
    );
  }

  if (error) {
    return (
      <div className="p-4 bg-red-50 border border-red-200 rounded-lg">
        <p className="text-red-600">Error: {error}</p>
        <Button onClick={handleRefresh} size="sm" className="mt-2">
          Try Again
        </Button>
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-2xl font-bold">Amplify Apps</h2>
        <Button
          onClick={handleRefresh}
          size="sm"
          variant="outline"
          disabled={refreshing}
        >
          <RefreshCw className={`w-4 h-4 mr-2 ${refreshing ? 'animate-spin' : ''}`} />
          Refresh
        </Button>
      </div>

      {apps.length === 0 ? (
        <Card>
          <CardContent className="p-6 text-center text-gray-500">
            No Amplify apps found for environment: {environment}
          </CardContent>
        </Card>
      ) : (
        apps.map((app) => (
          <AmplifyAppCard key={app.appId} app={app} onRefresh={fetchApps} />
        ))
      )}
    </div>
  );
};

// Separate component for each app
const AmplifyAppCard: React.FC<{ app: AmplifyAppInfo; onRefresh: () => void }> = ({ app, onRefresh }) => {
  const [expandedBranches, setExpandedBranches] = useState<Set<string>>(new Set());

  const toggleBranch = (branchName: string) => {
    const newExpanded = new Set(expandedBranches);
    if (newExpanded.has(branchName)) {
      newExpanded.delete(branchName);
    } else {
      newExpanded.add(branchName);
    }
    setExpandedBranches(newExpanded);
  };

  return (
    <Card>
      <CardHeader>
        <div className="flex items-start justify-between">
          <div>
            <CardTitle className="flex items-center gap-2">
              {app.name}
              <a
                href={`https://${app.defaultDomain}`}
                target="_blank"
                rel="noopener noreferrer"
                className="text-blue-500 hover:text-blue-600"
              >
                <ExternalLink className="w-4 h-4" />
              </a>
            </CardTitle>
            <CardDescription>
              <div className="space-y-1 mt-2">
                <p>Default: {app.defaultDomain}</p>
                {app.customDomain && <p>Custom: {app.customDomain}</p>}
                <p className="text-xs text-gray-500">
                  Repository: {app.repository}
                </p>
              </div>
            </CardDescription>
          </div>
        </div>
      </CardHeader>
      <CardContent>
        <div className="space-y-2">
          {app.branches.map((branch) => (
            <BranchItem
              key={branch.branchName}
              branch={branch}
              appId={app.appId}
              expanded={expandedBranches.has(branch.branchName)}
              onToggle={() => toggleBranch(branch.branchName)}
              onRefresh={onRefresh}
            />
          ))}
        </div>
      </CardContent>
    </Card>
  );
};

// Component for each branch
const BranchItem: React.FC<{
  branch: AmplifyBranchInfo;
  appId: string;
  expanded: boolean;
  onToggle: () => void;
  onRefresh: () => void;
}> = ({ branch, appId, expanded, onToggle, onRefresh }) => {
  const [triggering, setTriggering] = useState(false);

  const handleTriggerBuild = async () => {
    try {
      setTriggering(true);
      await amplifyApi.triggerBuild({
        appId,
        branchName: branch.branchName,
      });
      // Wait a bit then refresh to show new build status
      setTimeout(onRefresh, 2000);
    } catch (error) {
      console.error('Failed to trigger build:', error);
    } finally {
      setTriggering(false);
    }
  };

  return (
    <div className="border rounded-lg p-3 space-y-2">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <button
            onClick={onToggle}
            className="font-medium hover:text-blue-600"
          >
            {branch.branchName}
          </button>
          <Badge className={getStageColor(branch.stage)}>
            {branch.stage}
          </Badge>
          {branch.lastBuildStatus && (
            <Badge className={getBuildStatusColor(branch.lastBuildStatus)}>
              {branch.lastBuildStatus}
            </Badge>
          )}
        </div>
        <div className="flex items-center gap-2">
          <a
            href={branch.branchUrl}
            target="_blank"
            rel="noopener noreferrer"
            className="text-blue-500 hover:text-blue-600"
          >
            <ExternalLink className="w-4 h-4" />
          </a>
          <Button
            size="sm"
            variant="outline"
            onClick={handleTriggerBuild}
            disabled={triggering || branch.lastBuildStatus === 'RUNNING'}
          >
            {triggering ? (
              <RefreshCw className="w-4 h-4 animate-spin" />
            ) : (
              <PlayCircle className="w-4 h-4" />
            )}
          </Button>
        </div>
      </div>

      {expanded && branch.lastBuildTime && (
        <div className="pl-4 space-y-1 text-sm text-gray-600">
          <div className="flex items-center gap-2">
            <Clock className="w-3 h-3" />
            <span>
              Last build: {formatDistanceToNow(new Date(branch.lastBuildTime), { addSuffix: true })}
              {branch.lastBuildDuration && ` (${Math.floor(branch.lastBuildDuration / 60)}m ${branch.lastBuildDuration % 60}s)`}
            </span>
          </div>
          {branch.lastCommitMessage && (
            <div className="flex items-start gap-2">
              <GitCommit className="w-3 h-3 mt-0.5" />
              <div>
                <p className="line-clamp-2">{branch.lastCommitMessage}</p>
                {branch.lastCommitId && (
                  <p className="text-xs text-gray-500">
                    {branch.lastCommitId.substring(0, 7)}
                  </p>
                )}
              </div>
            </div>
          )}
          <div className="flex items-center gap-4 text-xs">
            {branch.enableAutoBuild && (
              <span className="text-green-600">✓ Auto-build</span>
            )}
            {branch.enablePullRequestPreview && (
              <span className="text-blue-600">✓ PR Preview</span>
            )}
          </div>
        </div>
      )}
    </div>
  );
};

function getStageColor(stage: string): string {
  switch (stage) {
    case 'PRODUCTION': return 'bg-green-600 text-white';
    case 'BETA': return 'bg-orange-500 text-white';
    case 'DEVELOPMENT': return 'bg-blue-600 text-white';
    case 'EXPERIMENTAL': return 'bg-purple-600 text-white';
    default: return 'bg-gray-500 text-white';
  }
}

function getBuildStatusColor(status: string): string {
  switch (status) {
    case 'SUCCEED': return 'bg-green-100 text-green-800 border-green-200';
    case 'FAILED': return 'bg-red-100 text-red-800 border-red-200';
    case 'RUNNING': return 'bg-blue-100 text-blue-800 border-blue-200 animate-pulse';
    case 'PENDING': return 'bg-yellow-100 text-yellow-800 border-yellow-200';
    case 'CANCELLED': return 'bg-gray-100 text-gray-800 border-gray-200';
    default: return 'bg-gray-100 text-gray-600 border-gray-200';
  }
}
```

### 2. Add to Your Main Dashboard

Update your existing dashboard or infrastructure view to include the Amplify component:

**File:** `web/src/components/Dashboard.tsx` (or wherever you want to add it)

```typescript
import { AmplifyApps } from './AmplifyApps';

// In your component
<div className="space-y-6">
  {/* Other dashboard sections */}
  
  {/* Amplify Apps Section */}
  <section>
    <AmplifyApps 
      environment={selectedEnvironment} 
      profile={selectedProfile}
    />
  </section>
</div>
```

### 3. Create a Build Status Widget

For a compact view in your infrastructure diagram:

**File:** `web/src/components/AmplifyStatusWidget.tsx`

```typescript
import React, { useEffect, useState } from 'react';
import { amplifyApi } from '../api/amplify';
import { AmplifyAppInfo } from '../types/amplify';
import { Badge } from './ui/badge';
import { ExternalLink, AlertCircle } from 'lucide-react';

interface AmplifyStatusWidgetProps {
  appName: string;
  environment: string;
  profile?: string;
  compact?: boolean;
}

export const AmplifyStatusWidget: React.FC<AmplifyStatusWidgetProps> = ({
  appName,
  environment,
  profile,
  compact = false
}) => {
  const [app, setApp] = useState<AmplifyAppInfo | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const fetchApp = async () => {
      try {
        const response = await amplifyApi.getApps(environment, profile);
        const foundApp = response.apps.find(a => a.name === appName);
        setApp(foundApp || null);
      } catch (error) {
        console.error('Failed to fetch Amplify app:', error);
      } finally {
        setLoading(false);
      }
    };

    fetchApp();
    const interval = setInterval(fetchApp, 60000); // Refresh every minute
    return () => clearInterval(interval);
  }, [appName, environment, profile]);

  if (loading || !app) {
    return compact ? null : <div>Loading...</div>;
  }

  // Find the production branch or first branch
  const mainBranch = app.branches.find(b => b.stage === 'PRODUCTION') || app.branches[0];
  
  if (compact) {
    return (
      <div className="flex items-center gap-2">
        <div className={`w-2 h-2 rounded-full ${
          mainBranch?.lastBuildStatus === 'SUCCEED' ? 'bg-green-500' :
          mainBranch?.lastBuildStatus === 'FAILED' ? 'bg-red-500' :
          mainBranch?.lastBuildStatus === 'RUNNING' ? 'bg-blue-500 animate-pulse' :
          'bg-gray-400'
        }`} />
        <span className="text-xs text-gray-600">{app.defaultDomain}</span>
      </div>
    );
  }

  return (
    <div className="p-3 border rounded-lg space-y-2">
      <div className="flex items-center justify-between">
        <h4 className="font-medium">{app.name}</h4>
        <a
          href={`https://${app.customDomain || app.defaultDomain}`}
          target="_blank"
          rel="noopener noreferrer"
          className="text-blue-500 hover:text-blue-600"
        >
          <ExternalLink className="w-4 h-4" />
        </a>
      </div>
      
      {mainBranch && (
        <div className="flex items-center gap-2 text-sm">
          <Badge className={getBuildStatusColor(mainBranch.lastBuildStatus || '')}>
            {mainBranch.lastBuildStatus || 'NO BUILDS'}
          </Badge>
          <span className="text-gray-500">{mainBranch.branchName}</span>
        </div>
      )}
      
      <div className="text-xs text-gray-500">
        {app.customDomain || app.defaultDomain}
      </div>
    </div>
  );
};

function getBuildStatusColor(status: string): string {
  switch (status) {
    case 'SUCCEED': return 'bg-green-100 text-green-800';
    case 'FAILED': return 'bg-red-100 text-red-800';
    case 'RUNNING': return 'bg-blue-100 text-blue-800 animate-pulse';
    case 'PENDING': return 'bg-yellow-100 text-yellow-800';
    default: return 'bg-gray-100 text-gray-600';
  }
}
```

### 4. Add Build Logs Viewer

Create a modal or drawer to view build logs:

**File:** `web/src/components/AmplifyBuildLogs.tsx`

```typescript
import React, { useState, useEffect } from 'react';
import { Dialog, DialogContent, DialogHeader, DialogTitle } from './ui/dialog';
import { Button } from './ui/button';
import { amplifyApi } from '../api/amplify';
import { ExternalLink, Download } from 'lucide-react';

interface AmplifyBuildLogsProps {
  appId: string;
  branchName: string;
  jobId: string;
  open: boolean;
  onClose: () => void;
}

export const AmplifyBuildLogs: React.FC<AmplifyBuildLogsProps> = ({
  appId,
  branchName,
  jobId,
  open,
  onClose
}) => {
  const [logUrl, setLogUrl] = useState<string>('');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (open && jobId) {
      fetchLogs();
    }
  }, [open, jobId]);

  const fetchLogs = async () => {
    try {
      setLoading(true);
      setError(null);
      const response = await amplifyApi.getBuildLogs(appId, branchName, jobId);
      setLogUrl(response.logUrl);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch logs');
    } finally {
      setLoading(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onClose}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>Build Logs - {branchName} (Job: {jobId})</DialogTitle>
        </DialogHeader>
        
        <div className="mt-4">
          {loading && <p>Loading logs...</p>}
          
          {error && (
            <div className="p-4 bg-red-50 border border-red-200 rounded">
              <p className="text-red-600">{error}</p>
            </div>
          )}
          
          {logUrl && !loading && (
            <div className="space-y-4">
              <div className="p-4 bg-gray-50 rounded">
                <p className="text-sm text-gray-600 mb-2">
                  Build logs are available at the following URL:
                </p>
                <div className="flex items-center gap-2">
                  <a
                    href={logUrl}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="text-blue-500 hover:text-blue-600 flex items-center gap-1"
                  >
                    <ExternalLink className="w-4 h-4" />
                    View Logs in AWS Console
                  </a>
                </div>
              </div>
              
              <Button
                onClick={() => window.open(logUrl, '_blank')}
                className="w-full"
              >
                <Download className="w-4 h-4 mr-2" />
                Open Logs
              </Button>
            </div>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
};
```

### 5. Integration with Infrastructure Diagram

If you have an infrastructure visualization, add Amplify indicators:

```typescript
// In your infrastructure node component
import { AmplifyStatusWidget } from './AmplifyStatusWidget';

// When rendering an Amplify app node
{node.type === 'amplify' && (
  <AmplifyStatusWidget
    appName={node.data.name}
    environment={currentEnvironment}
    profile={currentProfile}
    compact={true}
  />
)}
```

### 6. Add to Navigation/Menu

Update your navigation to include Amplify section:

```typescript
// In your navigation configuration
const navigationItems = [
  // ... other items
  {
    title: 'Frontend Apps',
    icon: <Globe className="w-4 h-4" />,
    path: '/amplify',
    description: 'Manage Amplify applications'
  }
];
```

### 7. Create Amplify Page Route

Add a dedicated page for Amplify management:

**File:** `web/src/pages/AmplifyPage.tsx`

```typescript
import React from 'react';
import { AmplifyApps } from '../components/AmplifyApps';
import { useEnvironment } from '../hooks/useEnvironment'; // Your environment hook

export const AmplifyPage: React.FC = () => {
  const { selectedEnvironment, selectedProfile } = useEnvironment();

  return (
    <div className="container mx-auto p-6">
      <AmplifyApps 
        environment={selectedEnvironment} 
        profile={selectedProfile}
      />
    </div>
  );
};
```

## Styling Guidelines

### Color Coding
- **Build Status**:
  - Success: Green (`bg-green-500`)
  - Failed: Red (`bg-red-500`)
  - Running: Blue with pulse animation (`bg-blue-500 animate-pulse`)
  - Pending: Yellow (`bg-yellow-500`)
  
- **Stages**:
  - Production: Green (`bg-green-600`)
  - Beta: Orange (`bg-orange-500`)
  - Development: Blue (`bg-blue-600`)
  - Experimental: Purple (`bg-purple-600`)

### Icons
- External link: `<ExternalLink />` for opening apps
- Refresh: `<RefreshCw />` for manual refresh
- Play: `<PlayCircle />` for triggering builds
- Clock: `<Clock />` for build times
- Git: `<GitCommit />` for commit info

## Error Handling

Always implement proper error handling:

```typescript
try {
  const response = await amplifyApi.getApps(environment);
  // Handle success
} catch (error) {
  // Show user-friendly error message
  setError('Unable to load Amplify apps. Please check your permissions.');
  console.error('Amplify API error:', error);
}
```

## Performance Optimization

1. **Caching**: Consider using React Query or SWR for data fetching
2. **Lazy Loading**: Load branch details only when expanded
3. **Pagination**: If you have many apps, implement pagination
4. **WebSocket**: For real-time build status updates (future enhancement)

## Testing

Create test files for your components:

```typescript
// AmplifyApps.test.tsx
import { render, screen, waitFor } from '@testing-library/react';
import { AmplifyApps } from './AmplifyApps';
import { amplifyApi } from '../api/amplify';

jest.mock('../api/amplify');

describe('AmplifyApps', () => {
  it('displays apps correctly', async () => {
    const mockApps = {
      apps: [{
        appId: 'test-id',
        name: 'test-app',
        defaultDomain: 'test.amplifyapp.com',
        branches: []
      }]
    };
    
    (amplifyApi.getApps as jest.Mock).mockResolvedValue(mockApps);
    
    render(<AmplifyApps environment="dev" />);
    
    await waitFor(() => {
      expect(screen.getByText('test-app')).toBeInTheDocument();
    });
  });
});
```

## Next Steps

1. **Add Notifications**: Show toast notifications for build triggers
2. **Build History**: Create a view to show build history over time
3. **Metrics Dashboard**: Add charts for build success rates
4. **Webhook Integration**: Set up real-time updates via webhooks
5. **Deployment Rollback**: Add ability to rollback to previous builds

## Troubleshooting

Common issues and solutions:

1. **No apps showing**: Check environment tags in AWS
2. **403 errors**: Verify IAM permissions for Amplify
3. **Slow loading**: Implement pagination or filtering
4. **Build trigger fails**: Check branch protection rules

## Required Dependencies

Make sure these are installed:

```bash
npm install date-fns lucide-react
```

Your existing UI components (Card, Badge, Button, etc.) should work as-is.