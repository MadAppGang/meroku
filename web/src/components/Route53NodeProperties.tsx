import { Globe, Info, Shield } from "lucide-react";
import type { YamlInfrastructureConfig } from "../types/yamlConfig";
import { Alert, AlertDescription } from "./ui/alert";
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
				// Auto-clear domain_name when disabling to prevent API domain creation
				domain_name: checked ? (config.domain?.domain_name || "") : "",
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
				add_env_domain_prefix: checked,
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
				// When disabled, set to empty string (or undefined) to skip API Gateway custom domain creation
				api_domain_prefix: checked ? "api" : "",
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
	const addPrefix = config.domain?.add_env_domain_prefix ?? true;

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

					{!isEnabled && domainName && (
						<Alert className="border-yellow-600 bg-yellow-900/20">
							<Info className="h-4 w-4 text-yellow-400" />
							<AlertDescription className="text-yellow-200 text-sm">
								Domain name will be cleared when domain module is disabled to
								prevent API Gateway domain creation without certificate.
							</AlertDescription>
						</Alert>
					)}

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

							{/* Create API Domain Record */}
							<div className="flex items-center justify-between">
								<div className="space-y-0.5">
									<Label htmlFor="create-api-domain">
										Create API Gateway Custom Domain
									</Label>
									<p className="text-xs text-gray-400">
										Create API Gateway custom domain and Route53 A record for
										backend API
									</p>
								</div>
								<Switch
									id="create-api-domain"
									checked={
										config.domain?.api_domain_prefix !== undefined &&
										config.domain?.api_domain_prefix !== null &&
										config.domain?.api_domain_prefix !== ""
									}
									onCheckedChange={handleCreateApiDomainChange}
								/>
							</div>

							{/* API Domain Prefix Input - shown when enabled */}
							{config.domain?.api_domain_prefix &&
								config.domain.api_domain_prefix !== "" && (
									<div className="space-y-2 ml-4">
										<Label htmlFor="api-prefix">API Domain Prefix</Label>
										<Input
											id="api-prefix"
											value={apiPrefix}
											onChange={(e) => handleApiPrefixChange(e.target.value)}
											placeholder="api"
											className="bg-gray-800 border-gray-600 text-white"
										/>
										<p className="text-xs text-gray-400">
											Subdomain prefix for API Gateway (e.g., "api" creates
											api.yourdomain.com)
										</p>
									</div>
								)}

							{/* Per-Service API Domain Configuration */}
							{config.services && config.services.length > 0 && (
								<div className="space-y-3 mt-4 p-4 bg-gray-900/50 border border-gray-700 rounded-lg">
									<h4 className="text-sm font-medium text-gray-300">
										Additional Services API Domains
									</h4>
									{config.services.map((service, index) => (
										<div key={service.name} className="space-y-2">
											<div className="flex items-center justify-between">
												<div className="flex-1">
													<Label htmlFor={`create-api-domain-${service.name}`}>
														{service.name}
													</Label>
													<p className="text-xs text-gray-500 mt-1">
														API Gateway custom domain for this service
													</p>
												</div>
												<Switch
													id={`create-api-domain-${service.name}`}
													checked={
														service.api_domain_prefix !== undefined &&
														service.api_domain_prefix !== null &&
														service.api_domain_prefix !== ""
													}
													onCheckedChange={(checked) => {
														const updatedServices = [...(config.services || [])];
														updatedServices[index] = {
															...service,
															api_domain_prefix: checked ? service.name : "",
														};
														onConfigChange({ services: updatedServices });
													}}
												/>
											</div>

											{service.api_domain_prefix && service.api_domain_prefix !== "" && (
												<div className="space-y-2 ml-4">
													<Label htmlFor={`api-prefix-${service.name}`}>
														API Domain Prefix
													</Label>
													<Input
														id={`api-prefix-${service.name}`}
														value={service.api_domain_prefix}
														onChange={(e: React.ChangeEvent<HTMLInputElement>) => {
															const updatedServices = [...(config.services || [])];
															updatedServices[index] = {
																...service,
																api_domain_prefix: e.target.value || service.name,
															};
															onConfigChange({ services: updatedServices });
														}}
														placeholder={service.name}
														className="bg-gray-800 border-gray-600 text-white"
													/>
													<p className="text-xs text-gray-400">
														Subdomain prefix for this service (default: {service.name})
													</p>
												</div>
											)}
										</div>
									))}
								</div>
							)}

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
												{config.domain?.create_domain_zone && (
													<div className="flex items-center gap-2 mt-2">
														<Info className="w-3 h-3 text-blue-400" />
														<span className="text-xs text-gray-400">
															A new Route 53 hosted zone will be created
														</span>
													</div>
												)}

												{/* API Gateway Custom Domains List */}
												<div className="mt-3 pt-3 border-t border-blue-700/30">
													<p className="text-gray-400 font-medium mb-2">
														API Gateway Custom Domains:
													</p>

													{/* Backend Service */}
													{config.domain?.api_domain_prefix &&
													config.domain.api_domain_prefix !== "" ? (
														<div className="flex items-start gap-2 mb-1">
															<Info className="w-3 h-3 text-green-400 mt-0.5" />
															<div className="flex-1">
																<code className="text-green-300">
																	{config.domain.api_domain_prefix}.{fullDomain}
																</code>
																<span className="text-gray-500 ml-2">(backend)</span>
															</div>
														</div>
													) : (
														<div className="flex items-start gap-2 mb-1">
															<Info className="w-3 h-3 text-yellow-400 mt-0.5" />
															<span className="text-gray-400">
																Backend: <span className="text-yellow-400">disabled</span>
															</span>
														</div>
													)}

													{/* Additional Services */}
													{config.services
														?.filter(
															(s) =>
																s.api_domain_prefix && s.api_domain_prefix !== ""
														)
														.map((service) => (
															<div
																key={service.name}
																className="flex items-start gap-2 mb-1"
															>
																<Info className="w-3 h-3 text-green-400 mt-0.5" />
																<div className="flex-1">
																	<code className="text-green-300">
																		{service.api_domain_prefix}.{fullDomain}
																	</code>
																	<span className="text-gray-500 ml-2">
																		({service.name})
																	</span>
																</div>
															</div>
														))}

													{(!config.domain?.api_domain_prefix ||
														config.domain.api_domain_prefix === "") &&
														(!config.services?.some(
															(s) =>
																s.api_domain_prefix && s.api_domain_prefix !== ""
														)) && (
															<div className="flex items-start gap-2">
																<Info className="w-3 h-3 text-yellow-400 mt-0.5" />
																<span className="text-gray-400">
																	No API Gateway custom domains configured
																</span>
															</div>
														)}
												</div>
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
