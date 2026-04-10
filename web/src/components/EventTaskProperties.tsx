import {
	AlertTriangle,
	ChevronDown,
	ChevronRight,
	Container,
	Settings,
	Zap,
} from "lucide-react";
import { useCallback, useId, useState } from "react";
import type { AccountInfo } from "../api/infrastructure";
import type { ComponentNode } from "../types";
import type {
	ECRConfig,
	EventBridgeRule,
	YamlInfrastructureConfig,
} from "../types/yamlConfig";
import { ECRConfigEditor } from "./ECRConfigEditor";
import { EventRulesList } from "./EventRulesList";
import { Alert, AlertDescription } from "./ui/alert";
import { Input } from "./ui/input";
import { Label } from "./ui/label";
import { Switch } from "./ui/switch";

interface EventTaskPropertiesProps {
	config: YamlInfrastructureConfig;
	onConfigChange: (config: Partial<YamlInfrastructureConfig>) => void;
	accountInfo?: AccountInfo;
	node: ComponentNode;
}

interface CollapsibleSectionProps {
	title: string;
	description?: string;
	icon: React.ElementType;
	iconColor?: string;
	defaultOpen?: boolean;
	children: React.ReactNode;
}

function CollapsibleSection({
	title,
	description,
	icon: Icon,
	iconColor = "text-gray-400",
	defaultOpen = false,
	children,
}: CollapsibleSectionProps) {
	const [isOpen, setIsOpen] = useState(defaultOpen);

	return (
		<div className="border border-gray-700 rounded-xl overflow-hidden bg-gray-800/30">
			<button
				type="button"
				onClick={() => setIsOpen(!isOpen)}
				className="w-full flex items-center justify-between px-4 py-3 hover:bg-gray-800/50 transition-colors"
			>
				<div className="flex items-center gap-3">
					<div className={`p-1.5 rounded-lg bg-gray-800 ${iconColor}`}>
						<Icon className="w-4 h-4" />
					</div>
					<div className="text-left">
						<h3 className="font-medium text-white text-sm">{title}</h3>
						{description && (
							<p className="text-xs text-gray-500">{description}</p>
						)}
					</div>
				</div>
				{isOpen ? (
					<ChevronDown className="w-4 h-4 text-gray-400" />
				) : (
					<ChevronRight className="w-4 h-4 text-gray-400" />
				)}
			</button>
			{isOpen && (
				<div className="px-4 pb-4 border-t border-gray-700/50">{children}</div>
			)}
		</div>
	);
}

export function EventTaskProperties({
	config,
	onConfigChange,
	accountInfo,
	node,
}: EventTaskPropertiesProps) {
	// Extract task name from node id (e.g., "event-order-processor" -> "order-processor")
	const taskName = node.id.replace("event-", "");

	// Find the event task configuration
	const eventTask = config.event_processor_tasks?.find(
		(task) => task.name === taskName,
	);

	// Local state for editing
	const [dockerImage, setDockerImage] = useState(
		() => eventTask?.docker_image || "",
	);
	const [ecrConfig, setEcrConfig] = useState<ECRConfig | undefined>(
		() => eventTask?.ecr_config || { mode: "create_ecr" },
	);
	const [cpu, setCpu] = useState(() => eventTask?.cpu?.toString() || "256");
	const [memory, setMemory] = useState(
		() => eventTask?.memory?.toString() || "512",
	);

	// Unique IDs for form elements
	const dockerImageId = useId();
	const cpuId = useId();
	const memoryId = useId();

	// ECR repository info
	const taskEcrRepoName = `${config.project}_task_${taskName}`;
	const taskEcrRepoUri = `${accountInfo?.accountId || config.ecr_account_id || "<ACCOUNT_ID>"}.dkr.ecr.${config.region}.amazonaws.com/${taskEcrRepoName}`;

	// Get rules (either from rules[] array or legacy fields)
	const getRules = useCallback((): EventBridgeRule[] => {
		if (!eventTask) return [];
		if (eventTask.rules && eventTask.rules.length > 0) {
			return eventTask.rules;
		}
		// Legacy format
		if (eventTask.rule_name && eventTask.sources && eventTask.detail_types) {
			return [
				{
					name: eventTask.rule_name,
					sources: eventTask.sources,
					detail_types: eventTask.detail_types,
				},
			];
		}
		return [];
	}, [eventTask]);

	const updateTaskConfig = useCallback(
		(updates: Partial<typeof eventTask>) => {
			if (!config.event_processor_tasks) return;

			const updatedTasks = config.event_processor_tasks.map((task) =>
				task.name === taskName ? { ...task, ...updates } : task,
			);

			onConfigChange({ event_processor_tasks: updatedTasks });
		},
		[config.event_processor_tasks, taskName, onConfigChange],
	);

	const handleRulesChange = useCallback(
		(newRules: EventBridgeRule[]) => {
			// When using rules[], clear legacy fields
			updateTaskConfig({
				rules: newRules,
				rule_name: undefined,
				sources: undefined,
				detail_types: undefined,
			});
		},
		[updateTaskConfig],
	);

	const handleEcrConfigChange = useCallback(
		(newConfig: ECRConfig | undefined) => {
			setEcrConfig(newConfig);
			updateTaskConfig({ ecr_config: newConfig });
		},
		[updateTaskConfig],
	);

	if (!eventTask) {
		return (
			<div className="text-center py-8 text-gray-500">
				<Zap className="w-8 h-8 mx-auto mb-2 opacity-50" />
				<p>Event task configuration not found</p>
			</div>
		);
	}

	return (
		<div className="space-y-4">
			{/* Enabled Toggle - at the very top */}
			<div className="flex items-center justify-between">
				<div className="flex-1">
					<Label htmlFor="enabled">Enabled</Label>
					<p className="text-xs text-gray-500 mt-1">
						When disabled, all settings are kept but the task is not deployed
					</p>
				</div>
				<Switch
					id="enabled"
					checked={eventTask.enabled !== false}
					onCheckedChange={(checked) =>
						updateTaskConfig({ enabled: checked })
					}
					className="data-[state=checked]:bg-green-500 data-[state=unchecked]:bg-gray-600"
				/>
			</div>

			{eventTask.enabled === false && (
				<Alert className="border-yellow-600 bg-yellow-900/20">
					<AlertTriangle className="h-4 w-4 text-yellow-400" />
					<AlertDescription className="text-xs text-gray-300">
						This event task is <strong>disabled</strong>. It will not be included in the next Terraform generation.
						All configuration is preserved and can be re-enabled at any time.
					</AlertDescription>
				</Alert>
			)}

			{/* Rules Section - Always visible, primary content */}
			<div className="space-y-4">
				<EventRulesList rules={getRules()} onRulesChange={handleRulesChange} />
			</div>

			{/* Container Configuration - Collapsible */}
			<CollapsibleSection
				title="Container Configuration"
				description="Docker image and resource settings"
				icon={Container}
				iconColor="text-cyan-400"
				defaultOpen={false}
			>
				<div className="pt-4 space-y-4">
					{/* ECR Configuration */}
					<div className="space-y-3">
						<Label className="text-sm font-medium">Image Source</Label>
						<ECRConfigEditor
							config={config}
							currentServiceName={taskName}
							currentServiceType="event_processor_tasks"
							ecrConfig={ecrConfig}
							onEcrConfigChange={handleEcrConfigChange}
							accountInfo={accountInfo}
						/>
					</div>

					<div className="space-y-2">
						<div className="flex items-center justify-between">
							<Label htmlFor={dockerImageId} className="text-sm">
								Image Tag
							</Label>
							<span className="text-xs text-gray-500">
								e.g., latest, v1.2.3
							</span>
						</div>
						<Input
							id={dockerImageId}
							value={dockerImage}
							onChange={(e) => {
								setDockerImage(e.target.value);
								updateTaskConfig({ docker_image: e.target.value });
							}}
							placeholder="latest"
							className="font-mono text-sm"
						/>
						{ecrConfig && (
							<div className="p-2 bg-cyan-950/30 border border-cyan-900/50 rounded-lg text-xs">
								<span className="text-gray-500">Full image: </span>
								<code className="text-cyan-400">
									{taskEcrRepoUri}:{dockerImage || "latest"}
								</code>
							</div>
						)}
					</div>

					{/* Resource Configuration */}
					<div className="pt-2 border-t border-gray-700/50">
						<div className="flex items-center gap-2 mb-3">
							<Settings className="w-4 h-4 text-gray-400" />
							<span className="text-sm font-medium">Resources</span>
						</div>
						<div className="grid grid-cols-2 gap-4">
							<div className="space-y-2">
								<Label htmlFor={cpuId} className="text-xs">
									CPU (units)
								</Label>
								<Input
									id={cpuId}
									value={cpu}
									onChange={(e) => {
										setCpu(e.target.value);
										updateTaskConfig({
											cpu: parseInt(e.target.value, 10) || 256,
										});
									}}
									placeholder="256"
									className="font-mono text-sm"
								/>
								<p className="text-xs text-gray-500">256 = 0.25 vCPU</p>
							</div>
							<div className="space-y-2">
								<Label htmlFor={memoryId} className="text-xs">
									Memory (MB)
								</Label>
								<Input
									id={memoryId}
									value={memory}
									onChange={(e) => {
										setMemory(e.target.value);
										updateTaskConfig({
											memory: parseInt(e.target.value, 10) || 512,
										});
									}}
									placeholder="512"
									className="font-mono text-sm"
								/>
							</div>
						</div>
					</div>
				</div>
			</CollapsibleSection>
		</div>
	);
}
