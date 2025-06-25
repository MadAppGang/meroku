import { useState, useCallback, useEffect } from 'react';
import { ServiceNode, Connection, ServiceType } from '../types';
import { servicesApi } from '../api';
import { ComponentType } from '../types';
import { 
  Server, 
  Layers, 
  Database, 
  HardDrive, 
  Shield, 
  Mail, 
  Container 
} from 'lucide-react';

export const useNodes = () => {
  const [nodes, setNodes] = useState<ServiceNode[]>([]);
  const [connections, setConnections] = useState<Connection[]>([]);
  const [selectedNode, setSelectedNode] = useState<ServiceNode | null>(null);
  const [loading, setLoading] = useState(true);

  const componentTypes: ComponentType[] = [
    { type: "backend", label: "Backend Service", icon: Server },
    { type: "frontend", label: "Frontend Service", icon: Layers },
    { type: "database", label: "PostgreSQL", icon: Database },
    { type: "redis", label: "Redis Cache", icon: HardDrive },
    { type: "cognito", label: "Cognito Auth", icon: Shield },
    { type: "ses", label: "SES Email", icon: Mail },
    { type: "sqs", label: "SQS Queue", icon: Container },
  ];

  // Load initial data
  useEffect(() => {
    const loadData = async () => {
      try {
        const [nodesData, connectionsData] = await Promise.all([
          servicesApi.getNodes(),
          servicesApi.getConnections()
        ]);
        setNodes(nodesData);
        setConnections(connectionsData);
      } catch (error) {
        console.error('Failed to load data:', error);
      } finally {
        setLoading(false);
      }
    };
    
    loadData();
  }, []);

  const addNode = useCallback(async (type: ServiceType) => {
    const componentType = componentTypes.find(c => c.type === type);
    const newNode = await servicesApi.createNode(
      type, 
      componentType?.label || "New Service"
    );
    setNodes(prev => [...prev, newNode]);
    return newNode;
  }, []);

  const updateNodePosition = useCallback((nodeId: string, x: number, y: number) => {
    setNodes(prev => prev.map(node => 
      node.id === nodeId ? { ...node, x, y } : node
    ));
  }, []);

  const updateNodeReplicas = useCallback(async (nodeId: string, change: number) => {
    const node = nodes.find(n => n.id === nodeId);
    if (!node) return;

    const newReplicas = Math.max(
      1,
      Math.min(node.maxReplicas || 10, (node.replicas || 1) + change)
    );

    await servicesApi.scaleService(nodeId, newReplicas);
    
    setNodes(prev => prev.map(n => 
      n.id === nodeId ? { ...n, replicas: newReplicas } : n
    ));
  }, [nodes]);

  const deleteNode = useCallback(async (nodeId: string) => {
    await servicesApi.deleteNode(nodeId);
    
    setNodes(prev => prev.filter(n => n.id !== nodeId));
    setConnections(prev => prev.filter(
      c => c.from !== nodeId && c.to !== nodeId
    ));
    
    if (selectedNode?.id === nodeId) {
      setSelectedNode(null);
    }
  }, [selectedNode]);

  const selectNode = useCallback((node: ServiceNode | null) => {
    setSelectedNode(node);
  }, []);

  return {
    nodes,
    connections,
    selectedNode,
    loading,
    componentTypes,
    addNode,
    updateNodePosition,
    updateNodeReplicas,
    deleteNode,
    selectNode
  };
};