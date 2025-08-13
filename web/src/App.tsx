import * as yaml from "js-yaml";
import { AlertCircle, CheckCircle, Loader2 } from "lucide-react";
import { useCallback, useEffect, useState } from "react";
import { ReactFlowProvider } from "reactflow";
import { type AccountInfo, infrastructureApi } from "./api/infrastructure";
import { AddAmplifyDialog } from "./components/AddAmplifyDialog";
import { AddEventTaskDialog } from "./components/AddEventTaskDialog";
import { AddScheduledTaskDialog } from "./components/AddScheduledTaskDialog";
import { AddServiceDialog } from "./components/AddServiceDialog";
import { DeploymentCanvas } from "./components/DeploymentCanvas";
import { EnvironmentSelector } from "./components/EnvironmentSelector";
import { Sidebar } from "./components/Sidebar";
import { Toaster } from "./components/ui/sonner";
import { usePricing } from "./hooks/use-pricing";
// Removed Tabs import - no longer needed
import type { ComponentNode } from "./types";
import type { YamlInfrastructureConfig } from "./types/yamlConfig";

export default function App() {
	const [selectedNode, setSelectedNode] = useState<ComponentNode | null>(null);
	const [sidebarOpen, setSidebarOpen] = useState(false);
	const [selectedEnvironment, setSelectedEnvironment] = useState<string | null>(
		null,
	);
	const [showEnvSelector, setShowEnvSelector] = useState(false);
	const [config, setConfig] = useState<YamlInfrastructureConfig | null>(null);
	const [accountInfo, setAccountInfo] = useState<AccountInfo | null>(null);
	const [showAddServiceDialog, setShowAddServiceDialog] = useState(false);
	const [showAddScheduledTaskDialog, setShowAddScheduledTaskDialog] =
		useState(false);
	const [showAddEventTaskDialog, setShowAddEventTaskDialog] = useState(false);
	const [showAddAmplifyDialog, setShowAddAmplifyDialog] = useState(false);
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
			} catch (error) {
				console.error("Failed to check active environment:", error);
				setShowEnvSelector(true);
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

	const handleConfigChange = async (
		updates: Partial<YamlInfrastructureConfig>,
	) => {
		if (config) {
			const updatedConfig = { ...config, ...updates };
			setConfig(updatedConfig);
			await saveConfigToBackend(updatedConfig);
		}
	};

	const saveConfigToBackend = async (
		updatedConfig: YamlInfrastructureConfig,
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
				yamlContent,
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
	};

	const handleAddService = async (
		service: NonNullable<YamlInfrastructureConfig["services"]>[0],
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
		task: NonNullable<YamlInfrastructureConfig["scheduled_tasks"]>[0],
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
		task: NonNullable<YamlInfrastructureConfig["event_processor_tasks"]>[0],
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
		amplifyApp: NonNullable<YamlInfrastructureConfig["amplify_apps"]>[0],
	) => {
		if (!config) return;

		const updatedConfig = {
			...config,
			amplify_apps: [...(config.amplify_apps || []), amplifyApp],
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
				(s) => s.name !== serviceName,
			);
		} else if (nodeType === "scheduled-task") {
			const taskName = nodeId.replace("scheduled-", "");
			updatedConfig.scheduled_tasks = (config.scheduled_tasks || []).filter(
				(t) => t.name !== taskName,
			);
		} else if (nodeType === "event-task") {
			const taskName = nodeId.replace("event-", "");
			updatedConfig.event_processor_tasks = (
				config.event_processor_tasks || []
			).filter((t) => t.name !== taskName);
		} else if (nodeType === "amplify") {
			const appName = nodeId.replace("amplify-", "");
			updatedConfig.amplify_apps = (config.amplify_apps || []).filter(
				(a) => a.name !== appName,
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

	const getAvailableServices = () => {
		const services = [
			"backend",
			...(config?.services || []).map((s) => s.name),
		];
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
						<div className="hidden xl:flex items-center gap-3 px-4 py-2">
							{/* Environment */}
							<div className="flex items-center gap-2">
								<div className="w-2 h-2 bg-green-500 rounded-full animate-pulse" />
								<span className="text-xs text-gray-400 uppercase tracking-wide">
									Environment
								</span>
								<span className="text-sm font-semibold text-white">
									{selectedEnvironment}
								</span>
							</div>

							{config && (
								<>
									<div className="h-5 w-px bg-gray-600" />

									{/* Project */}
									<div className="flex items-center gap-2">
										<span className="text-xs text-gray-400 uppercase tracking-wide">
											Project
										</span>
										<span className="text-sm font-semibold text-white">
											{config.project}
										</span>
									</div>

									<div className="h-5 w-px bg-gray-600" />

									{/* Region */}
									<div className="flex items-center gap-2 whitespace-nowrap">
										<span className="text-xs text-gray-400 uppercase tracking-wide">
											Region
										</span>
										<span className="text-sm font-semibold text-white">
											{config.region}
										</span>
									</div>

									{activeEnvironmentProfile && activeEnvironmentAccountId && (
										<>
											<div className="h-5 w-px bg-gray-600" />

											{/* AWS Profile */}
											<div className="flex items-center gap-2">
												<span className="text-xs text-gray-400 uppercase tracking-wide">
													Profile
												</span>
												<span className="text-sm font-semibold text-white">
													{activeEnvironmentProfile}
												</span>
											</div>

											<div className="h-5 w-px bg-gray-600" />

											{/* Account ID */}
											<div className="flex items-center gap-2">
												<span className="text-xs text-gray-400 uppercase tracking-wide">
													Account
												</span>
												<span className="text-sm font-semibold text-white">
													{activeEnvironmentAccountId}
												</span>
											</div>
										</>
									)}
								</>
							)}
						</div>

						{/* Vertical layout for medium screens */}
						<div className="hidden sm:block xl:hidden px-3 py-2 space-y-1.5">
							{/* Environment */}
							<div className="flex items-center gap-2">
								<div className="w-2 h-2 bg-green-500 rounded-full animate-pulse"></div>
								<span className="text-xs text-gray-400 uppercase tracking-wide">
									Environment
								</span>
								<span className="text-sm font-semibold text-white">
									{selectedEnvironment}
								</span>
							</div>

							{config && (
								<>
									{/* Two column layout for other info */}
									<div className="grid grid-cols-2 gap-x-4 gap-y-1">
										{/* Project */}
										<div className="flex items-center gap-2">
											<span className="text-xs text-gray-400 uppercase tracking-wide">
												Project
											</span>
											<span className="text-sm font-semibold text-white">
												{config.project}
											</span>
										</div>

										{/* Region */}
										<div className="flex items-center gap-2">
											<span className="text-xs text-gray-400 uppercase tracking-wide">
												Region
											</span>
											<span className="text-sm font-semibold text-white">
												{config.region}
											</span>
										</div>

										{activeEnvironmentProfile && (
											<div className="flex items-center gap-2">
												<span className="text-xs text-gray-400 uppercase tracking-wide">
													Profile
												</span>
												<span className="text-sm font-semibold text-white truncate">
													{activeEnvironmentProfile}
												</span>
											</div>
										)}

										{activeEnvironmentAccountId && (
											<div className="flex items-center gap-2">
												<span className="text-xs text-gray-400 uppercase tracking-wide">
													Account
												</span>
												<span className="text-sm font-semibold text-white">
													{activeEnvironmentAccountId}
												</span>
											</div>
										)}
									</div>
								</>
							)}
						</div>

						{/* Mobile layout for super narrow screens */}
						<div className="sm:hidden px-3 py-2 space-y-1">
							{/* Environment */}
							<div className="flex items-center gap-2">
								<div className="w-2 h-2 bg-green-500 rounded-full animate-pulse"></div>
								<span className="text-xs text-gray-400 uppercase tracking-wide">
									Env
								</span>
								<span className="text-sm font-semibold text-white">
									{selectedEnvironment}
								</span>
							</div>

							{config && (
								<div className="space-y-1">
									{/* Project & Region on same line */}
									<div className="flex items-center gap-4">
										<div className="flex items-center gap-1">
											<span className="text-xs text-gray-400">Proj:</span>
											<span className="text-xs font-semibold text-white">
												{config.project}
											</span>
										</div>
										<div className="flex items-center gap-1">
											<span className="text-xs text-gray-400">Reg:</span>
											<span className="text-xs font-semibold text-white">
												{config.region}
											</span>
										</div>
									</div>

									{/* Profile & Account on same line if both exist */}
									{(activeEnvironmentProfile || activeEnvironmentAccountId) && (
										<div className="flex items-center gap-4">
											{activeEnvironmentProfile && (
												<div className="flex items-center gap-1 min-w-0">
													<span className="text-xs text-gray-400">Prof:</span>
													<span className="text-xs font-semibold text-white truncate">
														{activeEnvironmentProfile}
													</span>
												</div>
											)}
											{activeEnvironmentAccountId && (
												<div className="flex items-center gap-1">
													<span className="text-xs text-gray-400">Acct:</span>
													<span className="text-xs font-semibold text-white">
														{activeEnvironmentAccountId}
													</span>
												</div>
											)}
										</div>
									)}
								</div>
							)}
						</div>
					</div>
				)}
			</div>

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
					onAddAmplify={() => setShowAddAmplifyDialog(true)}
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

			<AddAmplifyDialog
				open={showAddAmplifyDialog}
				onClose={() => setShowAddAmplifyDialog(false)}
				onAdd={handleAddAmplify}
				existingApps={getExistingAmplifyApps()}
				environmentName={selectedEnvironment || undefined}
				projectName={config?.project}
			/>

			{/* Toast notifications */}
			<Toaster />
		</div>
	);
}
