import { AlertCircle, Globe, Mail, Server, Shield } from "lucide-react";
import type { YamlInfrastructureConfig } from "../types/yamlConfig";
import {
	Card,
	CardContent,
	CardDescription,
	CardHeader,
	CardTitle,
} from "./ui/card";

interface Route53DNSRecordsProps {
	config: YamlInfrastructureConfig;
}

export function Route53DNSRecords({ config }: Route53DNSRecordsProps) {
	// Check if domain is enabled
	if (!config.domain?.enabled || !config.domain?.domain_name) {
		return (
			<div className="flex items-center justify-center py-8">
				<div className="text-center space-y-2">
					<AlertCircle className="w-8 h-8 text-gray-500 mx-auto" />
					<p className="text-sm text-gray-400">
						Domain configuration is not enabled
					</p>
				</div>
			</div>
		);
	}

	const domainName = config.domain.domain_name;
	const apiPrefix = config.domain.api_domain_prefix || "api";
	const addPrefix = config.domain.add_env_domain_prefix ?? true;

	// Calculate the full domain based on settings
	const fullDomain =
		addPrefix && !config.is_prod ? `${config.env}.${domainName}` : domainName;

	// Check which services are enabled
	const isALBEnabled = config.alb?.enabled !== false; // ALB is enabled by default
	const isAPIGatewayEnabled = true; // API Gateway is always enabled
	const isSESEnabled = config.ses?.enabled === true;
	const sesDomain = config.ses?.domain_name || `mail.${domainName}`;

	const dnsRecords = [];

	// ALB A Record
	if (isALBEnabled) {
		dnsRecords.push({
			type: "A",
			name: fullDomain,
			value: "Application Load Balancer (ALB)",
			icon: Server,
			description: "Routes traffic to your application load balancer",
			color: "text-blue-400",
			bgColor: "bg-blue-900/20",
			borderColor: "border-blue-700",
		});
	}

	// API Gateway CNAME
	if (isAPIGatewayEnabled) {
		dnsRecords.push({
			type: "CNAME",
			name: `${apiPrefix}.${fullDomain}`,
			value: "API Gateway endpoint",
			icon: Globe,
			description: "Routes API traffic to AWS API Gateway",
			color: "text-purple-400",
			bgColor: "bg-purple-900/20",
			borderColor: "border-purple-700",
		});
	}

	// SES MX Records
	if (isSESEnabled) {
		dnsRecords.push({
			type: "MX",
			name: sesDomain,
			value: "AWS SES mail servers",
			icon: Mail,
			description: "Email routing for AWS Simple Email Service",
			color: "text-green-400",
			bgColor: "bg-green-900/20",
			borderColor: "border-green-700",
		});
	}

	// Certificate Validation Records
	dnsRecords.push({
		type: "CNAME",
		name: `_acme-challenge.${fullDomain}`,
		value: "ACM certificate validation",
		icon: Shield,
		description: "SSL/TLS certificate validation records",
		color: "text-yellow-400",
		bgColor: "bg-yellow-900/20",
		borderColor: "border-yellow-700",
	});

	return (
		<div className="space-y-4">
			<Card>
				<CardHeader>
					<CardTitle>DNS Records</CardTitle>
					<CardDescription>
						DNS records that will be created in Route 53 based on your
						configuration
					</CardDescription>
				</CardHeader>
				<CardContent className="space-y-4">
					{dnsRecords.map((record, index) => {
						const Icon = record.icon;
						return (
							<div
								key={index}
								className={`${record.bgColor} border ${record.borderColor} rounded-lg p-4`}
							>
								<div className="flex items-start gap-3">
									<Icon className={`w-5 h-5 ${record.color} mt-0.5`} />
									<div className="flex-1 space-y-2">
										<div className="flex items-center gap-2">
											<span className={`text-sm font-medium ${record.color}`}>
												{record.type} Record
											</span>
											<span className="text-xs text-gray-500">•</span>
											<code className="text-xs text-gray-300">
												{record.name}
											</code>
										</div>
										<p className="text-xs text-gray-400">
											{record.description}
										</p>
										<div className="text-xs text-gray-500">
											Points to:{" "}
											<span className="text-gray-300">{record.value}</span>
										</div>
									</div>
								</div>
							</div>
						);
					})}

					{/* Zone Information */}
					<div className="mt-6 bg-gray-800 rounded-lg p-4">
						<h4 className="text-sm font-medium text-gray-300 mb-3">
							Hosted Zone Details
						</h4>
						<div className="space-y-2 text-xs">
							<div className="flex items-center justify-between">
								<span className="text-gray-400">Zone Name:</span>
								<code className="text-gray-300">{domainName}</code>
							</div>
							<div className="flex items-center justify-between">
								<span className="text-gray-400">Type:</span>
								<span className="text-gray-300">Public Hosted Zone</span>
							</div>
							<div className="flex items-center justify-between">
								<span className="text-gray-400">Record Count:</span>
								<span className="text-gray-300">
									{dnsRecords.length} records
								</span>
							</div>
						</div>
					</div>

					{/* Nameserver Instructions */}
					{config.domain.create_domain_zone && (
						<div className="bg-yellow-900/20 border border-yellow-700 rounded-lg p-4">
							<div className="flex items-start gap-2">
								<AlertCircle className="w-4 h-4 text-yellow-400 mt-0.5" />
								<div className="flex-1">
									<h4 className="text-sm font-medium text-yellow-400 mb-2">
										Action Required
									</h4>
									<p className="text-xs text-gray-300">
										After creating the hosted zone, update your domain
										registrar's nameservers to point to the Route 53
										nameservers. This is required for DNS resolution to work.
									</p>
								</div>
							</div>
						</div>
					)}
				</CardContent>
			</Card>
		</div>
	);
}
