import { Handle, Position, NodeProps } from 'reactflow';
import { 
  Github, 
  Database, 
  Server, 
  Globe, 
  BarChart3, 
  Zap,
  Clock,
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
  ShieldCheck,
  Timer,
  Calendar,
  Workflow,
  GitBranch
} from 'lucide-react';
import { ComponentNode } from '../types';
import { AmplifyStatusWidget } from './AmplifyStatusWidget';

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


export function ServiceNode({ data, selected }: NodeProps<ComponentNode>) {
  const Icon = serviceIcons[data.type];
  const serviceColor = serviceColors[data.type];
  
  // Determine if this is a service node that needs extended display
  const isServiceNode = ['backend', 'service', 'service-regular', 'service-periodic', 'service-event-driven', 
                        'scheduled-task', 'event-task', 'amplify'].includes(data.type);
  const minWidth = 'min-w-64'; // Keep compact width

  return (
    <div className={`
      border-2 rounded-lg p-4 ${minWidth} shadow-lg
      ${data.isExternal 
        ? 'bg-gray-900 border-dashed' 
        : data.type === 'amplify'
          ? 'bg-gradient-to-br from-red-950/20 to-gray-900 border-2'
          : isServiceNode 
            ? 'bg-gradient-to-br from-gray-800 to-gray-900 border-2'
            : 'bg-gray-800'
      }
      ${selected 
        ? 'border-blue-500 shadow-blue-500/20' 
        : data.isExternal 
          ? 'border-gray-500' 
          : isServiceNode
            ? data.type === 'backend' 
              ? 'border-blue-600/50 shadow-blue-900/20'
              : data.type === 'service'
                ? 'border-purple-600/50 shadow-purple-900/20'
                : data.type === 'scheduled-task'
                  ? 'border-green-600/50 shadow-green-900/20'
                  : data.type === 'event-task'
                    ? 'border-orange-600/50 shadow-orange-900/20'
                    : data.type === 'amplify'
                      ? 'border-red-600/50 shadow-red-900/20'
                      : 'border-gray-600'
            : 'border-gray-600'
      }
      hover:border-gray-400 transition-all duration-200
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
      
      {/* Service type indicator for service nodes */}
      {isServiceNode && (
        <div className={`absolute -top-1 -right-1 w-2 h-2 rounded-full
          ${data.type === 'backend' ? 'bg-blue-500' :
            data.type === 'service' ? 'bg-purple-500' :
            data.type === 'scheduled-task' ? 'bg-green-500' :
            data.type === 'event-task' ? 'bg-orange-500' :
            'bg-gray-500'
          }`} 
        />
      )}
      
      <div className="flex items-center gap-3 mb-2">
        <div className={`p-2 rounded-lg ${serviceColor} ${data.disabled ? 'opacity-60' : ''}`}>
          <Icon className="w-4 h-4 text-white" />
        </div>
        <div className="flex-1">
          <div className="flex items-center gap-2">
            <h3 className="font-medium text-white">{data.name}</h3>
            {data.type === 'amplify' && (
              <span className="px-2 py-0.5 text-xs font-medium rounded-full bg-yellow-500/20 text-yellow-400 border border-yellow-500/30">
                BETA
              </span>
            )}
          </div>
          {data.url && (
            <p className="text-sm text-gray-400 truncate">{data.url}</p>
          )}
        </div>
      </div>

      {data.description && (
        <div className="flex items-center gap-2 mb-2">
          <span className="text-xs text-gray-400">
            {data.description}
          </span>
        </div>
      )}
      
      {/* Extended information for service nodes */}
      {isServiceNode && data.configProperties && (
        <div className="space-y-2 mt-2">
          {/* Domain for backend/services */}
          {(data.type === 'backend' || data.type === 'service') && data.configProperties.domain && (
            <div className="text-xs bg-gray-900/50 rounded px-2 py-1">
              <div className="flex items-center gap-1 text-gray-400">
                <Globe className="w-3 h-3" />
                <span className="font-mono truncate">{data.configProperties.domain}</span>
              </div>
            </div>
          )}
          
          {/* Schedule for scheduled tasks */}
          {data.type === 'scheduled-task' && data.configProperties.schedule && (
            <div className="text-xs bg-gray-900/50 rounded px-2 py-1">
              <div className="flex items-center gap-1 text-gray-400">
                <Clock className="w-3 h-3" />
                <span className="font-mono">{data.configProperties.schedule}</span>
              </div>
            </div>
          )}
          
          {/* Event rules for event tasks */}
          {data.type === 'event-task' && data.configProperties.sources && (
            <div className="text-xs bg-gray-900/50 rounded px-2 py-1 space-y-1">
              <div className="flex items-center gap-1 text-gray-400">
                <Zap className="w-3 h-3" />
                <span>Event Rules:</span>
              </div>
              {data.configProperties.sources && (
                <div className="text-gray-300 ml-4">
                  Source: {data.configProperties.sources.join(', ')}
                </div>
              )}
              {data.configProperties.detailTypes && (
                <div className="text-gray-300 ml-4 truncate">
                  Types: {data.configProperties.detailTypes.join(', ')}
                </div>
              )}
            </div>
          )}
          
          {/* Amplify-specific information */}
          {data.type === 'amplify' && data.configProperties && (
            <>
              {/* Repository info */}
              {data.configProperties.repository && (
                <div className="text-xs bg-gray-900/50 rounded px-2 py-1 mb-2">
                  <div className="flex items-center gap-1 text-gray-400">
                    <Github className="w-3 h-3" />
                    <span className="truncate">{data.configProperties.repository}</span>
                  </div>
                </div>
              )}
              
              {/* Custom Domain from YAML config */}
              {data.configProperties.customDomain && (
                <div className="text-xs bg-gray-900/50 rounded px-2 py-1 mb-2">
                  <div className="flex items-center gap-1 text-gray-400">
                    <Globe className="w-3 h-3" />
                    <span className="text-gray-500">Custom:</span>
                    <span className="text-white ml-1">{data.configProperties.customDomain}</span>
                  </div>
                </div>
              )}
              
              {/* Branch Info */}
              {data.configProperties.branches && data.configProperties.branches.length > 0 && (
                <div className="text-xs bg-gray-900/50 rounded px-2 py-1 mb-2">
                  <div className="flex items-center gap-1 text-gray-400">
                    <GitBranch className="w-3 h-3" />
                    <span>{data.configProperties.branches.length} branch{data.configProperties.branches.length !== 1 ? 'es' : ''}</span>
                    {data.configProperties.branch && (
                      <span className="text-white ml-1">({data.configProperties.branch})</span>
                    )}
                  </div>
                </div>
              )}
              
              {/* Amplify Status Widget */}
              <div className="bg-gray-900/50 rounded px-2 py-1">
                <div className="text-xs text-gray-400 mb-1">Build Status:</div>
                <AmplifyStatusWidget
                  appName={data.name}
                  environment={data.configProperties.environment || 'dev'}
                  profile={data.configProperties.profile}
                  variant="compact"
                  showRefresh={false}
                  autoRefresh={true}
                  className="w-full"
                />
              </div>
            </>
          )}
          
          {/* Resources - show for all service types except Amplify */}
          {data.type !== 'amplify' && (
          <div className="bg-gray-900/50 rounded px-2 py-1">
            <div className="flex items-center justify-between text-xs">
              <div className="flex items-center gap-2">
                <span className="text-gray-400">Resources:</span>
                {data.type === 'backend' && data.configProperties.autoscalingEnabled ? (
                  <div className="flex items-center gap-0.5">
                    <Activity className="w-3 h-3 text-green-500" />
                    <span className="text-white font-medium">
                      {data.configProperties.autoscalingMinCapacity || 1}-{data.configProperties.autoscalingMaxCapacity || 10} instances
                    </span>
                  </div>
                ) : (
                  data.configProperties.desiredCount && (
                    <div className="flex items-center gap-0.5">
                      <Copy className="w-3 h-3 text-gray-500" />
                      <span className="text-white font-medium">
                        {data.configProperties.desiredCount} instance{data.configProperties.desiredCount !== 1 ? 's' : ''}
                      </span>
                    </div>
                  )
                )}
              </div>
              <div className="flex items-center gap-2 text-gray-300">
                <span>{data.configProperties.cpu || '0.25'} vCPU</span>
                <span className="text-gray-500">•</span>
                <span>{data.configProperties.memory || '512MB'}</span>
              </div>
            </div>
          </div>
          )}
          
          {/* Health Status - compact inline */}
          {data.configProperties.healthStatus && (
            <div className="flex items-center gap-3 text-xs">
              {data.configProperties.healthStatus.critical && (
                <div className="flex items-center gap-0.5 text-red-400">
                  <AlertCircle className="w-3 h-3" />
                  <span>Critical</span>
                </div>
              )}
              {data.configProperties.healthStatus.monitored && (
                <div className="flex items-center gap-0.5 text-green-400">
                  <Activity className="w-3 h-3" />
                  <span>Monitored</span>
                </div>
              )}
            </div>
          )}
        </div>
      )}

      {data.isExternal && (
        <div className="text-xs text-blue-400 mb-2 italic">
          External to infrastructure
        </div>
      )}

      {data.deploymentType && !isServiceNode && (
        <div className="text-xs text-gray-400 mb-2">
          {data.deploymentType}
        </div>
      )}

      {data.replicas && !isServiceNode && (
        <div className="flex items-center gap-2 text-xs text-gray-400">
          <Copy className="w-3 h-3" />
          <span>{data.replicas} Replicas</span>
        </div>
      )}

      {data.resources && !isServiceNode && (
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
          {/* Show autoscaling badge OR instance count badge, not both */}
          {data.type === 'backend' && data.configProperties.autoscalingEnabled ? (
            /* Autoscaling badge with range */
            <div className="bg-green-600 text-white rounded-full px-2 py-0.5 text-xs font-medium flex items-center gap-1">
              <Activity className="w-3 h-3" />
              {data.configProperties.autoscalingMinCapacity || 1}-{data.configProperties.autoscalingMaxCapacity || 10}
            </div>
          ) : (
            /* Instance count badge */
            <div className="bg-blue-600 text-white rounded-full px-2 py-0.5 text-xs font-medium flex items-center gap-1">
              <Copy className="w-3 h-3" />
              {data.configProperties.desiredCount || 1}
            </div>
          )}
        </div>
      )}
    </div>
  );
}