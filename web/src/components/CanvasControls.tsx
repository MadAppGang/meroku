import React from 'react';
import { Plus, Minus, Maximize, Grid3x3, Move, MousePointer } from 'lucide-react';
import { useReactFlow } from 'reactflow';
import { Button } from './ui/button';

export function CanvasControls() {
  const { zoomIn, zoomOut, fitView } = useReactFlow();

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
    </div>
  );
}