import {
	Activity,
	AlertCircle,
	DollarSign,
	Globe,
	Network,
	Shield,
} from "lucide-react";
import { useEffect, useState } from "react";
import type { YamlInfrastructureConfig } from "../types/yamlConfig";
import {
	DEFAULT_ALB_IDLE_TIMEOUT,
	MAX_ALB_IDLE_TIMEOUT,
	MIN_ALB_IDLE_TIMEOUT,
	parseAlbIdleTimeout,
} from "../utils/alb";
import { getAdditionalAlbDomain, getApiDomainUrl } from "../utils/apiDomain";
import { Button } from "./ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "./ui/card";
import { Input } from "./ui/input";
import { Label } from "./ui/label";
import { Switch } from "./ui/switch";

interface ALBNodePropertiesProps {
	config: YamlInfrastructureConfig;
	onConfigChange: (config: Partial<YamlInfrastructureConfig>) => void;
}

export function ALBNodeProperties({
	config,
	onConfigChange,
}: ALBNodePropertiesProps) {
	const isALBEnabled = config.alb?.enabled ?? false;
	const configuredIdleTimeout =
		config.alb?.idle_timeout ?? DEFAULT_ALB_IDLE_TIMEOUT;
	const idleTimeoutValidationMessage = `Enter a whole number from ${MIN_ALB_IDLE_TIMEOUT} to ${MAX_ALB_IDLE_TIMEOUT.toLocaleString()}.`;
	const [idleTimeoutInput, setIdleTimeoutInput] = useState(
		String(configuredIdleTimeout),
	);
	const [idleTimeoutError, setIdleTimeoutError] = useState("");
	const [costEstimate, setCostEstimate] = useState<string>("");
	const apiDomain = getApiDomainUrl(config);
	const additionalDomain = getAdditionalAlbDomain(config);

	useEffect(() => {
		// Calculate monthly cost estimate for ALB
		const baseCost = 16.43; // Base ALB cost per month (730 hours * $0.0225)
		const lcuCost = 5.84; // Estimated LCU cost per month
		const totalCost = baseCost + lcuCost;
		setCostEstimate(`$${totalCost.toFixed(2)}/month`);
	}, []);

	useEffect(() => {
		const value = String(configuredIdleTimeout);
		setIdleTimeoutInput(value);
		setIdleTimeoutError(
			parseAlbIdleTimeout(value) === null ? idleTimeoutValidationMessage : "",
		);
	}, [configuredIdleTimeout, idleTimeoutValidationMessage]);

	const handleALBToggle = (enabled: boolean) => {
		onConfigChange({
			alb: {
				...config.alb,
				enabled,
			},
		});
	};

	const commitIdleTimeout = () => {
		const seconds = parseAlbIdleTimeout(idleTimeoutInput);
		if (seconds === null) {
			setIdleTimeoutError(idleTimeoutValidationMessage);
			return;
		}

		setIdleTimeoutError("");
		setIdleTimeoutInput(String(seconds));
		if (seconds === configuredIdleTimeout) {
			return;
		}

		onConfigChange({
			alb: {
				...config.alb,
				enabled: isALBEnabled,
				idle_timeout: seconds,
			},
		});
	};

	const getHealthCheckUrl = () => {
		const hostname = apiDomain || additionalDomain;
		if (!hostname) {
			return "";
		}

		const healthEndpoint =
			config.workload?.backend_health_endpoint || "/health";
		const normalizedEndpoint = healthEndpoint.startsWith("/")
			? healthEndpoint
			: `/${healthEndpoint}`;
		return `https://${hostname}${normalizedEndpoint}`;
	};
	const healthCheckUrl = getHealthCheckUrl();

	return (
		<div className="space-y-4">
			{/* ALB Enable/Disable */}
			<Card>
				<CardHeader>
					<CardTitle className="flex items-center gap-2">
						<Network className="w-5 h-5" />
						Application Load Balancer Settings
					</CardTitle>
				</CardHeader>
				<CardContent className="space-y-4">
					<div className="flex items-center justify-between">
						<div className="space-y-1">
							<Label htmlFor="alb-enabled">Enable ALB</Label>
							<p className="text-sm text-gray-400">
								Use Application Load Balancer instead of API Gateway for
								incoming traffic
							</p>
						</div>
						<Switch
							id="alb-enabled"
							checked={isALBEnabled}
							onCheckedChange={handleALBToggle}
						/>
					</div>

					{isALBEnabled && (
						<div className="pl-4 pt-2 border-l-2 border-blue-500 space-y-3">
							<div className="text-sm text-gray-400">
								<AlertCircle className="w-4 h-4 inline mr-1 text-yellow-500" />
								API Gateway is disabled automatically. The ALB serves the same
								API domain, so your public URL does not change.
							</div>

							<div className="space-y-1">
								<Label htmlFor="alb-idle-timeout">Idle timeout (seconds)</Label>
								<Input
									id="alb-idle-timeout"
									type="number"
									min={MIN_ALB_IDLE_TIMEOUT}
									max={MAX_ALB_IDLE_TIMEOUT}
									step={1}
									value={idleTimeoutInput}
									onChange={(event) => {
										setIdleTimeoutInput(event.target.value);
										setIdleTimeoutError("");
									}}
									onBlur={commitIdleTimeout}
									onKeyDown={(event) => {
										if (event.key === "Enter") {
											event.currentTarget.blur();
										}
									}}
									aria-invalid={Boolean(idleTimeoutError)}
									aria-describedby="alb-idle-timeout-description alb-idle-timeout-error"
								/>
								<p
									id="alb-idle-timeout-description"
									className="text-sm text-gray-400"
								>
									The ALB closes a connection after this long with no bytes
									flowing. For SSE / streaming, set it above your app's
									heartbeat interval (300 is a common choice). API Gateway
									cannot stream at all — its 30s cap is fixed — so this is the
									reason to use an ALB.
								</p>
								{idleTimeoutError && (
									<p
										id="alb-idle-timeout-error"
										className="text-sm text-red-400"
										role="alert"
									>
										{idleTimeoutError}
									</p>
								)}
							</div>

							{!apiDomain && (
								<div className="text-sm text-yellow-400">
									<AlertCircle className="w-4 h-4 inline mr-1" />
									Enable the domain module and set a domain name before
									deploying the ALB.
								</div>
							)}
						</div>
					)}
				</CardContent>
			</Card>

			{/* ALB Configuration Details */}
			{isALBEnabled && (
				<>
					{/* Network Configuration */}
					<Card>
						<CardHeader>
							<CardTitle className="flex items-center gap-2">
								<Shield className="w-5 h-5" />
								Network Configuration
							</CardTitle>
						</CardHeader>
						<CardContent className="space-y-4">
							<div className="grid grid-cols-2 gap-4 text-sm">
								<div>
									<span className="text-gray-400">Type:</span>
									<p className="font-mono text-white">Application</p>
								</div>
								<div>
									<span className="text-gray-400">Scheme:</span>
									<p className="font-mono text-white">Internet-facing</p>
								</div>
								<div>
									<span className="text-gray-400">IP Address Type:</span>
									<p className="font-mono text-white">IPv4</p>
								</div>
								<div>
									<span className="text-gray-400">Availability Zones:</span>
									<p className="font-mono text-white">Multi-AZ</p>
								</div>
							</div>

							<div className="pt-3 border-t border-gray-700">
								<span className="text-gray-400 text-sm">Security Groups:</span>
								<div className="mt-2 space-y-1">
									<div className="flex items-center gap-2 text-sm">
										<div className="w-2 h-2 bg-green-500 rounded-full"></div>
										<span className="font-mono">
											HTTPS (443) from 0.0.0.0/0
										</span>
									</div>
									<div className="flex items-center gap-2 text-sm">
										<div className="w-2 h-2 bg-green-500 rounded-full"></div>
										<span className="font-mono">HTTP (80) from 0.0.0.0/0</span>
									</div>
								</div>
							</div>
						</CardContent>
					</Card>

					{/* Target Groups */}
					<Card>
						<CardHeader>
							<CardTitle className="flex items-center gap-2">
								<Activity className="w-5 h-5" />
								Target Groups
							</CardTitle>
						</CardHeader>
						<CardContent className="space-y-4">
							<div className="space-y-3">
								<div className="bg-gray-800 rounded-lg p-3">
									<div className="flex items-center justify-between mb-2">
										<span className="font-medium text-white">
											Backend Service
										</span>
									</div>
									<div className="text-sm text-gray-400 space-y-1">
										<div>
											Target Type: <span className="text-white">IP</span>
										</div>
										<div>
											Protocol: <span className="text-white">HTTP</span>
										</div>
										<div>
											Port:{" "}
											<span className="text-white">
												{config.workload?.backend_image_port || 8080}
											</span>
										</div>
										<div>
											Health Check:{" "}
											<span className="text-white font-mono">
												{config.workload?.backend_health_endpoint || "/health"}
											</span>
										</div>
									</div>
								</div>

								{config.services?.map((service) => (
									<div
										key={service.name}
										className="bg-gray-800 rounded-lg p-3"
									>
										<div className="flex items-center justify-between mb-2">
											<span className="font-medium text-white">
												{service.name}
											</span>
										</div>
										<div className="text-sm text-gray-400 space-y-1">
											<div>
												Target Type: <span className="text-white">IP</span>
											</div>
											<div>
												Protocol: <span className="text-white">HTTP</span>
											</div>
											<div>
												Port:{" "}
												<span className="text-white">
													{service.container_port || 8080}
												</span>
											</div>
											<div>
												Desired Count:{" "}
												<span className="text-white">
													{service.desired_count ?? 1}
												</span>
											</div>
										</div>
									</div>
								))}
							</div>
						</CardContent>
					</Card>

					{/* Listener Rules */}
					<Card>
						<CardHeader>
							<CardTitle className="flex items-center gap-2">
								<Globe className="w-5 h-5" />
								Listener Rules
							</CardTitle>
						</CardHeader>
						<CardContent>
							<div className="space-y-3">
								<div className="bg-gray-800 rounded-lg p-3">
									<div className="font-medium text-white mb-2">HTTPS:443</div>
									<div className="text-sm text-gray-400 space-y-2">
										{config.services?.map((service) => (
											<div
												key={service.name}
												className="pl-4 border-l-2 border-green-500"
											>
												<div className="font-mono">
													/{service.name}/* → {service.name} Target Group
												</div>
											</div>
										))}
										<div className="pl-4 border-l-2 border-blue-500">
											<div className="font-mono">
												Default Action → Backend Target Group
											</div>
											<div className="text-xs mt-1">
												SSL Certificate: ACM (Auto-managed)
											</div>
										</div>
									</div>
								</div>

								<div className="bg-gray-800 rounded-lg p-3">
									<div className="font-medium text-white mb-2">HTTP:80</div>
									<div className="text-sm text-gray-400 space-y-2">
										<div className="pl-4 border-l-2 border-orange-500">
											<div className="font-mono">Redirect to HTTPS</div>
											<div className="text-xs mt-1">
												Status Code: 301 (Permanent)
											</div>
										</div>
									</div>
								</div>
							</div>
						</CardContent>
					</Card>

					{/* Domain & SSL */}
					<Card>
						<CardHeader>
							<CardTitle className="flex items-center gap-2">
								<Shield className="w-5 h-5" />
								Domain & SSL Configuration
							</CardTitle>
						</CardHeader>
						<CardContent className="space-y-4">
							<div>
								<Label>API Domain (served by the ALB)</Label>
								<div className="mt-1 font-mono text-sm bg-gray-800 p-2 rounded">
									{apiDomain || "Domain not configured"}
								</div>
								<p className="text-sm text-gray-400 mt-1">
									The same hostname API Gateway serves — switching ingress does
									not change your public URL.
								</p>
							</div>

							<div>
								<Label>Additional hostname (optional)</Label>
								<div className="mt-1 font-mono text-sm bg-gray-800 p-2 rounded">
									{additionalDomain || "Not configured"}
								</div>
							</div>

							{healthCheckUrl && (
								<div className="space-y-3">
									<div className="flex items-center gap-2 text-sm">
										<div className="w-2 h-2 bg-green-500 rounded-full animate-pulse"></div>
										<span className="text-gray-400">
											SSL Certificate provisioned via AWS Certificate Manager
										</span>
									</div>

									<div>
										<Label>Health Check Endpoint</Label>
										<div className="mt-1 flex items-center gap-2">
											<code className="text-xs bg-gray-800 px-2 py-1 rounded flex-1">
												{healthCheckUrl}
											</code>
											<Button
												size="sm"
												variant="outline"
												onClick={() => window.open(healthCheckUrl, "_blank")}
											>
												Test
											</Button>
										</div>
									</div>
								</div>
							)}
						</CardContent>
					</Card>

					{/* Cost Estimation */}
					<Card>
						<CardHeader>
							<CardTitle className="flex items-center gap-2">
								<DollarSign className="w-5 h-5" />
								Cost Estimation
							</CardTitle>
						</CardHeader>
						<CardContent>
							<div className="space-y-3">
								<div className="flex justify-between items-center">
									<span className="text-gray-400">Base ALB Cost (24/7)</span>
									<span className="font-mono text-white">$16.43/month</span>
								</div>
								<div className="flex justify-between items-center">
									<span className="text-gray-400">
										Load Balancer Capacity Units (Est.)
									</span>
									<span className="font-mono text-white">$5.84/month</span>
								</div>
								<div className="border-t border-gray-700 pt-2">
									<div className="flex justify-between items-center">
										<span className="font-medium">Total Estimated Cost</span>
										<span className="font-mono text-green-400 font-medium">
											{costEstimate}
										</span>
									</div>
								</div>
								<div className="text-xs text-gray-500 mt-2">
									* Actual costs may vary based on traffic patterns and data
									transfer
								</div>
							</div>
						</CardContent>
					</Card>

					{/* ALB vs API Gateway Comparison */}
					<Card>
						<CardHeader>
							<CardTitle>ALB vs API Gateway Comparison</CardTitle>
						</CardHeader>
						<CardContent>
							<div className="space-y-4 text-sm">
								<div className="grid grid-cols-2 gap-4">
									<div>
										<h4 className="font-medium text-blue-400 mb-2">
											ALB Advantages
										</h4>
										<ul className="space-y-1 text-gray-400">
											<li>• Lower cost for high traffic</li>
											<li>• WebSocket support</li>
											<li>• Layer 7 load balancing</li>
											<li>• Direct ECS integration</li>
										</ul>
									</div>
									<div>
										<h4 className="font-medium text-orange-400 mb-2">
											API Gateway Advantages
										</h4>
										<ul className="space-y-1 text-gray-400">
											<li>• Built-in request validation</li>
											<li>• API versioning</li>
											<li>• Request/response transformation</li>
											<li>• Usage plans & API keys</li>
										</ul>
									</div>
								</div>
							</div>
						</CardContent>
					</Card>
				</>
			)}
		</div>
	);
}
