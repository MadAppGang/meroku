import {
	Activity,
	CheckCircle,
	Info,
	Network,
	Server,
	Settings,
	XCircle,
} from "lucide-react";
import type { YamlInfrastructureConfig } from "../types/yamlConfig";
import {
	Card,
	CardContent,
	CardDescription,
	CardHeader,
	CardTitle,
} from "./ui/card";
import { Label } from "./ui/label";
import { Switch } from "./ui/switch";

interface BackendXRayConfigurationProps {
	config: YamlInfrastructureConfig;
	onConfigChange?: (config: Partial<YamlInfrastructureConfig>) => void;
}

export function BackendXRayConfiguration({
	config,
	onConfigChange,
}: BackendXRayConfigurationProps) {
	const xrayEnabled = config.workload?.xray_enabled || false;
	const logGroupName = `${config.project}_adot_collector_${config.env}`;

	const handleXRayToggle = (checked: boolean) => {
		if (onConfigChange) {
			onConfigChange({
				workload: {
					...config.workload,
					xray_enabled: checked,
				},
			});
		}
	};

	return (
		<div className="space-y-6">
			{/* X-Ray Configuration */}
			<Card>
				<CardHeader>
					<CardTitle>X-Ray Tracing Configuration</CardTitle>
					<CardDescription>
						Enable distributed tracing for debugging and performance analysis
					</CardDescription>
				</CardHeader>
				<CardContent className="space-y-4">
					<div className="flex items-center justify-between">
						<div className="flex-1">
							<Label htmlFor="enable-xray">Enable AWS X-Ray Tracing</Label>
							<p className="text-xs text-gray-500 mt-1">
								Adds ADOT collector sidecar to capture and forward traces
							</p>
						</div>
						<Switch
							id="enable-xray"
							checked={xrayEnabled}
							onCheckedChange={handleXRayToggle}
							disabled={!onConfigChange}
							className="data-[state=checked]:bg-blue-500 data-[state=unchecked]:bg-gray-600"
						/>
					</div>

					<div
						className={`p-4 rounded-lg ${xrayEnabled ? "bg-green-900/20 border border-green-700" : "bg-gray-800"}`}
					>
						<div className="flex items-start gap-2">
							{xrayEnabled ? (
								<CheckCircle className="w-4 h-4 text-green-400 mt-0.5" />
							) : (
								<XCircle className="w-4 h-4 text-gray-400 mt-0.5" />
							)}
							<div className="flex-1">
								<p className="text-sm text-gray-300">
									{xrayEnabled
										? "X-Ray tracing is active. The ADOT collector sidecar is running alongside your backend service."
										: "X-Ray tracing is inactive. Enable it to start collecting distributed traces."}
								</p>
							</div>
						</div>
					</div>
				</CardContent>
			</Card>

			{xrayEnabled && (
				<>
					{/* Sidecar Container Details */}
					<Card>
						<CardHeader>
							<CardTitle className="flex items-center gap-2">
								<Server className="w-5 h-5 text-blue-400" />
								Sidecar Container Details
							</CardTitle>
						</CardHeader>
						<CardContent className="space-y-4">
							<div className="grid grid-cols-1 gap-4">
								<div>
									<label className="text-xs text-gray-400">
										Container Name
									</label>
									<p className="text-sm font-mono text-gray-300">
										adot-collector
									</p>
								</div>

								<div>
									<label className="text-xs text-gray-400">Image</label>
									<p className="text-sm font-mono text-gray-300 break-all">
										public.ecr.aws/aws-observability/aws-otel-collector:latest
									</p>
								</div>

								<div>
									<label className="text-xs text-gray-400">Configuration</label>
									<p className="text-sm font-mono text-gray-300">
										--config=/etc/ecs/container-insights/otel-task-metrics-config.yaml
									</p>
								</div>

								<div>
									<label className="text-xs text-gray-400">Log Group</label>
									<p className="text-sm font-mono text-gray-300">
										{logGroupName}
									</p>
								</div>
							</div>
						</CardContent>
					</Card>

					{/* Network Configuration */}
					<Card>
						<CardHeader>
							<CardTitle className="flex items-center gap-2">
								<Network className="w-5 h-5 text-purple-400" />
								Port Mappings
							</CardTitle>
						</CardHeader>
						<CardContent>
							<div className="space-y-3">
								<div className="grid grid-cols-3 gap-4 text-xs">
									<div>
										<p className="text-gray-400 mb-1">Port</p>
									</div>
									<div>
										<p className="text-gray-400 mb-1">Protocol</p>
									</div>
									<div>
										<p className="text-gray-400 mb-1">Description</p>
									</div>
								</div>

								<div className="space-y-2">
									<div className="grid grid-cols-3 gap-4 p-2 bg-gray-800 rounded text-xs">
										<p className="font-mono text-gray-300">2000</p>
										<p className="text-gray-300">UDP</p>
										<p className="text-gray-300">X-Ray daemon endpoint</p>
									</div>

									<div className="grid grid-cols-3 gap-4 p-2 bg-gray-800 rounded text-xs">
										<p className="font-mono text-gray-300">4317</p>
										<p className="text-gray-300">TCP</p>
										<p className="text-gray-300">OpenTelemetry gRPC endpoint</p>
									</div>

									<div className="grid grid-cols-3 gap-4 p-2 bg-gray-800 rounded text-xs">
										<p className="font-mono text-gray-300">4318</p>
										<p className="text-gray-300">TCP</p>
										<p className="text-gray-300">OpenTelemetry HTTP endpoint</p>
									</div>

									<div className="grid grid-cols-3 gap-4 p-2 bg-gray-800 rounded text-xs">
										<p className="font-mono text-gray-300">55681</p>
										<p className="text-gray-300">TCP</p>
										<p className="text-gray-300">
											Legacy Jaeger/Zipkin endpoint
										</p>
									</div>
								</div>
							</div>

							<div className="mt-4 p-3 bg-purple-900/20 border border-purple-700 rounded-lg">
								<div className="flex items-start gap-2">
									<Info className="w-4 h-4 text-purple-400 mt-0.5" />
									<div className="flex-1">
										<p className="text-xs text-gray-300">
											The sidecar runs in the same network namespace as your
											backend container (awsvpc mode). Communication happens via
											localhost.
										</p>
									</div>
								</div>
							</div>
						</CardContent>
					</Card>

					{/* Resource Allocation */}
					<Card>
						<CardHeader>
							<CardTitle className="flex items-center gap-2">
								<Settings className="w-5 h-5 text-green-400" />
								ADOT Collector Resources
							</CardTitle>
						</CardHeader>
						<CardContent>
							<div className="grid grid-cols-2 gap-4">
								<div className="bg-gray-800 rounded-lg p-4">
									<div className="flex items-center justify-between mb-2">
										<p className="text-xs text-gray-400">CPU</p>
										<Activity className="w-4 h-4 text-green-400" />
									</div>
									<p className="text-2xl font-medium text-gray-300">256</p>
									<p className="text-xs text-gray-500 mt-1">vCPU units</p>
									<p className="text-xs text-green-400 mt-2">ADOT sidecar</p>
								</div>

								<div className="bg-gray-800 rounded-lg p-4">
									<div className="flex items-center justify-between mb-2">
										<p className="text-xs text-gray-400">Memory</p>
										<Server className="w-4 h-4 text-green-400" />
									</div>
									<p className="text-2xl font-medium text-gray-300">512</p>
									<p className="text-xs text-gray-500 mt-1">MB</p>
									<p className="text-xs text-green-400 mt-2">ADOT sidecar</p>
								</div>
							</div>

							<div className="mt-4 p-3 bg-green-900/20 border border-green-700 rounded-lg">
								<p className="text-xs text-gray-300">
									The ADOT collector sidecar runs efficiently alongside your
									backend container with minimal resource overhead.
								</p>
							</div>
						</CardContent>
					</Card>

					{/* How It Works */}
					<Card>
						<CardHeader>
							<CardTitle>How X-Ray Tracing Works</CardTitle>
						</CardHeader>
						<CardContent>
							<ol className="space-y-3 text-xs text-gray-300">
								<li className="flex items-start gap-2">
									<span className="text-blue-400 font-medium flex-shrink-0">
										1.
									</span>
									<span>
										ADOT collector sidecar is added to your ECS task definition
									</span>
								</li>
								<li className="flex items-start gap-2">
									<span className="text-blue-400 font-medium flex-shrink-0">
										2.
									</span>
									<span>
										The sidecar listens on{" "}
										<code className="text-blue-300">localhost:2000</code> for
										X-Ray traces
									</span>
								</li>
								<li className="flex items-start gap-2">
									<span className="text-blue-400 font-medium flex-shrink-0">
										3.
									</span>
									<span>
										Your backend application sends traces to{" "}
										<code className="text-blue-300">localhost:2000</code>
									</span>
								</li>
								<li className="flex items-start gap-2">
									<span className="text-blue-400 font-medium flex-shrink-0">
										4.
									</span>
									<span>
										ADOT collector forwards traces to AWS X-Ray service
									</span>
								</li>
								<li className="flex items-start gap-2">
									<span className="text-blue-400 font-medium flex-shrink-0">
										5.
									</span>
									<span>
										View traces in the AWS X-Ray console for debugging and
										performance analysis
									</span>
								</li>
							</ol>

							<div className="mt-4 p-3 bg-blue-900/20 border border-blue-700 rounded-lg">
								<div className="flex items-start gap-2">
									<Info className="w-4 h-4 text-blue-400 mt-0.5" />
									<div className="flex-1">
										<h4 className="text-sm font-medium text-blue-400 mb-1">
											Environment Variable
										</h4>
										<p className="text-xs text-gray-300">
											The{" "}
											<code className="text-blue-300">
												ADOT_COLLECTOR_URL=localhost:2000
											</code>{" "}
											environment variable is automatically added to your
											backend container when X-Ray is enabled.
										</p>
									</div>
								</div>
							</div>
						</CardContent>
					</Card>

					{/* Integration Guide */}
					<Card>
						<CardHeader>
							<CardTitle>Integration Guide</CardTitle>
							<CardDescription>
								How to instrument your application for X-Ray
							</CardDescription>
						</CardHeader>
						<CardContent>
							<div className="space-y-4">
								<div className="p-3 bg-gray-800 rounded-lg">
									<h4 className="text-sm font-medium text-gray-300 mb-2">
										Node.js Example
									</h4>
									<pre className="text-xs text-gray-400 overflow-x-auto">
										{`const AWSXRay = require('aws-xray-sdk-core');
const AWS = AWSXRay.captureAWS(require('aws-sdk'));

// Capture HTTP calls
const http = AWSXRay.captureHTTPs(require('http'));

// The daemon endpoint is already configured
// via ADOT_COLLECTOR_URL env variable`}
									</pre>
								</div>

								<div className="p-3 bg-gray-800 rounded-lg">
									<h4 className="text-sm font-medium text-gray-300 mb-2">
										Python Example
									</h4>
									<pre className="text-xs text-gray-400 overflow-x-auto">
										{`from aws_xray_sdk.core import xray_recorder

# Configure the daemon address
xray_recorder.configure(
    daemon_address='localhost:2000'
)

# Or use the environment variable
# AWS_XRAY_DAEMON_ADDRESS=localhost:2000`}
									</pre>
								</div>

								<div className="p-3 bg-gray-800 rounded-lg">
									<h4 className="text-sm font-medium text-gray-300 mb-2">
										Java Example
									</h4>
									<pre className="text-xs text-gray-400 overflow-x-auto">
										{`// Add to your application
@EnableXRay

// The SDK will automatically use
// localhost:2000 as the daemon endpoint`}
									</pre>
								</div>
							</div>
						</CardContent>
					</Card>
				</>
			)}

			{/* Disabled State Information */}
			{!xrayEnabled && (
				<Card>
					<CardHeader>
						<CardTitle>Getting Started with X-Ray</CardTitle>
					</CardHeader>
					<CardContent>
						<div className="space-y-4">
							<p className="text-sm text-gray-300">
								Enable X-Ray tracing using the toggle above to:
							</p>

							<ul className="space-y-2 text-sm text-gray-300">
								<li>• Trace requests as they travel through your services</li>
								<li>• Identify performance bottlenecks</li>
								<li>• Debug distributed applications</li>
								<li>• Analyze service dependencies</li>
							</ul>

							<div className="bg-blue-900/20 border border-blue-700 rounded-lg p-4">
								<div className="flex items-start gap-2">
									<Info className="w-4 h-4 text-blue-400 mt-0.5" />
									<div className="flex-1">
										<h4 className="text-sm font-medium text-blue-400 mb-1">
											No Additional Resource Cost
										</h4>
										<p className="text-xs text-gray-300">
											X-Ray tracing runs efficiently within your existing
											container resources. The ADOT collector sidecar has
											minimal overhead.
										</p>
									</div>
								</div>
							</div>
						</div>
					</CardContent>
				</Card>
			)}
		</div>
	);
}
