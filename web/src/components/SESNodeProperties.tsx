import {
	AlertCircle,
	CheckCircle,
	ExternalLink,
	FileText,
	Globe,
	Info,
	Key,
	Mail,
	Plus,
	Shield,
	X,
} from "lucide-react";
import { useState } from "react";
import type { YamlInfrastructureConfig } from "../types/yamlConfig";
import { Alert, AlertDescription } from "./ui/alert";
import { Button } from "./ui/button";
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

interface SESNodePropertiesProps {
	config: YamlInfrastructureConfig;
	onConfigChange: (config: Partial<YamlInfrastructureConfig>) => void;
}

export function SESNodeProperties({
	config,
	onConfigChange,
}: SESNodePropertiesProps) {
	const [newTestEmail, setNewTestEmail] = useState("");

	const sesConfig = config.ses || { enabled: false };
	const testEmails = sesConfig.test_emails || [];

	// Determine the actual domain that will be used
	const isDomainEnabled = config.domain?.enabled;
	const mainDomain = config.domain?.domain_name;
	const isProd = config.env === "prod" || config.env === "production";
	const defaultDomain = mainDomain
		? isProd
			? `mail.${mainDomain}`
			: `mail.${config.env}.${mainDomain}`
		: "";
	const actualDomain = sesConfig.domain_name || defaultDomain;

	const handleToggleSES = (enabled: boolean) => {
		onConfigChange({
			ses: {
				...sesConfig,
				enabled,
			},
		});
	};

	const handleDomainChange = (domain_name: string) => {
		onConfigChange({
			ses: {
				...sesConfig,
				domain_name,
			},
		});
	};

	const handleAddTestEmail = () => {
		if (newTestEmail && !testEmails.includes(newTestEmail)) {
			onConfigChange({
				ses: {
					...sesConfig,
					test_emails: [...testEmails, newTestEmail],
				},
			});
			setNewTestEmail("");
		}
	};

	const handleRemoveTestEmail = (email: string) => {
		onConfigChange({
			ses: {
				...sesConfig,
				test_emails: testEmails.filter((e) => e !== email),
			},
		});
	};

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
							<Label htmlFor="ses-enabled" className="text-base">
								Enable SES
							</Label>
							<p className="text-sm text-gray-500">
								Set up email sending infrastructure with domain verification
							</p>
						</div>
						<Switch
							id="ses-enabled"
							checked={sesConfig.enabled}
							onCheckedChange={handleToggleSES}
						/>
					</div>
				</CardContent>
			</Card>

			{sesConfig.enabled && (
				<>
					{/* Domain Configuration */}
					<Card>
						<CardHeader>
							<CardTitle className="flex items-center gap-2">
								<Globe className="w-5 h-5" />
								Domain Configuration
							</CardTitle>
							<CardDescription>
								Configure the domain for sending emails (uses mail subdomain by
								default)
							</CardDescription>
						</CardHeader>
						<CardContent className="space-y-4">
							<div className="space-y-2">
								<Label htmlFor="domain-name">Email Domain (Optional)</Label>
								<Input
									id="domain-name"
									value={sesConfig.domain_name || ""}
									onChange={(e) => handleDomainChange(e.target.value)}
									placeholder={defaultDomain || "example.com"}
								/>
								<p className="text-xs text-gray-500">
									{isDomainEnabled && mainDomain && !sesConfig.domain_name ? (
										<>
											Default:{" "}
											<code className="text-blue-400">{defaultDomain}</code> (
											{isProd ? "mail" : `mail.${config.env}`} subdomain)
										</>
									) : (
										"The domain you'll use for sending emails (must have Route53 zone)"
									)}
								</p>
							</div>

							{actualDomain && (
								<Alert>
									<Info className="h-4 w-4" />
									<AlertDescription>
										<div className="space-y-1">
											<p>
												SES will use:{" "}
												<strong className="text-blue-400">
													{actualDomain}
												</strong>
											</p>
											<p className="text-xs">
												DNS records will be automatically created in Route53 for
												domain verification and DKIM authentication
											</p>
										</div>
									</AlertDescription>
								</Alert>
							)}

							{!isDomainEnabled && !sesConfig.domain_name && (
								<Alert className="border-yellow-600 bg-yellow-50">
									<AlertCircle className="h-4 w-4 text-yellow-600" />
									<AlertDescription>
										Domain module is not enabled. You must specify a domain name
										or enable the domain module to use the default mail
										subdomain.
									</AlertDescription>
								</Alert>
							)}
						</CardContent>
					</Card>

					{/* Test Email Addresses */}
					<Card>
						<CardHeader>
							<CardTitle className="flex items-center gap-2">
								<FileText className="w-5 h-5" />
								Test Email Addresses
							</CardTitle>
							<CardDescription>
								Email addresses to verify for testing (useful in SES sandbox
								mode)
							</CardDescription>
						</CardHeader>
						<CardContent className="space-y-4">
							<div className="flex items-center gap-2">
								<Input
									value={newTestEmail}
									onChange={(e) => setNewTestEmail(e.target.value)}
									placeholder="test@example.com"
									type="email"
									onKeyPress={(e) => e.key === "Enter" && handleAddTestEmail()}
								/>
								<Button
									size="sm"
									onClick={handleAddTestEmail}
									disabled={!newTestEmail}
								>
									<Plus className="w-4 h-4" />
								</Button>
							</div>

							{testEmails.length > 0 && (
								<div className="space-y-2">
									{testEmails.map((email, index) => (
										<div
											key={index}
											className="flex items-center justify-between p-2 bg-gray-800 rounded"
										>
											<span className="text-sm font-mono">{email}</span>
											<button
												onClick={() => handleRemoveTestEmail(email)}
												className="text-gray-400 hover:text-red-400 transition-colors"
											>
												<X className="w-4 h-4" />
											</button>
										</div>
									))}
								</div>
							)}

							<Alert>
								<AlertCircle className="h-4 w-4" />
								<AlertDescription>
									Test emails are useful when SES is in sandbox mode. Each email
									address will be verified individually.
								</AlertDescription>
							</Alert>
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
												Domain Identity
											</h4>
											<p className="text-xs text-gray-400 mt-1">
												Verifies ownership of {actualDomain || "your domain"}{" "}
												for sending emails
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
												Sets up DomainKeys Identified Mail for email
												authentication
											</p>
										</div>
									</div>

									<div className="flex items-start gap-3 p-3 bg-gray-800 rounded-lg">
										<CheckCircle className="w-5 h-5 text-green-400 mt-0.5" />
										<div className="flex-1">
											<h4 className="text-sm font-medium text-gray-200">
												DNS Records in Route53
											</h4>
											<ul className="text-xs text-gray-400 mt-1 ml-4 space-y-1">
												<li>• Domain verification TXT record</li>
												<li>• 3 DKIM CNAME records for authentication</li>
												<li>• DMARC record for email policy</li>
											</ul>
										</div>
									</div>

									<div className="flex items-start gap-3 p-3 bg-gray-800 rounded-lg">
										<CheckCircle className="w-5 h-5 text-green-400 mt-0.5" />
										<div className="flex-1">
											<h4 className="text-sm font-medium text-gray-200">
												Email Identities
											</h4>
											<p className="text-xs text-gray-400 mt-1">
												Verifies {testEmails.length} test email{" "}
												{testEmails.length === 1 ? "address" : "addresses"}
											</p>
										</div>
									</div>

									<div className="flex items-start gap-3 p-3 bg-gray-800 rounded-lg">
										<CheckCircle className="w-5 h-5 text-green-400 mt-0.5" />
										<div className="flex-1">
											<h4 className="text-sm font-medium text-gray-200">
												ACM Certificate
											</h4>
											<p className="text-xs text-gray-400 mt-1">
												SSL certificate for the email domain
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
									• The domain must be one you own and have a Route53 hosted
									zone for
								</p>
								{isDomainEnabled && mainDomain && (
									<p>
										• Default domain:{" "}
										<code className="text-blue-400">{defaultDomain}</code>{" "}
										{!isProd && "(includes environment prefix for non-prod)"}
									</p>
								)}
								<p>• Domain verification happens automatically via Route53</p>
								<p>• DMARC policy is set to quarantine unauthorized emails</p>
								<p>• Email authentication improves deliverability rates</p>
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
								<pre>{`ses:
  enabled: ${sesConfig.enabled}${
		sesConfig.domain_name
			? `
  domain_name: "${sesConfig.domain_name}"`
			: isDomainEnabled && mainDomain
				? `
  # domain_name not specified - will use default: ${defaultDomain}`
				: `
  domain_name: "example.com" # Required - domain module not enabled`
	}${
		testEmails.length > 0
			? `
  test_emails:${testEmails
		.map(
			(email) => `
    - "${email}"`,
		)
		.join("")}`
			: ""
	}`}</pre>
							</div>
						</CardContent>
					</Card>
				</>
			)}
		</div>
	);
}
