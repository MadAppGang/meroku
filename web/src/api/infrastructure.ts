import type { PricingResponse } from "../hooks/use-pricing";
import { fetchWithTokenRetry } from "../utils/fetchWithRetry";

export interface Environment {
	name: string;
	path: string;
	isActive?: boolean;
	profile?: string;
	accountId?: string;
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

export interface DatabaseConfig {
	type: "rds" | "aurora";
	minCapacity?: number;
	maxCapacity?: number;
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

// ECR Cross-Account interfaces
export interface ECRTrustedAccount {
	account_id: string;
	env: string;
	region: string;
}

export interface ECRSource {
	name: string;
	account_id: string;
	region: string;
	ecr_strategy: string;
	trusted_accounts: ECRTrustedAccount[];
}

export interface ECRSourcesResponse {
	sources: ECRSource[];
}

export interface ConfigureCrossAccountECRRequest {
	source_env: string;
	target_env: string;
}

export interface ConfigureCrossAccountECRResponse {
	success: boolean;
	modified_files: string[];
	source_env: {
		name: string;
		account_id: string;
		region: string;
	};
	target_env: {
		name: string;
		account_id: string;
		region: string;
	};
	next_steps: string[];
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
	type: "input" | "output" | "error" | "connected" | "disconnected";
	data: string;
}

// Logs interfaces
export interface LogEntry {
	timestamp: string;
	message: string;
	level: "info" | "warning" | "error" | "debug";
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
	type: "String" | "StringList" | "SecureString";
	version: number;
	description?: string;
	lastModifiedDate?: string;
	arn?: string;
}

export interface SSMParameterMetadata {
	name: string;
	type: "String" | "StringList" | "SecureString";
	lastModifiedDate: string;
	version: number;
	description?: string;
}

export interface SSMParameterCreateRequest {
	name: string;
	value: string;
	type: "String" | "StringList" | "SecureString";
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

export interface EdgeHandlePosition {
	edgeId: string;
	sourceHandle?: string;
	targetHandle?: string;
}

export interface BoardPositions {
	environment: string;
	positions: NodePosition[];
	edgeHandles?: EdgeHandlePosition[];
}

export interface TestEventRequest {
	source: string;
	detailType: string;
	detail?: Record<string, unknown>;
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

export interface S3BucketInfo {
	name: string;
	type: "static" | "configured";
	publicAccess: boolean;
	versioning: string;
	corsRules?: Array<{
		allowedHeaders: string[];
		allowedMethods: string[];
		allowedOrigins: string[];
		exposeHeaders: string[];
		maxAgeSeconds: number;
	}>;
	consoleUrl: string;
	region: string;
	creationDate?: string;
}

export interface FargateCPUOption {
	cpu: number;
	vcpu: string;
	memoryOptions: number[];
}

export interface FargateOptionsResponse {
	options: FargateCPUOption[];
}

// ---------------------------------------------------------------------------
// Compute pools (EC2 capacity) — the three READ-ONLY endpoints of
// architecture.md §3. Pool writes are YAML writes and go through
// /api/environment/update like every other config change (FR-43).
//
// One rule governs every one of these calls: an AWS failure never produces a
// non-200. A degraded answer arrives inside a 200 body and says so in `source`,
// `credentialsState` and `notice` (AC-2), so the UI must read those fields
// rather than assume a successful fetch means live data.
// ---------------------------------------------------------------------------

/** Envelope-level data provenance (FR-12). */
export type ComputeSource = "aws_api" | "fallback" | "partial";

export type ComputeCredentialsState = "ok" | "missing" | "expired" | "denied";

/** Per-type price provenance (FR-5). */
export type ComputePriceSource = "aws_api" | "fallback" | "unavailable";

export type ComputePosture = "performance-first" | "balanced" | "cost-first";

export const COMPUTE_POSTURES: ComputePosture[] = [
	"performance-first",
	"balanced",
	"cost-first",
];

export interface ComputeInstanceTypeInfo {
	instanceType: string;
	vcpu: number;
	memoryMiB: number;
	architectures: string[];
	currentGeneration: boolean;
	networkPerformance: string;
	baselineBandwidthMbps: number | null;
	/**
	 * The raw un-trunked ENI limit exactly as DescribeInstanceTypes reports it.
	 * NOT the task density: density is computed server-side and reported as
	 * `signals.densityBasis` on the recommendation.
	 */
	maximumNetworkInterfaces: number;
	gpuCount: number;
	gpuMemoryMiB: number | null;
	gpuName: string | null;
	burstable: boolean;
	bareMetal: boolean;
	supportedUsageClasses: string[];
	onDemandHourly: number | null;
	priceSource: ComputePriceSource;
}

export interface ComputeInstanceTypesResponse {
	region: string;
	source: ComputeSource;
	credentialsState: ComputeCredentialsState;
	filtered: boolean;
	totalAvailable: number;
	cachedAt: string | null;
	pricingDate: string | null;
	/**
	 * Non-null whenever ANY price in this payload is a fallback value, in which
	 * case the figures are indicative us-east-1 list prices and must never be
	 * rendered as the environment's own (C-18, DEV-22).
	 */
	pricingRegion: string | null;
	instanceDataDate: string | null;
	/** False under fallback: the region's availability was not verified (NFR-7). */
	availabilityVerified: boolean;
	notice: string | null;
	instanceTypes: ComputeInstanceTypeInfo[];
}

export interface ComputeSpotAZPrice {
	az: string;
	hourly: number;
	asOf: string;
}

export interface ComputeSpotQuote {
	instanceType: string;
	byAz: ComputeSpotAZPrice[];
	/** Null rather than zero when there is no spot market for this type (EC-5). */
	min: number | null;
	max: number | null;
	median: number | null;
	spotAvailable: boolean;
}

export interface ComputeSpotPricesResponse {
	region: string;
	source: ComputeSource;
	credentialsState: ComputeCredentialsState;
	asOf: string;
	notice: string | null;
	prices: ComputeSpotQuote[];
}

/**
 * The scoring dimensions, each in [0,1]. Mirrors `computeScores` in
 * `app/api_compute.go`.
 *
 * There are FOUR, not five. `waste` and `headroom` were near-inverses of one
 * axis — one measuring fill, the other slack — so every posture expressed its
 * real preference as the difference between two partly-cancelling terms, and on
 * a fleet with no CloudWatch history `headroom` was identical for every
 * candidate, which ranked nothing at all. They are replaced by one
 * `utilisation` term; `app/recommend/score.go` carries the analysis.
 */
export interface ComputeCandidateScores {
	/** Shape: how closely the instance's memory-per-vCPU matches the workload's. */
	fit: number;
	/** Fill: how close the instance's ACHIEVED occupancy lands to the posture's target. */
	utilisation: number;
	/** Money: the pool this candidate implies, per hour per task, against the cheapest survivor. */
	cost: number;
	/** Generation: newest of its family in this region's catalog scores 1. */
	modernity: number;
}

export interface ComputeCandidate {
	instanceType: string;
	vcpu: number;
	memoryMiB: number;
	architecture: string;
	scores: ComputeCandidateScores;
	total: number;
	effectiveHourly: number;
	tasksPerInstance: number;
	instancesAtFloor: number;
	/**
	 * What this candidate's pool costs per hour, per task it runs:
	 * effectiveHourly × instances ÷ tasks. It replaces `costPerTaskSlot`, which
	 * priced slots no task occupies and so rated an 8 vCPU box the better buy
	 * for a fleet that occupies 4% of it.
	 */
	costPerTask: number;
	spotMedianHourly: number | null;
	reason: string;
}

export interface ComputeMiss {
	instanceType: string;
	failedRule: string;
	needed: number;
	available: number;
	unit: string;
}

export interface ComputeShape {
	vcpu: number;
	memGiB: number;
	ratio: number;
	taskCount?: number;
}

export interface ComputeWeights {
	configured: number;
	actual: number;
}

/** C-10 — the memory-per-vCPU clamp is reported, never applied silently. */
export interface ComputeRatioSignal {
	raw: number;
	effective: number;
	catalogMin: number;
	catalogMax: number;
	clampedTo: "none" | "min" | "max";
}

export interface ComputeServiceSignal {
	name: string;
	datapoints: number;
	cpuAvg: number;
	cpuPeak: number;
	memAvg: number;
	memPeak: number;
	status: string;
	reason?: string;
}

export interface ComputeSignals {
	configured: ComputeShape;
	actual: ComputeShape | null;
	coverage: number;
	weights: ComputeWeights;
	ratio: ComputeRatioSignal;
	cloudwatch: "ok" | "partial" | "no_data" | "unavailable" | "timeout";
	networkMode: "bridge" | "awsvpc";
	trunking: "enabled" | "disabled" | "unknown" | "not_applicable";
	densityBasis: "cpu_memory_only" | "max_enis_minus_one" | "trunked_table";
	/** Candidates removed by the non-finite gate (C-4). Non-empty here is a bug. */
	dropped: ComputeMiss[];
	services: ComputeServiceSignal[];
}

/**
 * A YAML-ready pool. snake_case on purpose: the UI writes it straight into
 * `compute.pools` without translating a single key.
 */
export interface ComputeSuggestedPool {
	name: string;
	enabled: boolean;
	instance_types: string[];
	capacity_type: string;
	on_demand_base: number;
	min_size: number;
	max_size: number;
	target_capacity: number;
	network_mode: string;
	ami_family: string;
	root_volume_gb: number;
	/** EC-6: spot was asked for, spot was unavailable, the pool fell back. */
	downgraded: boolean;
}

export interface ComputeRecommendationResponse {
	region: string;
	source: ComputeSource;
	credentialsState: ComputeCredentialsState;
	posture: ComputePosture;
	classification: string;
	/**
	 * Present only when it differs from `classification`: the class read off
	 * measured utilisation, which the region carries no type able to serve, so
	 * sizing fell back to the ratio class in `classification`. Without it the
	 * fallback is silent and a fleet idling at 9 % CPU is sized memory-heavy with
	 * no hint that the utilisation was read at all.
	 */
	classificationSuppressed?: string;
	basis: "measured" | "configured" | "default";
	unsatisfiable: boolean;
	constraint: string | null;
	primary: ComputeCandidate | null;
	ranked: ComputeCandidate[];
	nearestMisses?: ComputeMiss[];
	signals: ComputeSignals;
	suggestedPool: ComputeSuggestedPool;
	pricingRegion: string | null;
	notice: string | null;
}

export interface ComputeRecommendationQuery {
	env: string;
	posture?: ComputePosture;
	pool?: string;
	services?: string[];
	window?: number;
	limit?: number;
	gpu?: boolean;
	amiFamily?: string;
	networkMode?: string;
}

const API_BASE_URL = import.meta.env.VITE_API_URL || "";

export const infrastructureApi = {
	async getEnvironments(): Promise<Environment[]> {
		const response = await fetchWithTokenRetry(
			`${API_BASE_URL}/api/environments`,
		);
		if (!response.ok) {
			throw new Error("Failed to fetch environments");
		}
		return response.json();
	},

	async getEnvironmentConfig(name: string): Promise<string> {
		const response = await fetchWithTokenRetry(
			`${API_BASE_URL}/api/environment?name=${encodeURIComponent(name)}`,
		);
		if (!response.ok) {
			const error: ErrorResponse = await response.json();
			throw new Error(error.error || "Failed to fetch environment config");
		}
		const data: ConfigResponse = await response.json();
		return data.content;
	},

	async updateEnvironmentConfig(name: string, content: string): Promise<void> {
		const response = await fetchWithTokenRetry(
			`${API_BASE_URL}/api/environment/update?name=${encodeURIComponent(name)}`,
			{
				method: "POST",
				headers: {
					"Content-Type": "application/json",
				},
				body: JSON.stringify({ content }),
			},
		);
		if (!response.ok) {
			const error: ErrorResponse = await response.json();
			throw new Error(error.error || "Failed to update environment config");
		}
	},

	async getAccountInfo(): Promise<AccountInfo> {
		const response = await fetchWithTokenRetry(`${API_BASE_URL}/api/account`);
		if (!response.ok) {
			throw new Error("Failed to fetch account info");
		}
		return response.json();
	},

	async getECSClusterInfo(env: string): Promise<ECSClusterInfo> {
		const response = await fetch(
			`${API_BASE_URL}/api/ecs/cluster?env=${encodeURIComponent(env)}`,
		);
		if (!response.ok) {
			throw new Error("Failed to fetch ECS cluster info");
		}
		return response.json();
	},

	async getECSNetworkInfo(env: string): Promise<ECSNetworkInfo> {
		const response = await fetch(
			`${API_BASE_URL}/api/ecs/network?env=${encodeURIComponent(env)}`,
		);
		if (!response.ok) {
			throw new Error("Failed to fetch ECS network info");
		}
		return response.json();
	},

	async getECSServicesInfo(env: string): Promise<ECSServicesInfo> {
		const response = await fetch(
			`${API_BASE_URL}/api/ecs/services?env=${encodeURIComponent(env)}`,
		);
		if (!response.ok) {
			throw new Error("Failed to fetch ECS services info");
		}
		return response.json();
	},

	async getServiceAutoscaling(
		env: string,
		serviceName: string,
	): Promise<ServiceAutoscalingInfo> {
		const response = await fetch(
			`${API_BASE_URL}/api/ecs/autoscaling?env=${encodeURIComponent(env)}&service=${encodeURIComponent(serviceName)}`,
		);
		if (!response.ok) {
			throw new Error("Failed to fetch autoscaling info");
		}
		return response.json();
	},

	async getServiceScalingHistory(
		env: string,
		serviceName: string,
		hours: number = 24,
	): Promise<ServiceScalingHistory> {
		const response = await fetch(
			`${API_BASE_URL}/api/ecs/scaling-history?env=${encodeURIComponent(env)}&service=${encodeURIComponent(serviceName)}&hours=${hours}`,
		);
		if (!response.ok) {
			throw new Error("Failed to fetch scaling history");
		}
		return response.json();
	},

	async getServiceMetrics(
		env: string,
		serviceName: string,
	): Promise<ServiceMetrics> {
		const response = await fetch(
			`${API_BASE_URL}/api/ecs/metrics?env=${encodeURIComponent(env)}&service=${encodeURIComponent(serviceName)}`,
		);
		if (!response.ok) {
			throw new Error("Failed to fetch service metrics");
		}
		return response.json();
	},

	async getECSTasks(
		clusterName: string,
		serviceName: string,
	): Promise<ECSTasksResponse> {
		const response = await fetch(
			`${API_BASE_URL}/api/ecs/tasks?cluster=${encodeURIComponent(clusterName)}&service=${encodeURIComponent(serviceName)}`,
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
		nextToken?: string,
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
		onConnect?: () => void,
	): WebSocket {
		// Handle both relative and absolute URLs
		let wsUrl: string;
		if (API_BASE_URL) {
			wsUrl = `${API_BASE_URL.replace(/^http/, "ws")}/ws/logs?env=${encodeURIComponent(env)}&service=${encodeURIComponent(serviceName)}`;
		} else {
			// If no base URL, use current host with ws protocol
			const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
			const host = window.location.host;
			wsUrl = `${protocol}//${host}/ws/logs?env=${encodeURIComponent(env)}&service=${encodeURIComponent(serviceName)}`;
		}

		const ws = new WebSocket(wsUrl);

		ws.onopen = () => {
			console.log("Connected to log stream");
			onConnect?.();
		};

		ws.onmessage = (event) => {
			try {
				const data = JSON.parse(event.data);
				if (data.type === "logs" && data.data) {
					onMessage(data.data);
				} else if (data.error) {
					onError?.(new Error(data.error));
				}
			} catch (err) {
				console.error("Failed to parse log message:", err);
			}
		};

		ws.onerror = (event) => {
			console.error("WebSocket error:", event);
			onError?.(new Error("WebSocket connection error"));
		};

		ws.onclose = () => {
			console.log("Disconnected from log stream");
		};

		return ws;
	},

	// Get tasks for a service
	async getServiceTasks(
		env: string,
		serviceName: string,
	): Promise<ServiceTasksResponse> {
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
	async checkSSHCapability(
		env: string,
		serviceName: string,
		taskArn: string,
	): Promise<SSHCapability> {
		const params = new URLSearchParams({
			env,
			service: serviceName,
			taskArn,
		});

		const response = await fetch(
			`${API_BASE_URL}/api/ssh/capability?${params}`,
		);
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
		onClose?: () => void,
	): WebSocket {
		const params = new URLSearchParams({
			env,
			service: serviceName,
			taskArn,
		});
		if (containerName) {
			params.append("container", containerName);
		}

		// Handle both relative and absolute URLs
		let wsUrl: string;
		if (API_BASE_URL) {
			wsUrl = `${API_BASE_URL.replace(/^http/, "ws")}/ws/ssh?${params}`;
		} else {
			// If no base URL, use current host with ws protocol
			const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
			const host = window.location.host;
			wsUrl = `${protocol}//${host}/ws/ssh?${params}`;
		}

		console.log("Connecting to SSH WebSocket:", wsUrl);

		// First check if the backend is reachable
		try {
			const ws = new WebSocket(wsUrl);

			// Add a connection timeout
			const connectionTimeout = setTimeout(() => {
				if (ws.readyState !== WebSocket.OPEN) {
					ws.close();
					onError?.(
						new Error(
							"SSH connection timeout. Please check if the backend server is running.",
						),
					);
				}
			}, 5000);

			ws.onopen = () => {
				clearTimeout(connectionTimeout);
				console.log("SSH WebSocket connected");
			};

			ws.onmessage = (event) => {
				try {
					const message: SSHMessage = JSON.parse(event.data);
					onMessage(message);
				} catch (err) {
					console.error("Failed to parse SSH message:", err);
				}
			};

			ws.onerror = (event) => {
				console.error("SSH WebSocket error:", event);
				// Provide more context about the error
				let errorMessage = "SSH WebSocket connection error";
				if (ws.readyState === WebSocket.CLOSED) {
					errorMessage =
						"Unable to connect to SSH WebSocket. The backend server may not be running or the /ws/ssh endpoint is not available.";
				}
				onError?.(new Error(errorMessage));
			};

			ws.onclose = () => {
				console.log("SSH WebSocket disconnected");
				onClose?.();
			};

			// Add send method for input
			(ws as WebSocket & { sendInput?: (input: string) => void }).sendInput = (
				input: string,
			) => {
				if (ws.readyState === WebSocket.OPEN) {
					ws.send(JSON.stringify({ type: "input", data: input }));
				}
			};

			return ws;
		} catch (error) {
			console.error("Failed to create WebSocket:", error);
			onError?.(new Error("Failed to create SSH WebSocket connection"));
			throw error;
		}
	},

	// SSM Parameter Store APIs
	async getSSMParameter(name: string): Promise<SSMParameter> {
		const response = await fetch(
			`${API_BASE_URL}/api/ssm/parameter?name=${encodeURIComponent(name)}`,
		);
		if (!response.ok) {
			const error: ErrorResponse = await response.json();
			throw new Error(error.error || "Failed to fetch SSM parameter");
		}
		return response.json();
	},

	async getDatabaseInfo(
		project: string,
		env: string,
	): Promise<{
		endpoint: string;
		port: number;
		isAurora: boolean;
		status: string;
		engine: string;
		engineVersion?: string;
	}> {
		const response = await fetch(
			`${API_BASE_URL}/api/rds/info?project=${encodeURIComponent(project)}&env=${encodeURIComponent(env)}`,
		);
		if (!response.ok) {
			const error: ErrorResponse = await response.json();
			throw new Error(error.error || "Failed to fetch database info");
		}
		return response.json();
	},

	async getDatabaseEndpoint(
		project: string,
		env: string,
		isAurora: boolean,
	): Promise<{
		endpoint: string;
		port: number;
		isAurora: boolean;
		status: string;
		engine: string;
		engineVersion?: string;
	}> {
		const response = await fetch(
			`${API_BASE_URL}/api/rds/endpoint?project=${encodeURIComponent(project)}&env=${encodeURIComponent(env)}&aurora=${isAurora}`,
		);
		if (!response.ok) {
			const error: ErrorResponse = await response.json();
			throw new Error(error.error || "Failed to fetch database endpoint");
		}
		return response.json();
	},

	async createOrUpdateSSMParameter(
		params: SSMParameterCreateRequest,
	): Promise<void> {
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
		const response = await fetch(
			`${API_BASE_URL}/api/ssm/parameter?name=${encodeURIComponent(name)}`,
			{
				method: "DELETE",
			},
		);
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
		const response = await fetch(
			`${API_BASE_URL}/api/ssm/parameters?${params}`,
		);
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
		const response = await fetch(
			`${API_BASE_URL}/api/positions?environment=${encodeURIComponent(environment)}`,
		);
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
		const response = await fetch(
			`${API_BASE_URL}/api/eventbridge/send-test-event`,
			{
				method: "POST",
				headers: {
					"Content-Type": "application/json",
				},
				body: JSON.stringify(event),
			},
		);
		if (!response.ok) {
			const error: ErrorResponse = await response.json();
			throw new Error(error.error || "Failed to send test event");
		}
		return response.json();
	},

	async getEventTasks(env: string): Promise<EventTaskInfo[]> {
		const response = await fetch(
			`${API_BASE_URL}/api/eventbridge/event-tasks?env=${encodeURIComponent(env)}`,
		);
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

	async sendTestEmail(
		env: string,
		to: string,
		subject: string,
		body: string,
	): Promise<{ success: boolean; messageId?: string; error?: string }> {
		const response = await fetch(
			`${API_BASE_URL}/api/ses/send-test-email?env=${encodeURIComponent(env)}`,
			{
				method: "POST",
				headers: {
					"Content-Type": "application/json",
				},
				body: JSON.stringify({ to, subject, body }),
			},
		);
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
		const response = await fetch(
			`${API_BASE_URL}/api/ses/production-access-prefill?env=${encodeURIComponent(env)}`,
		);
		if (!response.ok) {
			const error: ErrorResponse = await response.json();
			throw new Error(
				error.error || "Failed to fetch production access prefill",
			);
		}
		return response.json();
	},

	async requestSESProductionAccess(
		env: string,
		data: {
			websiteUrl: string;
			useCaseDescription: string;
			mailingListBuildProcess: string;
			bounceComplaintProcess: string;
			additionalInfo: string;
			expectedDailyVolume: string;
			expectedPeakVolume: string;
			contactLanguage?: string;
		},
	): Promise<{
		success: boolean;
		caseId?: string;
		error?: string;
		message?: string;
	}> {
		const response = await fetch(
			`${API_BASE_URL}/api/ses/request-production?env=${encodeURIComponent(env)}`,
			{
				method: "POST",
				headers: {
					"Content-Type": "application/json",
				},
				body: JSON.stringify(data),
			},
		);
		if (!response.ok) {
			const error: ErrorResponse = await response.json();
			throw new Error(error.error || "Failed to request SES production access");
		}
		return response.json();
	},

	// S3 Buckets API
	async getS3Buckets(env: string): Promise<S3BucketInfo[]> {
		const response = await fetch(
			`${API_BASE_URL}/api/s3/buckets?env=${encodeURIComponent(env)}`,
		);
		if (!response.ok) {
			const error: ErrorResponse = await response.json();
			throw new Error(error.error || "Failed to fetch S3 buckets");
		}
		return response.json();
	},

	// GitHub OAuth Device Flow APIs
	async initiateGitHubDeviceFlow(params: {
		app_name: string;
		project: string;
		environment: string;
		scope?: string;
	}): Promise<{
		user_code: string;
		verification_uri: string;
		expires_in: number;
		interval: number;
	}> {
		const response = await fetch(`${API_BASE_URL}/api/github/oauth/device`, {
			method: "POST",
			headers: {
				"Content-Type": "application/json",
			},
			body: JSON.stringify(params),
		});
		if (!response.ok) {
			const error: ErrorResponse = await response.json();
			throw new Error(error.error || "Failed to initiate GitHub device flow");
		}
		return response.json();
	},

	async checkGitHubAuthStatus(userCode: string): Promise<{
		status: "pending" | "authorized" | "expired" | "error";
		app_name: string;
		created_at: string;
		message?: string;
		error?: string;
	}> {
		const response = await fetch(
			`${API_BASE_URL}/api/github/oauth/status?user_code=${encodeURIComponent(userCode)}`,
		);
		if (!response.ok) {
			const error: ErrorResponse = await response.json();
			throw new Error(error.error || "Failed to check GitHub auth status");
		}
		return response.json();
	},

	async deleteGitHubOAuthSession(userCode: string): Promise<void> {
		const response = await fetch(
			`${API_BASE_URL}/api/github/oauth/session?user_code=${encodeURIComponent(userCode)}`,
			{
				method: "DELETE",
			},
		);
		if (!response.ok) {
			const error: ErrorResponse = await response.json();
			throw new Error(error.error || "Failed to delete GitHub OAuth session");
		}
	},

	async getPricing(env: string): Promise<PricingResponse> {
		const response = await fetch(
			`${API_BASE_URL}/api/pricing?env=${encodeURIComponent(env)}`,
		);
		if (!response.ok) {
			const error: ErrorResponse = await response.json();
			throw new Error(error.error || "Failed to fetch pricing data");
		}
		return response.json();
	},

	// ECR Cross-Account APIs
	async getECRSources(): Promise<ECRSourcesResponse> {
		const response = await fetch(
			`${API_BASE_URL}/api/environments/ecr-sources`,
		);
		if (!response.ok) {
			const error: ErrorResponse = await response.json();
			throw new Error(error.error || "Failed to fetch ECR sources");
		}
		return response.json();
	},

	async configureCrossAccountECR(
		request: ConfigureCrossAccountECRRequest,
	): Promise<ConfigureCrossAccountECRResponse> {
		const response = await fetch(
			`${API_BASE_URL}/api/environments/configure-cross-account-ecr`,
			{
				method: "POST",
				headers: {
					"Content-Type": "application/json",
				},
				body: JSON.stringify(request),
			},
		);
		if (!response.ok) {
			const error: ErrorResponse = await response.json();
			throw new Error(error.error || "Failed to configure cross-account ECR");
		}
		return response.json();
	},

	async checkECRTrustPolicy(
		sourceEnv: string,
		targetAccount: string,
	): Promise<{
		deployed: boolean;
		has_trust_for: boolean;
		repository: string;
		target_account?: string;
		reason?: string;
	}> {
		const response = await fetch(
			`${API_BASE_URL}/api/environments/check-ecr-trust-policy`,
			{
				method: "POST",
				headers: {
					"Content-Type": "application/json",
				},
				body: JSON.stringify({
					source_env: sourceEnv,
					target_account: targetAccount,
				}),
			},
		);
		if (!response.ok) {
			const error: ErrorResponse = await response.json();
			throw new Error(error.error || "Failed to check ECR trust policy");
		}
		return response.json();
	},

	async getFargateOptions(): Promise<FargateOptionsResponse> {
		const response = await fetch(`${API_BASE_URL}/api/fargate/options`);
		if (!response.ok) {
			throw new Error("Failed to fetch Fargate options");
		}
		return response.json();
	},

	/**
	 * The region's EC2 instance catalog with on-demand prices.
	 *
	 * A rejection here means the caller's own mistake (missing env, unreadable
	 * {env}.yaml) or the server being down — never "AWS was unreachable". That
	 * case arrives as a 200 whose `source` is `fallback` or `partial`.
	 */
	async getComputeInstanceTypes(
		env: string,
		options?: { refresh?: boolean; all?: boolean },
	): Promise<ComputeInstanceTypesResponse> {
		const params = new URLSearchParams({ env });
		if (options?.refresh) params.set("refresh", "true");
		if (options?.all) params.set("all", "true");

		const response = await fetch(
			`${API_BASE_URL}/api/compute/instance-types?${params.toString()}`,
		);
		if (!response.ok) {
			const error: ErrorResponse = await response.json().catch(() => ({
				error: "Failed to fetch instance types",
			}));
			throw new Error(error.error || "Failed to fetch instance types");
		}
		return response.json();
	},

	/** Current spot prices per AZ. At most 20 types per request (AC-8). */
	async getComputeSpotPrices(
		env: string,
		types: string[],
	): Promise<ComputeSpotPricesResponse> {
		const params = new URLSearchParams({ env, types: types.join(",") });

		const response = await fetch(
			`${API_BASE_URL}/api/compute/spot-prices?${params.toString()}`,
		);
		if (!response.ok) {
			const error: ErrorResponse = await response.json().catch(() => ({
				error: "Failed to fetch spot prices",
			}));
			throw new Error(error.error || "Failed to fetch spot prices");
		}
		return response.json();
	},

	/**
	 * The sizing recommendation for a pool — existing or not yet created.
	 *
	 * `amiFamily` and `networkMode` are sent for a pool that does not exist yet
	 * so the architecture filter and the density rule match what the pool will
	 * actually render with (DEV-23, DEV-24).
	 */
	async getComputeRecommendation(
		query: ComputeRecommendationQuery,
	): Promise<ComputeRecommendationResponse> {
		const params = new URLSearchParams({ env: query.env });
		if (query.posture) params.set("posture", query.posture);
		if (query.pool) params.set("pool", query.pool);
		if (query.services?.length)
			params.set("services", query.services.join(","));
		if (query.window !== undefined) params.set("window", String(query.window));
		if (query.limit !== undefined) params.set("limit", String(query.limit));
		if (query.gpu) params.set("gpu", "true");
		if (query.amiFamily) params.set("ami_family", query.amiFamily);
		if (query.networkMode) params.set("network_mode", query.networkMode);

		const response = await fetch(
			`${API_BASE_URL}/api/compute/recommendation?${params.toString()}`,
		);
		if (!response.ok) {
			const error: ErrorResponse = await response.json().catch(() => ({
				error: "Failed to fetch compute recommendation",
			}));
			throw new Error(error.error || "Failed to fetch compute recommendation");
		}
		return response.json();
	},

	async getAPIGatewayInfo(env: string): Promise<{
		defaultEndpoint: string;
		apiGatewayId: string;
		customDomainEnabled: boolean;
		customDomain?: string;
		region: string;
		error?: string;
	}> {
		const response = await fetch(
			`${API_BASE_URL}/api/apigateway/info?env=${encodeURIComponent(env)}`,
		);
		if (!response.ok) {
			const error: ErrorResponse = await response.json();
			throw new Error(error.error || "Failed to fetch API Gateway info");
		}
		return response.json();
	},
};
