import { Globe, Info, Shield } from "lucide-react";
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
import { Switch } from "./ui/switch";

interface Route53NodePropertiesProps {
	config: YamlInfrastructureConfig;
	onConfigChange: (config: Partial<YamlInfrastructureConfig>) => void;
}

export function Route53NodeProperties({
	config,
	onConfigChange,
}: Route53NodePropertiesProps) {
	const handleDomainEnabledChange = (checked: boolean) => {
		onConfigChange({
			...config,
			domain: {
				...config.domain,
				enabled: checked,
			},
		});
	};

	const handleDomainNameChange = (value: string) => {
		onConfigChange({
			...config,
			domain: {
				enabled: config.domain?.enabled ?? false,
				...config.domain,
				domain_name: value,
			},
		});
	};

	const handleCreateZoneChange = (checked: boolean) => {
		onConfigChange({
			...config,
			domain: {
				enabled: config.domain?.enabled ?? false,
				...config.domain,
				create_domain_zone: checked,
			},
		});
	};

	const handleApiPrefixChange = (value: string) => {
		onConfigChange({
			...config,
			domain: {
				enabled: config.domain?.enabled ?? false,
				...config.domain,
				api_domain_prefix: value,
			},
		});
	};

	const handleAddPrefixChange = (checked: boolean) => {
		onConfigChange({
			...config,
			domain: {
				enabled: config.domain?.enabled ?? false,
				...config.domain,
				add_domain_prefix: checked,
			},
		});
	};

	const handleZoneIdChange = (value: string) => {
		onConfigChange({
			...config,
			domain: {
				enabled: config.domain?.enabled ?? false,
				...config.domain,
				zone_id: value,
			},
		});
	};

	const isEnabled = config.domain?.enabled ?? false;
	const domainName = config.domain?.domain_name || "";
	const apiPrefix = config.domain?.api_domain_prefix || "api";
	const addPrefix = config.domain?.add_domain_prefix ?? true;

	// Calculate the full domain based on settings
	const fullDomain =
		addPrefix && !config.is_prod ? `${config.env}.${domainName}` : domainName;
	const apiDomain = `${apiPrefix}.${fullDomain}`;

	return (
		<div className="space-y-6">
			<Card>
				<CardHeader>
					<CardTitle>Route 53 Configuration</CardTitle>
					<CardDescription>
						Configure DNS and domain settings for your infrastructure
					</CardDescription>
				</CardHeader>
				<CardContent className="space-y-6">
					{/* Enable Domain */}
					<div className="flex items-center justify-between">
						<div className="space-y-0.5">
							<Label htmlFor="domain-enabled">Enable Domain</Label>
							<p className="text-xs text-gray-400">
								Enable Route 53 domain configuration
							</p>
						</div>
						<Switch
							id="domain-enabled"
							checked={isEnabled}
							onCheckedChange={handleDomainEnabledChange}
						/>
					</div>

					{isEnabled && (
						<>
							{/* Domain Name */}
							<div>
								<Label htmlFor="domain-name">Domain Name</Label>
								<Input
									id="domain-name"
									value={domainName}
									onChange={(e) => handleDomainNameChange(e.target.value)}
									placeholder="example.com"
									className="mt-1 bg-gray-800 border-gray-600 text-white"
								/>
								<p className="text-xs text-gray-400 mt-1">
									Your base domain name (without www)
								</p>
							</div>

							{/* Create Domain Zone */}
							<div className="flex items-center justify-between">
								<div className="space-y-0.5">
									<Label htmlFor="create-zone">Create Domain Zone</Label>
									<p className="text-xs text-gray-400">
										Create a new Route 53 hosted zone
									</p>
								</div>
								<Switch
									id="create-zone"
									checked={config.domain?.create_domain_zone ?? true}
									onCheckedChange={handleCreateZoneChange}
								/>
							</div>

							{/* Zone ID - Only show when using existing zone */}
							{!(config.domain?.create_domain_zone ?? true) && (
								<div>
									<Label htmlFor="zone-id">Zone ID (Optional)</Label>
									<Input
										id="zone-id"
										value={config.domain?.zone_id || ""}
										onChange={(e) => handleZoneIdChange(e.target.value)}
										placeholder="Z1234567890ABC"
										className="mt-1 bg-gray-800 border-gray-600 text-white"
									/>
									<p className="text-xs text-gray-400 mt-1">
										Route 53 zone ID for existing hosted zone (leave empty to lookup by name)
									</p>
								</div>
							)}

							{/* API Domain Prefix */}
							<div>
								<Label htmlFor="api-prefix">API Domain Prefix</Label>
								<Input
									id="api-prefix"
									value={apiPrefix}
									onChange={(e) => handleApiPrefixChange(e.target.value)}
									placeholder="api"
									className="mt-1 bg-gray-800 border-gray-600 text-white"
								/>
								<p className="text-xs text-gray-400 mt-1">
									Subdomain prefix for API endpoints
								</p>
							</div>

							{/* Add Environment Prefix */}
							<div className="flex items-center justify-between">
								<div className="space-y-0.5">
									<Label htmlFor="add-prefix">Add Environment Prefix</Label>
									<p className="text-xs text-gray-400">
										Add environment prefix to domain (disabled for production)
									</p>
								</div>
								<Switch
									id="add-prefix"
									checked={addPrefix}
									onCheckedChange={handleAddPrefixChange}
									disabled={config.is_prod}
								/>
							</div>

							{/* Domain Preview */}
							{domainName && (
								<div className="bg-blue-900/20 border border-blue-700 rounded-lg p-4">
									<div className="flex items-start gap-2">
										<Globe className="w-4 h-4 text-blue-400 mt-0.5" />
										<div className="flex-1 space-y-2">
											<h4 className="text-sm font-medium text-blue-400">
												Domain Configuration
											</h4>
											<div className="space-y-1 text-xs text-gray-300">
												<div className="flex items-center gap-2">
													<span className="text-gray-400">Main Domain:</span>
													<code className="text-blue-300">{fullDomain}</code>
												</div>
												<div className="flex items-center gap-2">
													<span className="text-gray-400">API Domain:</span>
													<code className="text-blue-300">{apiDomain}</code>
												</div>
												{config.domain?.create_domain_zone && (
													<div className="flex items-center gap-2 mt-2">
														<Info className="w-3 h-3 text-blue-400" />
														<span className="text-xs text-gray-400">
															A new Route 53 hosted zone will be created
														</span>
													</div>
												)}
											</div>
										</div>
									</div>
								</div>
							)}

							{/* SSL/TLS Certificate Info */}
							<div className="bg-green-900/20 border border-green-700 rounded-lg p-4">
								<div className="flex items-start gap-2">
									<Shield className="w-4 h-4 text-green-400 mt-0.5" />
									<div className="flex-1">
										<h4 className="text-sm font-medium text-green-400 mb-2">
											SSL/TLS Certificates
										</h4>
										<ul className="text-xs text-gray-300 space-y-1">
											<li>• ACM certificates will be automatically created</li>
											<li>
												• Wildcard certificate for{" "}
												<code className="text-green-300">*.{fullDomain}</code>
											</li>
											<li>• DNS validation will be configured in Route 53</li>
											<li>• HTTPS enforced on all endpoints</li>
										</ul>
									</div>
								</div>
							</div>

							{/* Important Notes */}
							<div className="bg-yellow-900/20 border border-yellow-700 rounded-lg p-4">
								<h4 className="text-sm font-medium text-yellow-400 mb-2">
									Important Notes
								</h4>
								<ul className="text-xs text-gray-300 space-y-1">
									<li>
										• Domain registration is not handled - register separately
									</li>
									<li>• Update nameservers to Route 53 after zone creation</li>
									<li>• DNS propagation may take up to 48 hours</li>
									<li>
										• Environment prefix helps prevent conflicts
										(dev.example.com)
									</li>
								</ul>
							</div>
						</>
					)}
				</CardContent>
			</Card>
		</div>
	);
}
