import type { YamlInfrastructureConfig } from "../types/yamlConfig";

export interface NodeStateConfig {
	id: string;
	name: string;
	type: string;
	enabled: (config: YamlInfrastructureConfig) => boolean;
	properties?: (config: YamlInfrastructureConfig) => Record<string, any>;
	description?: string;
}

/**
 * Maps node IDs to their state configuration based on YAML settings
 * Based on ARCHITECTURE_NODE_MAPPING.md
 */
export const nodeStateMapping: NodeStateConfig[] = [
	// External Entry Points
	{
		id: "client-app",
		name: "Client app",
		type: "client-app",
		enabled: () => true, // Always enabled (external)
		description: "External client applications (web, mobile)",
	},
	{
		id: "github",
		name: "GitHub actions",
		type: "github",
		enabled: (config) => config.workload?.enable_github_oidc === true,
		properties: (config) => ({
			subjects: config.workload?.github_oidc_subjects || [],
		}),
	},

	// Authentication Layer
	{
		id: "auth-system",
		name: "Authentication system",
		type: "auth",
		enabled: (config) => config.cognito?.enabled === true,
		properties: (config) => ({
			userPoolDomain: config.cognito?.user_pool_domain_prefix,
			webClientEnabled: config.cognito?.enable_web_client,
			autoVerifiedAttributes: config.cognito?.auto_verified_attributes || [],
		}),
	},

	// API Gateway Layer
	{
		id: "api-gateway",
		name: "Amazon API Gateway",
		type: "api-gateway",
		enabled: () => true, // Always enabled (default ingress)
		description: "HTTP API with VPC Links",
	},
	{
		id: "amplify",
		name: "AWS Amplify",
		type: "amplify",
		enabled: () => false, // Not implemented
		description: "Frontend distribution (not implemented)",
	},

	// Load Balancing Layer
	{
		id: "route53",
		name: "Amazon Route 53",
		type: "route53",
		enabled: (config) => config.domain?.enabled === true,
		properties: (config) => ({
			domainName: config.domain?.domain_name,
			createZone: config.domain?.create_domain_zone,
			apiDomainPrefix: config.domain?.api_domain_prefix,
			addEnvPrefix: config.domain?.add_domain_prefix,
		}),
	},
	{
		id: "waf",
		name: "AWS WAF",
		type: "waf",
		enabled: () => false, // Not implemented
		description: "Web Application Firewall (not implemented)",
	},

	// Container Orchestration
	{
		id: "ecs-cluster",
		name: "Amazon ECS Cluster",
		type: "ecs",
		enabled: () => true, // Always enabled (core component)
		properties: (config) => ({
			clusterName: `${config.project}_cluster_${config.env}`,
			launchType: "Fargate",
			containerInsights: true,
		}),
	},

	// ECS Services
	{
		id: "backend-service",
		name: "Backend service",
		type: "backend",
		enabled: () => true, // Always enabled (required)
		properties: (config) => ({
			serviceName: `${config.project}_backend_${config.env}`,
			port: config.workload?.backend_image_port || 8080,
			healthEndpoint: config.workload?.backend_health_endpoint || "/health",
			cpu: config.workload?.backend_cpu || '256',
			memory: config.workload?.backend_memory || '512',
			envVariables: config.workload?.backend_env_variables || [],
			desiredCount: config.workload?.backend_desired_count || 1,
			autoscalingEnabled: config.workload?.backend_autoscaling_enabled || false,
			autoscalingMinCapacity: config.workload?.backend_autoscaling_min_capacity || 1,
			autoscalingMaxCapacity: config.workload?.backend_autoscaling_max_capacity || 10,
		}),
	},

	// Container Registry
	{
		id: "ecr",
		name: "Amazon ECR",
		type: "ecr",
		enabled: () => true, // Always enabled
		properties: (config) => ({
			repository: `${config.project}_backend`,
			crossAccount: config.ecr_account_id && config.ecr_account_region,
			ecrAccountId: config.ecr_account_id,
			ecrRegion: config.ecr_account_region,
		}),
	},
	{
		id: "aurora",
		name: "PostgreSQL Database",
		type: "postgres",
		enabled: (config) => config.postgres?.enabled === true,
		properties: (config) => ({
			dbname: config.postgres?.dbname || config.project,
			username: config.postgres?.username || 'postgres',
			publicAccess: config.postgres?.public_access || false,
			engineVersion: config.postgres?.engine_version || '14',
			pgAdminEnabled: config.workload?.install_pg_admin || false,
			pgAdminEmail: config.workload?.pg_admin_email || 'admin@madappgang.com',
		}),
		description: "AWS RDS Aurora PostgreSQL Serverless v2",
	},

	// Storage Layer
	{
		id: "s3",
		name: "Amazon S3",
		type: "s3",
		enabled: () => true, // Always enabled (backend bucket required)
		properties: (config) => ({
			backendBucket: `${config.project}-backend-${config.env}-${config.workload?.bucket_postfix}`,
			backendBucketPublic: config.workload?.bucket_public,
			additionalBuckets: config.buckets || [],
		}),
	},
	{
		id: "eventbridge",
		name: "Amazon EventBridge",
		type: "eventbridge",
		enabled: () => true, // Always enabled for deployments
		description: "ECR image push events and custom event bus",
	},
	{
		id: "sns",
		name: "Amazon SNS",
		type: "sns",
		enabled: (config) => config.workload?.setup_fcnsns === true,
		description: "Firebase Cloud Messaging/SNS for push notifications",
	},
	{
		id: "sqs",
		name: "Amazon SQS",
		type: "sqs",
		enabled: (config) => config.sqs?.enabled === true,
		properties: (config) => ({
			queueName: config.sqs?.name || 'default-queue',
			queueUrl: `https://sqs.${config.region}.amazonaws.com/${config.region}/${config.project}-${config.env}-${config.sqs?.name || 'default-queue'}`,
		}),
		description: "Simple Queue Service for async task processing",
	},
	{
		id: "ses",
		name: "Amazon SES",
		type: "ses",
		enabled: (config) => config.ses?.enabled === true,
		properties: (config) => ({
			domainName:
				config.ses?.domain_name || `mail.${config.domain?.domain_name}`,
			testEmails: config.ses?.test_emails || [],
		}),
	},

	// Monitoring & Observability
	{
		id: "cloudwatch",
		name: "Amazon CloudWatch",
		type: "cloudwatch",
		enabled: () => true, // Always enabled
		description: "Logs, metrics, and monitoring",
	},
	{
		id: "xray",
		name: "AWS X-Ray",
		type: "xray",
		enabled: (config) => config.workload?.xray_enabled === true,
		description: "Distributed tracing and service map",
	},
	{
		id: "secrets-manager",
		name: "Parameter Store",
		type: "secrets-manager",
		enabled: () => true, // Always enabled for parameter storage
		description: "AWS Systems Manager Parameter Store",
	},
	{
		id: "alarms",
		name: "Alarm rules",
		type: "alarms",
		enabled: () => false, // Not implemented
		description: "CloudWatch alarms (not implemented)",
	},
];

/**
 * Get the enabled state of a node based on configuration
 */
export function getNodeState(
	nodeId: string,
	config: YamlInfrastructureConfig | null,
): boolean {
	if (!config) return true; // Show all nodes if no config loaded

	const nodeConfig = nodeStateMapping.find((n) => n.id === nodeId);
	if (!nodeConfig) return true; // Unknown nodes default to enabled

	return nodeConfig.enabled(config);
}

/**
 * Get node properties based on configuration
 */
export function getNodeProperties(
	nodeId: string,
	config: YamlInfrastructureConfig | null,
): Record<string, any> {
	if (!config) return {};

	const nodeConfig = nodeStateMapping.find((n) => n.id === nodeId);
	if (!nodeConfig || !nodeConfig.properties) return {};

	return nodeConfig.properties(config);
}

/**
 * Check if additional services exist in configuration
 */
export function hasAdditionalServices(
	config: YamlInfrastructureConfig | null,
): boolean {
	if (!config) return false;
	return (
		(config.services && config.services.length > 0) ||
		(config.scheduled_tasks && config.scheduled_tasks.length > 0) ||
		(config.event_processor_tasks && config.event_processor_tasks.length > 0)
	);
}

/**
 * Get additional services from configuration
 */
export function getAdditionalServices(config: YamlInfrastructureConfig | null) {
	if (!config) return { services: [], scheduledTasks: [], eventTasks: [] };

	return {
		services: config.services || [],
		scheduledTasks: config.scheduled_tasks || [],
		eventTasks: config.event_processor_tasks || [],
	};
}
