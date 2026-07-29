import {
	Check,
	Edit2,
	ExternalLink,
	Info,
	Key,
	Plus,
	Trash2,
	X,
	Zap,
} from "lucide-react";
import { useEffect, useState } from "react";
import type { AccountInfo } from "../api/infrastructure";
import type { ComponentNode } from "../types";
import type { YamlInfrastructureConfig } from "../types/yamlConfig";
import { Alert, AlertDescription } from "./ui/alert";
import { Button } from "./ui/button";
import { Input } from "./ui/input";
import { Label } from "./ui/label";
import { StyledSection } from "./ui/styled-section";

interface EventTaskEnvVarsProps {
	config: YamlInfrastructureConfig;
	node: ComponentNode;
	accountInfo?: AccountInfo;
	onConfigChange?: (config: Partial<YamlInfrastructureConfig>) => void;
}

export function EventTaskEnvVars({
	config,
	node,
	accountInfo,
	onConfigChange,
}: EventTaskEnvVarsProps) {
	const taskName = node.id.replace("event-", "");
	const taskConfig = config.event_processor_tasks?.find(
		(task) => task.name === taskName,
	);
	const [envVars, setEnvVars] = useState<Record<string, string>>(
		taskConfig?.environment_variables || {},
	);
	const [newVarName, setNewVarName] = useState("");
	const [newVarValue, setNewVarValue] = useState("");
	const [editingVar, setEditingVar] = useState<string | null>(null);
	const [editingVarValue, setEditingVarValue] = useState("");

	useEffect(() => {
		setEnvVars(taskConfig?.environment_variables || {});
	}, [taskConfig?.environment_variables]);

	const parameterStorePath = `/${config.env}/${config.project}/task/${taskName}/`;
	const hasSqsEnvVar = config.sqs?.enabled;

	const updateTaskConfig = (updatedVars: Record<string, string>) => {
		if (!onConfigChange || !config.event_processor_tasks) {
			return;
		}

		const updatedTasks = config.event_processor_tasks.map((task) =>
			task.name === taskName
				? {
						...task,
						environment_variables:
							Object.keys(updatedVars).length > 0
								? updatedVars
								: undefined,
					}
				: task,
		);
		onConfigChange({ event_processor_tasks: updatedTasks });
	};

	const handleAddVar = () => {
		if (!newVarName || !newVarValue) {
			return;
		}

		const updatedVars = { ...envVars, [newVarName]: newVarValue };
		setEnvVars(updatedVars);
		setNewVarName("");
		setNewVarValue("");
		updateTaskConfig(updatedVars);
	};

	const handleDeleteVar = (name: string) => {
		const updatedVars = { ...envVars };
		delete updatedVars[name];
		setEnvVars(updatedVars);
		updateTaskConfig(updatedVars);
	};

	const handleEditVar = (name: string) => {
		const updatedVars = { ...envVars, [name]: editingVarValue };
		setEnvVars(updatedVars);
		setEditingVar(null);
		updateTaskConfig(updatedVars);
	};

	return (
		<div className="space-y-4">
			<StyledSection
				title="Declarative Variables"
				description="Non-secret values stored in the environment YAML"
				icon={Info}
				iconColor="text-green-400"
			>
				<div className="space-y-3">
					{Object.keys(envVars).length === 0 ? (
						<p className="text-sm text-gray-500 py-2">
							No declarative environment variables defined
						</p>
					) : (
						Object.entries(envVars).map(([name, value]) => (
							<div key={name} className="p-2 bg-gray-900 rounded">
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
											value={editingVarValue}
											onChange={(event) =>
												setEditingVarValue(event.target.value)
											}
											className="w-full h-7 text-xs"
										/>
										<div className="flex items-center gap-1 mt-1">
											<Button
												size="sm"
												variant="ghost"
												onClick={() => handleEditVar(name)}
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

					<div className="border-t border-gray-700 pt-3">
						<div className="grid grid-cols-2 gap-2">
							<div>
								<Label
									htmlFor="event-new-var-name"
									className="text-xs text-gray-400"
								>
									Variable Name
								</Label>
								<Input
									id="event-new-var-name"
									placeholder="VARIABLE_NAME"
									value={newVarName}
									onChange={(event) =>
										setNewVarName(
											event.target.value
												.toUpperCase()
												.replace(/[^A-Z0-9_]/g, "_"),
										)
									}
									className="mt-1"
								/>
							</div>
							<div>
								<Label
									htmlFor="event-new-var-value"
									className="text-xs text-gray-400"
								>
									Value
								</Label>
								<Input
									id="event-new-var-value"
									placeholder="value"
									value={newVarValue}
									onChange={(event) => setNewVarValue(event.target.value)}
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

					<Alert>
						<Info className="h-4 w-4" />
						<AlertDescription className="text-xs">
							Use Parameter Store below for secrets.
						</AlertDescription>
					</Alert>
				</div>
			</StyledSection>

			{/* Static Environment Variables - only show if SQS enabled */}
			{hasSqsEnvVar && (
				<StyledSection
					title="Injected Variables"
					description="Automatically set by infrastructure"
					icon={Zap}
					iconColor="text-amber-400"
				>
					<div className="p-3 bg-gray-900 rounded-lg">
						<div className="flex items-center justify-between">
							<code className="text-sm font-mono text-blue-400">
								SQS_QUEUE_URL
							</code>
						</div>
						<div className="text-xs font-mono text-gray-400 mt-2 break-all">
							https://sqs.{config.region}.amazonaws.com/
							{accountInfo?.accountId || config.ecr_account_id || "<ACCOUNT_ID>"}/
							{config.project}-{config.env}-{config.sqs?.name || "queue"}
						</div>
					</div>
				</StyledSection>
			)}

			{/* Parameter Store - always show */}
			<StyledSection
				title="Custom Variables"
				description="Stored in AWS Parameter Store"
				icon={Key}
				iconColor="text-orange-400"
				actions={
					<Button
						size="sm"
						variant="outline"
						className="h-7 text-xs"
						onClick={() => {
							const region = config.region;
							window.open(
								`https://${region}.console.aws.amazon.com/systems-manager/parameters?region=${region}&tab=Table&path=${encodeURIComponent(parameterStorePath)}`,
								"_blank",
							);
						}}
					>
						<ExternalLink className="w-3 h-3 mr-1" />
						AWS Console
					</Button>
				}
			>
				<div className="p-3 bg-gray-900 rounded-lg">
					<code className="text-sm text-orange-400 font-mono">
						{parameterStorePath}*
					</code>
					<p className="text-xs text-gray-500 mt-2">
						Parameters created here become environment variables
					</p>
				</div>
			</StyledSection>
		</div>
	);
}
