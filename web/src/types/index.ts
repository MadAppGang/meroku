export type ServiceType = 
  | 'backend'
  | 'frontend'
  | 'database'
  | 'redis'
  | 'cognito'
  | 'ses'
  | 'sqs';

export type NodeStatus = 'running' | 'deploying' | 'stopped' | 'error';

export type Environment = 'production' | 'staging' | 'development';

export interface ServiceNode {
  id: string;
  x: number;
  y: number;
  label: string;
  type: ServiceType;
  status: NodeStatus;
  lastDeployed: string;
  project: string;
  framework?: string;
  version?: string;
  region: string;
  environment: Environment;
  scaling?: 'auto' | 'manual';
  memory?: string;
  timeout?: string;
  monitoring?: boolean;
  replicas: number;
  minReplicas?: number;
  maxReplicas?: number;
  storage?: string;
  connections?: number;
  backup?: boolean;
  features?: {
    ssr?: boolean;
    analytics?: boolean;
  };
  performance?: number;
}

export interface Connection {
  from: string;
  to: string;
}

export interface ComponentType {
  type: ServiceType;
  label: string;
  icon: React.ComponentType<any>;
}

export interface CanvasOffset {
  x: number;
  y: number;
}

export interface CanvasSize {
  width: number;
  height: number;
}