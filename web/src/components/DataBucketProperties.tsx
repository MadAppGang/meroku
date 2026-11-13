import { FolderLock, Plus, Trash2 } from "lucide-react";
import { useId, useState } from "react";
import type {
	DataBucketSharedFolder,
	YamlInfrastructureConfig,
} from "../types/yamlConfig";
import { dataBucketName } from "../utils/dataBucket";
import {
	unknownWorkloadNames,
	workloadNames,
} from "../utils/workloadReferences";
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
import { Input } from "./ui/input";
import { Label } from "./ui/label";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "./ui/select";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "./ui/table";

interface DataBucketPropertiesProps {
	config: YamlInfrastructureConfig;
	onConfigChange: (config: Partial<YamlInfrastructureConfig>) => void;
}

type FolderAccess = "none" | "read" | "read_write";

// Same rule as app/data_bucket.go and modules/data_bucket.
const SHARED_NAME_PATTERN = /^[a-z0-9]([a-z0-9-]{0,30}[a-z0-9])?$/;

function sharedFolderNameError(
	name: string,
	existing: DataBucketSharedFolder[],
): string | null {
	if (name === "") return null;
	if (!SHARED_NAME_PATTERN.test(name)) {
		return "Use 1 to 32 lower-case letters, digits and hyphens, starting and ending with a letter or a digit.";
	}
	if (existing.some((f) => f.name === name)) {
		return `A shared folder named "${name}" already exists.`;
	}
	return null;
}

function accessOf(folder: DataBucketSharedFolder, name: string): FolderAccess {
	if (folder.read_write?.includes(name)) return "read_write";
	if (folder.read?.includes(name)) return "read";
	return "none";
}

function withAccess(
	folder: DataBucketSharedFolder,
	name: string,
	access: FolderAccess,
): DataBucketSharedFolder {
	const readWrite = (folder.read_write || []).filter((n) => n !== name);
	const read = (folder.read || []).filter((n) => n !== name);
	if (access === "read_write") readWrite.push(name);
	if (access === "read") read.push(name);
	return { ...folder, read_write: readWrite, read };
}

function sharedEnvName(folder: string): string {
	return `DATA_BUCKET_SHARED_${folder.toUpperCase().replaceAll("-", "_")}`;
}

/**
 * The "Data bucket" tab of the S3 node: the environment's private data bucket,
 * the folder every workload gets, and the editor for shared folders.
 *
 * This component is the single owner of `data_bucket.shared`: it always writes
 * the whole list, computed from the current config.
 */
export function DataBucketProperties({
	config,
	onConfigChange,
}: DataBucketPropertiesProps) {
	const uid = useId();
	const [newFolderName, setNewFolderName] = useState("");

	const shared = config.data_bucket?.shared || [];
	const workloads = workloadNames(config);
	const newFolderError = sharedFolderNameError(newFolderName, shared);

	const bucketName = dataBucketName(config);

	const saveShared = (next: DataBucketSharedFolder[]) => {
		onConfigChange({ data_bucket: { shared: next } });
	};

	const handleAddFolder = () => {
		if (newFolderName === "" || newFolderError) return;
		saveShared([...shared, { name: newFolderName, read_write: [], read: [] }]);
		setNewFolderName("");
	};

	const handleRemoveFolder = (index: number) => {
		saveShared(shared.filter((_, i) => i !== index));
	};

	const handleAccessChange = (
		index: number,
		workload: string,
		access: FolderAccess,
	) => {
		saveShared(
			shared.map((folder, i) =>
				i === index ? withAccess(folder, workload, access) : folder,
			),
		);
	};

	const handleDropUnknown = (index: number) => {
		saveShared(
			shared.map((folder, i) => {
				if (i !== index) return folder;
				const unknown = new Set(
					unknownWorkloadNames(config, [
						...(folder.read_write || []),
						...(folder.read || []),
					]),
				);
				return {
					...folder,
					read_write: (folder.read_write || []).filter((n) => !unknown.has(n)),
					read: (folder.read || []).filter((n) => !unknown.has(n)),
				};
			}),
		);
	};

	return (
		<div className="space-y-4">
			<Card>
				<CardHeader>
					<CardTitle className="flex items-center gap-2">
						<FolderLock className="w-5 h-5" />
						Data bucket
					</CardTitle>
					<CardDescription>
						A private bucket every environment gets. Each workload reads and
						writes its own folder; shared folders below are opened to the
						workloads you choose.
					</CardDescription>
				</CardHeader>
				<CardContent className="space-y-3">
					<div>
						<Label>Bucket</Label>
						<p className="font-mono text-sm">{bucketName}</p>
						<p className="text-xs text-muted-foreground">
							Containers get it as DATA_BUCKET_NAME.
						</p>
					</div>
					<div className="flex flex-wrap gap-2">
						<Badge variant="secondary">Private</Badge>
						<Badge variant="secondary">Encrypted</Badge>
						<Badge variant="secondary">Versioned, 30 days</Badge>
						<Badge variant="outline">No CORS</Badge>
					</div>
					<Table>
						<TableHeader>
							<TableRow>
								<TableHead>Workload</TableHead>
								<TableHead>Own folder (DATA_BUCKET_PATH)</TableHead>
							</TableRow>
						</TableHeader>
						<TableBody>
							{workloads.map((name) => (
								<TableRow key={name}>
									<TableCell>{name}</TableCell>
									<TableCell variant="code">{name}/</TableCell>
								</TableRow>
							))}
						</TableBody>
					</Table>
				</CardContent>
			</Card>

			<Card>
				<CardHeader>
					<CardTitle>Shared folders</CardTitle>
					<CardDescription>
						Each folder is shared/&lt;name&gt;/ in the bucket and reaches the
						workloads that can use it as DATA_BUCKET_SHARED_&lt;NAME&gt;.
					</CardDescription>
				</CardHeader>
				<CardContent className="space-y-4">
					{shared.length === 0 && (
						<p className="text-sm text-muted-foreground">
							No shared folders yet.
						</p>
					)}

					{shared.map((folder, index) => {
						const unknown = unknownWorkloadNames(config, [
							...(folder.read_write || []),
							...(folder.read || []),
						]);
						return (
							<Card
								key={folder.name}
								data-testid={`shared-folder-${folder.name}`}
							>
								<CardHeader>
									<div className="flex items-center justify-between gap-2">
										<div>
											<CardTitle variant="code">
												shared/{folder.name}/
											</CardTitle>
											<CardDescription variant="code">
												{sharedEnvName(folder.name)}
											</CardDescription>
										</div>
										<Button
											variant="ghost"
											size="sm"
											aria-label={`Remove shared folder ${folder.name}`}
											onClick={() => handleRemoveFolder(index)}
										>
											<Trash2 className="w-4 h-4" />
										</Button>
									</div>
								</CardHeader>
								<CardContent className="space-y-3">
									{unknown.length > 0 && (
										<Alert variant="destructive">
											<AlertDescription>
												<span>
													{unknown.join(", ")}{" "}
													{unknown.length === 1 ? "is" : "are"} not a workload
													of this environment; generate refuses the folder until{" "}
													{unknown.length === 1 ? "it is" : "they are"} removed.
												</span>
												<Button
													variant="outline"
													size="sm"
													onClick={() => handleDropUnknown(index)}
												>
													Remove
												</Button>
											</AlertDescription>
										</Alert>
									)}
									<Table>
										<TableHeader>
											<TableRow>
												<TableHead>Workload</TableHead>
												<TableHead>Access</TableHead>
											</TableRow>
										</TableHeader>
										<TableBody>
											{workloads.map((name) => (
												<TableRow key={name}>
													<TableCell>{name}</TableCell>
													<TableCell>
														<Select
															value={accessOf(folder, name)}
															onValueChange={(value) =>
																handleAccessChange(
																	index,
																	name,
																	value as FolderAccess,
																)
															}
														>
															<SelectTrigger
																aria-label={`Access of ${name} to ${folder.name}`}
															>
																<SelectValue />
															</SelectTrigger>
															<SelectContent>
																<SelectItem value="none">No access</SelectItem>
																<SelectItem value="read">Read</SelectItem>
																<SelectItem value="read_write">
																	Read and write
																</SelectItem>
															</SelectContent>
														</Select>
													</TableCell>
												</TableRow>
											))}
										</TableBody>
									</Table>
								</CardContent>
							</Card>
						);
					})}

					<div className="space-y-2">
						<Label htmlFor={`${uid}-new-folder`}>New shared folder</Label>
						<div className="flex gap-2">
							<Input
								id={`${uid}-new-folder`}
								placeholder="uploads"
								value={newFolderName}
								aria-invalid={newFolderError !== null}
								onChange={(e) => setNewFolderName(e.target.value.trim())}
								onKeyDown={(e) => {
									if (e.key === "Enter") handleAddFolder();
								}}
							/>
							<Button
								onClick={handleAddFolder}
								disabled={newFolderName === "" || newFolderError !== null}
							>
								<Plus className="w-4 h-4" />
								Add
							</Button>
						</div>
						{newFolderError && (
							<p className="text-sm text-danger">{newFolderError}</p>
						)}
					</div>
				</CardContent>
			</Card>
		</div>
	);
}
