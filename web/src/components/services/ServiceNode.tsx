import React from 'react';
import { CheckCircle, Copy } from 'lucide-react';
import { ServiceNode as ServiceNodeType } from '../../types';
import { getServiceColor } from '../../utils/colors';
import { getServiceIcon } from '../../utils/icons';

export interface ServiceNodeProps {
  node: ServiceNodeType;
  isSelected: boolean;
  onMouseDown: (e: React.MouseEvent, nodeId: string) => void;
}

export const ServiceNode: React.FC<ServiceNodeProps> = ({
  node,
  isSelected,
  onMouseDown
}) => {
  const Icon = getServiceIcon(node.type);
  const nodeColor = getServiceColor(node.type);

  return (
    <g
      transform={`translate(${node.x}, ${node.y})`}
      onMouseDown={(e) => onMouseDown(e, node.id)}
      style={{ cursor: 'pointer' }}
    >
      {/* Card Shadow */}
      <rect
        width="250"
        height="160"
        rx="12"
        fill="rgba(0,0,0,0.1)"
        x="2"
        y="2"
      />
      
      {/* Card Background */}
      <rect
        width="250"
        height="160"
        rx="12"
        fill="#1e1e2e"
        stroke={isSelected ? nodeColor : "#2a2a3e"}
        strokeWidth={isSelected ? "2" : "1"}
      />

      {/* Header with Icon and Name */}
      <g transform="translate(20, 25)">
        {node.type === 'backend' ? (
          <g>
            <rect x="0" y="0" width="12" height="12" rx="2" fill="#3776ab" />
            <rect x="12" y="0" width="12" height="12" rx="2" fill="#ffd43b" />
            <rect x="0" y="12" width="12" height="12" rx="2" fill="#ffd43b" />
            <rect x="12" y="12" width="12" height="12" rx="2" fill="#3776ab" />
          </g>
        ) : (
          <Icon
            x="0"
            y="0"
            width="24"
            height="24"
            color={nodeColor}
          />
        )}
        <text
          x="35"
          y="17"
          fill="#ffffff"
          fontSize="16"
          fontWeight="500"
        >
          {node.label}
        </text>
      </g>

      {/* Status and Deployment info */}
      <g transform="translate(20, 70)">
        <CheckCircle
          x="0"
          y="0"
          width="20"
          height="20"
          color="#10b981"
        />
        <text x="30" y="15" fill="#9ca3af" fontSize="14">
          Deployed {node.lastDeployed}
        </text>
      </g>

      {/* Replicas info */}
      {node.replicas && (
        <g transform="translate(20, 110)">
          <Copy
            x="0"
            y="0"
            width="20"
            height="20"
            color="#9ca3af"
          />
          <text x="30" y="15" fill="#9ca3af" fontSize="14">
            {node.replicas} Replica{node.replicas > 1 ? "s" : ""}
          </text>
        </g>
      )}
    </g>
  );
};