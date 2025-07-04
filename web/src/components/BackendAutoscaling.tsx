import React from 'react';
import { ServiceAutoscaling } from './ServiceAutoscaling';

interface BackendAutoscalingProps {
  environment: string;
}

export function BackendAutoscaling({ environment }: BackendAutoscalingProps) {
  return <ServiceAutoscaling environment={environment} serviceName="backend" />;
}