import { useCallback, useRef, useEffect } from "react";
import type { AccountInfo } from "../api/infrastructure";
import type { ComponentNode } from "../types";
import type { ECRConfig, YamlInfrastructureConfig } from "../types/yamlConfig";
import { ScheduleExpressionBuilder } from "./ScheduleExpressionBuilder";
import { useDeepMemo } from "../hooks/useDeepMemo";
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
			console.log(`🔴 [ScheduledTaskProperties] UNMOUNTED for task: ${taskName}`);
		};
	}, [taskName]);

	// Track previous props to detect what changed
	const prevPropsRef = useRef({ config, accountInfo, node });
	useEffect(() => {
		const prev = prevPropsRef.current;
		const changes: string[] = [];
		if (prev.config !== config) changes.push('config');
		if (prev.accountInfo !== accountInfo) changes.push('accountInfo');
		if (prev.node !== node) changes.push('node');

		if (changes.length > 0) {
			console.log(`🔧 [ScheduledTaskProperties] Props changed: ${changes.join(', ')}`, {
				renderCount: renderCountRef.current,
				scheduled_tasks_changed: prev.config.scheduled_tasks !== config.scheduled_tasks,
			});
		}
		prevPropsRef.current = { config, accountInfo, node };
	}, [config, accountInfo, node]);

	console.log(`🔄 [ScheduledTaskProperties] Render #${renderCountRef.current} for task: ${taskName} [${isMountedRef.current ? 'MOUNTED' : 'MOUNTING'}]`);

	if (renderCountRef.current > 50) {
		console.error('⚠️ [ScheduledTaskProperties] INFINITE LOOP DETECTED - More than 50 renders!');
		console.trace('Stack trace at 50th render');
	}

	// Use ref to always access the latest config without causing re-renders
	const configRef = useRef(config);
	useEffect(() => {
		console.log(`📝 [ScheduledTaskProperties] Config updated for ${taskName}`, {
			scheduled_tasks_count: config.scheduled_tasks?.length,
			current_task_exists: !!config.scheduled_tasks?.find(t => t.name === taskName)
		});
		configRef.current = config;
	}, [config, taskName]);

	// **KEY FIX**: Use DEEP comparison ONLY for currentTask
	// This is the single critical piece that prevents infinite re-renders
	// When config.scheduled_tasks is recreated with same content, currentTask stays stable
	const currentTask = useDeepMemo(() => {
		const task = config.scheduled_tasks?.find(
			(t) => t.name === taskName,
		);
		console.log(`📋 [ScheduledTaskProperties] currentTask recalculated (DEEP COMPARE) for ${taskName}:`, task);
		return task;
	}, [config.scheduled_tasks, taskName]);

	// Derive ecrConfig directly from stable currentTask - no additional memoization needed
	const ecrConfig = currentTask?.ecr_config || { mode: "create_ecr" as const };
	console.log(`🐳 [ScheduledTaskProperties] ecrConfig derived from currentTask:`, ecrConfig);

	const handleTaskChange = useCallback((updates: Partial<typeof currentTask>) => {
		console.log(`🔧 [handleTaskChange] Called for ${taskName} with updates:`, updates);

		// Use configRef.current to access latest config without adding to dependencies
		// This prevents infinite re-render loops
		const tasks = configRef.current.scheduled_tasks || [];
		const existingTask = tasks.find(t => t.name === taskName);

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
				console.log(`⏭️ [handleTaskChange] No changes detected for ${taskName}, skipping update.`);
				return;
			}

			const updatedTasks = tasks.map((task) =>
				task.name === taskName ? updatedTask : task,
			);

			console.log(`✏️ [handleTaskChange] Updating existing task:`, updatedTask);
			onConfigChange({
				scheduled_tasks: updatedTasks,
			});
		}
	}, [taskName, onConfigChange]);

	const handleEcrConfigChange = useCallback((newConfig: ECRConfig | undefined) => {
		console.log(`🐳 [handleEcrConfigChange] Called for ${taskName} with newConfig:`, newConfig, {
			renderCount: renderCountRef.current,
			isMounted: isMountedRef.current,
		});
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
