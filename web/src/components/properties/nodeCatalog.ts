import {
	Activity,
	AlarmClock,
	Archive,
	Box,
	Boxes,
	Braces,
	Cloud,
	CloudCog,
	Container,
	Database,
	FileCode2,
	FolderKey,
	Gauge,
	Github,
	Globe2,
	HardDrive,
	KeyRound,
	Layers3,
	Mail,
	Network,
	Package,
	RadioTower,
	Route,
	Send,
	Share2,
	ShieldCheck,
	TestTube2,
	Upload,
	Workflow,
	Zap,
} from "lucide-react";
import type {
	PropertyFieldDefinition,
	PropertyInventoryEntry,
	PropertyKeyValue,
	PropertyPanelDefinition,
	PropertySectionDefinition,
	PropertyViewDefinition,
} from "./types";

const context = "dev · ap-southeast-2";

const cpuOptions = [
	{ label: "256 (0.25 vCPU)", value: "256" },
	{ label: "512 (0.5 vCPU)", value: "512" },
	{ label: "1024 (1 vCPU)", value: "1024" },
	{ label: "2048 (2 vCPU)", value: "2048" },
];

const memoryOptions = [
	{ label: "512 MB", value: "512" },
	{ label: "1024 MB", value: "1024" },
	{ label: "2048 MB", value: "2048" },
	{ label: "4096 MB", value: "4096" },
];

const kv = (
	id: string,
	key: string,
	value: string,
	options?: Pick<PropertyKeyValue, "secret" | "readOnly">,
): PropertyKeyValue => ({ id, key, value, ...options });

const section = (
	id: string,
	title: string,
	fields: PropertyFieldDefinition[],
	overrides: Partial<PropertySectionDefinition> = {},
): PropertySectionDefinition => ({ id, title, fields, ...overrides });

const view = (
	id: string,
	label: string,
	sections: PropertySectionDefinition[],
): PropertyViewDefinition => ({ id, label, sections });

const serviceSetupSections = (
	prefix: string,
	image: string,
	imageReadOnly: boolean,
): PropertySectionDefinition[] => [
	section(
		`${prefix}-deploy`,
		"Container Image",
		[
			{
				id: `${prefix}-image`,
				label: "Container image",
				kind: imageReadOnly ? "readonly" : "text",
				value: image,
				mono: true,
			},
			{
				id: `${prefix}-command`,
				label: "Command override",
				kind: "text",
				value: "npm, start",
				placeholder: "Use image entrypoint",
				mono: true,
			},
		],
		{
			description: "Image source and optional entrypoint override",
			icon: Container,
			iconTone: "info",
		},
	),
	section(
		`${prefix}-runtime`,
		"Runtime & Network",
		[
			{
				id: `${prefix}-port`,
				label: "Container port",
				kind: "number",
				value: 8080,
				span: "half",
			},
			{
				id: `${prefix}-health`,
				label: "Health path",
				kind: "text",
				value: "/health",
				mono: true,
				span: "half",
			},
			{
				id: `${prefix}-cpu`,
				label: "CPU",
				kind: "select",
				value: "512",
				options: cpuOptions,
				span: "half",
			},
			{
				id: `${prefix}-memory`,
				label: "Memory",
				kind: "select",
				value: "1024",
				options: memoryOptions,
				span: "half",
			},
			{
				id: `${prefix}-desired`,
				label: "Desired tasks",
				kind: "number",
				value: 2,
				span: "half",
			},
			{
				id: `${prefix}-autoscaling`,
				label: "Autoscaling",
				kind: "toggle",
				value: true,
				description: "Scale tasks from load",
				span: "half",
			},
			{
				id: `${prefix}-minimum`,
				label: "Minimum tasks",
				kind: "number",
				value: 1,
				span: "half",
			},
			{
				id: `${prefix}-maximum`,
				label: "Maximum tasks",
				kind: "number",
				value: 5,
				span: "half",
			},
			{
				id: `${prefix}-xray`,
				label: "Distributed tracing",
				kind: "toggle",
				value: true,
				description: "Run the X-Ray collector sidecar",
			},
		],
		{
			description: "Networking, resources, scaling, and tracing",
			icon: Gauge,
			iconTone: "warning",
		},
	),
	section(
		`${prefix}-environment`,
		"Environment Variables",
		[
			{
				id: `${prefix}-env`,
				label: "Variables",
				kind: "key-value",
				value: [
					kv(`${prefix}-env-1`, "NODE_ENV", "production", { readOnly: true }),
					kv(`${prefix}-env-2`, "DB_HOST", "postgres.internal", {
						readOnly: true,
					}),
					kv(`${prefix}-env-3`, "API_KEY", "sk_live_example", { secret: true }),
				],
			},
		],
		{
			description: "System-injected values and task-specific overrides",
			icon: Braces,
			iconTone: "success",
		},
	),
	section(
		`${prefix}-advanced`,
		"Advanced Settings",
		[
			{
				id: `${prefix}-host-port`,
				label: "Host port",
				kind: "number",
				value: 8080,
				span: "half",
			},
			{
				id: `${prefix}-deployment-timeout`,
				label: "Deploy timeout",
				kind: "number",
				value: 15,
				unit: "min",
				span: "half",
			},
			{
				id: `${prefix}-remote-access`,
				label: "Remote access",
				kind: "toggle",
				value: false,
				description: "Enable ECS Exec for this service",
			},
		],
		{ advanced: true },
	),
];

const serviceOperations = (prefix: string): PropertyViewDefinition =>
	view("operations", "Operations", [
		section(
			`${prefix}-runtime-status`,
			"Runtime Status",
			[
				{
					id: `${prefix}-status`,
					label: "Status",
					kind: "status",
					value: "Healthy",
				},
				{
					id: `${prefix}-service-name`,
					label: "ECS service",
					kind: "readonly",
					value: `circl_${prefix}_dev`,
					mono: true,
				},
				{
					id: `${prefix}-log-group`,
					label: "Log group",
					kind: "readonly",
					value: `/ecs/circl/${prefix}/dev`,
					mono: true,
				},
			],
			{ icon: Activity, iconTone: "success" },
		),
	]);

const serviceAccess = (prefix: string): PropertyViewDefinition =>
	view("access", "Access", [
		section(
			`${prefix}-access`,
			"IAM & Parameters",
			[
				{
					id: `${prefix}-task-role`,
					label: "Task role",
					kind: "readonly",
					value: `circl_${prefix}_task_dev`,
					mono: true,
				},
				{
					id: `${prefix}-parameter-prefix`,
					label: "Parameter prefix",
					kind: "readonly",
					value: `/dev/circl/${prefix}/`,
					mono: true,
				},
			],
			{ icon: ShieldCheck, iconTone: "info" },
		),
	]);

const makeService = (
	nodeType: "backend" | "service",
	name: string,
	image: string,
	imageReadOnly: boolean,
): PropertyPanelDefinition => ({
	nodeType,
	name,
	displayName:
		nodeType === "backend" ? "Backend service" : "Additional service",
	icon: Container,
	status: "Running",
	statusTone: "success",
	context,
	deletable: nodeType === "service",
	lastUpdated: "Last updated 2m ago",
	views: [
		view(
			"setup",
			"Setup",
			serviceSetupSections(nodeType, image, imageReadOnly),
		),
		serviceOperations(nodeType),
		serviceAccess(nodeType),
	],
});

export const PROPERTY_NODE_CATALOG: Record<string, PropertyPanelDefinition> = {
	"client-app": {
		nodeType: "client-app",
		name: "Client app",
		displayName: "Client app",
		icon: Globe2,
		status: "External",
		statusTone: "neutral",
		context,
		views: [
			view("details", "Details", [
				section(
					"client-details",
					"External Entry Point",
					[
						{
							id: "client-kind",
							label: "Type",
							kind: "readonly",
							value: "Web / mobile clients",
						},
						{
							id: "client-target",
							label: "Target",
							kind: "readonly",
							value: "Public API endpoint",
							mono: true,
						},
					],
					{ icon: Globe2 },
				),
			]),
		],
	},
	github: {
		nodeType: "github",
		name: "GitHub Actions",
		displayName: "GitHub Actions / OIDC",
		icon: Github,
		status: "Enabled",
		statusTone: "success",
		context,
		views: [
			view("setup", "Setup", [
				section(
					"github-oidc",
					"OpenID Connect",
					[
						{
							id: "github-enabled",
							label: "GitHub OIDC",
							kind: "toggle",
							value: true,
							description: "Allow passwordless deployments",
						},
						{
							id: "github-subjects",
							label: "Allowed subjects",
							kind: "tags",
							value: [
								"repo:madappgang/circl:*",
								"repo:madappgang/backend:ref:refs/heads/main",
							],
						},
					],
					{ icon: ShieldCheck, iconTone: "success" },
				),
				section(
					"github-advanced",
					"Advanced Settings",
					[
						{
							id: "github-audience",
							label: "Audience",
							kind: "readonly",
							value: "sts.amazonaws.com",
							mono: true,
						},
					],
					{ advanced: true },
				),
			]),
			view("workflow", "Workflow", [
				section(
					"github-example",
					"Deployment Workflow",
					[
						{
							id: "github-file",
							label: "File",
							kind: "readonly",
							value: ".github/workflows/deploy.yml",
							mono: true,
						},
						{
							id: "github-trigger",
							label: "Trigger",
							kind: "readonly",
							value: "push → main",
							mono: true,
						},
					],
					{ icon: Workflow, iconTone: "info" },
				),
			]),
		],
	},
	"api-gateway": {
		nodeType: "api-gateway",
		name: "Amazon API Gateway",
		displayName: "API Gateway",
		icon: Route,
		status: "Active",
		statusTone: "success",
		context,
		views: [
			view("setup", "Setup", [
				section(
					"api-main",
					"HTTP API",
					[
						{
							id: "api-kind",
							label: "API type",
							kind: "readonly",
							value: "HTTP API",
						},
						{
							id: "api-stage",
							label: "Stage",
							kind: "text",
							value: "$default",
							mono: true,
							span: "half",
						},
						{
							id: "api-cors",
							label: "CORS",
							kind: "toggle",
							value: true,
							span: "half",
						},
						{
							id: "api-endpoint",
							label: "Invoke URL",
							kind: "readonly",
							value: "https://api.dev.example.com",
							mono: true,
						},
					],
					{ icon: Route, iconTone: "info" },
				),
			]),
			view("details", "Routes", [
				section(
					"api-routes",
					"Routes & Integration",
					[
						{
							id: "api-route",
							label: "Default route",
							kind: "readonly",
							value: "$default → Cloud Map",
							mono: true,
						},
						{
							id: "api-timeout",
							label: "Integration timeout",
							kind: "number",
							value: 29,
							unit: "sec",
							span: "half",
						},
					],
					{ icon: Network },
				),
			]),
		],
	},
	alb: {
		nodeType: "alb",
		name: "Application Load Balancer",
		displayName: "Application Load Balancer",
		icon: Network,
		status: "Active",
		statusTone: "success",
		context,
		views: [
			view("setup", "Setup", [
				section(
					"alb-main",
					"Load Balancer",
					[
						{
							id: "alb-enabled",
							label: "Use ALB ingress",
							kind: "toggle",
							value: true,
							description: "Replace API Gateway ingress",
						},
						{
							id: "alb-domain",
							label: "Domain",
							kind: "text",
							value: "api.dev.example.com",
							mono: true,
						},
						{
							id: "alb-timeout",
							label: "Idle timeout",
							kind: "number",
							value: 300,
							unit: "sec",
							span: "half",
						},
						{
							id: "alb-health",
							label: "Health path",
							kind: "text",
							value: "/health",
							mono: true,
							span: "half",
						},
					],
					{ icon: Network, iconTone: "info" },
				),
			]),
			view("routing", "Routing", [
				section(
					"alb-rules",
					"Listener Rules",
					[
						{
							id: "alb-rule",
							label: "HTTPS :443",
							kind: "readonly",
							value: "/* → backend-service",
							mono: true,
						},
						{
							id: "alb-target",
							label: "Target status",
							kind: "status",
							value: "Healthy",
						},
					],
					{ icon: Route, iconTone: "success" },
				),
			]),
		],
	},
	route53: {
		nodeType: "route53",
		name: "Amazon Route 53",
		displayName: "Route 53",
		icon: Globe2,
		status: "Configured",
		statusTone: "success",
		context,
		views: [
			view("setup", "Setup", [
				section(
					"route53-domain",
					"Domain",
					[
						{
							id: "route53-enabled",
							label: "Custom domain",
							kind: "toggle",
							value: true,
						},
						{
							id: "route53-name",
							label: "Domain name",
							kind: "text",
							value: "coretechx.dev",
							mono: true,
						},
						{
							id: "route53-api-prefix",
							label: "API prefix",
							kind: "text",
							value: "api",
							mono: true,
							span: "half",
						},
						{
							id: "route53-env-prefix",
							label: "Add environment prefix",
							kind: "toggle",
							value: true,
							span: "half",
						},
						{
							id: "route53-create-zone",
							label: "Create hosted zone",
							kind: "toggle",
							value: false,
						},
					],
					{ icon: Globe2, iconTone: "info" },
				),
			]),
			view("dns", "DNS Records", [
				section(
					"route53-records",
					"Managed Records",
					[
						{
							id: "route53-api-record",
							label: "API",
							kind: "readonly",
							value: "api.dev.coretechx.dev → ALB",
							mono: true,
						},
						{
							id: "route53-zone",
							label: "Hosted zone",
							kind: "readonly",
							value: "Z093872EXAMPLE",
							mono: true,
						},
					],
					{ icon: Network },
				),
			]),
		],
	},
	ecs: {
		nodeType: "ecs",
		name: "Amazon ECS Cluster",
		displayName: "ECS cluster",
		icon: Boxes,
		status: "Running",
		statusTone: "success",
		context,
		views: [
			view("setup", "Setup", [
				section(
					"ecs-cluster",
					"Cluster",
					[
						{
							id: "ecs-name",
							label: "Cluster name",
							kind: "readonly",
							value: "circl_cluster_dev",
							mono: true,
						},
						{
							id: "ecs-launch",
							label: "Launch type",
							kind: "readonly",
							value: "AWS Fargate",
						},
						{
							id: "ecs-insights",
							label: "Container Insights",
							kind: "toggle",
							value: true,
						},
						{
							id: "ecs-notifications",
							label: "Deploy notifications",
							kind: "toggle",
							value: true,
						},
					],
					{ icon: Boxes, iconTone: "success" },
				),
			]),
			view("details", "Details", [
				section(
					"ecs-network",
					"Network & Services",
					[
						{
							id: "ecs-vpc",
							label: "VPC",
							kind: "readonly",
							value: "vpc-06f2example",
							mono: true,
						},
						{
							id: "ecs-services",
							label: "Active services",
							kind: "readonly",
							value: "4",
						},
					],
					{ icon: Network },
				),
			]),
		],
	},
	backend: makeService(
		"backend",
		"backend",
		"123456789.dkr.ecr.ap-southeast-2.amazonaws.com/circl_backend:latest",
		true,
	),
	service: makeService(
		"service",
		"terminator",
		"123456789.dkr.ecr.ap-southeast-2.amazonaws.com/terminator:latest",
		false,
	),
	ecr: {
		nodeType: "ecr",
		name: "Amazon ECR",
		displayName: "Elastic Container Registry",
		icon: Package,
		status: "Available",
		statusTone: "success",
		context,
		views: [
			view("setup", "Setup", [
				section(
					"ecr-strategy",
					"Registry Strategy",
					[
						{
							id: "ecr-mode",
							label: "Image source",
							kind: "select",
							value: "local",
							options: [
								{ label: "This account", value: "local" },
								{ label: "Cross-account", value: "cross" },
							],
						},
						{
							id: "ecr-repository",
							label: "Backend repository",
							kind: "readonly",
							value: "circl_backend",
							mono: true,
						},
					],
					{ icon: Package, iconTone: "info" },
				),
				section(
					"ecr-advanced",
					"Cross-account Settings",
					[
						{
							id: "ecr-account",
							label: "Source account",
							kind: "text",
							value: "123456789012",
							mono: true,
							span: "half",
						},
						{
							id: "ecr-region",
							label: "Source region",
							kind: "select",
							value: "ap-southeast-2",
							options: [
								{ label: "ap-southeast-2", value: "ap-southeast-2" },
								{ label: "us-east-1", value: "us-east-1" },
							],
							span: "half",
						},
					],
					{ advanced: true },
				),
			]),
			view("repositories", "Repositories", [
				section(
					"ecr-repos",
					"Repositories",
					[
						{
							id: "ecr-repo-list",
							label: "Repository",
							kind: "readonly",
							value: "circl_backend · 14 images",
							mono: true,
						},
					],
					{ icon: Archive },
				),
			]),
			view("push", "Push", [
				section(
					"ecr-push",
					"Push Instructions",
					[
						{
							id: "ecr-command",
							label: "Login command",
							kind: "readonly",
							value: "aws ecr get-login-password | docker login …",
							mono: true,
						},
					],
					{ icon: Upload },
				),
			]),
		],
	},
	postgres: {
		nodeType: "postgres",
		name: "PostgreSQL Database",
		displayName: "PostgreSQL / Aurora",
		icon: Database,
		status: "Available",
		statusTone: "success",
		context,
		views: [
			view("setup", "Setup", [
				section(
					"postgres-engine",
					"Database",
					[
						{
							id: "postgres-engine-kind",
							label: "Deployment",
							kind: "select",
							value: "rds",
							options: [
								{ label: "RDS PostgreSQL", value: "rds" },
								{ label: "Aurora Serverless v2", value: "aurora" },
							],
						},
						{
							id: "postgres-name",
							label: "Database name",
							kind: "text",
							value: "circl",
							mono: true,
							span: "half",
						},
						{
							id: "postgres-user",
							label: "Username",
							kind: "text",
							value: "postgres",
							mono: true,
							span: "half",
						},
						{
							id: "postgres-class",
							label: "Instance class",
							kind: "select",
							value: "db.t4g.micro",
							options: [
								{ label: "db.t4g.micro", value: "db.t4g.micro" },
								{ label: "db.t4g.small", value: "db.t4g.small" },
							],
							span: "half",
						},
						{
							id: "postgres-storage",
							label: "Storage",
							kind: "number",
							value: 20,
							unit: "GB",
							span: "half",
						},
						{
							id: "postgres-public",
							label: "Public access",
							kind: "toggle",
							value: false,
							span: "half",
						},
						{
							id: "postgres-multiaz",
							label: "Multi-AZ",
							kind: "toggle",
							value: false,
							span: "half",
						},
					],
					{ icon: Database, iconTone: "success" },
				),
				section(
					"postgres-advanced",
					"Advanced Settings",
					[
						{
							id: "postgres-encryption",
							label: "Storage encryption",
							kind: "toggle",
							value: true,
						},
						{
							id: "postgres-protection",
							label: "Deletion protection",
							kind: "toggle",
							value: false,
						},
						{
							id: "postgres-final-snapshot",
							label: "Skip final snapshot",
							kind: "toggle",
							value: true,
						},
					],
					{ advanced: true },
				),
			]),
			view("connection", "Connection", [
				section(
					"postgres-connection",
					"Connection Details",
					[
						{
							id: "postgres-host",
							label: "Host",
							kind: "readonly",
							value: "circl-dev.abc.ap-southeast-2.rds.amazonaws.com",
							mono: true,
						},
						{
							id: "postgres-port",
							label: "Port",
							kind: "readonly",
							value: "5432",
							mono: true,
							span: "half",
						},
						{
							id: "postgres-secret",
							label: "Password parameter",
							kind: "readonly",
							value: "/dev/circl/postgres/password",
							mono: true,
							span: "half",
						},
					],
					{ icon: KeyRound, iconTone: "info" },
				),
			]),
		],
	},
	s3: {
		nodeType: "s3",
		name: "Amazon S3",
		displayName: "S3 buckets",
		icon: HardDrive,
		status: "Available",
		statusTone: "success",
		context,
		views: [
			view("setup", "Buckets", [
				section(
					"s3-backend",
					"Backend Bucket",
					[
						{
							id: "s3-bucket",
							label: "Bucket",
							kind: "readonly",
							value: "circl-backend-dev-45fcj",
							mono: true,
						},
						{
							id: "s3-public",
							label: "Public access",
							kind: "toggle",
							value: false,
						},
						{
							id: "s3-additional",
							label: "Additional buckets",
							kind: "tags",
							value: ["uploads", "exports"],
						},
					],
					{ icon: HardDrive, iconTone: "info" },
				),
				section(
					"s3-advanced",
					"Advanced Settings",
					[
						{
							id: "s3-versioning",
							label: "Versioning",
							kind: "toggle",
							value: true,
							span: "half",
						},
						{
							id: "s3-cors",
							label: "CORS",
							kind: "toggle",
							value: false,
							span: "half",
						},
					],
					{ advanced: true },
				),
			]),
		],
	},
	eventbridge: {
		nodeType: "eventbridge",
		name: "Amazon EventBridge",
		displayName: "EventBridge",
		icon: RadioTower,
		status: "Active",
		statusTone: "success",
		context,
		views: [
			view("setup", "Setup", [
				section(
					"eventbridge-bus",
					"Event Bus",
					[
						{
							id: "eventbridge-name",
							label: "Bus",
							kind: "readonly",
							value: "default",
							mono: true,
						},
						{
							id: "eventbridge-rules",
							label: "Managed rules",
							kind: "readonly",
							value: "4",
						},
					],
					{ icon: RadioTower, iconTone: "info" },
				),
			]),
			view("test", "Test Event", [
				section(
					"eventbridge-test",
					"Publish Test Event",
					[
						{
							id: "eventbridge-source",
							label: "Source",
							kind: "text",
							value: "meroku.test",
							mono: true,
							span: "half",
						},
						{
							id: "eventbridge-type",
							label: "Detail type",
							kind: "text",
							value: "TestEvent",
							mono: true,
							span: "half",
						},
						{
							id: "eventbridge-detail",
							label: "Detail JSON",
							kind: "textarea",
							value: '{\n  "message": "hello"\n}',
							mono: true,
						},
					],
					{ icon: TestTube2, iconTone: "warning" },
				),
			]),
		],
	},
	"event-task": {
		nodeType: "event-task",
		name: "order-processor",
		displayName: "Event-driven task",
		icon: Zap,
		status: "Running",
		statusTone: "success",
		context,
		deletable: true,
		views: [
			view("setup", "Setup", [
				section(
					"event-task-rule",
					"Event Rule",
					[
						{
							id: "event-task-name",
							label: "Rule name",
							kind: "text",
							value: "orders-created",
							mono: true,
						},
						{
							id: "event-task-sources",
							label: "Sources",
							kind: "tags",
							value: ["com.circl.orders"],
						},
						{
							id: "event-task-types",
							label: "Detail types",
							kind: "tags",
							value: ["OrderCreated", "OrderUpdated"],
						},
					],
					{ icon: Zap, iconTone: "warning" },
				),
				...serviceSetupSections(
					"event-task",
					"123456789.dkr.ecr.ap-southeast-2.amazonaws.com/order-processor:latest",
					false,
				).filter((item) => item.id !== "event-task-runtime"),
				section(
					"event-task-resources",
					"Task Resources",
					[
						{
							id: "event-task-cpu",
							label: "CPU",
							kind: "select",
							value: "512",
							options: cpuOptions,
							span: "half",
						},
						{
							id: "event-task-memory",
							label: "Memory",
							kind: "select",
							value: "1024",
							options: memoryOptions,
							span: "half",
						},
					],
					{ icon: Gauge },
				),
			]),
			view("operations", "Operations", [
				section(
					"event-task-test",
					"Test & Logs",
					[
						{
							id: "event-task-test-source",
							label: "Test source",
							kind: "text",
							value: "com.circl.orders",
							mono: true,
						},
						{
							id: "event-task-logs",
							label: "Log group",
							kind: "readonly",
							value: "/ecs/circl/order-processor",
							mono: true,
						},
					],
					{ icon: Send, iconTone: "warning" },
				),
			]),
			serviceAccess("event-task"),
		],
	},
	"scheduled-task": {
		nodeType: "scheduled-task",
		name: "reconciliation",
		displayName: "Scheduled task",
		icon: AlarmClock,
		status: "Scheduled",
		statusTone: "info",
		context,
		deletable: true,
		views: [
			view("setup", "Setup", [
				section(
					"scheduled-schedule",
					"Schedule",
					[
						{
							id: "scheduled-expression",
							label: "Expression",
							kind: "text",
							value: "cron(0 3 * * ? *)",
							mono: true,
						},
						{
							id: "scheduled-timezone",
							label: "Timezone",
							kind: "select",
							value: "UTC",
							options: [
								{ label: "UTC", value: "UTC" },
								{ label: "Australia/Sydney", value: "Australia/Sydney" },
							],
						},
					],
					{ icon: AlarmClock, iconTone: "info" },
				),
				...serviceSetupSections(
					"scheduled-task",
					"123456789.dkr.ecr.ap-southeast-2.amazonaws.com/reconciliation:latest",
					false,
				).filter(
					(item) =>
						!["scheduled-task-runtime", "scheduled-task-advanced"].includes(
							item.id,
						),
				),
				section(
					"scheduled-resources",
					"Task Resources",
					[
						{
							id: "scheduled-cpu",
							label: "CPU",
							kind: "select",
							value: "256",
							options: cpuOptions,
							span: "half",
						},
						{
							id: "scheduled-memory",
							label: "Memory",
							kind: "select",
							value: "512",
							options: memoryOptions,
							span: "half",
						},
					],
					{ icon: Gauge },
				),
			]),
			serviceOperations("scheduled-task"),
			serviceAccess("scheduled-task"),
		],
	},
	sns: {
		nodeType: "sns",
		name: "Amazon SNS",
		displayName: "SNS notifications",
		icon: Send,
		status: "Enabled",
		statusTone: "success",
		context,
		views: [
			view("setup", "Setup", [
				section(
					"sns-main",
					"Mobile Push",
					[
						{
							id: "sns-enabled",
							label: "Firebase notifications",
							kind: "toggle",
							value: true,
						},
						{
							id: "sns-name",
							label: "Platform application",
							kind: "text",
							value: "circl-fcm-dev",
							mono: true,
						},
						{
							id: "sns-platform",
							label: "Platform",
							kind: "select",
							value: "GCM",
							options: [
								{ label: "Firebase / GCM", value: "GCM" },
								{ label: "Apple / APNS", value: "APNS" },
							],
							span: "half",
						},
						{
							id: "sns-key",
							label: "Server key",
							kind: "secret",
							value: "firebase-server-key",
							span: "half",
						},
					],
					{ icon: Send, iconTone: "success" },
				),
			]),
		],
	},
	sqs: {
		nodeType: "sqs",
		name: "Amazon SQS",
		displayName: "SQS queue",
		icon: Layers3,
		status: "Active",
		statusTone: "success",
		context,
		views: [
			view("setup", "Setup", [
				section(
					"sqs-main",
					"Queue",
					[
						{
							id: "sqs-enabled",
							label: "Enable queue",
							kind: "toggle",
							value: true,
						},
						{
							id: "sqs-name",
							label: "Queue name",
							kind: "text",
							value: "jobs",
							mono: true,
						},
						{
							id: "sqs-visibility",
							label: "Visibility timeout",
							kind: "number",
							value: 30,
							unit: "sec",
							span: "half",
						},
						{
							id: "sqs-retention",
							label: "Retention",
							kind: "number",
							value: 4,
							unit: "days",
							span: "half",
						},
						{
							id: "sqs-fifo",
							label: "FIFO queue",
							kind: "toggle",
							value: false,
						},
					],
					{ icon: Layers3, iconTone: "info" },
				),
				section(
					"sqs-details",
					"Generated Details",
					[
						{
							id: "sqs-url",
							label: "Queue URL",
							kind: "readonly",
							value:
								"https://sqs.ap-southeast-2.amazonaws.com/123456789012/circl-dev-jobs",
							mono: true,
						},
					],
					{ advanced: true },
				),
			]),
		],
	},
	ses: {
		nodeType: "ses",
		name: "Amazon SES",
		displayName: "Simple Email Service",
		icon: Mail,
		status: "Verified",
		statusTone: "success",
		context,
		views: [
			view("setup", "Setup", [
				section(
					"ses-domain",
					"Email Domain",
					[
						{
							id: "ses-enabled",
							label: "Enable SES",
							kind: "toggle",
							value: true,
						},
						{
							id: "ses-domain-name",
							label: "Domain",
							kind: "text",
							value: "mail.coretechx.dev",
							mono: true,
						},
						{
							id: "ses-test",
							label: "Test recipients",
							kind: "tags",
							value: ["ops@coretechx.dev"],
						},
					],
					{ icon: Mail, iconTone: "success" },
				),
			]),
			view("status", "Status", [
				section(
					"ses-status",
					"Verification",
					[
						{
							id: "ses-domain-status",
							label: "Domain identity",
							kind: "status",
							value: "Verified",
						},
						{
							id: "ses-dkim",
							label: "DKIM",
							kind: "status",
							value: "Verified",
						},
					],
					{ icon: Activity, iconTone: "success" },
				),
			]),
			view("send", "Send Test", [
				section(
					"ses-send",
					"Send Test Email",
					[
						{
							id: "ses-from",
							label: "From",
							kind: "text",
							value: "hello@mail.coretechx.dev",
							mono: true,
						},
						{
							id: "ses-to",
							label: "To",
							kind: "text",
							value: "ops@coretechx.dev",
							mono: true,
						},
					],
					{ icon: Send, iconTone: "info" },
				),
			]),
		],
	},
	cloudwatch: {
		nodeType: "cloudwatch",
		name: "Amazon CloudWatch",
		displayName: "CloudWatch",
		icon: Cloud,
		status: "Collecting",
		statusTone: "success",
		context,
		views: [
			view("details", "Details", [
				section(
					"cloudwatch-main",
					"Logs & Metrics",
					[
						{
							id: "cloudwatch-group",
							label: "Primary log group",
							kind: "readonly",
							value: "/ecs/circl/backend/dev",
							mono: true,
						},
						{
							id: "cloudwatch-retention",
							label: "Log retention",
							kind: "readonly",
							value: "30 days",
						},
						{
							id: "cloudwatch-metrics",
							label: "Container Insights",
							kind: "status",
							value: "Active",
						},
					],
					{ icon: Cloud, iconTone: "success" },
				),
			]),
		],
	},
	xray: {
		nodeType: "xray",
		name: "AWS X-Ray",
		displayName: "X-Ray tracing",
		icon: Activity,
		status: "Active",
		statusTone: "success",
		context,
		views: [
			view("setup", "Setup", [
				section(
					"xray-main",
					"Distributed Tracing",
					[
						{
							id: "xray-enabled",
							label: "X-Ray sidecar",
							kind: "toggle",
							value: true,
							description: "Run the ADOT collector",
						},
						{
							id: "xray-port",
							label: "Daemon endpoint",
							kind: "readonly",
							value: "localhost:2000/UDP",
							mono: true,
							span: "half",
						},
						{
							id: "xray-otel",
							label: "OpenTelemetry",
							kind: "readonly",
							value: "4317/TCP · 4318/TCP",
							mono: true,
							span: "half",
						},
						{
							id: "xray-sampling",
							label: "Sampling rate",
							kind: "number",
							value: 10,
							unit: "%",
							span: "half",
						},
					],
					{ icon: Activity, iconTone: "info" },
				),
			]),
		],
	},
	"secrets-manager": {
		nodeType: "secrets-manager",
		name: "Parameter Store",
		displayName: "SSM Parameter Store",
		icon: KeyRound,
		status: "Available",
		statusTone: "success",
		context,
		views: [
			view("setup", "Parameters", [
				section(
					"parameters-main",
					"Service Parameters",
					[
						{
							id: "parameters-prefix",
							label: "Prefix",
							kind: "readonly",
							value: "/dev/circl/backend/",
							mono: true,
						},
						{
							id: "parameters-values",
							label: "Parameters",
							kind: "key-value",
							value: [
								kv("parameter-db", "db_password", "retrieved-secret", {
									secret: true,
								}),
								kv("parameter-stripe", "stripe_secret_key", "not retrieved", {
									secret: true,
								}),
							],
						},
					],
					{ icon: KeyRound, iconTone: "info" },
				),
			]),
			view("details", "Description", [
				section(
					"parameters-help",
					"Runtime Mapping",
					[
						{
							id: "parameters-mode",
							label: "Injection",
							kind: "readonly",
							value: "ECS task secrets",
						},
						{
							id: "parameters-kms",
							label: "Encryption",
							kind: "readonly",
							value: "AWS managed KMS key",
						},
					],
					{ icon: FolderKey },
				),
			]),
		],
	},
	efs: {
		nodeType: "efs",
		name: "Amazon EFS",
		displayName: "Elastic File System",
		icon: HardDrive,
		status: "Mounted",
		statusTone: "success",
		context,
		views: [
			view("setup", "Setup", [
				section(
					"efs-main",
					"Shared Volumes",
					[
						{
							id: "efs-encrypted",
							label: "Encryption at rest",
							kind: "toggle",
							value: true,
						},
						{
							id: "efs-name",
							label: "Volume name",
							kind: "text",
							value: "shared-assets",
							mono: true,
							span: "half",
						},
						{
							id: "efs-path",
							label: "Container path",
							kind: "text",
							value: "/mnt/assets",
							mono: true,
							span: "half",
						},
						{
							id: "efs-id",
							label: "File system",
							kind: "readonly",
							value: "fs-01example",
							mono: true,
						},
					],
					{ icon: HardDrive, iconTone: "success" },
				),
			]),
		],
	},
	appsync: {
		nodeType: "appsync",
		name: "AWS AppSync",
		displayName: "AppSync GraphQL API",
		icon: Share2,
		status: "Active",
		statusTone: "success",
		context,
		views: [
			view("setup", "Setup", [
				section(
					"appsync-main",
					"GraphQL API",
					[
						{
							id: "appsync-enabled",
							label: "Enable AppSync",
							kind: "toggle",
							value: true,
						},
						{
							id: "appsync-schema",
							label: "Schema file",
							kind: "text",
							value: "schema.graphql",
							mono: true,
							span: "half",
						},
						{
							id: "appsync-auth",
							label: "Auth mode",
							kind: "select",
							value: "lambda",
							options: [
								{ label: "Lambda authorizer", value: "lambda" },
								{ label: "Cognito", value: "cognito" },
								{ label: "OIDC", value: "oidc" },
							],
							span: "half",
						},
						{
							id: "appsync-api-key",
							label: "API key",
							kind: "toggle",
							value: false,
						},
					],
					{ icon: Share2, iconTone: "info" },
				),
				section(
					"appsync-advanced",
					"Advanced Settings",
					[
						{
							id: "appsync-jwks",
							label: "JWKS URI",
							kind: "text",
							value: "",
							placeholder: "https://issuer/.well-known/jwks.json",
							mono: true,
						},
						{
							id: "appsync-resolvers",
							label: "Resolvers",
							kind: "tags",
							value: ["Query.getSession", "Mutation.publish"],
						},
					],
					{ advanced: true },
				),
			]),
		],
	},
	amplify: {
		nodeType: "amplify",
		name: "dashboard",
		displayName: "AWS Amplify app",
		icon: CloudCog,
		status: "Deployed",
		statusTone: "success",
		context,
		deletable: true,
		views: [
			view("setup", "Setup", [
				section(
					"amplify-main",
					"Application",
					[
						{
							id: "amplify-repo",
							label: "GitHub repository",
							kind: "text",
							value: "madappgang/circl-dashboard",
							mono: true,
						},
						{
							id: "amplify-branch",
							label: "Production branch",
							kind: "text",
							value: "main",
							mono: true,
							span: "half",
						},
						{
							id: "amplify-autobuild",
							label: "Auto build",
							kind: "toggle",
							value: true,
							span: "half",
						},
						{
							id: "amplify-domain",
							label: "Custom domain",
							kind: "text",
							value: "app.dev.coretechx.dev",
							mono: true,
						},
						{
							id: "amplify-env",
							label: "Environment",
							kind: "key-value",
							value: [
								kv(
									"amplify-env-api",
									"VITE_API_URL",
									"https://api.dev.coretechx.dev",
								),
							],
						},
					],
					{ icon: CloudCog, iconTone: "info" },
				),
			]),
			view("operations", "Operations", [
				section(
					"amplify-builds",
					"Build & Branches",
					[
						{
							id: "amplify-build",
							label: "Latest build",
							kind: "status",
							value: "Succeeded",
						},
						{
							id: "amplify-branches",
							label: "Branches",
							kind: "tags",
							value: ["main · production", "develop · development"],
						},
					],
					{ icon: Workflow, iconTone: "success" },
				),
			]),
		],
	},
	cloudfront: {
		nodeType: "cloudfront",
		name: "public-assets",
		displayName: "CloudFront distribution",
		icon: Cloud,
		status: "Deployed",
		statusTone: "success",
		context,
		deletable: true,
		views: [
			view("setup", "Setup", [
				section(
					"cloudfront-main",
					"Distribution",
					[
						{
							id: "cloudfront-enabled",
							label: "Enable distribution",
							kind: "toggle",
							value: true,
						},
						{
							id: "cloudfront-domains",
							label: "Domain aliases",
							kind: "tags",
							value: ["assets.dev.coretechx.dev"],
						},
						{
							id: "cloudfront-origin",
							label: "Primary origin",
							kind: "text",
							value: "circl-assets.s3.ap-southeast-2.amazonaws.com",
							mono: true,
						},
						{
							id: "cloudfront-spa",
							label: "SPA routing",
							kind: "toggle",
							value: false,
							span: "half",
						},
						{
							id: "cloudfront-price",
							label: "Price class",
							kind: "select",
							value: "PriceClass_100",
							options: [
								{ label: "US, Canada, Europe", value: "PriceClass_100" },
								{ label: "Most regions", value: "PriceClass_200" },
								{ label: "All edge locations", value: "PriceClass_All" },
							],
							span: "half",
						},
					],
					{ icon: Cloud, iconTone: "info" },
				),
				section(
					"cloudfront-advanced",
					"Advanced Settings",
					[
						{
							id: "cloudfront-root",
							label: "Default root object",
							kind: "text",
							value: "index.html",
							mono: true,
							span: "half",
						},
						{
							id: "cloudfront-logging",
							label: "Access logging",
							kind: "toggle",
							value: true,
							span: "half",
						},
					],
					{ advanced: true },
				),
			]),
			view("details", "Behaviors", [
				section(
					"cloudfront-behaviors",
					"Cache Behaviors",
					[
						{
							id: "cloudfront-default",
							label: "Default",
							kind: "readonly",
							value: "/* · CachingOptimized",
							mono: true,
						},
					],
					{ icon: Route },
				),
			]),
		],
	},
	"custom-terraform": {
		nodeType: "custom-terraform",
		name: "Custom Terraform",
		displayName: "Custom Terraform",
		icon: FileCode2,
		status: "3 files",
		statusTone: "info",
		context,
		views: [
			view("setup", "Editor", [
				section(
					"terraform-file",
					"Terraform File",
					[
						{
							id: "terraform-scope",
							label: "Scope",
							kind: "select",
							value: "environment",
							options: [
								{ label: "Environment", value: "environment" },
								{ label: "Shared", value: "shared" },
							],
							span: "half",
						},
						{
							id: "terraform-path",
							label: "Path",
							kind: "text",
							value: "post/observability.tf",
							mono: true,
							span: "half",
						},
						{
							id: "terraform-code",
							label: "HCL",
							kind: "textarea",
							value:
								'resource "aws_cloudwatch_log_group" "custom" {\n  name = "/custom/circl"\n}',
							mono: true,
							rows: 8,
						},
					],
					{ icon: FileCode2, iconTone: "info" },
				),
			]),
			view("details", "Modules", [
				section(
					"terraform-modules",
					"Detected Modules",
					[
						{
							id: "terraform-module-list",
							label: "References",
							kind: "tags",
							value: ["module.workloads", "module.vpc"],
						},
					],
					{ icon: Box },
				),
			]),
		],
	},
	alarms: {
		nodeType: "alarms",
		name: "Alarm rules",
		displayName: "CloudWatch alarms",
		icon: AlarmClock,
		status: "Not implemented",
		statusTone: "neutral",
		context,
		views: [
			view("details", "Details", [
				section(
					"alarms-disabled",
					"Alarm Rules",
					[
						{
							id: "alarms-state",
							label: "Availability",
							kind: "readonly",
							value: "Not implemented in the current Meroku schema",
						},
					],
					{ icon: AlarmClock },
				),
			]),
		],
	},
};

export const PROPERTY_INVENTORY: PropertyInventoryEntry[] = [
	{
		type: "client-app",
		component: "Generic read-only",
		currentViews: ["Settings", "Logs", "Metrics", "Environment", "Connections"],
		targetViews: ["Details"],
		coverage: "read-only",
	},
	{
		type: "github",
		component: "GitHubNodeProperties",
		currentViews: ["Settings", "Example"],
		targetViews: ["Setup", "Workflow"],
		coverage: "editable",
	},
	{
		type: "api-gateway",
		component: "APIGatewayProperties",
		currentViews: ["Settings"],
		targetViews: ["Setup", "Routes"],
		coverage: "read-only",
	},
	{
		type: "alb",
		component: "ALBNodeProperties",
		currentViews: ["Settings", "Routing"],
		targetViews: ["Setup", "Routing"],
		coverage: "editable",
	},
	{
		type: "route53",
		component: "Route53NodeProperties",
		currentViews: ["Settings", "DNS"],
		targetViews: ["Setup", "DNS Records"],
		coverage: "editable",
	},
	{
		type: "ecs",
		component: "ECSNodeProperties",
		currentViews: [
			"Settings",
			"Notifications",
			"Cluster",
			"Network",
			"Services",
		],
		targetViews: ["Setup", "Details"],
		coverage: "editable",
	},
	{
		type: "backend",
		component: "BackendServiceProperties",
		currentViews: [
			"Settings",
			"CI/CD",
			"Scaling",
			"X-Ray",
			"SSH",
			"Env Vars",
			"Parameters",
			"S3 Buckets",
			"SNS",
			"IAM",
			"Logs",
			"CloudWatch",
			"Alerts",
		],
		targetViews: ["Setup", "Operations", "Access"],
		coverage: "editable",
	},
	{
		type: "service",
		component: "ServiceProperties",
		currentViews: [
			"Settings",
			"CI/CD",
			"Scaling",
			"X-Ray",
			"SSH",
			"Env Vars",
			"Parameters",
			"IAM",
			"Logs",
			"CloudWatch",
		],
		targetViews: ["Setup", "Operations", "Access"],
		coverage: "editable",
	},
	{
		type: "ecr",
		component: "ECRNodeProperties",
		currentViews: ["Settings", "Repos", "Push"],
		targetViews: ["Setup", "Repositories", "Push"],
		coverage: "editable",
	},
	{
		type: "postgres",
		component: "PostgresNodeProperties",
		currentViews: ["Settings", "Connection"],
		targetViews: ["Setup", "Connection"],
		coverage: "editable",
	},
	{
		type: "s3",
		component: "S3NodeProperties",
		currentViews: ["Buckets"],
		targetViews: ["Buckets"],
		coverage: "editable",
	},
	{
		type: "eventbridge",
		component: "EventBridgeTestEvent",
		currentViews: ["Test Event"],
		targetViews: ["Setup", "Test Event"],
		coverage: "editable",
	},
	{
		type: "event-task",
		component: "EventTaskProperties",
		currentViews: [
			"Settings",
			"CI/CD",
			"Test Event",
			"Env Vars",
			"Parameters",
			"IAM",
			"CloudWatch",
			"Logs",
		],
		targetViews: ["Setup", "Operations", "Access"],
		coverage: "editable",
	},
	{
		type: "scheduled-task",
		component: "ScheduledTaskProperties",
		currentViews: [
			"Settings",
			"CI/CD",
			"Env Vars",
			"Parameters",
			"IAM",
			"CloudWatch",
			"Logs",
		],
		targetViews: ["Setup", "Operations", "Access"],
		coverage: "editable",
	},
	{
		type: "sns",
		component: "SNSNodeProperties",
		currentViews: ["Settings"],
		targetViews: ["Setup"],
		coverage: "editable",
	},
	{
		type: "sqs",
		component: "SQSNodeProperties",
		currentViews: ["Settings"],
		targetViews: ["Setup"],
		coverage: "editable",
	},
	{
		type: "ses",
		component: "SESNodeProperties",
		currentViews: ["Settings", "Status", "Send Email"],
		targetViews: ["Setup", "Status", "Send Test"],
		coverage: "editable",
	},
	{
		type: "cloudwatch",
		component: "BackendCloudWatch",
		currentViews: ["CloudWatch"],
		targetViews: ["Details"],
		coverage: "read-only",
	},
	{
		type: "xray",
		component: "BackendXRayConfiguration",
		currentViews: ["X-Ray"],
		targetViews: ["Setup"],
		coverage: "editable",
	},
	{
		type: "secrets-manager",
		component: "ParameterStoreNodeProperties",
		currentViews: ["Settings", "Description"],
		targetViews: ["Parameters", "Description"],
		coverage: "editable",
	},
	{
		type: "efs",
		component: "Generic properties",
		currentViews: ["Settings", "Logs", "Metrics", "Environment", "Connections"],
		targetViews: ["Setup"],
		coverage: "editable",
	},
	{
		type: "appsync",
		component: "Generic properties",
		currentViews: ["Settings", "Logs", "Metrics", "Environment", "Connections"],
		targetViews: ["Setup"],
		coverage: "editable",
	},
	{
		type: "amplify",
		component: "AmplifyNodeProperties",
		currentViews: ["Settings", "CI/CD", "Branches", "Build", "Domain"],
		targetViews: ["Setup", "Operations"],
		coverage: "editable",
	},
	{
		type: "cloudfront",
		component: "CloudFrontNodeProperties",
		currentViews: ["Settings", "Logs", "Metrics", "Environment", "Connections"],
		targetViews: ["Setup", "Behaviors"],
		coverage: "editable",
	},
	{
		type: "custom-terraform",
		component: "CustomTerraformNodeProperties",
		currentViews: ["Editor"],
		targetViews: ["Editor", "Modules"],
		coverage: "editable",
	},
	{
		type: "alarms",
		component: "Generic properties",
		currentViews: ["Settings"],
		targetViews: ["Details"],
		coverage: "disabled",
	},
];

export const PROPERTY_NODE_ORDER = PROPERTY_INVENTORY.map(
	(entry) => entry.type,
);
