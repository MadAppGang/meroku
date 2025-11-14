import { Info } from "lucide-react";
import type { AccountInfo } from "../api/infrastructure";
import type { YamlInfrastructureConfig } from "../types/yamlConfig";
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

interface BackendServicePropertiesProps {
	config: YamlInfrastructureConfig;
	onConfigChange: (config: Partial<YamlInfrastructureConfig>) => void;
	accountInfo?: AccountInfo;
}

export function BackendServiceProperties({
	config,
	onConfigChange,
	accountInfo,
}: BackendServicePropertiesProps) {
	// Generate the ECR repository name based on config
	const ecrRepoName = `${config.project}_backend`;

	// Use accountInfo if available, otherwise fall back to config values
	const accountId = accountInfo?.accountId || config.ecr_account_id;
	const region = config.ecr_account_region || config.region;

	// Always show the actual account ID when available
	const ecrUri = `${accountId || "<YOUR_ACCOUNT_ID>"}.dkr.ecr.${region}.amazonaws.com/${ecrRepoName}`;

	const handleWorkloadChange = (
		updates: Partial<YamlInfrastructureConfig["workload"]>,
	) => {
		onConfigChange({
			workload: {
				...config.workload,
				...updates,
			},
		});
	};

	const handleCreateApiDomainChange = (checked: boolean) => {
		onConfigChange({
			...config,
			domain: {
				enabled: config.domain?.enabled ?? false,
				...config.domain,
				// When enabled, set default prefix to "api"
				// When disabled, set to empty string to skip API Gateway custom domain creation
				api_domain_prefix: checked ? "api" : "",
			},
		});
	};

	return (
		<Card className="w-full">
			<CardHeader>
				<CardTitle>Backend Service Configuration</CardTitle>
				<CardDescription>
					Configure your backend service settings
				</CardDescription>
			</CardHeader>
			<CardContent className="space-y-4">
				<div className="space-y-2">
					<Label htmlFor="health_endpoint">Health Endpoint</Label>
					<Input
						id="health_endpoint"
						value={config.workload?.backend_health_endpoint || "/health"}
						onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
							handleWorkloadChange({ backend_health_endpoint: e.target.value })
						}
						placeholder="/health"
						className="bg-gray-800 border-gray-600 text-white"
					/>
					<p className="text-xs text-gray-500">
						API endpoint for health checks
					</p>
				</div>

				<div className="space-y-2">
					<Label htmlFor="backend_external_docker_image">
						External Docker Image
					</Label>
					<Input
						id="backend_external_docker_image"
						value={config.workload?.backend_external_docker_image || ""}
						onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
							handleWorkloadChange({
								backend_external_docker_image: e.target.value,
							})
						}
						placeholder="docker.io/myapp:latest"
						className="bg-gray-800 border-gray-600 text-white font-mono"
					/>
					<p className="text-xs text-gray-500">
						Optional: Use external Docker image instead of the ECR repository
					</p>

					{/* ECR Repository Info */}
					<div className="mt-2 p-3 bg-blue-900/20 border border-blue-700 rounded-lg">
						<div className="flex items-start gap-2">
							<Info className="w-4 h-4 text-blue-400 mt-0.5 flex-shrink-0" />
							<div className="flex-1">
								<p className="text-xs text-gray-300">
									<strong className="text-blue-400">
										Default ECR Repository:
									</strong>
								</p>
								<p className="text-xs font-mono text-gray-400 mt-1 break-all">
									{ecrUri}
								</p>
								<p className="text-xs text-gray-500 mt-2">
									If you leave this field empty, the backend service will use
									images from this ECR repository. Specify an external image
									only if you want to use a different registry.
								</p>
							</div>
						</div>
					</div>
				</div>

				<div className="space-y-2">
					<Label htmlFor="backend_container_command">Container Command</Label>
					<Input
						id="backend_container_command"
						value={
							Array.isArray(config.workload?.backend_container_command)
								? config.workload.backend_container_command.join(", ")
								: config.workload?.backend_container_command || ""
						}
						onChange={(e: React.ChangeEvent<HTMLInputElement>) => {
							const commands = e.target.value
								.split(",")
								.map((cmd: string) => cmd.trim())
								.filter((cmd: string) => cmd);
							handleWorkloadChange({
								backend_container_command:
									commands.length > 0 ? commands : undefined,
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

				<div className="space-y-2">
					<Label htmlFor="bucket_postfix">Backend Bucket Postfix</Label>
					<Input
						id="bucket_postfix"
						value={config.workload?.bucket_postfix || ""}
						onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
							handleWorkloadChange({ bucket_postfix: e.target.value })
						}
						placeholder="backend"
						className="bg-gray-800 border-gray-600 text-white"
					/>
					<p className="text-xs text-gray-500">
						S3 bucket name will be:{" "}
						<code className="text-blue-300">
							{config.project}-backend-{config.env}-
							{config.workload?.bucket_postfix || "backend"}
						</code>
					</p>
				</div>

				<div className="flex items-center justify-between">
					<div className="flex-1">
						<Label htmlFor="bucket_public">Public Bucket</Label>
						<p className="text-xs text-gray-500 mt-1">
							Allow public access to backend bucket
						</p>
					</div>
					<Switch
						id="bucket_public"
						checked={config.workload?.bucket_public || false}
						onCheckedChange={(checked) =>
							handleWorkloadChange({ bucket_public: checked })
						}
						className="data-[state=checked]:bg-blue-500 data-[state=unchecked]:bg-gray-600"
					/>
				</div>

				<Separator />

				<div className="space-y-2">
					<Label htmlFor="backend_image_port">Container Port</Label>
					<Input
						id="backend_image_port"
						type="number"
						value={config.workload?.backend_image_port || 8080}
						onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
							handleWorkloadChange({
								backend_image_port: parseInt(e.target.value) || 8080,
							})
						}
						placeholder="8080"
						className="bg-gray-800 border-gray-600 text-white"
					/>
					<p className="text-xs text-gray-500">
						Port your application listens on (default: 8080)
					</p>
				</div>

				<Separator />

				{/* API Gateway Custom Domain */}
				<div className="flex items-center justify-between">
					<div className="flex-1">
						<Label htmlFor="create-api-domain-backend">
							API Gateway Custom Domain
						</Label>
						<p className="text-xs text-gray-500 mt-1">
							Create API Gateway custom domain and Route53 A record for backend
							API
						</p>
					</div>
					<Switch
						id="create-api-domain-backend"
						checked={
							config.domain?.api_domain_prefix !== undefined &&
							config.domain?.api_domain_prefix !== null &&
							config.domain?.api_domain_prefix !== ""
						}
						onCheckedChange={handleCreateApiDomainChange}
						className="data-[state=checked]:bg-blue-500 data-[state=unchecked]:bg-gray-600"
					/>
				</div>

				{/* API Domain Prefix Input - shown when enabled */}
				{config.domain?.api_domain_prefix &&
					config.domain.api_domain_prefix !== "" && (
						<div className="space-y-2 ml-4">
							<Label htmlFor="api_domain_prefix">API Domain Prefix</Label>
							<Input
								id="api_domain_prefix"
								value={config.domain.api_domain_prefix}
								onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
									onConfigChange({
										...config,
										domain: {
											enabled: config.domain?.enabled ?? false,
											...config.domain,
											api_domain_prefix: e.target.value || "api",
										},
									})
								}
								placeholder="api"
								className="bg-gray-800 border-gray-600 text-white"
							/>
							<p className="text-xs text-gray-500">
								Subdomain prefix for API Gateway (e.g., "api" creates
								api.yourdomain.com)
							</p>
						</div>
					)}

				{config.domain?.api_domain_prefix &&
				config.domain.api_domain_prefix !== "" ? (
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
									Route53 A record will point to API Gateway for backend access
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
									Backend will use default AWS API Gateway domain
								</p>
							</div>
						</div>
					</div>
				)}
			</CardContent>
		</Card>
	);
}
