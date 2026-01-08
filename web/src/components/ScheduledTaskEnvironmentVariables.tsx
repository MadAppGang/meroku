import {
	Check,
	Edit2,
	Info,
	Plus,
	Settings,
	Trash2,
	X,
} from "lucide-react";
import { useState, useEffect } from "react";
import type { ComponentNode } from "../types";
import type { YamlInfrastructureConfig } from "../types/yamlConfig";
import { Alert, AlertDescription } from "./ui/alert";
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

interface ScheduledTaskEnvironmentVariablesProps {
	config: YamlInfrastructureConfig;
	node: ComponentNode;
	onConfigChange?: (config: Partial<YamlInfrastructureConfig>) => void;
}

export function ScheduledTaskEnvironmentVariables({
	config,
	node,
	onConfigChange,
}: ScheduledTaskEnvironmentVariablesProps) {
	// Extract task name from node id
	const taskName = node.id.replace("scheduled-", "");

	// Find the task configuration
	const taskConfig = config.scheduled_tasks?.find(
		(task) => task.name === taskName,
	);

	// State for environment variables
	const [envVars, setEnvVars] = useState<Record<string, string>>(
		taskConfig?.environment_variables || {},
	);
	const [newVarName, setNewVarName] = useState("");
	const [newVarValue, setNewVarValue] = useState("");
	const [editingVar, setEditingVar] = useState<string | null>(null);
	const [editingVarValue, setEditingVarValue] = useState("");

	// Sync state when task changes
	useEffect(() => {
		setEnvVars(taskConfig?.environment_variables || {});
	}, [taskConfig?.environment_variables]);

	const parameterStorePath = `/${config.env}/${config.project}/task/${taskName}`;

	// Handler to update config
	const updateTaskConfig = (updatedVars: Record<string, string>) => {
		if (onConfigChange && config.scheduled_tasks) {
			const updatedTasks = config.scheduled_tasks.map((task) =>
				task.name === taskName
					? {
							...task,
							environment_variables:
								Object.keys(updatedVars).length > 0 ? updatedVars : undefined,
						}
					: task,
			);
			onConfigChange({ scheduled_tasks: updatedTasks });
		}
	};

	// Handler functions for environment variables
	const handleAddVar = () => {
		if (newVarName && newVarValue) {
			const updatedVars = { ...envVars, [newVarName]: newVarValue };
			setEnvVars(updatedVars);
			setNewVarName("");
			setNewVarValue("");
			updateTaskConfig(updatedVars);
		}
	};

	const handleDeleteVar = (name: string) => {
		const updatedVars = { ...envVars };
		delete updatedVars[name];
		setEnvVars(updatedVars);
		updateTaskConfig(updatedVars);
	};

	const handleEditVar = (name: string, newValue: string) => {
		const updatedVars = { ...envVars, [name]: newValue };
		setEnvVars(updatedVars);
		setEditingVar(null);
		updateTaskConfig(updatedVars);
	};

	return (
		<Card>
			<CardHeader>
				<CardTitle className="flex items-center gap-2">
					<Settings className="w-4 h-4" />
					Environment Variables
				</CardTitle>
				<CardDescription>
					Configure environment variables for scheduled task: {taskName}
				</CardDescription>
			</CardHeader>
			<CardContent>
				<div className="space-y-3">
					{Object.entries(envVars).length === 0 ? (
						<div className="text-sm text-gray-500 py-2">
							No environment variables defined
						</div>
					) : (
						Object.entries(envVars).map(([name, value]) => (
							<div key={name} className="p-2 bg-gray-800 rounded">
								<div className="flex items-center justify-between">
									<code className="text-xs font-mono text-green-400">
										{name}
									</code>
									<div className="flex items-center gap-1">
										<Button
											size="sm"
											variant="ghost"
											onClick={() => {
												setEditingVar(name);
												setEditingVarValue(value);
											}}
											className="h-5 w-5 p-0"
										>
											<Edit2 className="w-3 h-3" />
										</Button>
										<Button
											size="sm"
											variant="ghost"
											onClick={() => handleDeleteVar(name)}
											className="h-5 w-5 p-0 text-red-400 hover:text-red-300"
										>
											<Trash2 className="w-3 h-3" />
										</Button>
									</div>
								</div>
								{editingVar === name ? (
									<div className="mt-1">
										<Input
											type="text"
											value={editingVarValue}
											onChange={(e) => setEditingVarValue(e.target.value)}
											className="w-full h-6 text-xs"
										/>
										<div className="flex items-center gap-1 mt-1">
											<Button
												size="sm"
												variant="ghost"
												onClick={() => handleEditVar(name, editingVarValue)}
												className="h-5 w-5 p-0"
											>
												<Check className="w-3 h-3" />
											</Button>
											<Button
												size="sm"
												variant="ghost"
												onClick={() => setEditingVar(null)}
												className="h-5 w-5 p-0"
											>
												<X className="w-3 h-3" />
											</Button>
										</div>
									</div>
								) : (
									<div className="text-xs text-gray-300 font-mono mt-1">
										{value}
									</div>
								)}
							</div>
						))
					)}

					{/* Add new variable form */}
					<div className="border-t border-gray-700 pt-3">
						<div className="grid grid-cols-2 gap-2">
							<div>
								<Label htmlFor="new-var-name" className="text-xs text-gray-400">
									Variable Name
								</Label>
								<Input
									id="new-var-name"
									type="text"
									placeholder="VARIABLE_NAME"
									value={newVarName}
									onChange={(e) =>
										setNewVarName(
											e.target.value
												.toUpperCase()
												.replace(/[^A-Z0-9_]/g, "_"),
										)
									}
									className="mt-1"
								/>
							</div>
							<div>
								<Label
									htmlFor="new-var-value"
									className="text-xs text-gray-400"
								>
									Value
								</Label>
								<Input
									id="new-var-value"
									type="text"
									placeholder="value"
									value={newVarValue}
									onChange={(e) => setNewVarValue(e.target.value)}
									className="mt-1"
								/>
							</div>
						</div>
						<Button
							onClick={handleAddVar}
							disabled={!newVarName || !newVarValue}
							className="mt-2 w-full"
							size="sm"
						>
							<Plus className="w-4 h-4 mr-2" />
							Add Variable
						</Button>
					</div>
				</div>

				<Alert className="mt-4">
					<Info className="h-4 w-4" />
					<AlertDescription className="text-xs">
						These variables are defined in the YAML configuration. For secrets,
						use Parameter Store at <code>{parameterStorePath}/*</code>
					</AlertDescription>
				</Alert>
			</CardContent>
		</Card>
	);
}
