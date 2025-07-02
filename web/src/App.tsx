import React, { useState, useCallback } from "react";
import { ReactFlowProvider } from "reactflow";
import { CanvasControls } from "./components/CanvasControls";
import { DeploymentCanvas } from "./components/DeploymentCanvas";
import { Sidebar } from "./components/Sidebar";
import { EnvironmentConfig } from "./components/EnvironmentConfig";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "./components/ui/tabs";
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
      <Tabs defaultValue="infrastructure" className="size-full">
        <TabsList className="absolute top-4 left-1/2 -translate-x-1/2 z-10">
          <TabsTrigger value="infrastructure">Infrastructure View</TabsTrigger>
          <TabsTrigger value="configuration">Environment Config</TabsTrigger>
        </TabsList>
        
        <TabsContent value="infrastructure" className="size-full">
          <ReactFlowProvider>
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
        </TabsContent>
        
        <TabsContent value="configuration" className="size-full p-8 overflow-auto">
          <EnvironmentConfig />
        </TabsContent>
      </Tabs>
    </div>
  );
}
