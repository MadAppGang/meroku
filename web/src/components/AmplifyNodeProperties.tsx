import {
	AlertCircle,
	Check,
	Copy,
	ExternalLink,
	Github,
	Loader2,
} from "lucide-react";
import { useEffect, useState } from "react";
import { infrastructureApi } from "../api/infrastructure";
import type { YamlInfrastructureConfig } from "../types/yamlConfig";
import { Button } from "./ui/button";
import { Input } from "./ui/input";
import { Label } from "./ui/label";

interface AmplifyNodePropertiesProps {
	config: YamlInfrastructureConfig;
	nodeId: string;
	onConfigChange?: (config: Partial<YamlInfrastructureConfig>) => void;
}

export function AmplifyNodeProperties({
	config,
	nodeId,
	onConfigChange,
}: AmplifyNodePropertiesProps) {
	const appName = nodeId.replace("amplify-", "");
	const amplifyAppIndex =
		config.amplify_apps?.findIndex((app) => app.name === appName) ?? -1;
	const amplifyApp = config.amplify_apps?.[amplifyAppIndex];

	const [tokenSaved, setTokenSaved] = useState(false);
	const [checkingToken, setCheckingToken] = useState(false);
	const [deviceFlowInProgress, setDeviceFlowInProgress] = useState(false);
	const [userCode, setUserCode] = useState("");
	const [verificationUri, setVerificationUri] = useState("");
	const [pollingInterval, setPollingInterval] = useState<NodeJS.Timeout | null>(
		null,
	);
	const [tokenError, setTokenError] = useState("");

	useEffect(() => {
		if (amplifyApp) {
			checkExistingToken();
		}
	}, [amplifyApp, checkExistingToken]);

	useEffect(() => {
		return () => {
			if (pollingInterval) {
				clearInterval(pollingInterval);
			}
		};
	}, [pollingInterval]);

	const getSsmParameterPath = () => {
		return `/${config.project}/${config.env}/github/amplify-token`;
	};

	const checkExistingToken = async () => {
		setCheckingToken(true);
		try {
			const parameterPath = getSsmParameterPath();
			await infrastructureApi.getSSMParameter(parameterPath);
			setTokenSaved(true);
		} catch (_error) {
			setTokenSaved(false);
		} finally {
			setCheckingToken(false);
		}
	};

	const startGitHubDeviceFlow = async () => {
		setDeviceFlowInProgress(true);
		setTokenError("");
		setUserCode("");
		setVerificationUri("");

		try {
			const data = await infrastructureApi.initiateGitHubDeviceFlow({
				app_name: "shared",
				project: config.project,
				environment: config.env,
				scope: "repo",
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

			// Start polling for authorization
			const interval = setInterval(
				async () => {
					try {
						const status = await infrastructureApi.checkGitHubAuthStatus(
							data.user_code,
						);

						if (status.status === "authorized") {
							clearInterval(interval);
							setPollingInterval(null);
							setDeviceFlowInProgress(false);
							setTokenSaved(true);
							setUserCode("");
							setVerificationUri("");

							// Update config with SSM path
							handleChange("github_oauth_token", getSsmParameterPath());
						} else if (status.status === "expired") {
							clearInterval(interval);
							setPollingInterval(null);
							setTokenError("Device code expired. Please try again.");
							setDeviceFlowInProgress(false);
							setUserCode("");
							setVerificationUri("");
						} else if (status.status === "error") {
							clearInterval(interval);
							setPollingInterval(null);
							setTokenError(status.error || "Authentication failed");
							setDeviceFlowInProgress(false);
							setUserCode("");
							setVerificationUri("");
						}
					} catch (error) {
						console.error("Polling error:", error);
					}
				},
				(data.interval || 5) * 1000,
			);

			setPollingInterval(interval);
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

	if (!amplifyApp) {
		return (
			<div className="text-gray-400">
				<p>Amplify app configuration not found.</p>
			</div>
		);
	}

	const handleChange = (field: string, value: any) => {
		if (onConfigChange && config.amplify_apps) {
			const updatedApps = [...config.amplify_apps];
			updatedApps[amplifyAppIndex] = {
				...amplifyApp,
				[field]: value,
			};
			onConfigChange({ amplify_apps: updatedApps });
		}
	};

	const region = config.region || "us-east-1";
	const consoleUrl = `https://${region}.console.aws.amazon.com/amplify/home?region=${region}#/apps`;

	return (
		<div className="space-y-6">
			<div>
				<h3 className="text-sm font-medium text-white mb-4">
					General Information
				</h3>
				<div className="space-y-3">
					<div>
						<Label htmlFor="name">App Name</Label>
						<Input
							id="name"
							value={amplifyApp.name}
							className="mt-1 bg-gray-800 border-gray-600 text-white"
							disabled
						/>
						<p className="text-xs text-gray-500 mt-1">
							App name cannot be changed after creation
						</p>
					</div>

					<div>
						<Label htmlFor="github_repository">GitHub Repository URL</Label>
						<Input
							id="github_repository"
							value={amplifyApp.github_repository}
							onChange={(e) =>
								handleChange("github_repository", e.target.value)
							}
							placeholder="https://github.com/username/repo"
							className="mt-1 bg-gray-800 border-gray-600 text-white"
						/>
					</div>

					<div className="bg-gray-800 rounded-lg p-4 border border-gray-700">
						<div className="flex items-center justify-between">
							<div>
								<p className="text-sm font-medium text-white">Branches</p>
								<p className="text-xs text-gray-400 mt-1">
									{amplifyApp.branches?.length || 0} branch
									{(amplifyApp.branches?.length || 0) !== 1 ? "es" : ""}{" "}
									configured
								</p>
							</div>
							<p className="text-xs text-gray-500">
								Manage branches in the Branches tab
							</p>
						</div>
					</div>
				</div>
			</div>

			<div>
				<h3 className="text-sm font-medium text-white mb-4">Authentication</h3>
				<div className="border rounded-lg p-4 bg-gray-800 border-gray-700">
					{checkingToken ? (
						<div className="flex items-center gap-2 text-sm text-gray-400">
							<Loader2 className="w-4 h-4 animate-spin" />
							Checking existing token...
						</div>
					) : !tokenSaved && !userCode ? (
						<>
							<p className="text-sm text-gray-300">
								Authenticate with GitHub to grant Amplify access to your
								repository
							</p>

							<Button
								type="button"
								onClick={startGitHubDeviceFlow}
								disabled={deviceFlowInProgress}
								className="w-full"
							>
								{deviceFlowInProgress ? (
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
									<code className="text-2xl font-mono bg-gray-900 text-white px-4 py-2 rounded select-all border border-gray-700">
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
									Code copied to clipboard! A popup window should have opened
									with GitHub.
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
									if (userCode) {
										try {
											await infrastructureApi.deleteGitHubOAuthSession(
												userCode,
											);
										} catch (error) {
											console.error("Failed to cleanup OAuth session:", error);
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
								<code className="bg-gray-900 text-gray-300 px-1 rounded">
									{getSsmParameterPath()}
								</code>
							</p>

							<Button
								type="button"
								size="sm"
								variant="outline"
								onClick={() => {
									setTokenSaved(false);
									setTokenError("");
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
				</div>
			</div>

			<div>
				<h3 className="text-sm font-medium text-white mb-4">
					Management Console
				</h3>
				<a
					href={consoleUrl}
					target="_blank"
					rel="noopener noreferrer"
					className="flex items-center gap-2 text-blue-400 hover:text-blue-300 transition-colors"
				>
					<ExternalLink className="w-4 h-4" />
					<span className="text-sm">Open in AWS Console</span>
				</a>
			</div>
		</div>
	);
}
