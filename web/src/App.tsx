import React, { useState, useCallback } from "react";
import { ReactFlowProvider } from "reactflow";
import { CanvasControls } from "./components/CanvasControls";
import { DeploymentCanvas } from "./components/DeploymentCanvas";
import { Sidebar } from "./components/Sidebar";
import type { ComponentNode } from "./types";

export default function App() {
  const [selectedNode, setSelectedNode] = useState<ComponentNode | null>(null);
  const [sidebarOpen, setSidebarOpen] = useState(false);

  const handleNodeSelect = useCallback((node: ComponentNode | null) => {
    setSelectedNode(node);
    setSidebarOpen(!!node);
  }, []);

  return (
    <div className="size-full bg-gray-950 text-white relative overflow-hidden">
      <ReactFlowProvider>
        {/* Canvas Controls */}
        <CanvasControls />

        {/* Main Canvas */}
        <DeploymentCanvas
          onNodeSelect={handleNodeSelect}
          selectedNode={selectedNode}
        />

        {/* Right Sidebar */}
        <Sidebar
          selectedNode={selectedNode}
          isOpen={sidebarOpen}
          onClose={() => {
            setSidebarOpen(false);
            setSelectedNode(null);
          }}
        />
      </ReactFlowProvider>
    </div>
  );
}
