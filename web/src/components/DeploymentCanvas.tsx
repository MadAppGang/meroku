import React, { useCallback, useMemo, useEffect } from 'react';
import ReactFlow, {
  Background,
  Controls,
  MiniMap,
  Node,
  Edge,
  useNodesState,
  useEdgesState,
  addEdge,
  Connection,
  MarkerType,
  NodeProps,
} from 'reactflow';
import 'reactflow/dist/style.css';
import { ServiceNode } from './ServiceNode';
import { GroupNode } from './GroupNode';
import { CanvasControls } from './CanvasControls';
import { ComponentNode } from '../types';
import { layoutNodesWithGroups } from '../utils/layoutUtils';

interface DeploymentCanvasProps {
  onNodeSelect: (node: ComponentNode | null) => void;
  selectedNode: ComponentNode | null;
}

const nodeTypes = {
  service: ServiceNode,
  group: GroupNode,
};

const initialNodes: Node[] = [
  // GitHub Actions
  {
    id: 'github',
    type: 'service',
    position: { x: 700, y: 20 },
    data: {
      id: 'github',
      type: 'github',
      name: 'GitHub actions',
      status: 'running',
      deletable: false,
    },
    deletable: false,
  },
  
  // Client Applications
  {
    id: 'client-app',
    type: 'service',
    position: { x: 50, y: 400 },
    data: {
      id: 'client-app',
      type: 'client-app',
      name: 'Client app',
      status: 'running',
    },
  },
  {
    id: 'admin-app',
    type: 'service',
    position: { x: 50, y: 550 },
    data: {
      id: 'admin-app',
      type: 'admin-app',
      name: 'Admin app',
      status: 'running',
    },
  },
  
  // Entry Points
  {
    id: 'route53',
    type: 'service',
    position: { x: 400, y: 200 },
    data: {
      id: 'route53',
      type: 'route53',
      name: 'Amazon Route 53',
      status: 'running',
      deletable: false,
    },
    deletable: false,
  },
  {
    id: 'waf',
    type: 'service',
    position: { x: 300, y: 400 },
    data: {
      id: 'waf',
      type: 'waf',
      name: 'AWS WAF',
      status: 'running',
      deletable: false,
    },
    deletable: false,
  },
  
  // API Gateway
  {
    id: 'api-gateway',
    type: 'service',
    position: { x: 500, y: 400 },
    data: {
      id: 'api-gateway',
      type: 'api-gateway',
      name: 'Amazon API Gateway',
      status: 'running',
      deletable: false,
    },
    deletable: false,
  },
  
  // ECS Cluster Group
  {
    id: 'ecs-cluster-group',
    type: 'group',
    position: { x: 550, y: 150 },
    data: {
      label: 'ECS Cluster',
    },
    style: {
      zIndex: -1,
      width: 750,
      height: 350,
    },
    draggable: false,
    selectable: false,
  },
  
  // ECS Services
  {
    id: 'ecs',
    type: 'service',
    position: { x: 600, y: 200 },
    data: {
      id: 'ecs',
      type: 'ecs',
      name: 'Amazon ECS Cluster',
      status: 'running',
      deletable: false,
    },
    deletable: false,
  },
  {
    id: 'ecr',
    type: 'service',
    position: { x: 750, y: 200 },
    data: {
      id: 'ecr',
      type: 'ecr',
      name: 'Amazon ECR',
      status: 'running',
      deletable: false,
    },
    deletable: false,
  },
  {
    id: 'aurora',
    type: 'service',
    position: { x: 900, y: 200 },
    data: {
      id: 'aurora',
      type: 'aurora',
      name: 'Amazon Aurora Serverless V2',
      status: 'running',
      deletable: false,
    },
    deletable: false,
  },
  {
    id: 'eventbridge',
    type: 'service',
    position: { x: 1050, y: 200 },
    data: {
      id: 'eventbridge',
      type: 'eventbridge',
      name: 'Amazon EventBridge',
      status: 'running',
      deletable: false,
    },
    deletable: false,
  },
  
  // Backend Services in ECS
  {
    id: 'backend-service',
    type: 'service',
    position: { x: 600, y: 320 },
    data: {
      id: 'backend-service',
      type: 'backend',
      name: 'Backend service',
      description: '1-n instances',
      status: 'running',
      group: 'ECS',
    },
  },
  {
    id: 'scheduled-service',
    type: 'service',
    position: { x: 750, y: 320 },
    data: {
      id: 'scheduled-service',
      type: 'backend',
      name: 'Scheduled service',
      status: 'running',
      group: 'ECS',
    },
  },
  {
    id: 'analytics-service',
    type: 'service',
    position: { x: 900, y: 320 },
    data: {
      id: 'analytics-service',
      type: 'analytics',
      name: 'Analytics services',
      status: 'running',
      group: 'ECS',
    },
  },
  {
    id: 'opa',
    type: 'service',
    position: { x: 1050, y: 320 },
    data: {
      id: 'opa',
      type: 'opa',
      name: 'Open Policy Agent',
      status: 'running',
      group: 'ECS',
    },
  },
  
  // Supporting Services
  {
    id: 'secrets-manager',
    type: 'service',
    position: { x: 600, y: 550 },
    data: {
      id: 'secrets-manager',
      type: 'secrets-manager',
      name: 'AWS Secrets Manager',
      status: 'running',
      deletable: false,
    },
    deletable: false,
  },
  {
    id: 'ses',
    type: 'service',
    position: { x: 750, y: 550 },
    data: {
      id: 'ses',
      type: 'ses',
      name: 'Amazon SES',
      status: 'running',
      deletable: false,
    },
    deletable: false,
  },
  {
    id: 'sns',
    type: 'service',
    position: { x: 900, y: 550 },
    data: {
      id: 'sns',
      type: 'sns',
      name: 'Amazon SNS',
      status: 'running',
      deletable: false,
    },
    deletable: false,
  },
  {
    id: 's3',
    type: 'service',
    position: { x: 1050, y: 550 },
    data: {
      id: 's3',
      type: 's3',
      name: 'Amazon S3',
      status: 'running',
      deletable: false,
    },
    deletable: false,
  },
  
  // Authentication
  {
    id: 'auth-system',
    type: 'service',
    position: { x: 450, y: 550 },
    data: {
      id: 'auth-system',
      type: 'auth',
      name: 'Authentication system',
      status: 'running',
    },
  },
  
  // Frontend Distribution
  {
    id: 'amplify',
    type: 'service',
    position: { x: 650, y: 700 },
    data: {
      id: 'amplify',
      type: 'amplify',
      name: 'AWS Amplify',
      description: 'Frontend distribution',
      status: 'running',
      deletable: false,
    },
    deletable: false,
  },
  
  // Observability Group
  {
    id: 'observability-group',
    type: 'group',
    position: { x: 1350, y: 150 },
    data: {
      label: 'Observability',
    },
    style: {
      zIndex: -1,
      width: 350,
      height: 350,
    },
    draggable: false,
    selectable: false,
  },
  
  // Observability Services
  {
    id: 'xray',
    type: 'service',
    position: { x: 1400, y: 200 },
    data: {
      id: 'xray',
      type: 'xray',
      name: 'AWS X-Ray',
      status: 'running',
      deletable: false,
    },
    deletable: false,
  },
  {
    id: 'cloudwatch',
    type: 'service',
    position: { x: 1550, y: 200 },
    data: {
      id: 'cloudwatch',
      type: 'cloudwatch',
      name: 'Amazon CloudWatch',
      status: 'running',
      deletable: false,
    },
    deletable: false,
  },
  {
    id: 'telemetry',
    type: 'service',
    position: { x: 1400, y: 320 },
    data: {
      id: 'telemetry',
      type: 'telemetry',
      name: 'Open telemetry collector',
      status: 'running',
    },
  },
  {
    id: 'alarms',
    type: 'service',
    position: { x: 1550, y: 320 },
    data: {
      id: 'alarms',
      type: 'alarms',
      name: 'Alarm rules',
      status: 'running',
    },
  },
];

const initialEdges: Edge[] = [
  // GitHub to ECR
  {
    id: 'github-ecr',
    source: 'github',
    target: 'ecr',
    type: 'smoothstep',
    animated: true,
    style: { stroke: '#6b7280', strokeWidth: 2 },
    markerEnd: { type: MarkerType.ArrowClosed, color: '#6b7280' },
  },
  
  // Client Apps to WAF
  {
    id: 'client-waf',
    source: 'client-app',
    target: 'waf',
    type: 'smoothstep',
    animated: true,
    label: 'API access',
    style: { stroke: '#4f46e5', strokeWidth: 2 },
    markerEnd: { type: MarkerType.ArrowClosed, color: '#4f46e5' },
  },
  {
    id: 'admin-waf',
    source: 'admin-app',
    target: 'waf',
    type: 'smoothstep',
    animated: true,
    style: { stroke: '#4f46e5', strokeWidth: 2 },
    markerEnd: { type: MarkerType.ArrowClosed, color: '#4f46e5' },
  },
  
  // WAF to API Gateway
  {
    id: 'waf-api',
    source: 'waf',
    target: 'api-gateway',
    type: 'smoothstep',
    animated: true,
    style: { stroke: '#4f46e5', strokeWidth: 2 },
    markerEnd: { type: MarkerType.ArrowClosed, color: '#4f46e5' },
  },
  
  // Route53 to API Gateway
  {
    id: 'route53-api',
    source: 'route53',
    target: 'api-gateway',
    type: 'smoothstep',
    animated: true,
    label: 'Cloud map',
    style: { stroke: '#8b5cf6', strokeWidth: 2 },
    markerEnd: { type: MarkerType.ArrowClosed, color: '#8b5cf6' },
  },
  
  // API Gateway to Backend
  {
    id: 'api-backend',
    source: 'api-gateway',
    target: 'backend-service',
    type: 'smoothstep',
    animated: true,
    style: { stroke: '#4f46e5', strokeWidth: 2 },
    markerEnd: { type: MarkerType.ArrowClosed, color: '#4f46e5' },
  },
  
  // Authentication flows
  {
    id: 'client-auth',
    source: 'client-app',
    target: 'auth-system',
    type: 'smoothstep',
    animated: true,
    label: 'authenticate',
    style: { stroke: '#6b7280', strokeWidth: 2 },
    markerEnd: { type: MarkerType.ArrowClosed, color: '#6b7280' },
  },
  {
    id: 'admin-auth',
    source: 'admin-app',
    target: 'auth-system',
    type: 'smoothstep',
    animated: true,
    style: { stroke: '#6b7280', strokeWidth: 2 },
    markerEnd: { type: MarkerType.ArrowClosed, color: '#6b7280' },
  },
  {
    id: 'auth-api',
    source: 'auth-system',
    target: 'api-gateway',
    type: 'smoothstep',
    animated: true,
    label: 'JWT/OIDC',
    style: { stroke: '#6b7280', strokeWidth: 2 },
    markerEnd: { type: MarkerType.ArrowClosed, color: '#6b7280' },
  },
  
  // Backend to services
  {
    id: 'backend-secrets',
    source: 'backend-service',
    target: 'secrets-manager',
    type: 'smoothstep',
    animated: true,
    style: { stroke: '#4f46e5', strokeWidth: 2 },
    markerEnd: { type: MarkerType.ArrowClosed, color: '#4f46e5' },
  },
  {
    id: 'backend-ses',
    source: 'backend-service',
    target: 'ses',
    type: 'smoothstep',
    animated: true,
    style: { stroke: '#4f46e5', strokeWidth: 2 },
    markerEnd: { type: MarkerType.ArrowClosed, color: '#4f46e5' },
  },
  {
    id: 'backend-sns',
    source: 'backend-service',
    target: 'sns',
    type: 'smoothstep',
    animated: true,
    style: { stroke: '#4f46e5', strokeWidth: 2 },
    markerEnd: { type: MarkerType.ArrowClosed, color: '#4f46e5' },
  },
  {
    id: 'backend-s3',
    source: 'backend-service',
    target: 's3',
    type: 'smoothstep',
    animated: true,
    style: { stroke: '#4f46e5', strokeWidth: 2 },
    markerEnd: { type: MarkerType.ArrowClosed, color: '#4f46e5' },
  },
  
  // Scheduled service connections
  {
    id: 'scheduled-ses',
    source: 'scheduled-service',
    target: 'ses',
    type: 'smoothstep',
    animated: true,
    style: { stroke: '#4f46e5', strokeWidth: 2 },
    markerEnd: { type: MarkerType.ArrowClosed, color: '#4f46e5' },
  },
  {
    id: 'scheduled-sns',
    source: 'scheduled-service',
    target: 'sns',
    type: 'smoothstep',
    animated: true,
    style: { stroke: '#4f46e5', strokeWidth: 2 },
    markerEnd: { type: MarkerType.ArrowClosed, color: '#4f46e5' },
  },
  
  // Analytics service connections
  {
    id: 'analytics-sns',
    source: 'analytics-service',
    target: 'sns',
    type: 'smoothstep',
    animated: true,
    style: { stroke: '#4f46e5', strokeWidth: 2 },
    markerEnd: { type: MarkerType.ArrowClosed, color: '#4f46e5' },
  },
  {
    id: 'analytics-s3',
    source: 'analytics-service',
    target: 's3',
    type: 'smoothstep',
    animated: true,
    style: { stroke: '#4f46e5', strokeWidth: 2 },
    markerEnd: { type: MarkerType.ArrowClosed, color: '#4f46e5' },
  },
  
  // Frontend to Amplify
  {
    id: 'client-amplify',
    source: 'client-app',
    target: 'amplify',
    type: 'smoothstep',
    animated: true,
    style: { stroke: '#6b7280', strokeWidth: 2 },
    markerEnd: { type: MarkerType.ArrowClosed, color: '#6b7280' },
  },
  {
    id: 'admin-amplify',
    source: 'admin-app',
    target: 'amplify',
    type: 'smoothstep',
    animated: true,
    style: { stroke: '#6b7280', strokeWidth: 2 },
    markerEnd: { type: MarkerType.ArrowClosed, color: '#6b7280' },
  },
  
  // Observability connections
  {
    id: 'backend-xray',
    source: 'backend-service',
    target: 'xray',
    type: 'smoothstep',
    animated: true,
    style: { stroke: '#8b5cf6', strokeWidth: 2, strokeDasharray: '5,5' },
    markerEnd: { type: MarkerType.ArrowClosed, color: '#8b5cf6' },
  },
  {
    id: 'backend-telemetry',
    source: 'backend-service',
    target: 'telemetry',
    type: 'smoothstep',
    animated: true,
    style: { stroke: '#8b5cf6', strokeWidth: 2, strokeDasharray: '5,5' },
    markerEnd: { type: MarkerType.ArrowClosed, color: '#8b5cf6' },
  },
  {
    id: 'scheduled-telemetry',
    source: 'scheduled-service',
    target: 'telemetry',
    type: 'smoothstep',
    animated: true,
    style: { stroke: '#8b5cf6', strokeWidth: 2, strokeDasharray: '5,5' },
    markerEnd: { type: MarkerType.ArrowClosed, color: '#8b5cf6' },
  },
  {
    id: 'analytics-telemetry',
    source: 'analytics-service',
    target: 'telemetry',
    type: 'smoothstep',
    animated: true,
    style: { stroke: '#8b5cf6', strokeWidth: 2, strokeDasharray: '5,5' },
    markerEnd: { type: MarkerType.ArrowClosed, color: '#8b5cf6' },
  },
];

export function DeploymentCanvas({ onNodeSelect, selectedNode }: DeploymentCanvasProps) {
  // Apply layout algorithm to prevent overlaps
  const layoutAdjustedNodes = useMemo(() => layoutNodesWithGroups(initialNodes), []);
  
  const [nodes, setNodes, onNodesChange] = useNodesState(layoutAdjustedNodes);
  const [edges, setEdges, onEdgesChange] = useEdgesState(initialEdges);
  
  // Function to manually trigger layout
  const handleAutoLayout = useCallback(() => {
    const adjustedNodes = layoutNodesWithGroups(nodes);
    setNodes(adjustedNodes);
  }, [nodes, setNodes]);

  const onConnect = useCallback(
    (params: Connection) => setEdges((eds) => addEdge(params, eds)),
    [setEdges]
  );

  const onNodeClick = useCallback(
    (event: React.MouseEvent, node: Node) => {
      if (node.type === 'service') {
        onNodeSelect(node.data as ComponentNode);
      }
    },
    [onNodeSelect]
  );

  const onPaneClick = useCallback(() => {
    onNodeSelect(null);
  }, [onNodeSelect]);

  // Update nodes to show selection state
  const nodesWithSelection = useMemo(() => {
    return nodes.map(node => ({
      ...node,
      selected: selectedNode?.id === node.id,
    }));
  }, [nodes, selectedNode]);

  return (
    <div className="size-full">
      <ReactFlow
        nodes={nodesWithSelection}
        edges={edges}
        onNodesChange={onNodesChange}
        onEdgesChange={onEdgesChange}
        onConnect={onConnect}
        onNodeClick={onNodeClick}
        onPaneClick={onPaneClick}
        nodeTypes={nodeTypes}
        fitView
        fitViewOptions={{
          padding: 0.3,
          includeHiddenNodes: false,
          minZoom: 0.3,
          maxZoom: 1,
        }}
        defaultViewport={{ x: 0, y: 0, zoom: 0.4 }}
        className="bg-gray-950"
        proOptions={{ hideAttribution: true }}
      >
        <Background 
          color="#374151" 
          gap={20} 
          size={1}
          variant="dots"
        />
        <MiniMap 
          nodeColor="#4f46e5"
          nodeStrokeWidth={3}
          className="bg-gray-800 border border-gray-700 rounded-lg"
        />
        <CanvasControls onAutoLayout={handleAutoLayout} />
      </ReactFlow>
    </div>
  );
}