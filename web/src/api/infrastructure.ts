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

// Autoscaling interfaces
export interface ServiceAutoscalingInfo {
	serviceName: string;
	enabled: boolean;
	currentDesiredCount: number;
	minCapacity: number;
	maxCapacity: number;
	targetCPU: number;
	targetMemory: number;
	cpu: number;
	memory: number;
	currentCPUUtilization?: number;
	currentMemoryUtilization?: number;
	lastScalingActivity?: {
		time: string;
		description: string;
		cause: string;
	};
}

export interface ServiceScalingHistory {
	serviceName: string;
	events: Array<{
		timestamp: string;
		activityType: string;
		fromCapacity: number;
		toCapacity: number;
		reason: string;
		statusCode: string;
	}>;
}

export interface ServiceMetrics {
	serviceName: string;
	metrics: {
		cpu: Array<{ timestamp: string; value: number }>;
		memory: Array<{ timestamp: string; value: number }>;
		taskCount: Array<{ timestamp: string; value: number }>;
		requestCount?: Array<{ timestamp: string; value: number }>;
	};
}

// ECS Task interfaces
export interface ECSTaskInfo {
	taskArn: string;
	taskDefinitionArn: string;
	serviceName: string;
	launchType: string;
	lastStatus: string;
	desiredStatus: string;
	healthStatus?: string;
	createdAt: string;
	startedAt?: string;
	stoppedAt?: string;
	cpu: string;
	memory: string;
	availabilityZone: string;
	connectivityAt?: string;
	pullStartedAt?: string;
	pullStoppedAt?: string;
}

export interface ServiceTasksResponse {
	serviceName: string;
	tasks: ECSTaskInfo[];
}

// Logs interfaces
export interface LogEntry {
	timestamp: string;
	message: string;
	level: 'info' | 'warning' | 'error' | 'debug';
	stream: string;
}

export interface LogsResponse {
	serviceName: string;
	logs: LogEntry[];
	nextToken?: string;
}

export interface ECSTask {
	taskArn: string;
	taskDefinitionArn: string;
	clusterArn: string;
	containerInstanceArn?: string;
	lastStatus: string;
	desiredStatus: string;
	healthStatus?: string;
	createdAt: string;
	startedAt?: string;
}

export interface ECSTasksResponse {
	tasks: ECSTask[];
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

	async getServiceAutoscaling(env: string, serviceName: string): Promise<ServiceAutoscalingInfo> {
		const response = await fetch(
			`${API_BASE_URL}/api/ecs/autoscaling?env=${encodeURIComponent(env)}&service=${encodeURIComponent(serviceName)}`
		);
		if (!response.ok) {
			throw new Error("Failed to fetch autoscaling info");
		}
		return response.json();
	},

	async getServiceScalingHistory(
		env: string,
		serviceName: string,
		hours: number = 24
	): Promise<ServiceScalingHistory> {
		const response = await fetch(
			`${API_BASE_URL}/api/ecs/scaling-history?env=${encodeURIComponent(env)}&service=${encodeURIComponent(serviceName)}&hours=${hours}`
		);
		if (!response.ok) {
			throw new Error("Failed to fetch scaling history");
		}
		return response.json();
	},

	async getServiceMetrics(env: string, serviceName: string): Promise<ServiceMetrics> {
		const response = await fetch(
			`${API_BASE_URL}/api/ecs/metrics?env=${encodeURIComponent(env)}&service=${encodeURIComponent(serviceName)}`
		);
		if (!response.ok) {
			throw new Error("Failed to fetch service metrics");
		}
		return response.json();
	},

	async getECSTasks(clusterName: string, serviceName: string): Promise<ECSTasksResponse> {
		const response = await fetch(
			`${API_BASE_URL}/api/ecs/tasks?cluster=${encodeURIComponent(clusterName)}&service=${encodeURIComponent(serviceName)}`
		);
		if (!response.ok) {
			throw new Error("Failed to fetch ECS tasks");
		}
		return response.json();
	},

	async getServiceLogs(
		env: string,
		serviceName: string,
		limit: number = 100,
		nextToken?: string
	): Promise<LogsResponse> {
		const params = new URLSearchParams({
			env,
			service: serviceName,
			limit: limit.toString(),
		});
		if (nextToken) {
			params.append("nextToken", nextToken);
		}
		
		const response = await fetch(`${API_BASE_URL}/api/logs?${params}`);
		if (!response.ok) {
			throw new Error("Failed to fetch logs");
		}
		return response.json();
	},

	// WebSocket connection for real-time logs
	connectToLogStream(
		env: string,
		serviceName: string,
		onMessage: (logs: LogEntry[]) => void,
		onError?: (error: Error) => void,
		onConnect?: () => void
	): WebSocket {
		const wsUrl = `${API_BASE_URL.replace(/^http/, 'ws')}/ws/logs?env=${encodeURIComponent(env)}&service=${encodeURIComponent(serviceName)}`;
		const ws = new WebSocket(wsUrl);

		ws.onopen = () => {
			console.log('Connected to log stream');
			onConnect?.();
		};

		ws.onmessage = (event) => {
			try {
				const data = JSON.parse(event.data);
				if (data.type === 'logs' && data.data) {
					onMessage(data.data);
				} else if (data.error) {
					onError?.(new Error(data.error));
				}
			} catch (err) {
				console.error('Failed to parse log message:', err);
			}
		};

		ws.onerror = (event) => {
			console.error('WebSocket error:', event);
			onError?.(new Error('WebSocket connection error'));
		};

		ws.onclose = () => {
			console.log('Disconnected from log stream');
		};

		return ws;
	},

	// Get tasks for a service
	async getServiceTasks(env: string, serviceName: string): Promise<ServiceTasksResponse> {
		const params = new URLSearchParams({
			env,
			service: serviceName,
		});
		
		const response = await fetch(`${API_BASE_URL}/api/ecs/tasks?${params}`);
		if (!response.ok) {
			throw new Error("Failed to fetch service tasks");
		}
		return response.json();
	},
};