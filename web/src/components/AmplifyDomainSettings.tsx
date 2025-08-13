import {
	AlertCircle,
	CheckCircle,
	GitBranch,
	Globe,
	Loader2,
} from "lucide-react";
import { useEffect, useState } from "react";
import { amplifyApi } from "../api";
import type { AmplifyAppInfo } from "../types/amplify";
import type { UpdateHandler } from "../types/components";
import type { YamlInfrastructureConfig } from "../types/yamlConfig";
import { Input } from "./ui/input";
import { Label } from "./ui/label";
import { Switch } from "./ui/switch";

interface AmplifyDomainSettingsProps {
	config: YamlInfrastructureConfig;
	nodeId: string;
	onConfigChange?: (config: Partial<YamlInfrastructureConfig>) => void;
}

export function AmplifyDomainSettings({
	config,
	nodeId,
	onConfigChange,
}: AmplifyDomainSettingsProps) {
	const appName = nodeId.replace("amplify-", "");
	const amplifyAppIndex =
		config.amplify_apps?.findIndex((app) => app.name === appName) ?? -1;
	const amplifyApp = config.amplify_apps?.[amplifyAppIndex];

	const [apiApp, setApiApp] = useState<AmplifyAppInfo | null>(null);
	const [loading, setLoading] = useState(true);

	useEffect(() => {
		const fetchAppData = async () => {
			try {
				const response = await amplifyApi.getApps(config.env || "dev");
				const foundApp = response.apps.find((a) => a.name === appName);
				if (foundApp) {
					setApiApp(foundApp);
				}
			} catch (err) {
				console.error("Failed to fetch Amplify app data:", err);
			} finally {
				setLoading(false);
			}
		};

		fetchAppData();
	}, [appName, config.env]);

	if (!amplifyApp) {
		return (
			<div className="text-gray-400">
				<p>Amplify app configuration not found.</p>
			</div>
		);
	}

	const handleChange: UpdateHandler<string | boolean> = (
		field: string,
		value,
	) => {
		if (onConfigChange && config.amplify_apps) {
			const updatedApps = [...config.amplify_apps];
			updatedApps[amplifyAppIndex] = {
				...amplifyApp,
				[field]: value,
			};
			onConfigChange({ amplify_apps: updatedApps });
		}
	};

	const hasCustomDomain = !!amplifyApp.custom_domain;
	const branches = amplifyApp.branches || [];
	const productionBranch = branches.find((b) => b.stage === "PRODUCTION");
	const hasProductionBranch = !!productionBranch;

	return (
		<div className="space-y-6">
			<div>
				<h3 className="text-sm font-medium text-white mb-4">
					Domain Configuration
				</h3>

				{/* Show API-provided domain info if available */}
				{apiApp && (
					<div className="mb-4 bg-gray-800 rounded-lg p-4 border border-gray-700">
						<div className="space-y-2">
							<div className="flex items-center gap-2">
								<Globe className="w-4 h-4 text-blue-400" />
								<span className="text-sm font-medium text-white">
									Live Domain Information
								</span>
							</div>
							{apiApp.defaultDomain && (
								<div>
									<p className="text-xs text-gray-400">Default Domain:</p>
									<p className="text-sm text-gray-300 font-mono">
										{apiApp.defaultDomain}
									</p>
								</div>
							)}
							{apiApp.customDomain && (
								<div>
									<p className="text-xs text-gray-400">
										Custom Domain (from AWS):
									</p>
									<p className="text-sm text-gray-300 font-mono">
										{apiApp.customDomain}
									</p>
								</div>
							)}
						</div>
					</div>
				)}

				<div className="space-y-3">
					<div>
						<Label htmlFor="custom_domain">Custom Domain</Label>
						<Input
							id="custom_domain"
							value={amplifyApp.custom_domain || ""}
							onChange={(e) => handleChange("custom_domain", e.target.value)}
							placeholder="example.com"
							className="mt-1 bg-gray-800 border-gray-600 text-white"
						/>
						{amplifyApp.custom_domain && (
							<div className="mt-2 flex items-center gap-2">
								<CheckCircle className="w-3 h-3 text-green-400" />
								<span className="text-xs text-green-400">
									Connected to Route 53
								</span>
							</div>
						)}
						{amplifyApp.custom_domain && !hasProductionBranch && (
							<div className="mt-2 flex items-center gap-2">
								<AlertCircle className="w-3 h-3 text-amber-400" />
								<span className="text-xs text-amber-400">
									Custom domain requires at least one PRODUCTION branch
								</span>
							</div>
						)}
						<p className="text-xs text-gray-500 mt-1">
							Leave empty to use default Amplify domain
						</p>
					</div>

					<div>
						<div className="flex items-center justify-between bg-gray-800 rounded-lg p-4 border border-gray-700">
							<Label
								htmlFor="enable_root_domain"
								className="font-normal cursor-pointer"
							>
								Enable root domain access
							</Label>
							<Switch
								id="enable_root_domain"
								checked={amplifyApp.enable_root_domain || false}
								onCheckedChange={(checked) =>
									handleChange("enable_root_domain", checked)
								}
								disabled={!amplifyApp.custom_domain}
							/>
						</div>
						{amplifyApp.enable_root_domain && amplifyApp.custom_domain && (
							<p className="text-xs text-gray-500 mt-1">
								Application accessible at {amplifyApp.custom_domain}
							</p>
						)}
					</div>
				</div>
			</div>

			<div>
				<h3 className="text-sm font-medium text-white mb-4">
					Branch URLs & Subdomains
				</h3>
				<div className="space-y-3">
					{branches.length === 0 ? (
						<div className="bg-gray-800 rounded-lg p-4 border border-gray-700">
							<p className="text-sm text-gray-500">
								No branches configured. Add branches in the Branches tab.
							</p>
						</div>
					) : (
						branches.map((branch) => {
							const customSubdomains = branch.custom_subdomains || [];
							const isProduction = branch.stage === "PRODUCTION";

							return (
								<div
									key={branch.name}
									className="bg-gray-800 rounded-lg p-4 border border-gray-700"
								>
									<div className="flex items-center justify-between mb-3">
										<div className="flex items-center gap-2">
											<GitBranch className="w-4 h-4 text-gray-400" />
											<span className="text-sm font-medium text-white">
												{branch.name}
											</span>
											<span
												className={`px-2 py-0.5 text-xs font-medium rounded-full ${
													branch.stage === "PRODUCTION"
														? "bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200"
														: branch.stage === "BETA"
															? "bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-200"
															: branch.stage === "DEVELOPMENT"
																? "bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200"
																: "bg-purple-100 text-purple-800 dark:bg-purple-900 dark:text-purple-200"
												}`}
											>
												{branch.stage || "DEVELOPMENT"}
											</span>
										</div>
										{branch.enable_pull_request_preview && (
											<span className="text-xs text-gray-400">
												PR previews enabled
											</span>
										)}
									</div>

									<div className="space-y-2">
										<div>
											<p className="text-xs text-gray-400">
												Default Amplify URL
											</p>
											{loading ? (
												<div className="flex items-center gap-2">
													<Loader2 className="w-3 h-3 animate-spin text-gray-500" />
													<span className="text-sm text-gray-500">
														Loading...
													</span>
												</div>
											) : apiApp ? (
												<div>
													{/* Show actual branch URL from API */}
													{(() => {
														const apiBranch = apiApp.branches.find(
															(b) => b.branchName === branch.name,
														);
														if (apiBranch?.branchUrl) {
															return (
																<p className="text-sm text-gray-300 font-mono">
																	{apiBranch.branchUrl}
																</p>
															);
														}
														// Fallback to constructed URL with actual app ID
														return (
															<p className="text-sm text-gray-300 font-mono">
																https://{branch.name.replace("/", "-")}.
																{apiApp.appId}.amplifyapp.com
															</p>
														);
													})()}
													{/* Also show the app's default domain if available */}
													{apiApp.defaultDomain && (
														<p className="text-xs text-gray-500 mt-1">
															App domain: {apiApp.defaultDomain}
														</p>
													)}
												</div>
											) : (
												/* Fallback to pattern when API not available */
												<p className="text-sm text-gray-300 font-mono">
													https://{branch.name.replace("/", "-")}.{"<app-id>"}
													.amplifyapp.com
												</p>
											)}
										</div>

										{hasCustomDomain && (
											<div>
												<p className="text-xs text-gray-400">
													Custom Domain Mappings
												</p>
												<div className="space-y-1 mt-1">
													{isProduction && amplifyApp.enable_root_domain && (
														<p className="text-sm text-gray-300 font-mono">
															{amplifyApp.custom_domain} → {branch.name}
														</p>
													)}
													{customSubdomains.length > 0 &&
														customSubdomains.map((subdomain) => (
															<p
																key={subdomain}
																className="text-sm text-gray-300 font-mono"
															>
																{subdomain}.{amplifyApp.custom_domain} →{" "}
																{branch.name}
															</p>
														))}
												</div>
											</div>
										)}

										{branch.enable_pull_request_preview && (
											<div>
												<p className="text-xs text-gray-400">
													PR Preview Pattern
												</p>
												<p className="text-sm text-gray-300 font-mono">
													https://pr-{"<pr-number>"}.{amplifyApp.name}
													.amplifyapp.com
												</p>
											</div>
										)}
									</div>
								</div>
							);
						})
					)}
				</div>
			</div>

			<div>
				<h3 className="text-sm font-medium text-white mb-4">SSL/TLS</h3>
				<div className="bg-gray-800 rounded-lg p-4 border border-gray-700">
					<div className="flex items-center gap-2">
						<CheckCircle className="w-4 h-4 text-green-400" />
						<span className="text-sm text-gray-300">
							SSL certificates are automatically managed by AWS Amplify
						</span>
					</div>
				</div>
			</div>

			<div>
				<h3 className="text-sm font-medium text-white mb-4">
					Domain Configuration Notes
				</h3>
				<div className="bg-gray-800 rounded-lg p-4 border border-gray-700 space-y-2">
					<div className="flex items-start gap-2">
						<span className="text-gray-400">•</span>
						<p className="text-sm text-gray-300">
							The root domain maps to the first PRODUCTION branch
						</p>
					</div>
					<div className="flex items-start gap-2">
						<span className="text-gray-400">•</span>
						<p className="text-sm text-gray-300">
							Branch names with slashes (/) are converted to hyphens (-) in URLs
						</p>
					</div>
					<div className="flex items-start gap-2">
						<span className="text-gray-400">•</span>
						<p className="text-sm text-gray-300">
							Configure subdomains for each branch in the Branches tab
						</p>
					</div>
				</div>
			</div>
		</div>
	);
}
