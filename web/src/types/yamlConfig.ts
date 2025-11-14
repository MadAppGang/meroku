/**
 * ECR Configuration (Schema v10)
 * Defines how ECR repositories are managed for services, event processors, and scheduled tasks
 */
export interface ECRConfig {
	mode?: "create_ecr" | "manual_repo" | "use_existing";
	repository_uri?: string; // For manual_repo mode
	source_service_name?: string; // For use_existing mode
	source_service_type?: "services" | "event_processor_tasks" | "scheduled_tasks"; // For use_existing mode
}

/**
 * Complete YAML configuration interface based on YAML_SPECIFICATION.md
 */
export interface YamlInfrastructureConfig {
	// Core Settings (Required)
	project: string;
	env: string;
	is_prod: boolean;
	region: string;
	account_id?: string;
	state_bucket: string;
	state_file: string;

	// VPC Configuration
	use_default_vpc?: boolean;
	vpc_cidr?: string;

	// API Configuration
	api_domain?: string;

	// Optional ECR configuration
	ecr_strategy?: "local" | "cross_account";
	ecr_account_id?: string;
	ecr_account_region?: string;
	ecr_trusted_accounts?: Array<{
		account_id: string;
		env: string;
		region: string;
	}>;

	// Workload Configuration
	workload?: {
		// Basic backend configuration
		backend_health_endpoint?: string;
		backend_external_docker_image?: string;
		backend_container_command?: string[];
		backend_image_port?: number;
		backend_remote_access?: boolean;
		backend_cpu?: string;
		backend_memory?: string;
		backend_desired_count?: number;
		backend_autoscaling_enabled?: boolean;
		backend_autoscaling_min_capacity?: number;
		backend_autoscaling_max_capacity?: number;

		// S3 bucket configuration
		bucket_postfix?: string;
		bucket_public?: boolean;

		// Environment configuration
		backend_env_variables?: Record<string, string>;
		env_files_s3?: Array<{
			bucket: string;
			key: string;
		}>;

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
		policy?: Array<{
			actions: string[];
			resources: string[];
		}>;

		// EFS mounts
		efs?: Array<{
			name: string;
			mount_point: string;
		}>;

		// ALB configuration
		backend_alb_domain_name?: string;
	};

	// Domain Configuration
	domain?: {
		enabled: boolean;
		create_domain_zone?: boolean;
		domain_name?: string;
		api_domain_prefix?: string;
		add_env_domain_prefix?: boolean;
		zone_id?: string;
		root_zone_id?: string;
		root_account_id?: string;
		is_dns_root?: boolean;
		dns_root_account_id?: string;
		delegation_role_arn?: string;
	};

	// Database Configuration
	postgres?: {
		enabled: boolean;
		dbname?: string;
		username?: string;
		public_access?: boolean;
		engine_version?: string;
		aurora?: boolean;
		min_capacity?: number;
		max_capacity?: number;
		// RDS-specific configuration (when aurora is false)
		instance_class?: string; // db.t4g.micro, db.m6i.large, etc.
		allocated_storage?: number; // 20-65536 GB
		storage_type?: string; // gp3 (only option)
		multi_az?: boolean; // High availability deployment
		storage_encrypted?: boolean; // Encryption at rest
		deletion_protection?: boolean; // Prevent accidental deletion
		skip_final_snapshot?: boolean; // Snapshot on delete
	};

	// Authentication Configuration
	cognito?: {
		enabled: boolean;
		enable_web_client?: boolean;
		enable_dashboard_client?: boolean;
		dashboard_callback_urls?: string[];
		enable_user_pool_domain?: boolean;
		user_pool_domain_prefix?: string;
		backend_confirm_signup?: boolean;
		auto_verified_attributes?: string[];
	};

	// Email Service Configuration
	ses?: {
		enabled: boolean;
		domain_name?: string;
		test_emails?: string[];
	};

	// Message Queue Configuration
	sqs?: {
		enabled: boolean;
		name?: string;
	};

	// File Storage Configuration
	efs?: Array<{
		name: string;
		path: string;
	}>;

	// Load Balancer Configuration
	alb?: {
		enabled: boolean;
	};

	// Scheduled Tasks
	scheduled_tasks?: Array<{
		name: string;
		schedule: string;
		docker_image?: string;
		container_command?: string;
		cpu?: number;
		memory?: number;
		environment_variables?: Record<string, string>;
		ecr_config?: ECRConfig;
	}>;

	// Event-driven Tasks
	event_processor_tasks?: Array<{
		name: string;
		rule_name: string;
		detail_types: string[];
		sources: string[];
		docker_image?: string;
		container_command?: string[];
		cpu?: number;
		memory?: number;
		environment_variables?: Record<string, string>;
		ecr_config?: ECRConfig;
	}>;

	// GraphQL API Configuration
	pubsub_appsync?: {
		enabled: boolean;
		schema?: boolean;
		auth_lambda?: boolean;
		resolvers?: boolean;
	};

	// Additional Services
	services?: Array<{
		name: string;
		docker_image?: string;
		container_command?: string[];
		container_port?: number;
		host_port?: number;
		cpu?: number;
		memory?: number;
		desired_count?: number;
		remote_access?: boolean;
		xray_enabled?: boolean;
		essential?: boolean;
		public_access?: boolean;
		health_check_path?: string;
		api_domain_prefix?: string;
		env_vars?: Record<string, string>;
		environment_variables?: Record<string, string>;
		env_variables?: Array<{
			name: string;
			value: string;
		}>;
		env_files_s3?: Array<{
			bucket: string;
			key: string;
		}>;
		ecr_config?: ECRConfig;
	}>;

	// S3 Buckets
	buckets?: Array<{
		name: string;
		public?: boolean;
		versioning?: boolean;
		cors_rules?: Array<{
			allowed_headers?: string[];
			allowed_methods?: string[];
			allowed_origins?: string[];
			expose_headers?: string[];
			max_age_seconds?: number;
		}>;
	}>;

	// AWS Amplify Apps
	amplify_apps?: Array<{
		name: string;
		github_repository: string;
		branches: Array<{
			name: string;
			stage?: "PRODUCTION" | "DEVELOPMENT" | "BETA" | "EXPERIMENTAL";
			enable_auto_build?: boolean;
			enable_pull_request_preview?: boolean;
			environment_variables?: Record<string, string>;
			custom_subdomains?: string[];
		}>;
		subdomain_prefix?: string;     // NEW: Auto-constructs domain using base_domain
		custom_domain?: string;         // For manual override (edge cases)
		environment_variables?: Record<string, string>; // App-level env vars
	}>;
}
