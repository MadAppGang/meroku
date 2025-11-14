import { AlertTriangle, Database, ExternalLink, Info, Share2 } from "lucide-react";
import type { AccountInfo } from "../api/infrastructure";
import type { ComponentNode } from "../types";
import type { YamlInfrastructureConfig } from "../types/yamlConfig";
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
import { Separator } from "./ui/separator";
import { Switch } from "./ui/switch";

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
	// Extract service name from node id
	const serviceName = node.id.replace("service-", "");

	// Find the service configuration
	const serviceConfig = config.services?.find(
		(service) => service.name === serviceName,
	);

	// Generate the ECR repository name based on config
	const ecrRepoName = `${config.project}_${serviceName}`;

	// Use accountInfo if available, otherwise fall back to config values
	const accountId = accountInfo?.accountId || config.ecr_account_id;
	const region = config.ecr_account_region || config.region;

	// ECR URI - note that ECR repos for additional services are only created in dev environment
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
				{/* Essential Container Toggle - at the top */}
				<div className="flex items-center justify-between">
					<div className="flex-1">
						<Label htmlFor="essential">Essential Container</Label>
						<p className="text-xs text-gray-500 mt-1">
							If this container stops, stop all other containers
						</p>
					</div>
					<Switch
						id="essential"
						checked={serviceConfig.essential !== false} // default true
						onCheckedChange={(checked) =>
							handleServiceChange({ essential: checked })
						}
						className="data-[state=checked]:bg-blue-500 data-[state=unchecked]:bg-gray-600"
					/>
				</div>

				<Separator />

				<div className="space-y-2">
					<Label htmlFor="docker_image">External Docker Image</Label>
					<Input
						id="docker_image"
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
										<Label className="text-xs text-gray-300">ECR Configuration:</Label>
										{serviceConfig.ecr_config.mode === "create_ecr" && (
											<Badge variant="default" className="text-xs">
												Dedicated Repository
											</Badge>
										)}
										{serviceConfig.ecr_config.mode === "manual_repo" && (
											<Badge variant="secondary" className="text-xs flex items-center gap-1">
												<ExternalLink className="w-3 h-3" />
												Manual Repository
											</Badge>
										)}
										{serviceConfig.ecr_config.mode === "use_existing" && (
											<Badge variant="outline" className="text-xs flex items-center gap-1">
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
												A dedicated ECR repository will be created for this service
											</p>
										</div>
									)}

									{serviceConfig.ecr_config.mode === "manual_repo" && serviceConfig.ecr_config.repository_uri && (
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
												Source: <span className="font-mono text-gray-400">
													{serviceConfig.ecr_config.source_service_type?.replace("_", " ")} / {serviceConfig.ecr_config.source_service_name}
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
										ECR repositories for services are only created in development
										environment. In production, you must use an external Docker
										image.
									</p>
								</div>
							</div>
						</div>
					)}
				</div>

				<div className="space-y-2">
					<Label htmlFor="container_command">Container Command</Label>
					<Input
						id="container_command"
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
					<Label htmlFor="container_port">Container Port</Label>
					<Input
						id="container_port"
						type="number"
						value={serviceConfig.container_port || 3000}
						onChange={(e: React.ChangeEvent<HTMLInputElement>) => {
							const port = parseInt(e.target.value) || 3000;
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
					<Label htmlFor="host_port">
						Host Port
						{!config.use_default_vpc && (
							<span className="text-xs text-gray-500 ml-2">(auto-synced)</span>
						)}
					</Label>
					<Input
						id="host_port"
						type="number"
						value={serviceConfig.host_port || serviceConfig.container_port || 3000}
						onChange={(e: React.ChangeEvent<HTMLInputElement>) => {
							// Only allow editing if using default VPC
							if (config.use_default_vpc) {
								handleServiceChange({
									host_port: parseInt(e.target.value) || 3000,
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

				<Separator />

				{/* API Gateway Custom Domain for this Service */}
				<div className="flex items-center justify-between">
					<div className="flex-1">
						<Label htmlFor={`create-api-domain-${serviceName}`}>
							API Gateway Custom Domain
						</Label>
						<p className="text-xs text-gray-500 mt-1">
							Create API Gateway custom domain for this service
						</p>
					</div>
					<Switch
						id={`create-api-domain-${serviceName}`}
						checked={
							serviceConfig.api_domain_prefix !== undefined &&
							serviceConfig.api_domain_prefix !== null &&
							serviceConfig.api_domain_prefix !== ""
						}
						onCheckedChange={(checked) =>
							handleServiceChange({
								api_domain_prefix: checked ? serviceName : "",
							})
						}
						className="data-[state=checked]:bg-blue-500 data-[state=unchecked]:bg-gray-600"
					/>
				</div>

				{/* API Domain Prefix Input - shown when enabled */}
				{serviceConfig.api_domain_prefix && serviceConfig.api_domain_prefix !== "" && (
					<div className="space-y-2 ml-4">
						<Label htmlFor={`api-prefix-${serviceName}`}>
							API Domain Prefix
						</Label>
						<Input
							id={`api-prefix-${serviceName}`}
							value={serviceConfig.api_domain_prefix}
							onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
								handleServiceChange({
									api_domain_prefix: e.target.value || serviceName,
								})
							}
							placeholder={serviceName}
							className="bg-gray-800 border-gray-600 text-white"
						/>
						<p className="text-xs text-gray-500">
							Subdomain prefix for this service (default: service name)
						</p>
					</div>
				)}

				{serviceConfig.api_domain_prefix && serviceConfig.api_domain_prefix !== "" ? (
					<div className="p-3 bg-green-900/20 border border-green-700 rounded-lg">
						<div className="flex items-start gap-2">
							<Info className="w-4 h-4 text-green-400 mt-0.5 flex-shrink-0" />
							<div className="flex-1">
								<p className="text-xs text-gray-300">
									<strong className="text-green-400">
										API Gateway custom domain will be created
									</strong>
								</p>
								<p className="text-xs text-gray-500 mt-1">
									Route53 A record will point to API Gateway for this service
								</p>
							</div>
						</div>
					</div>
				) : (
					<div className="p-3 bg-yellow-900/20 border border-yellow-700 rounded-lg">
						<div className="flex items-start gap-2">
							<Info className="w-4 h-4 text-yellow-400 mt-0.5 flex-shrink-0" />
							<div className="flex-1">
								<p className="text-xs text-gray-300">
									<strong className="text-yellow-400">
										API Gateway custom domain disabled
									</strong>
								</p>
								<p className="text-xs text-gray-500 mt-1">
									Service will use default AWS API Gateway domain
								</p>
							</div>
						</div>
					</div>
				)}
			</CardContent>
		</Card>
	);
}
