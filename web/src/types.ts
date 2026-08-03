import type { PricingResponse } from "./hooks/use-pricing";

export interface ComponentNode {
	id: string;
	type:
		| "frontend"
		| "backend"
		| "database"
		| "cache"
		| "api"
		| "analytics"
		| "infrastructure"
		| "container-registry"
		| "route53"
		| "waf"
		| "api-gateway"
		| "ecs"
		| "ecr"
		| "aurora"
		| "eventbridge"
		| "secrets-manager"
		| "ses"
		| "sns"
		| "s3"
		| "amplify"
		| "xray"
		| "cloudwatch"
		| "telemetry"
		| "alarms"
		| "github"
		| "auth"
		| "client-app"
		| "admin-app"
		| "opa"
		| "service-regular"
		| "service-periodic"
		| "service-event-driven"
		| "service"
		| "scheduled-task"
		| "event-task"
		| "postgres"
		| "sqs"
		| "efs"
		| "alb"
		| "appsync"
		| "custom-terraform"
		| "cloudfront";
	name: string;
	url?: string;
	status: "running" | "deploying" | "stopped" | "error" | "external";
	description?: string;
	isExternal?: boolean;
	deploymentType?: string;
	replicas?: number;
	resources?: {
		cpu: string;
		memory: string;
	};
	environment?: Record<string, string>;
	logs?: LogEntry[];
	metrics?: {
		cpu: number;
		memory: number;
		requests: number;
	};
	deletable?: boolean;
	group?: string;
	subgroup?: string;
	hasTelemetry?: boolean;
	disabled?: boolean;
	configProperties?: NodeConfigValues;
	pricing?: PricingResponse | null;
}

/**
 * Node-specific configuration surfaced on the canvas and fed into pricing
 * estimates. Every field is optional because which ones are present depends on
 * the node type: a scheduled task has a schedule, a database has an instance
 * class, and so on.
 */
export interface NodeConfigValues {
	// Compute sizing. cpu/memory arrive as strings from YAML but as numbers from
	// some producers, so both are accepted and narrowed at the use site.
	cpu?: string | number;
	memory?: string | number;
	desiredCount?: number;
	autoscalingEnabled?: boolean;
	autoscalingMinCapacity?: number;
	autoscalingMaxCapacity?: number;

	// Database
	aurora?: boolean;
	instanceClass?: string;
	allocatedStorage?: number;
	multiAz?: boolean;
	minCapacity?: number;
	maxCapacity?: number;

	// Scheduling and event routing
	schedule?: string;
	sources?: string[];
	detailTypes?: string[];

	// Deployment metadata
	domain?: string;
	customDomain?: string;
	branch?: string;
	branches?: string[];
	repository?: string;
	environment?: string;
	profile?: string;
	healthStatus?: {
		critical: boolean;
		monitored: boolean;
	};
}

export interface LogEntry {
	timestamp: string;
	level: "info" | "warning" | "error";
	message: string;
}

export interface Connection {
	id: string;
	source: string;
	target: string;
	type: string;
}
