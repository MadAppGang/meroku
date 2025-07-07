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

// SSH interfaces
export interface SSHCapability {
	enabled: boolean;
	reason?: string;
}

export interface SSHMessage {
	type: 'input' | 'output' | 'error' | 'connected' | 'disconnected';
	data: string;
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

// SSM Parameter interfaces
export interface SSMParameter {
	name: string;
	value: string;
	type: 'String' | 'StringList' | 'SecureString';
	version: number;
	description?: string;
	lastModifiedDate?: string;
	arn?: string;
}

export interface SSMParameterMetadata {
	name: string;
	type: 'String' | 'StringList' | 'SecureString';
	lastModifiedDate: string;
	version: number;
	description?: string;
}

export interface SSMParameterCreateRequest {
	name: string;
	value: string;
	type: 'String' | 'StringList' | 'SecureString';
	description?: string;
	overwrite?: boolean;
}

// S3 interfaces
export interface S3FileContent {
	bucket: string;
	key: string;
	content: string;
	lastModified?: string;
}

export interface S3PutFileRequest {
	bucket: string;
	key: string;
	content: string;
}

export interface NodePosition {
	nodeId: string;
	x: number;
	y: number;
}

export interface BoardPositions {
	environment: string;
	positions: NodePosition[];
}

export interface TestEventRequest {
	source: string;
	detailType: string;
	detail?: Record<string, any>;
}

export interface TestEventResponse {
	success: boolean;
	eventId?: string;
	message: string;
}

export interface EventTaskInfo {
	name: string;
	ruleName: string;
	sources: string[];
	detailTypes: string[];
	dockerImage?: string;
}

export interface SESStatusResponse {
	inSandbox: boolean;
	sendingEnabled: boolean;
	dailyQuota: number;
	maxSendRate: number;
	sentLast24Hours: number;
	verifiedDomains: string[];
	verifiedEmails: string[];
	suppressionListEnabled: boolean;
	reputationStatus: string;
	region: string;
}

export interface SESSandboxInfo {
	limitations: string[];
	howToExit: string[];
	requiredInfo: string[];
	tips: string[];
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
		// Handle both relative and absolute URLs
		let wsUrl: string;
		if (API_BASE_URL) {
			wsUrl = `${API_BASE_URL.replace(/^http/, 'ws')}/ws/logs?env=${encodeURIComponent(env)}&service=${encodeURIComponent(serviceName)}`;
		} else {
			// If no base URL, use current host with ws protocol
			const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
			const host = window.location.host;
			wsUrl = `${protocol}//${host}/ws/logs?env=${encodeURIComponent(env)}&service=${encodeURIComponent(serviceName)}`;
		}
		
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

	// Check SSH capability for a task
	async checkSSHCapability(env: string, serviceName: string, taskArn: string): Promise<SSHCapability> {
		const params = new URLSearchParams({
			env,
			service: serviceName,
			taskArn,
		});
		
		const response = await fetch(`${API_BASE_URL}/api/ssh/capability?${params}`);
		if (!response.ok) {
			throw new Error("Failed to check SSH capability");
		}
		return response.json();
	},

	// Connect to SSH session via WebSocket
	connectToSSH(
		env: string,
		serviceName: string,
		taskArn: string,
		containerName: string | undefined,
		onMessage: (message: SSHMessage) => void,
		onError?: (error: Error) => void,
		onClose?: () => void
	): WebSocket {
		const params = new URLSearchParams({
			env,
			service: serviceName,
			taskArn,
		});
		if (containerName) {
			params.append('container', containerName);
		}

		// Handle both relative and absolute URLs
		let wsUrl: string;
		if (API_BASE_URL) {
			wsUrl = `${API_BASE_URL.replace(/^http/, 'ws')}/ws/ssh?${params}`;
		} else {
			// If no base URL, use current host with ws protocol
			const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
			const host = window.location.host;
			wsUrl = `${protocol}//${host}/ws/ssh?${params}`;
		}
		
		console.log('Connecting to SSH WebSocket:', wsUrl);
		
		// First check if the backend is reachable
		try {
			const ws = new WebSocket(wsUrl);
			
			// Add a connection timeout
			const connectionTimeout = setTimeout(() => {
				if (ws.readyState !== WebSocket.OPEN) {
					ws.close();
					onError?.(new Error('SSH connection timeout. Please check if the backend server is running.'));
				}
			}, 5000);

			ws.onopen = () => {
				clearTimeout(connectionTimeout);
				console.log('SSH WebSocket connected');
			};

		ws.onmessage = (event) => {
			try {
				const message: SSHMessage = JSON.parse(event.data);
				onMessage(message);
			} catch (err) {
				console.error('Failed to parse SSH message:', err);
			}
		};

		ws.onerror = (event) => {
			console.error('SSH WebSocket error:', event);
			// Provide more context about the error
			let errorMessage = 'SSH WebSocket connection error';
			if (ws.readyState === WebSocket.CLOSED) {
				errorMessage = 'Unable to connect to SSH WebSocket. The backend server may not be running or the /ws/ssh endpoint is not available.';
			}
			onError?.(new Error(errorMessage));
		};

		ws.onclose = () => {
			console.log('SSH WebSocket disconnected');
			onClose?.();
		};

			// Add send method for input
			(ws as any).sendInput = (input: string) => {
				if (ws.readyState === WebSocket.OPEN) {
					ws.send(JSON.stringify({ type: 'input', data: input }));
				}
			};

			return ws;
		} catch (error) {
			console.error('Failed to create WebSocket:', error);
			onError?.(new Error('Failed to create SSH WebSocket connection'));
			throw error;
		}
	},

	// SSM Parameter Store APIs
	async getSSMParameter(name: string): Promise<SSMParameter> {
		const response = await fetch(`${API_BASE_URL}/api/ssm/parameter?name=${encodeURIComponent(name)}`);
		if (!response.ok) {
			const error: ErrorResponse = await response.json();
			throw new Error(error.error || "Failed to fetch SSM parameter");
		}
		return response.json();
	},

	async createOrUpdateSSMParameter(params: SSMParameterCreateRequest): Promise<void> {
		const response = await fetch(`${API_BASE_URL}/api/ssm/parameter`, {
			method: "PUT",
			headers: {
				"Content-Type": "application/json",
			},
			body: JSON.stringify(params),
		});
		if (!response.ok) {
			const error: ErrorResponse = await response.json();
			throw new Error(error.error || "Failed to create/update SSM parameter");
		}
	},

	async deleteSSMParameter(name: string): Promise<void> {
		const response = await fetch(`${API_BASE_URL}/api/ssm/parameter?name=${encodeURIComponent(name)}`, {
			method: "DELETE",
		});
		if (!response.ok) {
			const error: ErrorResponse = await response.json();
			throw new Error(error.error || "Failed to delete SSM parameter");
		}
	},

	async listSSMParameters(prefix?: string): Promise<SSMParameterMetadata[]> {
		const params = new URLSearchParams();
		if (prefix) {
			params.append("prefix", prefix);
		}
		const response = await fetch(`${API_BASE_URL}/api/ssm/parameters?${params}`);
		if (!response.ok) {
			const error: ErrorResponse = await response.json();
			throw new Error(error.error || "Failed to list SSM parameters");
		}
		return response.json();
	},

	// S3 File APIs
	async getS3File(bucket: string, key: string): Promise<S3FileContent> {
		const params = new URLSearchParams({ bucket, key });
		const response = await fetch(`${API_BASE_URL}/api/s3/file?${params}`);
		if (!response.ok) {
			const error: ErrorResponse = await response.json();
			throw new Error(error.error || "Failed to fetch S3 file");
		}
		return response.json();
	},

	async putS3File(params: S3PutFileRequest): Promise<void> {
		const response = await fetch(`${API_BASE_URL}/api/s3/file`, {
			method: "PUT",
			headers: {
				"Content-Type": "application/json",
			},
			body: JSON.stringify(params),
		});
		if (!response.ok) {
			const error: ErrorResponse = await response.json();
			throw new Error(error.error || "Failed to update S3 file");
		}
	},

	// Node Positions APIs
	async getNodePositions(environment: string): Promise<BoardPositions> {
		const response = await fetch(`${API_BASE_URL}/api/positions?environment=${encodeURIComponent(environment)}`);
		if (!response.ok) {
			const error: ErrorResponse = await response.json();
			throw new Error(error.error || "Failed to fetch node positions");
		}
		return response.json();
	},

	async saveNodePositions(positions: BoardPositions): Promise<void> {
		const response = await fetch(`${API_BASE_URL}/api/positions`, {
			method: "POST",
			headers: {
				"Content-Type": "application/json",
			},
			body: JSON.stringify(positions),
		});
		if (!response.ok) {
			const error: ErrorResponse = await response.json();
			throw new Error(error.error || "Failed to save node positions");
		}
	},

	// EventBridge APIs
	async sendTestEvent(event: TestEventRequest): Promise<TestEventResponse> {
		const response = await fetch(`${API_BASE_URL}/api/eventbridge/send-test-event`, {
			method: "POST",
			headers: {
				"Content-Type": "application/json",
			},
			body: JSON.stringify(event),
		});
		if (!response.ok) {
			const error: ErrorResponse = await response.json();
			throw new Error(error.error || "Failed to send test event");
		}
		return response.json();
	},

	async getEventTasks(env: string): Promise<EventTaskInfo[]> {
		const response = await fetch(`${API_BASE_URL}/api/eventbridge/event-tasks?env=${encodeURIComponent(env)}`);
		if (!response.ok) {
			const error: ErrorResponse = await response.json();
			throw new Error(error.error || "Failed to fetch event tasks");
		}
		return response.json();
	},

	// SES APIs
	async getSESStatus(): Promise<SESStatusResponse> {
		const response = await fetch(`${API_BASE_URL}/api/ses/status`);
		if (!response.ok) {
			const error: ErrorResponse = await response.json();
			throw new Error(error.error || "Failed to fetch SES status");
		}
		return response.json();
	},

	async getSESSandboxInfo(): Promise<SESSandboxInfo> {
		const response = await fetch(`${API_BASE_URL}/api/ses/sandbox-info`);
		if (!response.ok) {
			const error: ErrorResponse = await response.json();
			throw new Error(error.error || "Failed to fetch SES sandbox info");
		}
		return response.json();
	},

	async sendTestEmail(env: string, to: string, subject: string, body: string): Promise<{ success: boolean; messageId?: string; error?: string }> {
		const response = await fetch(`${API_BASE_URL}/api/ses/send-test-email?env=${encodeURIComponent(env)}`, {
			method: "POST",
			headers: {
				"Content-Type": "application/json",
			},
			body: JSON.stringify({ to, subject, body }),
		});
		if (!response.ok) {
			const error: ErrorResponse = await response.json();
			throw new Error(error.error || "Failed to send test email");
		}
		return response.json();
	},

	async getProductionAccessPrefill(env: string): Promise<{
		websiteUrl: string;
		useCaseDescription: string;
		mailingListBuildProcess: string;
		bounceComplaintProcess: string;
		additionalInfo: string;
		expectedDailyVolume: string;
		expectedPeakVolume: string;
		domainName: string;
	}> {
		const response = await fetch(`${API_BASE_URL}/api/ses/production-access-prefill?env=${encodeURIComponent(env)}`);
		if (!response.ok) {
			const error: ErrorResponse = await response.json();
			throw new Error(error.error || "Failed to fetch production access prefill");
		}
		return response.json();
	},

	async requestSESProductionAccess(env: string, data: {
		websiteUrl: string;
		useCaseDescription: string;
		mailingListBuildProcess: string;
		bounceComplaintProcess: string;
		additionalInfo: string;
		expectedDailyVolume: string;
		expectedPeakVolume: string;
		contactLanguage?: string;
	}): Promise<{
		success: boolean;
		caseId?: string;
		error?: string;
		message?: string;
	}> {
		const response = await fetch(`${API_BASE_URL}/api/ses/request-production?env=${encodeURIComponent(env)}`, {
			method: "POST",
			headers: {
				"Content-Type": "application/json",
			},
			body: JSON.stringify(data),
		});
		if (!response.ok) {
			const error: ErrorResponse = await response.json();
			throw new Error(error.error || "Failed to request SES production access");
		}
		return response.json();
	},
};