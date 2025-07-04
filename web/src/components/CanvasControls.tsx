import React from 'react';
import { Plus, Minus, Maximize, Grid3x3, Move, MousePointer, Layout, Printer, Eye, EyeOff } from 'lucide-react';
import { useReactFlow, useNodes } from 'reactflow';
import { Button } from './ui/button';

interface CanvasControlsProps {
  onAutoLayout?: () => void;
  showInactive: boolean;
  onToggleInactive: () => void;
}

export function CanvasControls({ onAutoLayout, showInactive, onToggleInactive }: CanvasControlsProps) {
  const { zoomIn, zoomOut, fitView } = useReactFlow();
  const nodes = useNodes();
  
  const printNodePositions = () => {
    const positions = nodes
      .filter(node => node.type === 'service')
      .map(node => ({
        id: node.id,
        position: node.position,
        type: node.data.type,
        name: node.data.name,
      }));
    
    console.log('=== Node Positions ===');
    positions.forEach(({ id, position, type, name }) => {
      console.log(`${id}: { x: ${Math.round(position.x)}, y: ${Math.round(position.y)} } // ${name}`);
    });
    console.log('=== End Positions ===');
    
    // Also log as a copyable object
    const positionsObj = positions.reduce((acc, { id, position }) => {
      acc[id] = { x: Math.round(position.x), y: Math.round(position.y) };
      return acc;
    }, {} as Record<string, { x: number; y: number }>);
    
    console.log('Positions object:', JSON.stringify(positionsObj, null, 2));
  };

  return (
    <div className="absolute left-4 top-4 z-40 flex flex-col gap-2">
      {/* Zoom Controls */}
      <div className="bg-gray-800 border border-gray-700 rounded-lg p-1 flex flex-col gap-1">
        <Button
          size="icon"
          variant="ghost"
          onClick={() => zoomIn()}
          className="w-8 h-8 text-gray-400 hover:text-white hover:bg-gray-700"
        >
          <Plus className="w-4 h-4" />
        </Button>
        <Button
          size="icon"
          variant="ghost"
          onClick={() => zoomOut()}
          className="w-8 h-8 text-gray-400 hover:text-white hover:bg-gray-700"
        >
          <Minus className="w-4 h-4" />
        </Button>
        <Button
          size="icon"
          variant="ghost"
          onClick={() => fitView()}
          className="w-8 h-8 text-gray-400 hover:text-white hover:bg-gray-700"
        >
          <Maximize className="w-4 h-4" />
        </Button>
      </div>

      {/* Tool Controls */}
      <div className="bg-gray-800 border border-gray-700 rounded-lg p-1 flex flex-col gap-1">
        <Button
          size="icon"
          variant="ghost"
          className="w-8 h-8 text-gray-400 hover:text-white hover:bg-gray-700"
        >
          <MousePointer className="w-4 h-4" />
        </Button>
        <Button
          size="icon"
          variant="ghost"
          className="w-8 h-8 text-gray-400 hover:text-white hover:bg-gray-700"
        >
          <Move className="w-4 h-4" />
        </Button>
        <Button
          size="icon"
          variant="ghost"
          className="w-8 h-8 text-gray-400 hover:text-white hover:bg-gray-700"
        >
          <Grid3x3 className="w-4 h-4" />
        </Button>
      </div>

      {/* Layout Control */}
      {onAutoLayout && (
        <div className="bg-gray-800 border border-gray-700 rounded-lg p-1">
          <Button
            size="icon"
            variant="ghost"
            onClick={onAutoLayout}
            className="w-8 h-8 text-gray-400 hover:text-white hover:bg-gray-700"
            title="Auto-layout nodes"
          >
            <Layout className="w-4 h-4" />
          </Button>
        </div>
      )}
      
      {/* Print Positions Control */}
      <div className="bg-gray-800 border border-gray-700 rounded-lg p-1">
        <Button
          size="icon"
          variant="ghost"
          onClick={printNodePositions}
          className="w-8 h-8 text-gray-400 hover:text-white hover:bg-gray-700"
          title="Print node positions to console"
        >
          <Printer className="w-4 h-4" />
        </Button>
      </div>

      {/* Toggle Inactive Items */}
      <div className="bg-gray-800 border border-gray-700 rounded-lg p-1">
        <Button
          size="icon"
          variant="ghost"
          onClick={onToggleInactive}
          className="w-8 h-8 text-gray-400 hover:text-white hover:bg-gray-700"
          title={showInactive ? "Hide inactive items" : "Show inactive items"}
        >
          {showInactive ? <Eye className="w-4 h-4" /> : <EyeOff className="w-4 h-4" />}
        </Button>
      </div>
    </div>
  );
}