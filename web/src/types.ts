export interface ComponentNode {
  id: string;
  type:
    | "frontend"
    | "backend"
    | "database"
    | "cache"
    | "api"
    | "analytics";
  name: string;
  url?: string;
  status: "running" | "deploying" | "stopped" | "error";
  description?: string;
  deploymentType?: string;
  replicas?: number;
  resources?: {
    cpu: string;
    memory: string;
  };
  environment?: Record<string, string>;
  logs?: LogEntry[];
  metrics?: {
    cpu: number;
    memory: number;
    requests: number;
  };
}

export interface LogEntry {
  timestamp: string;
  level: "info" | "warning" | "error";
  message: string;
}

export interface Connection {
  id: string;
  source: string;
  target: string;
  type: string;
}