export interface Environment {
	name: string;
	path: string;
}

export interface ConfigResponse {
	content: string;
}

export interface ErrorResponse {
	error: string;
}

const API_BASE_URL = import.meta.env.VITE_API_URL || "";

export const infrastructureApi = {
	async getEnvironments(): Promise<Environment[]> {
		const response = await fetch(`${API_BASE_URL}/api/environments`);
		if (!response.ok) {
			throw new Error("Failed to fetch environments");
		}
		return response.json();
	},

	async getEnvironmentConfig(name: string): Promise<string> {
		const response = await fetch(`${API_BASE_URL}/api/environment?name=${encodeURIComponent(name)}`);
		if (!response.ok) {
			const error: ErrorResponse = await response.json();
			throw new Error(error.error || "Failed to fetch environment config");
		}
		const data: ConfigResponse = await response.json();
		return data.content;
	},

	async updateEnvironmentConfig(name: string, content: string): Promise<void> {
		const response = await fetch(`${API_BASE_URL}/api/environment/update?name=${encodeURIComponent(name)}`, {
			method: "POST",
			headers: {
				"Content-Type": "application/json",
			},
			body: JSON.stringify({ content }),
		});
		if (!response.ok) {
			const error: ErrorResponse = await response.json();
			throw new Error(error.error || "Failed to update environment config");
		}
	},
};