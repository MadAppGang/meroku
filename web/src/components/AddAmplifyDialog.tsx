import {
	AlertCircle,
	Check,
	Copy,
	ExternalLink,
	Github,
	Globe,
	Loader2,
	Plus,
	Trash2,
} from "lucide-react";
import { useCallback, useEffect, useMemo, useState } from "react";
import { infrastructureApi } from "../api/infrastructure";
import type { YamlInfrastructureConfig } from "../types/yamlConfig";
import { Button } from "./ui/button";
import { Checkbox } from "./ui/checkbox";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "./ui/dialog";
import { Input } from "./ui/input";
import { Label } from "./ui/label";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "./ui/select";
import { Textarea } from "./ui/textarea";

/**
 * Generate smart defaults for Amplify app based on environment configuration
 */
function generateSmartDefaults(config?: YamlInfrastructureConfig): {
	suggestedName: string;
	suggestedSubdomain: string;
	previewDomain: string;
} {
	const projectName = config?.project || "project";
	const env = config?.env || "dev";

	// Suggested app name: project-frontend
	const suggestedName = `${projectName}-frontend`;

	// Suggested subdomain prefix
	const suggestedSubdomain = "app";

	// Preview domain based on configuration
	let previewDomain = "";
	if (config?.domain?.enabled && config?.domain?.domain_name) {
		const baseDomain = config.domain.domain_name;
		const addEnvPrefix = config.domain.add_env_domain_prefix ?? true;

		if (addEnvPrefix && env !== "prod") {
			previewDomain = `${suggestedSubdomain}.${env}.${baseDomain}`;
		} else {
			previewDomain = `${suggestedSubdomain}.${baseDomain}`;
		}
	}

	return { suggestedName, suggestedSubdomain, previewDomain };
}

/**
 * Calculate domain preview based on subdomain prefix and config
 */
function calculateDomainPreview(
	subdomainPrefix: string,
	config?: YamlInfrastructureConfig
): string {
	if (!subdomainPrefix || !config?.domain?.enabled || !config?.domain?.domain_name) {
		return "";
	}

	const baseDomain = config.domain.domain_name;
	const addEnvPrefix = config.domain.add_env_domain_prefix ?? true;
	const env = config.env || "dev";

	if (addEnvPrefix && env !== "prod") {
		return `${subdomainPrefix}.${env}.${baseDomain}`;
	}
	return `${subdomainPrefix}.${baseDomain}`;
}

interface AddAmplifyDialogProps {
	open: boolean;
	onClose: () => void;
	onAdd: (
		amplifyApp: NonNullable<YamlInfrastructureConfig["amplify_apps"]>[0],
	) => Promise<void>;
	existingApps?: string[];
	environmentName?: string;
	projectName?: string;
	config?: YamlInfrastructureConfig;
}

type BranchStage = "PRODUCTION" | "DEVELOPMENT" | "BETA" | "EXPERIMENTAL";

interface BranchFormData {
	name: string;
	stage: BranchStage;
	enable_auto_build: boolean;
	enable_pull_request_preview: boolean;
	environment_variables: Record<string, string>;
	environment_variables_text: string;
	custom_subdomains: string[];
	custom_subdomains_text: string;
}

export const AddAmplifyDialog: React.FC<AddAmplifyDialogProps> = ({
	open,
	onClose,
	onAdd,
	existingApps = [],
	environmentName = "dev",
	projectName = "project",
	config,
}) => {
	// Generate smart defaults based on config
	const smartDefaults = useMemo(
		() => generateSmartDefaults(config),
		[config]
	);

	const [formData, setFormData] = useState<{
		name: string;
		github_repository: string;
		github_oauth_token: string;
		branches: BranchFormData[];
		subdomain_prefix: string;
	}>({
		name: smartDefaults.suggestedName,
		github_repository: "",
		github_oauth_token: "",
		branches: [
			{
				name: "main",
				stage: "PRODUCTION",
				enable_auto_build: true,
				enable_pull_request_preview: false,
				environment_variables: {},
				environment_variables_text: "",
				custom_subdomains: [],
				custom_subdomains_text: "",
			},
		],
		subdomain_prefix: smartDefaults.suggestedSubdomain,
	});

	const [tokenSaved, setTokenSaved] = useState(false);
	const [tokenError, setTokenError] = useState("");
	const [deviceFlowInProgress, setDeviceFlowInProgress] = useState(false);
	const [userCode, setUserCode] = useState("");
	const [verificationUri, setVerificationUri] = useState("");
	const [pollingInterval, setPollingInterval] = useState<ReturnType<
		typeof setInterval
	> | null>(null);
	const [errors, setErrors] = useState<Record<string, string>>({});
	const [checkingExistingToken, setCheckingExistingToken] = useState(false);

	const ssmParameterPath = useMemo(
		() => `/${projectName}/${environmentName}/github/amplify-token`,
		[projectName, environmentName],
	);

	// Check for existing token (memoized)
	const checkExistingToken = useCallback(async () => {
		setCheckingExistingToken(true);
		setTokenError("");

		try {
			const parameter =
				await infrastructureApi.getSSMParameter(ssmParameterPath);

			// If parameter exists and has a value, mark as authenticated
			if (parameter?.value) {
				setTokenSaved(true);
				setFormData((prev) => ({
					...prev,
					github_oauth_token: ssmParameterPath,
				}));
			}
		} catch (_error) {
			// If parameter doesn't exist, that's fine - user needs to authenticate
			console.log("No existing GitHub token found");
		} finally {
			setCheckingExistingToken(false);
		}
	}, [ssmParameterPath]);

	// Check for existing token when dialog opens
	useEffect(() => {
		if (open) {
			checkExistingToken();
		}
	}, [open, checkExistingToken]);

	const startGitHubDeviceFlow = async () => {
		setDeviceFlowInProgress(true);
		setTokenError("");
		setUserCode("");
		setVerificationUri("");

		try {
			// Step 1: Request device code from backend
			const data = await infrastructureApi.initiateGitHubDeviceFlow({
				app_name: "shared", // Using shared token for all Amplify apps
				project: projectName,
				environment: environmentName,
				scope: "repo", // Required for Amplify to access private repos
			});

			setUserCode(data.user_code);
			setVerificationUri(data.verification_uri);

			// Copy code to clipboard
			try {
				await navigator.clipboard.writeText(data.user_code);
				console.log("Device code copied to clipboard");
			} catch (error) {
				console.error("Failed to copy code to clipboard:", error);
			}

			// Open GitHub verification page in popup
			const width = 600;
			const height = 700;
			const left = window.screen.width / 2 - width / 2;
			const top = window.screen.height / 2 - height / 2;

			window.open(
				data.verification_uri,
				"github-device-auth",
				`width=${width},height=${height},left=${left},top=${top},toolbar=no,menubar=no,scrollbars=yes,resizable=yes`,
			);

			// Step 2: Start polling for authorization
			const interval = setInterval(
				async () => {
					try {
						const status = await infrastructureApi.checkGitHubAuthStatus(
							data.user_code,
						);

						if (status.status === "authorized") {
							// Success! Token has been stored by backend
							clearInterval(interval);
							setPollingInterval(null);
							setDeviceFlowInProgress(false);
							setTokenSaved(true);
							setUserCode("");
							setVerificationUri("");

							// Update form data with SSM parameter path
							const ssmPath = ssmParameterPath;
							setFormData((prev) => ({
								...prev,
								github_oauth_token: ssmPath,
							}));

							if (status.message) {
								// Show success message from backend
								console.log(status.message);
							}
						} else if (status.status === "pending") {
							// Still waiting for user authorization
						} else if (status.status === "expired") {
							// Code expired
							clearInterval(interval);
							setPollingInterval(null);
							setTokenError("Device code expired. Please try again.");
							setDeviceFlowInProgress(false);
							setUserCode("");
							setVerificationUri("");
						} else if (status.status === "error") {
							// Other errors
							clearInterval(interval);
							setPollingInterval(null);
							setTokenError(status.error || "Authentication failed");
							setDeviceFlowInProgress(false);
							setUserCode("");
							setVerificationUri("");
						}
					} catch (error) {
						console.error("Polling error:", error);
						// Don't stop polling on transient errors
					}
				},
				(data.interval || 5) * 1000,
			); // Poll at the interval specified by backend

			setPollingInterval(interval);

			// Auto-cleanup after expiration
			setTimeout(() => {
				if (interval) {
					clearInterval(interval);
					setPollingInterval(null);
					if (deviceFlowInProgress) {
						setDeviceFlowInProgress(false);
						setTokenError("Device code expired. Please try again.");
						setUserCode("");
						setVerificationUri("");
					}
				}
			}, data.expires_in * 1000);
		} catch (error) {
			setDeviceFlowInProgress(false);
			setTokenError(
				error instanceof Error
					? error.message
					: "Failed to start authentication",
			);
		}
	};

	const copyToClipboard = (text: string) => {
		navigator.clipboard.writeText(text);
	};

	const openVerificationUrl = () => {
		if (verificationUri) {
			window.open(verificationUri, "_blank");
		}
	};

	// Branch management functions
	const addBranch = () => {
		setFormData((prev) => ({
			...prev,
			branches: [
				...prev.branches,
				{
					name: "",
					stage: "DEVELOPMENT" as BranchStage,
					enable_auto_build: true,
					enable_pull_request_preview: false,
					environment_variables: {},
					environment_variables_text: "",
					custom_subdomains: [],
					custom_subdomains_text: "",
				},
			],
		}));
	};

	const updateBranch = (
		index: number,
		field: string,
		value: string | boolean,
	) => {
		const updatedBranches = [...formData.branches];

		if (field === "environment_variables_text" && typeof value === "string") {
			// Parse environment variables from text
			const envVars: Record<string, string> = {};
			const lines = value.split("\n").filter((line: string) => line.trim());
			for (const line of lines) {
				const [key, ...valueParts] = line.split("=");
				if (key?.trim()) {
					envVars[key.trim()] = valueParts.join("=").trim();
				}
			}
			updatedBranches[index] = {
				...updatedBranches[index],
				environment_variables: envVars,
				environment_variables_text: value,
			};
		} else if (
			field === "custom_subdomains_text" &&
			typeof value === "string"
		) {
			// Parse custom subdomains from text (comma-separated)
			const subdomains = value
				.split(",")
				.map((s: string) => s.trim())
				.filter((s: string) => s);
			updatedBranches[index] = {
				...updatedBranches[index],
				custom_subdomains: subdomains,
				custom_subdomains_text: value,
			};
		} else {
			updatedBranches[index] = {
				...updatedBranches[index],
				[field]: value,
			};
		}

		setFormData((prev) => ({ ...prev, branches: updatedBranches }));
	};

	const removeBranch = (index: number) => {
		setFormData((prev) => ({
			...prev,
			branches: prev.branches.filter((_, i) => i !== index),
		}));
	};

	const handleSubmit = async () => {
		const newErrors: Record<string, string> = {};

		// Validate required fields
		if (!formData.name) {
			newErrors.name = "App name is required";
		} else if (!/^[a-z0-9-]+$/.test(formData.name)) {
			newErrors.name =
				"App name must contain only lowercase letters, numbers, and hyphens";
		} else if (existingApps.includes(formData.name)) {
			newErrors.name = "An Amplify app with this name already exists";
		}

		if (!formData.github_repository) {
			newErrors.github_repository = "GitHub repository URL is required";
		} else if (!formData.github_repository.startsWith("https://github.com/")) {
			newErrors.github_repository =
				"Repository URL must start with https://github.com/";
		}

		if (!tokenSaved) {
			newErrors.github_oauth_token = "Please authenticate with GitHub first";
		}

		// Validate branches
		if (!formData.branches || formData.branches.length === 0) {
			newErrors.branches = "At least one branch is required";
		} else {
			// Check for unique branch names
			const branchNames = formData.branches.map((b) => b.name);
			const uniqueNames = new Set(branchNames);
			if (uniqueNames.size !== branchNames.length) {
				newErrors.branches = "Branch names must be unique";
			}

			// Validate each branch
			formData.branches.forEach((branch, index) => {
				if (!branch.name) {
					newErrors[`branch_${index}_name`] = `Branch ${
						index + 1
					}: Name is required`;
				} else if (!/^[a-zA-Z0-9-_/]+$/.test(branch.name)) {
					newErrors[`branch_${index}_name`] = `Branch ${
						index + 1
					}: Invalid branch name`;
				}
			});
		}

		if (Object.keys(newErrors).length > 0) {
			setErrors(newErrors);
			return;
		}

		const amplifyApp = {
			name: formData.name,
			github_repository: formData.github_repository,
			github_oauth_token: formData.github_oauth_token,
			branches: formData.branches.map((branch) => ({
				name: branch.name,
				stage: branch.stage,
				enable_auto_build: branch.enable_auto_build,
				enable_pull_request_preview: branch.enable_pull_request_preview,
				environment_variables: branch.environment_variables,
				custom_subdomains: branch.custom_subdomains || [],
			})),
			...(formData.subdomain_prefix && { subdomain_prefix: formData.subdomain_prefix }),
		};

		await onAdd(amplifyApp);
		handleClose();
	};

	const handleClose = async () => {
		// Clean up polling interval if active
		if (pollingInterval) {
			clearInterval(pollingInterval);
			setPollingInterval(null);
		}

		// Clean up OAuth session if in progress
		if (userCode && deviceFlowInProgress) {
			try {
				await infrastructureApi.deleteGitHubOAuthSession(userCode);
			} catch (error) {
				console.error("Failed to cleanup OAuth session:", error);
			}
		}

		// Reset form with smart defaults
		setFormData({
			name: smartDefaults.suggestedName,
			github_repository: "",
			github_oauth_token: "",
			branches: [
				{
					name: "main",
					stage: "PRODUCTION" as BranchStage,
					enable_auto_build: true,
					enable_pull_request_preview: false,
					environment_variables: {},
					environment_variables_text: "",
					custom_subdomains: [],
					custom_subdomains_text: "",
				},
			],
			subdomain_prefix: smartDefaults.suggestedSubdomain,
		});
		setErrors({});
		setTokenSaved(false);
		setTokenError("");
		setDeviceFlowInProgress(false);
		setUserCode("");
		setVerificationUri("");
		onClose();
	};

	return (
		<Dialog open={open} onOpenChange={handleClose}>
			<DialogContent className="max-w-2xl max-h-[90vh] overflow-y-auto">
				<DialogHeader>
					<DialogTitle>Add Amplify App</DialogTitle>
					<DialogDescription>
						Configure AWS Amplify for hosting your frontend application with
						continuous deployment.
					</DialogDescription>
				</DialogHeader>

				<div className="grid gap-4 py-4">
					{/* Basic Configuration */}
					<div className="grid gap-2">
						<h3 className="text-sm font-semibold">Basic Configuration</h3>

						<div>
							<Label htmlFor="name">App Name *</Label>
							<Input
								id="name"
								value={formData.name}
								onChange={(e) =>
									setFormData({ ...formData, name: e.target.value })
								}
								placeholder="main-web"
								className={`mt-1 ${errors.name ? "border-red-500" : ""}`}
							/>
							{errors.name && (
								<p className="text-xs text-red-500 mt-1">{errors.name}</p>
							)}
							{smartDefaults.suggestedName && (
								<p className="text-xs text-blue-400 mt-1">
									💡 Pre-filled as {smartDefaults.suggestedName} based on project name
								</p>
							)}
						</div>

						<div>
							<Label htmlFor="github_repository">GitHub Repository URL *</Label>
							<Input
								id="github_repository"
								value={formData.github_repository}
								onChange={(e) =>
									setFormData({
										...formData,
										github_repository: e.target.value,
									})
								}
								placeholder="https://github.com/username/repo"
								className={`mt-1 ${
									errors.github_repository ? "border-red-500" : ""
								}`}
							/>
							{errors.github_repository && (
								<p className="text-xs text-red-500 mt-1">
									{errors.github_repository}
								</p>
							)}
						</div>

						<div>
							<Label>GitHub Authentication *</Label>

							<div className="border rounded-lg p-4 space-y-3 mt-1">
								{!tokenSaved && !userCode ? (
									<>
										<p className="text-sm text-gray-600">
											Authenticate with GitHub to grant Amplify access to your
											repository
										</p>

										<Button
											type="button"
											onClick={startGitHubDeviceFlow}
											disabled={deviceFlowInProgress || checkingExistingToken}
											className="w-full"
										>
											{checkingExistingToken ? (
												<>
													<Loader2 className="w-4 h-4 mr-2 animate-spin" />
													Checking existing token...
												</>
											) : deviceFlowInProgress ? (
												<>
													<Loader2 className="w-4 h-4 mr-2 animate-spin" />
													Starting authentication...
												</>
											) : (
												<>
													<Github className="w-4 h-4 mr-2" />
													Authenticate with GitHub
												</>
											)}
										</Button>
									</>
								) : !tokenSaved && userCode ? (
									<div className="space-y-4">
										<div className="text-center space-y-2">
											<p className="text-sm font-medium">
												Enter this code on GitHub:
											</p>
											<div className="flex items-center justify-center gap-2">
												<code className="text-2xl font-mono bg-gray-800 text-white px-4 py-2 rounded select-all border border-gray-700">
													{userCode}
												</code>
												<Button
													type="button"
													size="icon"
													variant="outline"
													onClick={() => copyToClipboard(userCode)}
													title="Copy code"
												>
													<Copy className="w-4 h-4" />
												</Button>
											</div>
											<p className="text-xs text-gray-500">
												Code copied to clipboard! A popup window should have
												opened with GitHub. If not, click the button below.
											</p>
										</div>

										<div className="flex flex-col gap-2">
											<Button
												type="button"
												onClick={openVerificationUrl}
												variant="secondary"
												className="w-full"
											>
												<ExternalLink className="w-4 h-4 mr-2" />
												Re-open GitHub verification page
											</Button>

											<p className="text-xs text-center text-gray-500">
												Or visit:{" "}
												<code className="bg-gray-800 text-gray-300 px-1 rounded">
													{verificationUri}
												</code>
											</p>
										</div>

										<div className="flex items-center justify-center gap-2 text-sm text-gray-600">
											<Loader2 className="w-4 h-4 animate-spin" />
											Waiting for authorization...
										</div>

										<Button
											type="button"
											variant="outline"
											size="sm"
											onClick={async () => {
												if (pollingInterval) {
													clearInterval(pollingInterval);
													setPollingInterval(null);
												}
												// Clean up the OAuth session on backend
												if (userCode) {
													try {
														await infrastructureApi.deleteGitHubOAuthSession(
															userCode,
														);
													} catch (error) {
														console.error(
															"Failed to cleanup OAuth session:",
															error,
														);
													}
												}
												setDeviceFlowInProgress(false);
												setUserCode("");
												setVerificationUri("");
											}}
											className="w-full"
										>
											Cancel
										</Button>
									</div>
								) : (
									<div className="space-y-2">
										<div className="flex items-center gap-2 text-green-600">
											<Check className="w-4 h-4" />
											<span className="text-sm font-medium">
												GitHub authenticated successfully
											</span>
										</div>

										<p className="text-xs text-gray-600">
											Token stored in SSM:{" "}
											<code className="bg-gray-800 text-gray-300 px-1 rounded">
												{ssmParameterPath}
											</code>
										</p>

										<Button
											type="button"
											size="sm"
											variant="outline"
											onClick={() => {
												setTokenSaved(false);
												setFormData({ ...formData, github_oauth_token: "" });
											}}
										>
											Re-authenticate
										</Button>
									</div>
								)}

								{tokenError && (
									<div className="flex items-center gap-2 text-sm text-red-500">
										<AlertCircle className="w-4 h-4" />
										{tokenError}
									</div>
								)}

								<div className="pt-2 border-t space-y-1">
									<p className="text-xs text-gray-500">
										Required scope:{" "}
										<code className="bg-gray-800 text-gray-300 px-1 rounded">
											repo
										</code>{" "}
										(for private repository access)
									</p>
									<p className="text-xs text-gray-500">
										Using GitHub Device Flow - no backend or client secret
										required
									</p>
								</div>
							</div>

							{errors.github_oauth_token && (
								<p className="text-xs text-red-500">
									{errors.github_oauth_token}
								</p>
							)}
						</div>
					</div>

					{/* Branches Configuration */}
					<div className="grid gap-2">
						<div className="flex items-center justify-between">
							<h3 className="text-sm font-semibold">Branches *</h3>
							<Button
								type="button"
								size="sm"
								onClick={addBranch}
								variant="outline"
							>
								<Plus className="w-4 h-4 mr-1" />
								Add Branch
							</Button>
						</div>

						{errors.branches && (
							<p className="text-xs text-red-500">{errors.branches}</p>
						)}

						<div className="space-y-3">
							{formData.branches.map((branch, index) => (
								<div
									key={branch.name || `branch-${index}`}
									className="border rounded-lg p-4 space-y-3"
								>
									<div className="flex items-center justify-between">
										<h4 className="text-sm font-medium">Branch {index + 1}</h4>
										{formData.branches.length > 1 && (
											<Button
												type="button"
												size="icon"
												variant="ghost"
												onClick={() => removeBranch(index)}
											>
												<Trash2 className="w-4 h-4" />
											</Button>
										)}
									</div>

									<div className="grid grid-cols-2 gap-3">
										<div>
											<Label htmlFor={`branch-name-${index}`}>
												Branch Name
											</Label>
											<Input
												id={`branch-name-${index}`}
												value={branch.name}
												onChange={(e) =>
													updateBranch(index, "name", e.target.value)
												}
												placeholder="main"
												className={`mt-1 ${
													errors[`branch_${index}_name`] ? "border-red-500" : ""
												}`}
											/>
											{errors[`branch_${index}_name`] && (
												<p className="text-xs text-red-500 mt-1">
													{errors[`branch_${index}_name`]}
												</p>
											)}
										</div>

										<div>
											<Label htmlFor={`branch-stage-${index}`}>Stage</Label>
											<Select
												value={branch.stage}
												onValueChange={(value) =>
													updateBranch(index, "stage", value)
												}
											>
												<SelectTrigger
													id={`branch-stage-${index}`}
													className="mt-1"
												>
													<SelectValue />
												</SelectTrigger>
												<SelectContent>
													<SelectItem value="PRODUCTION">Production</SelectItem>
													<SelectItem value="DEVELOPMENT">
														Development
													</SelectItem>
													<SelectItem value="BETA">Beta</SelectItem>
													<SelectItem value="EXPERIMENTAL">
														Experimental
													</SelectItem>
												</SelectContent>
											</Select>
										</div>
									</div>

									<div className="space-y-2">
										<div className="flex items-center space-x-2">
											<Checkbox
												id={`auto-build-${index}`}
												checked={branch.enable_auto_build}
												onCheckedChange={(checked) =>
													updateBranch(
														index,
														"enable_auto_build",
														Boolean(checked),
													)
												}
											/>
											<Label
												htmlFor={`auto-build-${index}`}
												className="font-normal"
											>
												Enable automatic builds on push
											</Label>
										</div>

										<div className="flex items-center space-x-2">
											<Checkbox
												id={`pr-preview-${index}`}
												checked={branch.enable_pull_request_preview}
												onCheckedChange={(checked) =>
													updateBranch(
														index,
														"enable_pull_request_preview",
														Boolean(checked),
													)
												}
											/>
											<Label
												htmlFor={`pr-preview-${index}`}
												className="font-normal"
											>
												Enable pull request previews
											</Label>
										</div>
									</div>

									<div>
										<Label htmlFor={`env-vars-${index}`}>
											Environment Variables
										</Label>
										<Textarea
											id={`env-vars-${index}`}
											value={branch.environment_variables_text || ""}
											onChange={(e) =>
												updateBranch(
													index,
													"environment_variables_text",
													e.target.value,
												)
											}
											placeholder="REACT_APP_API_URL=https://api.example.com&#10;REACT_APP_ENV=production"
											rows={3}
											className="mt-1"
										/>
										<p className="text-xs text-gray-500 mt-1">
											Enter one per line in KEY=VALUE format. Use ${"{"}variable
											{"}"} for interpolation.
										</p>
									</div>

									<div>
										<Label htmlFor={`custom-subdomains-${index}`}>
											Custom Subdomains
										</Label>
										<Input
											id={`custom-subdomains-${index}`}
											value={branch.custom_subdomains_text || ""}
											onChange={(e) =>
												updateBranch(
													index,
													"custom_subdomains_text",
													e.target.value,
												)
											}
											placeholder="api, staging, beta"
											className="mt-1"
										/>
										<p className="text-xs text-gray-500 mt-1">
											Enter subdomain prefixes separated by commas. These will
											map specifically to this branch.
										</p>
									</div>
								</div>
							))}
						</div>
					</div>

					{/* Amplify Build Information */}
					<div className="bg-blue-950/30 rounded-lg p-3 border border-blue-900/50">
						<p className="text-sm text-blue-200">
							<strong>Amplify Auto-Detection:</strong> Amplify will
							automatically detect your framework and build settings.
						</p>
						<p className="text-xs text-blue-100 mt-2">
							For custom builds, create an{" "}
							<code className="bg-blue-900/50 px-1 rounded">amplify.yml</code>{" "}
							file in your repository.
						</p>
					</div>

					{/* Domain Configuration */}
					<div className="grid gap-2">
						<h3 className="text-sm font-semibold">
							Domain Configuration
						</h3>

						<div>
							<Label htmlFor="subdomain_prefix">Subdomain Prefix</Label>
							<Input
								id="subdomain_prefix"
								value={formData.subdomain_prefix}
								onChange={(e) =>
									setFormData({ ...formData, subdomain_prefix: e.target.value })
								}
								placeholder="app"
								className="mt-1"
							/>
							{smartDefaults.suggestedSubdomain && formData.subdomain_prefix === smartDefaults.suggestedSubdomain && (
								<p className="text-xs text-blue-400 mt-1">
									💡 Pre-filled based on project name
								</p>
							)}

							{/* Domain Preview */}
							{formData.subdomain_prefix && config?.domain?.enabled && config?.domain?.domain_name && (
								<div className="mt-3 p-3 bg-gray-800/50 border border-gray-700 rounded-lg">
									<div className="flex items-center gap-2 mb-1">
										<Globe className="w-4 h-4 text-blue-400" />
										<span className="text-xs font-medium text-gray-400">Domain Preview</span>
									</div>
									<p className="text-sm text-white font-mono">
										{calculateDomainPreview(formData.subdomain_prefix, config)}
									</p>
									<p className="text-xs text-gray-500 mt-1">
										Automatically constructed from domain configuration
									</p>
								</div>
							)}

							{!config?.domain?.enabled && (
								<p className="text-xs text-amber-400 mt-1">
									ℹ️ Enable domain configuration to use custom domains
								</p>
							)}
						</div>
					</div>
				</div>

				<DialogFooter>
					<Button variant="outline" onClick={handleClose}>
						Cancel
					</Button>
					<Button onClick={handleSubmit}>Add Amplify App</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
};
