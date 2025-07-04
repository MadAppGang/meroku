# Autoscaling Frontend Implementation Guide

This guide provides complete instructions for implementing the autoscaling feature in the frontend, including UI components and API integration.

## Table of Contents

1. [Overview](#overview)
2. [Prerequisites](#prerequisites)
3. [File Structure](#file-structure)
4. [Implementation Steps](#implementation-steps)
5. [Component Integration](#component-integration)
6. [Testing](#testing)
7. [Troubleshooting](#troubleshooting)

## Overview

The autoscaling feature allows users to:
- View current autoscaling configuration and status
- Monitor real-time metrics (CPU, Memory, Task Count)
- Review scaling history
- Configure autoscaling parameters
- Estimate costs based on configuration

## Prerequisites

1. **Dependencies to Install**:
```bash
npm install recharts date-fns
```

2. **API Endpoints** (already implemented):
- `GET /api/ecs/autoscaling?env={env}&service={service}`
- `GET /api/ecs/scaling-history?env={env}&service={service}&hours={hours}`
- `GET /api/ecs/metrics?env={env}&service={service}`

3. **TypeScript Types** (already added to `api/infrastructure.ts`):
- `ServiceAutoscalingInfo`
- `ServiceScalingHistory`
- `ServiceMetrics`

## File Structure

```
web/src/
├── api/
│   └── infrastructure.ts         # API client with autoscaling methods
├── components/
│   ├── ServiceAutoscaling.tsx    # Main autoscaling component
│   ├── BackendAutoscaling.tsx    # Backend-specific wrapper (to create)
│   └── Sidebar.tsx              # Update to add autoscaling tab
└── types/
    └── index.ts                 # Type definitions
```

## Implementation Steps

### Step 1: Create the Backend Autoscaling Wrapper Component

Create `web/src/components/BackendAutoscaling.tsx`:

```tsx
import React from 'react';
import { ServiceAutoscaling } from './ServiceAutoscaling';

interface BackendAutoscalingProps {
  environment: string;
}

export function BackendAutoscaling({ environment }: BackendAutoscalingProps) {
  return <ServiceAutoscaling environment={environment} serviceName="backend" />;
}
```

### Step 2: Update the Sidebar Component

Add the autoscaling tab to `web/src/components/Sidebar.tsx`:

```tsx
// Import the new component
import { BackendAutoscaling } from './BackendAutoscaling';

// In the backend node tabs section, add:
{nodeData.id === 'backend-service' && (
  <Tabs defaultValue="env-vars" className="w-full">
    <TabsList className="grid w-full grid-cols-5">
      <TabsTrigger value="env-vars">Environment</TabsTrigger>
      <TabsTrigger value="params">Parameters</TabsTrigger>
      <TabsTrigger value="s3">S3 Buckets</TabsTrigger>
      <TabsTrigger value="iam">IAM</TabsTrigger>
      <TabsTrigger value="autoscaling">Autoscaling</TabsTrigger>
    </TabsList>
    {/* ... existing tabs ... */}
    <TabsContent value="autoscaling">
      <BackendAutoscaling environment={selectedEnvironment} />
    </TabsContent>
  </Tabs>
)}
```

### Step 3: Add Autoscaling for Additional Services

For services defined in the YAML configuration, create a generic wrapper:

```tsx
// In the service node section of Sidebar.tsx
{nodeData.type === 'service' && (
  <Tabs defaultValue="config" className="w-full">
    <TabsList className="grid w-full grid-cols-2">
      <TabsTrigger value="config">Configuration</TabsTrigger>
      <TabsTrigger value="autoscaling">Autoscaling</TabsTrigger>
    </TabsList>
    <TabsContent value="config">
      {/* Existing service config */}
    </TabsContent>
    <TabsContent value="autoscaling">
      <ServiceAutoscaling 
        environment={selectedEnvironment} 
        serviceName={nodeData.name} 
      />
    </TabsContent>
  </Tabs>
)}
```

### Step 4: Add Save Functionality

To save autoscaling changes, you'll need to:

1. **Create an UPDATE endpoint** in the backend (not yet implemented)
2. **Add the API method** in `infrastructure.ts`:

```typescript
async updateServiceAutoscaling(
  env: string,
  serviceName: string,
  config: Partial<ServiceAutoscalingInfo>
): Promise<void> {
  const response = await fetch(
    `${API_BASE_URL}/api/ecs/autoscaling?env=${encodeURIComponent(env)}&service=${encodeURIComponent(serviceName)}`,
    {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(config),
    }
  );
  if (!response.ok) {
    throw new Error("Failed to update autoscaling configuration");
  }
}
```

3. **Update the Save button handler** in `ServiceAutoscaling.tsx`:

```tsx
const handleSave = async () => {
  try {
    await infrastructureApi.updateServiceAutoscaling(environment, serviceName, {
      enabled: formData.enabled,
      minCapacity: formData.minCapacity,
      maxCapacity: formData.maxCapacity,
      targetCPU: formData.targetCPU,
      targetMemory: formData.targetMemory,
      // Note: CPU, memory, and desired count would require Terraform apply
    });
    // Show success message
  } catch (error) {
    // Show error message
  }
};
```

## Component Integration

### Complete Integration Example

Here's how all the pieces fit together:

1. **User clicks on a backend service node** in the deployment canvas
2. **Sidebar opens** showing service details with tabs
3. **User clicks "Autoscaling" tab**
4. **Component fetches** current configuration and metrics
5. **User can view**:
   - Current status and utilization
   - Historical metrics chart
   - Scaling event history
   - Cost estimates
6. **User can modify**:
   - Enable/disable autoscaling
   - Set min/max capacity
   - Adjust CPU/memory targets
   - Change resource allocation

### Data Flow

```
User Action → Component State → API Call → Backend → AWS APIs
                    ↑                           ↓
                    └─────── Response ←─────────┘
```

## Testing

### Manual Testing Steps

1. **Start the development server**:
```bash
cd web
npm run dev
```

2. **Start the backend**:
```bash
cd app
./meroku
```

3. **Test each feature**:
   - ✅ View current autoscaling status
   - ✅ Check real-time metrics display
   - ✅ Review scaling history
   - ✅ Modify configuration values
   - ✅ Verify cost calculations
   - ✅ Test refresh functionality

### API Testing

Use the provided test script:
```bash
./test_autoscaling_endpoints.sh dev backend
```

### Component Testing

Create test file `ServiceAutoscaling.test.tsx`:

```tsx
import { render, screen, waitFor } from '@testing-library/react';
import { ServiceAutoscaling } from './ServiceAutoscaling';
import { infrastructureApi } from '../api/infrastructure';

jest.mock('../api/infrastructure');

describe('ServiceAutoscaling', () => {
  it('displays autoscaling status', async () => {
    infrastructureApi.getServiceAutoscaling.mockResolvedValue({
      serviceName: 'backend',
      enabled: true,
      currentDesiredCount: 2,
      // ... other mock data
    });

    render(<ServiceAutoscaling environment="dev" serviceName="backend" />);
    
    await waitFor(() => {
      expect(screen.getByText('Enabled')).toBeInTheDocument();
      expect(screen.getByText('2')).toBeInTheDocument();
    });
  });
});
```

## Troubleshooting

### Common Issues and Solutions

1. **"Failed to fetch autoscaling data"**
   - Check if backend is running
   - Verify AWS credentials are configured
   - Ensure the service exists in the specified environment

2. **Empty metrics data**
   - Container Insights may not be enabled
   - Service might be newly created (no historical data)
   - Check CloudWatch permissions

3. **Scaling history not showing**
   - Autoscaling might not be enabled
   - No scaling events in the time window
   - Check Application Auto Scaling permissions

4. **Cost calculations seem incorrect**
   - Verify the AWS region (prices vary)
   - Check if additional features affect pricing
   - Remember these are estimates only

### Performance Optimization

1. **Reduce API calls**:
   - Implement caching for metrics data
   - Debounce configuration changes
   - Use React Query or SWR for data fetching

2. **Optimize chart rendering**:
   - Limit data points displayed
   - Use virtualization for long history lists
   - Implement lazy loading for tabs

## Next Steps

1. **Implement the UPDATE endpoint** in the backend to save configuration changes
2. **Add notifications** for successful saves and errors
3. **Create unit tests** for the component
4. **Add role-based permissions** for who can modify autoscaling
5. **Implement webhooks** for scaling event notifications
6. **Add export functionality** for metrics and cost data

## Additional Features to Consider

1. **Predictive Scaling**: Show predicted scaling based on historical patterns
2. **Scheduled Scaling**: Allow users to set time-based scaling rules
3. **Custom Metrics**: Support scaling based on custom CloudWatch metrics
4. **Alerts Configuration**: Set up CloudWatch alarms from the UI
5. **Multi-Service View**: Dashboard showing all services' autoscaling status

This implementation provides a complete autoscaling management interface that integrates seamlessly with your existing infrastructure visualization tool.