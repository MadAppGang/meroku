/**
 * properties-v3 — synthetic fixtures (instance data only).
 *
 * ALL values are synthetic demo data. No real infrastructure, account IDs,
 * secrets, or AWS calls. Account-looking IDs are 12-digit dummies; secrets
 * are masked placeholders. Telemetry carries explicit fixture labels and
 * fixed ISO timestamps, or is explicitly unresolved/empty — no fabricated
 * live stats.
 *
 * Baselines are keyed by field.id (the primary lookup path in helpers.ts).
 */

import type { PropertyPanelInstance, TelemetryReading } from "./types";

export const FIXTURE_LABEL = "Synthetic fixture data — not live";
export const FIXTURE_CAPTURED_AT = "2026-09-09T12:00:00Z";

const created = (
	nodeInstanceId: string,
	baseline: Record<string, unknown>,
	extra: Partial<PropertyPanelInstance> = {},
): PropertyPanelInstance => ({
	nodeInstanceId,
	lifecycleState: "created",
	baseline,
	cloudWatchDestination: null,
	...extra,
});

const reading = (
	fieldId: string,
	resolution: TelemetryReading["resolution"] = "runtime",
): TelemetryReading => ({
	fieldId,
	timestamp: FIXTURE_CAPTURED_AT,
	resolution,
});

/* ---------------------------------------------------------------------------
 * Backend
 * ------------------------------------------------------------------------- */

export const backendDefault: PropertyPanelInstance = created(
	"fixture-node-backend-default",
	{
		"backend-ecs-service-name": "myapp_service_dev",
		"backend-task-role-arn":
			"arn:aws:iam::000000000000:role/myapp_backend_task_dev",
		"backend-execution-role-arn":
			"arn:aws:iam::000000000000:role/myapp_backend_task_execution_dev",
		"backend-docker-image": "",
		"backend-container-command": [],
		"backend-cpu": "256", // string, per YAML schema
		"backend-memory": "512", // string, per YAML schema
		"backend-autoscaling-enabled": false,
		"backend-desired-count": 1,
		"backend-alb-domain-name": "",
		"backend-image-port": 8080,
		"backend-health-endpoint": "/health",
		"backend-remote-access": false,
		"backend-xray-enabled": false,
		"backend-env-variables": {
			LOG_LEVEL: "info",
			FEATURE_FLAGS: "experimental-login",
		},
		"backend-ssm-parameters": {
			DB_HOST: { value: "myapp-postgres.dev.internal", type: "String" },
			API_KEY: { value: "REDACTED-FIXTURE", type: "SecureString" },
		},
		"backend-bucket-postfix": "abc12",
		"backend-bucket-public": false,
		"backend-bucket-arn": "arn:aws:s3:::myapp-backend-dev-abc12",
		"backend-api-domain-prefix": "api",
		"backend-log-group-name": "/ecs/myapp_backend_dev",
		"backend-log-retention": 30,
		"backend-log-stream": [
			{ timestamp: FIXTURE_CAPTURED_AT, message: "[fixture] service steady" },
		],
		"backend-running-count": 1,
		"backend-cpu-utilization": 12.4,
		"backend-memory-utilization": 38.2,
		"backend-deployed-image-tag": "sha-fixture0001",
	},
	{
		// Only nodes with an explicitly configured CloudWatch destination get a
		// Logs tab; this demo fixture configures one.
		cloudWatchDestination: "/ecs/myapp_backend_dev",
		telemetry: [
			reading("backend-running-count"),
			reading("backend-cpu-utilization"),
			reading("backend-memory-utilization"),
			reading("backend-deployed-image-tag"),
			reading("backend-log-stream"),
		],
		resource: {
			id: "ecs-service-backend",
			name: "myapp_service_dev (ECS service)",
		},
	},
);

export const backendUnresolvedTelemetry: PropertyPanelInstance = {
	...backendDefault,
	nodeInstanceId: "fixture-node-backend-no-telemetry",
	cloudWatchDestination: null, // no CW destination → no Logs tab
	telemetry: [], // telemetry explicitly unresolved — never fabricated
	baseline: {
		...backendDefault.baseline,
		"backend-log-stream": undefined,
		"backend-running-count": undefined,
		"backend-cpu-utilization": undefined,
		"backend-memory-utilization": undefined,
		"backend-deployed-image-tag": undefined,
	},
};

/* ---------------------------------------------------------------------------
 * Service — CREATE before/after creation
 * ------------------------------------------------------------------------- */

export const serviceNotCreated: PropertyPanelInstance = {
	nodeInstanceId: "fixture-node-service-not-created",
	lifecycleState: "uncreated", // CREATE field `name` still editable
	baseline: {
		"service-name": "worker",
		"service-enabled": true,
		"service-docker-image": "",
		"service-container-command": [],
		"service-ecr-mode": "create_ecr",
		"service-cpu": 256, // number, per YAML schema (unlike backend strings)
		"service-memory": 512,
		"service-desired-count": 1,
		"service-container-port": 3000,
		"service-public-access": false,
		"service-environment-variables": { QUEUE: "jobs" },
		"service-xray-enabled": false,
		// AWS-assigned identity absent before creation (resolution: provisioned)
		"service-ecs-name": undefined,
		"service-task-def-arn": undefined,
	},
	cloudWatchDestination: null,
	resource: {
		id: "ecs-service-api",
		name: "myapp_service_api_dev (ECS service)",
	},
};

export const serviceCreated: PropertyPanelInstance = {
	...serviceNotCreated,
	nodeInstanceId: "fixture-node-service-created",
	lifecycleState: "created", // `name` now locked set-once / destructive
	baseline: {
		...serviceNotCreated.baseline,
		"service-ecs-name": "myapp_service_api_dev",
		"service-task-def-arn":
			"arn:aws:ecs:us-east-1:000000000000:task-definition/myapp_service_api:42",
		"service-running-count": 2,
		"service-status": "ACTIVE",
	},
	telemetry: [reading("service-running-count"), reading("service-status")],
};

/* ---------------------------------------------------------------------------
 * PostgreSQL — RDS default, Aurora variant, not-created, destructive drafts
 * ------------------------------------------------------------------------- */

export const postgresRdsDefault: PropertyPanelInstance = created(
	"fixture-node-postgres-rds",
	{
		"postgres-enabled": true,
		"postgres-aurora": false,
		"postgres-dbname": "myapp",
		"postgres-username": "postgres",
		"postgres-engine-version": "16.4",
		"postgres-public-access": false,
		"postgres-instance-class": "db.t4g.micro",
		"postgres-allocated-storage": 20,
		"postgres-multi-az": false,
		"postgres-storage-encrypted": true,
		"postgres-deletion-protection": false,
		"postgres-skip-final-snapshot": false,
		"postgres-iam-db-auth": false,
		"postgres-endpoint":
			"myapp-postgres-dev.cluster-fixture.us-east-1.rds.amazonaws.com",
		"postgres-port": 5432,
		"postgres-engine": "postgres",
		"postgres-engine-version-runtime": "16.4",
		"postgres-status": "available",
	},
	{
		telemetry: [reading("postgres-status")],
		resource: { id: "rds-postgres", name: "myapp_postgres_dev (PostgreSQL)" },
	},
);

export const postgresAurora: PropertyPanelInstance = {
	...postgresRdsDefault,
	nodeInstanceId: "fixture-node-postgres-aurora",
	baseline: {
		...postgresRdsDefault.baseline,
		"postgres-aurora": true,
		// Aurora-only fields; min_capacity 0 is VALID (pause when idle)
		"postgres-min-capacity": 0,
		"postgres-max-capacity": 8,
		// RDS-only fields removed from the record entirely
		"postgres-instance-class": undefined,
		"postgres-allocated-storage": undefined,
		"postgres-multi-az": undefined,
		"postgres-storage-encrypted": undefined,
	},
};

export const postgresNotCreated: PropertyPanelInstance = {
	nodeInstanceId: "fixture-node-postgres-not-created",
	lifecycleState: "uncreated",
	baseline: {
		"postgres-enabled": true,
		"postgres-aurora": false,
		"postgres-dbname": "myapp",
		"postgres-username": "postgres",
		"postgres-engine-version": "16.4",
		"postgres-public-access": false,
		"postgres-instance-class": "db.t4g.micro",
		"postgres-allocated-storage": 20,
		"postgres-endpoint": undefined,
		"postgres-port": undefined,
		"postgres-status": undefined,
	},
	cloudWatchDestination: null,
	resource: { id: "rds-postgres", name: "myapp_postgres_dev (PostgreSQL)" },
};

/**
 * Destructive draft: dbname + aurora toggle pending. Baseline is the created
 * RDS fixture; the draft below produces `recreates-data-loss` changes naming
 * the exact resource and property paths. Saving itself destroys nothing —
 * the next deploy may recreate and lose data.
 */
export const postgresDestructiveDraft: Record<string, unknown> = {
	...postgresRdsDefault.baseline,
	"postgres-dbname": "myapp_v2",
	"postgres-aurora": true,
	"postgres-min-capacity": 0.5,
	"postgres-max-capacity": 16,
	"postgres-instance-class": undefined,
	"postgres-allocated-storage": undefined,
	"postgres-multi-az": undefined,
	"postgres-storage-encrypted": undefined,
};

/* ---------------------------------------------------------------------------
 * S3 / CloudFront / Route 53 / ECR / ALB
 * ------------------------------------------------------------------------- */

export const s3Default: PropertyPanelInstance = created(
	"fixture-node-s3",
	{
		"s3-bucket-name": "myapp-assets",
		"s3-bucket-public": false,
		"s3-bucket-versioning": true,
		"s3-cors-rules": [
			{
				id: "web",
				allowed_origins: ["https://app.example.com"],
				allowed_methods: ["GET"],
				allowed_headers: ["*"],
				max_age_seconds: 3600,
			},
		],
		"s3-bucket-arn": "arn:aws:s3:::myapp-assets",
		"s3-regional-domain": "myapp-assets.s3.us-east-1.amazonaws.com",
		"s3-console-url":
			"https://s3.console.aws.amazon.com/s3/buckets/myapp-assets",
		"s3-creation-date": "2026-08-01T10:00:00Z",
		"s3-object-listing": [
			{ key: "index.html", size: 1024 },
			{ key: "app.js", size: 20480 },
		],
	},
	{
		cloudWatchDestination: null, // S3 never gets a CloudWatch Logs viewer here
		telemetry: [reading("s3-object-listing")],
		resource: { id: "s3-bucket-assets", name: "myapp-assets (S3 bucket)" },
	},
);

export const cloudFrontDefault: PropertyPanelInstance = created(
	"fixture-node-cloudfront",
	{
		"cloudfront-name": "myapp-distribution",
		"cloudfront-enabled": true,
		"cloudfront-price-class": "PriceClass_100",
		"cloudfront-default-root-object": "index.html",
		"cloudfront-spa-mode": true,
		"cloudfront-domain-aliases": ["app.example.com"],
		"cloudfront-origins": [
			{
				name: "assets",
				type: "s3",
				bucket_name: "myapp-assets",
				create_bucket: false,
				use_oac: true,
			},
		],
		"cloudfront-cache-behaviors": [
			{ path_pattern: "/static/*", origin_name: "assets", compress: true },
		],
		// S3 access logging configuration only — NOT a CloudWatch destination
		"cloudfront-logging-enabled": true,
		"cloudfront-logging-bucket": "myapp-access-logs",
		"cloudfront-distribution-id": "EFIXTURE000000",
		"cloudfront-domain-name": "dfixture0000.cloudfront.net",
		"cloudfront-deployment-status": "Deployed",
	},
	{
		cloudWatchDestination: null, // explicitly: no CW destination → no Logs tab
		telemetry: [reading("cloudfront-deployment-status")],
		resource: {
			id: "cloudfront-distribution",
			name: "myapp-distribution (CloudFront)",
		},
	},
);

export const route53Default: PropertyPanelInstance = created(
	"fixture-node-route53",
	{
		"route53-domain-enabled": true,
		"route53-domain-name": "example.com",
		"route53-create-zone": true,
		"route53-api-domain-prefix": "api",
		"route53-add-env-prefix": true,
		"route53-is-dns-root": true, // CREATE field, locked after creation
		"route53-additional-domains": [
			{ domain: "example.org", create_zone: true },
		],
		"route53-ns-records": [
			"ns-1-fixture.awsdns-00.example",
			"ns-2-fixture.awsdns-01.example",
		],
		"route53-delegation-role-arn": "",
		"route53-record-sets": [{ name: "api.example.com", type: "A", ttl: 300 }],
		"route53-propagation-status": "PROPAGATING",
	},
	{
		telemetry: [
			reading("route53-record-sets"),
			reading("route53-propagation-status"),
		],
		resource: { id: "route53-zone", name: "example.com (hosted zone)" },
	},
);

export const ecrDefault: PropertyPanelInstance = created(
	"fixture-node-ecr-local",
	{
		"ecr-strategy": "local",
		"ecr-trusted-accounts": [
			{ account_id: "111111111111", env: "staging", region: "us-east-1" },
		],
		"ecr-repository-uri":
			"000000000000.dkr.ecr.us-east-1.amazonaws.com/myapp_backend",
		"ecr-repository-arn":
			"arn:aws:ecr:us-east-1:000000000000:repository/myapp_backend",
		"ecr-image-tags": ["sha-fixture0001", "sha-fixture0002"],
		"ecr-image-pushed-at": "2026-09-08T18:30:00Z",
	},
	{
		telemetry: [reading("ecr-image-tags"), reading("ecr-image-pushed-at")],
		resource: { id: "ecr-repository", name: "myapp_backend (ECR repository)" },
	},
);

export const ecrCrossAccount: PropertyPanelInstance = {
	...ecrDefault,
	nodeInstanceId: "fixture-node-ecr-cross-account",
	baseline: {
		...ecrDefault.baseline,
		"ecr-strategy": "cross_account",
		"ecr-account-id": "111111111111", // dummy 12-digit fixture ID
		"ecr-account-region": "us-east-1",
		"ecr-trusted-accounts": undefined, // local-only control hidden
	},
};

export const albDefault: PropertyPanelInstance = created(
	"fixture-node-alb",
	{
		"alb-enabled": true,
		"alb-domain-name": "app.example.com",
		"alb-health-endpoint": "/health",
		"alb-image-port": 8080,
		// Derived routing rules — read-only Relationships overview
		"alb-routing-rules": [
			{ path: "/*", service: "backend (public)" },
			{ path: "/api/*", service: "api (public)" },
		],
		"alb-dns-name": "myapp-alb-dev-fixture.us-east-1.elb.amazonaws.com",
		"alb-arn":
			"arn:aws:elasticloadbalancing:us-east-1:000000000000:loadbalancer/app/myapp-alb-dev/fixture",
		"alb-target-group-arns": [
			"arn:aws:elasticloadbalancing:us-east-1:000000000000:targetgroup/myapp-service-backend-tg-dev/fixture",
		],
		"alb-target-health": "2/2 healthy",
		"alb-certificate-arn":
			"arn:aws:acm:us-east-1:000000000000:certificate/fixture",
	},
	{
		telemetry: [reading("alb-target-health")],
		resource: { id: "alb", name: "myapp-alb-dev (ALB)" },
	},
);

/* ---------------------------------------------------------------------------
 * Canvas-only groups — Layout only, values never reach AWS
 * ------------------------------------------------------------------------- */

export const groupDefault: PropertyPanelInstance = {
	nodeInstanceId: "fixture-node-group",
	lifecycleState: "created",
	baseline: {
		"group-label": "Public edge",
		"group-position-x": 120,
		"group-position-y": 80,
		"group-width": 420,
		"group-height": 260,
	},
	cloudWatchDestination: null,
};

export const dynamicGroupDefault: PropertyPanelInstance = {
	nodeInstanceId: "fixture-node-dynamic-group",
	lifecycleState: "created",
	baseline: {
		"dynamic-group-label": "Auto: services",
		"dynamic-group-position-x": 600,
		"dynamic-group-position-y": 80,
		"dynamic-group-width": 380,
		"dynamic-group-height": 240,
	},
	cloudWatchDestination: null,
};

/* ---------------------------------------------------------------------------
 * API Gateway
 * ------------------------------------------------------------------------- */

export const apiGatewayDefault: PropertyPanelInstance = created(
	"fixture-node-apigw",
	{
		"apigw-api-id": "fixture0api9",
		"apigw-stage": "dev",
		"apigw-api-type": "AWS HTTP API",
		"apigw-endpoint":
			"https://fixture0api9.execute-api.us-east-1.amazonaws.com",
		"apigw-routes": [
			{ method: "ANY", path: "/api/*", service: "backend (public)" },
		],
		"apigw-domain-prefix": "api",
		"apigw-rate-limit": 100,
		"apigw-burst-limit": 200,
		"apigw-log-retention": 30,
		"apigw-log-group": "/http-api/myapp-api-dev/access",
	},
	{
		cloudWatchDestination: "/http-api/myapp-api-dev/access", // explicit demo CW destination → Logs tab
		resource: { id: "apigw-http-api", name: "myapp-api-dev (HTTP API)" },
	},
);

/** Aggregate for story consumption. */
export const fixtures = {
	backendDefault,
	backendUnresolvedTelemetry,
	serviceNotCreated,
	serviceCreated,
	postgresRdsDefault,
	postgresAurora,
	postgresNotCreated,
	postgresDestructiveDraft,
	s3Default,
	cloudFrontDefault,
	route53Default,
	ecrDefault,
	ecrCrossAccount,
	albDefault,
	groupDefault,
	dynamicGroupDefault,
	apiGatewayDefault,
} as const;
