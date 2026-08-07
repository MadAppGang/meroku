export interface IAMRole {
  id: string;
  name: string;
  description: string;
  arn: string;
  policies: IAMPolicy[];
}

export interface IAMPolicy {
  id: string;
  name: string;
  type: 'Managed' | 'Inline';
  accessLevel: 'Read' | 'Write' | 'Full';
}

export interface EnvVar {
  id: string;
  key: string;
  value: string;
  isSecret: boolean;
  isPredefined?: boolean;
}

export interface SSMParameter {
  id: string;
  name: string;
  value: string | null; // null indicates not fetched
  isLoading?: boolean;
}

export interface ResourceConfig {
  instanceType: string;
  memory: number;
  cpu: number;
  autoScale: boolean;
  minInstances: number;
  maxInstances: number;
}

export enum TabOption {
  Settings = 'Settings',
  Autoscaling = 'Autoscaling',
  SSH = 'SSH',
  CICD = 'CI/CD',
  EnvVars = 'Env Vars',
  IAM = 'IAM',
  XRay = 'X-Ray',
  Logs = 'Logs'
}
