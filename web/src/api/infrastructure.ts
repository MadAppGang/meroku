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

export interface AccountInfo {
	profile: string;
	accountId: string;
	region: string;
}

export interface ECSClusterInfo {
	clusterName: string;
	clusterArn: string;
	status: string;
	registeredTasks: number;
	runningTasks: number;
	activeServices: number;
	capacityProviders: string[];
	containerInsights: string;
}

export interface ECSNetworkInfo {
	vpc: VPCInfo;
	availabilityZones: string[];
	subnets: SubnetInfo[];
	serviceDiscovery: ServiceDiscovery;
}

export interface VPCInfo {
	vpcId: string;
	cidrBlock: string;
	state: string;
}

export interface SubnetInfo {
	subnetId: string;
	availabilityZone: string;
	cidrBlock: string;
	availableIpCount: number;
	type: string;
}

export interface ServiceDiscovery {
	namespaceId: string;
	namespaceName: string;
	serviceCount: number;
}

export interface ECSServicesInfo {
	services: ServiceInfo[];
	scheduledTasks: TaskInfo[];
	eventTasks: TaskInfo[];
	totalTasks: number;
}

export interface ServiceInfo {
	serviceName: string;
	status: string;
	desiredCount: number;
	runningCount: number;
	pendingCount: number;
	launchType: string;
	taskDefinition: string;
}

export interface TaskInfo {
	taskName: string;
	taskType: string;
	schedule?: string;
	eventPattern?: string;
	enabled: boolean;
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

	async getAccountInfo(): Promise<AccountInfo> {
		const response = await fetch(`${API_BASE_URL}/api/account`);
		if (!response.ok) {
			throw new Error("Failed to fetch account info");
		}
		return response.json();
	},

	async getECSClusterInfo(env: string): Promise<ECSClusterInfo> {
		const response = await fetch(`${API_BASE_URL}/api/ecs/cluster?env=${encodeURIComponent(env)}`);
		if (!response.ok) {
			throw new Error("Failed to fetch ECS cluster info");
		}
		return response.json();
	},

	async getECSNetworkInfo(env: string): Promise<ECSNetworkInfo> {
		const response = await fetch(`${API_BASE_URL}/api/ecs/network?env=${encodeURIComponent(env)}`);
		if (!response.ok) {
			throw new Error("Failed to fetch ECS network info");
		}
		return response.json();
	},

	async getECSServicesInfo(env: string): Promise<ECSServicesInfo> {
		const response = await fetch(`${API_BASE_URL}/api/ecs/services?env=${encodeURIComponent(env)}`);
		if (!response.ok) {
			throw new Error("Failed to fetch ECS services info");
		}
		return response.json();
	},
};