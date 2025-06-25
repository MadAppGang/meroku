import React from 'react';
import { Handle, Position, NodeProps } from 'reactflow';
import { 
  Github, 
  Database, 
  Server, 
  Globe, 
  BarChart3, 
  Zap,
  CheckCircle,
  Clock,
  XCircle,
  AlertCircle,
  Copy
} from 'lucide-react';
import { ComponentNode } from '../types';

const serviceIcons = {
  frontend: Globe,
  backend: Github,
  database: Database,
  cache: Database,
  api: Server,
  analytics: BarChart3,
};

const serviceColors = {
  frontend: 'bg-yellow-500',
  backend: 'bg-blue-500',
  database: 'bg-red-500',
  cache: 'bg-red-500',
  api: 'bg-blue-400',
  analytics: 'bg-green-500',
};

const statusIcons = {
  running: CheckCircle,
  deploying: Clock,
  stopped: XCircle,
  error: AlertCircle,
};

const statusColors = {
  running: 'text-green-400',
  deploying: 'text-yellow-400',
  stopped: 'text-gray-400',
  error: 'text-red-400',
};

export function ServiceNode({ data, selected }: NodeProps<ComponentNode>) {
  const Icon = serviceIcons[data.type];
  const StatusIcon = statusIcons[data.status];
  const serviceColor = serviceColors[data.type];
  const statusColor = statusColors[data.status];

  return (
    <div className={`
      bg-gray-800 border-2 rounded-lg p-4 min-w-64 shadow-lg
      ${selected ? 'border-blue-500 shadow-blue-500/20' : 'border-gray-600'}
      hover:border-gray-500 transition-all duration-200
    `}>
      <Handle
        type="target"
        position={Position.Left}
        className="w-3 h-3 bg-gray-600 border-2 border-gray-700"
      />
      <Handle
        type="source"
        position={Position.Right}
        className="w-3 h-3 bg-gray-600 border-2 border-gray-700"
      />
      
      <div className="flex items-center gap-3 mb-2">
        <div className={`p-2 rounded-lg ${serviceColor}`}>
          <Icon className="w-4 h-4 text-white" />
        </div>
        <div className="flex-1">
          <h3 className="font-medium text-white">{data.name}</h3>
          {data.url && (
            <p className="text-sm text-gray-400 truncate">{data.url}</p>
          )}
        </div>
      </div>

      <div className="flex items-center gap-2 mb-2">
        <StatusIcon className={`w-4 h-4 ${statusColor}`} />
        <span className="text-sm text-gray-300">
          {data.description || 'No description'}
        </span>
      </div>

      {data.deploymentType && (
        <div className="text-xs text-gray-400 mb-2">
          {data.deploymentType}
        </div>
      )}

      {data.replicas && (
        <div className="flex items-center gap-2 text-xs text-gray-400">
          <Copy className="w-3 h-3" />
          <span>{data.replicas} Replicas</span>
        </div>
      )}

      {data.resources && (
        <div className="mt-2 p-2 bg-gray-700 rounded text-xs">
          <div className="flex justify-between items-center">
            <span className="text-gray-300">Resources:</span>
            <span className="text-gray-400">
              {data.resources.cpu}, {data.resources.memory}
            </span>
          </div>
        </div>
      )}
    </div>
  );
}