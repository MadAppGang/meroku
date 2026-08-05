import { AlertTriangle } from "lucide-react";
import { useCallback, useEffect, useId, useRef } from "react";
import type { AccountInfo } from "../api/infrastructure";
import { useFargateOptions } from "../hooks/use-fargate-options";
import { useDeepMemo } from "../hooks/useDeepMemo";
import type { ComponentNode } from "../types";
import type { ECRConfig, YamlInfrastructureConfig } from "../types/yamlConfig";
import { ECRConfigEditor } from "./ECRConfigEditor";
import { ScheduledTaskEnvironmentVariables } from "./ScheduledTaskEnvironmentVariables";
import { ScheduleExpressionBuilder } from "./ScheduleExpressionBuilder";
import { Alert, AlertDescription } from "./ui/alert";
import {
	Card,
	CardContent,
	CardDescription,
	CardHeader,
	CardTitle,
} from "./ui/card";
import { Input } from "./ui/input";
import { Label } from "./ui/label";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "./ui/select";
import { Separator } from "./ui/separator";
import { Switch } from "./ui/switch";

interface ScheduledTaskPropertiesProps {
	config: YamlInfrastructureConfig;
	onConfigChange: (config: Partial<YamlInfrastructureConfig>) => void;
	accountInfo?: AccountInfo;
	node: ComponentNode;
}

export function ScheduledTaskProperties({
	config,
	onConfigChange,
	accountInfo,
	node,
}: ScheduledTaskPropertiesProps) {
	const uid = useId();
	// Extract task name from node id (e.g., "scheduled-daily-report" -> "daily-report")
	const taskName = node.id.replace("scheduled-", "");

	// DEBUG: Render counter and mount tracking
	const renderCountRef = useRef(0);
	const isMountedRef = useRef(false);
	renderCountRef.current++;

	useEffect(() => {
		if (!isMountedRef.current) {
			console.log(`🟢 [ScheduledTaskProperties] MOUNTED for task: ${taskName}`);
			isMountedRef.current = true;
		}
		return () => {
			console.log(
				`🔴 [ScheduledTaskProperties] UNMOUNTED for task: ${taskName}`,
			);
		};
	}, [taskName]);

	// Track previous props to detect what changed
	const prevPropsRef = useRef({ config, accountInfo, node });
	useEffect(() => {
		const prev = prevPropsRef.current;
		const changes: string[] = [];
		if (prev.config !== config) changes.push("config");
		if (prev.accountInfo !== accountInfo) changes.push("accountInfo");
		if (prev.node !== node) changes.push("node");

		if (changes.length > 0) {
			console.log(
				`🔧 [ScheduledTaskProperties] Props changed: ${changes.join(", ")}`,
				{
					renderCount: renderCountRef.current,
					scheduled_tasks_changed:
						prev.config.scheduled_tasks !== config.scheduled_tasks,
				},
			);
		}
		prevPropsRef.current = { config, accountInfo, node };
	}, [config, accountInfo, node]);

	console.log(
		`🔄 [ScheduledTaskProperties] Render #${renderCountRef.current} for task: ${taskName} [${isMountedRef.current ? "MOUNTED" : "MOUNTING"}]`,
	);

	if (renderCountRef.current > 50) {
		console.error(
			"⚠️ [ScheduledTaskProperties] INFINITE LOOP DETECTED - More than 50 renders!",
		);
		console.trace("Stack trace at 50th render");
	}

	// Use ref to always access the latest config without causing re-renders
	const configRef = useRef(config);
	useEffect(() => {
		console.log(`📝 [ScheduledTaskProperties] Config updated for ${taskName}`, {
			scheduled_tasks_count: config.scheduled_tasks?.length,
			current_task_exists: !!config.scheduled_tasks?.find(
				(t) => t.name === taskName,
			),
		});
		configRef.current = config;
	}, [config, taskName]);

	// **KEY FIX**: Use DEEP comparison ONLY for currentTask
	// This is the single critical piece that prevents infinite re-renders
	// When config.scheduled_tasks is recreated with same content, currentTask stays stable
	const currentTask = useDeepMemo(() => {
		const task = config.scheduled_tasks?.find((t) => t.name === taskName);
		console.log(
			`📋 [ScheduledTaskProperties] currentTask recalculated (DEEP COMPARE) for ${taskName}:`,
			task,
		);
		return task;
	}, [config.scheduled_tasks, taskName]);

	// Derive ecrConfig directly from stable currentTask - no additional memoization needed
	const ecrConfig = currentTask?.ecr_config || { mode: "create_ecr" as const };
	console.log(
		`🐳 [ScheduledTaskProperties] ecrConfig derived from currentTask:`,
		ecrConfig,
	);

	const handleTaskChange = useCallback(
		(updates: Partial<typeof currentTask>) => {
			console.log(
				`🔧 [handleTaskChange] Called for ${taskName} with updates:`,
				updates,
			);

			// Use configRef.current to access latest config without adding to dependencies
			// This prevents infinite re-render loops
			const tasks = configRef.current.scheduled_tasks || [];
			const existingTask = tasks.find((t) => t.name === taskName);

			if (!existingTask) {
				// If task doesn't exist, create it
				const newTask = {
					name: taskName,
					schedule: "rate(1 day)",
					...updates,
				};
				console.log(`➕ [handleTaskChange] Creating new task:`, newTask);
				onConfigChange({
					scheduled_tasks: [...tasks, newTask],
				});
			} else {
				// Update existing task, but bail if nothing actually changes
				const updatedTask = { ...existingTask, ...updates };
				if (JSON.stringify(existingTask) === JSON.stringify(updatedTask)) {
					console.log(
						`⏭️ [handleTaskChange] No changes detected for ${taskName}, skipping update.`,
					);
					return;
				}

				const updatedTasks = tasks.map((task) =>
					task.name === taskName ? updatedTask : task,
				);

				console.log(
					`✏️ [handleTaskChange] Updating existing task:`,
					updatedTask,
				);
				onConfigChange({
					scheduled_tasks: updatedTasks,
				});
			}
		},
		[taskName, onConfigChange],
	);

	const handleEcrConfigChange = useCallback(
		(newConfig: ECRConfig | undefined) => {
			console.log(
				`🐳 [handleEcrConfigChange] Called for ${taskName} with newConfig:`,
				newConfig,
				{
					renderCount: renderCountRef.current,
					isMounted: isMountedRef.current,
				},
			);
			handleTaskChange({ ecr_config: newConfig });
		},
		[handleTaskChange, taskName],
	);

	const {
		options: fargateOptions,
		getMemoryOptions,
		formatMemory,
	} = useFargateOptions();
	const memoryOptions = getMemoryOptions(currentTask?.cpu || 256);

	// Use currentTask if it exists, otherwise use defaults
	const task = currentTask || {
		name: taskName,
		schedule: "rate(1 day)",
		docker_image: "",
		container_command: "",
	};

	return (
		<>
			<Card className="w-full">
				<CardHeader>
					<CardTitle>Scheduled Task Configuration</CardTitle>
					<CardDescription>
						Configure settings for scheduled task: {taskName}
					</CardDescription>
				</CardHeader>
				<CardContent className="space-y-4">
					{/* Enabled Toggle - at the very top */}
					<div className="flex items-center justify-between">
						<div className="flex-1">
							<Label htmlFor={`${uid}-enabled`}>Enabled</Label>
							<p className="text-xs text-gray-500 mt-1">
								When disabled, all settings are kept but the task is not
								deployed
							</p>
						</div>
						<Switch
							id={`${uid}-enabled`}
							checked={task.enabled !== false}
							onCheckedChange={(checked) =>
								handleTaskChange({ enabled: checked })
							}
							className="data-[state=checked]:bg-green-500 data-[state=unchecked]:bg-gray-600"
						/>
					</div>

					{task.enabled === false && (
						<Alert className="border-yellow-600 bg-yellow-900/20">
							<AlertTriangle className="h-4 w-4 text-yellow-400" />
							<AlertDescription className="text-xs text-gray-300">
								This scheduled task is <strong>disabled</strong>. It will not be
								included in the next Terraform generation. All configuration is
								preserved and can be re-enabled at any time.
							</AlertDescription>
						</Alert>
					)}

					<Separator />

					{/* Auto-Deploy Toggle */}
					<div className="flex items-center justify-between">
						<div className="flex-1">
							<Label htmlFor={`${uid}-auto-deploy`}>Auto-Deploy</Label>
							<p className="text-xs text-gray-500 mt-1">
								Register a new task definition automatically when a new image is
								pushed to this task's repository
							</p>
						</div>
						<Switch
							id={`${uid}-auto-deploy`}
							checked={task.auto_deploy !== false}
							onCheckedChange={(checked) =>
								handleTaskChange({ auto_deploy: checked })
							}
							className="data-[state=checked]:bg-green-500 data-[state=unchecked]:bg-gray-600"
						/>
					</div>

					{task.auto_deploy === false && (
						<Alert className="border-blue-600 bg-blue-900/20">
							<AlertDescription className="text-xs text-gray-300">
								Automatic deploys are <strong>off</strong> for this task. A push
								is still delivered to the CI/CD Lambda and logged as
								<code className="mx-1">
									auto_deploy is disabled for task:{taskName}
								</code>
								rather than silently doing nothing.
							</AlertDescription>
						</Alert>
					)}

					{task.auto_deploy !== false && config.env !== "dev" && (
						<Alert className="border-yellow-600 bg-yellow-900/20">
							<AlertTriangle className="h-4 w-4 text-yellow-400" />
							<AlertDescription className="text-xs text-gray-300">
								Auto-deploy is on, but{" "}
								<strong>
									no automatic trigger reaches a scheduled task outside{" "}
									<code>dev</code>
								</strong>
								: its ECR repository is only created in the dev environment, and
								an SSM change never redeploys a task. In{" "}
								<code>{config.env}</code> this setting enables the manual deploy
								path only.
							</AlertDescription>
						</Alert>
					)}

					<Separator />

					<div className="space-y-2">
						<Label>Schedule Expression</Label>
						<ScheduleExpressionBuilder
							value={task.schedule || "rate(1 day)"}
							onChange={(schedule) => handleTaskChange({ schedule })}
						/>
					</div>

					<Separator />

					{/* CPU and Memory Configuration */}
					<div className="grid grid-cols-2 gap-4">
						<div className="space-y-2">
							<Label>CPU (units)</Label>
							<Select
								value={(task.cpu || 256).toString()}
								onValueChange={(value: string) => {
									const newCpu = Number.parseInt(value, 10);
									const option = fargateOptions.find((o) => o.cpu === newCpu);
									const validMemory = option?.memoryOptions || [];
									const currentMemory = task.memory || 512;
									const newMemory = validMemory.includes(currentMemory)
										? currentMemory
										: validMemory[0] || 512;
									handleTaskChange({ cpu: newCpu, memory: newMemory });
								}}
							>
								<SelectTrigger>
									<SelectValue />
								</SelectTrigger>
								<SelectContent>
									{fargateOptions.map((option) => (
										<SelectItem key={option.cpu} value={option.cpu.toString()}>
											{option.cpu} ({option.vcpu} vCPU)
										</SelectItem>
									))}
								</SelectContent>
							</Select>
						</div>

						<div className="space-y-2">
							<Label>Memory (MB)</Label>
							<Select
								value={(task.memory || 512).toString()}
								onValueChange={(value: string) =>
									handleTaskChange({ memory: Number.parseInt(value, 10) })
								}
							>
								<SelectTrigger>
									<SelectValue />
								</SelectTrigger>
								<SelectContent>
									{memoryOptions.map((mem) => (
										<SelectItem key={mem} value={mem.toString()}>
											{formatMemory(mem)}
										</SelectItem>
									))}
								</SelectContent>
							</Select>
						</div>
					</div>

					<Separator />

					{/* ECR Configuration Display & Editor */}
					<ECRConfigEditor
						config={config}
						currentServiceName={taskName}
						currentServiceType="scheduled_tasks"
						ecrConfig={ecrConfig}
						onEcrConfigChange={handleEcrConfigChange}
						accountInfo={accountInfo}
					/>

					<Separator />

					<div className="space-y-2">
						<Label htmlFor={`${uid}-container_command`}>
							Container Command Override
						</Label>
						<Input
							id={`${uid}-container_command`}
							value={task.container_command || ""}
							onChange={(e) =>
								handleTaskChange({ container_command: e.target.value })
							}
							placeholder='["npm", "run", "report"]'
							className="bg-gray-800 border-gray-600 text-white font-mono"
						/>
						<p className="text-xs text-gray-500">
							Override container startup command (JSON array as string)
						</p>
					</div>
				</CardContent>
			</Card>

			{/* Environment Variables Editor */}
			<ScheduledTaskEnvironmentVariables
				config={config}
				node={node}
				onConfigChange={onConfigChange}
			/>
		</>
	);
}
