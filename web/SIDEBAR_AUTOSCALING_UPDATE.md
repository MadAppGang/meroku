# Adding Autoscaling Tab to Sidebar

This document shows how to update the Sidebar component to include the autoscaling functionality.

## Changes Required in Sidebar.tsx

### 1. Import the Autoscaling Component

Add this import at the top of the file:

```tsx
import { BackendAutoscaling } from './BackendAutoscaling';
```

### 2. Add Autoscaling Tab to Backend Service

The backend service tabs are already defined in the file. You need to add the 'autoscaling' tab to the array at line 174-182:

```tsx
] : selectedNode.type === 'backend' ? [
  { id: 'settings', label: 'Settings', icon: Settings },
  { id: 'scaling', label: 'Scaling', icon: Gauge },
  { id: 'autoscaling', label: 'Autoscaling', icon: Activity },  // ADD THIS LINE
  { id: 'xray', label: 'X-Ray', icon: Microscope },
  { id: 'env', label: 'Env Vars', icon: Zap },
  { id: 'params', label: 'Parameters', icon: Key },
  { id: 's3', label: 'S3 Buckets', icon: HardDrive },
  { id: 'iam', label: 'IAM', icon: Shield },
  { id: 'logs', label: 'Logs', icon: FileText },
] : [
```

### 3. Add the Tab Content Handler

Add this after line 520 (after the IAM tab content):

```tsx
{activeTab === 'autoscaling' && selectedNode.type === 'backend' && config && (
  <BackendAutoscaling environment={config.env} />
)}
```

## Complete Example

Here's what the relevant sections should look like after the changes:

### Import Section:
```tsx
// ... existing imports ...
import { BackendIAMPermissions } from './BackendIAMPermissions';
import { BackendXRayConfiguration } from './BackendXRayConfiguration';
import { BackendScalingConfiguration } from './BackendScalingConfiguration';
import { BackendAutoscaling } from './BackendAutoscaling';  // ADD THIS
```

### Tab Definition:
```tsx
] : selectedNode.type === 'backend' ? [
  { id: 'settings', label: 'Settings', icon: Settings },
  { id: 'scaling', label: 'Scaling', icon: Gauge },
  { id: 'autoscaling', label: 'Autoscaling', icon: Activity },
  { id: 'xray', label: 'X-Ray', icon: Microscope },
  { id: 'env', label: 'Env Vars', icon: Zap },
  { id: 'params', label: 'Parameters', icon: Key },
  { id: 's3', label: 'S3 Buckets', icon: HardDrive },
  { id: 'iam', label: 'IAM', icon: Shield },
  { id: 'logs', label: 'Logs', icon: FileText },
] : [
```

### Tab Content:
```tsx
{activeTab === 'iam' && selectedNode.type === 'backend' && config && (
  <BackendIAMPermissions config={config} />
)}

{activeTab === 'autoscaling' && selectedNode.type === 'backend' && config && (
  <BackendAutoscaling environment={config.env} />
)}

{activeTab === 'cluster' && selectedNode.type === 'ecs' && config && (
  <ECSClusterInfo config={config} />
)}
```

## For Additional Services

If you want to add autoscaling to other services (not just the backend), you would:

1. Add the autoscaling tab to the default service tabs (around line 184):

```tsx
] : [
  { id: 'settings', label: 'Settings', icon: Settings },
  { id: 'logs', label: 'Logs', icon: FileText },
  { id: 'metrics', label: 'Metrics', icon: BarChart },
  { id: 'autoscaling', label: 'Autoscaling', icon: Activity },  // ADD THIS
  { id: 'env', label: 'Environment', icon: Zap },
  { id: 'connections', label: 'Connections', icon: Link },
]
```

2. Add a generic service autoscaling handler:

```tsx
{activeTab === 'autoscaling' && selectedNode.type === 'service' && config && (
  <ServiceAutoscaling environment={config.env} serviceName={selectedNode.name} />
)}
```

## Testing

After making these changes:

1. Run the development server: `npm run dev`
2. Click on the backend service node in the deployment canvas
3. You should see the "Autoscaling" tab in the sidebar
4. Click on it to view the autoscaling configuration and metrics

The autoscaling tab will automatically fetch and display:
- Current autoscaling status
- Resource configuration options
- Real-time metrics charts
- Scaling history
- Cost estimates