import { AlertCircle, CheckCircle2, Info, Loader2 } from "lucide-react";
import { useState, useEffect } from "react";
import { infrastructureApi } from "../api/infrastructure";
import type { AccountInfo, ECRSource, ConfigureCrossAccountECRResponse } from "../api/infrastructure";
import type { YamlInfrastructureConfig } from "../types/yamlConfig";
import { Alert, AlertDescription } from "./ui/alert";
import {
	Card,
	CardContent,
	CardDescription,
	CardHeader,
	CardTitle,
} from "./ui/card";
import { Label } from "./ui/label";
import { RadioGroup, RadioGroupItem } from "./ui/radio-group";

interface ECRNodePropertiesProps {
	config: YamlInfrastructureConfig;
	onConfigChange: (config: Partial<YamlInfrastructureConfig>) => void;
	accountInfo?: AccountInfo;
}

export function ECRNodeProperties({
	config,
	onConfigChange,
	accountInfo,
}: ECRNodePropertiesProps) {
	const [ecrMode, setEcrMode] = useState<"create" | "cross-account">(
		config.ecr_strategy === "cross_account" ? "cross-account" : "create",
	);

	// ECR sources state
	const [ecrSources, setEcrSources] = useState<ECRSource[]>([]);
	const [selectedSource, setSelectedSource] = useState<string>("");
	const [isLoadingSources, setIsLoadingSources] = useState(false);
	const [isConfiguring, setIsConfiguring] = useState(false);
	const [configResult, setConfigResult] = useState<ConfigureCrossAccountECRResponse | null>(null);
	const [error, setError] = useState<string>("");

	// Deployment status (from real AWS check)
	const [deploymentStatus, setDeploymentStatus] = useState<{
		deployed: boolean;
		has_trust_for: boolean;
		checking: boolean;
		repository?: string;
	} | null>(null);

	// Load ECR sources when cross-account mode is selected
	useEffect(() => {
		if (ecrMode === "cross-account") {
			loadECRSources();
		}
	}, [ecrMode]);

	const loadECRSources = async () => {
		setIsLoadingSources(true);
		setError("");
		try {
			const response = await infrastructureApi.getECRSources();
			setEcrSources(response.sources);

			// Pre-select the current source if already configured
			if (config.ecr_account_id && config.ecr_account_region) {
				const currentSource = response.sources.find(
					s => s.account_id === config.ecr_account_id && s.region === config.ecr_account_region
				);
				if (currentSource) {
					setSelectedSource(currentSource.name);
					// Check deployment status for the current source
					checkDeploymentStatus(currentSource.name);
				}
			}
		} catch (err) {
			setError(err instanceof Error ? err.message : "Failed to load ECR sources");
		} finally {
			setIsLoadingSources(false);
		}
	};

	// Check real AWS deployment status
	const checkDeploymentStatus = async (sourceEnvName: string) => {
		if (!config.account_id) {
			return;
		}

		setDeploymentStatus({ deployed: false, has_trust_for: false, checking: true });

		try {
			const result = await infrastructureApi.checkECRTrustPolicy(
				sourceEnvName,
				config.account_id
			);
			setDeploymentStatus({
				deployed: result.deployed,
				has_trust_for: result.has_trust_for,
				checking: false,
				repository: result.repository,
			});
		} catch (err) {
			console.error("Failed to check deployment status:", err);
			setDeploymentStatus({ deployed: false, has_trust_for: false, checking: false });
		}
	};

	const handleSourceSelection = async (sourceEnvName: string) => {
		setSelectedSource(sourceEnvName);
		setIsConfiguring(true);
		setError("");
		setConfigResult(null);
		setDeploymentStatus(null); // Reset deployment status

		try {
			const response = await infrastructureApi.configureCrossAccountECR({
				source_env: sourceEnvName,
				target_env: config.env,
			});
			setConfigResult(response);

			// Update local config to reflect changes
			onConfigChange({
				ecr_strategy: "cross_account",
				ecr_account_id: response.source_env.account_id,
				ecr_account_region: response.source_env.region,
			});

			// Check deployment status after configuration
			await checkDeploymentStatus(sourceEnvName);
		} catch (err) {
			setError(err instanceof Error ? err.message : "Failed to configure cross-account ECR");
		} finally {
			setIsConfiguring(false);
		}
	};

	const handleModeChange = (value: string) => {
		const mode = value as "create" | "cross-account";
		setEcrMode(mode);

		if (mode === "create") {
			// Set to local strategy and clear cross-account settings
			setError("");
			setSelectedSource("");
			setConfigResult(null);
			onConfigChange({
				...config,
				ecr_strategy: "local",
				ecr_account_id: undefined,
				ecr_account_region: undefined,
			});
		} else {
			// Set to cross_account strategy
			onConfigChange({
				...config,
				ecr_strategy: "cross_account",
			});
		}
	};

	return (
		<div className="space-y-6">
			<Card>
				<CardHeader>
					<CardTitle>ECR Configuration</CardTitle>
					<CardDescription>
						Configure container registry for your application
					</CardDescription>
				</CardHeader>
				<CardContent className="space-y-4">
					<RadioGroup value={ecrMode} onValueChange={handleModeChange}>
						<div className="flex items-center space-x-2 p-3 rounded-lg border border-gray-700 hover:bg-gray-800">
							<RadioGroupItem value="create" id="create-ecr" />
							<Label htmlFor="create-ecr" className="flex-1 cursor-pointer">
								<div className="font-medium">Create ECR Repository</div>
								<div className="text-xs text-gray-400">
									Create a new ECR repository in this AWS account
								</div>
							</Label>
						</div>
						<div className="flex items-center space-x-2 p-3 rounded-lg border border-gray-700 hover:bg-gray-800">
							<RadioGroupItem value="cross-account" id="cross-account-ecr" />
							<Label
								htmlFor="cross-account-ecr"
								className="flex-1 cursor-pointer"
							>
								<div className="font-medium">Use Cross-Account ECR</div>
								<div className="text-xs text-gray-400">
									Use an ECR repository from another AWS account
								</div>
							</Label>
						</div>
					</RadioGroup>

					{ecrMode === "create" && (
						<div className="mt-4 space-y-3">
							<div className="bg-blue-900/20 border border-blue-700 rounded-lg p-3">
								<div className="flex items-start gap-2">
									<Info className="w-4 h-4 text-blue-400 mt-0.5" />
									<div className="space-y-1">
										<div className="text-xs text-gray-300">
											ECR repositories will be created automatically for each
											service in your infrastructure
										</div>
										<div className="text-xs text-gray-400">
											Note: A new ECR repository will be created in this
											environment
										</div>
									</div>
								</div>
							</div>

							<div className="grid grid-cols-2 gap-4">
								<div>
									<Label className="text-xs text-gray-400">
										Repository Naming
									</Label>
									<div className="text-sm font-medium">
										{config.project}_[type]_[name]
									</div>
								</div>
								<div>
									<Label className="text-xs text-gray-400">Region</Label>
									<div className="text-sm font-medium">{config.region}</div>
								</div>
							</div>

							{accountInfo && (
								<div className="grid grid-cols-2 gap-4">
									<div>
										<Label className="text-xs text-gray-400">
											AWS Account ID
										</Label>
										<div className="text-sm font-mono">
											{accountInfo.accountId}
										</div>
									</div>
									<div>
										<Label className="text-xs text-gray-400">AWS Profile</Label>
										<div className="text-sm font-medium">
											{accountInfo.profile}
										</div>
									</div>
								</div>
							)}

							<div className="bg-yellow-900/20 border border-yellow-700 rounded-lg p-3">
								<h4 className="text-xs font-medium text-yellow-400 mb-2">
									Repository Creation Rules
								</h4>
								<ul className="text-xs text-gray-300 space-y-1">
									<li>
										• Repositories are created in environments with{" "}
										<code className="text-yellow-400">ecr_strategy: local</code>
									</li>
									<li>
										• Repository names follow pattern:{" "}
										<code className="text-yellow-400">
											{config.project}_[type]_[name]
										</code>
									</li>
									<li>
										• Types: <code>backend</code>, <code>service</code>,{" "}
										<code>task</code>
									</li>
									<li>• Names must be unique within the AWS account</li>
								</ul>
							</div>
						</div>
					)}

					{ecrMode === "cross-account" && (
						<div className="mt-4 space-y-4">
							{/* YAML Update Info */}
							<Alert className="bg-blue-900/20 border-blue-700">
								<Info className="h-4 w-4 text-blue-400" />
								<AlertDescription className="text-xs">
									Selecting a source will automatically update both YAML files
									(source and target). The source will grant access to this environment.
								</AlertDescription>
							</Alert>

							{/* Error Display */}
							{error && (
								<Alert className="bg-red-900/20 border-red-700">
									<AlertCircle className="h-4 w-4 text-red-400" />
									<AlertDescription className="text-sm text-red-400">
										{error}
									</AlertDescription>
								</Alert>
							)}

							{/* Loading State */}
							{isLoadingSources && (
								<div className="flex items-center justify-center py-4">
									<Loader2 className="h-6 w-6 animate-spin text-blue-400" />
									<span className="ml-2 text-sm text-gray-400">
										Loading ECR sources...
									</span>
								</div>
							)}

							{/* ECR Source Selector */}
							{!isLoadingSources && ecrSources.length > 0 && (
								<div>
									<Label htmlFor="ecr-source">
										Select ECR Source Environment
									</Label>
									<select
										id="ecr-source"
										value={selectedSource}
										onChange={(e) => handleSourceSelection(e.target.value)}
										disabled={isConfiguring || isLoadingSources}
										className="mt-1 w-full bg-gray-800 border-gray-600 text-white rounded-md px-3 py-2 text-sm disabled:opacity-50"
									>
										<option value="">-- Choose an environment --</option>
										{ecrSources.map((source) => (
											<option key={source.name} value={source.name}>
												{source.name} ({source.account_id} - {source.region})
											</option>
										))}
									</select>
									<p className="text-xs text-gray-400 mt-1">
										Select an environment with local ECR repositories
									</p>
								</div>
							)}

							{/* No Sources Available */}
							{!isLoadingSources && ecrSources.length === 0 && !error && (
								<Alert className="bg-yellow-900/20 border-yellow-700">
									<AlertCircle className="h-4 w-4 text-yellow-400" />
									<AlertDescription>
										<div className="text-sm font-medium text-yellow-400">
											No ECR Sources Available
										</div>
										<div className="text-xs text-gray-300 mt-1">
											Create an environment with ecr_strategy: local first, then
											configure it with an AWS account ID and region.
										</div>
									</AlertDescription>
								</Alert>
							)}

							{/* Info: Configuration will update both files */}
							{selectedSource && (() => {
								const selectedSourceData = ecrSources.find(s => s.name === selectedSource);
								const isAlreadyTrusted = selectedSourceData?.trusted_accounts.some(
									ta => ta.account_id === config.account_id && ta.env === config.env
								);

								if (isAlreadyTrusted) {
									return (
										<Alert className="bg-blue-900/20 border-blue-700">
											<Info className="h-4 w-4 text-blue-400" />
											<AlertDescription className="text-sm text-blue-400">
												This environment is already configured to trust {config.env}.
												Re-selecting will update the configuration files.
											</AlertDescription>
										</Alert>
									);
								}

								return null;
							})()}

							{/* Deployment Status Check */}
							{deploymentStatus && !deploymentStatus.checking && selectedSource && (
								<>
									{deploymentStatus.deployed && deploymentStatus.has_trust_for && (
										<Alert className="bg-green-900/20 border-green-700">
											<CheckCircle2 className="h-4 w-4 text-green-400" />
											<AlertDescription className="text-sm text-green-400">
												<div className="font-medium">✓ Trust Policy Deployed in AWS</div>
												<div className="text-xs mt-1">
													Cross-account access is ready. Repository: {deploymentStatus.repository}
												</div>
											</AlertDescription>
										</Alert>
									)}

									{deploymentStatus.deployed && !deploymentStatus.has_trust_for && (
										<Alert className="bg-yellow-900/20 border-yellow-700">
											<Info className="h-4 w-4 text-yellow-400" />
											<AlertDescription className="text-sm text-yellow-400">
												<div className="font-medium">⚠ Configuration Updated - Deployment Required</div>
												<div className="text-xs mt-1 space-y-2">
													<div>
														We've updated <strong>{selectedSource}.yaml</strong> to grant access to this account ({config.account_id}).
														Now deploy the source environment to apply the trust policy to AWS:
													</div>
													<pre className="bg-gray-800 rounded p-2 text-xs">
														make infra-apply env={selectedSource}
													</pre>
													<div className="text-yellow-300">
														This will update the ECR repository policy in AWS to allow cross-account pull access.
													</div>
												</div>
											</AlertDescription>
										</Alert>
									)}

									{!deploymentStatus.deployed && (
										<Alert className="bg-yellow-900/20 border-yellow-700">
											<Info className="h-4 w-4 text-yellow-400" />
											<AlertDescription className="text-sm text-yellow-400">
												<div className="font-medium">⚠ Configuration Updated - Deployment Required</div>
												<div className="text-xs mt-1 space-y-2">
													<div>
														We've updated <strong>{selectedSource}.yaml</strong> to grant access to this account ({config.account_id}).
														Deploy the source environment to create the trust policy in AWS:
													</div>
													<pre className="bg-gray-800 rounded p-2 text-xs">
														make infra-apply env={selectedSource}
													</pre>
													<div className="text-yellow-300">
														This will create the ECR repository policy in AWS and enable cross-account pull access.
													</div>
												</div>
											</AlertDescription>
										</Alert>
									)}
								</>
							)}

							{/* Checking Deployment Status */}
							{deploymentStatus?.checking && (
								<div className="flex items-center gap-2 text-sm text-blue-400">
									<Loader2 className="h-4 w-4 animate-spin" />
									<span>Checking AWS deployment status...</span>
								</div>
							)}

							{/* Configuration Success */}
							{configResult && (
								<Alert className="bg-green-900/20 border-green-700">
									<CheckCircle2 className="h-4 w-4 text-green-400" />
									<AlertDescription>
										<div className="font-medium text-green-400 mb-2">
											Configuration Updated Successfully
										</div>
										<div className="text-xs text-gray-300 space-y-1">
											<div>Modified files:</div>
											<ul className="list-disc list-inside">
												{configResult.modified_files.map((file) => (
													<li key={file}>{file}</li>
												))}
											</ul>
											<div className="mt-3">Next steps:</div>
											<ol className="list-decimal list-inside space-y-1">
												{configResult.next_steps.map((step, index) => (
													<li key={index}>{step}</li>
												))}
											</ol>
										</div>
									</AlertDescription>
								</Alert>
							)}

							{/* ECR URL Preview */}
							{config.ecr_account_id && config.ecr_account_region && (
								<div className="bg-blue-900/20 border border-blue-700 rounded-lg p-3">
									<h4 className="text-xs font-medium text-blue-400 mb-2">
										ECR Repository URL
									</h4>
									<div className="bg-gray-800 rounded p-2">
										<code className="text-xs text-gray-300 break-all">
											{config.ecr_account_id}.dkr.ecr.{config.ecr_account_region}.amazonaws.com/{config.project}_backend
										</code>
									</div>
									<p className="text-xs text-gray-400 mt-2">
										Backend service will pull images from this URL
									</p>
								</div>
							)}
						</div>
					)}
				</CardContent>
			</Card>
		</div>
	);
}
