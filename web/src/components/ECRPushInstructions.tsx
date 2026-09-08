import { Check, Copy, Info, Terminal } from "lucide-react";
import { useState } from "react";
import type { AccountInfo } from "../api/infrastructure";
import type { YamlInfrastructureConfig } from "../types/yamlConfig";
import { Button } from "./ui/button";
import {
	Card,
	CardContent,
	CardDescription,
	CardHeader,
	CardTitle,
} from "./ui/card";

interface ECRPushInstructionsProps {
	config: YamlInfrastructureConfig;
	accountInfo?: AccountInfo;
}

export function ECRPushInstructions({
	config,
	accountInfo,
}: ECRPushInstructionsProps) {
	const [copiedCommand, setCopiedCommand] = useState<string | null>(null);

	const handleCopyCommand = (command: string, id: string) => {
		navigator.clipboard.writeText(command);
		setCopiedCommand(id);
		setTimeout(() => setCopiedCommand(null), 2000);
	};

	const isUsingCrossAccount = !!config.ecr_account_id;
	const accountId = isUsingCrossAccount
		? config.ecr_account_id
		: accountInfo?.accountId;
	const region =
		isUsingCrossAccount && config.ecr_account_region
			? config.ecr_account_region
			: config.region;
	const repositoryName = `${config.project}_backend`; // Example repository

	if (!accountId) {
		return (
			<div className="flex items-center justify-center py-8">
				<p className="text-sm text-gray-400">Loading account information...</p>
			</div>
		);
	}

	const ecrUri = `${accountId}.dkr.ecr.${region}.amazonaws.com`;
	const fullRepositoryUri = `${ecrUri}/${repositoryName}`;

	// Every push here carries TWO tags, and the immutable one is not optional.
	//
	// meroku's CI Lambda deploys on an ECR push, and the EventBridge rule that
	// triggers it deliberately excludes ":latest" (see the ci_ecr_pattern_*
	// locals in modules/workloads/lambda.tf). A push registers a task-definition
	// revision that PINS the image it carried, so pinning a mutable tag would
	// produce a revision that resolves to a different image tomorrow — not a
	// rollback point. This panel used to emit a ":latest"-only push, which under
	// that rule means no deployment at all, with no error anywhere: the push
	// succeeds, EventBridge matches nothing, and the first symptom is a service
	// that quietly stopped picking up builds.
	//
	// The tag is derived in the user's shell rather than here because the browser
	// cannot see their working copy. A short commit SHA when there is one, a UTC
	// timestamp when there is not, so it works outside a git repository too.
	const commands = {
		login: `aws ecr get-login-password --region ${region} | docker login --username AWS --password-stdin ${ecrUri}`,
		build: `docker build -t ${repositoryName} .`,
		imageTag:
			"IMAGE_TAG=$(git rev-parse --short HEAD 2>/dev/null || date -u +%Y%m%d-%H%M%S)",
		tag: [
			`docker tag ${repositoryName}:latest ${fullRepositoryUri}:$IMAGE_TAG`,
			`docker tag ${repositoryName}:latest ${fullRepositoryUri}:latest`,
		].join("\n"),
		push: [
			`docker push ${fullRepositoryUri}:$IMAGE_TAG`,
			`docker push ${fullRepositoryUri}:latest`,
		].join("\n"),
	};

	// Built once and used for both the rendered block and the copy button. They
	// used to be two hand-written copies of the same script, so the button could
	// hand you something different from what you had just read.
	const allInOneScript = [
		"#!/bin/bash",
		"set -e",
		"",
		"# Login to ECR",
		commands.login,
		"",
		"# Build the image",
		commands.build,
		"",
		"# Derive an immutable tag: the commit SHA, or a UTC timestamp outside a repo.",
		"# This is the tag meroku deploys — :latest alone triggers nothing.",
		commands.imageTag,
		"",
		"# Tag the image, both for the deploy and as the moving pointer",
		commands.tag,
		"",
		"# Push to ECR",
		commands.push,
	].join("\n");

	return (
		<div className="space-y-4">
			<Card>
				<CardHeader>
					<CardTitle>Push to ECR</CardTitle>
					<CardDescription>
						Step-by-step instructions to push Docker images to your ECR
						repository
					</CardDescription>
				</CardHeader>
				<CardContent className="space-y-6">
					{/* Prerequisites */}
					<div className="bg-blue-900/20 border border-blue-700 rounded-lg p-4">
						<div className="flex items-start gap-2">
							<Info className="w-4 h-4 text-blue-400 mt-0.5" />
							<div className="flex-1">
								<h4 className="text-sm font-medium text-blue-400 mb-2">
									Prerequisites
								</h4>
								<ul className="text-xs text-gray-300 space-y-1">
									<li>
										• AWS CLI installed and configured with appropriate
										credentials
									</li>
									<li>• Docker installed and running</li>
									<li>
										• ECR repository created (happens automatically in dev
										environment)
									</li>
									{isUsingCrossAccount && (
										<li>
											• Cross-account permissions configured for ECR access
										</li>
									)}
								</ul>
							</div>
						</div>
					</div>

					{/* Step 1: Login */}
					<div className="space-y-2">
						<div className="flex items-center gap-2">
							<div className="w-6 h-6 rounded-full bg-blue-500 text-white text-xs flex items-center justify-center font-bold">
								1
							</div>
							<h3 className="text-sm font-medium">
								Authenticate Docker to ECR
							</h3>
						</div>
						<div className="ml-8">
							<p className="text-xs text-gray-400 mb-2">
								Login to ECR using AWS CLI:
							</p>
							<div className="bg-gray-800 rounded-lg p-3 flex items-start gap-2">
								<Terminal className="w-4 h-4 text-gray-400 mt-0.5" />
								<code className="text-xs text-gray-300 flex-1 break-all">
									{commands.login}
								</code>
								<Button
									size="icon"
									variant="ghost"
									className="h-6 w-6"
									onClick={() => handleCopyCommand(commands.login, "login")}
								>
									{copiedCommand === "login" ? (
										<Check className="h-3 w-3 text-green-400" />
									) : (
										<Copy className="h-3 w-3" />
									)}
								</Button>
							</div>
						</div>
					</div>

					{/* Step 2: Build */}
					<div className="space-y-2">
						<div className="flex items-center gap-2">
							<div className="w-6 h-6 rounded-full bg-blue-500 text-white text-xs flex items-center justify-center font-bold">
								2
							</div>
							<h3 className="text-sm font-medium">Build Docker Image</h3>
						</div>
						<div className="ml-8">
							<p className="text-xs text-gray-400 mb-2">
								Build your Docker image (run from your project directory):
							</p>
							<div className="bg-gray-800 rounded-lg p-3 flex items-start gap-2">
								<Terminal className="w-4 h-4 text-gray-400 mt-0.5" />
								<code className="text-xs text-gray-300 flex-1">
									{commands.build}
								</code>
								<Button
									size="icon"
									variant="ghost"
									className="h-6 w-6"
									onClick={() => handleCopyCommand(commands.build, "build")}
								>
									{copiedCommand === "build" ? (
										<Check className="h-3 w-3 text-green-400" />
									) : (
										<Copy className="h-3 w-3" />
									)}
								</Button>
							</div>
						</div>
					</div>

					{/* Step 3: Immutable tag */}
					<div className="space-y-2">
						<div className="flex items-center gap-2">
							<div className="w-6 h-6 rounded-full bg-blue-500 text-white text-xs flex items-center justify-center font-bold">
								3
							</div>
							<h3 className="text-sm font-medium">Choose an Immutable Tag</h3>
						</div>
						<div className="ml-8">
							<p className="text-xs text-gray-400 mb-2">
								This is the tag meroku deploys. Run it in the same shell as the
								next two steps — they both use{" "}
								<code className="text-gray-300">$IMAGE_TAG</code>. It resolves
								to the short commit SHA, or to a UTC timestamp outside a git
								repository:
							</p>
							<div className="bg-gray-800 rounded-lg p-3 flex items-start gap-2">
								<Terminal className="w-4 h-4 text-gray-400 mt-0.5" />
								<code className="text-xs text-gray-300 flex-1 break-all">
									{commands.imageTag}
								</code>
								<Button
									size="icon"
									variant="ghost"
									className="h-6 w-6"
									onClick={() =>
										handleCopyCommand(commands.imageTag, "image-tag")
									}
								>
									{copiedCommand === "image-tag" ? (
										<Check className="h-3 w-3 text-green-400" />
									) : (
										<Copy className="h-3 w-3" />
									)}
								</Button>
							</div>
						</div>
					</div>

					{/* Step 4: Tag */}
					<div className="space-y-2">
						<div className="flex items-center gap-2">
							<div className="w-6 h-6 rounded-full bg-blue-500 text-white text-xs flex items-center justify-center font-bold">
								4
							</div>
							<h3 className="text-sm font-medium">Tag Image for ECR</h3>
						</div>
						<div className="ml-8">
							<p className="text-xs text-gray-400 mb-2">
								Tag your image with the ECR repository URI, twice — once with
								the immutable tag and once as{" "}
								<code className="text-gray-300">:latest</code>:
							</p>
							<div className="bg-gray-800 rounded-lg p-3 flex items-start gap-2">
								<Terminal className="w-4 h-4 text-gray-400 mt-0.5" />
								<code className="text-xs text-gray-300 flex-1 whitespace-pre-wrap break-all">
									{commands.tag}
								</code>
								<Button
									size="icon"
									variant="ghost"
									className="h-6 w-6"
									onClick={() => handleCopyCommand(commands.tag, "tag")}
								>
									{copiedCommand === "tag" ? (
										<Check className="h-3 w-3 text-green-400" />
									) : (
										<Copy className="h-3 w-3" />
									)}
								</Button>
							</div>
						</div>
					</div>

					{/* Step 5: Push */}
					<div className="space-y-2">
						<div className="flex items-center gap-2">
							<div className="w-6 h-6 rounded-full bg-blue-500 text-white text-xs flex items-center justify-center font-bold">
								5
							</div>
							<h3 className="text-sm font-medium">Push to ECR</h3>
						</div>
						<div className="ml-8">
							<p className="text-xs text-gray-400 mb-2">
								Push both tags. Order does not matter:
							</p>
							<div className="bg-gray-800 rounded-lg p-3 flex items-start gap-2">
								<Terminal className="w-4 h-4 text-gray-400 mt-0.5" />
								<code className="text-xs text-gray-300 flex-1 whitespace-pre-wrap break-all">
									{commands.push}
								</code>
								<Button
									size="icon"
									variant="ghost"
									className="h-6 w-6"
									onClick={() => handleCopyCommand(commands.push, "push")}
								>
									{copiedCommand === "push" ? (
										<Check className="h-3 w-3 text-green-400" />
									) : (
										<Copy className="h-3 w-3" />
									)}
								</Button>
							</div>
						</div>
					</div>

					{/* Why two tags */}
					<div className="bg-blue-900/20 border border-blue-700 rounded-lg p-4">
						<div className="flex items-start gap-2">
							<Info className="w-4 h-4 text-blue-400 mt-0.5" />
							<div className="flex-1">
								<h4 className="text-sm font-medium text-blue-400 mb-2">
									Why two tags?
								</h4>
								<ul className="text-xs text-gray-300 space-y-1">
									<li>
										• <code className="text-blue-300">$IMAGE_TAG</code> is what
										triggers the deployment. meroku registers a task-definition
										revision pinned to this exact image, so the revision stays a
										rollback point — it still names the same image months later.
									</li>
									<li>
										• <code className="text-blue-300">:latest</code> is only a
										moving pointer, for humans and for the bootstrap value in a
										freshly created task definition.
									</li>
									<li>
										• Pushes tagged{" "}
										<code className="text-blue-300">:latest</code> are
										deliberately ignored by the deploy trigger. A push with{" "}
										<strong>only</strong> that tag succeeds and deploys nothing,
										without an error anywhere — which is why the immutable tag
										is a requirement, not a nicety.
									</li>
								</ul>
							</div>
						</div>
					</div>

					{/* All-in-one script */}
					<div className="mt-6 space-y-2">
						<h3 className="text-sm font-medium text-gray-300">
							All-in-One Script
						</h3>
						<p className="text-xs text-gray-400">
							Run all commands in sequence. This is the copy to use if you are
							only going to take one — it derives the immutable tag itself:
						</p>
						<div className="bg-gray-800 rounded-lg p-3">
							<div className="flex items-start gap-2 mb-2">
								<Terminal className="w-4 h-4 text-gray-400 mt-0.5" />
								<code className="text-xs text-gray-300 flex-1 whitespace-pre-wrap break-all">
									{allInOneScript}
								</code>
								<Button
									size="icon"
									variant="ghost"
									className="h-6 w-6"
									onClick={() => handleCopyCommand(allInOneScript, "script")}
								>
									{copiedCommand === "script" ? (
										<Check className="h-3 w-3 text-green-400" />
									) : (
										<Copy className="h-3 w-3" />
									)}
								</Button>
							</div>
						</div>
					</div>

					{/* Additional tips */}
					<div className="bg-yellow-900/20 border border-yellow-700 rounded-lg p-4">
						<h4 className="text-sm font-medium text-yellow-400 mb-2">Tips</h4>
						<ul className="text-xs text-gray-300 space-y-1">
							<li>
								• Any immutable tag works in place of the commit SHA — a release
								number like <code className="text-yellow-300">:v1.0.0</code>, a
								build number, a branch name. Anything except{" "}
								<code className="text-yellow-300">:latest</code>, which the
								deploy trigger ignores.
							</li>
							<li>
								• Use <code className="text-yellow-300">docker images</code> to
								see all local images
							</li>
							<li>
								• Use{" "}
								<code className="text-yellow-300">
									aws ecr describe-images --repository-name {repositoryName}
								</code>{" "}
								to list pushed images
							</li>
							<li>
								• ECR automatically scans images for vulnerabilities if enabled
							</li>
						</ul>
					</div>
				</CardContent>
			</Card>
		</div>
	);
}
