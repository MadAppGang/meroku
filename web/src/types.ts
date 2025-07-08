export interface ComponentNode {
  id: string;
  type:
    | "frontend"
    | "backend"
    | "database"
    | "cache"
    | "api"
    | "analytics"
    | "infrastructure"
    | "container-registry"
    | "route53"
    | "waf"
    | "api-gateway"
    | "ecs"
    | "ecr"
    | "aurora"
    | "eventbridge"
    | "secrets-manager"
    | "ses"
    | "sns"
    | "s3"
    | "amplify"
    | "xray"
    | "cloudwatch"
    | "telemetry"
    | "alarms"
    | "github"
    | "auth"
    | "client-app"
    | "admin-app"
    | "opa"
    | "service-regular"
    | "service-periodic"
    | "service-event-driven"
    | "service"
    | "scheduled-task"
    | "event-task"
    | "postgres"
    | "sqs"
    | "efs"
    | "alb"
    | "appsync";
  name: string;
  url?: string;
  status: "running" | "deploying" | "stopped" | "error" | "external";
  description?: string;
  isExternal?: boolean;
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
  deletable?: boolean;
  group?: string;
  subgroup?: string;
  hasTelemetry?: boolean;
  disabled?: boolean;
  configProperties?: Record<string, any>;
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