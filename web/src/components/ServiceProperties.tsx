import {
	AlertTriangle,
	Database,
	ExternalLink,
	Info,
	Share2,
} from "lucide-react";
import { useId } from "react";
import type { AccountInfo } from "../api/infrastructure";
import type { ComponentNode } from "../types";
import type {
	ServiceNetworkAccess,
	YamlInfrastructureConfig,
} from "../types/yamlConfig";
import { Alert, AlertDescription } from "./ui/alert";
import { Badge } from "./ui/badge";
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

const NETWORK_ACCESS_OPTIONS: Array<{
	value: ServiceNetworkAccess;
	label: string;
	description: string;
}> = [
	{
		value: "public",
		label: "Public",
		description:
			"Reachable only through the project's front door (the ALB, or API Gateway when the ALB is off).",
	},
	{
		value: "internal",
		label: "Internal",
		description:
			"Reachable only from inside the VPC. Not on the ALB or API Gateway.",
	},
	{
		value: "full_open",
		label: "Full open",
		description:
			"Front door, and the container port open to the whole internet on the task's public IP.",
	},
	{
		value: "none",
		label: "None",
		description: "No inbound connections at all, for workers.",
	},
];

interface ServicePropertiesProps {
	config: YamlInfrastructureConfig;
	onConfigChange: (config: Partial<YamlInfrastructureConfig>) => void;
	accountInfo?: AccountInfo;
	node: ComponentNode;
}

export function ServiceProperties({
	config,
	onConfigChange,
	accountInfo,
	node,
}: ServicePropertiesProps) {
	const uid = useId();
	// Extract service name from node id
	const serviceName = node.id.replace("service-", "");

	// Find the service configuration
	const serviceConfig = config.services?.find(
		(service) => service.name === serviceName,
	);

	const ecrRepoName = `${config.project}_service_${serviceName}`;

	// Under cross_account the repository is the one in the source registry.
	const usesSharedEcr = config.ecr_strategy === "cross_account";
	const accountId = usesSharedEcr
		? config.ecr_account_id
		: accountInfo?.accountId || config.account_id;
	const region = usesSharedEcr
		? config.ecr_account_region || config.region
		: config.region;

	const ecrUri = `${accountId || "<YOUR_ACCOUNT_ID>"}.dkr.ecr.${region}.amazonaws.com/${ecrRepoName}`;

	const handleServiceChange = (
		updates: Partial<NonNullable<YamlInfrastructureConfig["services"]>[0]>,
	) => {
		if (!config.services) return;

		const updatedServices = config.services.map((service) =>
			service.name === serviceName ? { ...service, ...updates } : service,
		);

		onConfigChange({ services: updatedServices });
	};

	if (!serviceConfig) {
		return (
			<Alert className="border-red-600">
				<AlertTriangle className="h-4 w-4 text-red-600" />
				<AlertDescription>
					Service "{serviceName}" not found in configuration.
				</AlertDescription>
			</Alert>
		);
	}

	return (
		<Card className="w-full">
			<CardHeader>
				<CardTitle>{serviceName} Service Configuration</CardTitle>
				<CardDescription>Configure your service settings</CardDescription>
			</CardHeader>
			<CardContent className="space-y-4">
				{/* Enabled Toggle - at the very top */}
				<div className="flex items-center justify-between">
					<div className="flex-1">
						<Label htmlFor={`${uid}-enabled`}>Enabled</Label>
						<p className="text-xs text-gray-500 mt-1">
							When disabled, all settings are kept but the service is not
							deployed
						</p>
					</div>
					<Switch
						id={`${uid}-enabled`}
						checked={serviceConfig.enabled !== false}
						onCheckedChange={(checked) =>
							handleServiceChange({ enabled: checked })
						}
						className="data-[state=checked]:bg-green-500 data-[state=unchecked]:bg-gray-600"
					/>
				</div>

				{serviceConfig.enabled === false && (
					<Alert className="border-yellow-600 bg-yellow-900/20">
						<AlertTriangle className="h-4 w-4 text-yellow-400" />
						<AlertDescription className="text-xs text-gray-300">
							This service is <strong>disabled</strong>. It will not be included
							in the next Terraform generation. All configuration is preserved
							and can be re-enabled at any time.
						</AlertDescription>
					</Alert>
				)}

				<Separator />

				{/* Auto-Deploy Toggle */}
				<div className="flex items-center justify-between">
					<div className="flex-1">
						<Label htmlFor={`${uid}-auto-deploy`}>Auto-Deploy</Label>
						<p className="text-xs text-gray-500 mt-1">
							Redeploy this service automatically when a new image is pushed, or
							when its SSM/S3 configuration changes
						</p>
					</div>
					<Switch
						id={`${uid}-auto-deploy`}
						checked={serviceConfig.auto_deploy ?? config.auto_deploy ?? false}
						onCheckedChange={(checked) =>
							handleServiceChange({ auto_deploy: checked })
						}
						className="data-[state=checked]:bg-green-500 data-[state=unchecked]:bg-gray-600"
					/>
				</div>

				{!(serviceConfig.auto_deploy ?? config.auto_deploy ?? false) && (
					<Alert className="border-blue-600 bg-blue-900/20">
						<Info className="h-4 w-4 text-blue-400" />
						<AlertDescription className="text-xs text-gray-300">
							Automatic deploys are <strong>off</strong> for this service.
							Pushes and config changes are still delivered to the CI/CD Lambda
							and logged as
							<code className="mx-1">
								auto_deploy is disabled for {serviceName}
							</code>
							rather than silently doing nothing. Manual deploys still work.
						</AlertDescription>
					</Alert>
				)}

				<Separator />

				{/* Network Access (schema v30) */}
				<div className="space-y-2">
					<Label htmlFor={`${uid}-network-access`}>Network Access</Label>
					<Select
						value={serviceConfig.network_access ?? "public"}
						onValueChange={(value) =>
							handleServiceChange({
								network_access: value as ServiceNetworkAccess,
							})
						}
					>
						<SelectTrigger
							id={`${uid}-network-access`}
							className="bg-gray-800 border-gray-600 text-white"
						>
							<SelectValue />
						</SelectTrigger>
						<SelectContent>
							{NETWORK_ACCESS_OPTIONS.map((option) => (
								<SelectItem key={option.value} value={option.value}>
									{option.label}
								</SelectItem>
							))}
						</SelectContent>
					</Select>
					<p className="text-xs text-gray-500">
						{
							NETWORK_ACCESS_OPTIONS.find(
								(o) => o.value === (serviceConfig.network_access ?? "public"),
							)?.description
						}{" "}
						Outbound traffic is the same in every mode.
					</p>
				</div>

				{(serviceConfig.network_access === "internal" ||
					serviceConfig.network_access === "none") &&
					!serviceConfig.health_check_command?.length && (
						<Alert className="border-yellow-600 bg-yellow-900/20">
							<AlertTriangle className="h-4 w-4 text-yellow-400" />
							<AlertDescription className="text-xs text-gray-300">
								This service is not behind the ALB, so nothing health-checks it:
								ECS replaces a task only when its process exits. Add
								<code className="mx-1">health_check_command</code>
								to the service in the YAML, using a tool that exists in the
								image.
							</AlertDescription>
						</Alert>
					)}

				<Separator />

				{/* Essential Container Toggle */}
				<div className="flex items-center justify-between">
					<div className="flex-1">
						<Label htmlFor={`${uid}-essential`}>Essential Container</Label>
						<p className="text-xs text-gray-500 mt-1">
							If this container stops, stop all other containers
						</p>
					</div>
					<Switch
						id={`${uid}-essential`}
						checked={serviceConfig.essential !== false} // default true
						onCheckedChange={(checked) =>
							handleServiceChange({ essential: checked })
						}
						className="data-[state=checked]:bg-blue-500 data-[state=unchecked]:bg-gray-600"
					/>
				</div>

				<Separator />

				<div className="space-y-2">
					<Label htmlFor={`${uid}-docker_image`}>External Docker Image</Label>
					<Input
						id={`${uid}-docker_image`}
						value={serviceConfig.docker_image || ""}
						onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
							handleServiceChange({ docker_image: e.target.value })
						}
						placeholder="docker.io/myapp:latest"
						className="bg-gray-800 border-gray-600 text-white font-mono"
					/>
					<p className="text-xs text-gray-500">
						Optional: Use external Docker image instead of the ECR repository
					</p>

					{/* ECR Configuration Display */}
					{serviceConfig.ecr_config && (
						<div className="mt-2 p-3 bg-gray-900/50 border border-gray-700 rounded-lg">
							<div className="flex items-start gap-2">
								<Database className="w-4 h-4 text-gray-400 mt-0.5 flex-shrink-0" />
								<div className="flex-1 space-y-2">
									<div className="flex items-center gap-2">
										<Label className="text-xs text-gray-300">
											ECR Configuration:
										</Label>
										{serviceConfig.ecr_config.mode === "create_ecr" && (
											<Badge variant="default" className="text-xs">
												{usesSharedEcr
													? "Source Environment Repository"
													: "Dedicated Repository"}
											</Badge>
										)}
										{serviceConfig.ecr_config.mode === "manual_repo" && (
											<Badge
												variant="secondary"
												className="text-xs flex items-center gap-1"
											>
												<ExternalLink className="w-3 h-3" />
												Manual Repository
											</Badge>
										)}
										{serviceConfig.ecr_config.mode === "use_existing" && (
											<Badge
												variant="outline"
												className="text-xs flex items-center gap-1"
											>
												<Share2 className="w-3 h-3" />
												Shared Repository
											</Badge>
										)}
									</div>

									{serviceConfig.ecr_config.mode === "create_ecr" && (
										<div>
											<p className="text-xs text-gray-400 font-mono break-all">
												{ecrUri}
											</p>
											<p className="text-xs text-gray-500 mt-1">
												{usesSharedEcr
													? "Pulled from the source environment's ECR. Nothing is created here."
													: "A dedicated ECR repository will be created for this service"}
											</p>
										</div>
									)}

									{serviceConfig.ecr_config.mode === "manual_repo" &&
										serviceConfig.ecr_config.repository_uri && (
											<div>
												<p className="text-xs text-gray-400 font-mono break-all">
													{serviceConfig.ecr_config.repository_uri}
												</p>
												<p className="text-xs text-gray-500 mt-1">
													Using manually specified ECR repository
												</p>
											</div>
										)}

									{serviceConfig.ecr_config.mode === "use_existing" && (
										<div>
											<p className="text-xs text-gray-300">
												Source:{" "}
												<span className="font-mono text-gray-400">
													{serviceConfig.ecr_config.source_service_type?.replace(
														"_",
														" ",
													)}{" "}
													/ {serviceConfig.ecr_config.source_service_name}
												</span>
											</p>
											<p className="text-xs text-gray-500 mt-1">
												Sharing ECR repository from another service
											</p>
										</div>
									)}
								</div>
							</div>
						</div>
					)}

					{/* Legacy ECR Repository Info (when no ecr_config) */}
					{!serviceConfig.ecr_config && (
						<div className="mt-2 p-3 bg-blue-900/20 border border-blue-700 rounded-lg">
							<div className="flex items-start gap-2">
								<Info className="w-4 h-4 text-blue-400 mt-0.5 flex-shrink-0" />
								<div className="flex-1">
									<p className="text-xs text-gray-300">
										<strong className="text-blue-400">
											Default ECR Repository (Dev only):
										</strong>
									</p>
									<p className="text-xs font-mono text-gray-400 mt-1 break-all">
										{ecrUri}
									</p>
									<p className="text-xs text-gray-500 mt-2">
										ECR repositories for services are only created in
										development environment. In production, you must use an
										external Docker image.
									</p>
								</div>
							</div>
						</div>
					)}
				</div>

				<div className="space-y-2">
					<Label htmlFor={`${uid}-container_command`}>Container Command</Label>
					<Input
						id={`${uid}-container_command`}
						value={
							Array.isArray(serviceConfig.container_command)
								? serviceConfig.container_command.join(", ")
								: serviceConfig.container_command || ""
						}
						onChange={(e: React.ChangeEvent<HTMLInputElement>) => {
							const commands = e.target.value
								.split(",")
								.map((cmd: string) => cmd.trim())
								.filter((cmd: string) => cmd);
							handleServiceChange({
								container_command: commands.length > 0 ? commands : undefined,
							});
						}}
						placeholder="npm, start"
						className="bg-gray-800 border-gray-600 text-white font-mono"
					/>
					<p className="text-xs text-gray-500">
						Override container startup command (comma-separated)
					</p>
				</div>

				<Separator />

				{/* VPC-aware port configuration alert */}
				{!config.use_default_vpc ? (
					<Alert className="border-yellow-600 bg-yellow-900/20">
						<AlertTriangle className="h-4 w-4 text-yellow-400" />
						<AlertDescription className="text-xs text-gray-300">
							<strong>Custom VPC (awsvpc mode):</strong> host_port is
							automatically set to match container_port and cannot be changed.
							This is required for ECS Fargate with custom VPC.
						</AlertDescription>
					</Alert>
				) : (
					<Alert className="border-blue-600 bg-blue-900/20">
						<Info className="h-4 w-4 text-blue-400" />
						<AlertDescription className="text-xs text-gray-300">
							<strong>Default VPC:</strong> host_port is automatically synced
							with container_port. For ECS Fargate, these should always match.
						</AlertDescription>
					</Alert>
				)}

				<div className="space-y-2">
					<Label htmlFor={`${uid}-container_port`}>Container Port</Label>
					<Input
						id={`${uid}-container_port`}
						type="number"
						value={serviceConfig.container_port || 3000}
						onChange={(e: React.ChangeEvent<HTMLInputElement>) => {
							const port = parseInt(e.target.value, 10) || 3000;
							handleServiceChange({
								container_port: port,
								host_port: port, // Always sync for awsvpc compatibility
							});
						}}
						placeholder="3000"
						className="bg-gray-800 border-gray-600 text-white"
					/>
					<p className="text-xs text-gray-500">
						Port your application listens on (default: 3000)
					</p>
				</div>

				<div className="space-y-2">
					<Label htmlFor={`${uid}-host_port`}>
						Host Port
						{!config.use_default_vpc && (
							<span className="text-xs text-gray-500 ml-2">(auto-synced)</span>
						)}
					</Label>
					<Input
						id={`${uid}-host_port`}
						type="number"
						value={
							serviceConfig.host_port || serviceConfig.container_port || 3000
						}
						onChange={(e: React.ChangeEvent<HTMLInputElement>) => {
							// Only allow editing if using default VPC
							if (config.use_default_vpc) {
								handleServiceChange({
									host_port: parseInt(e.target.value, 10) || 3000,
								});
							}
						}}
						placeholder="3000"
						disabled={!config.use_default_vpc}
						className={`${
							!config.use_default_vpc
								? "bg-gray-900 border-gray-700 text-gray-500 cursor-not-allowed"
								: "bg-gray-800 border-gray-600 text-white"
						}`}
					/>
					<p className="text-xs text-gray-500">
						{!config.use_default_vpc
							? "Automatically matches container_port (required for custom VPC)"
							: "Host port mapping (should match container_port for awsvpc)"}
					</p>
				</div>
			</CardContent>
		</Card>
	);
}
