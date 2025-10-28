import { Plus, X } from "lucide-react";
import type React from "react";
import { useMemo, useState } from "react";
import type { EventTask } from "../types/components";
import type { ECRConfig, YamlInfrastructureConfig } from "../types/yamlConfig";
import { Button } from "./ui/button";
import {
	Dialog,
	DialogContent,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "./ui/dialog";
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
import { Textarea } from "./ui/textarea";
import { ECRConfigSection } from "./ECRConfigSection";

interface AddEventTaskDialogProps {
	open: boolean;
	onClose: () => void;
	onAdd: (task: EventTask) => void;
	existingTasks: string[];
	availableServices: string[];
	config: YamlInfrastructureConfig;
}

export function AddEventTaskDialog({
	open,
	onClose,
	onAdd,
	existingTasks,
	availableServices,
	config,
}: AddEventTaskDialogProps) {
	const [formData, setFormData] = useState({
		name: "",
		rule_name: "",
		detail_types: [""],
		sources: [""],
		docker_image: "",
		container_command: "",
		cpu: 256,
		memory: 512,
		environment_variables: "",
	});

	const [ecrConfig, setEcrConfig] = useState<ECRConfig>({ mode: "create_ecr" });
	const [errors, setErrors] = useState<Record<string, string>>({});

	// Calculate ECR URL for preview
	const getEcrUrl = (): string => {
		if (!formData.name) return "";

		const accountId = "<ACCOUNT_ID>";
		const region = config.region || "us-east-1";

		if (ecrConfig.mode === "create_ecr") {
			const taskEcrRepoName = `${config.project}_task_${formData.name}`;
			return `${accountId}.dkr.ecr.${region}.amazonaws.com/${taskEcrRepoName}`;
		} else if (ecrConfig.mode === "manual_repo" && ecrConfig.repository_uri) {
			return ecrConfig.repository_uri;
		} else if (ecrConfig.mode === "use_existing" && ecrConfig.source_service_name) {
			const sourceRepoName = `${config.project}_task_${ecrConfig.source_service_name}`;
			return `${accountId}.dkr.ecr.${region}.amazonaws.com/${sourceRepoName}`;
		}
		return "";
	};

	// Build available ECR sources from all service types
	const availableSources = useMemo(() => {
		const sources: Array<{ name: string; type: "services" | "event_processor_tasks" | "scheduled_tasks"; displayType: string }> = [];

		// Add services with create_ecr mode
		config.services?.forEach(svc => {
			if (!svc.ecr_config || svc.ecr_config.mode === "create_ecr") {
				sources.push({
					name: svc.name,
					type: "services",
					displayType: "Service",
				});
			}
		});

		// Add event processors with create_ecr mode
		config.event_processor_tasks?.forEach(ep => {
			if (ep.name !== formData.name && (!ep.ecr_config || ep.ecr_config.mode === "create_ecr")) {
				sources.push({
					name: ep.name,
					type: "event_processor_tasks",
					displayType: "Event Processor",
				});
			}
		});

		// Add scheduled tasks with create_ecr mode
		config.scheduled_tasks?.forEach(st => {
			if (!st.ecr_config || st.ecr_config.mode === "create_ecr") {
				sources.push({
					name: st.name,
					type: "scheduled_tasks",
					displayType: "Cron Job",
				});
			}
		});

		return sources;
	}, [config, formData.name]);

	const handleSubmit = (e: React.FormEvent) => {
		e.preventDefault();

		const newErrors: Record<string, string> = {};

		if (!formData.name) {
			newErrors.name = "Task name is required";
		} else if (!/^[a-z0-9-]+$/.test(formData.name)) {
			newErrors.name =
				"Task name must contain only lowercase letters, numbers, and hyphens";
		} else if (existingTasks.includes(formData.name)) {
			newErrors.name = "An event task with this name already exists";
		}

		if (!formData.rule_name) {
			newErrors.rule_name = "Rule name is required";
		}

		const validDetailTypes = formData.detail_types.filter((dt) => dt.trim());
		if (validDetailTypes.length === 0) {
			newErrors.detail_types = "At least one detail type is required";
		}

		const validSources = formData.sources.filter((s) => s.trim());
		if (validSources.length === 0) {
			newErrors.sources = "At least one source is required";
		}

		// Validate ECR config
		if (ecrConfig.mode === "manual_repo" && !ecrConfig.repository_uri) {
			newErrors.repository_uri = "Repository URI is required for manual mode";
		}

		if (ecrConfig.mode === "use_existing" && !ecrConfig.source_service_name) {
			newErrors.source_service_name = "Source service is required";
		}

		if (Object.keys(newErrors).length > 0) {
			setErrors(newErrors);
			return;
		}

		const task: EventTask = {
			name: formData.name,
			rule_name: formData.rule_name,
			detail_types: validDetailTypes,
			sources: validSources,
			cpu: formData.cpu,
			memory: formData.memory,
			ecr_config: ecrConfig,
		};

		if (formData.docker_image) {
			task.docker_image = formData.docker_image;
		}

		if (formData.container_command) {
			task.container_command = formData.container_command
				.split(",")
				.map((cmd) => cmd.trim())
				.filter((cmd) => cmd);
		}

		if (formData.environment_variables) {
			try {
				const envVars = formData.environment_variables
					.split("\n")
					.map((line) => line.trim())
					.filter((line) => line?.includes("="))
					.reduce(
						(acc, line) => {
							const [key, ...valueParts] = line.split("=");
							acc[key.trim()] = valueParts.join("=").trim();
							return acc;
						},
						{} as Record<string, string>,
					);

				if (Object.keys(envVars).length > 0) {
					task.environment_variables = envVars;
				}
			} catch (_error) {
				newErrors.environment_variables =
					"Invalid environment variables format";
				setErrors(newErrors);
				return;
			}
		}

		onAdd(task);
		handleClose();
	};

	const handleClose = () => {
		setFormData({
			name: "",
			rule_name: "",
			detail_types: [""],
			sources: [""],
			docker_image: "",
			container_command: "",
			cpu: 256,
			memory: 512,
			environment_variables: "",
		});
		setEcrConfig({ mode: "create_ecr" });
		setErrors({});
		onClose();
	};

	const addDetailType = () => {
		setFormData({ ...formData, detail_types: [...formData.detail_types, ""] });
	};

	const removeDetailType = (index: number) => {
		setFormData({
			...formData,
			detail_types: formData.detail_types.filter((_, i) => i !== index),
		});
	};

	const updateDetailType = (index: number, value: string) => {
		const newDetailTypes = [...formData.detail_types];
		newDetailTypes[index] = value;
		setFormData({ ...formData, detail_types: newDetailTypes });
	};

	const addSource = () => {
		setFormData({ ...formData, sources: [...formData.sources, ""] });
	};

	const removeSource = (index: number) => {
		setFormData({
			...formData,
			sources: formData.sources.filter((_, i) => i !== index),
		});
	};

	const updateSource = (index: number, value: string) => {
		const newSources = [...formData.sources];
		newSources[index] = value;
		setFormData({ ...formData, sources: newSources });
	};

	return (
		<Dialog open={open} onOpenChange={handleClose}>
			<DialogContent className="sm:max-w-[600px] max-h-[90vh] overflow-y-auto">
				<DialogHeader>
					<DialogTitle>Add Event Task</DialogTitle>
				</DialogHeader>
				<form onSubmit={handleSubmit}>
					<div className="grid gap-4 py-4">
						<div className="grid gap-2">
							<Label htmlFor="name">Task Name</Label>
							<Input
								id="name"
								value={formData.name}
								onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
									setFormData({ ...formData, name: e.target.value })
								}
								placeholder="process-orders"
							/>
							{errors.name && (
								<p className="text-sm text-red-500">{errors.name}</p>
							)}
						</div>

						<div className="grid gap-2">
							<Label htmlFor="rule_name">EventBridge Rule Name</Label>
							<Input
								id="rule_name"
								value={formData.rule_name}
								onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
									setFormData({ ...formData, rule_name: e.target.value })
								}
								placeholder="order-processing-rule"
							/>
							{errors.rule_name && (
								<p className="text-sm text-red-500">{errors.rule_name}</p>
							)}
						</div>

						<div className="grid gap-2">
							<Label>Event Detail Types</Label>
							{formData.detail_types.map((detailType, index) => (
								<div
									key={`detail-type-${index}`}
									className="flex gap-2"
								>
									<Input
										value={detailType}
										onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
											updateDetailType(index, e.target.value)
										}
										placeholder="order.created"
									/>
									{formData.detail_types.length > 1 && (
										<Button
											type="button"
											variant="outline"
											size="icon"
											onClick={() => removeDetailType(index)}
										>
											<X className="h-4 w-4" />
										</Button>
									)}
								</div>
							))}
							<Button
								type="button"
								variant="outline"
								size="sm"
								onClick={addDetailType}
								className="w-fit"
							>
								<Plus className="h-4 w-4 mr-2" />
								Add Detail Type
							</Button>
							{errors.detail_types && (
								<p className="text-sm text-red-500">{errors.detail_types}</p>
							)}
						</div>

						<div className="grid gap-2">
							<Label>Event Sources</Label>
							{formData.sources.map((source, index) => (
								<div key={`source-${index}`} className="flex gap-2">
									<Select
										value={source}
										onValueChange={(value: string) =>
											updateSource(index, value)
										}
									>
										<SelectTrigger>
											<SelectValue placeholder="Select a service" />
										</SelectTrigger>
										<SelectContent>
											{availableServices.map((service) => (
												<SelectItem key={service} value={service}>
													{service}
												</SelectItem>
											))}
											<SelectItem value="custom">Custom Source</SelectItem>
										</SelectContent>
									</Select>
									{source === "custom" && (
										<Input
											placeholder="Enter custom source"
											onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
												updateSource(index, e.target.value)
											}
										/>
									)}
									{formData.sources.length > 1 && (
										<Button
											type="button"
											variant="outline"
											size="icon"
											onClick={() => removeSource(index)}
										>
											<X className="h-4 w-4" />
										</Button>
									)}
								</div>
							))}
							<Button
								type="button"
								variant="outline"
								size="sm"
								onClick={addSource}
								className="w-fit"
							>
								<Plus className="h-4 w-4 mr-2" />
								Add Source
							</Button>
							{errors.sources && (
								<p className="text-sm text-red-500">{errors.sources}</p>
							)}
						</div>

						<div className="grid gap-2">
							<div className="flex items-center justify-between">
								<Label htmlFor="docker_image" className="text-sm font-medium">
									Image Tag (Optional)
								</Label>
								<span className="text-xs text-muted-foreground">
									e.g., latest, v1.2.3, nginx.1
								</span>
							</div>
							<Input
								id="docker_image"
								value={formData.docker_image}
								onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
									setFormData({ ...formData, docker_image: e.target.value })
								}
								placeholder="latest"
								className="font-mono text-sm"
							/>
							{getEcrUrl() && (
								<div className="p-2 bg-blue-950/30 border border-blue-900/50 rounded">
									<p className="text-xs text-muted-foreground">
										<span className="text-gray-500">Full image:</span>{" "}
										<code className="text-blue-400">
											{getEcrUrl()}:{formData.docker_image || "latest"}
										</code>
									</p>
								</div>
							)}
						</div>

						<div className="grid gap-2">
							<Label htmlFor="container_command">
								Container Command (comma-separated)
							</Label>
							<Input
								id="container_command"
								value={formData.container_command}
								onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
									setFormData({
										...formData,
										container_command: e.target.value,
									})
								}
								placeholder="node, scripts/process-event.js"
							/>
						</div>

						<div className="grid grid-cols-2 gap-4">
							<div className="grid gap-2">
								<Label htmlFor="cpu">CPU (units)</Label>
								<Select
									value={formData.cpu.toString()}
									onValueChange={(value: string) =>
										setFormData({ ...formData, cpu: parseInt(value) })
									}
								>
									<SelectTrigger id="cpu">
										<SelectValue />
									</SelectTrigger>
									<SelectContent>
										<SelectItem value="256">256 (0.25 vCPU)</SelectItem>
										<SelectItem value="512">512 (0.5 vCPU)</SelectItem>
										<SelectItem value="1024">1024 (1 vCPU)</SelectItem>
										<SelectItem value="2048">2048 (2 vCPU)</SelectItem>
									</SelectContent>
								</Select>
							</div>

							<div className="grid gap-2">
								<Label htmlFor="memory">Memory (MB)</Label>
								<Select
									value={formData.memory.toString()}
									onValueChange={(value: string) =>
										setFormData({ ...formData, memory: parseInt(value) })
									}
								>
									<SelectTrigger id="memory">
										<SelectValue />
									</SelectTrigger>
									<SelectContent>
										<SelectItem value="512">512 MB</SelectItem>
										<SelectItem value="1024">1 GB</SelectItem>
										<SelectItem value="2048">2 GB</SelectItem>
										<SelectItem value="4096">4 GB</SelectItem>
									</SelectContent>
								</Select>
							</div>
						</div>

						<div className="grid gap-2">
							<Label htmlFor="environment_variables">
								Environment Variables (KEY=VALUE, one per line)
							</Label>
							<Textarea
								id="environment_variables"
								value={formData.environment_variables}
								onChange={(e: React.ChangeEvent<HTMLTextAreaElement>) =>
									setFormData({
										...formData,
										environment_variables: e.target.value,
									})
								}
								placeholder="EVENT_PROCESSOR=true&#10;LOG_LEVEL=info"
								rows={4}
							/>
							{errors.environment_variables && (
								<p className="text-sm text-red-500">
									{errors.environment_variables}
								</p>
							)}
						</div>

						<Separator />

						<ECRConfigSection
							config={ecrConfig}
							onChange={setEcrConfig}
							availableSources={availableSources}
							currentServiceName={formData.name}
							errors={errors}
						/>
					</div>

					<DialogFooter>
						<Button type="button" variant="outline" onClick={handleClose}>
							Cancel
						</Button>
						<Button type="submit">Add Task</Button>
					</DialogFooter>
				</form>
			</DialogContent>
		</Dialog>
	);
}
