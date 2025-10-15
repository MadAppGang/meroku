export interface InfrastructureConfig {
	project: string;
	env: string;
	is_prod?: boolean;
	region: string;
	state_bucket?: string;
	modules?: string;
	state_file?: string;
	ecr_account_id?: string;
	ecr_account_region?: string;
	slack_deployment_webhook?: string;

	// Backend configuration
	health_endpoint?: string;
	backend_health_endpoint?: string;
	backend_external_docker_image?: string;
	image_bucket_postfix?: string;
	bucket_public?: boolean;
	backend_container_command?: string | string[];
	backend_image_port?: number;
	xray_enabled?: boolean;
	setup_FCM_SNS?: boolean;
	backend_env?: Array<{
		name: string;
		value: string;
	}>;

	// Services
	services?: Array<{
		name: string;
		remote_access?: boolean;
		container_port?: number;
		host_port?: number;
		cpu?: number;
		memory?: number;
		xray_enabled?: boolean;
		env_vars?: {
			name: string;
			value: string;
		};
	}>;

	// Domain configuration
	setup_domain?: boolean;
	domain?: string;

	// Database configuration
	setup_postgres?: boolean;
	pg_db_name?: string;
	pg_username?: string;
	pgadmin?: boolean;
	pgadmin_email?: string;
	pg_public?: boolean;
	pg_engine?: string;

	// Cognito configuration
	setup_cognito?: boolean;
	auto_verified_attributes?: string[];
	enable_web_client?: boolean;
	enable_dashboard_client?: boolean;
	dashboard_callback_urls?: string[];
	enable_user_pool_domain?: boolean;
	user_pool_domain_prefix?: string;
	allow_backend_task_to_confirm_signup?: boolean;

	// Scheduled tasks
	scheduled_tasks?: Array<{
		name: string;
		schedule: string;
	}>;

	// Event tasks
	event_tasks?: Array<{
		name: string;
		rule_name: string;
		sources: string[];
		detail_types: string[];
	}>;

	// SES configuration
	setup_ses?: boolean;
	ses_domain?: string;
	ses_test_emails?: string[];

	// EFS configuration
	efs?: Array<{
		name: string;
	}>;
}

// AWS Regions list
export const AWS_REGIONS = [
	{ value: "us-east-1", label: "US East (N. Virginia)" },
	{ value: "us-east-2", label: "US East (Ohio)" },
	{ value: "us-west-1", label: "US West (N. California)" },
	{ value: "us-west-2", label: "US West (Oregon)" },
	{ value: "af-south-1", label: "Africa (Cape Town)" },
	{ value: "ap-east-1", label: "Asia Pacific (Hong Kong)" },
	{ value: "ap-south-1", label: "Asia Pacific (Mumbai)" },
	{ value: "ap-south-2", label: "Asia Pacific (Hyderabad)" },
	{ value: "ap-southeast-1", label: "Asia Pacific (Singapore)" },
	{ value: "ap-southeast-2", label: "Asia Pacific (Sydney)" },
	{ value: "ap-southeast-3", label: "Asia Pacific (Jakarta)" },
	{ value: "ap-southeast-4", label: "Asia Pacific (Melbourne)" },
	{ value: "ap-northeast-1", label: "Asia Pacific (Tokyo)" },
	{ value: "ap-northeast-2", label: "Asia Pacific (Seoul)" },
	{ value: "ap-northeast-3", label: "Asia Pacific (Osaka)" },
	{ value: "ca-central-1", label: "Canada (Central)" },
	{ value: "ca-west-1", label: "Canada West (Calgary)" },
	{ value: "eu-central-1", label: "Europe (Frankfurt)" },
	{ value: "eu-central-2", label: "Europe (Zurich)" },
	{ value: "eu-west-1", label: "Europe (Ireland)" },
	{ value: "eu-west-2", label: "Europe (London)" },
	{ value: "eu-west-3", label: "Europe (Paris)" },
	{ value: "eu-south-1", label: "Europe (Milan)" },
	{ value: "eu-south-2", label: "Europe (Spain)" },
	{ value: "eu-north-1", label: "Europe (Stockholm)" },
	{ value: "il-central-1", label: "Israel (Tel Aviv)" },
	{ value: "me-south-1", label: "Middle East (Bahrain)" },
	{ value: "me-central-1", label: "Middle East (UAE)" },
	{ value: "sa-east-1", label: "South America (São Paulo)" },
] as const;
