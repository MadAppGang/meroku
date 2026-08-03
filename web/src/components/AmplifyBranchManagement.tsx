import { Check, GitBranch, Plus, Trash2, X } from "lucide-react";
import { useEffect, useId, useState } from "react";
import type { AmplifyBranch, BranchUpdateHandler } from "../types/components";
import type { YamlInfrastructureConfig } from "../types/yamlConfig";
import { Button } from "./ui/button";
import { Checkbox } from "./ui/checkbox";
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

// Represents an env var with a stable ID for React keys
interface EnvVarEntry {
	id: string;
	key: string;
	value: string;
}

interface AmplifyBranchManagementProps {
	config: YamlInfrastructureConfig;
	nodeId: string;
	onConfigChange?: (config: Partial<YamlInfrastructureConfig>) => void;
}

const STAGE_COLORS: Record<string, string> = {
	PRODUCTION:
		"bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200",
	BETA: "bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-200",
	DEVELOPMENT: "bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200",
	EXPERIMENTAL:
		"bg-purple-100 text-purple-800 dark:bg-purple-900 dark:text-purple-200",
};

// Defined at module scope: nesting it inside the parent recreated the
// component type on every render, remounting the badge and discarding state.
const StageBadge = ({ stage }: { stage?: string }) => (
	<span
		className={`px-2 py-0.5 text-xs font-medium rounded-full ${STAGE_COLORS[stage || "DEVELOPMENT"]}`}
	>
		{stage || "DEVELOPMENT"}
	</span>
);

export function AmplifyBranchManagement({
	config,
	nodeId,
	onConfigChange,
}: AmplifyBranchManagementProps) {
	const uid = useId();
	const appName = nodeId.replace("amplify-", "");
	const amplifyAppIndex =
		config.amplify_apps?.findIndex((app) => app.name === appName) ?? -1;
	const amplifyApp = config.amplify_apps?.[amplifyAppIndex];

	const [editingBranch, setEditingBranch] = useState<number | null>(null);
	const [newBranch, setNewBranch] = useState({
		name: "",
		stage: "DEVELOPMENT" as AmplifyBranch["stage"],
		enable_auto_build: true,
		enable_pull_request_preview: false,
		environment_variables_text: "",
	});
	const [showAddBranch, setShowAddBranch] = useState(false);

	// Local state for env vars being edited (with stable IDs)
	const [editingEnvVars, setEditingEnvVars] = useState<EnvVarEntry[]>([]);

	const branches = amplifyApp?.branches || [];

	// Initialize local env vars state when starting to edit a branch.
	// Must stay above the early return below: React requires every hook to run in
	// the same order on every render, so a hook after a conditional return crashes
	// with "rendered fewer hooks than expected" when amplifyApp becomes undefined.
	// Depend on the branch element rather than the branches array: the array is
	// rebuilt by `|| []` on every render and would loop, whereas the element is a
	// stable reference from config (or a stable undefined when absent).
	const editingBranchData =
		editingBranch !== null ? branches[editingBranch] : undefined;

	useEffect(() => {
		if (editingBranchData) {
			const envVars = Object.entries(
				editingBranchData.environment_variables || {},
			).map(([key, value], idx) => ({
				id: `env-${idx}-${Date.now()}`,
				key,
				value,
			}));
			setEditingEnvVars(envVars);
		} else {
			setEditingEnvVars([]);
		}
	}, [editingBranchData]);

	if (!amplifyApp) {
		return (
			<div className="text-gray-400">
				<p>Amplify app configuration not found.</p>
			</div>
		);
	}

	const handleUpdateBranches = (branches: typeof amplifyApp.branches) => {
		if (onConfigChange && config.amplify_apps) {
			const updatedApps = [...config.amplify_apps];
			updatedApps[amplifyAppIndex] = {
				...amplifyApp,
				branches,
			};
			onConfigChange({ amplify_apps: updatedApps });
		}
	};

	const handleAddBranch = () => {
		if (!newBranch.name) return;

		// Parse environment variables
		const envVars: Record<string, string> = {};
		if (newBranch.environment_variables_text) {
			const lines = newBranch.environment_variables_text
				.split("\n")
				.filter((line) => line.trim());
			for (const line of lines) {
				const [key, ...valueParts] = line.split("=");
				if (key?.trim()) {
					envVars[key.trim()] = valueParts.join("=").trim();
				}
			}
		}

		const updatedBranches = [
			...(amplifyApp.branches || []),
			{
				name: newBranch.name,
				stage: newBranch.stage,
				enable_auto_build: newBranch.enable_auto_build,
				enable_pull_request_preview: newBranch.enable_pull_request_preview,
				environment_variables: envVars,
				custom_subdomains: [], // Add this field for consistency
			},
		];

		handleUpdateBranches(updatedBranches);

		// Reset form
		setNewBranch({
			name: "",
			stage: "DEVELOPMENT" as AmplifyBranch["stage"],
			enable_auto_build: true,
			enable_pull_request_preview: false,
			environment_variables_text: "",
		});
		setShowAddBranch(false);
	};

	const handleDeleteBranch = (index: number) => {
		const updatedBranches = amplifyApp.branches.filter((_, i) => i !== index);
		handleUpdateBranches(updatedBranches);
	};

	const handleUpdateBranch: BranchUpdateHandler = (index: number, updates) => {
		const updatedBranches = [...amplifyApp.branches];
		updatedBranches[index] = {
			...updatedBranches[index],
			...updates,
		};
		handleUpdateBranches(updatedBranches);
	};

	// Save env vars from local state to config
	const saveEnvVarsToConfig = (branchIndex: number) => {
		const envVarsObj: Record<string, string> = {};
		for (const entry of editingEnvVars) {
			if (entry.key.trim()) {
				envVarsObj[entry.key] = entry.value;
			}
		}
		handleUpdateBranch(branchIndex, { environment_variables: envVarsObj });
	};

	// Update a single env var in local state (no config save)
	const updateLocalEnvVar = (
		id: string,
		field: "key" | "value",
		newValue: string,
	) => {
		setEditingEnvVars((prev) =>
			prev.map((entry) =>
				entry.id === id ? { ...entry, [field]: newValue } : entry,
			),
		);
	};

	// Add new env var to local state
	const addLocalEnvVar = () => {
		setEditingEnvVars((prev) => [
			...prev,
			{ id: `env-new-${Date.now()}`, key: "", value: "" },
		]);
	};

	// Remove env var from local state
	const removeLocalEnvVar = (id: string) => {
		setEditingEnvVars((prev) => prev.filter((entry) => entry.id !== id));
	};

	return (
		<div className="space-y-6">
			<div>
				<div className="flex items-center justify-between mb-4">
					<h3 className="text-sm font-medium text-white">Branches</h3>
					{!showAddBranch && (
						<Button size="sm" onClick={() => setShowAddBranch(true)}>
							<Plus className="w-4 h-4 mr-1" />
							Add Branch
						</Button>
					)}
				</div>

				{/* Add new branch form */}
				{showAddBranch && (
					<div className="bg-gray-800 rounded-lg p-4 border border-gray-700 mb-4 space-y-3">
						<h4 className="text-sm font-medium text-white">New Branch</h4>

						<div className="grid grid-cols-2 gap-3">
							<div>
								<Label htmlFor={`${uid}-new-branch-name`}>Branch Name</Label>
								<Input
									id={`${uid}-new-branch-name`}
									value={newBranch.name}
									onChange={(e) =>
										setNewBranch({ ...newBranch, name: e.target.value })
									}
									placeholder="feature/new-feature"
									className="mt-1 bg-gray-900 border-gray-600 text-white"
								/>
							</div>

							<div>
								<Label htmlFor={`${uid}-new-branch-stage`}>Stage</Label>
								<Select
									value={newBranch.stage}
									onValueChange={(value) =>
										setNewBranch({
											...newBranch,
											stage: value as AmplifyBranch["stage"],
										})
									}
								>
									<SelectTrigger
										id={`${uid}-new-branch-stage`}
										className="mt-1 bg-gray-900 border-gray-600 text-white"
									>
										<SelectValue />
									</SelectTrigger>
									<SelectContent>
										<SelectItem value="PRODUCTION">Production</SelectItem>
										<SelectItem value="DEVELOPMENT">Development</SelectItem>
										<SelectItem value="BETA">Beta</SelectItem>
										<SelectItem value="EXPERIMENTAL">Experimental</SelectItem>
									</SelectContent>
								</Select>
							</div>
						</div>

						<div className="space-y-2">
							<div className="flex items-center space-x-2">
								<Checkbox
									id={`${uid}-new-auto-build`}
									checked={newBranch.enable_auto_build}
									onCheckedChange={(checked) =>
										setNewBranch({
											...newBranch,
											enable_auto_build: checked as boolean,
										})
									}
								/>
								<Label
									htmlFor={`${uid}-new-auto-build`}
									className="font-normal"
								>
									Enable automatic builds on push
								</Label>
							</div>

							<div className="flex items-center space-x-2">
								<Checkbox
									id={`${uid}-new-pr-preview`}
									checked={newBranch.enable_pull_request_preview}
									onCheckedChange={(checked) =>
										setNewBranch({
											...newBranch,
											enable_pull_request_preview: checked as boolean,
										})
									}
								/>
								<Label
									htmlFor={`${uid}-new-pr-preview`}
									className="font-normal"
								>
									Enable pull request previews
								</Label>
							</div>
						</div>

						<div>
							<Label htmlFor={`${uid}-new-env-vars`}>
								Environment Variables
							</Label>
							<Textarea
								id={`${uid}-new-env-vars`}
								value={newBranch.environment_variables_text}
								onChange={(e) =>
									setNewBranch({
										...newBranch,
										environment_variables_text: e.target.value,
									})
								}
								placeholder="REACT_APP_API_URL=https://api.example.com&#10;REACT_APP_ENV=production"
								rows={3}
								className="mt-1 bg-gray-900 border-gray-600 text-white"
							/>
							<p className="text-xs text-gray-500 mt-1">
								Enter one per line in KEY=VALUE format
							</p>
						</div>

						<div className="flex justify-end gap-2">
							<Button
								size="sm"
								variant="outline"
								onClick={() => {
									setShowAddBranch(false);
									setNewBranch({
										name: "",
										stage: "DEVELOPMENT",
										enable_auto_build: true,
										enable_pull_request_preview: false,
										environment_variables_text: "",
									});
								}}
							>
								Cancel
							</Button>
							<Button
								size="sm"
								onClick={handleAddBranch}
								disabled={!newBranch.name}
							>
								Add Branch
							</Button>
						</div>
					</div>
				)}

				{/* Branches list */}
				{branches.length === 0 ? (
					<div className="bg-gray-800 rounded-lg p-4 border border-gray-700">
						<p className="text-sm text-gray-500">
							No branches configured. Add a branch to get started.
						</p>
					</div>
				) : (
					<div className="space-y-3">
						{branches.map((branch, index) => (
							<div
								key={`branch-${branch.name}-${index}`}
								className="bg-gray-800 rounded-lg p-4 border border-gray-700"
							>
								{editingBranch === index ? (
									// Edit mode
									<div className="space-y-3">
										<div className="flex items-center justify-between">
											<h4 className="text-sm font-medium text-white">
												Edit Branch
											</h4>
											<div className="flex gap-2">
												<Button
													size="icon"
													variant="ghost"
													onClick={() => setEditingBranch(null)}
												>
													<X className="w-4 h-4" />
												</Button>
											</div>
										</div>

										<div className="grid grid-cols-2 gap-3">
											<div>
												<Label>Branch Name</Label>
												<Input
													value={branch.name}
													onChange={(e) =>
														handleUpdateBranch(index, { name: e.target.value })
													}
													className="mt-1 bg-gray-900 border-gray-600 text-white"
												/>
											</div>

											<div>
												<Label>Stage</Label>
												<Select
													value={branch.stage || "DEVELOPMENT"}
													onValueChange={(value) =>
														handleUpdateBranch(index, {
															stage: value as AmplifyBranch["stage"],
														})
													}
												>
													<SelectTrigger className="mt-1 bg-gray-900 border-gray-600 text-white">
														<SelectValue />
													</SelectTrigger>
													<SelectContent>
														<SelectItem value="PRODUCTION">
															Production
														</SelectItem>
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
													checked={branch.enable_auto_build ?? true}
													onCheckedChange={(checked) =>
														handleUpdateBranch(index, {
															enable_auto_build: checked === true,
														})
													}
												/>
												<Label
													htmlFor={`auto-build-${index}`}
													className="font-normal"
												>
													Enable automatic builds
												</Label>
											</div>

											<div className="flex items-center space-x-2">
												<Checkbox
													id={`pr-preview-${index}`}
													checked={branch.enable_pull_request_preview ?? false}
													onCheckedChange={(checked) =>
														handleUpdateBranch(index, {
															enable_pull_request_preview: checked === true,
														})
													}
												/>
												<Label
													htmlFor={`pr-preview-${index}`}
													className="font-normal"
												>
													Enable PR previews
												</Label>
											</div>
										</div>

										<div>
											<Label className="text-sm">Environment Variables</Label>
											<div className="mt-2 space-y-2">
												{editingEnvVars.map((entry) => (
													<div key={entry.id} className="flex gap-2">
														<Input
															value={entry.key}
															onChange={(e) =>
																updateLocalEnvVar(
																	entry.id,
																	"key",
																	e.target.value,
																)
															}
															placeholder="KEY"
															className="flex-1 bg-gray-900 border-gray-600 text-white font-mono text-sm"
														/>
														<Input
															value={entry.value}
															onChange={(e) =>
																updateLocalEnvVar(
																	entry.id,
																	"value",
																	e.target.value,
																)
															}
															placeholder="VALUE"
															className="flex-[2] bg-gray-900 border-gray-600 text-white font-mono text-sm"
														/>
														<Button
															size="icon"
															variant="ghost"
															onClick={() => removeLocalEnvVar(entry.id)}
															className="text-red-400 hover:text-red-300"
														>
															<X className="w-4 h-4" />
														</Button>
													</div>
												))}
												<Button
													size="sm"
													variant="outline"
													onClick={addLocalEnvVar}
													className="w-full"
												>
													<Plus className="w-4 h-4 mr-1" />
													Add Variable
												</Button>
											</div>
											<p className="text-xs text-gray-500 mt-1">
												Use ${"{"}variable{"}"} for interpolation (e.g., ${"{"}
												cognito_user_pool_id{"}"})
											</p>
										</div>

										<Button
											size="sm"
											onClick={() => {
												saveEnvVarsToConfig(index);
												setEditingBranch(null);
											}}
										>
											<Check className="w-4 h-4 mr-1" />
											Save
										</Button>
									</div>
								) : (
									// View mode
									<div className="space-y-3">
										<div className="flex items-center justify-between">
											<div className="flex items-center gap-3">
												<GitBranch className="w-4 h-4 text-gray-400" />
												<span className="text-sm font-medium text-white">
													{branch.name}
												</span>
												<StageBadge stage={branch.stage} />
											</div>
											<div className="flex gap-2">
												<Button
													size="sm"
													variant="ghost"
													onClick={() => setEditingBranch(index)}
												>
													Edit
												</Button>
												{branches.length > 1 && (
													<Button
														size="icon"
														variant="ghost"
														onClick={() => handleDeleteBranch(index)}
														className="text-red-400 hover:text-red-300"
													>
														<Trash2 className="w-4 h-4" />
													</Button>
												)}
											</div>
										</div>

										<div className="grid grid-cols-2 gap-4 text-sm">
											<div>
												<p className="text-gray-400 text-xs">Auto Build</p>
												<p className="text-gray-300">
													{branch.enable_auto_build ? "Enabled" : "Disabled"}
												</p>
											</div>
											<div>
												<p className="text-gray-400 text-xs">PR Previews</p>
												<p className="text-gray-300">
													{branch.enable_pull_request_preview
														? "Enabled"
														: "Disabled"}
												</p>
											</div>
										</div>

										<div>
											<p className="text-gray-400 text-xs mb-1">
												Environment Variables
											</p>
											{Object.keys(branch.environment_variables || {}).length >
											0 ? (
												<div className="space-y-1 bg-gray-900 rounded p-2 max-h-32 overflow-y-auto">
													{Object.entries(
														branch.environment_variables || {},
													).map(([key, value]) => (
														<div key={key} className="text-xs font-mono">
															<span className="text-gray-400">{key}=</span>
															<span className="text-gray-300">{value}</span>
														</div>
													))}
												</div>
											) : (
												<p className="text-gray-300 text-sm">
													No variables configured
												</p>
											)}
										</div>
									</div>
								)}
							</div>
						))}
					</div>
				)}
			</div>
		</div>
	);
}
