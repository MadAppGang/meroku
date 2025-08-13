import {
	AlertCircle,
	Bell,
	Check,
	Eye,
	EyeOff,
	Info,
	Key,
	Loader2,
	RefreshCw,
} from "lucide-react";
import { useState } from "react";
import { infrastructureApi } from "../api/infrastructure";
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

interface SNSNodePropertiesProps {
	config: YamlInfrastructureConfig;
	onConfigChange: (config: Partial<YamlInfrastructureConfig>) => void;
}

export function SNSNodeProperties({
	config,
	onConfigChange,
}: SNSNodePropertiesProps) {
	const isEnabled = config.workload?.setup_fcnsns === true;
	const parameterPath = `/${config.env}/${config.project}/backend/gcm-server-key`;

	const [gcmServerKey, setGcmServerKey] = useState("");
	const [showKey, setShowKey] = useState(false);
	const [loading, setLoading] = useState(false);
	const [saving, setSaving] = useState(false);
	const [error, setError] = useState<string | null>(null);
	const [success, setSuccess] = useState(false);
	const [hasLoaded, setHasLoaded] = useState(false);

	const loadParameter = async () => {
		try {
			setLoading(true);
			setError(null);
			const param = await infrastructureApi.getSSMParameter(parameterPath);
			setGcmServerKey(param.value || "");
			setHasLoaded(true);
		} catch (err) {
			// If parameter doesn't exist, that's OK - user can create it
			if (err instanceof Error && err.message.includes("ParameterNotFound")) {
				setGcmServerKey("");
				setHasLoaded(true);
			} else {
				setError("Failed to load parameter");
			}
		} finally {
			setLoading(false);
		}
	};

	const handleToggle = (checked: boolean) => {
		onConfigChange({
			workload: {
				...config.workload,
				setup_fcnsns: checked,
			},
		});
		// Reset state when toggling
		if (!checked) {
			setHasLoaded(false);
			setGcmServerKey("");
			setError(null);
			setSuccess(false);
		}
	};

	const handleSaveKey = async () => {
		try {
			setSaving(true);
			setError(null);
			setSuccess(false);

			await infrastructureApi.createOrUpdateSSMParameter({
				name: parameterPath,
				value: gcmServerKey,
				type: "SecureString",
				overwrite: true,
			});

			setSuccess(true);
			setTimeout(() => setSuccess(false), 3000);
		} catch (err) {
			setError(err instanceof Error ? err.message : "Failed to save parameter");
		} finally {
			setSaving(false);
		}
	};

	return (
		<Card className="w-full">
			<CardHeader>
				<CardTitle className="flex items-center gap-2">
					<Bell className="w-5 h-5" />
					Amazon SNS Configuration
				</CardTitle>
				<CardDescription>
					Firebase Cloud Messaging (FCM) push notification settings
				</CardDescription>
			</CardHeader>
			<CardContent className="space-y-4">
				<div className="flex items-center justify-between">
					<div className="flex-1">
						<Label htmlFor="sns-enabled">Enable SNS/FCM</Label>
						<p className="text-xs text-gray-500 mt-1">
							Enable Firebase Cloud Messaging platform application for push
							notifications
						</p>
					</div>
					<Switch
						id="sns-enabled"
						checked={isEnabled}
						onCheckedChange={handleToggle}
						className="data-[state=checked]:bg-blue-500 data-[state=unchecked]:bg-gray-600"
					/>
				</div>

				{isEnabled && (
					<>
						<div className="mt-4 p-3 bg-gray-800 rounded-lg space-y-3">
							<h4 className="text-sm font-medium text-gray-200">
								Configuration Details
							</h4>

							<div className="space-y-2 text-sm">
								<div className="flex items-start gap-2">
									<span className="text-gray-400 min-w-[140px]">
										Platform App Name:
									</span>
									<code className="text-pink-400 font-mono">
										{config.project}-fcm-{config.env}
									</code>
								</div>

								<div className="flex items-start gap-2">
									<span className="text-gray-400 min-w-[140px]">Platform:</span>
									<span className="text-gray-200">
										GCM (Google Cloud Messaging)
									</span>
								</div>

								<div className="flex items-start gap-2">
									<span className="text-gray-400 min-w-[140px]">
										Server Key Path:
									</span>
									<code className="text-blue-400 font-mono text-xs break-all">
										/{config.env}/{config.project}/backend/gcm-server-key
									</code>
								</div>
							</div>
						</div>

						<div className="p-3 bg-gray-800 rounded-lg space-y-3">
							<div className="flex items-center justify-between">
								<h4 className="text-sm font-medium text-gray-200">
									GCM/FCM Server Key
								</h4>
								{hasLoaded && (
									<Button
										onClick={loadParameter}
										disabled={loading}
										size="sm"
										variant="ghost"
										className="h-6 px-2"
									>
										<RefreshCw
											className={`w-3 h-3 ${loading ? "animate-spin" : ""}`}
										/>
									</Button>
								)}
							</div>

							{!hasLoaded ? (
								<div className="space-y-3">
									<div className="flex items-center gap-2">
										<div className="relative flex-1">
											<Input
												type={showKey ? "text" : "password"}
												value={gcmServerKey}
												onChange={(e) => setGcmServerKey(e.target.value)}
												placeholder="Enter your FCM server key"
												className="bg-gray-900 border-gray-700 text-white pr-10"
											/>
											<button
												type="button"
												onClick={() => setShowKey(!showKey)}
												className="absolute right-2 top-1/2 -translate-y-1/2 text-gray-400 hover:text-gray-300"
											>
												{showKey ? (
													<EyeOff className="w-4 h-4" />
												) : (
													<Eye className="w-4 h-4" />
												)}
											</button>
										</div>
										<Button
											onClick={handleSaveKey}
											disabled={saving || !gcmServerKey}
											size="sm"
											className="min-w-[80px]"
										>
											{saving ? (
												<Loader2 className="w-4 h-4 animate-spin" />
											) : success ? (
												<>
													<Check className="w-4 h-4 mr-1" />
													Saved
												</>
											) : (
												"Upload"
											)}
										</Button>
									</div>

									<div className="flex items-center justify-between text-xs">
										<p className="text-gray-500">
											<Key className="w-3 h-3 inline mr-1" />
											Will be stored at:{" "}
											<code className="text-blue-400">{parameterPath}</code>
										</p>
										<Button
											onClick={loadParameter}
											disabled={loading}
											size="sm"
											variant="ghost"
											className="h-6 px-2 text-xs"
										>
											{loading ? (
												<>
													<Loader2 className="w-3 h-3 mr-1 animate-spin" />
													Loading...
												</>
											) : (
												<>
													<Eye className="w-3 h-3 mr-1" />
													Show Existing
												</>
											)}
										</Button>
									</div>

									{error && (
										<Alert className="border-red-600">
											<AlertCircle className="h-4 w-4 text-red-600" />
											<AlertDescription className="text-xs">
												{error}
											</AlertDescription>
										</Alert>
									)}
								</div>
							) : loading ? (
								<div className="flex items-center justify-center py-4">
									<Loader2 className="w-5 h-5 animate-spin text-gray-400" />
								</div>
							) : (
								<>
									<div className="flex items-center gap-2">
										<div className="relative flex-1">
											<Input
												type={showKey ? "text" : "password"}
												value={gcmServerKey}
												onChange={(e) => setGcmServerKey(e.target.value)}
												placeholder="Enter your FCM server key"
												className="bg-gray-900 border-gray-700 text-white pr-10"
											/>
											<button
												type="button"
												onClick={() => setShowKey(!showKey)}
												className="absolute right-2 top-1/2 -translate-y-1/2 text-gray-400 hover:text-gray-300"
											>
												{showKey ? (
													<EyeOff className="w-4 h-4" />
												) : (
													<Eye className="w-4 h-4" />
												)}
											</button>
										</div>
										<Button
											onClick={handleSaveKey}
											disabled={saving || !gcmServerKey}
											size="sm"
											className="min-w-[80px]"
										>
											{saving ? (
												<Loader2 className="w-4 h-4 animate-spin" />
											) : success ? (
												<>
													<Check className="w-4 h-4 mr-1" />
													Saved
												</>
											) : (
												"Update"
											)}
										</Button>
									</div>

									<p className="text-xs text-gray-500">
										<Key className="w-3 h-3 inline mr-1" />
										Stored at:{" "}
										<code className="text-blue-400">{parameterPath}</code>
									</p>

									{error && (
										<Alert className="border-red-600">
											<AlertCircle className="h-4 w-4 text-red-600" />
											<AlertDescription className="text-xs">
												{error}
											</AlertDescription>
										</Alert>
									)}
								</>
							)}
						</div>

						<div className="p-3 bg-gray-800 rounded-lg">
							<h4 className="text-sm font-medium text-gray-200 mb-2">
								IAM Permissions
							</h4>
							<p className="text-xs text-gray-400 mb-2">
								The backend service will have these SNS permissions:
							</p>
							<ul className="text-xs text-gray-400 space-y-1 ml-4">
								<li>• sns:Publish - Send push notifications</li>
								<li>• sns:CreatePlatformEndpoint - Register device tokens</li>
								<li>• sns:DeleteEndpoint - Remove device tokens</li>
								<li>• sns:GetEndpointAttributes - Get endpoint details</li>
								<li>• sns:SetEndpointAttributes - Update endpoint settings</li>
							</ul>
						</div>
					</>
				)}

				<Alert className="border-blue-600">
					<Info className="h-4 w-4 text-blue-600" />
					<AlertDescription className="text-xs">
						<strong>Note:</strong> This configuration is specifically for
						Firebase Cloud Messaging (FCM) push notifications. For other SNS
						features like topics, email subscriptions, or SMS, additional
						Terraform configuration would be required.
					</AlertDescription>
				</Alert>
			</CardContent>
		</Card>
	);
}
