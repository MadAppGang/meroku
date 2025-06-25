import { ServiceType, NodeStatus } from '../types';

export const getStatusColor = (status: NodeStatus): string => {
  switch (status) {
    case 'running':
      return '#10b981';
    case 'deploying':
      return '#3b82f6';
    case 'stopped':
      return '#6b7280';
    case 'error':
      return '#ef4444';
    default:
      return '#6b7280';
  }
};

export const getServiceColor = (type: ServiceType): string => {
  switch (type) {
    case 'backend':
      return '#3b82f6';
    case 'frontend':
      return '#10b981';
    case 'database':
      return '#8b5cf6';
    case 'redis':
      return '#ef4444';
    case 'cognito':
      return '#f59e0b';
    case 'ses':
      return '#06b6d4';
    case 'sqs':
      return '#ec4899';
    default:
      return '#6b7280';
  }
};