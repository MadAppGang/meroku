import React, { useState, useCallback, useEffect } from "react";
import { ReactFlowProvider } from "reactflow";
import * as yaml from "js-yaml";
import { CanvasControls } from "./components/CanvasControls";
import { DeploymentCanvas } from "./components/DeploymentCanvas";
import { Sidebar } from "./components/Sidebar";
import { EnvironmentConfig } from "./components/EnvironmentConfig";
import { EnvironmentSelector } from "./components/EnvironmentSelector";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "./components/ui/tabs";
import type { ComponentNode } from "./types";
import { InfrastructureConfig } from "./types/config";
import { YamlInfrastructureConfig } from "./types/yamlConfig";
import { infrastructureApi } from "./api/infrastructure";

export default function App() {
  const [selectedNode, setSelectedNode] = useState<ComponentNode | null>(null);
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const [selectedEnvironment, setSelectedEnvironment] = useState<string | null>(null);
  const [showEnvSelector, setShowEnvSelector] = useState(true);
  const [config, setConfig] = useState<YamlInfrastructureConfig | null>(null);

  const handleNodeSelect = useCallback((node: ComponentNode | null) => {
    setSelectedNode(node);
    setSidebarOpen(!!node);
  }, []);

  const handleEnvironmentSelect = useCallback((environment: string) => {
    setSelectedEnvironment(environment);
    setShowEnvSelector(false);
  }, []);

  // Load configuration when environment is selected
  useEffect(() => {
    if (selectedEnvironment) {
      loadConfiguration(selectedEnvironment);
    }
  }, [selectedEnvironment]);

  const loadConfiguration = async (envName: string) => {
    try {
      const content = await infrastructureApi.getEnvironmentConfig(envName);
      const parsed = yaml.load(content) as YamlInfrastructureConfig;
      setConfig(parsed);
    } catch (error) {
      console.error("Failed to load configuration:", error);
    }
  };

  const handleConfigChange = (updates: Partial<YamlInfrastructureConfig>) => {
    if (config) {
      const updatedConfig = { ...config, ...updates };
      setConfig(updatedConfig);
      // TODO: Save to backend
    }
  };

  return (
    <div className="size-full bg-gray-950 text-white relative overflow-hidden">
      <EnvironmentSelector 
        open={showEnvSelector} 
        onSelect={handleEnvironmentSelect} 
      />
      <Tabs defaultValue="infrastructure" className="size-full">
        <div className="absolute top-4 left-1/2 -translate-x-1/2 z-10 flex items-center gap-4">
          {selectedEnvironment && (
            <div className="flex items-center gap-2 px-3 py-1 bg-gray-800 rounded-md">
              <span className="text-sm text-gray-400">Environment:</span>
              <span className="text-sm font-medium">{selectedEnvironment}</span>
              <button
                onClick={() => setShowEnvSelector(true)}
                className="ml-2 text-xs text-blue-400 hover:text-blue-300"
              >
                Change
              </button>
            </div>
          )}
          <TabsList>
            <TabsTrigger value="infrastructure">Infrastructure View</TabsTrigger>
            <TabsTrigger value="configuration">Environment Config</TabsTrigger>
          </TabsList>
        </div>
        
        <TabsContent value="infrastructure" className="size-full">
          <ReactFlowProvider>
            {/* Main Canvas */}
            <DeploymentCanvas
              onNodeSelect={handleNodeSelect}
              selectedNode={selectedNode}
              config={config}
            />

            {/* Right Sidebar */}
            <Sidebar
              selectedNode={selectedNode}
              isOpen={sidebarOpen}
              onClose={() => {
                setSidebarOpen(false);
                setSelectedNode(null);
              }}
              config={config || undefined}
              onConfigChange={handleConfigChange}
            />
          </ReactFlowProvider>
        </TabsContent>
        
        <TabsContent value="configuration" className="size-full p-8 overflow-auto">
          <EnvironmentConfig selectedEnvironment={selectedEnvironment} />
        </TabsContent>
      </Tabs>
    </div>
  );
}
