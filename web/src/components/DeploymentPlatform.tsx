import React, { useState } from 'react';
import { Plus, Activity } from 'lucide-react';
import { Environment } from '../types';
import { useCanvas, useNodes } from '../hooks';
import { Header, Sidebar } from './layout';
import { 
  Canvas, 
  AddComponentModal, 
  NodeSettingsPanel, 
  ActivityPanel 
} from './services';
import { Button } from './ui';

export const DeploymentPlatform: React.FC = () => {
  const [environment, setEnvironment] = useState<Environment>('production');
  const [showAddComponentModal, setShowAddComponentModal] = useState(false);
  const [showActivityPanel, setShowActivityPanel] = useState(false);
  const [activeTab, setActiveTab] = useState('settings');

  const {
    nodes,
    connections,
    selectedNode,
    componentTypes,
    addNode,
    updateNodePosition,
    updateNodeReplicas,
    deleteNode,
    selectNode
  } = useNodes();

  const {
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
  } = useCanvas();

  const handleNodeMouseDown = (e: React.MouseEvent, nodeId: string) => {
    if (e.button === 0) {
      startDragging(nodeId);
      const node = nodes.find(n => n.id === nodeId);
      if (node) selectNode(node);
    }
  };

  const handleCanvasMouseDown = (e: React.MouseEvent) => {
    if (e.target === e.currentTarget) {
      selectNode(null);
    }
  };

  const handleCanvasMouseMove = (e: React.MouseEvent) => {
    handleMouseMove(e, updateNodePosition);
  };

  const handleAddComponent = async (type: string) => {
    await addNode(type as any);
    setShowAddComponentModal(false);
  };

  return (
    <div className="h-screen bg-gray-900 flex flex-col">
      <Header
        currentEnvironment={environment}
        onEnvironmentChange={setEnvironment}
        onDeploy={() => console.log('Deploy clicked')}
      />

      <div className="flex-1 flex relative">
        <Sidebar
          onAddComponent={() => setShowAddComponentModal(true)}
          onZoomIn={handleZoomIn}
          onZoomOut={handleZoomOut}
        />

        <div className="flex-1 relative overflow-hidden bg-gray-900">
          <Canvas
            nodes={nodes}
            connections={connections}
            selectedNode={selectedNode}
            draggingNode={draggingNode}
            canvasOffset={canvasOffset}
            zoom={zoom}
            onNodeMouseDown={handleNodeMouseDown}
            onCanvasMouseMove={handleCanvasMouseMove}
            onCanvasMouseUp={handleMouseUp}
            onCanvasMouseDown={handleCanvasMouseDown}
            onCanvasSizeChange={setCanvasSize}
          />

          <Button
            variant="primary"
            icon={<Plus className="w-4 h-4" />}
            onClick={() => setShowAddComponentModal(true)}
            className="absolute top-4 right-4 shadow-lg"
          >
            Create
          </Button>

          <div className="absolute bottom-4 left-4 bg-gray-800 px-3 py-1 rounded-lg text-sm text-gray-400">
            {Math.round(zoom * 100)}%
          </div>
        </div>

        {selectedNode && (
          <NodeSettingsPanel
            node={selectedNode}
            activeTab={activeTab}
            onTabChange={setActiveTab}
            onClose={() => selectNode(null)}
            onUpdateReplicas={updateNodeReplicas}
            onDeleteNode={deleteNode}
          />
        )}

        <ActivityPanel
          isOpen={showActivityPanel}
          onClose={() => setShowActivityPanel(false)}
        />
      </div>

      <AddComponentModal
        isOpen={showAddComponentModal}
        onClose={() => setShowAddComponentModal(false)}
        componentTypes={componentTypes}
        onAddComponent={handleAddComponent}
      />

      <Button
        variant="primary"
        icon={<Activity className="w-5 h-5" />}
        onClick={() => setShowActivityPanel(!showActivityPanel)}
        className="absolute bottom-4 right-4 rounded-full p-3 shadow-lg"
      />
    </div>
  );
};