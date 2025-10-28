import {
	Activity,
	Clock,
	FileText,
	Globe,
	Link,
	Route,
	Shield,
	Zap,
	ExternalLink,
	Copy,
	Check,
	Server,
	GitBranch,
} from "lucide-react";
import { useEffect, useState } from "react";
import { infrastructureApi } from "../api/infrastructure";
import type { YamlInfrastructureConfig } from "../types/yamlConfig";
import { Card, CardContent, CardHeader, CardTitle } from "./ui/card";
import { Label } from "./ui/label";

interface APIGatewayPropertiesProps {
	config: YamlInfrastructureConfig;
}

interface APIGatewayInfo {
	defaultEndpoint: string;
	apiGatewayId: string;
	customDomainEnabled: boolean;
	customDomain?: string;
	region: string;
	error?: string;
}

export function APIGatewayProperties({ config }: APIGatewayPropertiesProps) {
	const apiName = `${config.project}-${config.env}`;
	const logGroup = `/aws/api_gateway/${apiName}`;
	const [apiGatewayInfo, setApiGatewayInfo] = useState<APIGatewayInfo | null>(
		null,
	);
	const [loading, setLoading] = useState(true);
	const [copiedEndpoint, setCopiedEndpoint] = useState(false);

	useEffect(() => {
		const fetchAPIGatewayInfo = async () => {
			try {
				const data = await infrastructureApi.getAPIGatewayInfo(config.env);
				setApiGatewayInfo(data);
			} catch (error) {
				console.error("Failed to fetch API Gateway info:", error);
			} finally {
				setLoading(false);
			}
		};

		fetchAPIGatewayInfo();
	}, [config.env]);

	const copyToClipboard = async (text: string) => {
		await navigator.clipboard.writeText(text);
		setCopiedEndpoint(true);
		setTimeout(() => setCopiedEndpoint(false), 2000);
	};

	return (
		<div className="space-y-4">
			{/* Summary Card */}
			<Card className="bg-gradient-to-br from-gray-800 to-gray-850 border-gray-700 shadow-lg">
				<CardContent className="pt-6">
					<div className="flex items-start justify-between mb-4">
						<div>
							<div className="flex items-center gap-2 mb-1">
								<Server className="w-5 h-5 text-blue-400" />
								<h3 className="text-lg font-semibold text-white">{apiName}</h3>
							</div>
							<div className="flex items-center gap-2 text-sm text-gray-400">
								<GitBranch className="w-3.5 h-3.5" />
								<span>HTTP API Gateway</span>
								<span className="text-gray-600">•</span>
								<span className="capitalize">{config.env} Stage</span>
							</div>
						</div>
						<div className="flex items-center gap-2">
							<span className="px-2.5 py-1 bg-blue-500/10 text-blue-400 text-xs font-medium rounded-full border border-blue-500/20">
								Regional
							</span>
							<span className="px-2.5 py-1 bg-green-500/10 text-green-400 text-xs font-medium rounded-full border border-green-500/20 flex items-center gap-1">
								<Activity className="w-3 h-3" />
								Active
							</span>
						</div>
					</div>

					{/* Endpoint Section */}
					{apiGatewayInfo && !loading && !apiGatewayInfo.error && (
						<div className="mt-4 p-4 bg-gray-900/50 rounded-lg border border-gray-700">
							<div className="flex items-center justify-between mb-2">
								<Label className="text-xs font-semibold text-gray-300 uppercase tracking-wider">
									Default Endpoint
								</Label>
								<button
									type="button"
									onClick={() => copyToClipboard(apiGatewayInfo.defaultEndpoint)}
									className="flex items-center gap-1.5 px-2 py-1 text-xs text-gray-400 hover:text-white transition-colors rounded hover:bg-gray-800"
									title="Copy to clipboard"
								>
									{copiedEndpoint ? (
										<>
											<Check className="w-3.5 h-3.5 text-green-400" />
											<span className="text-green-400">Copied!</span>
										</>
									) : (
										<>
											<Copy className="w-3.5 h-3.5" />
											<span>Copy</span>
										</>
									)}
								</button>
							</div>
							<a
								href={apiGatewayInfo.defaultEndpoint}
								target="_blank"
								rel="noopener noreferrer"
								className="text-sm text-blue-400 hover:text-blue-300 font-mono flex items-center gap-2 break-all transition-colors group"
							>
								<ExternalLink className="w-4 h-4 flex-shrink-0 group-hover:scale-110 transition-transform" />
								<span className="group-hover:underline">{apiGatewayInfo.defaultEndpoint}</span>
							</a>
						</div>
					)}

					{apiGatewayInfo?.error && (
						<div className="mt-4 p-4 bg-yellow-500/10 rounded-lg border border-yellow-500/20">
							<div className="flex items-center gap-2 text-yellow-400 text-sm">
								<Clock className="w-4 h-4" />
								<span>{apiGatewayInfo.error}</span>
							</div>
						</div>
					)}
				</CardContent>
			</Card>

			{/* Info Grid */}
			<div className="grid grid-cols-2 gap-4">
				<Card className="bg-gray-800 border-gray-700">
					<CardContent className="pt-6">
						<div className="flex items-center gap-3">
							<div className="p-2.5 bg-purple-500/10 rounded-lg border border-purple-500/20">
								<Globe className="w-5 h-5 text-purple-400" />
							</div>
							<div>
								<div className="text-xs text-gray-400 mb-0.5">Protocol</div>
								<div className="text-sm font-semibold text-white">HTTP API</div>
							</div>
						</div>
					</CardContent>
				</Card>

				<Card className="bg-gray-800 border-gray-700">
					<CardContent className="pt-6">
						<div className="flex items-center gap-3">
							<div className="p-2.5 bg-blue-500/10 rounded-lg border border-blue-500/20">
								<GitBranch className="w-5 h-5 text-blue-400" />
							</div>
							<div>
								<div className="text-xs text-gray-400 mb-0.5">Stage</div>
								<div className="text-sm font-semibold text-white capitalize">{config.env}</div>
							</div>
						</div>
					</CardContent>
				</Card>
			</div>

			{/* Domain Mapping - Only show if custom domain is enabled */}
			{apiGatewayInfo?.customDomainEnabled && !loading && (
				<Card className="bg-gray-800 border-gray-700">
					<CardHeader className="pb-3">
						<CardTitle className="text-sm font-medium text-gray-300 flex items-center gap-2">
							<Link className="w-4 h-4 text-cyan-400" />
							Custom Domain Mapping
						</CardTitle>
					</CardHeader>
					<CardContent className="space-y-4">
						<div className="p-3 bg-gray-900/50 rounded-lg">
							<Label className="text-xs text-gray-400 mb-1 block">Domain</Label>
							<div className="text-sm text-white font-mono">
								{apiGatewayInfo.customDomain ||
									config.api_domain ||
									`api.${config.project}.com`}
							</div>
						</div>
						<div className="flex items-center gap-2 p-3 bg-green-500/5 rounded-lg border border-green-500/20">
							<Shield className="w-4 h-4 text-green-400" />
							<div className="flex-1">
								<div className="text-xs text-gray-400">Security</div>
								<div className="text-sm text-white">TLS 1.2 with ACM certificate</div>
							</div>
						</div>
						<div className="flex items-center gap-2 p-3 bg-blue-500/5 rounded-lg border border-blue-500/20">
							<Globe className="w-4 h-4 text-blue-400" />
							<div className="flex-1">
								<div className="text-xs text-gray-400">DNS</div>
								<div className="text-sm text-white">Route 53 A record → API Gateway</div>
							</div>
						</div>
					</CardContent>
				</Card>
			)}

			{/* Stage Configuration */}
			<Card className="bg-gray-800 border-gray-700">
				<CardHeader className="pb-3">
					<CardTitle className="text-sm font-medium text-gray-300 flex items-center gap-2">
						<Zap className="w-4 h-4 text-yellow-400" />
						Performance & Throttling
					</CardTitle>
				</CardHeader>
				<CardContent className="space-y-3">
					<div className="flex items-center gap-2 p-3 bg-green-500/5 rounded-lg border border-green-500/20">
						<Activity className="w-4 h-4 text-green-400" />
						<div className="flex-1">
							<div className="text-xs text-gray-400">Auto-Deploy</div>
							<div className="text-sm font-medium text-green-400">Enabled</div>
						</div>
					</div>
					<div className="grid grid-cols-2 gap-3">
						<div className="p-3 bg-gray-900/50 rounded-lg border border-gray-700">
							<div className="text-xs text-gray-400 mb-1">Burst Limit</div>
							<div className="text-lg font-bold text-white font-mono">5,000</div>
							<div className="text-xs text-gray-500">requests</div>
						</div>
						<div className="p-3 bg-gray-900/50 rounded-lg border border-gray-700">
							<div className="text-xs text-gray-400 mb-1">Rate Limit</div>
							<div className="text-lg font-bold text-white font-mono">10,000</div>
							<div className="text-xs text-gray-500">req/sec</div>
						</div>
					</div>
				</CardContent>
			</Card>

			{/* Logging */}
			<Card className="bg-gray-800 border-gray-700">
				<CardHeader className="pb-3">
					<CardTitle className="text-sm font-medium text-gray-300 flex items-center gap-2">
						<FileText className="w-4 h-4 text-orange-400" />
						CloudWatch Logging
					</CardTitle>
				</CardHeader>
				<CardContent className="space-y-3">
					<div className="p-3 bg-gray-900/50 rounded-lg border border-gray-700">
						<Label className="text-xs text-gray-400 mb-2 block">Log Group</Label>
						<div className="text-xs text-white font-mono break-all bg-gray-950 p-2 rounded">
							{logGroup}
						</div>
					</div>
					<div className="grid grid-cols-2 gap-3">
						<div className="flex items-center gap-2 p-3 bg-gray-900/50 rounded-lg border border-gray-700">
							<Clock className="w-4 h-4 text-gray-400" />
							<div>
								<div className="text-xs text-gray-400">Retention</div>
								<div className="text-sm font-medium text-white">30 days</div>
							</div>
						</div>
						<div className="flex items-center gap-2 p-3 bg-gray-900/50 rounded-lg border border-gray-700">
							<FileText className="w-4 h-4 text-gray-400" />
							<div>
								<div className="text-xs text-gray-400">Format</div>
								<div className="text-sm font-medium text-white">JSON</div>
							</div>
						</div>
					</div>
				</CardContent>
			</Card>

			{/* Routes */}
			<Card className="bg-gray-800 border-gray-700">
				<CardHeader className="pb-3">
					<CardTitle className="text-sm font-medium text-gray-300 flex items-center gap-2">
						<Route className="w-4 h-4 text-green-400" />
						API Routes
					</CardTitle>
				</CardHeader>
				<CardContent className="space-y-3">
					<div className="space-y-2">
						<div className="p-3 bg-gradient-to-r from-blue-500/10 to-blue-500/5 rounded-lg border border-blue-500/20">
							<div className="flex items-center gap-2 mb-2">
								<div className="px-2 py-0.5 bg-blue-500/20 text-blue-400 text-xs font-bold rounded">
									ANY
								</div>
								<span className="text-xs text-gray-400">Backend Route</span>
							</div>
							<div className="font-mono text-xs text-white bg-gray-950 p-2 rounded">
								/{"{proxy+}"} → Backend service
							</div>
						</div>

						{config.services && config.services.length > 0 && (
							<div className="space-y-2">
								<div className="text-xs text-gray-400 font-medium mt-3 mb-2">
									Service Routes
								</div>
								{config.services.map((service) => (
									<div
										key={service.name}
										className="p-3 bg-gradient-to-r from-purple-500/10 to-purple-500/5 rounded-lg border border-purple-500/20"
									>
										<div className="flex items-center gap-2 mb-2">
											<div className="px-2 py-0.5 bg-purple-500/20 text-purple-400 text-xs font-bold rounded">
												ANY
											</div>
											<span className="text-xs text-purple-300 font-medium">
												{service.name}
											</span>
										</div>
										<div className="font-mono text-xs text-white bg-gray-950 p-2 rounded">
											/{service.name}/{"{proxy+}"} → {service.name} service
										</div>
									</div>
								))}
							</div>
						)}
					</div>
				</CardContent>
			</Card>
		</div>
	);
}
