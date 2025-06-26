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
import { DynamicGroupNode } from './DynamicGroupNode';
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
  dynamicGroup: DynamicGroupNode,
};

const initialNodes: Node[] = [
  // GitHub Actions (top)
  {
    id: 'github',
    type: 'service',
    position: { x: 230, y: -80 },
    data: {
      id: 'github',
      type: 'github',
      name: 'GitHub actions',
      status: 'running',
      deletable: false,
    },
    deletable: false,
  },
  
  // Client Applications (left)
  {
    id: 'client-app',
    type: 'service',
    position: { x: -639, y: 388 },
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
    position: { x: -637, y: 538 },
    data: {
      id: 'admin-app',
      type: 'admin-app',
      name: 'Admin app',
      status: 'running',
    },
  },
  
  // Entry Points (left-middle)
  {
    id: 'route53',
    type: 'service',
    position: { x: -131, y: 158 },
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
    position: { x: -81, y: 368 },
    data: {
      id: 'waf',
      type: 'waf',
      name: 'AWS WAF',
      status: 'running',
      deletable: false,
    },
    deletable: false,
  },
  {
    id: 'api-gateway',
    type: 'service',
    position: { x: -83, y: 518 },
    data: {
      id: 'api-gateway',
      type: 'api-gateway',
      name: 'Amazon API Gateway',
      status: 'running',
      deletable: false,
    },
    deletable: false,
  },
  
  // Main ECS Cluster Group (center) - contains all ECS-related elements
  {
    id: 'ecs-cluster-group',
    type: 'dynamicGroup',
    position: { x: 0, y: 0 }, // Position will be calculated
    data: {
      label: 'ECS Cluster',
      nodeIds: ['ecs-cluster', 'backend-service', 'xray', 'cloudwatch', 'alarms'],
    },
    style: {
      zIndex: -3,
      backgroundColor: 'rgba(59, 130, 246, 0.05)',
      border: '2px solid #3b82f6',
    },
    draggable: false,
    selectable: false,
  },
  
  // ECS Cluster node (center-top)
  {
    id: 'ecs-cluster',
    type: 'service',
    position: { x: 284, y: 283 },
    data: {
      id: 'ecs-cluster',
      type: 'ecs',
      name: 'Amazon ECS Cluster',
      status: 'running',
      deletable: false,
      group: 'ECS Cluster',
    },
    deletable: false,
  },
  
  // ECR (standalone - not in ECS cluster)
  {
    id: 'ecr',
    type: 'service',
    position: { x: 280, y: 110 },
    data: {
      id: 'ecr',
      type: 'ecr',
      name: 'Amazon ECR',
      status: 'running',
      deletable: false,
    },
    deletable: false,
  },
  
  
  // Services in the services subgroup
  {
    id: 'backend-service',
    type: 'service',
    position: { x: 292, y: 459 },
    data: {
      id: 'backend-service',
      type: 'backend',
      name: 'Backend service',
      description: 'Main backend (required)',
      status: 'running',
      group: 'ECS Cluster',
      subgroup: 'Services',
      hasTelemetry: true,
    },
  },
  
  // Aurora DB (standalone - not in ECS cluster)
  {
    id: 'aurora',
    type: 'service',
    position: { x: 600, y: 110 },
    data: {
      id: 'aurora',
      type: 'aurora',
      name: 'Amazon Aurora',
      status: 'running',
      deletable: false,
    },
    deletable: false,
  },
  
  // EventBridge (standalone - not in ECS cluster)
  {
    id: 'eventbridge',
    type: 'service',
    position: { x: 290, y: 620 },
    data: {
      id: 'eventbridge',
      type: 'eventbridge',
      name: 'Amazon EventBridge',
      status: 'running',
      deletable: false,
    },
    deletable: false,
  },
  
  
  // Observability Services
  {
    id: 'xray',
    type: 'service',
    position: { x: 860, y: 280 },
    data: {
      id: 'xray',
      type: 'xray',
      name: 'AWS X-Ray',
      status: 'running',
      deletable: false,
      group: 'ECS Cluster',
      subgroup: 'Observability',
    },
    deletable: false,
  },
  {
    id: 'cloudwatch',
    type: 'service',
    position: { x: 580, y: 280 },
    data: {
      id: 'cloudwatch',
      type: 'cloudwatch',
      name: 'Amazon CloudWatch',
      status: 'running',
      deletable: false,
      group: 'ECS Cluster',
      subgroup: 'Observability',
    },
    deletable: false,
  },
  {
    id: 'alarms',
    type: 'service',
    position: { x: 1140, y: 280 },
    data: {
      id: 'alarms',
      type: 'alarms',
      name: 'Alarm rules',
      status: 'running',
      group: 'ECS Cluster',
      subgroup: 'Observability',
    },
  },
  
  // Supporting Services (right side)
  {
    id: 'secrets-manager',
    type: 'service',
    position: { x: 900, y: 110 },
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
    position: { x: 1170, y: 620 },
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
    position: { x: 580, y: 620 },
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
    position: { x: 880, y: 620 },
    data: {
      id: 's3',
      type: 's3',
      name: 'Amazon S3',
      status: 'running',
      deletable: false,
    },
    deletable: false,
  },
  
  // Authentication (bottom-left)
  {
    id: 'auth-system',
    type: 'service',
    position: { x: -71, y: 818 },
    data: {
      id: 'auth-system',
      type: 'auth',
      name: 'Authentication system',
      status: 'running',
    },
  },
  
  // Frontend Distribution (bottom)
  {
    id: 'amplify',
    type: 'service',
    position: { x: -76, y: 668 },
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
  
  // Entry points to ECS Cluster
  {
    id: 'route53-ecs',
    source: 'route53',
    target: 'ecs-cluster',
    type: 'smoothstep',
    animated: true,
    label: 'DNS',
    style: { stroke: '#8b5cf6', strokeWidth: 2 },
    markerEnd: { type: MarkerType.ArrowClosed, color: '#8b5cf6' },
  },
  {
    id: 'waf-ecs',
    source: 'waf',
    target: 'ecs-cluster',
    type: 'smoothstep',
    animated: true,
    style: { stroke: '#4f46e5', strokeWidth: 2 },
    markerEnd: { type: MarkerType.ArrowClosed, color: '#4f46e5' },
  },
  {
    id: 'api-ecs',
    source: 'api-gateway',
    target: 'ecs-cluster',
    type: 'smoothstep',
    animated: true,
    style: { stroke: '#4f46e5', strokeWidth: 2 },
    markerEnd: { type: MarkerType.ArrowClosed, color: '#4f46e5' },
  },
  
  // ECS to Supporting Services
  {
    id: 'ecs-secrets',
    source: 'ecs-cluster',
    target: 'secrets-manager',
    type: 'smoothstep',
    animated: true,
    style: { stroke: '#4f46e5', strokeWidth: 2 },
    markerEnd: { type: MarkerType.ArrowClosed, color: '#4f46e5' },
  },
  {
    id: 'ecs-ses',
    source: 'ecs-cluster',
    target: 'ses',
    type: 'smoothstep',
    animated: true,
    style: { stroke: '#4f46e5', strokeWidth: 2 },
    markerEnd: { type: MarkerType.ArrowClosed, color: '#4f46e5' },
  },
  {
    id: 'ecs-sns',
    source: 'ecs-cluster',
    target: 'sns',
    type: 'smoothstep',
    animated: true,
    style: { stroke: '#4f46e5', strokeWidth: 2 },
    markerEnd: { type: MarkerType.ArrowClosed, color: '#4f46e5' },
  },
  {
    id: 'ecs-s3',
    source: 'ecs-cluster',
    target: 's3',
    type: 'smoothstep',
    animated: true,
    style: { stroke: '#4f46e5', strokeWidth: 2 },
    markerEnd: { type: MarkerType.ArrowClosed, color: '#4f46e5' },
  },
  
  // Backend to Aurora
  {
    id: 'backend-aurora',
    source: 'backend-service',
    target: 'aurora',
    type: 'smoothstep',
    animated: true,
    style: { stroke: '#10b981', strokeWidth: 2 },
    markerEnd: { type: MarkerType.ArrowClosed, color: '#10b981' },
  },
  
  // Backend to EventBridge
  {
    id: 'backend-eventbridge',
    source: 'backend-service',
    target: 'eventbridge',
    type: 'smoothstep',
    animated: true,
    style: { stroke: '#10b981', strokeWidth: 2 },
    markerEnd: { type: MarkerType.ArrowClosed, color: '#10b981' },
  },
  
  // Services to Observability
  {
    id: 'backend-xray',
    source: 'backend-service',
    target: 'xray',
    type: 'smoothstep',
    animated: true,
    style: { stroke: '#a855f7', strokeWidth: 2, strokeDasharray: '5,5' },
    markerEnd: { type: MarkerType.ArrowClosed, color: '#a855f7' },
  },
  {
    id: 'backend-cloudwatch',
    source: 'backend-service',
    target: 'cloudwatch',
    type: 'smoothstep',
    animated: true,
    style: { stroke: '#a855f7', strokeWidth: 2, strokeDasharray: '5,5' },
    markerEnd: { type: MarkerType.ArrowClosed, color: '#a855f7' },
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
    id: 'auth-ecs',
    source: 'auth-system',
    target: 'ecs-cluster',
    type: 'smoothstep',
    animated: true,
    label: 'JWT/OIDC',
    style: { stroke: '#6b7280', strokeWidth: 2 },
    markerEnd: { type: MarkerType.ArrowClosed, color: '#6b7280' },
  },
  
  // Frontend to Amplify
  {
    id: 'ecs-amplify',
    source: 'ecs-cluster',
    target: 'amplify',
    type: 'smoothstep',
    animated: true,
    style: { stroke: '#4f46e5', strokeWidth: 2 },
    markerEnd: { type: MarkerType.ArrowClosed, color: '#4f46e5' },
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
      // Log node position and details
      console.log('Node clicked:', {
        id: node.id,
        type: node.type,
        position: node.position,
        data: node.data,
      });
      
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
        snapToGrid={true}
        snapGrid={[10, 10]}
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