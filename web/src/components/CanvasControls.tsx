import { Plus, Minus, Maximize, Grid3x3, Move, MousePointer, Eye, EyeOff, Server, Clock, Zap, Globe } from 'lucide-react';
import { useReactFlow } from 'reactflow';
import { Button } from './ui/button';

interface CanvasControlsProps {
  showInactive: boolean;
  onToggleInactive: () => void;
  onAddService?: () => void;
  onAddScheduledTask?: () => void;
  onAddEventTask?: () => void;
  onAddAmplify?: () => void;
}

export function CanvasControls({ showInactive, onToggleInactive, onAddService, onAddScheduledTask, onAddEventTask, onAddAmplify }: CanvasControlsProps) {
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

      {/* Add Service Controls */}
      <div className="bg-gray-800 border border-gray-700 rounded-lg p-1 flex flex-col gap-1">
        <Button
          size="icon"
          variant="ghost"
          onClick={onAddService}
          className="w-8 h-8 text-gray-400 hover:text-white hover:bg-blue-700"
          title="Add Service"
        >
          <Server className="w-4 h-4" />
        </Button>
        <Button
          size="icon"
          variant="ghost"
          onClick={onAddScheduledTask}
          className="w-8 h-8 text-gray-400 hover:text-white hover:bg-green-700"
          title="Add Scheduled Task"
        >
          <Clock className="w-4 h-4" />
        </Button>
        <Button
          size="icon"
          variant="ghost"
          onClick={onAddEventTask}
          className="w-8 h-8 text-gray-400 hover:text-white hover:bg-purple-700"
          title="Add Event Task"
        >
          <Zap className="w-4 h-4" />
        </Button>
        <Button
          size="icon"
          variant="ghost"
          onClick={onAddAmplify}
          className="w-8 h-8 text-gray-400 hover:text-white hover:bg-orange-700"
          title="Add Amplify App"
        >
          <Globe className="w-4 h-4" />
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