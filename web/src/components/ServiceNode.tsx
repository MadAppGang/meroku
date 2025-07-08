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
  Copy,
  Network,
  Package,
  Shield,
  Key,
  Mail,
  Bell,
  HardDrive,
  Cloud,
  Activity,
  Eye,
  Gauge,
  Siren,
  Users,
  Monitor,
  Smartphone,
  ShieldCheck,
  Timer,
  Calendar,
  Workflow
} from 'lucide-react';
import { ComponentNode } from '../types';

const serviceIcons = {
  frontend: Globe,
  backend: Server,
  database: Database,
  cache: Database,
  api: Server,
  analytics: BarChart3,
  infrastructure: Network,
  'container-registry': Package,
  route53: Network,
  waf: Shield,
  'api-gateway': Server,
  ecs: Package,
  ecr: Package,
  aurora: Database,
  eventbridge: Zap,
  'secrets-manager': Key,
  ses: Mail,
  sns: Bell,
  s3: HardDrive,
  amplify: Cloud,
  xray: Activity,
  cloudwatch: Eye,
  telemetry: Gauge,
  alarms: Siren,
  github: Github,
  auth: Users,
  'client-app': Users,
  'admin-app': Monitor,
  opa: ShieldCheck,
  'service-regular': Server,
  'service-periodic': Calendar,
  'service-event-driven': Workflow,
  'service': Server,
  'scheduled-task': Timer,
  'event-task': Zap,
  'postgres': Database,
  'sqs': Bell,
  'efs': HardDrive,
  'alb': Network,
  'appsync': Network,
};

const serviceColors = {
  frontend: 'bg-yellow-500',
  backend: 'bg-blue-500',
  database: 'bg-red-500',
  cache: 'bg-red-500',
  api: 'bg-blue-400',
  analytics: 'bg-green-500',
  infrastructure: 'bg-purple-500',
  'container-registry': 'bg-orange-500',
  route53: 'bg-purple-600',
  waf: 'bg-red-600',
  'api-gateway': 'bg-pink-600',
  ecs: 'bg-orange-600',
  ecr: 'bg-orange-500',
  aurora: 'bg-purple-500',
  eventbridge: 'bg-pink-500',
  'secrets-manager': 'bg-red-500',
  ses: 'bg-red-500',
  sns: 'bg-pink-500',
  s3: 'bg-green-600',
  amplify: 'bg-red-500',
  xray: 'bg-purple-500',
  cloudwatch: 'bg-pink-500',
  telemetry: 'bg-yellow-600',
  alarms: 'bg-blue-500',
  github: 'bg-gray-700',
  auth: 'bg-gray-800',
  'client-app': 'bg-indigo-700',
  'admin-app': 'bg-gray-600',
  opa: 'bg-gray-700',
  'service-regular': 'bg-blue-600',
  'service-periodic': 'bg-purple-600',
  'service-event-driven': 'bg-green-600',
  'service': 'bg-blue-600',
  'scheduled-task': 'bg-purple-600',
  'event-task': 'bg-green-600',
  'postgres': 'bg-indigo-600',
  'sqs': 'bg-yellow-600',
  'efs': 'bg-teal-600',
  'alb': 'bg-pink-600',
  'appsync': 'bg-purple-600',
};

const statusIcons = {
  running: CheckCircle,
  deploying: Clock,
  stopped: XCircle,
  error: AlertCircle,
  external: Globe,
};

const statusColors = {
  running: 'text-green-400',
  deploying: 'text-yellow-400',
  stopped: 'text-gray-400',
  error: 'text-red-400',
  external: 'text-blue-400',
};

export function ServiceNode({ data, selected }: NodeProps<ComponentNode>) {
  const Icon = serviceIcons[data.type];
  const StatusIcon = statusIcons[data.status];
  const serviceColor = serviceColors[data.type];
  const statusColor = statusColors[data.status];

  return (
    <div className={`
      border-2 rounded-lg p-4 min-w-64 shadow-lg
      ${data.isExternal 
        ? 'bg-gray-900 border-dashed' 
        : 'bg-gray-800'
      }
      ${selected 
        ? 'border-blue-500 shadow-blue-500/20' 
        : data.isExternal 
          ? 'border-gray-500' 
          : 'border-gray-600'
      }
      hover:border-gray-500 transition-all duration-200
      ${data.disabled ? 'opacity-50' : ''}
    `}>
      {/* Handles on all sides for flexible connections */}
      <Handle
        type="target"
        position={Position.Top}
        id="target-top"
        className="w-3 h-3 bg-gray-600 border-2 border-gray-700"
      />
      <Handle
        type="target"
        position={Position.Left}
        id="target-left"
        className="w-3 h-3 bg-gray-600 border-2 border-gray-700"
      />
      <Handle
        type="target"
        position={Position.Bottom}
        id="target-bottom"
        className="w-3 h-3 bg-gray-600 border-2 border-gray-700"
      />
      <Handle
        type="target"
        position={Position.Right}
        id="target-right"
        className="w-3 h-3 bg-gray-600 border-2 border-gray-700"
      />
      
      {/* Source handles */}
      <Handle
        type="source"
        position={Position.Top}
        id="source-top"
        className="w-3 h-3 bg-gray-600 border-2 border-gray-700"
      />
      <Handle
        type="source"
        position={Position.Left}
        id="source-left"
        className="w-3 h-3 bg-gray-600 border-2 border-gray-700"
      />
      <Handle
        type="source"
        position={Position.Bottom}
        id="source-bottom"
        className="w-3 h-3 bg-gray-600 border-2 border-gray-700"
      />
      <Handle
        type="source"
        position={Position.Right}
        id="source-right"
        className="w-3 h-3 bg-gray-600 border-2 border-gray-700"
      />
      
      <div className="flex items-center gap-3 mb-2">
        <div className={`p-2 rounded-lg ${serviceColor} ${data.disabled ? 'opacity-60' : ''}`}>
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
        <StatusIcon className={`w-4 h-4 ${statusColor} ${data.disabled ? 'opacity-60' : ''}`} />
        <span className="text-sm text-gray-300">
          {data.description || 'No description'}
        </span>
      </div>

      {data.isExternal && (
        <div className="text-xs text-blue-400 mb-2 italic">
          External to infrastructure
        </div>
      )}

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

      {data.group && (
        <div className="mt-2 flex items-center gap-1 text-xs text-gray-500">
          <Network className="w-3 h-3" />
          <span>{data.group}</span>
          {data.subgroup && <span>/ {data.subgroup}</span>}
        </div>
      )}
      
      {data.hasTelemetry && (
        <div className="absolute -top-2 -right-2 bg-purple-600 rounded-full p-1">
          <Gauge className="w-3 h-3 text-white" />
        </div>
      )}
      
      {/* Instance count badges for services */}
      {(['backend', 'service'].includes(data.type)) && data.configProperties && (
        <div className="absolute -top-2 -left-2 flex flex-col gap-1">
          {/* Instance count badge */}
          <div className="bg-blue-600 text-white rounded-full px-2 py-0.5 text-xs font-medium flex items-center gap-1">
            <Copy className="w-3 h-3" />
            {data.configProperties.desiredCount || 1}
          </div>
          
          {/* Additional badges in a row below */}
          <div className="flex gap-1">
            {/* Autoscaling badge (backend service only) */}
            {data.type === 'backend' && data.configProperties.autoscalingEnabled && (
              <div className="bg-green-600 text-white rounded-full px-2 py-0.5 text-xs font-medium flex items-center gap-1">
                <Activity className="w-3 h-3" />
                Auto
              </div>
            )}


          </div>
        </div>
      )}
    </div>
  );
}