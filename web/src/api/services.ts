import { ServiceNode, Connection, ServiceType } from '../types';

// Mock data for initial development
const mockNodes: ServiceNode[] = [
  {
    id: "1",
    x: 100,
    y: 200,
    label: "Backend",
    type: "backend",
    status: "running",
    lastDeployed: "just now",
    project: "myapp-backend",
    framework: "Python",
    version: "3.9",
    region: "us-east-1",
    environment: "production",
    scaling: "auto",
    memory: "1GB",
    timeout: "30s",
    monitoring: true,
    replicas: 3,
    minReplicas: 1,
    maxReplicas: 10,
  },
  {
    id: "2",
    x: 450,
    y: 100,
    label: "Frontend",
    type: "frontend",
    status: "running",
    lastDeployed: "2 hours ago",
    project: "myapp-frontend",
    framework: "Next.js",
    version: "14.0.0",
    region: "us-east-1",
    environment: "production",
    features: {
      ssr: true,
      analytics: true,
    },
    performance: 98,
    replicas: 1,
  },
  {
    id: "3",
    x: 450,
    y: 300,
    label: "PostgreSQL",
    type: "database",
    status: "running",
    lastDeployed: "1 week ago",
    project: "pg-data",
    version: "15.3",
    region: "us-east-1",
    environment: "production",
    storage: "100GB",
    connections: 50,
    replicas: 2,
    backup: true,
  },
];

const mockConnections: Connection[] = [
  { from: "1", to: "2" },
  { from: "2", to: "3" },
];

export const servicesApi = {
  // Fetch all nodes
  async getNodes(): Promise<ServiceNode[]> {
    // Simulate API call delay
    await new Promise(resolve => setTimeout(resolve, 100));
    return mockNodes;
  },

  // Fetch all connections
  async getConnections(): Promise<Connection[]> {
    await new Promise(resolve => setTimeout(resolve, 100));
    return mockConnections;
  },

  // Create a new node
  async createNode(type: ServiceType, label: string): Promise<ServiceNode> {
    await new Promise(resolve => setTimeout(resolve, 200));
    
    const newNode: ServiceNode = {
      id: `${Date.now()}`,
      x: 300 + Math.random() * 200,
      y: 200 + Math.random() * 200,
      label,
      type,
      status: "deploying",
      lastDeployed: "Just now",
      project: `${type}-${Date.now()}`,
      region: "us-east-1",
      environment: "production",
      replicas: 1,
    };
    
    return newNode;
  },

  // Update node configuration
  async updateNode(nodeId: string, updates: Partial<ServiceNode>): Promise<ServiceNode> {
    await new Promise(resolve => setTimeout(resolve, 200));
    
    const node = mockNodes.find(n => n.id === nodeId);
    if (!node) {
      throw new Error('Node not found');
    }
    
    return { ...node, ...updates };
  },

  // Delete a node
  async deleteNode(nodeId: string): Promise<void> {
    await new Promise(resolve => setTimeout(resolve, 200));
    // In a real app, this would make an API call
  },

  // Create a connection
  async createConnection(from: string, to: string): Promise<Connection> {
    await new Promise(resolve => setTimeout(resolve, 200));
    return { from, to };
  },

  // Delete a connection
  async deleteConnection(from: string, to: string): Promise<void> {
    await new Promise(resolve => setTimeout(resolve, 200));
    // In a real app, this would make an API call
  },

  // Deploy a service
  async deployService(nodeId: string): Promise<void> {
    await new Promise(resolve => setTimeout(resolve, 1000));
    // In a real app, this would trigger a deployment
  },

  // Scale a service
  async scaleService(nodeId: string, replicas: number): Promise<void> {
    await new Promise(resolve => setTimeout(resolve, 300));
    // In a real app, this would scale the service
  }
};