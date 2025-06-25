import React from 'react';
import { X } from 'lucide-react';
import { Settings, FileText, BarChart3, Terminal } from 'lucide-react';
import { ServiceNode } from '../../types';
import { getServiceColor } from '../../utils/colors';
import { getServiceIcon } from '../../utils/icons';
import { IconButton, Input, Select, Button, Tabs } from '../ui';

export interface NodeSettingsPanelProps {
  node: ServiceNode;
  activeTab: string;
  onTabChange: (tabId: string) => void;
  onClose: () => void;
  onUpdateReplicas: (nodeId: string, change: number) => void;
  onDeleteNode: (nodeId: string) => void;
}

export const NodeSettingsPanel: React.FC<NodeSettingsPanelProps> = ({
  node,
  activeTab,
  onTabChange,
  onClose,
  onUpdateReplicas,
  onDeleteNode
}) => {
  const Icon = getServiceIcon(node.type);
  const color = getServiceColor(node.type);

  const settingsContent = (
    <div className="space-y-4">
      <Input
        label="Container Image"
        value={node.project}
        readOnly
      />
      
      <Select label="Region" value={node.region}>
        <option value="us-east-1">us-east-1</option>
        <option value="us-west-2">us-west-2</option>
        <option value="eu-west-1">eu-west-1</option>
        <option value="ap-southeast-1">ap-southeast-1</option>
      </Select>
      
      {node.type === 'backend' && (
        <>
          <div>
            <label className="text-gray-400 text-sm">Replicas</label>
            <div className="flex items-center gap-2 mt-1">
              <Button
                variant="secondary"
                size="sm"
                onClick={() => onUpdateReplicas(node.id, -1)}
              >
                -
              </Button>
              <input
                className="w-20 text-center bg-gray-700 text-white px-3 py-2 rounded-lg border border-gray-600"
                value={node.replicas || 1}
                readOnly
              />
              <Button
                variant="secondary"
                size="sm"
                onClick={() => onUpdateReplicas(node.id, 1)}
              >
                +
              </Button>
            </div>
          </div>
          
          <Select label="Memory" value={node.memory || '1GB'}>
            <option value="512MB">512MB</option>
            <option value="1GB">1GB</option>
            <option value="2GB">2GB</option>
            <option value="4GB">4GB</option>
          </Select>
        </>
      )}
      
      <div className="pt-4 space-y-2">
        <Button variant="primary" className="w-full">
          Update Configuration
        </Button>
        <Button
          variant="danger"
          className="w-full"
          onClick={() => onDeleteNode(node.id)}
        >
          Delete Component
        </Button>
      </div>
    </div>
  );

  const logsContent = (
    <div className="font-mono text-xs space-y-1">
      <div className="text-gray-400">
        [2024-06-25 08:05:50] Starting deployment...
      </div>
      <div className="text-blue-400">
        [2024-06-25 08:06:20] Pulling Docker image...
      </div>
      <div className="text-green-400">
        [2024-06-25 08:06:50] Container started successfully
      </div>
      <div className="text-gray-400">
        [2024-06-25 08:07:00] Health check passed
      </div>
      <div className="text-green-400">
        [2024-06-25 08:07:10] Deployment complete
      </div>
    </div>
  );

  const metricsContent = (
    <div className="space-y-4">
      <div className="bg-gray-700 p-4 rounded-lg">
        <h4 className="text-gray-300 text-sm mb-2">CPU Usage</h4>
        <div className="text-2xl text-purple-400">24%</div>
      </div>
      <div className="bg-gray-700 p-4 rounded-lg">
        <h4 className="text-gray-300 text-sm mb-2">Memory Usage</h4>
        <div className="text-2xl text-purple-400">512 MB / 1024 MB</div>
      </div>
      <div className="bg-gray-700 p-4 rounded-lg">
        <h4 className="text-gray-300 text-sm mb-2">Request Rate</h4>
        <div className="text-2xl text-purple-400">1.2k req/min</div>
      </div>
    </div>
  );

  const envContent = (
    <div className="space-y-4">
      <div>
        <label className="text-gray-400 text-sm">Environment Variables</label>
        <div className="mt-2 space-y-2">
          <div className="flex gap-2">
            <input
              className="flex-1 bg-gray-700 text-white px-3 py-2 rounded-lg text-sm border border-gray-600"
              placeholder="KEY"
            />
            <input
              className="flex-1 bg-gray-700 text-white px-3 py-2 rounded-lg text-sm border border-gray-600"
              placeholder="VALUE"
            />
            <IconButton
              icon={<X className="w-4 h-4" />}
              className="text-red-400 hover:text-red-300"
            />
          </div>
        </div>
        <button className="mt-2 text-purple-400 text-sm hover:text-purple-300">
          + Add Variable
        </button>
      </div>
    </div>
  );

  const tabs = [
    { id: 'settings', label: 'Settings', icon: Settings, content: settingsContent },
    { id: 'logs', label: 'Logs', icon: FileText, content: logsContent },
    { id: 'metrics', label: 'Metrics', icon: BarChart3, content: metricsContent },
    { id: 'env', label: 'Environment', icon: Terminal, content: envContent }
  ];

  return (
    <div className="w-96 bg-gray-800 border-l border-gray-700 flex flex-col">
      <div className="p-4 border-b border-gray-700 flex items-center justify-between">
        <div className="flex items-center gap-3">
          <Icon className="w-5 h-5" style={{ color }} />
          <div>
            <h2 className="text-white font-medium">{node.label}</h2>
            <p className="text-gray-400 text-sm">{node.project}</p>
          </div>
        </div>
        <IconButton
          icon={<X className="w-5 h-5" />}
          onClick={onClose}
        />
      </div>
      
      <Tabs
        tabs={tabs}
        activeTab={activeTab}
        onTabChange={onTabChange}
      />
    </div>
  );
};