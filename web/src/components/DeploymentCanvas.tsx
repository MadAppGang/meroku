import React, { useCallback, useMemo, useEffect, useRef, useState } from "react";
import ReactFlow, {
  Background,
  MiniMap,
  type Node,
  type Edge,
  useNodesState,
  useEdgesState,
  addEdge,
  type Connection,
  MarkerType,
} from "reactflow";
import "reactflow/dist/style.css";
import {
  type BoardPositions,
  type NodePosition,
  type EdgeHandlePosition,
  infrastructureApi,
} from "../api/infrastructure";
import type { ComponentNode } from "../types";
import type { YamlInfrastructureConfig } from "../types/yamlConfig";
import {
  generateAdditionalServiceNodes,
  updateEcsClusterGroup,
} from "../utils/additionalServicesNodes";
import { generateHiddenComponentNodes } from "../utils/hiddenComponentsNodes";
import { layoutNodesWithGroups } from "../utils/layoutUtils";
import { getNodeProperties, getNodeState } from "../utils/nodeStateMapping";
import { CanvasControls } from "./CanvasControls";
import { DynamicGroupNode } from "./DynamicGroupNode";
import { GroupNode } from "./GroupNode";
import { ServiceNode } from "./ServiceNode";
import { EdgeHandleSelector } from "./EdgeHandleSelector";
import { CustomEdge } from "./CustomEdge";

interface DeploymentCanvasProps {
  onNodeSelect: (node: ComponentNode | null) => void;
  selectedNode: ComponentNode | null;
  config?: YamlInfrastructureConfig | null;
  environmentName?: string;
  onAddService?: () => void;
  onAddScheduledTask?: () => void;
  onAddEventTask?: () => void;
}

const nodeTypes = {
  service: ServiceNode,
  group: GroupNode,
  dynamicGroup: DynamicGroupNode,
};

const edgeTypes = {
  smoothstep: CustomEdge,
};

const initialNodes: Node[] = [
  // GitHub Actions (top)
  {
    id: "github",
    type: "service",
    position: { x: 230, y: -80 },
    data: {
      id: "github",
      type: "github",
      name: "GitHub Actions",
      description: "Source code & CI/CD",
      status: "running",
      deletable: false,
    },
    deletable: false,
  },

  // Client Applications (left) - External to infrastructure
  {
    id: "client-app",
    type: "service",
    position: { x: -639, y: 388 },
    data: {
      id: "client-app",
      type: "client-app",
      name: "Client Applications",
      description: "Web, Mobile, Desktop apps",
      status: "external",
      isExternal: true,
      deletable: false,
    },
    deletable: false,
  },

  // Entry Points (left-middle)
  {
    id: "route53",
    type: "service",
    position: { x: -131, y: 158 },
    data: {
      id: "route53",
      type: "route53",
      name: "Amazon Route 53",
      description: "DNS management",
      status: "running",
      deletable: false,
    },
    deletable: false,
  },
  {
    id: "api-gateway",
    type: "service",
    position: { x: -83, y: 518 },
    data: {
      id: "api-gateway",
      type: "api-gateway",
      name: "Amazon API Gateway",
      description: "API routing & throttling",
      status: "running",
      deletable: false,
    },
    deletable: false,
  },

  // Main ECS Cluster Group (center) - contains all ECS-related elements
  {
    id: "ecs-cluster-group",
    type: "dynamicGroup",
    position: { x: 0, y: 0 }, // Position will be calculated
    data: {
      label: "ECS Cluster",
      nodeIds: ["ecs-cluster", "backend-service"],
    },
    style: {
      zIndex: -3,
      backgroundColor: "rgba(59, 130, 246, 0.05)",
      border: "2px solid #3b82f6",
    },
    draggable: false,
    selectable: false,
  },

  // ECS Cluster node (center-top)
  {
    id: "ecs-cluster",
    type: "service",
    position: { x: 284, y: 283 },
    data: {
      id: "ecs-cluster",
      type: "ecs",
      name: "Amazon ECS Cluster",
      description: "Container orchestration",
      status: "running",
      deletable: false,
      group: "ECS Cluster",
    },
    deletable: false,
  },

  // ECR (standalone - not in ECS cluster)
  {
    id: "ecr",
    type: "service",
    position: { x: 280, y: 110 },
    data: {
      id: "ecr",
      type: "ecr",
      name: "Amazon ECR",
      description: "Docker image registry",
      status: "running",
      deletable: false,
    },
    deletable: false,
  },

  // Services in the services subgroup
  {
    id: "backend-service",
    type: "service",
    position: { x: 292, y: 459 },
    data: {
      id: "backend-service",
      type: "backend",
      name: "Backend service",
      description: "Main API service",
      status: "running",
      group: "ECS Cluster",
      subgroup: "Services",
    },
  },

  // PostgreSQL Database (standalone - not in ECS cluster)
  {
    id: "aurora",
    type: "service",
    position: { x: 600, y: 110 },
    data: {
      id: "aurora",
      type: "postgres",
      name: "PostgreSQL Database",
      description: "Relational data storage",
      status: "running",
      deletable: false,
    },
    deletable: false,
  },

  // EventBridge (standalone - not in ECS cluster)
  {
    id: "eventbridge",
    type: "service",
    position: { x: 290, y: 620 },
    data: {
      id: "eventbridge",
      type: "eventbridge",
      name: "Amazon EventBridge",
      description: "Event routing",
      status: "running",
      deletable: false,
    },
    deletable: false,
  },

  // Supporting Services (right side)
  {
    id: "ses",
    type: "service",
    position: { x: 1170, y: 620 },
    data: {
      id: "ses",
      type: "ses",
      name: "Amazon SES",
      description: "Email delivery",
      status: "running",
      deletable: false,
      disabled: true,
    },
    deletable: false,
  },
  {
    id: "sns",
    type: "service",
    position: { x: 580, y: 620 },
    data: {
      id: "sns",
      type: "sns",
      name: "Amazon SNS",
      description: "Push notifications",
      status: "running",
      deletable: false,
    },
    deletable: false,
  },
  {
    id: "sqs",
    type: "service",
    position: { x: 730, y: 620 },
    data: {
      id: "sqs",
      type: "sqs",
      name: "Amazon SQS",
      description: "Message queue",
      status: "running",
      deletable: false,
      disabled: true,
    },
    deletable: false,
  },
  {
    id: "s3",
    type: "service",
    position: { x: 880, y: 620 },
    data: {
      id: "s3",
      type: "s3",
      name: "Amazon S3",
      description: "Object storage",
      status: "running",
      deletable: false,
    },
    deletable: false,
  },
];

const initialEdges: Edge[] = [
  // CI/CD Flow: GitHub Actions to ECR
  {
    id: "github-ecr",
    source: "github",
    target: "ecr",
    sourceHandle: "source-bottom",
    targetHandle: "target-top",
    type: "smoothstep",
    animated: true,
    label: "push",
    style: { stroke: "#60a5fa", strokeWidth: 2 },
    markerEnd: { type: MarkerType.ArrowClosed, color: "#6b7280" },
  },

  // ECR to ECS deployment flow
  {
    id: "ecr-ecs",
    source: "ecr",
    target: "ecs-cluster",
    sourceHandle: "source-bottom",
    targetHandle: "target-top",
    type: "smoothstep",
    animated: true,
    label: "deploy",
    style: { stroke: "#60a5fa", strokeWidth: 2 },
    markerEnd: { type: MarkerType.ArrowClosed, color: "#6b7280" },
  },

  // Client entry points
  {
    id: "client-route53",
    source: "client-app",
    target: "route53",
    type: "smoothstep",
    animated: true,
    label: "DNS",
    style: { stroke: "#8b5cf6", strokeWidth: 2 },
    markerEnd: { type: MarkerType.ArrowClosed, color: "#4f46e5" },
  },
  {
    id: "client-api",
    source: "client-app",
    target: "api-gateway",
    type: "smoothstep",
    animated: true,
    label: "HTTPS",
    style: { stroke: "#8b5cf6", strokeWidth: 2 },
    markerEnd: { type: MarkerType.ArrowClosed, color: "#4f46e5" },
  },

  // Route53 to API Gateway
  {
    id: "route53-api",
    source: "route53",
    target: "api-gateway",
    type: "smoothstep",
    animated: true,
    label: "resolve",
    style: { stroke: "#a78bfa", strokeWidth: 2 },
    markerEnd: { type: MarkerType.ArrowClosed, color: "#8b5cf6" },
  },

  // API Gateway to Backend Service
  {
    id: "api-backend",
    source: "api-gateway",
    target: "backend-service",
    type: "smoothstep",
    animated: true,
    label: "route",
    style: { stroke: "#6366f1", strokeWidth: 2 },
    markerEnd: { type: MarkerType.ArrowClosed, color: "#4f46e5" },
  },

  // Backend Service connections
  {
    id: "backend-aurora",
    source: "backend-service",
    target: "aurora",
    type: "smoothstep",
    animated: true,
    label: "SQL",
    style: { stroke: "#10b981", strokeWidth: 2 },
    markerEnd: { type: MarkerType.ArrowClosed, color: "#10b981" },
  },
  {
    id: "backend-s3",
    source: "backend-service",
    target: "s3",
    type: "smoothstep",
    animated: true,
    label: "S3",
    style: { stroke: "#10b981", strokeWidth: 2 },
    markerEnd: { type: MarkerType.ArrowClosed, color: "#10b981" },
  },
  {
    id: "backend-ses",
    source: "backend-service",
    target: "ses",
    type: "smoothstep",
    animated: true,
    label: "SMTP",
    style: { stroke: "#10b981", strokeWidth: 2 },
    markerEnd: { type: MarkerType.ArrowClosed, color: "#10b981" },
  },
  {
    id: "backend-sns",
    source: "backend-service",
    target: "sns",
    type: "smoothstep",
    animated: true,
    label: "publish",
    style: { stroke: "#10b981", strokeWidth: 2 },
    markerEnd: { type: MarkerType.ArrowClosed, color: "#10b981" },
  },
  {
    id: "backend-sqs",
    source: "backend-service",
    target: "sqs",
    type: "smoothstep",
    animated: true,
    label: "queue",
    style: { stroke: "#10b981", strokeWidth: 2 },
    markerEnd: { type: MarkerType.ArrowClosed, color: "#10b981" },
  },
  {
    id: "backend-eventbridge",
    source: "backend-service",
    target: "eventbridge",
    type: "smoothstep",
    animated: true,
    label: "events",
    style: { stroke: "#10b981", strokeWidth: 2 },
    markerEnd: { type: MarkerType.ArrowClosed, color: "#10b981" },
  },
  {
    id: "backend-secrets",
    source: "backend-service",
    target: "secrets-manager",
    type: "smoothstep",
    animated: true,
    label: "secrets",
    style: { stroke: "#10b981", strokeWidth: 2 },
    markerEnd: { type: MarkerType.ArrowClosed, color: "#10b981" },
  },

  // Monitoring connections
  {
    id: "backend-xray",
    source: "backend-service",
    target: "xray",
    type: "smoothstep",
    animated: false,
    label: "trace",
    style: { stroke: "#94a3b8", strokeWidth: 1.5, strokeDasharray: "4,4", opacity: 0.6 },
    markerEnd: { type: MarkerType.ArrowClosed, color: "#94a3b8" },
  },
  {
    id: "backend-cloudwatch",
    source: "backend-service",
    target: "cloudwatch",
    type: "smoothstep",
    animated: false,
    label: "logs",
    style: { stroke: "#94a3b8", strokeWidth: 1.5, strokeDasharray: "4,4", opacity: 0.6 },
    markerEnd: { type: MarkerType.ArrowClosed, color: "#94a3b8" },
  },
];

export function DeploymentCanvas({
  onNodeSelect,
  selectedNode,
  config,
  environmentName,
  onAddService,
  onAddScheduledTask,
  onAddEventTask,
}: DeploymentCanvasProps) {
  const saveTimeoutRef = useRef<NodeJS.Timeout | null>(null);
  const [savedPositions, setSavedPositions] = React.useState<
    Map<string, { x: number; y: number }>
  >(new Map());
  const [savedEdgeHandles, setSavedEdgeHandles] = useState<Map<string, EdgeHandlePosition>>(new Map());
  const [isLoadingPositions, setIsLoadingPositions] = useState(true);
  const [showInactive, setShowInactive] = React.useState(true);
  const [selectedEdge, setSelectedEdge] = useState<Edge | null>(null);
  const [edgeSelectorPosition, setEdgeSelectorPosition] = useState<{ x: number; y: number } | null>(null);
  const nodesRef = useRef<Node[]>([]);

  // Generate all nodes including dynamic ones
  const allNodes = useMemo(() => {
    // Start with initial nodes
    let combinedNodes = [...initialNodes];

    // Add dynamic service nodes
    const additionalServices = generateAdditionalServiceNodes(config || null, 292, 459);
    const additionalServiceIds = additionalServices.map((n) => n.id);
    combinedNodes = [...combinedNodes, ...additionalServices];

    // Add hidden component nodes
    const hiddenComponents = generateHiddenComponentNodes(config || null);
    combinedNodes = [...combinedNodes, ...hiddenComponents];

    // Update ECS cluster group to include dynamic services
    combinedNodes = combinedNodes.map((node) =>
      node.id === "ecs-cluster-group"
        ? updateEcsClusterGroup(node, additionalServiceIds)
        : node
    );

    // Apply saved positions if available
    if (savedPositions.size > 0) {
      combinedNodes = combinedNodes.map((node) => {
        const savedPos = savedPositions.get(node.id);
        if (savedPos) {
          return { ...node, position: savedPos };
        }
        return node;
      });
      // Don't re-layout if we have saved positions
      return combinedNodes;
    }

    // Only apply layout if no saved positions
    return layoutNodesWithGroups(combinedNodes);
  }, [config, savedPositions]);

  const [nodes, setNodes, onNodesChange] = useNodesState(allNodes);
  
  // Update nodes ref whenever nodes change
  useEffect(() => {
    nodesRef.current = nodes;
  }, [nodes]);
  
  // Create dynamic edges based on enabled nodes
  const dynamicEdges = useMemo(() => {
    // Helper function to check if a node is enabled
    const isNodeEnabled = (nodeId: string) => {
      return getNodeState(nodeId, config || null);
    };

    // Filter initial edges to only include those where both source and target are enabled
    return initialEdges
      .filter(edge => {
        // Always show edges from external nodes (like client-app) or CI/CD nodes
        const sourceNode = nodes.find(n => n.id === edge.source);
        if (sourceNode?.data?.isExternal || edge.source === 'github' || edge.source === 'ecr') {
          return isNodeEnabled(edge.target);
        }
        
        // For other edges, both source and target must be enabled
        return isNodeEnabled(edge.source) && isNodeEnabled(edge.target);
      })
      .map(edge => {
        // Apply saved handle positions if available
        const savedHandle = savedEdgeHandles.get(edge.id);
        if (savedHandle) {
          return {
            ...edge,
            sourceHandle: savedHandle.sourceHandle || edge.sourceHandle,
            targetHandle: savedHandle.targetHandle || edge.targetHandle,
          };
        }
        return edge;
      });
  }, [config, nodes, savedEdgeHandles]);
  
  const [edges, setEdges, onEdgesChange] = useEdgesState([]);

  // Load saved positions when component mounts or environment changes
  useEffect(() => {
    if (environmentName) {
      setIsLoadingPositions(true);
      infrastructureApi
        .getNodePositions(environmentName)
        .then((data: BoardPositions) => {

          const posMap = new Map<string, { x: number; y: number }>();
          data.positions.forEach((pos) => {
            posMap.set(pos.nodeId, { x: pos.x, y: pos.y });
          });
          console.log("Loading positions for nodes:", data.positions.filter(p => ['s3', 'ses', 'sqs', 'eventbridge'].includes(p.nodeId)));
          setSavedPositions(posMap);
          
          // Load edge handle positions
          if (data.edgeHandles) {
            const handleMap = new Map<string, EdgeHandlePosition>();
            data.edgeHandles.forEach((handle) => {
              handleMap.set(handle.edgeId, handle);
            });
            setSavedEdgeHandles(handleMap);
          }
          setIsLoadingPositions(false);
        })
        .catch((error) => {
          console.error("Failed to load node positions:", error);
          setIsLoadingPositions(false);
        });
    } else {
      setIsLoadingPositions(false);
    }
  }, [environmentName]);

  // Update nodes when config changes
  useEffect(() => {
    setNodes(allNodes);
  }, [allNodes, setNodes]);

  // Update edges when saved handles change or when initial load completes
  useEffect(() => {
    if (!isLoadingPositions) {
      setEdges(dynamicEdges);
    }
  }, [isLoadingPositions, dynamicEdges, setEdges]);


  // Toggle inactive nodes visibility
  const handleToggleInactive = useCallback(() => {
    setShowInactive((prev) => !prev);
  }, []);

  const onConnect = useCallback(
    (params: Connection) => setEdges((eds) => addEdge(params, eds)),
    [setEdges]
  );

  const onNodeClick = useCallback(
    (_event: React.MouseEvent, node: Node) => {
      if (node.type === "service") {
        const nodeData = node.data as ComponentNode;
        // Don't open sidebar for external nodes like client applications
        if (!nodeData.isExternal) {
          onNodeSelect(nodeData);
        }
      }
    },
    [onNodeSelect]
  );

  const onPaneClick = useCallback(() => {
    onNodeSelect(null);
    setSelectedEdge(null);
    setEdgeSelectorPosition(null);
  }, [onNodeSelect]);

  // Handle edge click
  const onEdgeClick = useCallback((event: React.MouseEvent, edge: Edge) => {
    event.stopPropagation();
    setSelectedEdge(edge);
    setEdgeSelectorPosition({ x: event.clientX, y: event.clientY });
  }, []);

  // Update nodes to show selection state and apply config-based states
  const nodesWithSelection = useMemo(() => {
    return nodes
      .map((node) => {
        // Apply configuration-based state
        const isEnabled = getNodeState(node.id, config || null);
        const properties = getNodeProperties(node.id, config || null);

        return {
          ...node,
          selected: selectedNode?.id === node.id,
          data: {
            ...node.data,
            disabled: !isEnabled,
            configProperties: properties,
          },
        };
      })
      .filter((node) => {
        // Always show group nodes
        if (node.type === 'group' || node.type === 'dynamicGroup') {
          return true;
        }
        // Filter out disabled nodes if showInactive is false
        if (!showInactive && node.data.disabled) {
          return false;
        }
        return true;
      });
  }, [nodes, selectedNode, config, showInactive]);

  // Update edges to show dimmed state when connected to disabled nodes
  const edgesWithState = useMemo(() => {
    const nodeMap = new Map(nodesWithSelection.map((n) => [n.id, n]));
    const visibleNodeIds = new Set(nodesWithSelection.map((n) => n.id));
    
    return edges
      .filter((edge) => {
        // Only show edges where both source and target nodes are visible
        return visibleNodeIds.has(edge.source) && visibleNodeIds.has(edge.target);
      })
      .map((edge) => {
        const sourceNode = nodeMap.get(edge.source);
        const targetNode = nodeMap.get(edge.target);
        const isDimmed = sourceNode?.data?.disabled || targetNode?.data?.disabled;

        return {
          ...edge,
          style: {
            ...edge.style,
            opacity: isDimmed ? 0.3 : 1,
            animated: isDimmed ? false : edge.animated,
          },
          animated: isDimmed ? false : edge.animated,
        };
      });
  }, [edges, nodesWithSelection]);

  // Save positions when nodes are moved
  const savePositions = useCallback(() => {
    if (!environmentName || isLoadingPositions) return;

    // Use the ref to get the latest node positions
    const currentNodes = nodesRef.current;
    const positions: NodePosition[] = currentNodes.map((node) => ({
      nodeId: node.id,
      x: node.position.x,
      y: node.position.y,
    }));

    // Convert saved edge handles to array
    const edgeHandles: EdgeHandlePosition[] = Array.from(savedEdgeHandles.values());

    const boardPositions: BoardPositions = {
      environment: environmentName,
      positions,
      edgeHandles: edgeHandles.length > 0 ? edgeHandles : undefined,
    };

    console.log("Saving positions - total nodes:", positions.length);
    console.log("Saving positions for problematic nodes:", positions.filter(p => ['s3', 'ses', 'sqs', 'eventbridge', 'sns'].includes(p.nodeId)));

    infrastructureApi.saveNodePositions(boardPositions).catch((error) => {
      console.error("Failed to save node positions:", error);
    });
  }, [environmentName, savedEdgeHandles, isLoadingPositions]);

  // Handle edge handle change
  const handleEdgeHandleChange = useCallback((edgeId: string, sourceHandle?: string, targetHandle?: string) => {
    const newHandle: EdgeHandlePosition = { edgeId, sourceHandle, targetHandle };
    setSavedEdgeHandles(prev => new Map(prev).set(edgeId, newHandle));

    // Save after a delay
    if (saveTimeoutRef.current) {
      clearTimeout(saveTimeoutRef.current);
    }
    saveTimeoutRef.current = setTimeout(() => {
      savePositions();
    }, 1000);
  }, [savePositions]);

  // Handle node position changes with debouncing
  const handleNodesChange = useCallback(
    (changes: any) => {
      onNodesChange(changes);

      // Check if any changes are position changes
      const hasPositionChange = changes.some(
        (change: any) => change.type === "position" && change.dragging === false
      );

      if (hasPositionChange && environmentName) {
        // Clear existing timeout
        if (saveTimeoutRef.current) {
          clearTimeout(saveTimeoutRef.current);
        }

        // Set new timeout to save positions after a shorter delay
        saveTimeoutRef.current = setTimeout(() => {
          savePositions();
        }, 500);
      }
    },
    [onNodesChange, savePositions, environmentName]
  );

  return (
    <div className="size-full">
      <ReactFlow
        nodes={nodesWithSelection}
        edges={edgesWithState}
        onNodesChange={handleNodesChange}
        onEdgesChange={onEdgesChange}
        onConnect={onConnect}
        onNodeClick={onNodeClick}
        onEdgeClick={onEdgeClick}
        onPaneClick={onPaneClick}
        nodeTypes={nodeTypes}
        edgeTypes={edgeTypes}
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
        <Background color="#374151" gap={20} size={1} />
        <MiniMap
          nodeColor="#4f46e5"
          nodeStrokeWidth={3}
          className="bg-gray-800 border border-gray-700 rounded-lg"
        />
        <CanvasControls 
          showInactive={showInactive}
          onToggleInactive={handleToggleInactive}
          onAddService={onAddService}
          onAddScheduledTask={onAddScheduledTask}
          onAddEventTask={onAddEventTask}
        />
      </ReactFlow>
      
      {/* Edge Handle Selector */}
      {selectedEdge && edgeSelectorPosition && (
        <EdgeHandleSelector
          edge={edges.find(e => e.id === selectedEdge.id) || selectedEdge}
          position={edgeSelectorPosition}
          onClose={() => {
            setSelectedEdge(null);
            setEdgeSelectorPosition(null);
          }}
          onHandleChange={handleEdgeHandleChange}
        />
      )}
    </div>
  );
}
