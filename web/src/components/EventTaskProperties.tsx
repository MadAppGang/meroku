import {
	Cloud,
	Container,
	Link,
	Send,
	Settings,
	Zap,
} from "lucide-react";
import { useState, useCallback } from "react";
import type { AccountInfo } from "../api/infrastructure";
import type { ComponentNode } from "../types";
import type { ECRConfig, EventBridgeRule, YamlInfrastructureConfig } from "../types/yamlConfig";
import { Button } from "./ui/button";
import {
	Card,
	CardContent,
	CardDescription,
	CardHeader,
	CardTitle,
} from "./ui/card";
import { Input } from "./ui/input";
import { Label } from "./ui/label";
import { ECRConfigEditor } from "./ECRConfigEditor";
import { EventRulesList } from "./EventRulesList";
import { EventTestPanel } from "./EventTestPanel";

interface EventTaskPropertiesProps {
	config: YamlInfrastructureConfig;
	onConfigChange: (config: Partial<YamlInfrastructureConfig>) => void;
	accountInfo?: AccountInfo;
	node: ComponentNode;
}

type TabType = "rules" | "test" | "container";

export function EventTaskProperties({
	config,
	onConfigChange,
	accountInfo,
	node,
}: EventTaskPropertiesProps) {
	const [activeTab, setActiveTab] = useState<TabType>("rules");

	// Extract task name from node id (e.g., "event-order-processor" -> "order-processor")
	const taskName = node.id.replace("event-", "");

	// Find the event task configuration
	const eventTask = config.event_processor_tasks?.find(
		(task) => task.name === taskName,
	);

	// Local state for editing
	const [dockerImage, setDockerImage] = useState(() => eventTask?.docker_image || "");
	const [ecrConfig, setEcrConfig] = useState<ECRConfig | undefined>(() =>
		eventTask?.ecr_config || { mode: "create_ecr" }
	);
	const [cpu, setCpu] = useState(() => eventTask?.cpu?.toString() || "256");
	const [memory, setMemory] = useState(() => eventTask?.memory?.toString() || "512");

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
			return [{
				name: eventTask.rule_name,
				sources: eventTask.sources,
				detail_types: eventTask.detail_types,
			}];
		}
		return [];
	}, [eventTask]);

	const updateTaskConfig = useCallback((updates: Partial<typeof eventTask>) => {
		if (!config.event_processor_tasks) return;

		const updatedTasks = config.event_processor_tasks.map((task) =>
			task.name === taskName ? { ...task, ...updates } : task,
		);

		onConfigChange({ event_processor_tasks: updatedTasks });
	}, [config.event_processor_tasks, taskName, onConfigChange]);

	const handleRulesChange = useCallback((newRules: EventBridgeRule[]) => {
		// When using rules[], clear legacy fields
		updateTaskConfig({
			rules: newRules,
			rule_name: undefined,
			sources: undefined,
			detail_types: undefined,
		});
	}, [updateTaskConfig]);

	const handleEcrConfigChange = useCallback((newConfig: ECRConfig | undefined) => {
		setEcrConfig(newConfig);
		updateTaskConfig({ ecr_config: newConfig });
	}, [updateTaskConfig]);

	const tabs = [
		{ id: "rules" as const, label: "Rules", icon: Zap },
		{ id: "test" as const, label: "Test", icon: Send },
		{ id: "container" as const, label: "Container", icon: Container },
	];

	return (
		<div className="space-y-4">
			{/* Tab Navigation */}
			<div className="flex border-b border-gray-700">
				{tabs.map((tab) => {
					const Icon = tab.icon;
					return (
						<button
							key={tab.id}
							onClick={() => setActiveTab(tab.id)}
							className={`flex items-center gap-2 px-4 py-2 text-sm font-medium border-b-2 -mb-px transition-colors ${
								activeTab === tab.id
									? "border-blue-500 text-blue-400"
									: "border-transparent text-gray-400 hover:text-gray-300"
							}`}
						>
							<Icon className="w-4 h-4" />
							{tab.label}
						</button>
					);
				})}
			</div>

			{/* Tab Content */}
			{activeTab === "rules" && eventTask && (
				<div className="space-y-4">
					<EventRulesList
						rules={getRules()}
						onRulesChange={handleRulesChange}
					/>

					{/* Resource Information */}
					<Card>
						<CardHeader>
							<CardTitle className="flex items-center gap-2 text-sm">
								<Cloud className="w-4 h-4" />
								Resources
							</CardTitle>
						</CardHeader>
						<CardContent>
							<div className="grid grid-cols-2 gap-2 text-xs">
								<div className="flex justify-between p-2 bg-gray-800 rounded">
									<span className="text-gray-500">ECR</span>
									<code className="text-gray-300">{taskEcrRepoName}</code>
								</div>
								<div className="flex justify-between p-2 bg-gray-800 rounded">
									<span className="text-gray-500">Logs</span>
									<code className="text-gray-300">{config.project}_task_{taskName}_{config.env}</code>
								</div>
							</div>
						</CardContent>
					</Card>
				</div>
			)}

			{activeTab === "test" && eventTask && (
				<EventTestPanel eventTask={eventTask} />
			)}

			{activeTab === "container" && eventTask && (
				<div className="space-y-4">
					{/* ECR Configuration */}
					<Card>
						<CardHeader>
							<CardTitle className="flex items-center gap-2">
								<Container className="w-5 h-5" />
								Container Image
							</CardTitle>
							<CardDescription>
								Configure the Docker image for this task
							</CardDescription>
						</CardHeader>
						<CardContent className="space-y-4">
							<ECRConfigEditor
								config={config}
								currentServiceName={taskName}
								currentServiceType="event_processor_tasks"
								ecrConfig={ecrConfig}
								onEcrConfigChange={handleEcrConfigChange}
								accountInfo={accountInfo}
							/>

							<div className="space-y-2">
								<div className="flex items-center justify-between">
									<Label htmlFor="docker-image" className="text-sm">
										Image Tag
									</Label>
									<span className="text-xs text-gray-500">
										e.g., latest, v1.2.3
									</span>
								</div>
								<Input
									id="docker-image"
									value={dockerImage}
									onChange={(e) => {
										setDockerImage(e.target.value);
										updateTaskConfig({ docker_image: e.target.value });
									}}
									placeholder="latest"
									className="font-mono text-sm"
								/>
								{ecrConfig && (
									<div className="p-2 bg-blue-950/30 border border-blue-900/50 rounded text-xs">
										<span className="text-gray-500">Full image: </span>
										<code className="text-blue-400">
											{taskEcrRepoUri}:{dockerImage || "latest"}
										</code>
									</div>
								)}
							</div>
						</CardContent>
					</Card>

					{/* Resource Configuration */}
					<Card>
						<CardHeader>
							<CardTitle className="flex items-center gap-2">
								<Settings className="w-5 h-5" />
								Task Resources
							</CardTitle>
						</CardHeader>
						<CardContent className="space-y-4">
							<div className="grid grid-cols-2 gap-4">
								<div className="space-y-2">
									<Label htmlFor="cpu">CPU (units)</Label>
									<Input
										id="cpu"
										value={cpu}
										onChange={(e) => {
											setCpu(e.target.value);
											updateTaskConfig({ cpu: parseInt(e.target.value) || 256 });
										}}
										placeholder="256"
										className="font-mono"
									/>
									<p className="text-xs text-gray-500">256 = 0.25 vCPU</p>
								</div>
								<div className="space-y-2">
									<Label htmlFor="memory">Memory (MB)</Label>
									<Input
										id="memory"
										value={memory}
										onChange={(e) => {
											setMemory(e.target.value);
											updateTaskConfig({ memory: parseInt(e.target.value) || 512 });
										}}
										placeholder="512"
										className="font-mono"
									/>
								</div>
							</div>
						</CardContent>
					</Card>

					{/* Environment Variables */}
					<Card>
						<CardHeader>
							<CardTitle className="flex items-center gap-2">
								<Settings className="w-5 h-5" />
								Environment Variables
							</CardTitle>
							<CardDescription>
								Managed via AWS Systems Manager Parameter Store
							</CardDescription>
						</CardHeader>
						<CardContent>
							<div className="p-3 bg-gray-800 rounded-lg space-y-2">
								<code className="text-xs text-blue-400">
									/{config.env}/{config.project}/task/{taskName}/
								</code>
								<Button
									size="sm"
									variant="outline"
									onClick={() => {
										const region = config.region;
										const path = `/${config.env}/${config.project}/task/${taskName}/`;
										window.open(
											`https://${region}.console.aws.amazon.com/systems-manager/parameters?region=${region}&tab=Table&path=${encodeURIComponent(path)}`,
											"_blank",
										);
									}}
								>
									<Link className="w-3 h-3 mr-1" />
									Open in AWS Console
								</Button>
							</div>
						</CardContent>
					</Card>
				</div>
			)}
		</div>
	);
}
