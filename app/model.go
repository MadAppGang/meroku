package main

import (
	"fmt"
	"math/rand"
	"os"
	"strings"

	"gopkg.in/yaml.v2"
)

type Env struct {
	SchemaVersion  int    `yaml:"schema_version,omitempty"`
	Project        string `yaml:"project"`
	Env            string `yaml:"env"`
	IsProd         bool   `yaml:"is_prod"`
	Region         string `yaml:"region"`
	AccountID      string `yaml:"account_id"`
	AWSProfile     string `yaml:"aws_profile"`
	StateBucket    string `yaml:"state_bucket"`
	StateFile      string `yaml:"state_file"`
	StateLockTable string `yaml:"state_lock_table,omitempty"` // DynamoDB table for state locking (Schema v16)
	// VPC Configuration
	UseDefaultVPC bool   `yaml:"use_default_vpc"`
	VPCCIDR       string `yaml:"vpc_cidr,omitempty"` // Optional, VPC module has default
	// ECR Configuration
	ECRStrategy      string `yaml:"ecr_strategy,omitempty"`       // "local" or "cross_account"
	ECRAccountID     string `yaml:"ecr_account_id,omitempty"`     // For cross-account ECR access
	ECRAccountRegion string `yaml:"ecr_account_region,omitempty"` // For cross-account ECR access
	// ECR Trusted Accounts (Schema v8)
	ECRTrustedAccounts []ECRTrustedAccount `yaml:"ecr_trusted_accounts,omitempty"`
	// Services
	Workload            Workload             `yaml:"workload"`
	Domain              Domain               `yaml:"domain"`
	Postgres            Postgres             `yaml:"postgres"`
	Cognito             Cognito              `yaml:"cognito"`
	Ses                 Ses                  `yaml:"ses"`
	Sqs                 Sqs                  `yaml:"sqs"`
	ALB                 ALB                  `yaml:"alb"`
	ScheduledTasks      []ScheduledTask      `yaml:"scheduled_tasks"`
	EventProcessorTasks []EventProcessorTask `yaml:"event_processor_tasks"`
	AppSyncPubSub       AppSync              `yaml:"pubsub_appsync"`
	Buckets             []BucketConfig       `yaml:"buckets"`
	Services            []Service            `yaml:"services"`
	AmplifyApps         []AmplifyApp         `yaml:"amplify_apps,omitempty"`
	// CloudFront CDN Configuration (Schema v14 -> v15: changed to array)
	CloudFrontDistributions []CloudFront `yaml:"cloudfront_distributions,omitempty"`
	// Custom Extensions (for SNS, SQS, Lambda, etc.)
	Extensions Extensions `yaml:"extensions,omitempty"`
	// ManageDNSRecords decides whether Terraform writes the Route53 records for
	// Amplify domains. Amplify creates them itself when the zone is in the same
	// account, so this is only turned on for a cross-account or externally
	// managed zone. A pointer because absent must reach the template as absent:
	// the module's default is false, and an explicit false means the same thing
	// while a missing key means "let the module decide".
	ManageDNSRecords *bool `yaml:"manage_dns_records,omitempty"`
}

// AppSync auth modes (schema v23). These are the values accepted by
// pubsub_appsync.auth_mode and by modules/appsync's own auth_mode variable; the
// two must stay in step.
const (
	// AppSyncAuthCognito verifies tokens against the Cognito user pool this
	// environment already creates. AWS does the verification; no Lambda runs.
	AppSyncAuthCognito = "cognito"
	// AppSyncAuthOIDC verifies tokens against an OIDC issuer's discovery
	// document and JWKS. AWS does the verification; no Lambda runs.
	AppSyncAuthOIDC = "oidc"
	// AppSyncAuthLambda runs the bundled Lambda authorizer. For identity
	// providers the two native modes cannot express.
	AppSyncAuthLambda = "lambda"
)

// appSyncAuthModes is the set of valid modes, in the order they should be
// suggested: the two that cost nothing to run come first.
var appSyncAuthModes = []string{AppSyncAuthCognito, AppSyncAuthOIDC, AppSyncAuthLambda}

// normalizeAppSyncAuthMode maps a raw YAML value to a mode.
//
// Absent/empty means "lambda" everywhere — in this function, in
// modules/appsync/variables.tf's default, and in the template's `default`
// helper. That is not a preference for lambda: it is what the module did before
// auth_mode existed (authentication_type was hardcoded to AWS_LAMBDA), so an
// un-migrated file keeps deploying exactly what it deploys today.
func normalizeAppSyncAuthMode(raw string) string {
	mode := strings.ToLower(strings.TrimSpace(raw))
	if mode == "" {
		return AppSyncAuthLambda
	}
	return mode
}

type AppSync struct {
	Enabled bool `yaml:"enabled"`
	Schema  bool `yaml:"schema"`
	// AuthLambda selects a project-supplied authorizer source tree
	// (custom/appsync/auth_lambda) instead of the one bundled with the module.
	// It only has an effect when AuthMode is "lambda".
	AuthLambda bool `yaml:"auth_lambda"`
	Resolvers  bool `yaml:"resolvers"`

	// AuthMode (schema v23) picks how AppSync authenticates callers:
	// "cognito", "oidc" or "lambda". Empty means "lambda", which is what the
	// module hardcoded before this field existed.
	AuthMode string `yaml:"auth_mode"`

	// APIKeyEnabled (schema v23) attaches an API_KEY provider alongside
	// AuthMode.
	//
	// It defaults to false and should stay there. An API key BYPASSES AuthMode
	// entirely: whoever holds it reaches every resolver without presenting a
	// token, so none of the checks below apply to them. The module used to
	// create one unconditionally, which made the authorizer decorative.
	APIKeyEnabled bool `yaml:"api_key_enabled"`

	// CognitoAppIDClientRegex (schema v23) restricts which app clients of the
	// user pool this API accepts, used when AuthMode is "cognito".
	//
	// Unset means every app client in the pool is accepted, and meroku's cognito
	// module creates web, mobile and dashboard clients on one pool — so an API
	// meant for one of them accepts tokens minted for the others. User pool mode
	// has no separate audience field; this is it. Matched against `aud` in an ID
	// token and `client_id` in an access token. Several clients are written as a
	// pipe-separated list ("1F4G9H|1J6L4B").
	CognitoAppIDClientRegex string `yaml:"cognito_app_id_client_regex,omitempty"`

	// OIDC configuration, used when AuthMode is "oidc".
	//
	// Deliberately configuration and not a built-in: meroku must not ship an
	// issuer URL for anybody's identity platform. Whoever controls the issuer
	// can mint tokens the API accepts, so the value has to be stated by the
	// project.
	//
	// OIDCClientID is AppSync's audience check — AppSync has no separate
	// audience field. It is matched against `aud`, falling back to `azp`, and
	// accepts a pipe-separated list so one API can serve several clients.
	OIDCIssuer   string `yaml:"oidc_issuer,omitempty"`
	OIDCClientID string `yaml:"oidc_client_id,omitempty"`

	// Lambda authorizer configuration (Schema v21), used when AuthMode is
	// "lambda".
	//
	// The authorizer has no built-in identity provider: JWKSURI is the only key
	// source it trusts, so whoever controls that endpoint can mint tokens the
	// API accepts. There is deliberately no default here or in
	// modules/appsync/variables.tf — an empty value is rejected by
	// validateAppSyncConfig before any terraform runs.
	JWKSURI     string `yaml:"jwks_uri"`
	JWTIssuer   string `yaml:"jwt_issuer"`
	JWTAudience string `yaml:"jwt_audience"`

	// RequiredClaims (schema v23) is the claim policy the authorizer enforces
	// after signature, issuer and audience: claim name -> accepted values, where
	// an empty list means "must be present".
	//
	// This is what selects lambda mode. Neither native mode can check a claim
	// beyond iss/aud, so `role` or `tenant_id` policies cannot be expressed with
	// them — whereas accepting several audiences can be, and is not a reason to
	// take on a Lambda. Only read in lambda mode; validation refuses any other
	// mode rather than letting a policy be silently ignored.
	//
	// For POLICY claims, not identity: `sub` names one caller, so there is no
	// fixed value to list. Per-user decisions belong in resolvers, which read
	// `sub` from the authorizer's resolverContext.
	RequiredClaims map[string][]string `yaml:"required_claims,omitempty"`
}

type Workload struct {
	BackendHealthEndpoint      string `yaml:"backend_health_endpoint"`
	BackendExternalDockerImage string `yaml:"backend_external_docker_image"`
	// list(string) in modules/workloads/variables.tf, and a scalar here for most
	// of its life. Schema v26 converts the scalar form. See ScheduledTask.
	BackendContainerCommand []string          `yaml:"backend_container_command,omitempty"`
	BucketPostfix           string            `yaml:"bucket_postfix"`
	BucketPublic            bool              `yaml:"bucket_public"`
	BackendImagePort        int               `yaml:"backend_image_port"`
	SetupFCNSNS             bool              `yaml:"setup_fcnsns"`
	XrayEnabled             bool              `yaml:"xray_enabled"`
	BackendEnvVariables     map[string]string `yaml:"backend_env_variables"`
	Policies                []string          `yaml:"policies"`
	BackendPolicies         []Policy          `yaml:"backend_policies"`
	EnvFilesS3              []S3EnvFile       `yaml:"env_files_s3"`

	SlackWebhook       string   `yaml:"slack_webhook"`
	EnableGithubOIDC   bool     `yaml:"enable_github_oidc"`
	GithubOIDCSubjects []string `yaml:"github_oidc_subjects"`

	InstallPgAdmin bool   `yaml:"install_pg_admin"`
	PgAdminEmail   string `yaml:"pg_admin_email"`

	BackendALBDomainName string `yaml:"backend_alb_domain_name"`

	// Backend scaling configuration
	BackendDesiredCount           int32  `yaml:"backend_desired_count"`
	BackendAutoscalingEnabled     bool   `yaml:"backend_autoscaling_enabled"`
	BackendAutoscalingMinCapacity int32  `yaml:"backend_autoscaling_min_capacity"`
	BackendAutoscalingMaxCapacity int32  `yaml:"backend_autoscaling_max_capacity"`
	BackendCPU                    string `yaml:"backend_cpu"`
	BackendMemory                 string `yaml:"backend_memory"`

	// BackendAutoDeploy (schema v22) is the backend's half of the per-target
	// auto-deploy policy.
	//
	// The backend is the setting's main consumer, so leaving it out would have
	// made auto_deploy a policy you can express for every target except the one
	// that is actually deployed — "do not auto-deploy prod" would have been
	// unsayable. nil/true keeps today's behaviour; a manual deploy always works.
	BackendAutoDeploy *bool `yaml:"backend_auto_deploy,omitempty"`
}

type S3EnvFile struct {
	Bucket string `yaml:"bucket"`
	Key    string `yaml:"key"`
}

type Policy struct {
	Actions   []string `yaml:"actions"`
	Resources []string `yaml:"resources"`
}

type SetupDomainType string

// AdditionalDomain represents an additional domain for CloudFront and other services
// These are managed centrally in the domain configuration
type AdditionalDomain struct {
	Domain            string `yaml:"domain"`                       // The domain name (e.g., "otherdomain.com")
	CreateZone        bool   `yaml:"create_zone,omitempty"`        // Whether to create a new Route 53 zone
	ZoneID            string `yaml:"zone_id,omitempty"`            // Existing zone ID (if not creating)
	CreateCertificate *bool  `yaml:"create_certificate,omitempty"` // Create ACM certificate (default: true)
}

type Domain struct {
	// EXISTING FIELDS - DON'T TOUCH
	Enabled           bool   `yaml:"enabled"`
	CreateDomainZone  bool   `yaml:"create_domain_zone"`
	DomainName        string `yaml:"domain_name"` // Keep as-is - always root
	IsDNSRoot         bool   `yaml:"is_dns_root"`
	DNSRootAccountID  string `yaml:"dns_root_account_id"`
	DelegationRoleArn string `yaml:"delegation_role_arn"`

	// Additional fields from original structure (if missing)
	APIDomainPrefix    string `yaml:"api_domain_prefix,omitempty"`
	AddEnvDomainPrefix bool   `yaml:"add_env_domain_prefix,omitempty"`

	// NEW DNS MANAGEMENT FIELDS
	ZoneID        string `yaml:"zone_id,omitempty"`         // For existing zones
	RootZoneID    string `yaml:"root_zone_id,omitempty"`    // For subdomain delegation
	RootAccountID string `yaml:"root_account_id,omitempty"` // For cross-account access

	// Additional domains for CloudFront and other services (centralized management)
	AdditionalDomains []AdditionalDomain `yaml:"additional_domains,omitempty"`
}

type PostgresEngineVersion string

type Postgres struct {
	Enabled       bool    `yaml:"enabled"`
	Dbname        string  `yaml:"dbname"`
	Username      string  `yaml:"username"`
	PublicAccess  bool    `yaml:"public_access"`
	EngineVersion string  `yaml:"engine_version"`
	Aurora        bool    `yaml:"aurora"`
	MinCapacity   float64 `yaml:"min_capacity"`
	MaxCapacity   float64 `yaml:"max_capacity"`
	// RDS-specific fields (when aurora is false)
	InstanceClass                    string `yaml:"instance_class"`
	AllocatedStorage                 int    `yaml:"allocated_storage"`
	StorageType                      string `yaml:"storage_type"`
	MultiAZ                          bool   `yaml:"multi_az"`
	StorageEncrypted                 bool   `yaml:"storage_encrypted"`
	DeletionProtection               bool   `yaml:"deletion_protection"`
	SkipFinalSnapshot                bool   `yaml:"skip_final_snapshot"`
	IAMDatabaseAuthenticationEnabled bool   `yaml:"iam_database_authentication_enabled"`
}

type Cognito struct {
	Enabled               bool `yaml:"enabled"`
	EnableWebClient       bool `yaml:"enable_web_client"`
	EnableDashboardClient bool `yaml:"enable_dashboard_client"`
	// The tag here read "dashboard_callback_ur_ls" — an acronym-splitting
	// snake_case conversion of DashboardCallbackURLs. env/main.hbs, the cognito
	// module and the web UI all use dashboard_callback_urls, so every load
	// silently dropped the configured URLs and every save wrote them back under
	// the misspelt key. Schema v24 repairs configs already carrying it.
	DashboardCallbackURLs  []string `yaml:"dashboard_callback_urls"`
	EnableUserPoolDomain   bool     `yaml:"enable_user_pool_domain"`
	UserPoolDomainPrefix   string   `yaml:"user_pool_domain_prefix"`
	BackendConfirmSignup   bool     `yaml:"backend_confirm_signup"`
	AutoVerifiedAttributes []string `yaml:"auto_verified_attributes"`
}

type Ses struct {
	Enabled bool `yaml:"enabled"`
	// Legacy single domain support (for backward compatibility)
	DomainName string   `yaml:"domain_name,omitempty"`
	TestEmails []string `yaml:"test_emails,omitempty"`

	// Multi-domain support (Schema v17)
	Domains []SESDomain `yaml:"domains,omitempty"`

	// Global SES configuration (applies to all domains)
	EnableMailFrom    *bool  `yaml:"enable_mail_from,omitempty"`    // Default: true
	MailFromSubdomain string `yaml:"mail_from_subdomain,omitempty"` // Default: "bounce"
	DMARCPolicy       string `yaml:"dmarc_policy,omitempty"`        // Default: "none"
	DMARCRuaEmail     string `yaml:"dmarc_rua_email,omitempty"`     // Optional
}

type SESDomain struct {
	Domain string `yaml:"domain"`            // Required: e.g., "mail.example.com"
	ZoneID string `yaml:"zone_id,omitempty"` // Optional: Route53 zone ID

	// Per-domain overrides (optional)
	EnableMailFrom    *bool  `yaml:"enable_mail_from,omitempty"`    // Override global setting
	MailFromSubdomain string `yaml:"mail_from_subdomain,omitempty"` // Override global setting
	DMARCPolicy       string `yaml:"dmarc_policy,omitempty"`        // Override global setting
	DMARCRuaEmail     string `yaml:"dmarc_rua_email,omitempty"`     // Override global setting
}

type Sqs struct {
	Enabled bool   `yaml:"enabled"`
	Name    string `yaml:"name"`
}

type ALB struct {
	Enabled bool `yaml:"enabled"`
	// Seconds the ALB holds an idle connection open. This is the one knob SSE
	// and other long-lived streams need, and the reason to choose the ALB over
	// API Gateway at all: API Gateway's 30s integration timeout is fixed and
	// cannot stream. 0/absent uses the module default of 60.
	IdleTimeout int `yaml:"idle_timeout,omitempty"`
}

type ScheduledTask struct {
	Name    string `yaml:"name"`
	Enabled *bool  `yaml:"enabled,omitempty"` // Schema v20: nil/true = deployed, false = config kept but not deployed
	// AutoDeploy (schema v22) is CI policy, not deployment: nil/true lets the
	// CI Lambda register a new task-definition revision when a new image lands,
	// false makes it log "auto_deploy is disabled for task:{name}" instead.
	//
	// Note that outside dev nothing can reach a scheduled task automatically at
	// all — modules/ecs_task creates its ECR repository only in dev, and an SSM
	// change deliberately never redeploys a task — so true there enables only
	// the manual path. See modules/workloads/ci_lambda/README.md.
	AutoDeploy          *bool  `yaml:"auto_deploy,omitempty"`
	Schedule            string `yaml:"schedule"`
	ExternalDockerImage string `yaml:"docker_image"`
	// A list of arguments, matching Terraform's list(string). Was a scalar
	// string until schema v25, which converts existing values on load.
	ContainerCommand []string   `yaml:"container_command"`
	CPU              int        `yaml:"cpu,omitempty"`
	Memory           int        `yaml:"memory,omitempty"`
	ECRConfig        *ECRConfig `yaml:"ecr_config,omitempty"` // Schema v9
	// IANA timezone the schedule is evaluated in. DST-aware, so a job set for
	// 09:00 stays at 09:00 local across the change. Empty uses the module
	// default of UTC.
	Timezone string `yaml:"timezone,omitempty"`
	// Retry attempts for the schedule target. A pointer because absent and 0
	// differ: absent leaves AWS's own default of 185 in place, while 0 means
	// do not retry at all.
	MaxRetryAttempts *int `yaml:"max_retry_attempts,omitempty"`
	// SQS queue ARN for failed invocations. Empty disables the DLQ and the
	// scoped sqs:SendMessage grant that comes with it.
	DLQArn string `yaml:"dlq_arn,omitempty"`
}

// EventBridgeRule defines a single EventBridge rule pattern (Schema v13)
type EventBridgeRule struct {
	Name        string   `yaml:"name"`
	Sources     []string `yaml:"sources"`
	DetailTypes []string `yaml:"detail_types"`
}

type EventProcessorTask struct {
	Name    string `yaml:"name"`
	Enabled *bool  `yaml:"enabled,omitempty"` // Schema v20: nil/true = deployed, false = config kept but not deployed
	// New multi-rule support (Schema v13) - preferred format
	Rules []EventBridgeRule `yaml:"rules,omitempty"`
	// Legacy single-rule fields (Schema <= 12) - kept for backward compatibility
	RuleName    string   `yaml:"rule_name,omitempty"`
	DetailTypes []string `yaml:"detail_types,omitempty"`
	Sources     []string `yaml:"sources,omitempty"`
	// Container configuration
	ExternalDockerImage string     `yaml:"docker_image,omitempty"`
	ContainerCommand    []string   `yaml:"container_command,omitempty"`
	CPU                 int        `yaml:"cpu,omitempty"`
	Memory              int        `yaml:"memory,omitempty"`
	ECRConfig           *ECRConfig `yaml:"ecr_config,omitempty"` // Schema v9
}

type EnvVariable struct {
	Name  string `yaml:"name"`
	Value string `yaml:"value"`
}

type Service struct {
	Name    string `yaml:"name"`
	Enabled *bool  `yaml:"enabled,omitempty"` // Schema v19: nil/true = deployed, false = config kept but not deployed
	// AutoDeploy (schema v22) is CI policy, not deployment. `enabled` decides
	// whether the service exists in AWS at all; `auto_deploy` decides whether
	// an ECR push, an SSM change or an S3 env-file write may redeploy it on its
	// own. nil/true keeps today's behaviour. A manual deploy always works.
	AutoDeploy       *bool             `yaml:"auto_deploy,omitempty"`
	DockerImage      string            `yaml:"docker_image"`
	ContainerCommand []string          `yaml:"container_command"`
	ContainerPort    int               `yaml:"container_port"`
	HostPort         int               `yaml:"host_port"`
	CPU              int               `yaml:"cpu"`
	Memory           int               `yaml:"memory"`
	DesiredCount     int               `yaml:"desired_count"`
	RemoteAccess     bool              `yaml:"remote_access"`
	XrayEnabled      bool              `yaml:"xray_enabled"`
	Essential        bool              `yaml:"essential"`
	EnvVars          map[string]string `yaml:"env_vars"`
	EnvVariables     []EnvVariable     `yaml:"env_variables"`
	EnvFilesS3       []S3EnvFile       `yaml:"env_files_s3"`
	ECRConfig        *ECRConfig        `yaml:"ecr_config,omitempty"` // Schema v9
}

type DNSConfig struct {
	RootDomain     string          `yaml:"root_domain"`
	RootAccount    DNSRootAccount  `yaml:"root_account"`
	DelegatedZones []DelegatedZone `yaml:"delegated_zones"`

	// ParentZones remembers where each parent domain is hosted, so the next
	// environment does not have to rediscover it by scanning every AWS profile.
	//
	// Keyed by parent domain rather than by subdomain: what is worth remembering
	// is "coretechx.dev lives in profile mag", which is what makes
	// staging.coretechx.dev cheap after dev.coretechx.dev has been set up.
	// DelegatedZones is keyed by subdomain and so can never serve that purpose.
	ParentZones []ParentZoneRef `yaml:"parent_zones,omitempty"`
}

// ParentZoneRef is a remembered answer to "which AWS profile manages this
// domain?".
//
// It is a hint, never a fact: the profile is re-probed and re-verified against
// public DNS on every use, so a stale entry costs one wasted API call and
// nothing else.
type ParentZoneRef struct {
	Domain    string `yaml:"domain"`
	Profile   string `yaml:"profile"`
	ZoneID    string `yaml:"zone_id,omitempty"`
	AccountID string `yaml:"account_id,omitempty"`
}

type DNSRootAccount struct {
	AccountID         string `yaml:"account_id"`
	ZoneID            string `yaml:"zone_id"`
	DelegationRoleArn string `yaml:"delegation_role_arn"`
}

type DelegatedZone struct {
	Subdomain string   `yaml:"subdomain"`
	AccountID string   `yaml:"account_id"`
	ZoneID    string   `yaml:"zone_id"`
	NSRecords []string `yaml:"ns_records"`
	Status    string   `yaml:"status"`
}

type ECRTrustedAccount struct {
	AccountID string `yaml:"account_id"`
	Env       string `yaml:"env"`
	Region    string `yaml:"region"`
}

// ECRConfig defines per-service ECR repository configuration (Schema v9)
type ECRConfig struct {
	Mode              string `yaml:"mode,omitempty"`                // "create_ecr", "manual_repo", or "use_existing"
	RepositoryURI     string `yaml:"repository_uri,omitempty"`      // For manual_repo mode
	SourceServiceName string `yaml:"source_service_name,omitempty"` // For use_existing mode
	SourceServiceType string `yaml:"source_service_type,omitempty"` // "services", "event_processor_tasks", "scheduled_tasks"
}

// AmplifyApp represents an AWS Amplify application configuration
type AmplifyApp struct {
	Name             string            `yaml:"name"`
	GitHubRepository string            `yaml:"github_repository"`
	GitHubOAuthToken string            `yaml:"github_oauth_token,omitempty"`
	Branches         []AmplifyBranch   `yaml:"branches"`
	SubdomainPrefix  string            `yaml:"subdomain_prefix,omitempty"`      // NEW: Auto-constructs domain
	CustomDomain     string            `yaml:"custom_domain,omitempty"`         // For manual override
	EnvVariables     map[string]string `yaml:"environment_variables,omitempty"` // App-level env vars
	SPAMode          bool              `yaml:"spa_mode,omitempty"`              // Enable SPA routing (200 rewrite instead of 404-200)
}

// AmplifyBranch represents a branch configuration for an Amplify app
type AmplifyBranch struct {
	Name                     string            `yaml:"name"`
	Stage                    string            `yaml:"stage,omitempty"` // PRODUCTION, DEVELOPMENT, BETA, EXPERIMENTAL
	EnableAutoBuild          bool              `yaml:"enable_auto_build,omitempty"`
	EnablePullRequestPreview bool              `yaml:"enable_pull_request_preview,omitempty"`
	EnvironmentVariables     map[string]string `yaml:"environment_variables,omitempty"`
	CustomSubdomains         []string          `yaml:"custom_subdomains,omitempty"` // For branch-specific subdomains
}

// ============================================================================
// CLOUDFRONT CDN CONFIGURATION (Schema v14)
// ============================================================================

// CloudFront represents CloudFront CDN configuration
type CloudFront struct {
	Name              string                     `yaml:"name"` // Unique identifier for this distribution
	Enabled           bool                       `yaml:"enabled"`
	Origins           []CloudFrontOrigin         `yaml:"origins,omitempty"`
	DomainAliases     []string                   `yaml:"domain_aliases,omitempty"`      // e.g., ["*.app.example.com", "app.example.com"]
	AdditionalZones   []CloudFrontAdditionalZone `yaml:"additional_zones,omitempty"`    // Route 53 zones for non-main domain aliases
	CacheBehaviors    []CloudFrontCacheBehavior  `yaml:"cache_behaviors,omitempty"`     // Path-based routing rules
	PriceClass        string                     `yaml:"price_class,omitempty"`         // PriceClass_100, PriceClass_200, PriceClass_All
	DefaultRootObject string                     `yaml:"default_root_object,omitempty"` // e.g., "index.html"
	SPAMode           bool                       `yaml:"spa_mode,omitempty"`            // Enable SPA error handling (404 -> index.html)
	Logging           *CloudFrontLogging         `yaml:"logging,omitempty"`
}

// CloudFrontOrigin represents a CloudFront origin configuration
type CloudFrontOrigin struct {
	Name           string            `yaml:"name"`
	Type           string            `yaml:"type"`                  // "s3", "amplify", "alb", "custom"
	DomainName     string            `yaml:"domain_name,omitempty"` // For custom/alb origins, auto-resolved for amplify/s3
	OriginPath     string            `yaml:"origin_path,omitempty"`
	ProtocolPolicy string            `yaml:"protocol_policy,omitempty"` // https-only, http-only, match-viewer
	CustomHeaders  map[string]string `yaml:"custom_headers,omitempty"`
	// For S3 origins
	BucketName   string `yaml:"bucket_name,omitempty"`   // S3 bucket name (for type: s3)
	CreateBucket bool   `yaml:"create_bucket,omitempty"` // Create a new S3 bucket for this origin
	UseOAC       bool   `yaml:"use_oac,omitempty"`       // Use Origin Access Control for S3
	// For Amplify origins
	AmplifyAppName string `yaml:"amplify_app_name,omitempty"` // Amplify app name (for type: amplify)
}

// CloudFrontCacheBehavior represents path-based routing configuration
type CloudFrontCacheBehavior struct {
	PathPattern          string   `yaml:"path_pattern"` // e.g., "/api/*"
	OriginName           string   `yaml:"origin_name"`  // Reference to origin name
	AllowedMethods       []string `yaml:"allowed_methods,omitempty"`
	CachedMethods        []string `yaml:"cached_methods,omitempty"`
	ForwardQueryString   bool     `yaml:"forward_query_string,omitempty"`
	ForwardHeaders       []string `yaml:"forward_headers,omitempty"`
	ForwardCookies       string   `yaml:"forward_cookies,omitempty"` // none, whitelist, all
	ViewerProtocolPolicy string   `yaml:"viewer_protocol_policy,omitempty"`
	MinTTL               int      `yaml:"min_ttl,omitempty"`
	DefaultTTL           int      `yaml:"default_ttl,omitempty"`
	MaxTTL               int      `yaml:"max_ttl,omitempty"`
	Compress             bool     `yaml:"compress,omitempty"`
}

// CloudFrontLogging represents CloudFront access logging configuration
type CloudFrontLogging struct {
	Enabled        bool   `yaml:"enabled"`
	BucketName     string `yaml:"bucket_name,omitempty"`
	Prefix         string `yaml:"prefix,omitempty"`
	IncludeCookies bool   `yaml:"include_cookies,omitempty"`
}

// CloudFrontAdditionalZone represents a Route 53 zone for non-main domain aliases
type CloudFrontAdditionalZone struct {
	Domain     string `yaml:"domain"`                // The domain name (e.g., "otherdomain.com")
	ZoneID     string `yaml:"zone_id,omitempty"`     // Existing zone ID (if not creating)
	CreateZone bool   `yaml:"create_zone,omitempty"` // Whether to create a new Route 53 zone
}

// ============================================================================
// CUSTOM EXTENSIONS
// ============================================================================

// Extensions represents custom infrastructure extensions defined in YAML
type Extensions struct {
	SNSTopics []SNSTopicExtension `yaml:"sns_topics,omitempty"`
	SQSQueues []SQSQueueExtension `yaml:"sqs_queues,omitempty"`
}

// SNSTopicExtension defines an SNS topic with optional webhooks
type SNSTopicExtension struct {
	Name              string       `yaml:"name"`
	DisplayName       string       `yaml:"display_name,omitempty"`
	AddToBackendEnv   string       `yaml:"add_to_backend_env,omitempty"` // Env var name for ARN
	FIFO              bool         `yaml:"fifo,omitempty"`
	ContentBasedDedup bool         `yaml:"content_based_dedup,omitempty"`
	KMSKeyID          string       `yaml:"kms_key_id,omitempty"`
	Webhooks          []SNSWebhook `yaml:"webhooks,omitempty"`
}

// SNSWebhook defines an HTTP(S) subscription to an SNS topic
type SNSWebhook struct {
	Path               string                 `yaml:"path"`
	RawMessageDelivery bool                   `yaml:"raw_message_delivery,omitempty"`
	FilterPolicy       map[string]interface{} `yaml:"filter_policy,omitempty"`
}

// SQSQueueExtension defines an SQS queue with optional DLQ
type SQSQueueExtension struct {
	Name               string   `yaml:"name"`
	AddToBackendEnv    string   `yaml:"add_to_backend_env,omitempty"`     // Env var for URL
	AddARNToBackendEnv string   `yaml:"add_arn_to_backend_env,omitempty"` // Env var for ARN
	FIFO               bool     `yaml:"fifo,omitempty"`
	VisibilityTimeout  int      `yaml:"visibility_timeout,omitempty"` // default: 30
	MessageRetention   int      `yaml:"message_retention,omitempty"`  // default: 345600 (4 days)
	MaxMessageSize     int      `yaml:"max_message_size,omitempty"`   // default: 262144
	DelaySeconds       int      `yaml:"delay_seconds,omitempty"`
	ReceiveWaitTime    int      `yaml:"receive_wait_time,omitempty"`
	DLQEnabled         bool     `yaml:"dlq_enabled,omitempty"`     // default: true
	DLQMaxReceive      int      `yaml:"dlq_max_receive,omitempty"` // default: 3
	DLQRetention       int      `yaml:"dlq_retention,omitempty"`   // default: 1209600 (14 days)
	KMSKeyID           string   `yaml:"kms_key_id,omitempty"`
	SNSSubscriptions   []string `yaml:"sns_subscriptions,omitempty"` // Names of SNS topics to subscribe to
}

// create function which generate random string
func generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	result := make([]byte, length)
	for i := 0; i < length; i++ {
		result[i] = charset[rand.Intn(len(charset))]
	}
	return string(result)
}

func createEnv(name, env string) Env {
	return Env{
		SchemaVersion: CurrentSchemaVersion, // Always create with latest schema version
		Project:       name,
		Env:           env,
		IsProd:        false,
		Region:        "", // Will be filled when AWS profile is selected
		AccountID:     "", // Will be filled when AWS profile is selected
		AWSProfile:    "", // Will be filled when AWS profile is selected
		StateBucket:   fmt.Sprintf("sate-bucket-%s-%s-%s", name, env, generateRandomString(5)),
		StateFile:     "state.tfstate",
		// VPC Configuration (schema v6)
		// Default to custom VPC for new projects (simpler, more control)
		// Always creates 2 AZs with public subnets only, no NAT gateway
		UseDefaultVPC: false,         // Use custom VPC by default
		VPCCIDR:       "10.0.0.0/16", // Optional, VPC module has this default
		// ECR Configuration (schema v7)
		// Default to local ECR for new projects
		ECRStrategy:        "local",
		ECRAccountID:       "",
		ECRAccountRegion:   "",
		ECRTrustedAccounts: []ECRTrustedAccount{},
		Workload: Workload{
			SlackWebhook:               "",
			BucketPostfix:              generateRandomString(5),
			BucketPublic:               true,
			BackendHealthEndpoint:      "",
			BackendExternalDockerImage: "",
			SetupFCNSNS:                false,
			BackendImagePort:           8080,
			EnableGithubOIDC:           false,
			GithubOIDCSubjects:         []string{"repo:MadAppGang/*", "repo:MadAppGang/project_backend:ref:refs/heads/main"},
			BackendContainerCommand:    nil,
			InstallPgAdmin:             false,
			PgAdminEmail:               "",
			XrayEnabled:                false,
			BackendEnvVariables:        map[string]string{"TEST": "passed"},
			BackendPolicies:            []Policy{},
			// Backend scaling defaults (schema v4)
			BackendDesiredCount:           1,
			BackendAutoscalingEnabled:     false,
			BackendAutoscalingMinCapacity: 1,
			BackendAutoscalingMaxCapacity: 4,
			BackendCPU:                    "256",
			BackendMemory:                 "512",
			BackendALBDomainName:          "",
		},
		Domain: Domain{
			Enabled:          false,
			CreateDomainZone: true,
			DomainName:       "",
		},
		Postgres: Postgres{
			Enabled:       false,
			Dbname:        "",
			Username:      "",
			PublicAccess:  false,
			EngineVersion: "16.x",
			// Aurora defaults (schema v2)
			Aurora:      false,
			MinCapacity: 0.5,
			MaxCapacity: 1.0,
		},
		Cognito: Cognito{
			Enabled:                false,
			EnableWebClient:        false,
			EnableDashboardClient:  false,
			DashboardCallbackURLs:  []string{},
			EnableUserPoolDomain:   false,
			UserPoolDomainPrefix:   "",
			BackendConfirmSignup:   false,
			AutoVerifiedAttributes: []string{},
		},
		Ses: Ses{
			Enabled:    false,
			DomainName: "",
			TestEmails: []string{"i@madappgang.com"},
		},
		Sqs: Sqs{
			Enabled: false,
			Name:    "",
		},
		ALB: ALB{
			Enabled: false, // Schema v2
		},
		AppSyncPubSub: AppSync{
			Enabled:    false,
			Schema:     false,
			AuthLambda: false,
			Resolvers:  false,
			// "lambda" matches what an absent auth_mode means, so a new project
			// and an un-migrated one behave identically. It is not a
			// recommendation: cognito is the mode to reach for when the project
			// has a user pool, and oidc when an external provider publishes a
			// discovery document. Both are verified by AWS with no Lambda on the
			// request path. AppSync is disabled here anyway, so this value only
			// takes effect once someone turns it on — and validation then names
			// whichever field their chosen mode is missing.
			AuthMode: AppSyncAuthLambda,
			// No environment gets an API key it did not ask for. An API key
			// skips auth_mode entirely, so a silent one would undo every check
			// configured below it.
			APIKeyEnabled: false,
			// Left empty on purpose: meroku must never pick an identity provider
			// on the user's behalf, for either mode. Enabling AppSync without
			// filling in the field its mode needs fails validation rather than
			// trusting someone else's keys.
			OIDCIssuer:              "",
			OIDCClientID:            "",
			CognitoAppIDClientRegex: "",
			JWKSURI:                 "",
			JWTIssuer:               "",
			JWTAudience:             "",
			RequiredClaims:          nil,
		},
		Buckets:                 []BucketConfig{},
		Services:                []Service{},
		ScheduledTasks:          []ScheduledTask{},
		EventProcessorTasks:     []EventProcessorTask{},
		CloudFrontDistributions: []CloudFront{},
	}
}

func loadEnv(name string) (Env, error) {
	// Use the migration-aware loader
	return loadEnvWithMigration(name)
}

// loadEnvFromPath loads environment config from multiple possible paths
// This is useful when running from env/dev or env/prod subdirectories
// Now uses migration-aware loading
func loadEnvFromPath(name string) (Env, error) {
	// Use the migration-aware loader which handles multiple paths
	return loadEnvWithMigration(name)
}

// loadEnvToMap reads a config as a raw map for template rendering.
//
// The typed Env struct cannot be used here: Handlebars addresses the config
// generically, and any YAML key Go does not declare would vanish from the
// rendered Terraform. So this loader exists alongside loadEnv on purpose.
//
// What was not on purpose is that only loadEnv migrated. Migration sat on the
// caller's side of the boundary, so `meroku deploy` -- which calls loadEnv at
// deploy.go:83 before templating -- migrated and then re-read a current file,
// while `meroku generate` went straight to applyTemplate and rendered whatever
// was on disk. A config several schema versions behind parses perfectly well; it
// is simply missing the keys the template expects, so generation succeeded and
// quietly substituted defaults for every field the missing versions added.
//
// Migrating here makes it structural rather than remembered: there is no way to
// read this config for templating without it being current, whoever calls.
func loadEnvToMap(name string) (map[string]interface{}, error) {
	var e map[string]interface{}

	if err := migrateFileIfNeeded(name); err != nil {
		return nil, err
	}

	data, err := os.ReadFile(name)
	if err != nil {
		wd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("error getting current working directory: %v", err)
		}
		return nil, fmt.Errorf("error reading YAML file: %v, current folder: %s", err, wd)
	}

	err = yaml.Unmarshal(data, &e)
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling YAML: %v", err)
	}

	// Convert to JSON-compatible format for template rendering
	converted := convertToJSONCompatible(e)
	if convertedMap, ok := converted.(map[string]interface{}); ok {
		return convertedMap, nil
	}

	return e, nil
}

func saveEnv(e Env) error {
	yamlData, err := yaml.Marshal(e)
	if err != nil {
		return err
	}
	filename := e.Env + ".yaml"
	return os.WriteFile(filename, yamlData, 0o644)
}

func saveEnvToFile(e Env, filepath string) error {
	yamlData, err := yaml.Marshal(e)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath, yamlData, 0o644)
}

// var AWSRegions = []string{
// 	"us-east-1",
// 	"us-east-2",
// 	"us-west-1",
// 	"us-west-2",
// 	"af-south-1",
// 	"ap-east-1",
// 	"ap-south-1",
// 	"ap-northeast-1",
// 	"ap-northeast-2",
// 	"ap-northeast-3",
// 	"ap-southeast-1",
// 	"ap-southeast-2",
// 	"ap-northeast-3",
// 	"ca-central-1",
// 	"eu-central-1",
// 	"eu-west-1",
// 	"eu-west-2",
// 	"eu-south-1",
// 	"eu-west-3",
// 	"eu-north-1",
// 	"me-south-1",
// 	"sa-east-1",
// }
