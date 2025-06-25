import {
  Server,
  Layers,
  Database,
  HardDrive,
  Shield,
  Mail,
  Container,
  Zap,
  LucideIcon
} from 'lucide-react';
import { ServiceType } from '../types';

export const getServiceIcon = (type: ServiceType): LucideIcon => {
  switch (type) {
    case 'backend':
      return Server;
    case 'frontend':
      return Layers;
    case 'database':
      return Database;
    case 'redis':
      return HardDrive;
    case 'cognito':
      return Shield;
    case 'ses':
      return Mail;
    case 'sqs':
      return Container;
    default:
      return Zap;
  }
};