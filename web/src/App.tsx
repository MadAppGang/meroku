import * as yaml from "js-yaml";
import { AlertCircle, CheckCircle, Loader2 } from "lucide-react";
import { useCallback, useEffect, useState } from "react";
import { ReactFlowProvider } from "reactflow";
import { type AccountInfo, infrastructureApi } from "./api/infrastructure";
import { AddAmplifyDialog } from "./components/AddAmplifyDialog";
import { AddCloudFrontDialog } from "./components/AddCloudFrontDialog";
import { AddEventTaskDialog } from "./components/AddEventTaskDialog";
import { AddScheduledTaskDialog } from "./components/AddScheduledTaskDialog";
import { AddServiceDialog } from "./components/AddServiceDialog";
import { CustomTerraformManager } from "./components/CustomTerraformManager";
import { DeploymentCanvas } from "./components/DeploymentCanvas";
import { EnvironmentSelector } from "./components/EnvironmentSelector";
import { Sidebar } from "./components/Sidebar";
import { StatusLine } from "./components/StatusLine";
import { Toaster } from "./components/ui/sonner";
import { PricingProvider } from "./contexts/PricingContext";
import { usePricing } from "./hooks/use-pricing";
import type { ComponentNode } from "./types";
import type { YamlInfrastructureConfig } from "./types/yamlConfig";
import { mergeConfigUpdates } from "./utils/configMerge";

export default function App() {
  const [selectedNode, setSelectedNode] = useState<ComponentNode | null>(null);
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const [selectedEnvironment, setSelectedEnvironment] = useState<string | null>(
    null
  );
  const [viewMode, setViewMode] = useState<"visual" | "code">("visual");
  const [showEnvSelector, setShowEnvSelector] = useState(false);
  const [config, setConfig] = useState<YamlInfrastructureConfig | null>(null);
  const [accountInfo, setAccountInfo] = useState<AccountInfo | null>(null);
  const [showAddServiceDialog, setShowAddServiceDialog] = useState(false);
  const [showAddScheduledTaskDialog, setShowAddScheduledTaskDialog] =
    useState(false);
  const [showAddEventTaskDialog, setShowAddEventTaskDialog] = useState(false);
  const [showAddAmplifyDialog, setShowAddAmplifyDialog] = useState(false);
  const [showAddCloudFrontDialog, setShowAddCloudFrontDialog] = useState(false);
  const [saveStatus, setSaveStatus] = useState<
    "idle" | "saving" | "success" | "error"
  >("idle");
  const [activeEnvironmentProfile, setActiveEnvironmentProfile] = useState<
    string | null
  >(null);
  const [activeEnvironmentAccountId, setActiveEnvironmentAccountId] = useState<
    string | null
  >(null);
  const [pricingRefreshTrigger, setPricingRefreshTrigger] = useState(0);
  const [backendError, setBackendError] = useState<string | null>(null);
  const [connecting, setConnecting] = useState(true);

  // Use pricing hook with refresh trigger
  const { pricing } = usePricing(selectedEnvironment, pricingRefreshTrigger);

  const handleNodeSelect = useCallback((node: ComponentNode | null) => {
    setSelectedNode(node);
    setSidebarOpen(!!node);
  }, []);

  const handleEnvironmentSelect = useCallback(async (environment: string) => {
    setSelectedEnvironment(environment);
    setShowEnvSelector(false);

    // Fetch the updated environment info to get profile and account ID
    try {
      const environments = await infrastructureApi.getEnvironments();
      const selectedEnv = environments.find((env) => env.name === environment);
      if (selectedEnv) {
        setActiveEnvironmentProfile(selectedEnv.profile || null);
        setActiveEnvironmentAccountId(selectedEnv.accountId || null);
      }
    } catch (error) {
      console.error("Failed to fetch environment details:", error);
    }
  }, []);

  // Check for active environment on mount
  useEffect(() => {
    const checkActiveEnvironment = async () => {
      try {
        const environments = await infrastructureApi.getEnvironments();
        const activeEnv = environments.find((env) => env.isActive);

        if (activeEnv) {
          // Use the active environment automatically
          setSelectedEnvironment(activeEnv.name);
          setActiveEnvironmentProfile(activeEnv.profile || null);
          setActiveEnvironmentAccountId(activeEnv.accountId || null);
          setShowEnvSelector(false);
        } else {
          // No active environment, show selector
          setShowEnvSelector(true);
        }
        setConnecting(false);
      } catch (error) {
        console.error("Failed to check active environment:", error);

        // Detect network/connection errors (backend unreachable)
        const isNetworkError =
          error instanceof TypeError ||
          (error instanceof Error &&
            /Failed to fetch|NetworkError|net::ERR_|SSL|ECONNREFUSED/i.test(
              error.message
            ));

        if (isNetworkError) {
          setBackendError(
            error instanceof Error ? error.message : String(error)
          );
        } else {
          setShowEnvSelector(true);
        }
        setConnecting(false);
      }
    };

    checkActiveEnvironment();
  }, []);

  const loadConfiguration = useCallback(async (envName: string) => {
    try {
      const content = await infrastructureApi.getEnvironmentConfig(envName);
      const parsed = yaml.load(content) as YamlInfrastructureConfig;
      setConfig(parsed);
    } catch (error) {
      console.error("Failed to load configuration:", error);
    }
  }, []);

  const loadAccountInfo = useCallback(async () => {
    try {
      const info = await infrastructureApi.getAccountInfo();
      setAccountInfo(info);
    } catch (error) {
      console.error("Failed to load account info:", error);
    }
  }, []);

  // Load configuration and account info when environment is selected
  useEffect(() => {
    if (selectedEnvironment) {
      loadConfiguration(selectedEnvironment);
      loadAccountInfo();
    }
  }, [selectedEnvironment, loadAccountInfo, loadConfiguration]);

  const saveConfigToBackend = useCallback(async (
    updatedConfig: YamlInfrastructureConfig
  ) => {
    if (!selectedEnvironment) return;

    setSaveStatus("saving");

    try {
      const yamlContent = yaml.dump(updatedConfig, {
        indent: 2,
        lineWidth: -1,
        noRefs: true,
        sortKeys: false,
      });
      await infrastructureApi.updateEnvironmentConfig(
        selectedEnvironment,
        yamlContent
      );
      console.log("Configuration saved successfully");
      setSaveStatus("success");

      // Refresh pricing after configuration update
      setPricingRefreshTrigger((prev) => prev + 1);

      // Reset status after 2 seconds
      setTimeout(() => setSaveStatus("idle"), 2000);
    } catch (error) {
      console.error("Failed to save configuration:", error);
      setSaveStatus("error");
      // Reset status after 3 seconds
      setTimeout(() => setSaveStatus("idle"), 3000);
    } finally {
    }
  }, [selectedEnvironment]);

  const handleConfigChange = useCallback(async (
    updates: Partial<YamlInfrastructureConfig>
  ) => {
    setConfig((prevConfig) => {
      if (!prevConfig) return prevConfig;

      // Deep merge nested objects to prevent stale closure overwrites, and to
      // preserve fields no editor panel surfaces (e.g. pubsub_appsync.jwks_uri).
      const updatedConfig = mergeConfigUpdates(prevConfig, updates);

      // Same reference means nothing changed - skip re-render and save
      if (updatedConfig === prevConfig) {
        return prevConfig;
      }

      // Call saveConfigToBackend asynchronously (don't await here to avoid blocking)
      saveConfigToBackend(updatedConfig);
      return updatedConfig;
    });
  }, [saveConfigToBackend]);

  const handleAddService = async (
    service: NonNullable<YamlInfrastructureConfig["services"]>[0]
  ) => {
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

  const handleAddScheduledTask = async (
    task: NonNullable<YamlInfrastructureConfig["scheduled_tasks"]>[0]
  ) => {
    if (!config) return;

    const updatedConfig = {
      ...config,
      scheduled_tasks: [...(config.scheduled_tasks || []), task],
    };
    setConfig(updatedConfig);
    await saveConfigToBackend(updatedConfig);
  };

  const handleAddEventTask = async (
    task: NonNullable<YamlInfrastructureConfig["event_processor_tasks"]>[0]
  ) => {
    if (!config) return;

    const updatedConfig = {
      ...config,
      event_processor_tasks: [...(config.event_processor_tasks || []), task],
    };
    setConfig(updatedConfig);
    await saveConfigToBackend(updatedConfig);
  };

  const handleAddAmplify = async (
    amplifyApp: NonNullable<YamlInfrastructureConfig["amplify_apps"]>[0]
  ) => {
    if (!config) return;

    const updatedConfig = {
      ...config,
      amplify_apps: [...(config.amplify_apps || []), amplifyApp],
    };
    setConfig(updatedConfig);
    await saveConfigToBackend(updatedConfig);
  };

  const handleAddCloudFront = async (
    distribution: NonNullable<YamlInfrastructureConfig["cloudfront_distributions"]>[0]
  ) => {
    if (!config) return;

    const updatedConfig = {
      ...config,
      cloudfront_distributions: [...(config.cloudfront_distributions || []), distribution],
    };
    setConfig(updatedConfig);
    await saveConfigToBackend(updatedConfig);
  };

  const handleDeleteNode = async (nodeId: string, nodeType: string) => {
    if (!config) return;

    const updatedConfig = { ...config };

    if (nodeType === "service") {
      const serviceName = nodeId.replace("service-", "");
      updatedConfig.services = (config.services || []).filter(
        (s) => s.name !== serviceName
      );
    } else if (nodeType === "scheduled-task") {
      const taskName = nodeId.replace("scheduled-", "");
      updatedConfig.scheduled_tasks = (config.scheduled_tasks || []).filter(
        (t) => t.name !== taskName
      );
    } else if (nodeType === "event-task") {
      const taskName = nodeId.replace("event-", "");
      updatedConfig.event_processor_tasks = (
        config.event_processor_tasks || []
      ).filter((t) => t.name !== taskName);
    } else if (nodeType === "amplify") {
      const appName = nodeId.replace("amplify-", "");
      updatedConfig.amplify_apps = (config.amplify_apps || []).filter(
        (a) => a.name !== appName
      );
    } else if (nodeType === "cloudfront") {
      const distName = nodeId.replace("cloudfront-", "");
      updatedConfig.cloudfront_distributions = (config.cloudfront_distributions || []).filter(
        (d) => d.name !== distName
      );
    }

    setConfig(updatedConfig);
    await saveConfigToBackend(updatedConfig);
  };

  const getExistingServices = () => {
    return (config?.services || []).map((s) => s.name);
  };

  const getExistingScheduledTasks = () => {
    return (config?.scheduled_tasks || []).map((t) => t.name);
  };

  const getExistingEventTasks = () => {
    return (config?.event_processor_tasks || []).map((t) => t.name);
  };

  const getExistingAmplifyApps = () => {
    return (config?.amplify_apps || []).map((a) => a.name);
  };

  const getExistingCloudFrontDistributions = () => {
    return (config?.cloudfront_distributions || []).map((d) => d.name);
  };

  const getAvailableServices = () => {
    const services = [
      "backend",
      ...(config?.services || []).map((s) => s.name),
    ];
    return services;
  };

  return (
    <PricingProvider>
      <div className="h-screen w-full bg-gray-950 text-white relative overflow-hidden flex flex-col">
        {connecting ? (
          <div className="flex-1 flex items-center justify-center">
            <div className="flex flex-col items-center gap-8 text-center px-6">
              {/* Animated logo / spinner */}
              <div className="relative">
                <div className="w-20 h-20 rounded-2xl bg-gradient-to-br from-blue-500 to-purple-600 flex items-center justify-center shadow-lg shadow-blue-500/20">
                  <svg viewBox="0 0 24 24" className="w-10 h-10 text-white" fill="none" stroke="currentColor" strokeWidth="1.5">
                    <path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z" />
                    <polyline points="7.5 4.21 12 6.81 16.5 4.21" />
                    <line x1="12" y1="22" x2="12" y2="6.81" className="animate-pulse" />
                  </svg>
                </div>
                {/* Orbiting dot */}
                <div className="absolute inset-0 animate-spin" style={{ animationDuration: '2s' }}>
                  <div className="absolute -top-1 left-1/2 -translate-x-1/2 w-2.5 h-2.5 rounded-full bg-blue-400 shadow-md shadow-blue-400/50" />
                </div>
              </div>

              <div className="space-y-2">
                <h1 className="text-xl font-semibold text-white">Connecting to backend</h1>
                <div className="flex items-center justify-center gap-1.5">
                  <div className="w-1.5 h-1.5 rounded-full bg-blue-400 animate-bounce" style={{ animationDelay: '0ms' }} />
                  <div className="w-1.5 h-1.5 rounded-full bg-blue-400 animate-bounce" style={{ animationDelay: '150ms' }} />
                  <div className="w-1.5 h-1.5 rounded-full bg-blue-400 animate-bounce" style={{ animationDelay: '300ms' }} />
                </div>
              </div>

              <p className="text-sm text-gray-500 max-w-xs">
                Waiting for the meroku server on port 8080
              </p>
            </div>
          </div>
        ) : backendError ? (
          <div className="flex-1 flex items-center justify-center">
            <div className="flex flex-col items-center gap-6 max-w-md text-center px-6">
              <AlertCircle className="w-16 h-16 text-red-400" />
              <h1 className="text-2xl font-semibold text-white">
                Backend Not Available
              </h1>
              <p className="text-gray-400">
                The meroku backend server is not reachable. The web interface
                requires a running backend to function.
              </p>
              <p className="text-sm text-gray-500 font-mono bg-gray-900 rounded-lg px-4 py-2 w-full break-all">
                {backendError}
              </p>
              <button
                type="button"
                onClick={() => {
                  setBackendError(null);
                  setConnecting(true);
                  (async () => {
                    try {
                      const environments =
                        await infrastructureApi.getEnvironments();
                      const activeEnv = environments.find(
                        (env) => env.isActive
                      );
                      if (activeEnv) {
                        setSelectedEnvironment(activeEnv.name);
                        setActiveEnvironmentProfile(activeEnv.profile || null);
                        setActiveEnvironmentAccountId(
                          activeEnv.accountId || null
                        );
                        setShowEnvSelector(false);
                      } else {
                        setShowEnvSelector(true);
                      }
                      setConnecting(false);
                    } catch (retryError) {
                      console.error(
                        "Retry failed:",
                        retryError
                      );
                      setConnecting(false);
                      const isNetworkError =
                        retryError instanceof TypeError ||
                        (retryError instanceof Error &&
                          /Failed to fetch|NetworkError|net::ERR_|SSL|ECONNREFUSED/i.test(
                            retryError.message
                          ));
                      if (isNetworkError) {
                        setBackendError(
                          retryError instanceof Error
                            ? retryError.message
                            : String(retryError)
                        );
                      } else {
                        setShowEnvSelector(true);
                      }
                    }
                  })();
                }}
                className="px-6 py-2.5 bg-blue-600 hover:bg-blue-500 text-white rounded-lg font-medium transition-colors"
              >
                Retry
              </button>
              <p className="text-xs text-gray-600">
                Make sure the meroku backend is running (task tui)
              </p>
            </div>
          </div>
        ) : (
        <>
        <EnvironmentSelector
          open={showEnvSelector}
          onSelect={handleEnvironmentSelect}
        />

      {/* Top Panel - Removed in favor of StatusLine */}

      {/* Save Status Indicator */}
      {saveStatus !== "idle" && (
        <div className="absolute top-4 right-4 z-50">
          <div
            className={`
            flex items-center gap-2 px-4 py-2 rounded-lg shadow-lg
            ${
              saveStatus === "saving"
                ? "bg-blue-900/90 border border-blue-700"
                : ""
            }
            ${
              saveStatus === "success"
                ? "bg-green-900/90 border border-green-700"
                : ""
            }
            ${
              saveStatus === "error"
                ? "bg-red-900/90 border border-red-700"
                : ""
            }
          `}
          >
            {saveStatus === "saving" && (
              <>
                <Loader2 className="w-4 h-4 animate-spin text-blue-400" />
                <span className="text-sm text-blue-200">
                  Saving configuration...
                </span>
              </>
            )}
            {saveStatus === "success" && (
              <>
                <CheckCircle className="w-4 h-4 text-green-400" />
                <span className="text-sm text-green-200">
                  Configuration saved
                </span>
              </>
            )}
            {saveStatus === "error" && (
              <>
                <AlertCircle className="w-4 h-4 text-red-400" />
                <span className="text-sm text-red-200">
                  Failed to save configuration
                </span>
              </>
            )}
          </div>
        </div>
      )}

      {/* Main Content */}
      <div className="flex-1 w-full relative overflow-hidden flex flex-col">
        {/* Main Canvas */}
        <div className="flex-1 w-full h-full absolute inset-0 z-10 bg-gray-950">
          <ReactFlowProvider>
            <DeploymentCanvas
              onNodeSelect={handleNodeSelect}
              selectedNode={selectedNode}
              config={config}
              environmentName={selectedEnvironment || undefined}
              onAddService={() => setShowAddServiceDialog(true)}
              onAddScheduledTask={() => setShowAddScheduledTaskDialog(true)}
              onAddEventTask={() => setShowAddEventTaskDialog(true)}
              onAddAmplify={() => setShowAddAmplifyDialog(true)}
              onAddCloudFront={() => setShowAddCloudFrontDialog(true)}
              onManageCustomTerraform={() => setViewMode("code")}
              pricing={pricing}
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
        </div>

        {/* Custom Terraform Manager Overlay */}
        <div className={`flex-1 w-full h-full absolute inset-0 bg-gray-950 transition-opacity duration-300 ${viewMode === 'code' ? 'z-50 opacity-100' : 'z-0 opacity-0 pointer-events-none'}`}>
          {selectedEnvironment && viewMode === 'code' && (
            <CustomTerraformManager
              environment={selectedEnvironment}
              onClose={() => setViewMode("visual")}
            />
          )}
        </div>
      </div>

      {/* Dialogs */}
      <AddServiceDialog
        open={showAddServiceDialog}
        onClose={() => setShowAddServiceDialog(false)}
        onAdd={handleAddService}
        existingServices={getExistingServices()}
        config={config || {} as YamlInfrastructureConfig}
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
        config={config || {} as YamlInfrastructureConfig}
      />

      <AddAmplifyDialog
        open={showAddAmplifyDialog}
        onClose={() => setShowAddAmplifyDialog(false)}
        onAdd={handleAddAmplify}
        existingApps={getExistingAmplifyApps()}
        environmentName={selectedEnvironment || undefined}
        projectName={config?.project}
        config={config || undefined}
      />

      <AddCloudFrontDialog
        open={showAddCloudFrontDialog}
        onClose={() => setShowAddCloudFrontDialog(false)}
        onAdd={handleAddCloudFront}
        existingDistributions={getExistingCloudFrontDistributions()}
        config={config || undefined}
      />

      {/* Status Line */}
      <StatusLine
        selectedEnvironment={selectedEnvironment}
        config={config}
        activeEnvironmentProfile={activeEnvironmentProfile}
        activeEnvironmentAccountId={activeEnvironmentAccountId}
        viewMode={viewMode}
        onViewModeChange={setViewMode}
        onConfigChange={handleConfigChange}
      />

      {/* Toast notifications */}
      <Toaster />
        </>
        )}
      </div>
    </PricingProvider>
  );
}
