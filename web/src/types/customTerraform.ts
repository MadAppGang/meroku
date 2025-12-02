export interface CustomTerraformFile {
	path: string;
	name: string;
	content: string;
	scope: "shared" | "environment";
	lastModified?: string;
}

export interface BridgeVariable {
	name: string;
	description: string;
	type: string;
	conditional?: string;
}

export interface CustomModuleOutput {
	name: string;
	description: string;
	value: string;
}

export interface CustomModuleStatus {
	moduleName: string;
	deployed: boolean;
	resourceCount?: number;
	lastApplied?: string;
	outputs?: CustomModuleOutput[];
}

export interface ListFilesResponse {
	files: CustomTerraformFile[];
}

export interface GetFileResponse {
	file: CustomTerraformFile;
}

export interface SaveFileRequest {
	path: string;
	content: string;
	scope: "shared" | "environment";
}

export interface DeleteFileRequest {
	path: string;
	scope: "shared" | "environment";
}

export interface BridgeVariablesResponse {
	variables: BridgeVariable[];
}

export interface CustomModuleStatusResponse {
	modules: CustomModuleStatus[];
}
