/**
 * properties-v3 — declarative node catalog (definitions only).
 *
 * Eleven panels: Backend, Service, API Gateway, unified PostgreSQL/Aurora,
 * S3, CloudFront, Route 53, ECR, Group, ALB, Dynamic Group.
 *
 * REPRESENTATIVE coverage only: each panel declares a meaningful subset of
 * the settings documented in ai_docs/WEB_APP_NODE_PROPERTIES.md with their
 * documented property paths and mutability classes. No claim is made that
 * all 413 documented settings are covered.
 *
 * Definitions contain metadata only — no concrete runtime values. Fixtures
 * (fixtures.ts) hydrate instances separately.
 */

import type {
	AffectedResource,
	PropertyFieldDefinition,
	PropertyPanelDefinition,
} from "./types";

const auroraOn = (
	fieldId: string,
): { all: { kind: "equals"; key: string; value: unknown }[] } => ({
	all: [{ kind: "equals", key: fieldId, value: true }],
});
const auroraOff = (
	fieldId: string,
): { all: { kind: "notEquals"; key: string; value: unknown }[] } => ({
	all: [{ kind: "notEquals", key: fieldId, value: true }],
});

/* ---------------------------------------------------------------------------
 * Shared synthetic affected-resource identities (names match CLAUDE.md
 * naming conventions; all values are fixture-only).
 * ------------------------------------------------------------------------- */

const backendBucket: AffectedResource = {
	id: "s3-backend-bucket",
	name: "myapp-backend-dev (S3 bucket)",
};
const postgresDb: AffectedResource = {
	id: "rds-postgres",
	name: "myapp_postgres_dev (PostgreSQL)",
};
const s3BucketResource: AffectedResource = {
	id: "s3-bucket-assets",
	name: "myapp-assets (S3 bucket)",
};
const cloudfrontResource: AffectedResource = {
	id: "cloudfront-distribution",
	name: "myapp-distribution (CloudFront)",
};
const route53Zone: AffectedResource = {
	id: "route53-zone",
	name: "example.com (hosted zone)",
};
const ecrRepository: AffectedResource = {
	id: "ecr-repository",
	name: "myapp_backend (ECR repository)",
};
const serviceEcs: AffectedResource = {
	id: "ecs-service-api",
	name: "myapp_service_api_dev (ECS service)",
};
const serviceEcr: AffectedResource = {
	id: "ecr-repository-api",
	name: "myapp_service_api (ECR repository)",
};

/* ---------------------------------------------------------------------------
 * Backend
 * ------------------------------------------------------------------------- */

const backendFields: readonly PropertyFieldDefinition[] = [
	{
		id: "backend-ecs-service-name",
		property: "clusterName",
		label: "ECS service",
		mutability: "RO",
		valueKind: "aws-assigned",
		controlType: "readonly-value",
		section: "identity",
	},
	{
		id: "backend-task-role-arn",
		property: "Task role ARN",
		label: "Task role ARN",
		mutability: "RO",
		valueKind: "aws-assigned",
		controlType: "readonly-value",
		section: "identity",
	},
	{
		id: "backend-execution-role-arn",
		property: "Execution role ARN",
		label: "Execution role ARN",
		mutability: "RO",
		valueKind: "aws-assigned",
		controlType: "readonly-value",
		section: "identity",
	},
	{
		id: "backend-docker-image",
		property: "workload.backend_external_docker_image",
		label: "External docker image",
		mutability: "TF-DEPLOY",
		valueKind: "editable",
		controlType: "text",
		section: "source",
		placeholder: "registry.example.com/myapp:sha-…",
		description:
			"Optional external image; overrides the project ECR repository.",
	},
	{
		id: "backend-container-command",
		property: "workload.backend_container_command",
		label: "Container command",
		mutability: "TF-DEPLOY",
		valueKind: "editable",
		controlType: "textarea",
		valueType: "string-array",
		section: "source",
	},
	{
		id: "backend-cpu",
		property: "workload.backend_cpu",
		label: "CPU",
		mutability: "TF-DEPLOY",
		valueKind: "editable",
		controlType: "select",
		section: "compute-scaling",
		description: 'Backend CPU is a STRING in YAML (e.g. "256").',
		options: [
			{ label: "256 (0.25 vCPU)", value: "256" },
			{ label: "512 (0.5 vCPU)", value: "512" },
			{ label: "1024 (1 vCPU)", value: "1024" },
			{ label: "2048 (2 vCPU)", value: "2048" },
		],
	},
	{
		id: "backend-memory",
		property: "workload.backend_memory",
		label: "Memory",
		mutability: "TF-DEPLOY",
		valueKind: "editable",
		controlType: "select",
		section: "compute-scaling",
		description: "Memory is a STRING in YAML and must pair with CPU.",
		options: [
			{ label: "512 MB", value: "512" },
			{ label: "1 GB", value: "1024" },
			{ label: "2 GB", value: "2048" },
			{ label: "4 GB", value: "4096" },
		],
	},
	{
		id: "backend-autoscaling-enabled",
		property: "workload.backend_autoscaling_enabled",
		label: "Autoscaling",
		mutability: "TF",
		valueKind: "editable",
		controlType: "boolean",
		section: "compute-scaling",
	},
	{
		id: "backend-desired-count",
		property: "workload.backend_desired_count",
		label: "Desired count",
		mutability: "TF",
		valueKind: "editable",
		controlType: "number",
		section: "compute-scaling",
		visibleWhen: auroraOff("backend-autoscaling-enabled"),
		validation: { min: 0, max: 10 },
	},
	{
		id: "backend-autoscaling-min",
		property: "workload.backend_autoscaling_min_capacity",
		label: "Min capacity",
		mutability: "TF",
		valueKind: "editable",
		controlType: "number",
		section: "compute-scaling",
		visibleWhen: auroraOn("backend-autoscaling-enabled"),
		validation: { min: 1 },
	},
	{
		id: "backend-autoscaling-max",
		property: "workload.backend_autoscaling_max_capacity",
		label: "Max capacity",
		mutability: "TF",
		valueKind: "editable",
		controlType: "number",
		section: "compute-scaling",
		visibleWhen: auroraOn("backend-autoscaling-enabled"),
		validation: { min: 1 },
	},
	{
		id: "backend-alb-domain-name",
		property: "workload.backend_alb_domain_name",
		label: "ALB domain name",
		mutability: "TF",
		valueKind: "editable",
		controlType: "text",
		section: "network-routing",
	},
	{
		id: "backend-image-port",
		property: "workload.backend_image_port",
		label: "Image port",
		mutability: "TF-DEPLOY",
		valueKind: "editable",
		controlType: "number",
		section: "network-routing",
	},
	{
		id: "backend-health-endpoint",
		property: "workload.backend_health_endpoint",
		label: "Health endpoint",
		mutability: "TF-DEPLOY",
		valueKind: "editable",
		controlType: "text",
		section: "network-routing",
	},
	{
		id: "backend-remote-access",
		property: "workload.backend_remote_access",
		label: "Remote access (SSH)",
		mutability: "TF-DEPLOY",
		valueKind: "editable",
		controlType: "boolean",
		section: "security-access",
	},
	{
		id: "backend-xray-enabled",
		property: "workload.xray_enabled",
		label: "X-Ray tracing",
		mutability: "TF-DEPLOY",
		valueKind: "editable",
		controlType: "boolean",
		section: "observability",
	},
	{
		id: "backend-env-variables",
		property: "workload.backend_env_variables",
		label: "Environment variables (plaintext)",
		mutability: "TF-DEPLOY",
		valueKind: "editable",
		controlType: "key-value-table",
		// Whole-field type is a string→string map (structured), not a scalar.
		valueType: "json",
		section: "environment-secrets",
		description:
			"Plaintext task env vars are embedded in the ECS task definition: EVERY change (including value edits) is a rolling deployment.",
		validation: {
			// Map values must be strings; keys are always strings by JS shape.
			// Absent/null and raw-string drafts pass here (valueType:json covers
			// JSON-parseability of raw editor text).
			validate: (value) =>
				value === null || value === undefined || typeof value !== "object"
					? null
					: Object.values(value).every((entry) => typeof entry === "string")
						? null
						: "Environment variables must map keys to text values",
		},
		collection: {
			recordKeys: true,
			diffRules: { forceImpact: "rolling-restart" },
		},
	},
	{
		id: "backend-ssm-parameters",
		property: "workload.ssm_parameters",
		label: "SSM parameters",
		mutability: "LIVE-RESTART",
		valueKind: "editable",
		controlType: "key-value-table",
		valueType: "json",
		section: "environment-secrets",
		description:
			"Value edits are live but need a container restart (LIVE-RESTART); adding/removing a parameter changes the task definition (rolling deployment).",
		collection: {
			recordKeys: true,
			// Explicit column scope: parameter type is create-only metadata, so
			// only Value is editable. Hidden keys (e.g. `type`) are preserved on
			// value/key edits via the row round-trip — never edited or dropped.
			columns: [{ key: "value", valueType: "string" }],
			diffRules: {
				valueProperties: ["value"],
				valueEditImpact: "restart-to-observe",
				valueEditChannel: "live",
				membershipImpact: "rolling-restart",
			},
		},
	},
	{
		id: "backend-bucket-postfix",
		property: "workload.bucket_postfix",
		label: "Bucket postfix",
		mutability: "TF-REPLACE",
		valueKind: "editable",
		controlType: "text",
		section: "storage-data",
		description:
			"Changing the postfix renames (replaces) the backend S3 bucket.",
		affectedResources: [backendBucket],
	},
	{
		id: "backend-bucket-public",
		property: "workload.bucket_public",
		label: "Bucket public",
		mutability: "TF",
		valueKind: "editable",
		controlType: "boolean",
		section: "storage-data",
	},
	{
		id: "backend-bucket-arn",
		property: "Bucket ARN",
		label: "Bucket ARN",
		mutability: "RO",
		valueKind: "aws-assigned",
		controlType: "readonly-value",
		section: "storage-data",
	},
	{
		id: "backend-api-domain-prefix",
		property: "domain.api_domain_prefix",
		label: "API domain prefix",
		mutability: "TF",
		valueKind: "editable",
		controlType: "text",
		section: "network-routing",
	},
	{
		id: "backend-log-group-name",
		property: "Log group name",
		label: "CloudWatch log group",
		mutability: "RO",
		valueKind: "aws-assigned",
		controlType: "readonly-value",
		section: "observability",
		logRole: "destination",
	},
	{
		id: "backend-log-retention",
		property: "Log retention",
		label: "Log retention (days)",
		mutability: "TF",
		valueKind: "editable",
		controlType: "number",
		section: "observability",
		logRole: "retention",
	},
	{
		id: "backend-log-stream",
		property: "Log stream contents",
		label: "Log events",
		mutability: "RUNTIME",
		valueKind: "telemetry",
		controlType: "telemetry-row",
		section: "observability",
		logRole: "viewer",
	},
	{
		id: "backend-running-count",
		property: "runningCount",
		label: "Running tasks",
		mutability: "RUNTIME",
		valueKind: "telemetry",
		controlType: "telemetry-row",
		section: "health-metrics",
	},
	{
		id: "backend-cpu-utilization",
		property: "currentCPUUtilization",
		label: "CPU utilization",
		mutability: "RUNTIME",
		valueKind: "telemetry",
		controlType: "telemetry-row",
		section: "health-metrics",
	},
	{
		id: "backend-memory-utilization",
		property: "currentMemoryUtilization",
		label: "Memory utilization",
		mutability: "RUNTIME",
		valueKind: "telemetry",
		controlType: "telemetry-row",
		section: "health-metrics",
	},
	{
		id: "backend-deployed-image-tag",
		property: "Deployed image tag",
		label: "Deployed image tag",
		mutability: "RUNTIME",
		valueKind: "telemetry",
		controlType: "telemetry-row",
		section: "deployments",
	},
];

const backend: PropertyPanelDefinition = {
	id: "backend",
	title: "Backend",
	sections: [
		{
			key: "identity",
			archetype: "readonly-identity",
			fieldIds: [
				"backend-ecs-service-name",
				"backend-task-role-arn",
				"backend-execution-role-arn",
			],
		},
		{
			key: "source",
			archetype: "container-process",
			fieldIds: ["backend-docker-image", "backend-container-command"],
		},
		{
			key: "compute-scaling",
			archetype: "autoscaling-group",
			fieldIds: [
				"backend-cpu",
				"backend-memory",
				"backend-autoscaling-enabled",
				"backend-desired-count",
				"backend-autoscaling-min",
				"backend-autoscaling-max",
			],
		},
		{
			key: "network-routing",
			archetype: "container-process",
			fieldIds: [
				"backend-image-port",
				"backend-health-endpoint",
				"backend-alb-domain-name",
				"backend-api-domain-prefix",
			],
		},
		{
			key: "security-access",
			archetype: "standard",
			fieldIds: ["backend-remote-access"],
		},
		{
			key: "environment-secrets",
			archetype: "environment-variables",
			fieldIds: ["backend-env-variables", "backend-ssm-parameters"],
		},
		{
			key: "storage-data",
			archetype: "standard",
			fieldIds: [
				"backend-bucket-postfix",
				"backend-bucket-public",
				"backend-bucket-arn",
			],
		},
		{
			key: "observability",
			archetype: "standard",
			fieldIds: [
				"backend-xray-enabled",
				"backend-log-group-name",
				"backend-log-retention",
				"backend-log-stream",
			],
		},
		{
			key: "deployments",
			archetype: "telemetry-metrics",
			fieldIds: ["backend-deployed-image-tag"],
		},
		{
			key: "health-metrics",
			archetype: "telemetry-metrics",
			fieldIds: [
				"backend-running-count",
				"backend-cpu-utilization",
				"backend-memory-utilization",
			],
		},
	],
	fields: backendFields,
};

/* ---------------------------------------------------------------------------
 * Service (named ECS service)
 * ------------------------------------------------------------------------- */

const serviceFields: readonly PropertyFieldDefinition[] = [
	{
		id: "service-name",
		property: "services[].name",
		label: "Name",
		mutability: "CREATE / TF-REPLACE",
		valueKind: "editable",
		controlType: "text",
		section: "naming-lifecycle",
		description:
			"Editable before creation; after creation a rename recreates the exact named ECS service, ECR repository and log group.",
		validation: { required: true, minLength: 2 },
		affectedResources: [serviceEcs, serviceEcr],
	},
	{
		id: "service-enabled",
		property: "services[].enabled",
		label: "Enabled",
		mutability: "TF-REPLACE",
		valueKind: "editable",
		controlType: "boolean",
		section: "naming-lifecycle",
		description:
			"Disabling removes the named ECS service, its ECR repository and log group.",
		affectedResources: [serviceEcs, serviceEcr],
	},
	{
		id: "service-docker-image",
		property: "services[].docker_image",
		label: "Docker image",
		mutability: "TF-DEPLOY",
		valueKind: "editable",
		controlType: "text",
		section: "source",
	},
	{
		id: "service-container-command",
		property: "services[].container_command",
		label: "Container command",
		mutability: "TF-DEPLOY",
		valueKind: "editable",
		controlType: "textarea",
		valueType: "string-array",
		section: "source",
	},
	{
		id: "service-ecr-mode",
		property: "services[].ecr_config.mode",
		label: "Registry mode",
		mutability: "TF-REPLACE",
		valueKind: "editable",
		controlType: "select",
		section: "source",
		description:
			"Registry strategy selector: reveals the matching repository controls.",
		options: [
			{ label: "Create ECR repository", value: "create_ecr" },
			{ label: "Manual repository URI", value: "manual_repo" },
			{ label: "Use existing service image", value: "use_existing" },
		],
		affectedResources: [serviceEcr],
	},
	{
		id: "service-ecr-repository-uri",
		property: "services[].ecr_config.repository_uri",
		label: "Repository URI",
		mutability: "TF-DEPLOY",
		valueKind: "editable",
		controlType: "text",
		section: "source",
		visibleWhen: {
			all: [{ kind: "equals", key: "service-ecr-mode", value: "manual_repo" }],
		},
	},
	{
		id: "service-ecr-source-name",
		property: "services[].ecr_config.source_service_name",
		label: "Source service name",
		mutability: "TF-DEPLOY",
		valueKind: "editable",
		controlType: "text",
		section: "source",
		visibleWhen: {
			all: [{ kind: "equals", key: "service-ecr-mode", value: "use_existing" }],
		},
	},
	{
		id: "service-ecr-source-type",
		property: "services[].ecr_config.source_service_type",
		label: "Source service type",
		mutability: "TF-DEPLOY",
		valueKind: "editable",
		controlType: "select",
		section: "source",
		visibleWhen: {
			all: [{ kind: "equals", key: "service-ecr-mode", value: "use_existing" }],
		},
		options: [
			{ label: "services", value: "services" },
			{ label: "event_processor_tasks", value: "event_processor_tasks" },
			{ label: "scheduled_tasks", value: "scheduled_tasks" },
		],
	},
	{
		id: "service-cpu",
		property: "services[].cpu",
		label: "CPU",
		mutability: "TF-DEPLOY",
		valueKind: "editable",
		controlType: "number",
		valueType: "number",
		section: "compute-scaling",
		description:
			"Service CPU is a NUMBER in YAML (unlike backend's string form).",
	},
	{
		id: "service-memory",
		property: "services[].memory",
		label: "Memory",
		mutability: "TF-DEPLOY",
		valueKind: "editable",
		controlType: "number",
		section: "compute-scaling",
	},
	{
		id: "service-desired-count",
		property: "services[].desired_count",
		label: "Desired count",
		mutability: "TF",
		valueKind: "editable",
		controlType: "number",
		section: "compute-scaling",
	},
	{
		id: "service-container-port",
		property: "services[].container_port",
		label: "Container port",
		mutability: "TF-DEPLOY",
		valueKind: "editable",
		controlType: "number",
		section: "network-routing",
	},
	{
		id: "service-host-port",
		property: "services[].host_port",
		label: "Host port",
		mutability: "TF-DEPLOY",
		valueKind: "editable",
		controlType: "number",
		section: "advanced",
		description: "Rare setting with a safe default.",
	},
	{
		id: "service-public-access",
		property: "services[].public_access",
		label: "Public access",
		mutability: "TF",
		valueKind: "editable",
		controlType: "boolean",
		section: "network-routing",
	},
	{
		id: "service-health-check-path",
		property: "services[].health_check_path",
		label: "Health check path",
		mutability: "TF",
		valueKind: "editable",
		controlType: "text",
		section: "network-routing",
		visibleWhen: {
			all: [{ kind: "equals", key: "service-public-access", value: true }],
		},
	},
	{
		id: "service-environment-variables",
		property: ["services[].environment_variables", "services[].env_vars"],
		label: "Environment variables (plaintext)",
		mutability: "TF-DEPLOY",
		valueKind: "editable",
		controlType: "key-value-table",
		// Whole-field type is a string→string map (structured), not a scalar.
		valueType: "json",
		section: "environment-secrets",
		description: "Plaintext env vars: all operations are rolling deployments.",
		validation: {
			// Map values must be strings; keys are always strings by JS shape.
			// Absent/null and raw-string drafts pass here (valueType:json covers
			// JSON-parseability of raw editor text).
			validate: (value) =>
				value === null || value === undefined || typeof value !== "object"
					? null
					: Object.values(value).every((entry) => typeof entry === "string")
						? null
						: "Environment variables must map keys to text values",
		},
		collection: {
			recordKeys: true,
			diffRules: { forceImpact: "rolling-restart" },
		},
	},
	{
		id: "service-ssm-parameters",
		property: "services[].ssm_parameters",
		label: "SSM parameters",
		mutability: "LIVE-RESTART",
		valueKind: "editable",
		controlType: "key-value-table",
		valueType: "json",
		section: "environment-secrets",
		collection: {
			recordKeys: true,
			// Explicit column scope: parameter type is create-only metadata, so
			// only Value is editable. Hidden keys (e.g. `type`) are preserved on
			// value/key edits via the row round-trip — never edited or dropped.
			columns: [{ key: "value", valueType: "string" }],
			diffRules: {
				valueProperties: ["value"],
				valueEditImpact: "restart-to-observe",
				valueEditChannel: "live",
				membershipImpact: "rolling-restart",
			},
		},
	},
	{
		id: "service-xray-enabled",
		property: "services[].xray_enabled",
		label: "X-Ray tracing",
		mutability: "TF-DEPLOY",
		valueKind: "editable",
		controlType: "boolean",
		section: "observability",
	},
	{
		id: "service-ecs-name",
		property: "ECS service name",
		label: "ECS service name",
		mutability: "RO",
		valueKind: "aws-assigned",
		controlType: "readonly-value",
		section: "identity",
	},
	{
		id: "service-task-def-arn",
		property: "Task definition ARN",
		label: "Task definition ARN",
		mutability: "RO",
		valueKind: "aws-assigned",
		controlType: "readonly-value",
		section: "identity",
	},
	{
		id: "service-running-count",
		property: "runningCount",
		label: "Running tasks",
		mutability: "RUNTIME",
		valueKind: "telemetry",
		controlType: "telemetry-row",
		section: "health-metrics",
	},
	{
		id: "service-status",
		property: "status",
		label: "Service status",
		mutability: "RUNTIME",
		valueKind: "telemetry",
		controlType: "telemetry-row",
		section: "health-metrics",
	},
];

const service: PropertyPanelDefinition = {
	id: "service",
	title: "Service",
	sections: [
		{
			key: "identity",
			archetype: "readonly-identity",
			fieldIds: ["service-ecs-name", "service-task-def-arn"],
		},
		{
			key: "naming-lifecycle",
			archetype: "standard",
			fieldIds: ["service-name", "service-enabled"],
		},
		{
			key: "source",
			archetype: "registry-strategy",
			fieldIds: [
				"service-ecr-mode",
				"service-docker-image",
				"service-container-command",
				"service-ecr-repository-uri",
				"service-ecr-source-name",
				"service-ecr-source-type",
			],
		},
		{
			key: "compute-scaling",
			archetype: "standard",
			fieldIds: ["service-cpu", "service-memory", "service-desired-count"],
		},
		{
			key: "network-routing",
			archetype: "container-process",
			fieldIds: [
				"service-container-port",
				"service-public-access",
				"service-health-check-path",
			],
		},
		{
			key: "environment-secrets",
			archetype: "environment-variables",
			fieldIds: ["service-environment-variables", "service-ssm-parameters"],
		},
		{
			key: "observability",
			archetype: "standard",
			fieldIds: ["service-xray-enabled"],
		},
		{ key: "advanced", archetype: "standard", fieldIds: ["service-host-port"] },
		{
			key: "health-metrics",
			archetype: "telemetry-metrics",
			fieldIds: ["service-running-count", "service-status"],
		},
	],
	fields: serviceFields,
};

/* ---------------------------------------------------------------------------
 * API Gateway
 * ------------------------------------------------------------------------- */

const apiGatewayFields: readonly PropertyFieldDefinition[] = [
	{
		id: "apigw-api-id",
		property: "API ID",
		label: "API ID",
		mutability: "RO",
		valueKind: "aws-assigned",
		controlType: "readonly-value",
		section: "identity",
	},
	{
		id: "apigw-stage",
		property: "Stage",
		label: "Stage",
		mutability: "RO",
		valueKind: "aws-assigned",
		controlType: "readonly-value",
		section: "identity",
	},
	{
		id: "apigw-api-type",
		property: "API type",
		label: "API type",
		mutability: "RO",
		valueKind: "aws-assigned",
		controlType: "readonly-value",
		section: "identity",
	},
	{
		id: "apigw-endpoint",
		property: "AWS endpoint",
		label: "Invoke URL",
		mutability: "RO",
		valueKind: "aws-assigned",
		controlType: "readonly-value",
		section: "endpoints",
	},
	{
		id: "apigw-routes",
		property: "Routes",
		label: "Routes",
		mutability: "TF",
		valueKind: "derived",
		controlType: "readonly-value",
		section: "relationships",
		description:
			"Derived from public services — read-only relationship overview.",
	},
	{
		id: "apigw-domain-prefix",
		property: "domain.api_domain_prefix",
		label: "API domain prefix",
		mutability: "TF",
		valueKind: "editable",
		controlType: "text",
		section: "network-routing",
	},
	{
		id: "apigw-rate-limit",
		property: "Rate limit",
		label: "Rate limit (req/s)",
		mutability: "TF",
		valueKind: "editable",
		controlType: "number",
		section: "network-routing",
		validation: { min: 1 },
	},
	{
		id: "apigw-burst-limit",
		property: "Burst limit",
		label: "Burst limit",
		mutability: "TF",
		valueKind: "editable",
		controlType: "number",
		section: "network-routing",
		validation: { min: 1 },
	},
	{
		id: "apigw-log-retention",
		property: "Log retention",
		label: "Log retention (days)",
		mutability: "TF",
		valueKind: "editable",
		controlType: "number",
		section: "observability",
		logRole: "retention",
	},
	{
		id: "apigw-log-group",
		property: "API access log group",
		label: "Access log group",
		mutability: "RO",
		valueKind: "aws-assigned",
		controlType: "readonly-value",
		section: "observability",
		logRole: "destination",
	},
];

const apiGateway: PropertyPanelDefinition = {
	id: "apiGateway",
	title: "API Gateway",
	sections: [
		{
			key: "identity",
			archetype: "readonly-identity",
			fieldIds: ["apigw-api-id", "apigw-stage", "apigw-api-type"],
		},
		{
			key: "endpoints",
			archetype: "readonly-identity",
			fieldIds: ["apigw-endpoint"],
		},
		{
			key: "relationships",
			archetype: "readonly-identity",
			fieldIds: ["apigw-routes"],
		},
		{
			key: "network-routing",
			archetype: "standard",
			fieldIds: [
				"apigw-domain-prefix",
				"apigw-rate-limit",
				"apigw-burst-limit",
			],
		},
		{
			key: "observability",
			archetype: "standard",
			fieldIds: ["apigw-log-retention", "apigw-log-group"],
		},
	],
	fields: apiGatewayFields,
};

/* ---------------------------------------------------------------------------
 * PostgreSQL / Aurora (unified)
 * ------------------------------------------------------------------------- */

const postgresFields: readonly PropertyFieldDefinition[] = [
	{
		id: "postgres-enabled",
		property: "postgres.enabled",
		label: "PostgreSQL enabled",
		mutability: "TF-REPLACE",
		valueKind: "editable",
		controlType: "boolean",
		section: "naming-lifecycle",
		affectedResources: [postgresDb],
	},
	{
		id: "postgres-aurora",
		property: "postgres.aurora",
		label: "Use Aurora Serverless v2",
		mutability: "TF-REPLACE",
		valueKind: "editable",
		controlType: "boolean",
		section: "storage-data",
		description:
			"Destructive engine switch: toggling replaces the database. Drives RDS-only vs Aurora-only field visibility.",
		affectedResources: [postgresDb],
	},
	{
		id: "postgres-dbname",
		property: "postgres.dbname",
		label: "Database name",
		mutability: "TF-REPLACE",
		valueKind: "editable",
		controlType: "text",
		section: "storage-data",
		validation: { required: true },
		affectedResources: [postgresDb],
	},
	{
		id: "postgres-username",
		property: "postgres.username",
		label: "Master username",
		mutability: "TF-REPLACE",
		valueKind: "editable",
		controlType: "text",
		section: "security-access",
		validation: { required: true },
		affectedResources: [postgresDb],
	},
	{
		id: "postgres-engine-version",
		property: "postgres.engine_version",
		label: "Engine version",
		mutability: "TF",
		valueKind: "editable",
		controlType: "text",
		section: "storage-data",
	},
	{
		id: "postgres-public-access",
		property: "postgres.public_access",
		label: "Public access",
		mutability: "TF",
		valueKind: "editable",
		controlType: "boolean",
		section: "network-routing",
	},
	{
		id: "postgres-instance-class",
		property: "postgres.instance_class",
		label: "Instance class",
		mutability: "TF",
		valueKind: "editable",
		controlType: "select",
		section: "compute-scaling",
		visibleWhen: auroraOff("postgres-aurora"),
		description: "RDS only.",
		// Same classes the shipped PostgresNodeProperties offers.
		options: [
			{ label: "db.t4g.micro", value: "db.t4g.micro" },
			{ label: "db.t4g.small", value: "db.t4g.small" },
			{ label: "db.t4g.medium", value: "db.t4g.medium" },
			{ label: "db.t4g.large", value: "db.t4g.large" },
			{ label: "db.m6i.large", value: "db.m6i.large" },
			{ label: "db.m6i.xlarge", value: "db.m6i.xlarge" },
			{ label: "db.m6i.2xlarge", value: "db.m6i.2xlarge" },
			{ label: "db.r6i.large", value: "db.r6i.large" },
			{ label: "db.r6i.xlarge", value: "db.r6i.xlarge" },
		],
	},
	{
		id: "postgres-allocated-storage",
		property: "postgres.allocated_storage",
		label: "Allocated storage (GB)",
		mutability: "TF",
		valueKind: "editable",
		controlType: "number",
		section: "storage-data",
		visibleWhen: auroraOff("postgres-aurora"),
		validation: { min: 20, max: 65536 },
	},
	{
		id: "postgres-multi-az",
		property: "postgres.multi_az",
		label: "Multi-AZ",
		mutability: "TF",
		valueKind: "editable",
		controlType: "boolean",
		section: "storage-data",
		visibleWhen: auroraOff("postgres-aurora"),
	},
	{
		id: "postgres-storage-encrypted",
		property: "postgres.storage_encrypted",
		label: "Storage encrypted",
		mutability: "TF-REPLACE",
		valueKind: "editable",
		controlType: "boolean",
		section: "security-access",
		visibleWhen: auroraOff("postgres-aurora"),
		description: "RDS only; changing encryption replaces the instance.",
		affectedResources: [postgresDb],
	},
	{
		id: "postgres-min-capacity",
		property: "postgres.min_capacity",
		label: "Min capacity (ACU)",
		mutability: "TF",
		valueKind: "editable",
		controlType: "number",
		section: "compute-scaling",
		visibleWhen: auroraOn("postgres-aurora"),
		description:
			"Aurora Serverless v2 only. 0 is a VALID value (pause when idle).",
		validation: { min: 0, max: 128 },
	},
	{
		id: "postgres-max-capacity",
		property: "postgres.max_capacity",
		label: "Max capacity (ACU)",
		mutability: "TF",
		valueKind: "editable",
		controlType: "number",
		section: "compute-scaling",
		visibleWhen: auroraOn("postgres-aurora"),
		validation: { min: 0.5, max: 128 },
	},
	{
		id: "postgres-deletion-protection",
		property: "postgres.deletion_protection",
		label: "Deletion protection",
		mutability: "TF",
		valueKind: "editable",
		controlType: "boolean",
		section: "naming-lifecycle",
	},
	{
		id: "postgres-skip-final-snapshot",
		property: "postgres.skip_final_snapshot",
		label: "Skip final snapshot",
		mutability: "TF",
		valueKind: "editable",
		controlType: "boolean",
		section: "advanced",
	},
	{
		id: "postgres-iam-db-auth",
		property: "postgres.iam_database_authentication_enabled",
		label: "IAM database authentication",
		mutability: "TF",
		valueKind: "editable",
		controlType: "boolean",
		section: "security-access",
	},
	{
		id: "postgres-endpoint",
		property: "endpoint",
		label: "Writer endpoint",
		mutability: "RO",
		valueKind: "aws-assigned",
		controlType: "readonly-value",
		section: "endpoints",
	},
	{
		id: "postgres-port",
		property: "port",
		label: "Port",
		mutability: "RO",
		valueKind: "aws-assigned",
		controlType: "readonly-value",
		section: "endpoints",
	},
	{
		id: "postgres-engine",
		property: "engine",
		label: "Engine",
		mutability: "RO",
		valueKind: "aws-assigned",
		controlType: "readonly-value",
		section: "identity",
	},
	{
		id: "postgres-engine-version-runtime",
		property: "engineVersion",
		label: "Deployed engine version",
		mutability: "RO",
		valueKind: "aws-assigned",
		controlType: "readonly-value",
		section: "identity",
	},
	{
		id: "postgres-status",
		property: "status",
		label: "Status",
		mutability: "RUNTIME",
		valueKind: "telemetry",
		controlType: "telemetry-row",
		section: "health-metrics",
	},
];

const postgres: PropertyPanelDefinition = {
	id: "postgres",
	title: "PostgreSQL",
	sections: [
		{
			key: "identity",
			archetype: "readonly-identity",
			fieldIds: ["postgres-engine", "postgres-engine-version-runtime"],
		},
		{
			key: "endpoints",
			archetype: "readonly-identity",
			fieldIds: ["postgres-endpoint", "postgres-port"],
		},
		{
			key: "naming-lifecycle",
			archetype: "standard",
			fieldIds: ["postgres-enabled", "postgres-deletion-protection"],
		},
		{
			key: "compute-scaling",
			archetype: "standard",
			fieldIds: [
				"postgres-instance-class",
				"postgres-min-capacity",
				"postgres-max-capacity",
			],
		},
		{
			key: "storage-data",
			archetype: "standard",
			fieldIds: [
				"postgres-aurora",
				"postgres-dbname",
				"postgres-engine-version",
				"postgres-allocated-storage",
				"postgres-multi-az",
			],
		},
		{
			key: "network-routing",
			archetype: "standard",
			fieldIds: ["postgres-public-access"],
		},
		{
			key: "security-access",
			archetype: "standard",
			fieldIds: [
				"postgres-username",
				"postgres-storage-encrypted",
				"postgres-iam-db-auth",
			],
		},
		{
			key: "advanced",
			archetype: "standard",
			fieldIds: ["postgres-skip-final-snapshot"],
		},
		{
			key: "health-metrics",
			archetype: "telemetry-metrics",
			fieldIds: ["postgres-status"],
		},
	],
	fields: postgresFields,
};

/* ---------------------------------------------------------------------------
 * S3
 * ------------------------------------------------------------------------- */

const s3Fields: readonly PropertyFieldDefinition[] = [
	{
		id: "s3-bucket-name",
		property: "buckets[].name",
		label: "Bucket name",
		mutability: "TF-REPLACE",
		valueKind: "editable",
		controlType: "text",
		section: "naming-lifecycle",
		validation: { required: true },
		affectedResources: [s3BucketResource],
	},
	{
		id: "s3-bucket-public",
		property: "buckets[].public",
		label: "Public",
		mutability: "TF",
		valueKind: "editable",
		controlType: "boolean",
		section: "security-access",
	},
	{
		id: "s3-bucket-versioning",
		property: "buckets[].versioning",
		label: "Versioning",
		mutability: "TF",
		valueKind: "editable",
		controlType: "boolean",
		section: "storage-data",
	},
	{
		id: "s3-cors-rules",
		property: "buckets[].cors_rules",
		label: "CORS rules",
		mutability: "TF",
		valueKind: "editable",
		controlType: "key-value-table",
		section: "network-routing",
		collection: {
			identityKeys: ["id"],
			columns: [
				{ key: "id", valueType: "string", required: true },
				{ key: "allowed_origins", valueType: "string-array" },
				{ key: "allowed_methods", valueType: "string-array" },
				{ key: "allowed_headers", valueType: "string-array" },
				{ key: "max_age_seconds", valueType: "number" },
			],
		},
	},
	{
		id: "s3-bucket-arn",
		property: "Bucket ARN",
		label: "Bucket ARN",
		mutability: "RO",
		valueKind: "aws-assigned",
		controlType: "readonly-value",
		section: "identity",
	},
	{
		id: "s3-regional-domain",
		property: "Regional domain name",
		label: "Regional domain name",
		mutability: "RO",
		valueKind: "aws-assigned",
		controlType: "readonly-value",
		section: "endpoints",
	},
	{
		id: "s3-console-url",
		property: "consoleUrl",
		label: "AWS console",
		mutability: "RO",
		valueKind: "aws-assigned",
		controlType: "readonly-value",
		section: "endpoints",
	},
	{
		id: "s3-creation-date",
		property: "creationDate",
		label: "Created",
		mutability: "RO",
		valueKind: "aws-assigned",
		controlType: "readonly-value",
		section: "identity",
	},
	{
		id: "s3-object-listing",
		property: "Object listing",
		label: "Objects",
		mutability: "RUNTIME",
		valueKind: "telemetry",
		controlType: "telemetry-row",
		section: "health-metrics",
	},
];

const s3: PropertyPanelDefinition = {
	id: "s3",
	title: "S3",
	sections: [
		{
			key: "identity",
			archetype: "readonly-identity",
			fieldIds: ["s3-bucket-arn", "s3-creation-date"],
		},
		{
			key: "endpoints",
			archetype: "readonly-identity",
			fieldIds: ["s3-regional-domain", "s3-console-url"],
		},
		{
			key: "naming-lifecycle",
			archetype: "standard",
			fieldIds: ["s3-bucket-name"],
		},
		{
			key: "storage-data",
			archetype: "standard",
			fieldIds: ["s3-bucket-versioning"],
		},
		{
			key: "network-routing",
			archetype: "standard",
			fieldIds: ["s3-cors-rules"],
		},
		{
			key: "security-access",
			archetype: "standard",
			fieldIds: ["s3-bucket-public"],
		},
		{
			key: "health-metrics",
			archetype: "telemetry-metrics",
			fieldIds: ["s3-object-listing"],
		},
	],
	fields: s3Fields,
};

/* ---------------------------------------------------------------------------
 * CloudFront
 * ------------------------------------------------------------------------- */

const cloudFrontFields: readonly PropertyFieldDefinition[] = [
	{
		id: "cloudfront-name",
		property: "name",
		label: "Distribution name",
		mutability: "TF-REPLACE",
		valueKind: "editable",
		controlType: "text",
		section: "naming-lifecycle",
		validation: { required: true },
		affectedResources: [cloudfrontResource],
	},
	{
		id: "cloudfront-enabled",
		property: "enabled",
		label: "Enabled",
		mutability: "TF",
		valueKind: "editable",
		controlType: "boolean",
		section: "naming-lifecycle",
	},
	{
		id: "cloudfront-price-class",
		property: "price_class",
		label: "Price class",
		mutability: "TF",
		valueKind: "editable",
		controlType: "select",
		section: "advanced",
		options: [
			{ label: "PriceClass_100", value: "PriceClass_100" },
			{ label: "PriceClass_200", value: "PriceClass_200" },
			{ label: "PriceClass_All", value: "PriceClass_All" },
		],
	},
	{
		id: "cloudfront-default-root-object",
		property: "default_root_object",
		label: "Default root object",
		mutability: "TF",
		valueKind: "editable",
		controlType: "text",
		section: "advanced",
	},
	{
		id: "cloudfront-spa-mode",
		property: "spa_mode",
		label: "SPA mode",
		mutability: "TF",
		valueKind: "editable",
		controlType: "boolean",
		section: "network-routing",
	},
	{
		id: "cloudfront-domain-aliases",
		property: "domain_aliases",
		label: "Domain aliases",
		mutability: "TF",
		valueKind: "editable",
		controlType: "textarea",
		valueType: "string-array",
		section: "network-routing",
		description: "Requires a matching certificate in us-east-1.",
	},
	{
		id: "cloudfront-origins",
		property: "origins",
		label: "Origins",
		mutability: "TF",
		valueKind: "editable",
		controlType: "key-value-table",
		section: "network-routing",
		collection: {
			identityKeys: ["name"],
			columns: [
				{ key: "name", valueType: "string", required: true },
				{ key: "type", valueType: "string" },
				{ key: "bucket_name", valueType: "string" },
				{ key: "create_bucket", valueType: "boolean" },
				{ key: "use_oac", valueType: "boolean" },
			],
		},
	},
	{
		id: "cloudfront-cache-behaviors",
		property: "cache_behaviors",
		label: "Cache behaviors",
		mutability: "TF",
		valueKind: "editable",
		controlType: "key-value-table",
		section: "network-routing",
		collection: { identityKeys: ["path_pattern"] },
	},
	{
		id: "cloudfront-logging-enabled",
		property: "logging.enabled",
		label: "Access logging",
		mutability: "TF",
		valueKind: "editable",
		controlType: "boolean",
		section: "observability",
		description:
			"S3 access logging configuration — does NOT create a CloudWatch Logs viewer.",
	},
	{
		id: "cloudfront-logging-bucket",
		property: "logging.bucket_name",
		label: "Logging bucket",
		mutability: "TF",
		valueKind: "editable",
		controlType: "text",
		section: "observability",
		visibleWhen: {
			all: [{ kind: "equals", key: "cloudfront-logging-enabled", value: true }],
		},
	},
	{
		id: "cloudfront-distribution-id",
		property: "Distribution ID",
		label: "Distribution ID",
		mutability: "RO",
		valueKind: "aws-assigned",
		controlType: "readonly-value",
		section: "identity",
	},
	{
		id: "cloudfront-domain-name",
		property: "Distribution domain name",
		label: "Domain name",
		mutability: "RO",
		valueKind: "aws-assigned",
		controlType: "readonly-value",
		section: "endpoints",
	},
	{
		id: "cloudfront-deployment-status",
		property: "Deployment status",
		label: "Deployment status",
		mutability: "RUNTIME",
		valueKind: "telemetry",
		controlType: "telemetry-row",
		section: "health-metrics",
	},
];

const cloudFront: PropertyPanelDefinition = {
	id: "cloudfront",
	title: "CloudFront",
	sections: [
		{
			key: "identity",
			archetype: "readonly-identity",
			fieldIds: ["cloudfront-distribution-id"],
		},
		{
			key: "endpoints",
			archetype: "readonly-identity",
			fieldIds: ["cloudfront-domain-name"],
		},
		{
			key: "naming-lifecycle",
			archetype: "standard",
			fieldIds: ["cloudfront-name", "cloudfront-enabled"],
		},
		{
			key: "network-routing",
			archetype: "standard",
			fieldIds: [
				"cloudfront-origins",
				"cloudfront-cache-behaviors",
				"cloudfront-spa-mode",
				"cloudfront-domain-aliases",
			],
		},
		{
			key: "observability",
			archetype: "standard",
			fieldIds: ["cloudfront-logging-enabled", "cloudfront-logging-bucket"],
		},
		{
			key: "advanced",
			archetype: "standard",
			fieldIds: ["cloudfront-price-class", "cloudfront-default-root-object"],
		},
		{
			key: "health-metrics",
			archetype: "telemetry-metrics",
			fieldIds: ["cloudfront-deployment-status"],
		},
	],
	fields: cloudFrontFields,
};

/* ---------------------------------------------------------------------------
 * Route 53
 * ------------------------------------------------------------------------- */

const route53Fields: readonly PropertyFieldDefinition[] = [
	{
		id: "route53-domain-enabled",
		property: "domain.enabled",
		label: "DNS enabled",
		mutability: "TF",
		valueKind: "editable",
		controlType: "boolean",
		section: "naming-lifecycle",
	},
	{
		id: "route53-domain-name",
		property: "domain.domain_name",
		label: "Domain name",
		mutability: "TF-REPLACE",
		valueKind: "editable",
		controlType: "text",
		section: "naming-lifecycle",
		validation: { required: true },
		affectedResources: [route53Zone],
	},
	{
		id: "route53-create-zone",
		property: "domain.create_domain_zone",
		label: "Create zone",
		mutability: "TF-REPLACE",
		valueKind: "editable",
		controlType: "boolean",
		section: "naming-lifecycle",
		affectedResources: [route53Zone],
	},
	{
		id: "route53-zone-id",
		property: "domain.zone_id",
		label: "Zone ID",
		mutability: "TF",
		valueKind: "editable",
		controlType: "text",
		section: "naming-lifecycle",
		visibleWhen: {
			all: [{ kind: "equals", key: "route53-create-zone", value: false }],
		},
	},
	{
		id: "route53-api-domain-prefix",
		property: "domain.api_domain_prefix",
		label: "API domain prefix",
		mutability: "TF",
		valueKind: "editable",
		controlType: "text",
		section: "network-routing",
	},
	{
		id: "route53-add-env-prefix",
		property: "domain.add_env_domain_prefix",
		label: "Add environment prefix",
		mutability: "TF",
		valueKind: "editable",
		controlType: "boolean",
		section: "network-routing",
	},
	{
		id: "route53-is-dns-root",
		property: "domain.is_dns_root",
		label: "DNS root",
		mutability: "CREATE",
		valueKind: "editable",
		controlType: "boolean",
		section: "naming-lifecycle",
		description: "Set once at creation; locked read-only afterwards.",
	},
	{
		id: "route53-additional-domains",
		property: "domain.additional_domains",
		label: "Additional domains",
		mutability: "TF-REPLACE",
		valueKind: "editable",
		controlType: "key-value-table",
		section: "network-routing",
		collection: { identityKeys: ["domain"] },
		affectedResources: [route53Zone],
	},
	{
		id: "route53-ns-records",
		property: "Zone NS records",
		label: "Zone NS records",
		mutability: "RO",
		valueKind: "aws-assigned",
		controlType: "readonly-value",
		section: "identity",
	},
	{
		id: "route53-delegation-role-arn",
		property: "domain.delegation_role_arn",
		label: "Delegation role ARN",
		mutability: "RO",
		valueKind: "aws-assigned",
		controlType: "readonly-value",
		section: "identity",
	},
	{
		id: "route53-record-sets",
		property: "Record set list",
		label: "Record sets",
		mutability: "RUNTIME",
		valueKind: "telemetry",
		controlType: "telemetry-row",
		section: "health-metrics",
	},
	{
		id: "route53-propagation-status",
		property: "Delegation propagation status",
		label: "Delegation propagation",
		mutability: "RUNTIME",
		valueKind: "telemetry",
		controlType: "telemetry-row",
		section: "health-metrics",
	},
];

const route53: PropertyPanelDefinition = {
	id: "route53",
	title: "Route 53",
	sections: [
		{
			key: "identity",
			archetype: "readonly-identity",
			fieldIds: ["route53-ns-records", "route53-delegation-role-arn"],
		},
		{
			key: "naming-lifecycle",
			archetype: "standard",
			fieldIds: [
				"route53-domain-enabled",
				"route53-domain-name",
				"route53-create-zone",
				"route53-zone-id",
				"route53-is-dns-root",
			],
		},
		{
			key: "network-routing",
			archetype: "standard",
			fieldIds: [
				"route53-api-domain-prefix",
				"route53-add-env-prefix",
				"route53-additional-domains",
			],
		},
		{
			key: "health-metrics",
			archetype: "telemetry-metrics",
			fieldIds: ["route53-record-sets", "route53-propagation-status"],
		},
	],
	fields: route53Fields,
};

/* ---------------------------------------------------------------------------
 * ECR
 * ------------------------------------------------------------------------- */

const ecrFields: readonly PropertyFieldDefinition[] = [
	{
		id: "ecr-strategy",
		property: "ecr_strategy",
		label: "Registry strategy",
		mutability: "TF-REPLACE",
		valueKind: "editable",
		controlType: "select",
		section: "source",
		description:
			"Local repositories vs cross-account pull. Switching replaces the registry wiring.",
		options: [
			{ label: "Local", value: "local" },
			{ label: "Cross-account", value: "cross_account" },
		],
		affectedResources: [ecrRepository],
	},
	{
		id: "ecr-account-id",
		property: "ecr_account_id",
		label: "Source account ID",
		mutability: "TF-DEPLOY",
		valueKind: "editable",
		controlType: "text",
		valueType: "string",
		section: "source",
		visibleWhen: {
			all: [{ kind: "equals", key: "ecr-strategy", value: "cross_account" }],
		},
		validation: { required: true, pattern: "^\\d{12}$" },
	},
	{
		id: "ecr-account-region",
		property: "ecr_account_region",
		label: "Source region",
		mutability: "TF-DEPLOY",
		valueKind: "editable",
		controlType: "text",
		valueType: "string",
		section: "source",
		visibleWhen: {
			all: [{ kind: "equals", key: "ecr-strategy", value: "cross_account" }],
		},
		validation: { required: true },
	},
	{
		id: "ecr-trusted-accounts",
		property: "ecr_trusted_accounts",
		label: "Trusted accounts",
		mutability: "TF",
		valueKind: "editable",
		controlType: "key-value-table",
		section: "security-access",
		visibleWhen: {
			all: [{ kind: "equals", key: "ecr-strategy", value: "local" }],
		},
		collection: {
			identityKeys: ["account_id"],
			columns: [
				{ key: "account_id", valueType: "string", required: true },
				{ key: "env", valueType: "string" },
				{ key: "region", valueType: "string" },
			],
		},
	},
	{
		id: "ecr-repository-uri",
		property: "Repository URI",
		label: "Repository URI",
		mutability: "RO",
		valueKind: "aws-assigned",
		controlType: "readonly-value",
		section: "endpoints",
	},
	{
		id: "ecr-repository-arn",
		property: "Repository ARN",
		label: "Repository ARN",
		mutability: "RO",
		valueKind: "aws-assigned",
		controlType: "readonly-value",
		section: "identity",
	},
	{
		id: "ecr-image-tags",
		property: "Image tag list",
		label: "Image tags",
		mutability: "RUNTIME",
		valueKind: "telemetry",
		controlType: "telemetry-row",
		section: "health-metrics",
	},
	{
		id: "ecr-image-pushed-at",
		property: "Image pushed-at",
		label: "Last push",
		mutability: "RUNTIME",
		valueKind: "telemetry",
		controlType: "telemetry-row",
		section: "health-metrics",
	},
];

const ecr: PropertyPanelDefinition = {
	id: "ecr",
	title: "ECR",
	sections: [
		{
			key: "identity",
			archetype: "readonly-identity",
			fieldIds: ["ecr-repository-arn"],
		},
		{
			key: "endpoints",
			archetype: "readonly-identity",
			fieldIds: ["ecr-repository-uri"],
		},
		{
			key: "source",
			archetype: "registry-strategy",
			fieldIds: ["ecr-strategy", "ecr-account-id", "ecr-account-region"],
		},
		{
			key: "security-access",
			archetype: "standard",
			fieldIds: ["ecr-trusted-accounts"],
		},
		{
			key: "health-metrics",
			archetype: "telemetry-metrics",
			fieldIds: ["ecr-image-tags", "ecr-image-pushed-at"],
		},
	],
	fields: ecrFields,
};

/* ---------------------------------------------------------------------------
 * ALB — proves derived-routing precedence (routes → Overview/Relationships)
 * ------------------------------------------------------------------------- */

const albFields: readonly PropertyFieldDefinition[] = [
	{
		id: "alb-enabled",
		property: "alb.enabled",
		label: "ALB enabled",
		mutability: "TF",
		valueKind: "editable",
		controlType: "boolean",
		section: "naming-lifecycle",
	},
	{
		id: "alb-domain-name",
		property: "workload.backend_alb_domain_name",
		label: "Domain name",
		mutability: "TF",
		valueKind: "editable",
		controlType: "text",
		section: "network-routing",
	},
	{
		id: "alb-health-endpoint",
		property: "workload.backend_health_endpoint",
		label: "Health endpoint",
		mutability: "TF",
		valueKind: "editable",
		controlType: "text",
		section: "network-routing",
	},
	{
		id: "alb-image-port",
		property: "workload.backend_image_port",
		label: "Image port",
		mutability: "TF-DEPLOY",
		valueKind: "editable",
		controlType: "number",
		section: "network-routing",
	},
	{
		id: "alb-routing-rules",
		property: "Routing rules",
		label: "Routing rules",
		mutability: "TF",
		valueKind: "derived",
		controlType: "readonly-value",
		section: "relationships",
		description:
			"Derived from public services — read-only relationship overview; `derived` outranks editability.",
	},
	{
		id: "alb-dns-name",
		property: "ALB DNS name",
		label: "DNS name",
		mutability: "RO",
		valueKind: "aws-assigned",
		controlType: "readonly-value",
		section: "endpoints",
	},
	{
		id: "alb-arn",
		property: "ALB ARN",
		label: "ALB ARN",
		mutability: "RO",
		valueKind: "aws-assigned",
		controlType: "readonly-value",
		section: "identity",
	},
	{
		id: "alb-target-group-arns",
		property: "Target group ARNs",
		label: "Target groups",
		mutability: "RO",
		valueKind: "aws-assigned",
		controlType: "readonly-value",
		section: "relationships",
	},
	{
		id: "alb-target-health",
		property: "Target health",
		label: "Target health",
		mutability: "RUNTIME",
		valueKind: "telemetry",
		controlType: "telemetry-row",
		section: "health-metrics",
	},
	{
		id: "alb-certificate-arn",
		property: "Listener certificate ARN",
		label: "Listener certificate",
		mutability: "RO",
		valueKind: "aws-assigned",
		controlType: "readonly-value",
		section: "security-access",
	},
];

const alb: PropertyPanelDefinition = {
	id: "alb",
	title: "Application Load Balancer",
	sections: [
		{ key: "identity", archetype: "readonly-identity", fieldIds: ["alb-arn"] },
		{
			key: "endpoints",
			archetype: "readonly-identity",
			fieldIds: ["alb-dns-name"],
		},
		{
			key: "relationships",
			archetype: "readonly-identity",
			fieldIds: ["alb-routing-rules", "alb-target-group-arns"],
		},
		{
			key: "naming-lifecycle",
			archetype: "standard",
			fieldIds: ["alb-enabled"],
		},
		{
			key: "network-routing",
			archetype: "standard",
			fieldIds: ["alb-domain-name", "alb-health-endpoint", "alb-image-port"],
		},
		{
			key: "security-access",
			archetype: "standard",
			fieldIds: ["alb-certificate-arn"],
		},
		{
			key: "health-metrics",
			archetype: "telemetry-metrics",
			fieldIds: ["alb-target-health"],
		},
	],
	fields: albFields,
};

/* ---------------------------------------------------------------------------
 * Canvas-only nodes — Layout tab only, never any AWS controls
 * ------------------------------------------------------------------------- */

const canvasLayoutFields = (prefix: string): PropertyFieldDefinition[] => [
	{
		id: `${prefix}-label`,
		property: "label",
		label: "Label",
		mutability: "LIVE",
		valueKind: "canvas-only",
		controlType: "text",
		section: "layout",
		persistenceChannel: "layout",
	},
	{
		id: `${prefix}-position-x`,
		property: "position.x",
		label: "Position X",
		mutability: "LIVE",
		valueKind: "canvas-only",
		controlType: "number",
		section: "layout",
		persistenceChannel: "layout",
	},
	{
		id: `${prefix}-position-y`,
		property: "position.y",
		label: "Position Y",
		mutability: "LIVE",
		valueKind: "canvas-only",
		controlType: "number",
		section: "layout",
		persistenceChannel: "layout",
	},
	{
		id: `${prefix}-width`,
		property: "style.width",
		label: "Width",
		mutability: "LIVE",
		valueKind: "canvas-only",
		controlType: "number",
		section: "layout",
		persistenceChannel: "layout",
	},
	{
		id: `${prefix}-height`,
		property: "style.height",
		label: "Height",
		mutability: "LIVE",
		valueKind: "canvas-only",
		controlType: "number",
		section: "layout",
		persistenceChannel: "layout",
	},
];

const group: PropertyPanelDefinition = {
	id: "group",
	title: "Group",
	canvasOnly: true,
	sections: [
		{
			key: "layout",
			archetype: "standard",
			fieldIds: canvasLayoutFields("group").map((f) => f.id),
		},
	],
	fields: canvasLayoutFields("group"),
};

const dynamicGroup: PropertyPanelDefinition = {
	id: "dynamicGroup",
	title: "Dynamic Group",
	canvasOnly: true,
	sections: [
		{
			key: "layout",
			archetype: "standard",
			fieldIds: canvasLayoutFields("dynamic-group").map((f) => f.id),
		},
	],
	fields: canvasLayoutFields("dynamic-group"),
};

/* ---------------------------------------------------------------------------
 * Catalog export
 * ------------------------------------------------------------------------- */

/**
 * The eleven required catalog panels. Representative coverage only —
 * see WEB_APP_NODE_PROPERTIES.md for the full 413-setting surface.
 */
export const NODE_CATALOG_V3: readonly PropertyPanelDefinition[] = [
	backend,
	service,
	apiGateway,
	postgres,
	s3,
	cloudFront,
	route53,
	ecr,
	group,
	alb,
	dynamicGroup,
];

export const NODE_CATALOG_BY_ID_V3: Readonly<
	Record<string, PropertyPanelDefinition>
> = Object.fromEntries(NODE_CATALOG_V3.map((panel) => [panel.id, panel]));
