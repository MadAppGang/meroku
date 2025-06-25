import type { Meta, StoryObj } from '@storybook/react';
import { Sidebar } from './Sidebar';
import { ComponentNode } from '../types';

const meta = {
  title: 'Components/Sidebar/Tabs',
  component: Sidebar,
  parameters: {
    layout: 'fullscreen',
  },
  tags: ['autodocs'],
} satisfies Meta<typeof Sidebar>;

export default meta;
type Story = StoryObj<typeof meta>;

const sampleNode: ComponentNode = {
  id: '1',
  name: 'backend',
  type: 'backend',
  status: 'running',
  url: 'backend-prod.up.railway.app',
  description: 'Deployed just now',
  deploymentType: 'via GitHub',
  replicas: 3,
  resources: {
    cpu: '2 vCPU',
    memory: '4GB RAM',
  },
};

// Settings Tab
export const SettingsTab: Story = {
  args: {
    selectedNode: sampleNode,
    isOpen: true,
    onClose: () => console.log('Close clicked'),
  },
  parameters: {
    docs: {
      description: {
        story: 'Settings tab showing service configuration, status, and deployment options',
      },
    },
  },
};

// Logs Tab
export const LogsTab: Story = {
  args: {
    selectedNode: sampleNode,
    isOpen: true,
    onClose: () => console.log('Close clicked'),
  },
  play: async ({ canvasElement }) => {
    // Simulate clicking on the Logs tab
    const logsButton = canvasElement.querySelector('button:has(.lucide-file-text)');
    if (logsButton) {
      (logsButton as HTMLButtonElement).click();
    }
  },
  parameters: {
    docs: {
      description: {
        story: 'Logs tab displaying recent application logs with different severity levels',
      },
    },
  },
};

// Metrics Tab
export const MetricsTab: Story = {
  args: {
    selectedNode: sampleNode,
    isOpen: true,
    onClose: () => console.log('Close clicked'),
  },
  play: async ({ canvasElement }) => {
    // Simulate clicking on the Metrics tab
    const metricsButton = canvasElement.querySelector('button:has(.lucide-bar-chart-3)');
    if (metricsButton) {
      (metricsButton as HTMLButtonElement).click();
    }
  },
  parameters: {
    docs: {
      description: {
        story: 'Metrics tab showing performance metrics like CPU, Memory, Requests/min, and Uptime',
      },
    },
  },
};

// Environment Variables Tab
export const EnvironmentTab: Story = {
  args: {
    selectedNode: sampleNode,
    isOpen: true,
    onClose: () => console.log('Close clicked'),
  },
  play: async ({ canvasElement }) => {
    // Simulate clicking on the Environment tab
    const envButton = canvasElement.querySelector('button:has(.lucide-zap)');
    if (envButton) {
      (envButton as HTMLButtonElement).click();
    }
  },
  parameters: {
    docs: {
      description: {
        story: 'Environment variables tab for managing service configuration values',
      },
    },
  },
};

// Connections Tab
export const ConnectionsTab: Story = {
  args: {
    selectedNode: sampleNode,
    isOpen: true,
    onClose: () => console.log('Close clicked'),
  },
  play: async ({ canvasElement }) => {
    // Simulate clicking on the Connections tab
    const connectionsButton = canvasElement.querySelector('button:has(.lucide-link)');
    if (connectionsButton) {
      (connectionsButton as HTMLButtonElement).click();
    }
  },
  parameters: {
    docs: {
      description: {
        story: 'Connections tab showing which other services this service is connected to',
      },
    },
  },
};