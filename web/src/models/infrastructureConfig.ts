// Infrastructure Configuration Model based on YAML specification

export interface InfrastructureConfig {
	// Core Settings (Required)
	project: string;
	env: string;
	is_prod: boolean;
	region: string;
	state_bucket: string;
	state_file: string;

	// Optional ECR configuration
	ecr_account_id?: string;
	ecr_account_region?: string;

	// Workload Configuration
	workload: WorkloadConfig;

	// Domain Configuration
	domain?: DomainConfig;

	// Database Configuration
	postgres?: PostgresConfig;

	// Authentication Configuration
	cognito?: CognitoConfig;

	// Email Service Configuration
	ses?: SESConfig;

	// Message Queue Configuration
	sqs?: SQSConfig;

	// File Storage Configuration
	efs?: EFSConfig[];
	buckets?: S3BucketConfig[];

	// Load Balancer Configuration
	alb?: ALBConfig;

	// Scheduled Tasks
	scheduled_tasks?: ScheduledTaskConfig[];

	// Event-Driven Tasks
	event_processor_tasks?: EventProcessorTaskConfig[];

	// GraphQL API Configuration
	pubsub_appsync?: AppSyncConfig;

	// Additional Services
	services?: ServiceConfig[];
}

export interface WorkloadConfig {
	// Basic backend configuration
	backend_health_endpoint?: string;
	backend_external_docker_image?: string;
	backend_container_command?: string[];
	backend_image_port?: number;
	backend_remote_access?: boolean;

	// S3 bucket configuration
	bucket_postfix: string;
	bucket_public?: boolean;

	// Environment configuration
	backend_env_variables?: Record<string, string>;
	env_files_s3?: S3EnvFile[];

	// Monitoring and observability
	xray_enabled?: boolean;

	// Notifications
	setup_fcnsns?: boolean;
	slack_webhook?: string;

	// CI/CD configuration
	enable_github_oidc?: boolean;
	github_oidc_subjects?: string[];

	// Database admin tools
	install_pg_admin?: boolean;
	pg_admin_email?: string;

	// IAM policies
	policy?: IAMPolicy[];

	// EFS mounts
	efs?: EFSMount[];

	// ALB configuration
	backend_alb_domain_name?: string;
}

export interface DomainConfig {
	enabled: boolean;
	create_domain_zone?: boolean;
	domain_name?: string;
	api_domain_prefix?: string;
	add_domain_prefix?: boolean;
}

export interface PostgresConfig {
	enabled: boolean;
	dbname?: string;
	username?: string;
	public_access?: boolean;
	engine_version?: string;
}

export interface CognitoConfig {
	enabled: boolean;
	enable_web_client?: boolean;
	enable_dashboard_client?: boolean;
	dashboard_callback_urls?: string[];
	enable_user_pool_domain?: boolean;
	user_pool_domain_prefix?: string;
	backend_confirm_signup?: boolean;
	auto_verified_attributes?: string[];
}

export interface SESConfig {
	enabled: boolean;
	domain_name?: string;
	test_emails?: string[];
}

export interface SQSConfig {
	enabled: boolean;
	name?: string;
}

export interface EFSConfig {
	name: string;
	path: string;
}

export interface S3BucketConfig {
	name: string;
	public: boolean;
}

export interface ALBConfig {
	enabled: boolean;
}

export interface ScheduledTaskConfig {
	name: string;
	schedule: string;
	docker_image?: string;
	container_command?: string;
	allow_public_access?: boolean;
}

export interface EventProcessorTaskConfig {
	name: string;
	rule_name: string;
	detail_types: string[];
	sources: string[];
	docker_image?: string;
	container_command?: string;
	allow_public_access?: boolean;
}

export interface AppSyncConfig {
	enabled: boolean;
	schema?: boolean;
	auth_lambda?: boolean;
	resolvers?: boolean;
}

export interface ServiceConfig {
	name: string;
	// Container configuration
	docker_image?: string;
	container_command?: string[];
	container_port?: number;
	host_port?: number;

	// Resource allocation
	cpu?: number;
	memory?: number;
	desired_count?: number;

	// Features
	remote_access?: boolean;
	xray_enabled?: boolean;
	essential?: boolean;

	// Environment configuration
	env_vars?: EnvVariable[];
	env_files_s3?: S3EnvFile[];
}

// Helper interfaces
export interface EnvVariable {
	name: string;
	value: string;
}

export interface S3EnvFile {
	bucket: string;
	key: string;
}

export interface IAMPolicy {
	actions: string[];
	resources: string[];
}

export interface EFSMount {
	name: string;
	mount_point: string;
}
