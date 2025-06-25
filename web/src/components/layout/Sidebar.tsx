import React from 'react';
import { Grid3X3, Plus, Minimize2, Maximize2 } from 'lucide-react';
import { IconButton } from '../ui';

export interface SidebarProps {
  onAddComponent: () => void;
  onZoomIn: () => void;
  onZoomOut: () => void;
}

export const Sidebar: React.FC<SidebarProps> = ({
  onAddComponent,
  onZoomIn,
  onZoomOut
}) => {
  return (
    <div className="w-16 bg-gray-800 border-r border-gray-700 flex flex-col items-center py-4 gap-4">
      <IconButton icon={<Grid3X3 className="w-5 h-5" />} />
      <IconButton 
        icon={<Plus className="w-5 h-5" />} 
        onClick={onAddComponent}
      />
      <IconButton 
        icon={<Minimize2 className="w-5 h-5" />} 
        onClick={onZoomOut}
      />
      <IconButton 
        icon={<Maximize2 className="w-5 h-5" />} 
        onClick={onZoomIn}
      />
    </div>
  );
};