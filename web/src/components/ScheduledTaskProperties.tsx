import { useState, useCallback } from "react";
import type { AccountInfo } from "../api/infrastructure";
import type { ComponentNode } from "../types";
import type { ECRConfig, YamlInfrastructureConfig } from "../types/yamlConfig";
import { ScheduleExpressionBuilder } from "./ScheduleExpressionBuilder";
import {
	Card,
	CardContent,
	CardDescription,
	CardHeader,
	CardTitle,
} from "./ui/card";
import { Input } from "./ui/input";
import { Label } from "./ui/label";
import { Separator } from "./ui/separator";
import { ECRConfigEditor } from "./ECRConfigEditor";

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
	// Extract task name from node id (e.g., "scheduled-daily-report" -> "daily-report")
	const taskName = node.id.replace("scheduled-", "");

	// Find the current task in config
	const currentTask = config.scheduled_tasks?.find(
		(task) => task.name === taskName,
	);

	// Local state for ECR configuration - use current task's config or default
	const [ecrConfig, setEcrConfig] = useState<ECRConfig | undefined>(() =>
		currentTask?.ecr_config || { mode: "create_ecr" }
	);

	const handleTaskChange = useCallback((updates: Partial<typeof currentTask>) => {
		// Get the latest config and tasks from the closure
		const tasks = config.scheduled_tasks || [];
		const existingTask = tasks.find(t => t.name === taskName);

		if (!existingTask) {
			// If task doesn't exist, create it
			const newTask = {
				name: taskName,
				schedule: "rate(1 day)",
				...updates,
			};
			onConfigChange({
				scheduled_tasks: [...tasks, newTask],
			});
		} else {
			// Update existing task
			const updatedTasks = tasks.map((task) =>
				task.name === taskName ? { ...task, ...updates } : task,
			);

			onConfigChange({
				scheduled_tasks: updatedTasks,
			});
		}
	}, [taskName, config, onConfigChange]);

	const handleEcrConfigChange = useCallback((newConfig: ECRConfig | undefined) => {
		setEcrConfig(newConfig);
		handleTaskChange({ ecr_config: newConfig });
	}, [handleTaskChange]);

	// Use currentTask if it exists, otherwise use defaults
	const task = currentTask || {
		name: taskName,
		schedule: "rate(1 day)",
		docker_image: "",
		container_command: "",
	};

	return (
		<Card className="w-full">
			<CardHeader>
				<CardTitle>Scheduled Task Configuration</CardTitle>
				<CardDescription>
					Configure settings for scheduled task: {taskName}
				</CardDescription>
			</CardHeader>
			<CardContent className="space-y-4">
				<div className="space-y-2">
					<Label>Schedule Expression</Label>
					<ScheduleExpressionBuilder
						value={task.schedule || "rate(1 day)"}
						onChange={(schedule) => handleTaskChange({ schedule })}
					/>
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
					<Label htmlFor="container_command">Container Command Override</Label>
					<Input
						id="container_command"
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
	);
}
