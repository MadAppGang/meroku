import type React from "react";
import { useMemo, useState } from "react";
import type { Service } from "../types/components";
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

interface AddServiceDialogProps {
	open: boolean;
	onClose: () => void;
	onAdd: (service: Service) => void;
	existingServices: string[];
	config: YamlInfrastructureConfig;
}

export function AddServiceDialog({
	open,
	onClose,
	onAdd,
	existingServices,
	config,
}: AddServiceDialogProps) {
	const [formData, setFormData] = useState({
		name: "",
		docker_image: "",
		container_command: "",
		container_port: 8080,
		cpu: 256,
		memory: 512,
		desired_count: 1,
		health_check_path: "/health",
		environment_variables: "",
	});

	const [ecrConfig, setEcrConfig] = useState<ECRConfig>({ mode: "create_ecr" });
	const [errors, setErrors] = useState<Record<string, string>>({});

	// Build available ECR sources from all service types
	const availableSources = useMemo(() => {
		const sources: Array<{ name: string; type: "services" | "event_processor_tasks" | "scheduled_tasks"; displayType: string }> = [];

		// Add services with create_ecr mode
		config.services?.forEach(svc => {
			if (svc.name !== formData.name && (!svc.ecr_config || svc.ecr_config.mode === "create_ecr")) {
				sources.push({
					name: svc.name,
					type: "services",
					displayType: "Service",
				});
			}
		});

		// Add event processors with create_ecr mode
		config.event_processor_tasks?.forEach(ep => {
			if (!ep.ecr_config || ep.ecr_config.mode === "create_ecr") {
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
			newErrors.name = "Service name is required";
		} else if (!/^[a-z0-9-]+$/.test(formData.name)) {
			newErrors.name =
				"Service name must contain only lowercase letters, numbers, and hyphens";
		} else if (existingServices.includes(formData.name)) {
			newErrors.name = "A service with this name already exists";
		}

		// Docker image is only required for manual_repo mode
		if (ecrConfig.mode === "manual_repo" && !formData.docker_image) {
			newErrors.docker_image = "Docker image is required for manual repository mode";
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

		const service: Service = {
			name: formData.name,
			docker_image: formData.docker_image,
			container_port: formData.container_port,
			host_port: formData.container_port, // Must match container_port for awsvpc network mode
			cpu: formData.cpu,
			memory: formData.memory,
			desired_count: formData.desired_count,
			health_check_path: formData.health_check_path,
			api_domain_prefix: formData.name, // Enable custom domain by default with service name as prefix
			ecr_config: ecrConfig,
		};

		if (formData.container_command) {
			service.container_command = formData.container_command
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
					service.environment_variables = envVars;
				}
			} catch (_error) {
				newErrors.environment_variables =
					"Invalid environment variables format";
				setErrors(newErrors);
				return;
			}
		}

		onAdd(service);
		handleClose();
	};

	const handleClose = () => {
		setFormData({
			name: "",
			docker_image: "",
			container_command: "",
			container_port: 8080,
			cpu: 256,
			memory: 512,
			desired_count: 1,
			health_check_path: "/health",
			environment_variables: "",
		});
		setEcrConfig({ mode: "create_ecr" });
		setErrors({});
		onClose();
	};

	return (
		<Dialog open={open} onOpenChange={handleClose}>
			<DialogContent className="sm:max-w-[600px] max-h-[90vh] overflow-y-auto">
				<DialogHeader>
					<DialogTitle>Add New Service</DialogTitle>
				</DialogHeader>
				<form onSubmit={handleSubmit}>
					<div className="grid gap-4 py-4">
						<div className="grid gap-2">
							<Label htmlFor="name">Service Name</Label>
							<Input
								id="name"
								value={formData.name}
								onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
									setFormData({ ...formData, name: e.target.value })
								}
								placeholder="my-service"
							/>
							{errors.name && (
								<p className="text-sm text-red-500">{errors.name}</p>
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
								placeholder="npm, start"
							/>
						</div>

						<div className="grid grid-cols-2 gap-4">
							<div className="grid gap-2">
								<Label htmlFor="container_port">Container Port</Label>
								<Input
									id="container_port"
									type="number"
									value={formData.container_port}
									onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
										setFormData({
											...formData,
											container_port: Number.parseInt(e.target.value) || 8080,
										})
									}
								/>
							</div>

							<div className="grid gap-2">
								<Label htmlFor="desired_count">Desired Count</Label>
								<Input
									id="desired_count"
									type="number"
									value={formData.desired_count}
									onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
										setFormData({
											...formData,
											desired_count: Number.parseInt(e.target.value) || 1,
										})
									}
									min="0"
									max="10"
								/>
							</div>
						</div>

						<div className="grid grid-cols-2 gap-4">
							<div className="grid gap-2">
								<Label htmlFor="cpu">CPU (units)</Label>
								<Select
									value={formData.cpu.toString()}
									onValueChange={(value: string) =>
										setFormData({ ...formData, cpu: Number.parseInt(value) })
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
										<SelectItem value="4096">4096 (4 vCPU)</SelectItem>
									</SelectContent>
								</Select>
							</div>

							<div className="grid gap-2">
								<Label htmlFor="memory">Memory (MB)</Label>
								<Select
									value={formData.memory.toString()}
									onValueChange={(value: string) =>
										setFormData({ ...formData, memory: Number.parseInt(value) })
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
										<SelectItem value="8192">8 GB</SelectItem>
									</SelectContent>
								</Select>
							</div>
						</div>

						<div className="grid gap-2">
							<Label htmlFor="health_check_path">Health Check Path</Label>
							<Input
								id="health_check_path"
								value={formData.health_check_path}
								onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
									setFormData({
										...formData,
										health_check_path: e.target.value,
									})
								}
							/>
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
								placeholder="NODE_ENV=production&#10;PORT=8080"
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
							accountId={config.account_id}
							region={config.region}
						/>
					</div>

					<DialogFooter>
						<Button type="button" variant="outline" onClick={handleClose}>
							Cancel
						</Button>
						<Button type="submit">Add Service</Button>
					</DialogFooter>
				</form>
			</DialogContent>
		</Dialog>
	);
}
