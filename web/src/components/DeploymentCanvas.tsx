import React, { useCallback, useMemo } from 'react';
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
} from 'reactflow';
import 'reactflow/dist/style.css';
import { ServiceNode } from './ServiceNode';
import { ComponentNode } from '../types';

interface DeploymentCanvasProps {
  onNodeSelect: (node: ComponentNode | null) => void;
  selectedNode: ComponentNode | null;
}

const nodeTypes = {
  service: ServiceNode,
};

const initialNodes: Node[] = [
  {
    id: '1',
    type: 'service',
    position: { x: 100, y: 100 },
    data: {
      id: '1',
      type: 'frontend',
      name: 'frontend',
      url: 'frontend-prod.up.railway.app',
      status: 'running',
      description: 'Deployed just now',
      deploymentType: 'via GitHub',
    },
  },
  {
    id: '2',
    type: 'service',
    position: { x: 400, y: 100 },
    data: {
      id: '2',
      type: 'backend',
      name: 'backend',
      status: 'running',
      description: 'Deployed just now',
      deploymentType: 'via GitHub',
      replicas: 3,
    },
  },
  {
    id: '3',
    type: 'service',
    position: { x: 700, y: 100 },
    data: {
      id: '3',
      type: 'database',
      name: 'redis',
      status: 'running',
      description: 'Just deployed',
      resources: { cpu: '1 vCPU', memory: '1GB RAM' },
    },
  },
  {
    id: '4',
    type: 'service',
    position: { x: 400, y: 300 },
    data: {
      id: '4',
      type: 'database',
      name: 'postgres',
      status: 'running',
      description: 'Deployed via Docker Image',
      resources: { cpu: '2 vCPU', memory: '4GB RAM' },
    },
  },
  {
    id: '5',
    type: 'service',
    position: { x: 100, y: 300 },
    data: {
      id: '5',
      type: 'analytics',
      name: 'ackee analytics',
      url: 'ackee-prod.up.railway.app',
      status: 'running',
      description: 'Deployed via Docker Image',
    },
  },
  {
    id: '6',
    type: 'service',
    position: { x: 400, y: 500 },
    data: {
      id: '6',
      type: 'api',
      name: 'api gateway',
      url: 'api-prod.up.railway.app',
      status: 'running',
      description: 'Deployed just now',
    },
  },
];

const initialEdges: Edge[] = [
  {
    id: 'e1-2',
    source: '1',
    target: '2',
    type: 'smoothstep',
    animated: true,
    style: { stroke: '#4f46e5', strokeWidth: 2 },
    markerEnd: { type: MarkerType.ArrowClosed, color: '#4f46e5' },
  },
  {
    id: 'e2-3',
    source: '2',
    target: '3',
    type: 'smoothstep',
    animated: true,
    style: { stroke: '#4f46e5', strokeWidth: 2 },
    markerEnd: { type: MarkerType.ArrowClosed, color: '#4f46e5' },
  },
  {
    id: 'e2-4',
    source: '2',
    target: '4',
    type: 'smoothstep',
    animated: true,
    style: { stroke: '#4f46e5', strokeWidth: 2 },
    markerEnd: { type: MarkerType.ArrowClosed, color: '#4f46e5' },
  },
  {
    id: 'e5-6',
    source: '5',
    target: '6',
    type: 'smoothstep',
    animated: true,
    style: { stroke: '#4f46e5', strokeWidth: 2 },
    markerEnd: { type: MarkerType.ArrowClosed, color: '#4f46e5' },
  },
];

export function DeploymentCanvas({ onNodeSelect, selectedNode }: DeploymentCanvasProps) {
  const [nodes, setNodes, onNodesChange] = useNodesState(initialNodes);
  const [edges, setEdges, onEdgesChange] = useEdgesState(initialEdges);

  const onConnect = useCallback(
    (params: Connection) => setEdges((eds) => addEdge(params, eds)),
    [setEdges]
  );

  const onNodeClick = useCallback(
    (event: React.MouseEvent, node: Node) => {
      onNodeSelect(node.data as ComponentNode);
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
      </ReactFlow>
    </div>
  );
}