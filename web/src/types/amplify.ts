export interface AmplifyAppInfo {
  appId: string;
  name: string;
  defaultDomain: string;
  customDomain?: string;
  repository: string;
  createTime: string;
  lastUpdateTime: string;
  branches: AmplifyBranchInfo[];
}

export interface AmplifyBranchInfo {
  branchName: string;
  stage: 'PRODUCTION' | 'BETA' | 'DEVELOPMENT' | 'EXPERIMENTAL' | 'PULL_REQUEST';
  displayName: string;
  enableAutoBuild: boolean;
  enablePullRequestPreview: boolean;
  branchUrl: string;
  lastBuildStatus?: 'PENDING' | 'PROVISIONING' | 'RUNNING' | 'FAILED' | 'SUCCEED' | 'CANCELLING' | 'CANCELLED';
  lastBuildTime?: string;
  lastBuildDuration?: number; // in seconds
  lastCommitId?: string;
  lastCommitMessage?: string;
  lastCommitTime?: string;
  createTime: string;
  updateTime: string;
}

export interface AmplifyAppsResponse {
  apps: AmplifyAppInfo[];
}

export interface AmplifyBuildLogsResponse {
  logUrl: string;
  job: any; // Full job details
}

export interface TriggerBuildRequest {
  appId: string;
  branchName: string;
  profile?: string;
}

export interface TriggerBuildResponse {
  jobId: string;
  status: string;
  message: string;
}