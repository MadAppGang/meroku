import type { Meta, StoryObj } from '@storybook/react';
import { useState } from 'react';
import { Settings, FileText, BarChart3, Terminal } from 'lucide-react';
import { Tabs } from './Tabs';

const meta: Meta<typeof Tabs> = {
  title: 'UI/Tabs',
  component: Tabs,
  parameters: {
    layout: 'padded',
  },
  tags: ['autodocs'],
};

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  render: () => {
    const [activeTab, setActiveTab] = useState('settings');
    
    const tabs = [
      { 
        id: 'settings', 
        label: 'Settings',
        content: <div className="text-gray-300">Settings content</div>
      },
      { 
        id: 'logs', 
        label: 'Logs',
        content: <div className="text-gray-300">Logs content</div>
      },
      { 
        id: 'metrics', 
        label: 'Metrics',
        content: <div className="text-gray-300">Metrics content</div>
      },
    ];
    
    return (
      <div className="h-96 bg-gray-800 rounded-lg">
        <Tabs
          tabs={tabs}
          activeTab={activeTab}
          onTabChange={setActiveTab}
        />
      </div>
    );
  },
};

export const WithIcons: Story = {
  render: () => {
    const [activeTab, setActiveTab] = useState('settings');
    
    const tabs = [
      { 
        id: 'settings', 
        label: 'Settings',
        icon: Settings,
        content: (
          <div className="space-y-4">
            <div className="bg-gray-700 p-4 rounded-lg">
              <h3 className="text-white font-medium mb-2">Configuration</h3>
              <p className="text-gray-300 text-sm">Manage your settings here</p>
            </div>
          </div>
        )
      },
      { 
        id: 'logs', 
        label: 'Logs',
        icon: FileText,
        content: (
          <div className="font-mono text-xs space-y-1">
            <div className="text-gray-400">[2024-06-25 08:05:50] Starting deployment...</div>
            <div className="text-blue-400">[2024-06-25 08:06:20] Pulling Docker image...</div>
            <div className="text-green-400">[2024-06-25 08:06:50] Container started successfully</div>
          </div>
        )
      },
      { 
        id: 'metrics', 
        label: 'Metrics',
        icon: BarChart3,
        content: (
          <div className="space-y-4">
            <div className="bg-gray-700 p-4 rounded-lg">
              <h4 className="text-gray-300 text-sm mb-2">CPU Usage</h4>
              <div className="text-2xl text-purple-400">24%</div>
            </div>
          </div>
        )
      },
      { 
        id: 'env', 
        label: 'Environment',
        icon: Terminal,
        content: (
          <div className="text-gray-300">
            <p className="mb-2">Environment Variables:</p>
            <div className="bg-gray-700 p-2 rounded font-mono text-sm">
              NODE_ENV=production<br/>
              API_URL=https://api.example.com
            </div>
          </div>
        )
      },
    ];
    
    return (
      <div className="h-96 bg-gray-800 rounded-lg">
        <Tabs
          tabs={tabs}
          activeTab={activeTab}
          onTabChange={setActiveTab}
        />
      </div>
    );
  },
};