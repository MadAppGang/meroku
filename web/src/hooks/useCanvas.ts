import { useState, useCallback, useRef } from 'react';
import { CanvasOffset, CanvasSize } from '../types';

export const useCanvas = () => {
  const [canvasOffset, setCanvasOffset] = useState<CanvasOffset>({ x: 0, y: 0 });
  const [zoom, setZoom] = useState(1);
  const [canvasSize, setCanvasSize] = useState<CanvasSize>({ width: 800, height: 600 });
  const [draggingNode, setDraggingNode] = useState<string | null>(null);
  const canvasRef = useRef<SVGSVGElement>(null);

  const handleZoomIn = useCallback(() => {
    setZoom((prev) => Math.min(2, prev + 0.1));
  }, []);

  const handleZoomOut = useCallback(() => {
    setZoom((prev) => Math.max(0.5, prev - 0.1));
  }, []);

  const handleMouseMove = useCallback((e: React.MouseEvent, nodePositionSetter: (id: string, x: number, y: number) => void) => {
    if (draggingNode && canvasRef.current) {
      const rect = canvasRef.current.getBoundingClientRect();
      const x = (e.clientX - rect.left - canvasOffset.x) / zoom;
      const y = (e.clientY - rect.top - canvasOffset.y) / zoom;
      nodePositionSetter(draggingNode, x, y);
    }
  }, [draggingNode, canvasOffset, zoom]);

  const handleMouseUp = useCallback(() => {
    setDraggingNode(null);
  }, []);

  const startDragging = useCallback((nodeId: string) => {
    setDraggingNode(nodeId);
  }, []);

  return {
    canvasRef,
    canvasOffset,
    zoom,
    canvasSize,
    draggingNode,
    setCanvasSize,
    handleZoomIn,
    handleZoomOut,
    handleMouseMove,
    handleMouseUp,
    startDragging
  };
};