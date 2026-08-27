import { Info, Plus, X } from "lucide-react";
import { useEffect, useId, useState } from "react";
import { infrastructureApi } from "../api/infrastructure";
import type { YamlInfrastructureConfig } from "../types/yamlConfig";
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

interface GitHubNodePropertiesProps {
	config: YamlInfrastructureConfig;
	onConfigChange: (config: Partial<YamlInfrastructureConfig>) => void;
}

export function GitHubNodeProperties({
	config,
	onConfigChange,
}: GitHubNodePropertiesProps) {
	const uid = useId();
	const isEnabled = config.workload?.enable_github_oidc ?? false;
	const subjects = config.workload?.github_oidc_subjects || [];

	// AWS holds one GitHub OIDC provider per account, so enabling this in a
	// second project that shares an account used to fail the apply with
	// EntityAlreadyExists and offered nothing to do about it. Resolve it here,
	// while the user is looking at the setting, rather than at deploy time.
	const [providerStatus, setProviderStatus] = useState<Awaited<
		ReturnType<typeof infrastructureApi.getGitHubOIDCStatus>
	> | null>(null);
	const [providerError, setProviderError] = useState<string | null>(null);

	useEffect(() => {
		if (!isEnabled || !config.env) {
			setProviderStatus(null);
			setProviderError(null);
			return;
		}

		// Guards against a slow response for one environment landing after the
		// user has already switched to another.
		let current = true;

		infrastructureApi
			.getGitHubOIDCStatus(config.env)
			.then((status) => {
				if (!current) return;
				setProviderStatus(status);
				setProviderError(null);
			})
			.catch((err: Error) => {
				if (!current) return;
				// A failed read is not evidence that no provider exists, so this
				// reports the failure and changes nothing.
				setProviderStatus(null);
				setProviderError(err.message);
			});

		return () => {
			current = false;
		};
	}, [isEnabled, config.env]);

	const handleToggleOIDC = (checked: boolean) => {
		onConfigChange({
			workload: {
				enable_github_oidc: checked,
				github_oidc_subjects:
					checked && subjects.length === 0
						? ["repo:Owner/Repo:ref:refs/heads/main"]
						: subjects,
			},
		});
	};

	const handleAddSubject = () => {
		const newSubjects = [...subjects, "repo:Owner/Repo:ref:refs/heads/main"];
		onConfigChange({
			workload: {
				github_oidc_subjects: newSubjects,
			},
		});
	};

	const handleRemoveSubject = (index: number) => {
		const newSubjects = subjects.filter((_, i) => i !== index);
		onConfigChange({
			workload: {
				github_oidc_subjects: newSubjects,
			},
		});
	};

	const handleUpdateSubject = (index: number, value: string) => {
		const newSubjects = [...subjects];
		newSubjects[index] = value;
		onConfigChange({
			workload: {
				github_oidc_subjects: newSubjects,
			},
		});
	};

	return (
		<div className="space-y-6">
			<Card>
				<CardHeader>
					<CardTitle>GitHub Actions Configuration</CardTitle>
					<CardDescription>
						Configure GitHub OIDC for passwordless deployments
					</CardDescription>
				</CardHeader>
				<CardContent className="space-y-4">
					<div className="flex items-center justify-between">
						<div className="space-y-1">
							<Label htmlFor={`${uid}-github-oidc`}>Enable GitHub OIDC</Label>
							<p className="text-xs text-gray-400">
								Allow GitHub Actions to deploy without credentials
							</p>
						</div>
						<Switch
							id={`${uid}-github-oidc`}
							checked={isEnabled}
							onCheckedChange={handleToggleOIDC}
						/>
					</div>

					{isEnabled &&
						providerStatus &&
						!providerStatus.owned_by_this_env &&
						providerStatus.exists && (
							<div className="rounded-lg border border-blue-500/40 bg-blue-500/10 p-3">
								<div className="flex gap-2">
									<Info className="w-4 h-4 text-blue-400 shrink-0 mt-0.5" />
									<div className="space-y-2 text-xs">
										<p className="text-gray-200">
											This AWS account already trusts GitHub
											{providerStatus.owner_project
												? `, through the ${providerStatus.owner_project} project`
												: ""}
											. AWS allows one GitHub identity provider per account, so
											this environment will reuse it instead of creating a
											second.
										</p>
										{providerStatus.arn && (
											<code className="block break-all text-blue-300">
												{providerStatus.arn}
											</code>
										)}
										<p className="text-gray-400">
											Deployments from this project stay isolated: they use
											their own role, scoped to the subjects below.
										</p>
										{providerStatus.changed && (
											<p className="text-gray-400">
												Set{" "}
												<code className="text-blue-300">
													github_oidc_create_provider: false
												</code>{" "}
												in{" "}
												<code className="text-blue-300">{config.env}.yaml</code>
												{providerStatus.backup
													? ` (backup: ${providerStatus.backup})`
													: ""}
												.
											</p>
										)}
									</div>
								</div>
							</div>
						)}

					{isEnabled && providerError && (
						<div className="rounded-lg border border-yellow-500/40 bg-yellow-500/10 p-3">
							<div className="flex gap-2">
								<Info className="w-4 h-4 text-yellow-400 shrink-0 mt-0.5" />
								<div className="space-y-1 text-xs">
									<p className="text-gray-200">
										Could not check this account's GitHub identity provider.
									</p>
									<p className="text-gray-400">{providerError}</p>
									<p className="text-gray-400">
										If the deploy fails with{" "}
										<code className="text-yellow-300">EntityAlreadyExists</code>
										, another project in this account owns the provider. Set{" "}
										<code className="text-yellow-300">
											github_oidc_create_provider: false
										</code>{" "}
										in{" "}
										<code className="text-yellow-300">{config.env}.yaml</code>.
									</p>
								</div>
							</div>
						</div>
					)}

					{isEnabled && (
						<div className="space-y-4 pt-4 border-t border-gray-700">
							<div>
								<div className="flex items-center justify-between mb-2">
									<Label>OIDC Subjects</Label>
									<Button
										size="sm"
										variant="outline"
										onClick={handleAddSubject}
										className="h-7 text-xs"
									>
										<Plus className="w-3 h-3 mr-1" />
										Add Subject
									</Button>
								</div>
								<p className="text-xs text-gray-400 mb-3">
									Define which GitHub repositories and branches can deploy
								</p>

								{subjects.length === 0 ? (
									<div className="text-sm text-gray-500 italic">
										No subjects configured. Add one to enable deployments.
									</div>
								) : (
									<div className="space-y-2">
										{subjects.map((subject, index) => (
											<div
												key={`subject-${index}-${subject}`}
												className="flex items-center gap-2"
											>
												<Input
													value={subject}
													onChange={(e) =>
														handleUpdateSubject(index, e.target.value)
													}
													placeholder="repo:Owner/Repo:ref:refs/heads/main"
													className="flex-1 bg-gray-800 border-gray-600 text-white text-sm"
												/>
												<Button
													size="icon"
													variant="ghost"
													onClick={() => handleRemoveSubject(index)}
													className="h-8 w-8 text-gray-400 hover:text-red-400"
												>
													<X className="w-4 h-4" />
												</Button>
											</div>
										))}
									</div>
								)}
							</div>

							<div className="rounded-lg bg-gray-800 p-3">
								<h4 className="text-sm font-medium text-gray-300 mb-2">
									Subject Format Examples:
								</h4>
								<ul className="space-y-1 text-xs text-gray-400">
									<li>
										•{" "}
										<code className="text-blue-400">
											repo:Owner/Repo:ref:refs/heads/main
										</code>{" "}
										- Main branch only
									</li>
									<li>
										•{" "}
										<code className="text-blue-400">
											repo:Owner/Repo:ref:refs/heads/*
										</code>{" "}
										- All branches
									</li>
									<li>
										•{" "}
										<code className="text-blue-400">
											repo:Owner/Repo:ref:refs/tags/*
										</code>{" "}
										- All tags
									</li>
									<li>
										•{" "}
										<code className="text-blue-400">
											repo:Owner/Repo:environment:production
										</code>{" "}
										- Specific environment
									</li>
								</ul>
							</div>
						</div>
					)}
				</CardContent>
			</Card>
		</div>
	);
}
