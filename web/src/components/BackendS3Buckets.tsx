import {
	AlertCircle,
	Check,
	Edit2,
	Eye,
	FileText,
	Folder,
	Globe,
	HardDrive,
	Loader2,
	Lock,
	Plus,
	Trash2,
	X,
} from "lucide-react";
import { useState } from "react";
import { infrastructureApi } from "../api/infrastructure";
import type { YamlInfrastructureConfig } from "../types/yamlConfig";
import { Alert, AlertDescription } from "./ui/alert";
import { Badge } from "./ui/badge";
import { Button } from "./ui/button";
import {
	Card,
	CardContent,
	CardDescription,
	CardHeader,
	CardTitle,
} from "./ui/card";
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
import { Textarea } from "./ui/textarea";

interface BackendS3BucketsProps {
	config: YamlInfrastructureConfig;
	onConfigChange?: (config: Partial<YamlInfrastructureConfig>) => void;
}

export function BackendS3Buckets({
	config,
	onConfigChange,
}: BackendS3BucketsProps) {
	const primaryBucketName = `${config.project}-backend-${config.env}${config.workload?.bucket_postfix || ""}`;
	const isPublic = config.workload?.bucket_public !== false;

	// Environment files from S3
	const [envFiles, setEnvFiles] = useState(config.workload?.env_files_s3 || []);
	const [showNewEnvFile, setShowNewEnvFile] = useState(false);
	const [newBucket, setNewBucket] = useState(primaryBucketName); // Default to full backend bucket name
	const [newKey, setNewKey] = useState("");
	const [editingIndex, setEditingIndex] = useState<number | null>(null);
	const [editBucket, setEditBucket] = useState("");
	const [editKey, setEditKey] = useState("");

	// File content dialog state
	const [showFileDialog, setShowFileDialog] = useState(false);
	const [selectedFile, setSelectedFile] = useState<{
		bucket: string;
		key: string;
	} | null>(null);
	const [editingFileContent, setEditingFileContent] = useState("");
	const [loadingFile, setLoadingFile] = useState(false);
	const [savingFile, setSavingFile] = useState(false);
	const [fileError, setFileError] = useState<string | null>(null);

	const handleAddEnvFile = () => {
		if (newBucket && newKey) {
			const updatedEnvFiles = [...envFiles, { bucket: newBucket, key: newKey }];
			setEnvFiles(updatedEnvFiles);
			setNewBucket("");
			setNewKey("");
			setShowNewEnvFile(false);

			// Update the config
			if (onConfigChange) {
				onConfigChange({
					workload: {
						...config.workload,
						env_files_s3: updatedEnvFiles,
					},
				});
			}
		}
	};

	const handleUpdateEnvFile = (index: number) => {
		const updated = [...envFiles];
		updated[index] = { bucket: editBucket, key: editKey };
		setEnvFiles(updated);
		setEditingIndex(null);

		// Update the config
		if (onConfigChange) {
			onConfigChange({
				workload: {
					...config.workload,
					env_files_s3: updated,
				},
			});
		}
	};

	const handleDeleteEnvFile = (index: number) => {
		const updated = envFiles.filter((_, i) => i !== index);
		setEnvFiles(updated);

		// Update the config
		if (onConfigChange) {
			onConfigChange({
				workload: {
					...config.workload,
					env_files_s3: updated,
				},
			});
		}
	};

	const handleViewFile = async (envFile: { bucket: string; key: string }) => {
		try {
			setSelectedFile(envFile);
			setShowFileDialog(true);
			setLoadingFile(true);
			setFileError(null);

			// Use bucket name directly (it's already the full name)
			const file = await infrastructureApi.getS3File(
				envFile.bucket,
				envFile.key,
			);
			setEditingFileContent(file.content || "");
		} catch (_err) {
			// If file doesn't exist, start with empty content
			setEditingFileContent(
				"# Environment variables\n# Add your configuration here\n\n",
			);
			setFileError("File does not exist yet. You can create it by saving.");
		} finally {
			setLoadingFile(false);
		}
	};

	const handleSaveFile = async () => {
		if (!selectedFile) return;

		try {
			setSavingFile(true);
			setFileError(null);

			// Use bucket name directly (it's already the full name)
			await infrastructureApi.putS3File({
				bucket: selectedFile.bucket,
				key: selectedFile.key,
				content: editingFileContent,
			});

			setShowFileDialog(false);
		} catch (err) {
			setFileError(err instanceof Error ? err.message : "Failed to save file");
		} finally {
			setSavingFile(false);
		}
	};

	return (
		<div className="space-y-4">
			<Card>
				<CardHeader>
					<CardTitle className="flex items-center gap-2">
						<HardDrive className="w-5 h-5" />
						Primary Backend Bucket
					</CardTitle>
					<CardDescription>
						Main S3 bucket for backend file storage
					</CardDescription>
				</CardHeader>
				<CardContent>
					<div className="space-y-3">
						<div className="flex items-center justify-between p-3 bg-gray-800 rounded">
							<div className="space-y-1">
								<div className="flex items-center gap-2">
									<Folder className="w-4 h-4 text-green-400" />
									<code className="text-sm font-mono text-green-400">
										{primaryBucketName}
									</code>
								</div>
								<div className="flex items-center gap-4 mt-2">
									<div className="flex items-center gap-2">
										{isPublic ? (
											<Globe className="w-3 h-3 text-yellow-400" />
										) : (
											<Lock className="w-3 h-3 text-gray-400" />
										)}
										<Badge
											variant={isPublic ? "outline" : "secondary"}
											className="text-xs"
										>
											{isPublic ? "Public Access" : "Private"}
										</Badge>
									</div>
									<Badge variant="outline" className="text-xs">
										Primary
									</Badge>
								</div>
							</div>
						</div>

						<div className="space-y-2 text-sm text-gray-400">
							<p className="flex items-center gap-2">
								<span className="text-gray-500">Environment Variable:</span>
								<code className="font-mono text-blue-400">AWS_S3_BUCKET</code>
							</p>
							<p className="flex items-center gap-2">
								<span className="text-gray-500">Purpose:</span>
								<span>File uploads, static assets, temporary storage</span>
							</p>
						</div>

						<div className="p-3 bg-gray-800 rounded space-y-2">
							<p className="text-xs font-medium text-gray-300">
								IAM Permissions:
							</p>
							<ul className="text-xs text-gray-400 space-y-1 ml-4">
								<li>• s3:GetObject</li>
								<li>• s3:PutObject</li>
								<li>• s3:DeleteObject</li>
								<li>• s3:ListBucket</li>
							</ul>
						</div>
					</div>
				</CardContent>
			</Card>

			{/* Environment Files from S3 */}
			<Card>
				<CardHeader>
					<div className="flex items-center justify-between">
						<div>
							<CardTitle className="flex items-center gap-2">
								<FileText className="w-5 h-5" />
								Environment Files
							</CardTitle>
							<CardDescription>Load .env files from S3 buckets</CardDescription>
						</div>
						<Button
							size="sm"
							onClick={() => setShowNewEnvFile(true)}
							disabled={showNewEnvFile}
						>
							<Plus className="w-4 h-4 mr-1" />
							Add File
						</Button>
					</div>
				</CardHeader>
				<CardContent>
					<div className="space-y-3">
						{/* New env file form */}
						{showNewEnvFile && (
							<div className="border border-blue-700 bg-blue-900/10 rounded-lg p-3 space-y-3">
								<div className="grid grid-cols-2 gap-3">
									<div>
										<Label htmlFor="new-bucket" className="text-xs">
											Bucket Name
										</Label>
										<Input
											id="new-bucket"
											placeholder={primaryBucketName}
											value={newBucket}
											onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
												setNewBucket(e.target.value)
											}
											className="mt-1 h-8 text-sm"
										/>
										<p className="text-xs text-gray-500 mt-1">
											Enter full S3 bucket name
										</p>
									</div>
									<div>
										<Label htmlFor="new-key" className="text-xs">
											File Path
										</Label>
										<Input
											id="new-key"
											placeholder="backend/.env"
											value={newKey}
											onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
												setNewKey(e.target.value)
											}
											className="mt-1 h-8 text-sm"
										/>
									</div>
								</div>
								<div className="flex justify-end gap-2">
									<Button
										size="sm"
										variant="ghost"
										onClick={() => {
											setShowNewEnvFile(false);
											setNewBucket(primaryBucketName); // Reset to default
											setNewKey("");
										}}
									>
										Cancel
									</Button>
									<Button
										size="sm"
										onClick={handleAddEnvFile}
										disabled={!newBucket || !newKey}
									>
										<Check className="w-4 h-4 mr-1" />
										Add
									</Button>
								</div>
							</div>
						)}

						{/* Environment files list */}
						{envFiles.map((envFile, index) => {
							const isEditing = editingIndex === index;

							return (
								<div
									key={index}
									className="border border-gray-700 rounded-lg p-3"
								>
									{isEditing ? (
										<div className="space-y-3">
											<div className="grid grid-cols-2 gap-3">
												<div>
													<Label className="text-xs">Bucket Name</Label>
													<Input
														value={editBucket}
														onChange={(
															e: React.ChangeEvent<HTMLInputElement>,
														) => setEditBucket(e.target.value)}
														className="mt-1 h-8 text-sm"
													/>
													<p className="text-xs text-gray-500 mt-1">
														Enter full S3 bucket name
													</p>
												</div>
												<div>
													<Label className="text-xs">Key</Label>
													<Input
														value={editKey}
														onChange={(
															e: React.ChangeEvent<HTMLInputElement>,
														) => setEditKey(e.target.value)}
														className="mt-1 h-8 text-sm"
													/>
												</div>
											</div>
											<div className="flex justify-end gap-2">
												<Button
													size="sm"
													variant="ghost"
													onClick={() => setEditingIndex(null)}
												>
													<X className="w-3 h-3" />
												</Button>
												<Button
													size="sm"
													onClick={() => handleUpdateEnvFile(index)}
												>
													<Check className="w-3 h-3 mr-1" />
													Save
												</Button>
											</div>
										</div>
									) : (
										<div className="flex items-center justify-between">
											<div className="space-y-1">
												<div className="flex items-center gap-2">
													<FileText className="w-4 h-4 text-green-400" />
													<code className="text-sm font-mono text-green-400">
														{envFile.key}
													</code>
												</div>
												<div className="flex items-center gap-2 text-xs text-gray-400">
													<HardDrive className="w-3 h-3" />
													<span>
														s3://
														<code className="font-mono">
															{envFile.bucket}/{envFile.key}
														</code>
													</span>
												</div>
											</div>
											<div className="flex items-center gap-1">
												<Button
													size="sm"
													variant="ghost"
													onClick={() => handleViewFile(envFile)}
													className="h-6 px-2 text-xs"
												>
													<Eye className="w-3 h-3 mr-1" />
													View
												</Button>
												<Button
													size="sm"
													variant="ghost"
													onClick={() => {
														setEditingIndex(index);
														setEditBucket(envFile.bucket);
														setEditKey(envFile.key);
													}}
													className="h-6 w-6 p-0"
												>
													<Edit2 className="w-3 h-3" />
												</Button>
												<Button
													size="sm"
													variant="ghost"
													onClick={() => handleDeleteEnvFile(index)}
													className="h-6 w-6 p-0 text-red-400 hover:text-red-300"
												>
													<Trash2 className="w-3 h-3" />
												</Button>
											</div>
										</div>
									)}
								</div>
							);
						})}

						{envFiles.length === 0 && !showNewEnvFile && (
							<div className="text-center py-8 text-gray-400">
								<FileText className="w-8 h-8 mx-auto mb-2 opacity-50" />
								<p className="text-sm">No environment files configured</p>
								<p className="text-xs mt-1">
									Click "Add File" to configure S3 environment files
								</p>
							</div>
						)}
					</div>
				</CardContent>
			</Card>

			{/* File Content Dialog */}
			<Dialog open={showFileDialog} onOpenChange={setShowFileDialog}>
				<DialogContent className="max-w-4xl w-[90vw] max-h-[85vh]">
					<DialogHeader>
						<DialogTitle className="flex items-center gap-2">
							<FileText className="w-4 h-4" />
							{selectedFile?.key}
						</DialogTitle>
						<DialogDescription>
							{selectedFile &&
								`s3://${selectedFile.bucket}/${selectedFile.key}`}
						</DialogDescription>
					</DialogHeader>

					<div className="space-y-4 py-4">
						{fileError && (
							<Alert>
								<AlertCircle className="h-4 w-4" />
								<AlertDescription>{fileError}</AlertDescription>
							</Alert>
						)}

						{loadingFile ? (
							<div className="flex items-center justify-center py-8">
								<Loader2 className="w-6 h-6 animate-spin" />
							</div>
						) : (
							<div className="space-y-2">
								<Label>File Content</Label>
								<Textarea
									value={editingFileContent}
									onChange={(e: React.ChangeEvent<HTMLTextAreaElement>) =>
										setEditingFileContent(e.target.value)
									}
									className="font-mono text-sm min-h-[400px] resize-y"
									placeholder="# Environment variables\nKEY=value\nANOTHER_KEY=another_value"
								/>
								<p className="text-xs text-gray-400">
									Use standard .env format. One variable per line.
								</p>
							</div>
						)}
					</div>

					<DialogFooter>
						<Button
							variant="outline"
							onClick={() => setShowFileDialog(false)}
							disabled={savingFile}
						>
							Cancel
						</Button>
						<Button
							onClick={handleSaveFile}
							disabled={savingFile || loadingFile}
						>
							{savingFile ? (
								<>
									<Loader2 className="w-4 h-4 mr-2 animate-spin" />
									Saving...
								</>
							) : (
								<>
									<Check className="w-4 h-4 mr-2" />
									Save File
								</>
							)}
						</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>
		</div>
	);
}
