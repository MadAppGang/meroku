import { useState, useCallback, useEffect } from "react";
import { ReactFlowProvider } from "reactflow";
import * as yaml from "js-yaml";
import { CheckCircle, AlertCircle, Loader2 } from "lucide-react";
import { DeploymentCanvas } from "./components/DeploymentCanvas";
import { Sidebar } from "./components/Sidebar";
import { EnvironmentSelector } from "./components/EnvironmentSelector";
import { AddServiceDialog } from "./components/AddServiceDialog";
import { AddScheduledTaskDialog } from "./components/AddScheduledTaskDialog";
import { AddEventTaskDialog } from "./components/AddEventTaskDialog";
// Removed Tabs import - no longer needed
import type { ComponentNode } from "./types";
import type { YamlInfrastructureConfig } from "./types/yamlConfig";
import { infrastructureApi, type AccountInfo } from "./api/infrastructure";

export default function App() {
  const [selectedNode, setSelectedNode] = useState<ComponentNode | null>(null);
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const [selectedEnvironment, setSelectedEnvironment] = useState<string | null>(null);
  const [showEnvSelector, setShowEnvSelector] = useState(true);
  const [config, setConfig] = useState<YamlInfrastructureConfig | null>(null);
  const [accountInfo, setAccountInfo] = useState<AccountInfo | null>(null);
  const [showAddServiceDialog, setShowAddServiceDialog] = useState(false);
  const [showAddScheduledTaskDialog, setShowAddScheduledTaskDialog] = useState(false);
  const [showAddEventTaskDialog, setShowAddEventTaskDialog] = useState(false);
  const [saveStatus, setSaveStatus] = useState<'idle' | 'saving' | 'success' | 'error'>('idle');

  const handleNodeSelect = useCallback((node: ComponentNode | null) => {
    setSelectedNode(node);
    setSidebarOpen(!!node);
  }, []);

  const handleEnvironmentSelect = useCallback((environment: string) => {
    setSelectedEnvironment(environment);
    setShowEnvSelector(false);
  }, []);

  // Load configuration and account info when environment is selected
  useEffect(() => {
    if (selectedEnvironment) {
      loadConfiguration(selectedEnvironment);
      loadAccountInfo();
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

  const loadAccountInfo = async () => {
    try {
      const info = await infrastructureApi.getAccountInfo();
      setAccountInfo(info);
    } catch (error) {
      console.error("Failed to load account info:", error);
    }
  };

  const handleConfigChange = async (updates: Partial<YamlInfrastructureConfig>) => {
    if (config) {
      const updatedConfig = { ...config, ...updates };
      setConfig(updatedConfig);
      await saveConfigToBackend(updatedConfig);
    }
  };

  const saveConfigToBackend = async (updatedConfig: YamlInfrastructureConfig) => {
    if (!selectedEnvironment) return;
    
    setSaveStatus('saving');
    
    try {
      const yamlContent = yaml.dump(updatedConfig, {
        indent: 2,
        lineWidth: -1,
        noRefs: true,
        sortKeys: false,
      });
      await infrastructureApi.updateEnvironmentConfig(selectedEnvironment, yamlContent);
      console.log('Configuration saved successfully');
      setSaveStatus('success');
      // Reset status after 2 seconds
      setTimeout(() => setSaveStatus('idle'), 2000);
    } catch (error) {
      console.error('Failed to save configuration:', error);
      setSaveStatus('error');
      // Reset status after 3 seconds
      setTimeout(() => setSaveStatus('idle'), 3000);
    } finally {
    }
  };

  const handleAddService = async (service: any) => {
    if (!config) return;
    
    const updatedConfig = {
      ...config,
      services: [...(config.services || []), service],
    };
    // Optimistic update - update UI immediately
    setConfig(updatedConfig);
    // Save to backend in the background
    await saveConfigToBackend(updatedConfig);
  };

  const handleAddScheduledTask = async (task: any) => {
    if (!config) return;
    
    const updatedConfig = {
      ...config,
      scheduled_tasks: [...(config.scheduled_tasks || []), task],
    };
    setConfig(updatedConfig);
    await saveConfigToBackend(updatedConfig);
  };

  const handleAddEventTask = async (task: any) => {
    if (!config) return;
    
    const updatedConfig = {
      ...config,
      event_processor_tasks: [...(config.event_processor_tasks || []), task],
    };
    setConfig(updatedConfig);
    await saveConfigToBackend(updatedConfig);
  };

  const handleDeleteNode = async (nodeId: string, nodeType: string) => {
    if (!config) return;
    
    let updatedConfig = { ...config };
    
    if (nodeType === 'service') {
      const serviceName = nodeId.replace('service-', '');
      updatedConfig.services = (config.services || []).filter(s => s.name !== serviceName);
    } else if (nodeType === 'scheduled-task') {
      const taskName = nodeId.replace('scheduled-', '');
      updatedConfig.scheduled_tasks = (config.scheduled_tasks || []).filter(t => t.name !== taskName);
    } else if (nodeType === 'event-task') {
      const taskName = nodeId.replace('event-', '');
      updatedConfig.event_processor_tasks = (config.event_processor_tasks || []).filter(t => t.name !== taskName);
    }
    
    setConfig(updatedConfig);
    await saveConfigToBackend(updatedConfig);
  };

  const getExistingServices = () => {
    return (config?.services || []).map(s => s.name);
  };

  const getExistingScheduledTasks = () => {
    return (config?.scheduled_tasks || []).map(t => t.name);
  };

  const getExistingEventTasks = () => {
    return (config?.event_processor_tasks || []).map(t => t.name);
  };

  const getAvailableServices = () => {
    const services = ['backend', ...(config?.services || []).map(s => s.name)];
    return services;
  };

  return (
    <div className="size-full bg-gray-950 text-white relative overflow-hidden">
      <EnvironmentSelector 
        open={showEnvSelector} 
        onSelect={handleEnvironmentSelect} 
      />
      
      {/* Top Panel */}
      <div className="absolute top-4 left-1/2 -translate-x-1/2 z-10 px-4 max-w-full">
        {selectedEnvironment && (
          <div className="bg-gray-800/95 backdrop-blur-sm rounded-lg border border-gray-700 shadow-lg">
            {/* Horizontal layout for larger screens */}
            <div className="hidden sm:flex items-center gap-3 px-4 py-2">
              {/* Environment */}
              <div className="flex items-center gap-2">
                <div className="w-2 h-2 bg-green-500 rounded-full animate-pulse"></div>
                <span className="text-xs text-gray-400 uppercase tracking-wide">Environment</span>
                <span className="text-sm font-semibold text-white">{selectedEnvironment}</span>
                <button
                  onClick={() => setShowEnvSelector(true)}
                  className="ml-1 px-2 py-0.5 text-xs text-blue-400 hover:text-blue-300 hover:bg-blue-500/10 rounded transition-colors"
                >
                  Change
                </button>
              </div>
              
              {config && (
                <>
                  <div className="h-5 w-px bg-gray-600"></div>
                  
                  {/* Project */}
                  <div className="flex items-center gap-2">
                    <span className="text-xs text-gray-400 uppercase tracking-wide">Project</span>
                    <span className="text-sm font-semibold text-white">{config.project}</span>
                  </div>
                  
                  <div className="h-5 w-px bg-gray-600"></div>
                  
                  {/* Region */}
                  <div className="flex items-center gap-2 whitespace-nowrap">
                    <span className="text-xs text-gray-400 uppercase tracking-wide">Region</span>
                    <span className="text-sm font-semibold text-white">{config.region}</span>
                  </div>
                </>
              )}
            </div>
            
            {/* Vertical layout for smaller screens */}
            <div className="sm:hidden px-3 py-2 space-y-2">
              {/* Environment */}
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <div className="w-2 h-2 bg-green-500 rounded-full animate-pulse"></div>
                  <span className="text-xs text-gray-400 uppercase tracking-wide">Env</span>
                  <span className="text-sm font-semibold text-white">{selectedEnvironment}</span>
                </div>
                <button
                  onClick={() => setShowEnvSelector(true)}
                  className="px-2 py-0.5 text-xs text-blue-400 hover:text-blue-300 hover:bg-blue-500/10 rounded transition-colors"
                >
                  Change
                </button>
              </div>
              
              {config && (
                <>
                  {/* Project */}
                  <div className="flex items-center gap-2">
                    <span className="text-xs text-gray-400 uppercase tracking-wide ml-4">Project</span>
                    <span className="text-sm font-semibold text-white">{config.project}</span>
                  </div>
                  
                  {/* Region */}
                  <div className="flex items-center gap-2">
                    <span className="text-xs text-gray-400 uppercase tracking-wide ml-4">Region</span>
                    <span className="text-sm font-semibold text-white">{config.region}</span>
                  </div>
                </>
              )}
            </div>
          </div>
        )}
      </div>
      
      {/* Save Status Indicator */}
      {saveStatus !== 'idle' && (
        <div className="absolute top-4 right-4 z-50">
          <div className={`
            flex items-center gap-2 px-4 py-2 rounded-lg shadow-lg
            ${saveStatus === 'saving' ? 'bg-blue-900/90 border border-blue-700' : ''}
            ${saveStatus === 'success' ? 'bg-green-900/90 border border-green-700' : ''}
            ${saveStatus === 'error' ? 'bg-red-900/90 border border-red-700' : ''}
          `}>
            {saveStatus === 'saving' && (
              <>
                <Loader2 className="w-4 h-4 animate-spin text-blue-400" />
                <span className="text-sm text-blue-200">Saving configuration...</span>
              </>
            )}
            {saveStatus === 'success' && (
              <>
                <CheckCircle className="w-4 h-4 text-green-400" />
                <span className="text-sm text-green-200">Configuration saved</span>
              </>
            )}
            {saveStatus === 'error' && (
              <>
                <AlertCircle className="w-4 h-4 text-red-400" />
                <span className="text-sm text-red-200">Failed to save configuration</span>
              </>
            )}
          </div>
        </div>
      )}
      
      {/* Main Content */}
      <ReactFlowProvider>
        {/* Main Canvas */}
        <DeploymentCanvas
          onNodeSelect={handleNodeSelect}
          selectedNode={selectedNode}
          config={config}
          environmentName={selectedEnvironment || undefined}
          onAddService={() => setShowAddServiceDialog(true)}
          onAddScheduledTask={() => setShowAddScheduledTaskDialog(true)}
          onAddEventTask={() => setShowAddEventTaskDialog(true)}
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
          accountInfo={accountInfo || undefined}
          onDeleteNode={handleDeleteNode}
        />
      </ReactFlowProvider>
      
      {/* Dialogs */}
      <AddServiceDialog
        open={showAddServiceDialog}
        onClose={() => setShowAddServiceDialog(false)}
        onAdd={handleAddService}
        existingServices={getExistingServices()}
      />
      
      <AddScheduledTaskDialog
        open={showAddScheduledTaskDialog}
        onClose={() => setShowAddScheduledTaskDialog(false)}
        onAdd={handleAddScheduledTask}
        existingTasks={getExistingScheduledTasks()}
      />
      
      <AddEventTaskDialog
        open={showAddEventTaskDialog}
        onClose={() => setShowAddEventTaskDialog(false)}
        onAdd={handleAddEventTask}
        existingTasks={getExistingEventTasks()}
        availableServices={getAvailableServices()}
      />
    </div>
  );
}
