import React, { useRef, useEffect } from 'react';
import { ServiceNode, Connection, CanvasOffset, CanvasSize } from '../../types';
import { ServiceNode as ServiceNodeComponent } from './ServiceNode';
import { ServiceConnection } from './ServiceConnection';

export interface CanvasProps {
  nodes: ServiceNode[];
  connections: Connection[];
  selectedNode: ServiceNode | null;
  draggingNode: string | null;
  canvasOffset: CanvasOffset;
  zoom: number;
  onNodeMouseDown: (e: React.MouseEvent, nodeId: string) => void;
  onCanvasMouseMove: (e: React.MouseEvent) => void;
  onCanvasMouseUp: () => void;
  onCanvasMouseDown: (e: React.MouseEvent) => void;
  onCanvasSizeChange: (size: CanvasSize) => void;
}

export const Canvas: React.FC<CanvasProps> = ({
  nodes,
  connections,
  selectedNode,
  draggingNode,
  canvasOffset,
  zoom,
  onNodeMouseDown,
  onCanvasMouseMove,
  onCanvasMouseUp,
  onCanvasMouseDown,
  onCanvasSizeChange
}) => {
  const canvasRef = useRef<SVGSVGElement>(null);

  useEffect(() => {
    const updateCanvasSize = () => {
      if (canvasRef.current) {
        const rect = canvasRef.current.getBoundingClientRect();
        onCanvasSizeChange({ width: rect.width, height: rect.height });
      }
    };
    
    updateCanvasSize();
    window.addEventListener('resize', updateCanvasSize);
    return () => window.removeEventListener('resize', updateCanvasSize);
  }, [onCanvasSizeChange]);

  return (
    <svg
      ref={canvasRef}
      className="w-full h-full"
      onMouseMove={onCanvasMouseMove}
      onMouseUp={onCanvasMouseUp}
      onMouseDown={onCanvasMouseDown}
      style={{ cursor: draggingNode ? 'grabbing' : 'default' }}
    >
      {/* Grid Pattern */}
      <defs>
        <pattern
          id="grid"
          width="20"
          height="20"
          patternUnits="userSpaceOnUse"
        >
          <circle cx="1" cy="1" r="0.5" fill="#374151" />
        </pattern>
      </defs>
      <rect width="100%" height="100%" fill="url(#grid)" />

      <g transform={`translate(${canvasOffset.x}, ${canvasOffset.y}) scale(${zoom})`}>
        {/* Connections */}
        {connections.map((conn, idx) => (
          <ServiceConnection
            key={idx}
            connection={conn}
            nodes={nodes}
          />
        ))}

        {/* Nodes */}
        {nodes.map((node) => (
          <ServiceNodeComponent
            key={node.id}
            node={node}
            isSelected={selectedNode?.id === node.id}
            onMouseDown={onNodeMouseDown}
          />
        ))}
      </g>
    </svg>
  );
};