import type {
	BridgeVariablesResponse,
	CustomModuleStatusResponse,
	CustomTerraformFile,
	DeleteFileRequest,
	GetFileResponse,
	ListFilesResponse,
	SaveFileRequest,
} from "../types/customTerraform";
import { fetchWithTokenRetry } from "../utils/fetchWithRetry";

const API_BASE_URL = import.meta.env.VITE_API_URL || "";

export const customTerraformApi = {
	/**
	 * List all custom Terraform files for the current environment
	 */
	async listFiles(environment: string): Promise<CustomTerraformFile[]> {
		const response = await fetchWithTokenRetry(
			`${API_BASE_URL}/api/custom-terraform/files?env=${encodeURIComponent(environment)}`,
		);
		if (!response.ok) {
			throw new Error("Failed to fetch custom Terraform files");
		}
		const data: ListFilesResponse = await response.json();
		return data.files || [];
	},

	/**
	 * Get a specific custom Terraform file
	 */
	async getFile(
		environment: string,
		path: string,
		scope: "shared" | "environment",
	): Promise<CustomTerraformFile> {
		const response = await fetchWithTokenRetry(
			`${API_BASE_URL}/api/custom-terraform/file?env=${encodeURIComponent(environment)}&path=${encodeURIComponent(path)}&scope=${scope}`,
		);
		if (!response.ok) {
			throw new Error("Failed to fetch file");
		}
		const data: GetFileResponse = await response.json();
		return data.file;
	},

	/**
	 * Save a custom Terraform file
	 */
	async saveFile(environment: string, request: SaveFileRequest): Promise<void> {
		const response = await fetchWithTokenRetry(
			`${API_BASE_URL}/api/custom-terraform/file?env=${encodeURIComponent(environment)}`,
			{
				method: "POST",
				headers: {
					"Content-Type": "application/json",
				},
				body: JSON.stringify(request),
			},
		);
		if (!response.ok) {
			const error = await response.text();
			throw new Error(error || "Failed to save file");
		}
	},

	/**
	 * Delete a custom Terraform file
	 */
	async deleteFile(
		environment: string,
		request: DeleteFileRequest,
	): Promise<void> {
		const response = await fetchWithTokenRetry(
			`${API_BASE_URL}/api/custom-terraform/file?env=${encodeURIComponent(environment)}`,
			{
				method: "DELETE",
				headers: {
					"Content-Type": "application/json",
				},
				body: JSON.stringify(request),
			},
		);
		if (!response.ok) {
			throw new Error("Failed to delete file");
		}
	},

	/**
	 * Get available bridge variables
	 */
	async getBridgeVariables(
		environment: string,
	): Promise<BridgeVariablesResponse> {
		const response = await fetchWithTokenRetry(
			`${API_BASE_URL}/api/custom-terraform/bridge-vars?env=${encodeURIComponent(environment)}`,
		);
		if (!response.ok) {
			throw new Error("Failed to fetch bridge variables");
		}
		return response.json();
	},

	/**
	 * Get custom module deployment status
	 */
	async getCustomModuleStatus(
		environment: string,
	): Promise<CustomModuleStatusResponse> {
		const response = await fetchWithTokenRetry(
			`${API_BASE_URL}/api/custom-terraform/modules?env=${encodeURIComponent(environment)}`,
		);
		if (!response.ok) {
			throw new Error("Failed to fetch custom module status");
		}
		return response.json();
	},
};
