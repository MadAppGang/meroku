import type { Meta, StoryObj } from '@storybook/react';
import { ServiceNode } from './ServiceNode';
import { ReactFlowProvider } from 'reactflow';
import 'reactflow/dist/style.css';

const meta = {
  title: 'Components/ServiceNode',
  component: ServiceNode,
  parameters: {
    layout: 'centered',
  },
  decorators: [
    (Story) => (
      <ReactFlowProvider>
        <div style={{ width: '400px', height: '300px', position: 'relative' }}>
          <Story />
        </div>
      </ReactFlowProvider>
    ),
  ],
  tags: ['autodocs'],
} satisfies Meta<typeof ServiceNode>;

export default meta;
type Story = StoryObj<typeof meta>;

const baseNodeProps = {
  id: '1',
  position: { x: 0, y: 0 },
  selected: false,
};

// Frontend Service Stories
export const FrontendRunning: Story = {
  args: {
    ...baseNodeProps,
    data: {
      id: 'frontend-1',
      name: 'frontend',
      type: 'frontend',
      status: 'running',
      url: 'frontend-prod.up.railway.app',
      description: 'Deployed just now',
      deploymentType: 'via GitHub',
    },
  },
};

export const FrontendDeploying: Story = {
  args: {
    ...baseNodeProps,
    data: {
      id: 'frontend-2',
      name: 'frontend',
      type: 'frontend',
      status: 'deploying',
      url: 'frontend-prod.up.railway.app',
      description: 'Deploying...',
      deploymentType: 'via GitHub',
    },
  },
};

export const FrontendError: Story = {
  args: {
    ...baseNodeProps,
    data: {
      id: 'frontend-3',
      name: 'frontend',
      type: 'frontend',
      status: 'error',
      url: 'frontend-prod.up.railway.app',
      description: 'Deployment failed',
      deploymentType: 'via GitHub',
    },
  },
};

// Backend Service Stories
export const BackendWithReplicas: Story = {
  args: {
    ...baseNodeProps,
    data: {
      id: 'backend-1',
      name: 'backend',
      type: 'backend',
      status: 'running',
      description: 'Deployed just now',
      deploymentType: 'via GitHub',
      replicas: 3,
    },
  },
};

// Database Service Stories
export const DatabaseWithResources: Story = {
  args: {
    ...baseNodeProps,
    data: {
      id: 'database-1',
      name: 'postgres',
      type: 'database',
      status: 'running',
      description: 'Deployed via Docker Image',
      resources: {
        cpu: '2 vCPU',
        memory: '4GB RAM',
      },
    },
  },
};

// Redis Cache Stories
export const RedisCache: Story = {
  args: {
    ...baseNodeProps,
    data: {
      id: 'cache-1',
      name: 'redis',
      type: 'cache',
      status: 'running',
      description: 'Just deployed',
      resources: {
        cpu: '1 vCPU',
        memory: '1GB RAM',
      },
    },
  },
};

// API Gateway Stories
export const APIGateway: Story = {
  args: {
    ...baseNodeProps,
    data: {
      id: 'api-1',
      name: 'api gateway',
      type: 'api',
      status: 'running',
      url: 'api-prod.up.railway.app',
      description: 'Deployed just now',
    },
  },
};

// Analytics Service Stories
export const AnalyticsService: Story = {
  args: {
    ...baseNodeProps,
    data: {
      id: 'analytics-1',
      name: 'ackee analytics',
      type: 'analytics',
      status: 'running',
      url: 'ackee-prod.up.railway.app',
      description: 'Deployed via Docker Image',
    },
  },
};

// Selected State
export const SelectedNode: Story = {
  args: {
    ...baseNodeProps,
    selected: true,
    data: {
      id: 'backend-2',
      name: 'backend',
      type: 'backend',
      status: 'running',
      description: 'Selected service node',
      deploymentType: 'via GitHub',
      replicas: 3,
    },
  },
};

// All Service Types
export const AllServiceTypes: Story = {
  render: () => (
    <div style={{ display: 'grid', gridTemplateColumns: 'repeat(2, 1fr)', gap: '20px', width: '800px' }}>
      <ServiceNode
        {...baseNodeProps}
        data={{
          id: 'frontend-all',
          name: 'frontend',
          type: 'frontend',
          status: 'running',
          url: 'frontend-prod.up.railway.app',
          description: 'Deployed just now',
          deploymentType: 'via GitHub',
        }}
      />
      <ServiceNode
        {...baseNodeProps}
        data={{
          id: 'backend-all',
          name: 'backend',
          type: 'backend',
          status: 'running',
          description: 'Deployed just now',
          deploymentType: 'via GitHub',
          replicas: 3,
        }}
      />
      <ServiceNode
        {...baseNodeProps}
        data={{
          id: 'database-all',
          name: 'postgres',
          type: 'database',
          status: 'running',
          description: 'Deployed via Docker Image',
          resources: {
            cpu: '2 vCPU',
            memory: '4GB RAM',
          },
        }}
      />
      <ServiceNode
        {...baseNodeProps}
        data={{
          id: 'cache-all',
          name: 'redis',
          type: 'cache',
          status: 'running',
          description: 'Just deployed',
          resources: {
            cpu: '1 vCPU',
            memory: '1GB RAM',
          },
        }}
      />
      <ServiceNode
        {...baseNodeProps}
        data={{
          id: 'api-all',
          name: 'api gateway',
          type: 'api',
          status: 'running',
          url: 'api-prod.up.railway.app',
          description: 'Deployed just now',
        }}
      />
      <ServiceNode
        {...baseNodeProps}
        data={{
          id: 'analytics-all',
          name: 'ackee analytics',
          type: 'analytics',
          status: 'running',
          url: 'ackee-prod.up.railway.app',
          description: 'Deployed via Docker Image',
        }}
      />
    </div>
  ),
};

// All Status States
export const AllStatusStates: Story = {
  render: () => (
    <div style={{ display: 'grid', gridTemplateColumns: 'repeat(2, 1fr)', gap: '20px', width: '800px' }}>
      <ServiceNode
        {...baseNodeProps}
        data={{
          id: 'status-running',
          name: 'Running Service',
          type: 'backend',
          status: 'running',
          description: 'Service is running normally',
        }}
      />
      <ServiceNode
        {...baseNodeProps}
        data={{
          id: 'status-deploying',
          name: 'Deploying Service',
          type: 'backend',
          status: 'deploying',
          description: 'Service is being deployed',
        }}
      />
      <ServiceNode
        {...baseNodeProps}
        data={{
          id: 'stopped-service',
          name: 'Stopped Service',
          type: 'backend',
          status: 'stopped',
          description: 'Service has been stopped',
        }}
      />
      <ServiceNode
        {...baseNodeProps}
        data={{
          id: 'error-service',
          name: 'Error Service',
          type: 'backend',
          status: 'error',
          description: 'Service encountered an error',
        }}
      />
    </div>
  ),
};