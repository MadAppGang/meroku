import {
	AlertCircle,
	Calendar,
	Cloud,
	Container,
	Info,
	Link,
	Plus,
	Settings,
	X,
	Zap,
} from "lucide-react";
import { useState, useCallback } from "react";
import type { AccountInfo } from "../api/infrastructure";
import type { ComponentNode } from "../types";
import type { ECRConfig, YamlInfrastructureConfig } from "../types/yamlConfig";
import { Alert, AlertDescription } from "./ui/alert";
import { Badge } from "./ui/badge";
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

interface EventTaskPropertiesProps {
	config: YamlInfrastructureConfig;
	onConfigChange: (config: Partial<YamlInfrastructureConfig>) => void;
	accountInfo?: AccountInfo;
	node: ComponentNode;
}

export function EventTaskProperties({
	config,
	onConfigChange,
	accountInfo,
	node,
}: EventTaskPropertiesProps) {
	// Extract task name from node id (e.g., "event-order-processor" -> "order-processor")
	const taskName = node.id.replace("event-", "");

	console.log('[EventTaskProperties] RENDER', {
		taskName,
		timestamp: Date.now()
	});

	// Find the event task configuration
	const eventTask = config.event_processor_tasks?.find(
		(task) => task.name === taskName,
	);

	// Local state for editing - initialize from current task
	const [ruleName, setRuleName] = useState(() => eventTask?.rule_name || "");
	const [eventSources, setEventSources] = useState<string[]>(() =>
		eventTask?.sources || []
	);
	const [detailTypes, setDetailTypes] = useState<string[]>(() =>
		eventTask?.detail_types || []
	);
	const [dockerImage, setDockerImage] = useState(() => eventTask?.docker_image || "");
	const [ecrConfig, setEcrConfig] = useState<ECRConfig | undefined>(() =>
		eventTask?.ecr_config || { mode: "create_ecr" }
	);

	// State for input fields
	const [newSource, setNewSource] = useState("");
	const [newDetailType, setNewDetailType] = useState("");

	// ECR repository info for image preview
	const taskEcrRepoName = `${config.project}_task_${taskName}`;
	const taskEcrRepoUri = `${accountInfo?.accountId || config.ecr_account_id || "<ACCOUNT_ID>"}.dkr.ecr.${config.region}.amazonaws.com/${taskEcrRepoName}`;

	const handleAddSource = () => {
		if (newSource && !eventSources.includes(newSource)) {
			const updatedSources = [...eventSources, newSource];
			setEventSources(updatedSources);
			setNewSource("");
			updateEventPattern(updatedSources, detailTypes);
		}
	};

	const handleRemoveSource = (source: string) => {
		const updatedSources = eventSources.filter((s) => s !== source);
		setEventSources(updatedSources);
		updateEventPattern(updatedSources, detailTypes);
	};

	const handleAddDetailType = () => {
		if (newDetailType && !detailTypes.includes(newDetailType)) {
			const updatedTypes = [...detailTypes, newDetailType];
			setDetailTypes(updatedTypes);
			setNewDetailType("");
			updateEventPattern(eventSources, updatedTypes);
		}
	};

	const handleRemoveDetailType = (type: string) => {
		const updatedTypes = detailTypes.filter((t) => t !== type);
		setDetailTypes(updatedTypes);
		updateEventPattern(eventSources, updatedTypes);
	};

	const updateEventPattern = useCallback((sources: string[], types: string[]) => {
		if (!config.event_processor_tasks) return;

		const updatedTasks = config.event_processor_tasks.map((task) =>
			task.name === taskName
				? {
						...task,
						sources: sources,
						detail_types: types,
					}
				: task,
		);

		onConfigChange({ event_processor_tasks: updatedTasks });
	}, [config.event_processor_tasks, taskName, onConfigChange]);

	const updateTaskConfig = useCallback((updates: Partial<typeof eventTask>) => {
		if (!config.event_processor_tasks) return;

		const updatedTasks = config.event_processor_tasks.map((task) =>
			task.name === taskName ? { ...task, ...updates } : task,
		);

		onConfigChange({ event_processor_tasks: updatedTasks });
	}, [config.event_processor_tasks, taskName, onConfigChange, eventTask]);

	const handleEcrConfigChange = useCallback((newConfig: ECRConfig | undefined) => {
		console.log('[EventTaskProperties] handleEcrConfigChange', {
			taskName,
			oldMode: ecrConfig?.mode,
			newMode: newConfig?.mode,
			timestamp: Date.now()
		});
		setEcrConfig(newConfig);
		updateTaskConfig({ ecr_config: newConfig });
	}, [taskName, ecrConfig?.mode, updateTaskConfig]);

	// Common event examples
	const commonEventSources = [
		{ value: "aws.s3", description: "S3 bucket events" },
		{ value: "aws.sns", description: "SNS topic messages" },
		{ value: "aws.apigateway", description: "API Gateway requests" },
		{ value: "aws.dynamodb", description: "DynamoDB stream events" },
		{ value: "aws.kinesis", description: "Kinesis stream events" },
		{ value: "custom.app", description: "Your application events" },
	];

	const commonDetailTypes = [
		{ value: "Object Created", description: "S3 object uploads" },
		{ value: "Object Deleted", description: "S3 object deletions" },
		{ value: "API Request", description: "API Gateway calls" },
		{ value: "Record Insert", description: "Database insertions" },
		{ value: "Record Update", description: "Database updates" },
		{ value: "Record Delete", description: "Database deletions" },
	];

	return (
		<div className="space-y-6">
			{/* Event Rule Configuration */}
			<Card>
				<CardHeader>
					<CardTitle className="flex items-center gap-2">
						<Calendar className="w-5 h-5" />
						Event Rule Configuration
					</CardTitle>
					<CardDescription>
						Configure the EventBridge rule that triggers this task
					</CardDescription>
				</CardHeader>
				<CardContent className="space-y-4">
					<div className="space-y-2">
						<Label htmlFor="rule-name">Rule Name</Label>
						<Input
							id="rule-name"
							value={ruleName}
							onChange={(e) => {
								setRuleName(e.target.value);
								updateTaskConfig({ rule_name: e.target.value });
							}}
							placeholder="order-processing-rule"
							className="font-mono"
						/>
						<p className="text-xs text-gray-500">
							EventBridge rule name for processing specific events
						</p>
					</div>
				</CardContent>
			</Card>

			{/* Event Pattern Settings */}
			<Card>
				<CardHeader>
					<CardTitle className="flex items-center gap-2">
						<Zap className="w-5 h-5" />
						Event Pattern Settings
					</CardTitle>
					<CardDescription>
						Define which events will trigger this task
					</CardDescription>
				</CardHeader>
				<CardContent className="space-y-4">
					{/* Event Sources */}
					<div className="space-y-2">
						<Label>Event Sources</Label>
						<div className="flex items-center gap-2">
							<Input
								value={newSource}
								onChange={(e) => setNewSource(e.target.value)}
								placeholder="e.g., order.service"
								onKeyPress={(e) => e.key === "Enter" && handleAddSource()}
							/>
							<Button size="sm" onClick={handleAddSource} disabled={!newSource}>
								<Plus className="w-4 h-4" />
							</Button>
						</div>
						<div className="flex flex-wrap gap-2 mt-2">
							{eventSources.map((source, index) => (
								<Badge
									key={index}
									variant="secondary"
									className="flex items-center gap-1 pr-1"
								>
									<span>{source}</span>
									<button
										type="button"
										onClick={(e) => {
											e.preventDefault();
											e.stopPropagation();
											handleRemoveSource(source);
										}}
										className="ml-1 rounded-sm hover:bg-gray-600 focus:outline-none focus:ring-2 focus:ring-gray-400 focus:ring-offset-0"
									>
										<X className="w-3 h-3" />
									</button>
								</Badge>
							))}
						</div>
						<details className="mt-2">
							<summary className="text-xs text-gray-400 cursor-pointer">
								Common sources
							</summary>
							<div className="mt-2 space-y-1">
								{commonEventSources.map((example) => (
									<div
										key={example.value}
										className="text-xs text-gray-500 hover:text-gray-300 cursor-pointer"
										onClick={() => setNewSource(example.value)}
									>
										<code className="text-blue-400">{example.value}</code> -{" "}
										{example.description}
									</div>
								))}
							</div>
						</details>
					</div>

					{/* Event Detail Types */}
					<div className="space-y-2">
						<Label>Event Detail Types</Label>
						<div className="flex items-center gap-2">
							<Input
								value={newDetailType}
								onChange={(e) => setNewDetailType(e.target.value)}
								placeholder="e.g., Order Created"
								onKeyPress={(e) => e.key === "Enter" && handleAddDetailType()}
							/>
							<Button
								size="sm"
								onClick={handleAddDetailType}
								disabled={!newDetailType}
							>
								<Plus className="w-4 h-4" />
							</Button>
						</div>
						<div className="flex flex-wrap gap-2 mt-2">
							{detailTypes.map((type, index) => (
								<Badge
									key={index}
									variant="secondary"
									className="flex items-center gap-1 pr-1"
								>
									<span>{type}</span>
									<button
										type="button"
										onClick={(e) => {
											e.preventDefault();
											e.stopPropagation();
											handleRemoveDetailType(type);
										}}
										className="ml-1 rounded-sm hover:bg-gray-600 focus:outline-none focus:ring-2 focus:ring-gray-400 focus:ring-offset-0"
									>
										<X className="w-3 h-3" />
									</button>
								</Badge>
							))}
						</div>
						<details className="mt-2">
							<summary className="text-xs text-gray-400 cursor-pointer">
								Common detail types
							</summary>
							<div className="mt-2 space-y-1">
								{commonDetailTypes.map((example) => (
									<div
										key={example.value}
										className="text-xs text-gray-500 hover:text-gray-300 cursor-pointer"
										onClick={() => setNewDetailType(example.value)}
									>
										<code className="text-blue-400">{example.value}</code> -{" "}
										{example.description}
									</div>
								))}
							</div>
						</details>
					</div>
				</CardContent>
			</Card>

			{/* Container Settings */}
			<Card>
				<CardHeader>
					<CardTitle className="flex items-center gap-2">
						<Container className="w-5 h-5" />
						Container Settings
					</CardTitle>
					<CardDescription>
						Docker image and network configuration
					</CardDescription>
				</CardHeader>
				<CardContent className="space-y-4">
					{/* ECR Configuration Display & Editor */}
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
							<Label htmlFor="docker-image" className="text-sm font-medium">
								Image Tag {eventTask?.ecr_config ? "(Optional)" : ""}
							</Label>
							<span className="text-xs text-gray-500">
								{eventTask?.ecr_config ? "e.g., latest, v1.2.3, nginx.1" : "Full image or tag"}
							</span>
						</div>
						<Input
							id="docker-image"
							value={dockerImage}
							onChange={(e) => {
								setDockerImage(e.target.value);
								updateTaskConfig({ docker_image: e.target.value });
							}}
							placeholder={eventTask?.ecr_config ? "latest" : "nginx:latest or just latest"}
							className="font-mono text-sm"
						/>
						{eventTask?.ecr_config && (
							<div className="mt-2 p-2 bg-blue-950/30 border border-blue-900/50 rounded">
								<p className="text-xs text-gray-400">
									<span className="text-gray-500">Full image:</span>{" "}
									<code className="text-blue-400">
										{taskEcrRepoUri}:{dockerImage || "latest"}
									</code>
								</p>
							</div>
						)}
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
						<p className="text-sm text-gray-300">
							Environment variables are managed in Parameter Store
						</p>
						<div className="flex items-center gap-2">
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
					</div>
				</CardContent>
			</Card>

			{/* Resource Information */}
			<Card>
				<CardHeader>
					<CardTitle className="flex items-center gap-2">
						<Cloud className="w-5 h-5" />
						Resource Information
					</CardTitle>
					<CardDescription>
						AWS resources created for this event task
					</CardDescription>
				</CardHeader>
				<CardContent>
					<div className="space-y-3">
						<div className="grid grid-cols-1 gap-3">
							<div className="flex items-center justify-between p-2 bg-gray-800 rounded">
								<span className="text-sm text-gray-400">ECR Repository</span>
								<code className="text-sm text-gray-300">{taskEcrRepoName}</code>
							</div>
							<div className="flex items-center justify-between p-2 bg-gray-800 rounded">
								<span className="text-sm text-gray-400">Log Group</span>
								<code className="text-sm text-gray-300">
									{config.project}_task_{taskName}_{config.env}
								</code>
							</div>
							<div className="flex items-center justify-between p-2 bg-gray-800 rounded">
								<span className="text-sm text-gray-400">Task Definition</span>
								<code className="text-sm text-gray-300">
									{config.project}_task_{taskName}_{config.env}
								</code>
							</div>
							<div className="flex items-center justify-between p-2 bg-gray-800 rounded">
								<span className="text-sm text-gray-400">Memory</span>
								<span className="text-sm text-gray-300">512 MB</span>
							</div>
							<div className="flex items-center justify-between p-2 bg-gray-800 rounded">
								<span className="text-sm text-gray-400">CPU</span>
								<span className="text-sm text-gray-300">
									256 units (0.25 vCPU)
								</span>
							</div>
						</div>
					</div>
				</CardContent>
			</Card>

			{/* Event Pattern Preview */}
			<Card>
				<CardHeader>
					<CardTitle className="flex items-center gap-2">
						<Info className="w-5 h-5" />
						Event Pattern Preview
					</CardTitle>
					<CardDescription>
						The EventBridge rule pattern that will be created
					</CardDescription>
				</CardHeader>
				<CardContent>
					<div className="bg-gray-900 rounded-lg p-3 font-mono text-xs text-gray-300 overflow-x-auto">
						<pre>
							{JSON.stringify(
								{
									source:
										eventSources.length > 0 ? eventSources : ["<add sources>"],
									"detail-type":
										detailTypes.length > 0
											? detailTypes
											: ["<add detail types>"],
								},
								null,
								2,
							)}
						</pre>
					</div>
					{(eventSources.length === 0 || detailTypes.length === 0) && (
						<Alert className="mt-3">
							<AlertCircle className="h-4 w-4" />
							<AlertDescription>
								Add at least one event source and detail type to create a valid
								event pattern
							</AlertDescription>
						</Alert>
					)}
				</CardContent>
			</Card>
		</div>
	);
}
