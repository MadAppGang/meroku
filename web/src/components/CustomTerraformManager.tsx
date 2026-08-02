import {
	ArrowLeft,
	FileCode,
	Folder,
	FolderOpen,
	Plus,
	Save,
	Trash2,
} from "lucide-react";
import { useCallback, useEffect, useId, useState } from "react";
import { toast } from "sonner";
import { customTerraformApi } from "../api/customTerraform";
import type {
	BridgeVariable,
	CustomTerraformFile,
} from "../types/customTerraform";
import { SidebarRight } from "./SidebarRight";
import { TerraformEditor } from "./TerraformEditor";
import { Button } from "./ui/button";
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
import { ScrollArea } from "./ui/scroll-area";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "./ui/select";

interface CustomTerraformManagerProps {
	environment: string;
	onClose?: () => void;
}

export function CustomTerraformManager({
	environment,
	onClose,
}: CustomTerraformManagerProps) {
	const [files, setFiles] = useState<CustomTerraformFile[]>([]);
	const [selectedFile, setSelectedFile] = useState<CustomTerraformFile | null>(
		null,
	);
	const [editorContent, setEditorContent] = useState("");
	const [bridgeVariables, setBridgeVariables] = useState<BridgeVariable[]>([]);
	const [isSaving, setIsSaving] = useState(false);
	const [showNewFileDialog, setShowNewFileDialog] = useState(false);
	const [showDeleteDialog, setShowDeleteDialog] = useState(false);
	const [newFileName, setNewFileName] = useState("");
	const [newFileScope, setNewFileScope] = useState<"shared" | "environment">(
		"environment",
	);
	const [hasUnsavedChanges, setHasUnsavedChanges] = useState(false);
	const fileNameId = useId();

	const loadFiles = useCallback(async () => {
		try {
			const loadedFiles = await customTerraformApi.listFiles(environment);
			setFiles(loadedFiles || []);
		} catch (error) {
			console.error("Failed to load files:", error);
			toast.error("Failed to load custom Terraform files");
		} finally {
		}
	}, [environment]);

	const loadBridgeVariables = useCallback(async () => {
		try {
			const response = await customTerraformApi.getBridgeVariables(environment);
			setBridgeVariables(response.variables);
		} catch (error) {
			console.error("Failed to load bridge variables:", error);
		}
	}, [environment]);

	useEffect(() => {
		loadFiles();
		loadBridgeVariables();
	}, [loadFiles, loadBridgeVariables]);

	const handleFileSelect = async (file: CustomTerraformFile) => {
		if (hasUnsavedChanges) {
			if (
				!window.confirm(
					"You have unsaved changes. Are you sure you want to switch files?",
				)
			) {
				return;
			}
		}

		try {
			const loadedFile = await customTerraformApi.getFile(
				environment,
				file.path,
				file.scope as "shared" | "environment",
			);
			setSelectedFile(loadedFile);
			setEditorContent(loadedFile.content);
			setHasUnsavedChanges(false);
		} catch (error) {
			console.error("Failed to load file:", error);
			toast.error("Failed to load file");
		}
	};

	const handleEditorChange = (value: string) => {
		setEditorContent(value);
		setHasUnsavedChanges(true);
	};

	const handleSave = async () => {
		if (!selectedFile) return;

		try {
			setIsSaving(true);
			await customTerraformApi.saveFile(environment, {
				path: selectedFile.path,
				content: editorContent,
				scope: selectedFile.scope as "shared" | "environment",
			});
			toast.success("File saved successfully");
			setHasUnsavedChanges(false);
			await loadFiles();
		} catch (error) {
			console.error("Failed to save file:", error);
			toast.error("Failed to save file");
		} finally {
			setIsSaving(false);
		}
	};

	const handleNewFile = async () => {
		if (!newFileName.trim()) {
			toast.error("Please enter a file name");
			return;
		}

		if (!/^[a-zA-Z0-9_-]+$/.test(newFileName.trim())) {
			toast.error(
				"File name can only contain letters, numbers, underscores, and hyphens",
			);
			return;
		}

		const fileName = newFileName.trim().endsWith(".tf")
			? newFileName.trim()
			: `${newFileName.trim()}.tf`;

		try {
			await customTerraformApi.saveFile(environment, {
				path: fileName,
				content: `# ${fileName}\n# Custom Terraform configuration\n\n`,
				scope: newFileScope,
			});
			toast.success("File created successfully");
			setShowNewFileDialog(false);
			setNewFileName("");
			await loadFiles();
		} catch (error) {
			console.error("Failed to create file:", error);
			toast.error("Failed to create file");
		}
	};

	const handleDeleteFile = async () => {
		if (!selectedFile) return;

		try {
			await customTerraformApi.deleteFile(environment, {
				path: selectedFile.path,
				scope: selectedFile.scope as "shared" | "environment",
			});
			toast.success("File deleted successfully");
			setShowDeleteDialog(false);
			setSelectedFile(null);
			setEditorContent("");
			setHasUnsavedChanges(false);
			await loadFiles();
		} catch (error) {
			console.error("Failed to delete file:", error);
			toast.error("Failed to delete file");
		}
	};

	const handleInsert = (text: string) => {
		setEditorContent((prev) => `${prev}\n${text}`);
		setHasUnsavedChanges(true);
		toast.success("Inserted");
	};

	const groupedFiles = (files || []).reduce(
		(acc, file) => {
			if (file.scope === "shared") {
				acc.shared.push(file);
			} else {
				acc.environment.push(file);
			}
			return acc;
		},
		{
			shared: [] as CustomTerraformFile[],
			environment: [] as CustomTerraformFile[],
		},
	);

	return (
		<div className="flex h-full">
			{/* File Tree Sidebar */}
			<div className="w-64 border-r border-gray-700 bg-gray-900 flex flex-col">
				<div className="p-3 border-b border-gray-700 space-y-2">
					{onClose && (
						<Button
							onClick={onClose}
							variant="ghost"
							size="sm"
							className="w-full justify-start text-gray-400 hover:text-white mb-2"
						>
							<ArrowLeft className="w-4 h-4 mr-2" />
							Back to Canvas
						</Button>
					)}
					<Button
						onClick={() => setShowNewFileDialog(true)}
						size="sm"
						className="w-full"
					>
						<Plus className="w-4 h-4 mr-2" />
						New File
					</Button>
				</div>

				<ScrollArea className="flex-1">
					<div className="p-2">
						{/* Shared Files */}
						<div className="mb-4">
							<div className="flex items-center gap-2 text-xs font-semibold text-gray-400 mb-2 px-2">
								<FolderOpen className="w-4 h-4" />
								<span>Shared (_shared)</span>
							</div>
							{groupedFiles.shared.length === 0 ? (
								<div className="text-xs text-gray-500 px-4 py-2">No files</div>
							) : (
								groupedFiles.shared.map((file) => (
									<button
										type="button"
										key={file.path}
										onClick={() => handleFileSelect(file)}
										className={`w-full flex items-center gap-2 px-4 py-2 text-sm rounded hover:bg-gray-800 transition-colors ${
											selectedFile?.path === file.path &&
											selectedFile?.scope === file.scope
												? "bg-gray-800 text-blue-400"
												: "text-gray-300"
										}`}
									>
										<FileCode className="w-4 h-4" />
										<span className="truncate">{file.name}</span>
									</button>
								))
							)}
						</div>

						{/* Environment Files */}
						<div>
							<div className="flex items-center gap-2 text-xs font-semibold text-gray-400 mb-2 px-2">
								<Folder className="w-4 h-4" />
								<span>Environment ({environment})</span>
							</div>
							{groupedFiles.environment.length === 0 ? (
								<div className="text-xs text-gray-500 px-4 py-2">No files</div>
							) : (
								groupedFiles.environment.map((file) => (
									<button
										type="button"
										key={file.path}
										onClick={() => handleFileSelect(file)}
										className={`w-full flex items-center gap-2 px-4 py-2 text-sm rounded hover:bg-gray-800 transition-colors ${
											selectedFile?.path === file.path &&
											selectedFile?.scope === file.scope
												? "bg-gray-800 text-blue-400"
												: "text-gray-300"
										}`}
									>
										<FileCode className="w-4 h-4" />
										<span className="truncate">{file.name}</span>
									</button>
								))
							)}
						</div>
					</div>
				</ScrollArea>
			</div>

			{/* Editor Panel */}
			<div className="flex-1 flex flex-col">
				{selectedFile ? (
					<>
						{/* Editor Header */}
						<div className="p-3 border-b border-gray-700 bg-gray-900 flex items-center justify-between">
							<div className="flex items-center gap-3">
								<FileCode className="w-5 h-5 text-gray-400" />
								<div>
									<div className="text-sm font-medium text-white">
										{selectedFile.name}
										{hasUnsavedChanges && (
											<span className="ml-2 text-xs text-yellow-400">
												(unsaved)
											</span>
										)}
									</div>
									<div className="text-xs text-gray-400">
										{selectedFile.scope === "shared"
											? "Shared"
											: `Environment: ${environment}`}
									</div>
								</div>
							</div>
							<div className="flex items-center gap-2">
								<Button
									onClick={handleSave}
									disabled={!hasUnsavedChanges || isSaving}
									size="sm"
									variant="default"
								>
									<Save className="w-4 h-4 mr-2" />
									{isSaving ? "Saving..." : "Save"}
								</Button>
								<Button
									onClick={() => setShowDeleteDialog(true)}
									size="sm"
									variant="destructive"
								>
									<Trash2 className="w-4 h-4" />
								</Button>
							</div>
						</div>

						{/* Editor */}
						<div className="flex-1">
							<TerraformEditor
								value={editorContent}
								onChange={handleEditorChange}
								bridgeVariables={bridgeVariables}
							/>
						</div>
					</>
				) : (
					<div className="flex-1 flex items-center justify-center text-gray-500">
						<div className="text-center">
							<FileCode className="w-16 h-16 mx-auto mb-4 text-gray-600" />
							<p className="text-lg font-medium">No file selected</p>
							<p className="text-sm mt-2">
								Select a file from the sidebar or create a new one
							</p>
						</div>
					</div>
				)}
			</div>

			{/* Bridge Variables Reference Panel */}
			<SidebarRight
				variables={bridgeVariables}
				onInsert={handleInsert}
				environment={environment}
			/>

			{/* New File Dialog */}
			<Dialog open={showNewFileDialog} onOpenChange={setShowNewFileDialog}>
				<DialogContent className="bg-gray-900 border-gray-700">
					<DialogHeader>
						<DialogTitle className="text-white">Create New File</DialogTitle>
						<DialogDescription className="text-gray-400">
							Create a new custom Terraform file
						</DialogDescription>
					</DialogHeader>
					<div className="space-y-4 py-4">
						<div className="space-y-2">
							<Label htmlFor={fileNameId} className="text-white">
								File Name
							</Label>
							<Input
								id={fileNameId}
								placeholder="my-custom-resources.tf"
								value={newFileName}
								onChange={(e) => setNewFileName(e.target.value)}
								className="bg-gray-800 border-gray-700 text-white"
							/>
						</div>
						<div className="space-y-2">
							<Label htmlFor="fileScope" className="text-white">
								Scope
							</Label>
							<Select
								value={newFileScope}
								onValueChange={(value: "shared" | "environment") =>
									setNewFileScope(value)
								}
							>
								<SelectTrigger className="bg-gray-800 border-gray-700 text-white">
									<SelectValue />
								</SelectTrigger>
								<SelectContent className="bg-gray-800 border-gray-700">
									<SelectItem value="environment">
										Environment ({environment})
									</SelectItem>
									<SelectItem value="shared">
										Shared (all environments)
									</SelectItem>
								</SelectContent>
							</Select>
							<p className="text-xs text-gray-400">
								{newFileScope === "shared"
									? "Shared files are available to all environments"
									: "Environment files are specific to this environment"}
							</p>
						</div>
					</div>
					<DialogFooter>
						<Button
							variant="ghost"
							onClick={() => setShowNewFileDialog(false)}
							className="text-gray-400"
						>
							Cancel
						</Button>
						<Button onClick={handleNewFile}>Create</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>

			{/* Delete Confirmation Dialog */}
			<Dialog open={showDeleteDialog} onOpenChange={setShowDeleteDialog}>
				<DialogContent className="bg-gray-900 border-gray-700">
					<DialogHeader>
						<DialogTitle className="text-white">Delete File</DialogTitle>
						<DialogDescription className="text-gray-400">
							Are you sure you want to delete "{selectedFile?.name}"? This
							action cannot be undone.
						</DialogDescription>
					</DialogHeader>
					<DialogFooter>
						<Button
							variant="ghost"
							onClick={() => setShowDeleteDialog(false)}
							className="text-gray-400"
						>
							Cancel
						</Button>
						<Button variant="destructive" onClick={handleDeleteFile}>
							Delete
						</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>
		</div>
	);
}
