import React, { useCallback, useMemo, useEffect, useRef } from "react";
import ReactFlow, {
  Background,
  Controls,
  MiniMap,
  type Node,
  type Edge,
  useNodesState,
  useEdgesState,
  addEdge,
  type Connection,
  MarkerType,
  NodeProps,
  NodeDragHandler,
} from "reactflow";
import "reactflow/dist/style.css";
import {
  type BoardPositions,
  type NodePosition,
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

interface DeploymentCanvasProps {
  onNodeSelect: (node: ComponentNode | null) => void;
  selectedNode: ComponentNode | null;
  config?: YamlInfrastructureConfig | null;
  environmentName?: string;
}

const nodeTypes = {
  service: ServiceNode,
  group: GroupNode,
  dynamicGroup: DynamicGroupNode,
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
      name: "GitHub actions",
      status: "running",
      deletable: false,
    },
    deletable: false,
  },

  // Client Applications (left)
  {
    id: "client-app",
    type: "service",
    position: { x: -639, y: 388 },
    data: {
      id: "client-app",
      type: "client-app",
      name: "Client app",
      status: "running",
    },
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
      status: "running",
      deletable: false,
    },
    deletable: false,
  },
  {
    id: "waf",
    type: "service",
    position: { x: -81, y: 368 },
    data: {
      id: "waf",
      type: "waf",
      name: "AWS WAF",
      status: "running",
      deletable: false,
      disabled: true,
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
      description: "Main backend (required)",
      status: "running",
      group: "ECS Cluster",
      subgroup: "Services",
    },
  },

  // Aurora DB (standalone - not in ECS cluster)
  {
    id: "aurora",
    type: "service",
    position: { x: 600, y: 110 },
    data: {
      id: "aurora",
      type: "aurora",
      name: "Amazon Aurora",
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
      status: "running",
      deletable: false,
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
      status: "running",
      deletable: false,
    },
    deletable: false,
  },

  // Authentication (bottom-left)
  {
    id: "auth-system",
    type: "service",
    position: { x: -71, y: 818 },
    data: {
      id: "auth-system",
      type: "auth",
      name: "Authentication system",
      status: "running",
    },
  },

  // Frontend Distribution (bottom)
  {
    id: "amplify",
    type: "service",
    position: { x: -76, y: 668 },
    data: {
      id: "amplify",
      type: "amplify",
      name: "AWS Amplify",
      description: "Frontend distribution",
      status: "running",
      deletable: false,
    },
    deletable: false,
  },
];

const initialEdges: Edge[] = [
  // GitHub to ECR
  {
    id: "github-ecr",
    source: "github",
    target: "ecr",
    type: "smoothstep",
    animated: true,
    style: { stroke: "#6b7280", strokeWidth: 2 },
    markerEnd: { type: MarkerType.ArrowClosed, color: "#6b7280" },
  },

  // Client Apps to WAF
  {
    id: "client-waf",
    source: "client-app",
    target: "waf",
    type: "smoothstep",
    animated: true,
    style: { stroke: "#4f46e5", strokeWidth: 2 },
    markerEnd: { type: MarkerType.ArrowClosed, color: "#4f46e5" },
  },

  // Entry points to ECS Cluster
  {
    id: "route53-ecs",
    source: "route53",
    target: "ecs-cluster",
    type: "smoothstep",
    animated: true,
    label: "DNS",
    style: { stroke: "#8b5cf6", strokeWidth: 2 },
    markerEnd: { type: MarkerType.ArrowClosed, color: "#8b5cf6" },
  },
  {
    id: "waf-ecs",
    source: "waf",
    target: "ecs-cluster",
    type: "smoothstep",
    animated: true,
    style: { stroke: "#4f46e5", strokeWidth: 2 },
    markerEnd: { type: MarkerType.ArrowClosed, color: "#4f46e5" },
  },
  {
    id: "api-ecs",
    source: "api-gateway",
    target: "ecs-cluster",
    type: "smoothstep",
    animated: true,
    style: { stroke: "#4f46e5", strokeWidth: 2 },
    markerEnd: { type: MarkerType.ArrowClosed, color: "#4f46e5" },
  },

  // ECS to Supporting Services
  {
    id: "ecs-ses",
    source: "ecs-cluster",
    target: "ses",
    type: "smoothstep",
    animated: true,
    style: { stroke: "#4f46e5", strokeWidth: 2 },
    markerEnd: { type: MarkerType.ArrowClosed, color: "#4f46e5" },
  },
  {
    id: "ecs-sns",
    source: "ecs-cluster",
    target: "sns",
    type: "smoothstep",
    animated: true,
    style: { stroke: "#4f46e5", strokeWidth: 2 },
    markerEnd: { type: MarkerType.ArrowClosed, color: "#4f46e5" },
  },
  {
    id: "ecs-s3",
    source: "ecs-cluster",
    target: "s3",
    type: "smoothstep",
    animated: true,
    style: { stroke: "#4f46e5", strokeWidth: 2 },
    markerEnd: { type: MarkerType.ArrowClosed, color: "#4f46e5" },
  },

  // Backend to Aurora
  {
    id: "backend-aurora",
    source: "backend-service",
    target: "aurora",
    type: "smoothstep",
    animated: true,
    style: { stroke: "#10b981", strokeWidth: 2 },
    markerEnd: { type: MarkerType.ArrowClosed, color: "#10b981" },
  },

  // Backend to EventBridge
  {
    id: "backend-eventbridge",
    source: "backend-service",
    target: "eventbridge",
    type: "smoothstep",
    animated: true,
    style: { stroke: "#10b981", strokeWidth: 2 },
    markerEnd: { type: MarkerType.ArrowClosed, color: "#10b981" },
  },

  // Authentication flows
  {
    id: "client-auth",
    source: "client-app",
    target: "auth-system",
    type: "smoothstep",
    animated: true,
    label: "authenticate",
    style: { stroke: "#6b7280", strokeWidth: 2 },
    markerEnd: { type: MarkerType.ArrowClosed, color: "#6b7280" },
  },
  {
    id: "auth-ecs",
    source: "auth-system",
    target: "ecs-cluster",
    type: "smoothstep",
    animated: true,
    label: "JWT/OIDC",
    style: { stroke: "#6b7280", strokeWidth: 2 },
    markerEnd: { type: MarkerType.ArrowClosed, color: "#6b7280" },
  },

  // Frontend to Amplify
  {
    id: "ecs-amplify",
    source: "ecs-cluster",
    target: "amplify",
    type: "smoothstep",
    animated: true,
    style: { stroke: "#4f46e5", strokeWidth: 2 },
    markerEnd: { type: MarkerType.ArrowClosed, color: "#4f46e5" },
  },
];

export function DeploymentCanvas({
  onNodeSelect,
  selectedNode,
  config,
  environmentName,
}: DeploymentCanvasProps) {
  const saveTimeoutRef = useRef<NodeJS.Timeout | null>(null);
  const [savedPositions, setSavedPositions] = React.useState<
    Map<string, { x: number; y: number }>
  >(new Map());

  // Generate all nodes including dynamic ones
  const allNodes = useMemo(() => {
    // Start with initial nodes
    let combinedNodes = [...initialNodes];

    // Add dynamic service nodes
    const additionalServices = generateAdditionalServiceNodes(config, 292, 459);
    const additionalServiceIds = additionalServices.map((n) => n.id);
    combinedNodes = [...combinedNodes, ...additionalServices];

    // Add hidden component nodes
    const hiddenComponents = generateHiddenComponentNodes(config);
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
    }

    return layoutNodesWithGroups(combinedNodes);
  }, [config, savedPositions]);

  const [nodes, setNodes, onNodesChange] = useNodesState(allNodes);
  const [edges, setEdges, onEdgesChange] = useEdgesState(initialEdges);

  // Load saved positions when component mounts or environment changes
  useEffect(() => {
    if (environmentName) {
      infrastructureApi
        .getNodePositions(environmentName)
        .then((data: BoardPositions) => {
          const posMap = new Map<string, { x: number; y: number }>();
          data.positions.forEach((pos) => {
            posMap.set(pos.nodeId, { x: pos.x, y: pos.y });
          });
          setSavedPositions(posMap);
        })
        .catch((error) => {
          console.error("Failed to load node positions:", error);
        });
    }
  }, [environmentName]);

  // Update nodes when config changes
  useEffect(() => {
    setNodes(allNodes);
  }, [allNodes, setNodes]);

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
      console.log("Node clicked:", {
        id: node.id,
        type: node.type,
        position: node.position,
        data: node.data,
      });

      if (node.type === "service") {
        onNodeSelect(node.data as ComponentNode);
      }
    },
    [onNodeSelect]
  );

  const onPaneClick = useCallback(() => {
    onNodeSelect(null);
  }, [onNodeSelect]);

  // Update nodes to show selection state and apply config-based states
  const nodesWithSelection = useMemo(() => {
    return nodes.map((node) => {
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
    });
  }, [nodes, selectedNode, config]);

  // Update edges to show dimmed state when connected to disabled nodes
  const edgesWithState = useMemo(() => {
    const nodeMap = new Map(nodes.map((n) => [n.id, n]));
    return edges.map((edge) => {
      const sourceNode = nodeMap.get(edge.source);
      const targetNode = nodeMap.get(edge.target);
      const isDimmed = sourceNode?.data?.disabled || targetNode?.data?.disabled;

      return {
        ...edge,
        style: {
          ...edge.style,
          opacity: isDimmed ? 0.3 : 1,
        },
        animated: isDimmed ? false : edge.animated,
      };
    });
  }, [edges, nodes]);

  // Save positions when nodes are moved
  const savePositions = useCallback(() => {
    if (!environmentName) return;

    const positions: NodePosition[] = nodes.map((node) => ({
      nodeId: node.id,
      x: node.position.x,
      y: node.position.y,
    }));

    const boardPositions: BoardPositions = {
      environment: environmentName,
      positions,
    };

    infrastructureApi.saveNodePositions(boardPositions).catch((error) => {
      console.error("Failed to save node positions:", error);
    });
  }, [nodes, environmentName]);

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

        // Set new timeout to save positions after 1 second of inactivity
        saveTimeoutRef.current = setTimeout(() => {
          savePositions();
        }, 1000);
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
        <Background color="#374151" gap={20} size={1} variant="dots" />
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
