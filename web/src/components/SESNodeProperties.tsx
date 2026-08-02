import {
	AlertCircle,
	CheckCircle,
	ChevronDown,
	ChevronRight,
	ExternalLink,
	FileText,
	Globe,
	Info,
	Key,
	Mail,
	Plus,
	Settings,
	Shield,
	Trash2,
	X,
} from "lucide-react";
import { useId, useState } from "react";
import type {
	SESConfig,
	SESDomain,
	YamlInfrastructureConfig,
} from "../types/yamlConfig";
import { Alert, AlertDescription } from "./ui/alert";
import { Badge } from "./ui/badge";
import { Button } from "./ui/button";
import {
	Card,
	CardContent,
	CardDescription,
	CardHeader,
	CardTitle,
} from "./ui/card";
import {
	Collapsible,
	CollapsibleContent,
	CollapsibleTrigger,
} from "./ui/collapsible";
import { Input } from "./ui/input";
import { Label } from "./ui/label";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "./ui/select";
import { Switch } from "./ui/switch";

interface SESNodePropertiesProps {
	config: YamlInfrastructureConfig;
	onConfigChange: (config: Partial<YamlInfrastructureConfig>) => void;
}

// Helper to check if a domain is a subdomain of or equals the parent domain
// Mirrors the isSubdomainOf helper in raymond.go
function isSubdomainOf(domain: string, parentDomain: string): boolean {
	if (!domain || !parentDomain) return false;

	// Normalize: remove trailing dots
	const d = domain.replace(/\.$/, "");
	const p = parentDomain.replace(/\.$/, "");

	// Exact match
	if (d === p) return true;

	// Check if domain ends with ".parentDomain"
	return d.endsWith(`.${p}`);
}

export function SESNodeProperties({
	config,
	onConfigChange,
}: SESNodePropertiesProps) {
	const sesEnabledId = useId();
	const newDomainId = useId();
	const newZoneIdInputId = useId();
	const [newDomain, setNewDomain] = useState("");
	const [newDomainZoneId, setNewDomainZoneId] = useState("");
	const [showAddDomain, setShowAddDomain] = useState(false);
	const [expandedDomains, setExpandedDomains] = useState<Set<string>>(
		new Set(),
	);
	const [newTestEmail, setNewTestEmail] = useState("");

	const sesConfig: SESConfig = config.ses || { enabled: false };

	// Get Route53 domain configuration for auto-detection
	const domainConfig = config.domain;
	const isDomainModuleEnabled = domainConfig?.enabled ?? false;
	const managedDomain = domainConfig?.domain_name || "";

	// Collect all Route53-managed domains (main domain + additional domains)
	const getAllManagedDomains = (): string[] => {
		const domains: string[] = [];
		if (isDomainModuleEnabled && managedDomain) {
			domains.push(managedDomain);
		}
		// Add additional domains that have zone_id or create_zone=true
		if (domainConfig?.additional_domains) {
			for (const ad of domainConfig.additional_domains) {
				if (ad.domain && (ad.zone_id || ad.create_zone)) {
					domains.push(ad.domain);
				}
			}
		}
		return domains;
	};

	const allManagedDomains = getAllManagedDomains();

	// Check if a domain will be auto-managed by Route53, returns the matching managed domain
	const getAutoManagedParent = (domain: string): string | null => {
		// Check against all managed domains (main + additional)
		for (const managed of allManagedDomains) {
			if (isSubdomainOf(domain, managed)) {
				return managed;
			}
		}
		return null;
	};

	// Check if a domain will be auto-managed by Route53
	const isDomainAutoManaged = (domain: string): boolean => {
		return getAutoManagedParent(domain) !== null;
	};

	// Get all domains (merge legacy + new format, deduplicated)
	const getAllDomains = (): SESDomain[] => {
		const domains: SESDomain[] = [];

		// Add new multi-domain format first (preferred)
		if (sesConfig.domains && sesConfig.domains.length > 0) {
			domains.push(...sesConfig.domains);
		}

		// Add legacy domain only if NOT already in domains array
		if (sesConfig.domain_name) {
			const alreadyExists = domains.some(
				(d) => d.domain === sesConfig.domain_name,
			);
			if (!alreadyExists) {
				domains.push({
					domain: sesConfig.domain_name,
				});
			}
		}

		return domains;
	};

	const allDomains = getAllDomains();

	const handleToggleSES = (enabled: boolean) => {
		onConfigChange({
			ses: {
				...sesConfig,
				enabled,
			},
		});
	};

	const handleAddDomain = () => {
		if (!newDomain.trim()) return;

		// Check for duplicate
		if (allDomains.some((d) => d.domain === newDomain.trim())) {
			return;
		}

		const newDomainEntry: SESDomain = {
			domain: newDomain.trim(),
			...(newDomainZoneId.trim() && { zone_id: newDomainZoneId.trim() }),
		};

		// Migrate to new format if using legacy
		const updatedDomains = [...(sesConfig.domains || []), newDomainEntry];

		// If we have a legacy domain, migrate it to the new format
		if (sesConfig.domain_name && !sesConfig.domains) {
			updatedDomains.unshift({
				domain: sesConfig.domain_name,
			});
		}

		onConfigChange({
			ses: {
				...sesConfig,
				domains: updatedDomains,
				// Clear legacy fields after migration
				domain_name: undefined,
				test_emails: undefined,
			},
		});

		setNewDomain("");
		setNewDomainZoneId("");
		setShowAddDomain(false);
	};

	const handleRemoveDomain = (domainToRemove: string) => {
		// If removing legacy domain
		if (sesConfig.domain_name === domainToRemove && !sesConfig.domains) {
			onConfigChange({
				ses: {
					...sesConfig,
					domain_name: undefined,
					test_emails: undefined,
				},
			});
			return;
		}

		// Remove from domains array
		const updatedDomains = (sesConfig.domains || []).filter(
			(d) => d.domain !== domainToRemove,
		);

		onConfigChange({
			ses: {
				...sesConfig,
				domains: updatedDomains.length > 0 ? updatedDomains : undefined,
			},
		});
	};

	const handleUpdateDomain = (
		domainName: string,
		updates: Partial<SESDomain>,
	) => {
		// Handle legacy domain - migrate to new format on first update
		if (sesConfig.domain_name === domainName && !sesConfig.domains) {
			const migratedDomains = [{ domain: sesConfig.domain_name, ...updates }];
			onConfigChange({
				ses: {
					...sesConfig,
					domains: migratedDomains,
					domain_name: undefined,
				},
			});
			return;
		}

		// Update in domains array
		const updatedDomains = (sesConfig.domains || []).map((d) =>
			d.domain === domainName ? { ...d, ...updates } : d,
		);

		onConfigChange({
			ses: {
				...sesConfig,
				domains: updatedDomains,
			},
		});
	};

	// Test emails are now global (account-wide in AWS SES)
	const handleAddTestEmail = () => {
		const email = newTestEmail.trim();
		if (!email) return;

		const currentEmails = sesConfig.test_emails || [];
		if (currentEmails.includes(email)) return;

		onConfigChange({
			ses: {
				...sesConfig,
				test_emails: [...currentEmails, email],
			},
		});
		setNewTestEmail("");
	};

	const handleRemoveTestEmail = (email: string) => {
		onConfigChange({
			ses: {
				...sesConfig,
				test_emails: (sesConfig.test_emails || []).filter((e) => e !== email),
			},
		});
	};

	const handleUpdateGlobalSettings = (updates: Partial<SESConfig>) => {
		onConfigChange({
			ses: {
				...sesConfig,
				...updates,
			},
		});
	};

	const toggleDomainExpanded = (domain: string) => {
		setExpandedDomains((prev) => {
			const next = new Set(prev);
			if (next.has(domain)) {
				next.delete(domain);
			} else {
				next.add(domain);
			}
			return next;
		});
	};

	// Check if any domain requires manual DNS (no explicit zone_id AND not auto-managed)
	const domainsNeedingManualDNS = allDomains.filter(
		(d) => !d.zone_id && !isDomainAutoManaged(d.domain),
	);
	const hasManualDNS = domainsNeedingManualDNS.length > 0;

	return (
		<div className="space-y-6">
			{/* Enable/Disable SES */}
			<Card>
				<CardHeader>
					<CardTitle className="flex items-center gap-2">
						<Mail className="w-5 h-5" />
						Simple Email Service (SES)
					</CardTitle>
					<CardDescription>
						Configure AWS SES for sending transactional and marketing emails
					</CardDescription>
				</CardHeader>
				<CardContent>
					<div className="flex items-center justify-between">
						<div className="space-y-1">
							<Label htmlFor={sesEnabledId} className="text-base">
								Enable SES
							</Label>
							<p className="text-sm text-gray-500">
								Set up email sending infrastructure with domain verification
							</p>
						</div>
						<Switch
							id={sesEnabledId}
							checked={sesConfig.enabled}
							onCheckedChange={handleToggleSES}
						/>
					</div>
				</CardContent>
			</Card>

			{sesConfig.enabled && (
				<>
					{/* Domain List */}
					<Card>
						<CardHeader>
							<div className="flex items-center justify-between">
								<div>
									<CardTitle className="flex items-center gap-2">
										<Globe className="w-5 h-5" />
										Email Domains
									</CardTitle>
									<CardDescription>
										Configure domains for sending emails (multiple domains
										supported)
									</CardDescription>
								</div>
								<Button
									size="sm"
									variant="outline"
									onClick={() => setShowAddDomain(!showAddDomain)}
								>
									<Plus className="w-4 h-4 mr-1" />
									Add Domain
								</Button>
							</div>
						</CardHeader>
						<CardContent className="space-y-4">
							{/* Add Domain Form */}
							{showAddDomain && (
								<div className="p-4 border rounded-lg space-y-4 bg-muted/30">
									<div className="grid grid-cols-2 gap-4">
										<div className="space-y-2">
											<Label htmlFor={newDomainId}>Domain Name</Label>
											<Input
												id={newDomainId}
												value={newDomain}
												onChange={(e) => setNewDomain(e.target.value)}
												placeholder="example.com"
											/>
										</div>
										<div className="space-y-2">
											<Label htmlFor={newZoneIdInputId}>
												Zone ID{" "}
												<span className="text-xs text-gray-500">
													(optional)
												</span>
											</Label>
											<Input
												id={newZoneIdInputId}
												value={newDomainZoneId}
												onChange={(e) => setNewDomainZoneId(e.target.value)}
												placeholder="Z1234567890ABC"
											/>
										</div>
									</div>
									<p className="text-xs text-gray-500">
										Provide a Route53 Zone ID for automatic DNS record creation,
										or leave empty for manual DNS setup.
									</p>
									<div className="flex justify-end gap-2">
										<Button
											variant="ghost"
											size="sm"
											onClick={() => {
												setShowAddDomain(false);
												setNewDomain("");
												setNewDomainZoneId("");
											}}
										>
											Cancel
										</Button>
										<Button
											size="sm"
											onClick={handleAddDomain}
											disabled={!newDomain.trim()}
										>
											Add Domain
										</Button>
									</div>
								</div>
							)}

							{/* Domain Cards */}
							{allDomains.length === 0 ? (
								<div className="text-center py-8 text-gray-500">
									<Mail className="w-12 h-12 mx-auto mb-2 opacity-50" />
									<p>No domains configured</p>
									<p className="text-sm">
										Add a domain to start sending emails
									</p>
								</div>
							) : (
								<div className="space-y-3">
									{allDomains.map((domain) => (
										<DomainCard
											key={domain.domain}
											domain={domain}
											sesConfig={sesConfig}
											isExpanded={expandedDomains.has(domain.domain)}
											onToggleExpand={() => toggleDomainExpanded(domain.domain)}
											onRemove={() => handleRemoveDomain(domain.domain)}
											onUpdate={(updates) =>
												handleUpdateDomain(domain.domain, updates)
											}
											isAutoManaged={isDomainAutoManaged(domain.domain)}
											managedZoneDomain={
												getAutoManagedParent(domain.domain) || ""
											}
										/>
									))}
								</div>
							)}
						</CardContent>
					</Card>

					{/* Manual DNS Instructions */}
					{hasManualDNS && (
						<Card className="border-yellow-600/50">
							<CardHeader>
								<CardTitle className="flex items-center gap-2">
									<AlertCircle className="w-5 h-5 text-yellow-500" />
									Manual DNS Required
								</CardTitle>
								<CardDescription>
									Some domains require manual DNS configuration
								</CardDescription>
							</CardHeader>
							<CardContent className="space-y-4">
								<Alert className="border-yellow-600 bg-yellow-50/10">
									<AlertCircle className="h-4 w-4 text-yellow-600" />
									<AlertDescription>
										<p className="mb-2">
											The following domains do not have a Route53 Zone ID
											configured:
										</p>
										<ul className="list-disc list-inside space-y-1">
											{domainsNeedingManualDNS.map((d) => (
												<li key={d.domain} className="font-mono text-sm">
													{d.domain}
												</li>
											))}
										</ul>
									</AlertDescription>
								</Alert>

								<div className="space-y-3">
									<h4 className="text-sm font-medium">
										After running terraform apply, add these DNS records
										manually:
									</h4>

									<div className="bg-gray-900 rounded-lg p-3 font-mono text-xs space-y-2">
										<p className="text-gray-400">
											# Domain verification (TXT record)
										</p>
										<p>
											_amazonses.DOMAIN TXT "verification-token-from-output"
										</p>
										<p className="text-gray-400 mt-3">
											# DKIM records (3 CNAME records)
										</p>
										<p>
											token1._domainkey.DOMAIN CNAME token1.dkim.amazonses.com
										</p>
										<p>
											token2._domainkey.DOMAIN CNAME token2.dkim.amazonses.com
										</p>
										<p>
											token3._domainkey.DOMAIN CNAME token3.dkim.amazonses.com
										</p>
										<p className="text-gray-400 mt-3"># SPF record (TXT)</p>
										<p>DOMAIN TXT "v=spf1 include:amazonses.com ~all"</p>
										<p className="text-gray-400 mt-3"># MAIL FROM records</p>
										<p>
											bounce.DOMAIN MX 10 feedback-smtp.REGION.amazonses.com
										</p>
										<p>bounce.DOMAIN TXT "v=spf1 include:amazonses.com ~all"</p>
										<p className="text-gray-400 mt-3"># DMARC record</p>
										<p>
											_dmarc.DOMAIN TXT "v=DMARC1; p=none; pct=100;
											rua=mailto:dmarc-reports@DOMAIN"
										</p>
									</div>

									<p className="text-xs text-gray-500">
										Check the terraform output for the actual verification
										tokens and DKIM values.
									</p>
								</div>
							</CardContent>
						</Card>
					)}

					{/* Global Settings */}
					<Card>
						<CardHeader>
							<CardTitle className="flex items-center gap-2">
								<Settings className="w-5 h-5" />
								Global Settings
							</CardTitle>
							<CardDescription>
								Default settings applied to all domains (can be overridden
								per-domain)
							</CardDescription>
						</CardHeader>
						<CardContent className="space-y-4">
							{/* Test Email Addresses - Account-wide for SES sandbox */}
							<div className="space-y-2">
								<Label className="flex items-center gap-2">
									<FileText className="w-4 h-4" />
									Test Email Addresses
								</Label>
								<div className="flex items-center gap-2">
									<Input
										value={newTestEmail}
										onChange={(e) => setNewTestEmail(e.target.value)}
										placeholder="test@example.com"
										type="email"
										onKeyDown={(e) => e.key === "Enter" && handleAddTestEmail()}
									/>
									<Button
										size="sm"
										onClick={handleAddTestEmail}
										disabled={!newTestEmail.trim()}
									>
										<Plus className="w-4 h-4" />
									</Button>
								</div>

								{(sesConfig.test_emails?.length ?? 0) > 0 && (
									<div className="space-y-2">
										{sesConfig.test_emails?.map((email) => (
											<div
												key={email}
												className="flex items-center justify-between p-2 bg-gray-800 rounded"
											>
												<span className="text-sm font-mono">{email}</span>
												<button
													type="button"
													onClick={() => handleRemoveTestEmail(email)}
													className="text-gray-400 hover:text-red-400 transition-colors"
												>
													<X className="w-4 h-4" />
												</button>
											</div>
										))}
									</div>
								)}
								<p className="text-xs text-gray-500">
									Test emails are verified for SES sandbox mode testing
									(account-wide, not per-domain)
								</p>
							</div>

							<div className="border-t border-gray-700 pt-4" />

							{/* MAIL FROM Settings */}
							<div className="flex items-center justify-between">
								<div className="space-y-1">
									<Label>Enable MAIL FROM</Label>
									<p className="text-xs text-gray-500">
										Use your domain for envelope sender (improves
										deliverability)
									</p>
								</div>
								<Switch
									checked={sesConfig.global_enable_mail_from !== false}
									onCheckedChange={(checked) =>
										handleUpdateGlobalSettings({
											global_enable_mail_from: checked,
										})
									}
								/>
							</div>

							{sesConfig.global_enable_mail_from !== false && (
								<div className="space-y-2">
									<Label>Default MAIL FROM Subdomain</Label>
									<Input
										value={sesConfig.global_mail_from_subdomain || "bounce"}
										onChange={(e) =>
											handleUpdateGlobalSettings({
												global_mail_from_subdomain: e.target.value,
											})
										}
										placeholder="bounce"
									/>
									<p className="text-xs text-gray-500">
										Bounce messages will come from{" "}
										<code className="text-blue-400">
											{sesConfig.global_mail_from_subdomain || "bounce"}
											.yourdomain.com
										</code>
									</p>
								</div>
							)}

							{/* DMARC Settings */}
							<div className="space-y-2">
								<Label>Default DMARC Policy</Label>
								<Select
									value={sesConfig.global_dmarc_policy || "none"}
									onValueChange={(value) =>
										handleUpdateGlobalSettings({
											global_dmarc_policy: value as
												| "none"
												| "quarantine"
												| "reject",
										})
									}
								>
									<SelectTrigger>
										<SelectValue />
									</SelectTrigger>
									<SelectContent>
										<SelectItem value="none">
											None (Monitor only - recommended for initial setup)
										</SelectItem>
										<SelectItem value="quarantine">
											Quarantine (Send failed emails to spam)
										</SelectItem>
										<SelectItem value="reject">
											Reject (Block failed emails entirely)
										</SelectItem>
									</SelectContent>
								</Select>
								<p className="text-xs text-gray-500">
									Start with "none" until SPF/DKIM are verified, then gradually
									increase protection.
								</p>
							</div>

							<div className="space-y-2">
								<Label>Default DMARC Report Email</Label>
								<Input
									value={sesConfig.global_dmarc_rua_email || ""}
									onChange={(e) =>
										handleUpdateGlobalSettings({
											global_dmarc_rua_email: e.target.value,
										})
									}
									placeholder="dmarc-reports@yourdomain.com"
								/>
								<p className="text-xs text-gray-500">
									Leave empty to use dmarc-reports@[domain] for each domain
								</p>
							</div>
						</CardContent>
					</Card>

					{/* What SES Creates */}
					<Card>
						<CardHeader>
							<CardTitle className="flex items-center gap-2">
								<Shield className="w-5 h-5" />
								AWS Resources Created
							</CardTitle>
							<CardDescription>
								Resources that will be created when SES is enabled
							</CardDescription>
						</CardHeader>
						<CardContent>
							<div className="space-y-4">
								<div className="grid grid-cols-1 gap-3">
									<div className="flex items-start gap-3 p-3 bg-gray-800 rounded-lg">
										<CheckCircle className="w-5 h-5 text-green-400 mt-0.5" />
										<div className="flex-1">
											<h4 className="text-sm font-medium text-gray-200">
												Domain Identities
											</h4>
											<p className="text-xs text-gray-400 mt-1">
												{allDomains.length} domain
												{allDomains.length !== 1 ? "s" : ""} configured for
												sending emails
											</p>
										</div>
									</div>

									<div className="flex items-start gap-3 p-3 bg-gray-800 rounded-lg">
										<CheckCircle className="w-5 h-5 text-green-400 mt-0.5" />
										<div className="flex-1">
											<h4 className="text-sm font-medium text-gray-200">
												DKIM Configuration
											</h4>
											<p className="text-xs text-gray-400 mt-1">
												DomainKeys Identified Mail for email authentication (3
												CNAME records per domain)
											</p>
										</div>
									</div>

									<div className="flex items-start gap-3 p-3 bg-gray-800 rounded-lg">
										<CheckCircle className="w-5 h-5 text-green-400 mt-0.5" />
										<div className="flex-1">
											<h4 className="text-sm font-medium text-gray-200">
												SPF & DMARC Records
											</h4>
											<p className="text-xs text-gray-400 mt-1">
												Email authorization and policy records for each domain
											</p>
										</div>
									</div>

									<div className="flex items-start gap-3 p-3 bg-gray-800 rounded-lg">
										<CheckCircle className="w-5 h-5 text-green-400 mt-0.5" />
										<div className="flex-1">
											<h4 className="text-sm font-medium text-gray-200">
												Test Email Identities
											</h4>
											<p className="text-xs text-gray-400 mt-1">
												{sesConfig.test_emails?.length || 0} test email{" "}
												{(sesConfig.test_emails?.length || 0) === 1
													? "address"
													: "addresses"}{" "}
												for sandbox testing (account-wide)
											</p>
										</div>
									</div>
								</div>
							</div>
						</CardContent>
					</Card>

					{/* Important Notes */}
					<Card>
						<CardHeader>
							<CardTitle className="flex items-center gap-2">
								<Info className="w-5 h-5" />
								Important Notes
							</CardTitle>
							<CardDescription>Key information about using SES</CardDescription>
						</CardHeader>
						<CardContent className="space-y-3">
							<Alert>
								<AlertCircle className="h-4 w-4" />
								<AlertDescription>
									<strong>Sandbox Mode:</strong> SES starts in sandbox mode with
									sending restrictions. You need to request production access
									from AWS to send to unverified addresses.
								</AlertDescription>
							</Alert>

							<div className="space-y-2 text-sm text-gray-300">
								<p>
									- Each domain requires domain verification and DNS records
								</p>
								<p>
									- Domains with Route53 Zone ID get automatic DNS configuration
								</p>
								<p>- DKIM authentication improves email deliverability rates</p>
								<p>
									- Start with DMARC policy "none" until verification is
									complete
								</p>
							</div>

							<div className="pt-2">
								<Button
									variant="outline"
									size="sm"
									onClick={() =>
										window.open(
											"https://console.aws.amazon.com/ses/home",
											"_blank",
										)
									}
								>
									<ExternalLink className="w-4 h-4 mr-2" />
									Open SES Console
								</Button>
							</div>
						</CardContent>
					</Card>

					{/* Configuration Preview */}
					<Card>
						<CardHeader>
							<CardTitle className="flex items-center gap-2">
								<Key className="w-5 h-5" />
								Configuration Preview
							</CardTitle>
							<CardDescription>
								YAML configuration that will be generated
							</CardDescription>
						</CardHeader>
						<CardContent>
							<div className="bg-gray-900 rounded-lg p-3 font-mono text-xs text-gray-300 overflow-x-auto">
								<pre>{generateYamlPreview(sesConfig, allDomains)}</pre>
							</div>
						</CardContent>
					</Card>
				</>
			)}
		</div>
	);
}

interface DomainCardProps {
	domain: SESDomain;
	sesConfig: SESConfig;
	isExpanded: boolean;
	onToggleExpand: () => void;
	onRemove: () => void;
	onUpdate: (updates: Partial<SESDomain>) => void;
	// Route53 auto-detection
	isAutoManaged: boolean;
	managedZoneDomain: string;
}

function DomainCard({
	domain,
	sesConfig,
	isExpanded,
	onToggleExpand,
	onRemove,
	onUpdate,
	isAutoManaged,
	managedZoneDomain,
}: DomainCardProps) {
	const [showAdvanced, setShowAdvanced] = useState(false);

	const hasExplicitZoneId = Boolean(domain.zone_id);

	// Determine effective settings (per-domain or global fallback)
	const effectiveMailFrom =
		domain.enable_mail_from ?? sesConfig.global_enable_mail_from ?? true;
	const effectiveMailFromSubdomain =
		domain.mail_from_subdomain ||
		sesConfig.global_mail_from_subdomain ||
		"bounce";
	const effectiveDmarcPolicy =
		domain.dmarc_policy || sesConfig.global_dmarc_policy || "none";

	return (
		<div className="border rounded-lg overflow-hidden">
			{/* Domain Header */}
			<div className="flex items-center justify-between p-3 bg-gray-800/50">
				<div className="flex items-center gap-3">
					<button
						type="button"
						onClick={onToggleExpand}
						className="p-1 hover:bg-gray-700 rounded"
					>
						{isExpanded ? (
							<ChevronDown className="w-4 h-4" />
						) : (
							<ChevronRight className="w-4 h-4" />
						)}
					</button>
					<Globe className="w-4 h-4 text-blue-400" />
					<span className="font-medium">{domain.domain}</span>
				</div>
				<div className="flex items-center gap-2">
					{hasExplicitZoneId ? (
						<Badge variant="default" className="text-xs">
							<CheckCircle className="w-3 h-3 mr-1" />
							Custom Zone
						</Badge>
					) : isAutoManaged ? (
						<Badge variant="default" className="text-xs bg-blue-600">
							<CheckCircle className="w-3 h-3 mr-1" />
							Auto (Route53)
						</Badge>
					) : (
						<Badge
							variant="outline"
							className="text-xs text-yellow-400 border-yellow-400"
						>
							<AlertCircle className="w-3 h-3 mr-1" />
							Manual DNS
						</Badge>
					)}
					<Button
						variant="ghost"
						size="sm"
						onClick={onRemove}
						className="h-7 w-7 p-0"
					>
						<Trash2 className="w-4 h-4 text-red-400" />
					</Button>
				</div>
			</div>

			{/* Domain Details (Expanded) */}
			{isExpanded && (
				<div className="p-4 space-y-4 border-t border-gray-700">
					{/* Zone ID */}
					<div className="space-y-2">
						<Label>Route53 Zone ID</Label>
						<Input
							value={domain.zone_id || ""}
							onChange={(e) =>
								onUpdate({ zone_id: e.target.value || undefined })
							}
							placeholder={
								isAutoManaged
									? `Managed by Route53 (${managedZoneDomain} zone)`
									: "Z1234567890ABC (optional)"
							}
						/>
						{hasExplicitZoneId ? (
							<p className="text-xs text-green-400">
								<CheckCircle className="w-3 h-3 inline mr-1" />
								DNS records will be created in custom zone
							</p>
						) : isAutoManaged ? (
							<p className="text-xs text-blue-400">
								<CheckCircle className="w-3 h-3 inline mr-1" />
								DNS records will be created in Route53 zone for{" "}
								<strong>{managedZoneDomain}</strong>
							</p>
						) : (
							<p className="text-xs text-yellow-400">
								<AlertCircle className="w-3 h-3 inline mr-1" />
								DNS records must be added manually (domain not managed by
								Route53)
							</p>
						)}
					</div>

					{/* Advanced Settings (Collapsible) */}
					<Collapsible open={showAdvanced} onOpenChange={setShowAdvanced}>
						<CollapsibleTrigger asChild>
							<Button
								variant="ghost"
								size="sm"
								className="w-full justify-start"
							>
								<Settings className="w-4 h-4 mr-2" />
								Advanced Settings
								{showAdvanced ? (
									<ChevronDown className="w-4 h-4 ml-auto" />
								) : (
									<ChevronRight className="w-4 h-4 ml-auto" />
								)}
							</Button>
						</CollapsibleTrigger>
						<CollapsibleContent className="pt-4 space-y-4">
							<p className="text-xs text-gray-500">
								Override global settings for this domain only
							</p>

							{/* Per-domain MAIL FROM */}
							<div className="flex items-center justify-between">
								<div className="space-y-1">
									<Label className="text-sm">Custom MAIL FROM</Label>
									<p className="text-xs text-gray-500">
										{domain.enable_mail_from === undefined
											? `Using global (${effectiveMailFrom ? "enabled" : "disabled"})`
											: "Domain-specific"}
									</p>
								</div>
								<Select
									value={
										domain.enable_mail_from === undefined
											? "global"
											: domain.enable_mail_from
												? "enabled"
												: "disabled"
									}
									onValueChange={(value) => {
										if (value === "global") {
											onUpdate({ enable_mail_from: undefined });
										} else {
											onUpdate({ enable_mail_from: value === "enabled" });
										}
									}}
								>
									<SelectTrigger className="w-32">
										<SelectValue />
									</SelectTrigger>
									<SelectContent>
										<SelectItem value="global">Global</SelectItem>
										<SelectItem value="enabled">Enabled</SelectItem>
										<SelectItem value="disabled">Disabled</SelectItem>
									</SelectContent>
								</Select>
							</div>

							{effectiveMailFrom && (
								<div className="space-y-2">
									<Label className="text-sm">MAIL FROM Subdomain</Label>
									<Input
										value={domain.mail_from_subdomain || ""}
										onChange={(e) =>
											onUpdate({
												mail_from_subdomain: e.target.value || undefined,
											})
										}
										placeholder={`Use global (${effectiveMailFromSubdomain})`}
									/>
								</div>
							)}

							{/* Per-domain DMARC */}
							<div className="space-y-2">
								<Label className="text-sm">DMARC Policy</Label>
								<Select
									value={domain.dmarc_policy || "global"}
									onValueChange={(value) => {
										if (value === "global") {
											onUpdate({ dmarc_policy: undefined });
										} else {
											onUpdate({
												dmarc_policy: value as "none" | "quarantine" | "reject",
											});
										}
									}}
								>
									<SelectTrigger>
										<SelectValue />
									</SelectTrigger>
									<SelectContent>
										<SelectItem value="global">
											Use global ({effectiveDmarcPolicy})
										</SelectItem>
										<SelectItem value="none">None (Monitor only)</SelectItem>
										<SelectItem value="quarantine">
											Quarantine (Send to spam)
										</SelectItem>
										<SelectItem value="reject">
											Reject (Block entirely)
										</SelectItem>
									</SelectContent>
								</Select>
							</div>

							<div className="space-y-2">
								<Label className="text-sm">DMARC Report Email</Label>
								<Input
									value={domain.dmarc_rua_email || ""}
									onChange={(e) =>
										onUpdate({ dmarc_rua_email: e.target.value || undefined })
									}
									placeholder={`dmarc-reports@${domain.domain}`}
								/>
							</div>
						</CollapsibleContent>
					</Collapsible>
				</div>
			)}
		</div>
	);
}

function generateYamlPreview(
	sesConfig: SESConfig,
	allDomains: SESDomain[],
): string {
	const lines: string[] = ["ses:"];
	lines.push(`  enabled: ${sesConfig.enabled}`);

	// Test emails (global, account-wide)
	if (sesConfig.test_emails && sesConfig.test_emails.length > 0) {
		lines.push("  test_emails:");
		for (const email of sesConfig.test_emails) {
			lines.push(`    - "${email}"`);
		}
	}

	// Global settings
	if (sesConfig.global_enable_mail_from !== undefined) {
		lines.push(
			`  global_enable_mail_from: ${sesConfig.global_enable_mail_from}`,
		);
	}
	if (sesConfig.global_mail_from_subdomain) {
		lines.push(
			`  global_mail_from_subdomain: "${sesConfig.global_mail_from_subdomain}"`,
		);
	}
	if (sesConfig.global_dmarc_policy) {
		lines.push(`  global_dmarc_policy: "${sesConfig.global_dmarc_policy}"`);
	}
	if (sesConfig.global_dmarc_rua_email) {
		lines.push(
			`  global_dmarc_rua_email: "${sesConfig.global_dmarc_rua_email}"`,
		);
	}

	// Domains
	if (allDomains.length > 0) {
		lines.push("  domains:");
		for (const domain of allDomains) {
			lines.push(`    - domain: "${domain.domain}"`);
			if (domain.zone_id) {
				lines.push(`      zone_id: "${domain.zone_id}"`);
			}
			// Per-domain overrides
			if (domain.enable_mail_from !== undefined) {
				lines.push(`      enable_mail_from: ${domain.enable_mail_from}`);
			}
			if (domain.mail_from_subdomain) {
				lines.push(
					`      mail_from_subdomain: "${domain.mail_from_subdomain}"`,
				);
			}
			if (domain.dmarc_policy) {
				lines.push(`      dmarc_policy: "${domain.dmarc_policy}"`);
			}
			if (domain.dmarc_rua_email) {
				lines.push(`      dmarc_rua_email: "${domain.dmarc_rua_email}"`);
			}
		}
	}

	return lines.join("\n");
}
