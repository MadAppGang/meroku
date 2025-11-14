import type { NodeProperties } from "../types/components";
import type { YamlInfrastructureConfig } from "../types/yamlConfig";

export interface NodeStateConfig {
	id: string;
	name: string;
	type: string;
	enabled: (config: YamlInfrastructureConfig) => boolean;
	properties?: (config: YamlInfrastructureConfig) => NodeProperties;
	description?: string | ((config: YamlInfrastructureConfig) => string);
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

	// API Gateway Layer
	{
		id: "api-gateway",
		name: "Amazon API Gateway",
		type: "api-gateway",
		enabled: (config) => !config.alb?.enabled, // Enabled when ALB is NOT enabled
		description: "HTTP API with VPC Links",
	},
	{
		id: "alb",
		name: "Application Load Balancer",
		type: "alb",
		enabled: (config) => config.alb?.enabled === true, // Enabled when ALB is enabled
		properties: (config) => ({
			domainName: config.workload?.backend_alb_domain_name || "",
		}),
		description: "Application Load Balancer for HTTP/HTTPS routing",
	},

	// Load Balancing Layer
	{
		id: "route53",
		name: "Amazon Route 53",
		type: "route53",
		enabled: (config) => config.domain?.enabled === true,
		properties: (config) => ({
			domainName: config.domain?.domain_name || "",
			createZone: config.domain?.create_domain_zone || false,
			apiDomainPrefix: config.domain?.api_domain_prefix || "",
			addEnvPrefix: config.domain?.add_env_domain_prefix ?? true,
		}),
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
		properties: (config) => {
			// Calculate the actual Route53 domain if custom domain is enabled
			let domain = "";
			if (config.domain?.enabled && config.domain?.api_domain_prefix) {
				const baseDomain = config.domain.domain_name || "";
				const apiPrefix = config.domain.api_domain_prefix || "api";
				const addEnvPrefix = config.domain.add_env_domain_prefix ?? true;
				const envPrefix = addEnvPrefix ? `${config.env}.` : "";
				domain = baseDomain ? `${apiPrefix}.${envPrefix}${baseDomain}` : "";
			}

			return {
				serviceName: `${config.project}_backend_${config.env}`,
				port: config.workload?.backend_image_port || 8080,
				healthEndpoint: config.workload?.backend_health_endpoint || "/health",
				cpu: config.workload?.backend_cpu || "256",
				memory: config.workload?.backend_memory || "512",
				envVariables: config.workload?.backend_env_variables || {},
				desiredCount: config.workload?.backend_desired_count || 1,
				autoscalingEnabled: config.workload?.backend_autoscaling_enabled || false,
				autoscalingMinCapacity:
					config.workload?.backend_autoscaling_min_capacity || 1,
				autoscalingMaxCapacity:
					config.workload?.backend_autoscaling_max_capacity || 10,
				domain: domain,
			};
		},
	},

	// Container Registry
	{
		id: "ecr",
		name: "Amazon ECR",
		type: "ecr",
		enabled: () => true, // Always enabled
		properties: (config) => ({
			repository: `${config.project}_backend`,
			crossAccount:
				config.ecr_account_id && config.ecr_account_region ? "true" : "false",
			ecrAccountId: config.ecr_account_id || "",
			ecrRegion: config.ecr_account_region || "",
		}),
	},
	{
		id: "aurora",
		name: "PostgreSQL Database",
		type: "postgres",
		enabled: (config) => config.postgres?.enabled === true,
		properties: (config) => ({
			dbname: config.postgres?.dbname || config.project,
			username: config.postgres?.username || "postgres",
			publicAccess: config.postgres?.public_access || false,
			engineVersion: config.postgres?.engine_version || "16",
			aurora: config.postgres?.aurora || false,
			minCapacity: config.postgres?.min_capacity ?? 0,
			maxCapacity: config.postgres?.max_capacity || 1,
			// RDS-specific properties
			instanceClass: config.postgres?.instance_class || "db.t4g.micro",
			allocatedStorage: config.postgres?.allocated_storage || 20,
			storageType: config.postgres?.storage_type || "gp3",
			multiAz: config.postgres?.multi_az || false,
			storageEncrypted: config.postgres?.storage_encrypted ?? true,
			deletionProtection: config.postgres?.deletion_protection || false,
			skipFinalSnapshot: config.postgres?.skip_final_snapshot ?? true,
			pgAdminEnabled: config.workload?.install_pg_admin || false,
			pgAdminEmail: config.workload?.pg_admin_email || "admin@madappgang.com",
		}),
		description: (config) =>
			config.postgres?.aurora
				? "AWS Aurora PostgreSQL Serverless v2"
				: "AWS RDS PostgreSQL Instance",
	},

	// Storage Layer
	{
		id: "s3",
		name: "Amazon S3",
		type: "s3",
		enabled: () => true, // Always enabled (backend bucket required)
		properties: (config) => ({
			backendBucket: `${config.project}-backend-${config.env}-${config.workload?.bucket_postfix}`,
			backendBucketPublic: config.workload?.bucket_public || false,
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
		properties: (config) => ({
			platformApplicationName: `${config.project}-fcm-${config.env}`,
			platform: "GCM",
			gcmServerKeyPath: `/${config.env}/${config.project}/backend/gcm-server-key`,
		}),
		description: "Firebase Cloud Messaging/SNS for push notifications",
	},
	{
		id: "sqs",
		name: "Amazon SQS",
		type: "sqs",
		enabled: (config) => config.sqs?.enabled === true,
		properties: (config) => ({
			queueName: config.sqs?.name || "default-queue",
			queueUrl: `https://sqs.${config.region}.amazonaws.com/${config.region}/${config.project}-${config.env}-${config.sqs?.name || "default-queue"}`,
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
 * Dynamic node state mappings for services that are created from config
 */
export function getDynamicNodeStateMapping(
	config: YamlInfrastructureConfig,
): NodeStateConfig[] {
	const dynamicMappings: NodeStateConfig[] = [];

	// Add mappings for additional services
	if (config.services) {
		config.services.forEach((service) => {
			dynamicMappings.push({
				id: `service-${service.name}`,
				name: service.name,
				type: "service",
				enabled: () => true,
				properties: () => ({
					desiredCount: service.desired_count || 1,
					cpu: service.cpu || 256,
					memory: service.memory || 512,
					containerPort: service.container_port || 3000,
					xrayEnabled: service.xray_enabled || false,
					remoteAccess: service.remote_access || false,
				}),
				description: `Additional service: ${service.name}`,
			});
		});
	}

	// Add mappings for event processor tasks
	if (config.event_processor_tasks) {
		config.event_processor_tasks.forEach((task) => {
			dynamicMappings.push({
				id: `event-${task.name}`,
				name: `Event: ${task.name}`,
				type: "event-task",
				enabled: () => true,
				properties: () => ({
					ruleName: task.rule_name,
					detailTypes: task.detail_types,
					sources: task.sources,
					publicAccess: true, // Always enabled for internet access
				}),
				description: `Event processor: ${task.rule_name}`,
			});
		});
	}

	// Add mappings for scheduled tasks
	if (config.scheduled_tasks) {
		config.scheduled_tasks.forEach((task) => {
			dynamicMappings.push({
				id: `scheduled-${task.name}`,
				name: `Scheduled: ${task.name}`,
				type: "scheduled-task",
				enabled: () => true,
				properties: () => ({
					taskCount: 1, // Scheduled tasks always run single instances
					schedule: task.schedule,
					publicAccess: true, // Always enabled for internet access
				}),
				description: `Scheduled task: ${task.schedule}`,
			});
		});
	}

	return dynamicMappings;
}

/**
 * Get the enabled state of a node based on configuration
 */
export function getNodeState(
	nodeId: string,
	config: YamlInfrastructureConfig | null,
): boolean {
	if (!config) return true; // Show all nodes if no config loaded

	// Check static mappings first
	const nodeConfig = nodeStateMapping.find((n) => n.id === nodeId);
	if (nodeConfig) {
		return nodeConfig.enabled(config);
	}

	// Check dynamic mappings for services and tasks
	const dynamicMappings = getDynamicNodeStateMapping(config);
	const dynamicNodeConfig = dynamicMappings.find((n) => n.id === nodeId);
	if (dynamicNodeConfig) {
		return dynamicNodeConfig.enabled(config);
	}

	return true; // Unknown nodes default to enabled
}

/**
 * Get node description based on configuration
 */
export function getNodeDescription(
	nodeId: string,
	config: YamlInfrastructureConfig | null,
): string | undefined {
	if (!config) return undefined;

	const nodeConfig = nodeStateMapping.find((n) => n.id === nodeId);
	if (nodeConfig?.description) {
		return typeof nodeConfig.description === "function"
			? nodeConfig.description(config)
			: nodeConfig.description;
	}

	return undefined;
}

/**
 * Get node properties based on configuration
 */
export function getNodeProperties(
	nodeId: string,
	config: YamlInfrastructureConfig | null,
): NodeProperties {
	if (!config) return {};

	// Check static mappings first
	const nodeConfig = nodeStateMapping.find((n) => n.id === nodeId);
	if (nodeConfig?.properties) {
		return nodeConfig.properties(config);
	}

	// Check dynamic mappings for services and tasks
	const dynamicMappings = getDynamicNodeStateMapping(config);
	const dynamicNodeConfig = dynamicMappings.find((n) => n.id === nodeId);
	if (dynamicNodeConfig?.properties) {
		return dynamicNodeConfig.properties(config);
	}

	return {};
}

/**
 * Check if additional services exist in configuration
 */
export function hasAdditionalServices(
	config: YamlInfrastructureConfig | null,
): boolean {
	if (!config) return false;
	return !!(
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
