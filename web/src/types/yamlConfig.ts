/**
 * ECR Configuration (Schema v10)
 * Defines how ECR repositories are managed for services, event processors, and scheduled tasks
 */
export interface ECRConfig {
	mode?: "create_ecr" | "manual_repo" | "use_existing";
	repository_uri?: string; // For manual_repo mode
	source_service_name?: string; // For use_existing mode
	source_service_type?:
		| "services"
		| "event_processor_tasks"
		| "scheduled_tasks"; // For use_existing mode
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
		/**
		 * CI/CD auto-deploy policy for the backend (schema v22). Absent means
		 * true. A manual deploy always works regardless.
		 */
		backend_auto_deploy?: boolean;

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
		// Additional domains for CloudFront and other services
		additional_domains?: AdditionalDomain[];
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
		iam_database_authentication_enabled?: boolean; // Enable IAM authentication
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
	ses?: SESConfig;

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
		enabled?: boolean;
		/**
		 * CI/CD auto-deploy policy (schema v22). Absent means true.
		 *
		 * Distinct from `enabled`: `enabled` decides whether the task exists in
		 * AWS at all, `auto_deploy` decides whether a new image may redeploy it
		 * without anyone asking. Outside `dev` no automatic trigger reaches a
		 * scheduled task in the first place, so `true` there enables only the
		 * manual path.
		 */
		auto_deploy?: boolean;
		schedule: string;
		docker_image?: string;
		container_command?: string;
		cpu?: number;
		memory?: number;
		environment_variables?: Record<string, string>;
		ecr_config?: ECRConfig;
	}>;

	// Event-driven Tasks
	event_processor_tasks?: Array<EventProcessorTask>;

	// GraphQL API Configuration
	pubsub_appsync?: {
		enabled: boolean;
		schema?: boolean;
		/**
		 * Package a project-supplied authorizer from custom/appsync/auth_lambda
		 * instead of the one bundled with the module. Only has an effect when
		 * auth_mode is "lambda".
		 */
		auth_lambda?: boolean;
		resolvers?: boolean;

		/**
		 * How AppSync authenticates callers (schema v23). Absent means "lambda",
		 * which is what the module hardcoded before this field existed.
		 *
		 * - "cognito": AWS validates the token against this environment's Cognito
		 *   user pool. Requires cognito.enabled. No Lambda runs.
		 * - "oidc": AWS validates the token against oidc_issuer's discovery
		 *   document and JWKS. No Lambda runs.
		 * - "lambda": the bundled authorizer verifies RS256 JWTs against
		 *   jwks_uri. The only mode that can check claims beyond iss/aud, and it
		 *   puts a Lambda invocation on the request path.
		 */
		auth_mode?: "cognito" | "oidc" | "lambda";

		/**
		 * Attach an API_KEY provider alongside auth_mode (schema v23).
		 *
		 * Defaults to false. An API key BYPASSES auth_mode entirely: whoever
		 * holds it reaches every resolver without presenting a token. Existing
		 * environments were migrated to true to preserve a key they already had.
		 */
		api_key_enabled?: boolean;

		/**
		 * cognito mode: which app clients of the pool are accepted, as a
		 * pipe-separated list ("1F4G9H|1J6L4B").
		 *
		 * Unset accepts EVERY app client in the pool, and meroku's cognito module
		 * creates web, mobile and dashboard clients on one pool. User pool mode
		 * has no separate audience field, so this is it. Matched against `aud` in
		 * an ID token and `client_id` in an access token.
		 */
		cognito_app_id_client_regex?: string;

		/**
		 * oidc mode: issuer URL of the identity provider. Required in that mode.
		 *
		 * Note that on an API whose only authorization type is OPENID_CONNECT,
		 * AppSync skips comparing the token's `iss` against this value; the
		 * signature is still verified against this issuer's JWKS. Use auth_mode
		 * "lambda" with jwt_issuer if `iss` must be asserted.
		 */
		oidc_issuer?: string;
		/**
		 * oidc mode: the client identifier registered with the provider. This is
		 * AppSync's audience check — matched against `aud`, falling back to
		 * `azp`. Pipe-separated for several clients ("1F4G9H|1J6L4B").
		 */
		oidc_client_id?: string;

		/**
		 * JWKS endpoint whose keys sign the JWTs this API accepts (schema v21).
		 * Required in "lambda" mode — the Terraform module has no default,
		 * because an unset value used to mean "trust a hardcoded third party".
		 * Must be an https:// URL.
		 */
		jwks_uri?: string;
		/** lambda mode: expected `iss` claim. Comma-separated list allowed. */
		jwt_issuer?: string;
		/** lambda mode: expected `aud` claim. Comma-separated list allowed. */
		jwt_audience?: string;
		/**
		 * lambda mode: claims a verified token must carry, checked after
		 * signature, issuer and audience (schema v23).
		 *
		 * Claim name to accepted values; an empty list means "must be present".
		 * Rejected at generate time in any other mode, because neither native
		 * mode can enforce it. For policy claims (role, scope, tenant_id), not
		 * for identity — `sub` belongs in resolver logic, not here.
		 */
		required_claims?: Record<string, string[]>;
	};

	// Additional Services
	services?: Array<{
		name: string;
		enabled?: boolean;
		/**
		 * CI/CD auto-deploy policy (schema v22). Absent means true.
		 *
		 * Distinct from `enabled`: `enabled` decides whether the service exists
		 * in AWS at all, `auto_deploy` decides whether an ECR push, an SSM
		 * change or an S3 env-file write may redeploy it without anyone asking.
		 * A manual deploy always works.
		 */
		auto_deploy?: boolean;
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
		subdomain_prefix?: string;
		custom_domain?: string;
		environment_variables?: Record<string, string>;
		spa_mode?: boolean; // Enable SPA routing (200 rewrite instead of 404-200)
	}>;

	// CloudFront CDN Configuration (Schema v14 -> v15: changed to array)
	cloudfront_distributions?: CloudFrontConfig[];
}

/**
 * CloudFront CDN Configuration
 * Supports multiple origins (S3, Amplify, ALB, custom) with path-based routing
 */
export interface CloudFrontConfig {
	name: string; // Unique identifier for this distribution
	enabled: boolean;
	origins?: CloudFrontOrigin[];
	domain_aliases?: string[]; // e.g., ["*.app.example.com", "app.example.com"]
	additional_zones?: CloudFrontAdditionalZone[]; // Route 53 zones for non-main domain aliases
	cache_behaviors?: CloudFrontCacheBehavior[];
	price_class?: "PriceClass_100" | "PriceClass_200" | "PriceClass_All";
	default_root_object?: string; // e.g., "index.html"
	spa_mode?: boolean; // Enable SPA error handling (404 -> index.html)
	logging?: CloudFrontLogging;
}

/**
 * CloudFront Origin Configuration
 */
export interface CloudFrontOrigin {
	name: string;
	type: "s3" | "amplify" | "alb" | "custom";
	domain_name?: string; // For custom/alb origins, auto-resolved for amplify/s3
	origin_path?: string;
	protocol_policy?: "https-only" | "http-only" | "match-viewer";
	custom_headers?: Record<string, string>;
	// For S3 origins
	bucket_name?: string;
	create_bucket?: boolean; // Create a new S3 bucket for this origin
	use_oac?: boolean; // Use Origin Access Control
	// For Amplify origins
	amplify_app_name?: string;
}

/**
 * CloudFront Cache Behavior (path-based routing)
 */
export interface CloudFrontCacheBehavior {
	path_pattern: string; // e.g., "/api/*"
	origin_name: string; // Reference to origin name
	allowed_methods?: string[];
	cached_methods?: string[];
	forward_query_string?: boolean;
	forward_headers?: string[];
	forward_cookies?: "none" | "whitelist" | "all";
	viewer_protocol_policy?: "redirect-to-https" | "https-only" | "allow-all";
	min_ttl?: number;
	default_ttl?: number;
	max_ttl?: number;
	compress?: boolean;
}

/**
 * CloudFront Logging Configuration
 */
export interface CloudFrontLogging {
	enabled: boolean;
	bucket_name?: string;
	prefix?: string;
	include_cookies?: boolean;
}

/**
 * Additional Domain Configuration
 * For managing additional domains in Route53 (centralized domain management)
 * These domains can be used by CloudFront, services, etc.
 */
export interface AdditionalDomain {
	domain: string; // The domain name (e.g., "otherdomain.com")
	create_zone?: boolean; // Whether to create a new Route 53 zone
	zone_id?: string; // Existing zone ID (if not creating)
	create_certificate?: boolean; // Create ACM certificate (default: true)
}

/**
 * CloudFront Additional Zone Configuration
 * @deprecated Use domain.additional_domains instead for centralized domain management
 * For domain aliases that are not subdomains of the main domain
 */
export interface CloudFrontAdditionalZone {
	domain: string; // The domain name (e.g., "otherdomain.com")
	zone_id?: string; // Existing zone ID (if not creating)
	create_zone?: boolean; // Whether to create a new Route 53 zone
}

/**
 * EventBridge Rule (Schema v13)
 * Defines a single EventBridge rule pattern for event-driven tasks
 */
export interface EventBridgeRule {
	name: string;
	sources: string[];
	detail_types: string[];
}

/**
 * Event Processor Task Configuration
 * Supports both legacy single-rule format and new multi-rule format (Schema v13)
 */
export interface EventProcessorTask {
	name: string;
	enabled?: boolean;
	// New multi-rule support (Schema v13) - preferred format
	rules?: EventBridgeRule[];
	// Legacy single-rule fields (Schema <= 12) - kept for backward compatibility
	rule_name?: string;
	detail_types?: string[];
	sources?: string[];
	// Container configuration
	docker_image?: string;
	container_command?: string[];
	cpu?: number;
	memory?: number;
	environment_variables?: Record<string, string>;
	ecr_config?: ECRConfig;
}

/**
 * SES Email Domain Configuration (Schema v18)
 * Defines a single email domain with optional per-domain settings
 * Note: test_emails are at global SES level (account-wide in AWS)
 */
export interface SESDomain {
	domain: string; // Required: The email domain (e.g., "example.com")
	zone_id?: string; // Optional: Route53 zone ID for automatic DNS record creation
	// Per-domain overrides (optional, defaults to global settings)
	enable_mail_from?: boolean; // Enable custom MAIL FROM domain
	mail_from_subdomain?: string; // Subdomain for MAIL FROM (e.g., "bounce")
	dmarc_policy?: "none" | "quarantine" | "reject"; // DMARC policy
	dmarc_rua_email?: string; // Email for DMARC reports
}

/**
 * SES Configuration (Schema v18)
 * Supports both legacy single domain and new multi-domain format
 */
export interface SESConfig {
	enabled: boolean;
	// New multi-domain support (Schema v17+) - preferred format
	domains?: SESDomain[];
	// Test emails - account-wide for SES sandbox mode (Schema v18)
	test_emails?: string[];
	// Global settings (apply to all domains unless overridden)
	global_enable_mail_from?: boolean;
	global_mail_from_subdomain?: string;
	global_dmarc_policy?: "none" | "quarantine" | "reject";
	global_dmarc_rua_email?: string;
	// Legacy single domain configuration (backward compatibility)
	domain_name?: string;
}
