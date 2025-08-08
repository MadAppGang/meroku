import type {
	AmplifyAppsResponse,
	AmplifyBuildLogsResponse,
	TriggerBuildRequest,
	TriggerBuildResponse,
} from "../types/amplify";

const API_BASE_URL = "/api";

export const amplifyApi = {
	/**
	 * Get all Amplify apps for a specific environment
	 */
	async getApps(
		environment: string,
		profile?: string,
	): Promise<AmplifyAppsResponse> {
		const params = new URLSearchParams({ environment });
		if (profile) {
			params.append("profile", profile);
		}

		const response = await fetch(`${API_BASE_URL}/amplify/apps?${params}`);
		if (!response.ok) {
			const error = await response.text();
			throw new Error(`Failed to fetch Amplify apps: ${error}`);
		}
		return response.json();
	},

	/**
	 * Get build logs for a specific job
	 */
	async getBuildLogs(
		appId: string,
		branchName: string,
		jobId: string,
		profile?: string,
	): Promise<AmplifyBuildLogsResponse> {
		const params = new URLSearchParams({ appId, branchName, jobId });
		if (profile) {
			params.append("profile", profile);
		}

		const response = await fetch(
			`${API_BASE_URL}/amplify/build-logs?${params}`,
		);
		if (!response.ok) {
			const error = await response.text();
			throw new Error(`Failed to fetch build logs: ${error}`);
		}
		return response.json();
	},

	/**
	 * Trigger a new build for a branch
	 */
	async triggerBuild(
		request: TriggerBuildRequest,
	): Promise<TriggerBuildResponse> {
		const response = await fetch(`${API_BASE_URL}/amplify/trigger-build`, {
			method: "POST",
			headers: {
				"Content-Type": "application/json",
			},
			body: JSON.stringify(request),
		});

		if (!response.ok) {
			const error = await response.text();
			throw new Error(`Failed to trigger build: ${error}`);
		}
		return response.json();
	},
};
