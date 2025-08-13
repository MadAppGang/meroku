import { Plus, X } from "lucide-react";
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
	const isEnabled = config.workload?.enable_github_oidc ?? false;
	const subjects = config.workload?.github_oidc_subjects || [];

	const handleToggleOIDC = (checked: boolean) => {
		onConfigChange({
			workload: {
				...config.workload,
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
				...config.workload,
				github_oidc_subjects: newSubjects,
			},
		});
	};

	const handleRemoveSubject = (index: number) => {
		const newSubjects = subjects.filter((_, i) => i !== index);
		onConfigChange({
			workload: {
				...config.workload,
				github_oidc_subjects: newSubjects,
			},
		});
	};

	const handleUpdateSubject = (index: number, value: string) => {
		const newSubjects = [...subjects];
		newSubjects[index] = value;
		onConfigChange({
			workload: {
				...config.workload,
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
							<Label htmlFor="github-oidc">Enable GitHub OIDC</Label>
							<p className="text-xs text-gray-400">
								Allow GitHub Actions to deploy without credentials
							</p>
						</div>
						<Switch
							id="github-oidc"
							checked={isEnabled}
							onCheckedChange={handleToggleOIDC}
						/>
					</div>

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
