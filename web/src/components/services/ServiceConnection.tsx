import React from 'react';
import { ServiceNode, Connection } from '../../types';

export interface ServiceConnectionProps {
  connection: Connection;
  nodes: ServiceNode[];
}

export const ServiceConnection: React.FC<ServiceConnectionProps> = ({
  connection,
  nodes
}) => {
  const fromNode = nodes.find((n) => n.id === connection.from);
  const toNode = nodes.find((n) => n.id === connection.to);
  
  if (!fromNode || !toNode) return null;

  const fromY = fromNode.y + 80;
  const toY = toNode.y + 80;

  return (
    <line
      x1={fromNode.x + 125}
      y1={fromY}
      x2={toNode.x + 125}
      y2={toY}
      stroke="#4b5563"
      strokeWidth="2"
      strokeDasharray="5,5"
    />
  );
};