import { AlertCircle, CheckCircle2, Info } from "lucide-react";
import { useState, useEffect } from "react";
import type { AccountInfo } from "../api/infrastructure";
import { AWS_REGIONS } from "../types/config";
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
	const [accountIdError, setAccountIdError] = useState<string>("");

	// Validate account ID (must be 12 digits)
	const validateAccountId = (accountId: string): boolean => {
		if (!accountId) return false;
		const accountIdRegex = /^\d{12}$/;
		return accountIdRegex.test(accountId);
	};

	// Check if cross-account config is complete
	const isCrossAccountComplete =
		ecrMode === "cross-account" &&
		validateAccountId(config.ecr_account_id || "") &&
		config.ecr_account_region;

	// Generate ECR URL preview
	const getEcrUrl = () => {
		if (!config.ecr_account_id || !config.ecr_account_region) return "";
		return `${config.ecr_account_id}.dkr.ecr.${config.ecr_account_region}.amazonaws.com/${config.project}_backend`;
	};

	const handleModeChange = (value: string) => {
		const mode = value as "create" | "cross-account";
		setEcrMode(mode);

		if (mode === "create") {
			// Set to local strategy and clear cross-account settings
			setAccountIdError("");
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

	const handleAccountIdChange = (value: string) => {
		// Validate account ID format
		if (value && !validateAccountId(value)) {
			if (value.length > 12) {
				setAccountIdError("Account ID must be exactly 12 digits");
			} else if (!/^\d*$/.test(value)) {
				setAccountIdError("Account ID must contain only digits");
			} else {
				setAccountIdError("Account ID must be 12 digits");
			}
		} else {
			setAccountIdError("");
		}

		onConfigChange({
			...config,
			ecr_account_id: value || undefined,
		});
	};

	const handleRegionChange = (value: string) => {
		onConfigChange({
			...config,
			ecr_account_region: value || undefined,
		});
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
							{/* Configuration Status */}
							{isCrossAccountComplete ? (
								<div className="bg-green-900/20 border border-green-700 rounded-lg p-3">
									<div className="flex items-start gap-2">
										<CheckCircle2 className="w-4 h-4 text-green-400 mt-0.5" />
										<div className="flex-1">
											<div className="text-sm font-medium text-green-400">
												Cross-Account Configuration Complete
											</div>
											<div className="text-xs text-gray-300 mt-1">
												ECS tasks can pull images from the source account
											</div>
										</div>
									</div>
								</div>
							) : (
								<div className="bg-yellow-900/20 border border-yellow-700 rounded-lg p-3">
									<div className="flex items-start gap-2">
										<AlertCircle className="w-4 h-4 text-yellow-400 mt-0.5" />
										<div className="flex-1">
											<div className="text-sm font-medium text-yellow-400">
												Configuration Incomplete
											</div>
											<div className="text-xs text-gray-300 mt-1">
												Both Account ID and Region are required for
												cross-account access
											</div>
										</div>
									</div>
								</div>
							)}

							<div>
								<Label htmlFor="ecr-account-id">
									ECR Account ID{" "}
									<span className="text-red-400">*</span>
								</Label>
								<Input
									id="ecr-account-id"
									value={config.ecr_account_id || ""}
									onChange={(e) => handleAccountIdChange(e.target.value)}
									placeholder="123456789012"
									className={`mt-1 bg-gray-800 border-gray-600 text-white font-mono ${
										accountIdError ? "border-red-500" : ""
									}`}
									maxLength={12}
								/>
								{accountIdError ? (
									<p className="text-xs text-red-400 mt-1">{accountIdError}</p>
								) : (
									<p className="text-xs text-gray-400 mt-1">
										12-digit AWS account ID where ECR repository exists
									</p>
								)}
							</div>

							<div>
								<Label htmlFor="ecr-region">
									ECR Region <span className="text-red-400">*</span>
								</Label>
								<select
									id="ecr-region"
									value={config.ecr_account_region || config.region}
									onChange={(e) => handleRegionChange(e.target.value)}
									className="mt-1 w-full bg-gray-800 border-gray-600 text-white rounded-md px-3 py-2 text-sm"
								>
									{AWS_REGIONS.map((region) => (
										<option key={region.value} value={region.value}>
											{region.label} ({region.value})
										</option>
									))}
								</select>
								<p className="text-xs text-gray-400 mt-1">
									AWS region where the ECR repository is located
								</p>
							</div>

							{/* ECR URL Preview */}
							{getEcrUrl() && (
								<div className="bg-blue-900/20 border border-blue-700 rounded-lg p-3">
									<h4 className="text-xs font-medium text-blue-400 mb-2">
										ECR Repository URL
									</h4>
									<div className="bg-gray-800 rounded p-2">
										<code className="text-xs text-gray-300 break-all">
											{getEcrUrl()}
										</code>
									</div>
									<p className="text-xs text-gray-400 mt-2">
										Backend service will pull images from this URL
									</p>
								</div>
							)}

							{/* AWS Organizations Info */}
							<div className="bg-purple-900/20 border border-purple-700 rounded-lg p-3">
								<div className="flex items-start gap-2">
									<Info className="w-4 h-4 text-purple-400 mt-0.5" />
									<div className="flex-1">
										<h4 className="text-xs font-medium text-purple-400 mb-1">
											AWS Organizations Required
										</h4>
										<p className="text-xs text-gray-300">
											Both AWS accounts must be in the same AWS Organization
											for automatic cross-account access. The source ECR
											repository policy allows access to all accounts in the
											organization.
										</p>
									</div>
								</div>
							</div>

							{/* Required Permissions */}
							{config.ecr_account_id && validateAccountId(config.ecr_account_id) && (
								<div className="bg-yellow-900/20 border border-yellow-700 rounded-lg p-3">
									<h4 className="text-sm font-medium text-yellow-400 mb-2">
										Manual Setup (If Not Using AWS Organizations)
									</h4>
									<p className="text-xs text-gray-300 mb-2">
										If accounts are not in the same AWS Organization, add this
										policy to the source ECR repository:
									</p>
									<pre className="text-xs text-gray-400 overflow-x-auto bg-gray-800 p-2 rounded">
										{`{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Principal": {
      "AWS": "arn:aws:iam::${accountInfo?.accountId || "<THIS_ACCOUNT_ID>"}:root"
    },
    "Action": [
      "ecr:GetDownloadUrlForLayer",
      "ecr:BatchGetImage",
      "ecr:BatchCheckLayerAvailability",
      "ecr:DescribeImages",
      "ecr:DescribeRepositories"
    ]
  }]
}`}
									</pre>
								</div>
							)}
						</div>
					)}
				</CardContent>
			</Card>
		</div>
	);
}
