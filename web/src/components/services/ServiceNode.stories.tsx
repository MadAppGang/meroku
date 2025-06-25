import type { Meta, StoryObj } from '@storybook/react';
import { ServiceNode } from './ServiceNode';
import { ServiceNode as ServiceNodeType } from '../../types';

const meta: Meta<typeof ServiceNode> = {
  title: 'Services/ServiceNode',
  component: ServiceNode,
  parameters: {
    layout: 'centered',
    backgrounds: {
      default: 'dark',
    },
  },
  tags: ['autodocs'],
  decorators: [
    (Story) => (
      <div style={{ padding: '2rem', background: '#0a0a0a' }}>
        <svg width="400" height="300" style={{ background: '#1a1a1a', borderRadius: '8px' }}>
          <Story />
        </svg>
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof meta>;

const baseNode: ServiceNodeType = {
  id: "1",
  x: 150,
  y: 100,
  label: "Backend API",
  type: "backend",
  status: "running",
  lastDeployed: "2 hours ago",
  project: "myapp-backend",
  framework: "Python",
  version: "3.9",
  region: "us-east-1",
  environment: "production",
  replicas: 3,
};

export const BackendNode: Story = {
  args: {
    node: baseNode,
    isSelected: false,
    onMouseDown: () => {},
  },
};

export const FrontendNode: Story = {
  args: {
    node: {
      ...baseNode,
      id: "2",
      label: "Frontend",
      type: "frontend",
      project: "myapp-frontend",
      framework: "Next.js",
      version: "14.0.0",
      replicas: 1,
    },
    isSelected: false,
    onMouseDown: () => {},
  },
};

export const DatabaseNode: Story = {
  args: {
    node: {
      ...baseNode,
      id: "3",
      label: "PostgreSQL",
      type: "database",
      project: "pg-data",
      version: "15.3",
      replicas: 2,
    },
    isSelected: false,
    onMouseDown: () => {},
  },
};

export const SelectedNode: Story = {
  args: {
    node: baseNode,
    isSelected: true,
    onMouseDown: () => {},
  },
};

export const NodeWithHighReplicas: Story = {
  args: {
    node: {
      ...baseNode,
      replicas: 10,
    },
    isSelected: false,
    onMouseDown: () => {},
  },
};