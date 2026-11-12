package main

import (
	"fmt"
	"math/rand"
	"os"
	"strings"

	"gopkg.in/yaml.v2"
)

type Env struct {
	SchemaVersion int    `yaml:"schema_version,omitempty"`
	Project       string `yaml:"project"`
	Env           string `yaml:"env"`
	IsProd        bool   `yaml:"is_prod"`
	// AutoDeploy is the environment-wide answer to "may a new image or config
	// redeploy a target on its own?". A target's own auto_deploy overrides it.
	AutoDeploy     *bool  `yaml:"auto_deploy,omitempty"`
	Region         string `yaml:"region"`
	AccountID      string `yaml:"account_id"`
	AWSProfile     string `yaml:"aws_profile"`
	StateBucket    string `yaml:"state_bucket"`
	StateFile      string `yaml:"state_file"`
	StateLockTable string `yaml:"state_lock_table,omitempty"` // DynamoDB table for state locking (Schema v16)
	// VPC Configuration
	UseDefaultVPC bool   `yaml:"use_default_vpc"`
	VPCCIDR       string `yaml:"vpc_cidr,omitempty"` // Optional, VPC module has default
	// AllowVPCCIDROverride turns off meroku's vpc_cidr rule checks for this
	// environment — the escape hatch for a VPC already deployed with a range the
	// rules now refuse. Overlap warnings still print. See app/vpc_cidr.go.
	AllowVPCCIDROverride bool `yaml:"allow_vpc_cidr_override,omitempty"`
	// EgressStrategy (schema v27) decides how ECS tasks reach the internet:
	// "public_ip" (default), "nat_gateway", or "nat_gateway_ha". It can be
	// changed at any time; switching replaces the tasks' network configuration
	// and triggers a rolling redeploy. See ai_docs/EGRESS_COST_MODEL.md.
	EgressStrategy string `yaml:"egress_strategy,omitempty"`
	// ECR Configuration
	ECRStrategy      string `yaml:"ecr_strategy,omitempty"`       // "local" or "cross_account"
	ECRAccountID     string `yaml:"ecr_account_id,omitempty"`     // For cross-account ECR access
	ECRAccountRegion string `yaml:"ecr_account_region,omitempty"` // For cross-account ECR access
	// ECR Trusted Accounts (Schema v8)
	ECRTrustedAccounts []ECRTrustedAccount `yaml:"ecr_trusted_accounts,omitempty"`
	// ECRImageEvents lets a cross_account environment receive the source
	// registry's image-push events. The source enables its half with
	// forward_image_events on the matching ecr_trusted_accounts entry.
	ECRImageEvents bool `yaml:"ecr_image_events,omitempty"`
	// Compute (schema v26) declares the EC2 capacity pools ECS may place tasks
	// on. Absent on every environment written before v26, and that absence is
	// the point: no pools means no EC2 resources and no plan diff. The v26
	// migration deliberately does not create this block.
	Compute Compute `yaml:"compute,omitempty"`
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
	// EFS is the top-level `efs` list: the filesystems modules/efs creates,
	// which the backend mounts through Workload.EFS. Absent renders no module.
	EFS         []EFSConfig    `yaml:"efs,omitempty"`
	Buckets     []BucketConfig `yaml:"buckets"`
	Services    []Service      `yaml:"services"`
	AmplifyApps []AmplifyApp   `yaml:"amplify_apps,omitempty"`
	// CloudFront CDN Configuration (Schema v14 -> v15: changed to array)
	CloudFrontDistributions []CloudFront `yaml:"cloudfront_distributions,omitempty"`
	// Custom Extensions (for SNS, SQS, Lambda, etc.)
	Extensions Extensions `yaml:"extensions,omitempty"`
	// GitHubRunners declares CodeBuild-hosted GitHub Actions runners. A
	// pointer so an environment without the block saves without one: absent
	// and `enabled: false` both render nothing. See ai_docs/GITHUB_RUNNERS.md.
	GitHubRunners *GitHubRunners `yaml:"github_runners,omitempty"`
	// ManageDNSRecords decides whether Terraform writes the Route53 records for
	// Amplify domains. Amplify creates them itself when the zone is in the same
	// account, so this is only turned on for a cross-account or externally
	// managed zone. A pointer because absent must reach the template as absent:
	// the module's default is false, and an explicit false means the same thing
	// while a missing key means "let the module decide".
	ManageDNSRecords *bool `yaml:"manage_dns_records,omitempty"`
	// Neon (schema v32) is a Neon Postgres project and the databases on its
	// default branch. nil renders nothing; see ai_docs/NEON.md.
	Neon *Neon `yaml:"neon,omitempty"`
}

// Neon is the env's top-level `neon:` block. A Neon project is the trust zone:
// every role meroku makes lives on the project's default branch.
type Neon struct {
	// Mode is "create" (meroku creates the project) or "reuse" (an existing
	// project, read as data and never changed).
	Mode string `yaml:"mode"`
	// ProjectID names the existing project. Reuse mode only.
	ProjectID string `yaml:"project_id,omitempty"`
	// PGVersion is the reused project's Postgres major version, checked
	// against the project at deploy. Reuse mode only; create mode uses
	// Project.PGVersion.
	PGVersion int `yaml:"pg_version,omitempty"`
	// Project describes the project meroku creates. Create mode only.
	Project *NeonProject `yaml:"project,omitempty"`
	// OwnerRole owns every database meroku creates and every table, sequence
	// and schema in them; Terraform connects as it. Empty means neondb_owner.
	OwnerRole string `yaml:"owner_role,omitempty"`
	// Databases live on the project's default branch.
	Databases []NeonDatabase `yaml:"databases,omitempty"`
}

// NeonProject is `neon.project`, the project meroku creates.
type NeonProject struct {
	Name     string `yaml:"name"`
	OrgID    string `yaml:"org_id"`
	RegionID string `yaml:"region_id"`
	// PGVersion is 15, 16 or 17; 0 means 17.
	PGVersion int `yaml:"pg_version,omitempty"`
}

// NeonDatabase is one `neon.databases[]` entry. Create true: meroku creates it
// and never deletes it. False: it exists and is read as data.
type NeonDatabase struct {
	Name   string `yaml:"name"`
	Create bool   `yaml:"create"`
}

// NeonAccess is a service's database access: `workload.backend_neon` or
// `services[].neon`. meroku gives the service its own LOGIN role and writes
// its DATABASE_URL to the service's SSM path.
type NeonAccess struct {
	// Database names an entry of neon.databases.
	Database string `yaml:"database"`
	// Schemas the role gets rights on, also its search_path. Empty means
	// ["public"].
	Schemas []string `yaml:"schemas,omitempty"`
	// Access is "readonly" or "readwrite".
	Access string `yaml:"access"`
	// PasswordVersion 0 means 1. Raising it generates a new password for the
	// same role.
	PasswordVersion int `yaml:"password_version,omitempty"`
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
	// of its life. Schema v29 converts the scalar form. See ScheduledTask.
	BackendContainerCommand []string `yaml:"backend_container_command,omitempty"`
	BucketPostfix           string   `yaml:"bucket_postfix"`
	BucketPublic            bool     `yaml:"bucket_public"`
	BackendImagePort        int      `yaml:"backend_image_port"`
	// BackendRemoteAccess turns on ECS Exec for the backend container and the IAM
	// policy it needs. A *bool because the template defaults an absent key to
	// true: an operator's explicit false has to survive a save as false, or the
	// next save silently re-enables shell access to the container.
	BackendRemoteAccess *bool             `yaml:"backend_remote_access,omitempty"`
	SetupFCNSNS         bool              `yaml:"setup_fcnsns"`
	XrayEnabled         bool              `yaml:"xray_enabled"`
	BackendEnvVariables map[string]string `yaml:"backend_env_variables"`
	Policies            []string          `yaml:"policies"`
	BackendPolicies     []Policy          `yaml:"backend_policies"`
	EnvFilesS3          []S3EnvFile       `yaml:"env_files_s3"`

	SlackWebhook     string `yaml:"slack_webhook"`
	EnableGithubOIDC bool   `yaml:"enable_github_oidc"`
	// Grants the GitHub Actions role push access to BackendExternalDockerImage
	// when it is an ECR repository in this account and region. See
	// modules/github_policy/main.tf.
	GithubPushExternalImage bool     `yaml:"github_push_external_image,omitempty"`
	GithubOIDCSubjects      []string `yaml:"github_oidc_subjects"`
	// *bool because absent and false mean different things here. AWS allows one
	// GitHub OIDC provider per account, so nil means "not decided yet, meroku may
	// resolve it against the account" while an explicit false is an operator's
	// (or a previous resolution's) statement that another project owns it.
	// Schema v28. nil/true = create it, false = federate against the existing one.
	GithubOIDCCreateProvider *bool `yaml:"github_oidc_create_provider,omitempty"`

	InstallPgAdmin bool   `yaml:"install_pg_admin"`
	PgAdminEmail   string `yaml:"pg_admin_email"`

	BackendALBDomainName string `yaml:"backend_alb_domain_name"`

	// Backend scaling configuration
	BackendDesiredCount           int32 `yaml:"backend_desired_count"`
	BackendAutoscalingEnabled     bool  `yaml:"backend_autoscaling_enabled"`
	BackendAutoscalingMinCapacity int32 `yaml:"backend_autoscaling_min_capacity"`
	BackendAutoscalingMaxCapacity int32 `yaml:"backend_autoscaling_max_capacity"`
	// The two scaling targets are pointers, and the three fields above them are
	// not, because those three have something writing a value into the configs
	// meroku itself produces: migrateToV4 adds them to an older one and createEnv
	// sets them for a new one. That is not every config: a hand-written one that
	// already declares the current version reaches neither, because
	// applyMigrations returns before the chain runs, so a plain int32 there can
	// still stand for "absent", and does, as 0. The targets have no populator on
	// any path: their only default is the template's
	// {{default workload.backend_autoscaling_target_cpu 70}}.
	//
	// A plain int32 here would save an unset target as 0, and the default helper
	// returns 0 for 0 rather than falling back, so both target-tracking policies
	// would render target_value = 0. The template passes all five arguments
	// unconditionally while both policies are
	// count = var.backend_autoscaling_enabled ? 1 : 0, so that 0 reaches AWS only
	// in a config that turned autoscaling on — and there AWS rejects it, turning a
	// config that only omitted an optional key into a failed apply. nil keeps the
	// key out of the YAML and leaves the template's default in force.
	BackendAutoscalingTargetCPU    *int32 `yaml:"backend_autoscaling_target_cpu,omitempty"`
	BackendAutoscalingTargetMemory *int32 `yaml:"backend_autoscaling_target_memory,omitempty"`
	BackendCPU                     string `yaml:"backend_cpu"`
	BackendMemory                  string `yaml:"backend_memory"`

	// BackendRuntime (schema v26) is fargate | ec2. The v26 migration stamps
	// "fargate" on every existing environment so a runtime choice is visible in
	// the YAML before anyone has to make one, and so the rendered Terraform is
	// unchanged. BackendComputePool names the pool to place on and is required
	// when BackendRuntime is "ec2".
	BackendRuntime     string `yaml:"backend_runtime,omitempty"`
	BackendComputePool string `yaml:"backend_compute_pool,omitempty"`

	// BackendAutoDeploy (schema v22) is the backend's half of the per-target
	// auto-deploy policy.
	//
	// The backend is the setting's main consumer, so leaving it out would have
	// made auto_deploy a policy you can express for every target except the one
	// that is actually deployed — "do not auto-deploy prod" would have been
	// unsayable. nil/true keeps today's behaviour; a manual deploy always works.
	BackendAutoDeploy *bool `yaml:"backend_auto_deploy,omitempty"`

	// BackendHealthCheck is the backend container's ECS health check. nil
	// renders nothing, so the container definition is the one meroku has
	// always generated. See modules/workloads/backend.tf.
	BackendHealthCheck *BackendHealthCheck `yaml:"backend_health_check,omitempty"`
	// BackendCircuitBreaker is the backend service's ECS deployment circuit
	// breaker. nil renders no block.
	BackendCircuitBreaker *BackendCircuitBreaker `yaml:"backend_circuit_breaker,omitempty"`
	// BackendHealthCheckGracePeriod is the backend service's
	// health_check_grace_period_seconds, used only while the ALB is on. nil or 0
	// sets nothing.
	BackendHealthCheckGracePeriod *int `yaml:"backend_health_check_grace_period,omitempty"`
	// BackendNeon is the backend's Neon database access. nil means none.
	BackendNeon *NeonAccess `yaml:"backend_neon,omitempty"`

	// Policy is the user's own `policy` block: statements meroku adds to the
	// backend task role on top of the grants it builds itself. It renders into
	// backend_policy, which modules/workloads/backend.tf creates as a
	// customer-managed aws_iam_policy and attaches to the role.
	//
	// Not BackendPolicies above: that one's Policy type carries no effect and no
	// conditions.
	Policy []IAMStatement `yaml:"policy,omitempty"`

	// EFS is workload.efs: the filesystems from the environment's top-level efs
	// list that the backend container mounts. Absent renders neither
	// available_efs nor backend_efs_mounts.
	EFS []EFSMount `yaml:"efs,omitempty"`
}

// BackendHealthCheck is workload.backend_health_check. Command takes the same
// string-or-list forms as services[].health_check_command. The numbers are
// pointers so an explicit 0 (valid for start_period) survives a load and save,
// and an absent field stays absent; the module supplies the defaults.
type BackendHealthCheck struct {
	Command     HealthCheckCommand `yaml:"command,omitempty"`
	Interval    *int               `yaml:"interval,omitempty"`
	Timeout     *int               `yaml:"timeout,omitempty"`
	Retries     *int               `yaml:"retries,omitempty"`
	StartPeriod *int               `yaml:"start_period,omitempty"`
}

// BackendCircuitBreaker is workload.backend_circuit_breaker. A nil field takes
// the module default (true).
type BackendCircuitBreaker struct {
	Enable   *bool `yaml:"enable,omitempty"`
	Rollback *bool `yaml:"rollback,omitempty"`
}

type S3EnvFile struct {
	Bucket string `yaml:"bucket"`
	Key    string `yaml:"key"`
}

type Policy struct {
	Actions   []string `yaml:"actions"`
	Resources []string `yaml:"resources"`
}

// IAMStatement is one statement of a `policy` block: the IAM policy a workload,
// service or task carries in its own config.
//
// It is deliberately not the Policy type above, which serves `backend_policies`
// and `instance_policies`. That type has neither an effect nor conditions, so it
// cannot express the Deny that keeps a broad grant narrow, nor scope a statement
// to a region or a transport.
type IAMStatement struct {
	// Effect is "Allow" or "Deny". Empty means Allow, which is IAM's own default.
	Effect    string   `yaml:"effect,omitempty"`
	Actions   []string `yaml:"actions"`
	Resources []string `yaml:"resources"`
	// Conditions restrict when the statement applies, such as a region or a
	// transport requirement. Empty means the statement is unconditional.
	Conditions []IAMCondition `yaml:"conditions,omitempty"`
}

// IAMCondition is one condition block of an IAMStatement: a test applied to the
// values of a condition key.
type IAMCondition struct {
	Test     string   `yaml:"test"`
	Variable string   `yaml:"variable"`
	Values   []string `yaml:"values"`
}

// Compute (schema v26) is the EC2 capacity side of the environment: the pools
// of container instances ECS services may be placed on when their runtime is
// "ec2". An environment with no pools is the normal state and renders nothing.
type Compute struct {
	Pools []ComputePool `yaml:"pools,omitempty"`
}

// ComputePool is one Auto Scaling group fronted by an ECS capacity provider.
//
// The numeric fields are pointers for the same reason Service.Enabled is a
// *bool: min_size: 0 and on_demand_base: 0 are meaningful values that have to
// survive a load/save round-trip as something other than "absent". A plain int
// would make "scale this pool to nothing when idle" indistinguishable from
// "this key was never written", and saving would then invent a 0 for every pool
// that omitted the key.
//
// Scope of that claim, because it is easy to over-read: these tags govern the
// TUI's load/save path (loadEnvWithMigration -> Env -> saveEnvToFile). They do
// not execute on the generate path. Generation renders from loadEnvToMap, which
// unmarshals into map[string]interface{} — ComputePool is never instantiated
// there. An absent YAML key is an absent map key and renders as absent; 0 is
// present and renders as 0. Both behaviours are correct, but they come from the
// map, not from these struct tags.
type ComputePool struct {
	Name          string   `yaml:"name"`
	Enabled       *bool    `yaml:"enabled,omitempty"`
	InstanceTypes []string `yaml:"instance_types"`
	// CapacityType is on_demand | spot | spot_with_base.
	CapacityType string `yaml:"capacity_type,omitempty"`
	// OnDemandBase counts INSTANCES, not tasks: it is the ASG's on-demand base
	// capacity, held before the spot percentage applies to anything above it.
	OnDemandBase   *int `yaml:"on_demand_base,omitempty"`
	MinSize        *int `yaml:"min_size,omitempty"`
	MaxSize        *int `yaml:"max_size,omitempty"`
	TargetCapacity *int `yaml:"target_capacity,omitempty"`
	// NetworkMode is a plain string rather than a pointer: "" and "bridge" mean
	// the same thing in Go, in the template and in the module's optional()
	// default, so absence needs no separate representation. bridge is the
	// default because a task on a bridge-mode instance egresses through the
	// instance's own public ENI, which is the one thing an awsvpc task ENI in a
	// public subnet does not get.
	NetworkMode string `yaml:"network_mode,omitempty"`
	// AssumeEgress is the operator's assertion that this pool's subnets have an
	// outbound path, and it is the sole unlock for a pool that sets
	// network_mode: awsvpc. Default absent, which means false.
	//
	// It lives on the pool rather than on a top-level vpc: map because this
	// schema has no vpc: map — Env carries flat use_default_vpc and vpc_cidr —
	// so a new one would be a second convention for the same subject. Keeping
	// it here also puts the assertion next to the only thing it unlocks, and
	// round-trips it through the pool struct the TUI already saves. A key with
	// no struct field would be dropped by the next unrelated save, and the
	// operator's next generate would fail with no edit to blame.
	//
	// Pointer for the same reason as the numeric fields: an explicit false and
	// an absent key must stay distinguishable.
	AssumeEgress *bool `yaml:"assume_egress,omitempty"`
	// AMIFamily is al2023 | al2023_arm64 | al2023_gpu. AMIID overrides it.
	AMIFamily        string          `yaml:"ami_family,omitempty"`
	AMIID            string          `yaml:"ami_id,omitempty"`
	RootVolumeGB     *int            `yaml:"root_volume_gb,omitempty"`
	UserDataExtra    string          `yaml:"user_data_extra,omitempty"`
	ExtraVolumes     []ComputeVolume `yaml:"extra_volumes,omitempty"`
	InstancePolicies []Policy        `yaml:"instance_policies,omitempty"`
}

// ComputeVolume is an extra EBS volume attached to every instance in a pool.
type ComputeVolume struct {
	DeviceName string `yaml:"device_name"`
	SizeGB     int    `yaml:"size_gb"`
	// Type defaults to gp3 in the module when this is empty.
	Type string `yaml:"type,omitempty"`
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
	APIDomainPrefix string `yaml:"api_domain_prefix,omitempty"`
	// AddEnvDomainPrefix is a pointer because false is meaningful and absent
	// means true: omitempty on a plain bool would drop an explicit false on save.
	AddEnvDomainPrefix *bool `yaml:"add_env_domain_prefix,omitempty"`

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

	// Apex TXT record set (optional). SPFRecord replaces the default
	// "v=spf1 include:amazonses.com ~all"; SPFTTL replaces the default 600 and
	// is a pointer so that 0 is kept as a value. ApexTXTValues are written into
	// the same record set as the SPF value.
	SPFRecord     string   `yaml:"spf_record,omitempty"`
	SPFTTL        *int     `yaml:"spf_ttl,omitempty"`
	ApexTXTValues []string `yaml:"apex_txt_values,omitempty"`

	// Extra TXT record sets at names below the domain, such as a Google
	// Workspace DKIM key at google._domainkey (optional).
	TXTRecords []SESTXTRecord `yaml:"txt_records,omitempty"`
}

// SESTXTRecord is one extra TXT record set of an SES domain. Name is relative
// to the domain. Values longer than 255 characters are split automatically.
// TTL is a pointer so that 0 is kept as a value; unset means 600.
type SESTXTRecord struct {
	Name   string   `yaml:"name" json:"name"`
	Values []string `yaml:"values" json:"values"`
	TTL    *int     `yaml:"ttl,omitempty" json:"ttl,omitempty"`
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
	// SSLPolicy is the HTTPS listener's ELB security policy. "" uses the module
	// default, ELBSecurityPolicy-2016-08.
	SSLPolicy string `yaml:"ssl_policy,omitempty"`
	// DeregistrationDelay is the target groups' draining time in seconds. A
	// pointer, because 0 is valid and must survive a load and save.
	DeregistrationDelay *int `yaml:"deregistration_delay,omitempty"`
	// RemoveAPIGateway drops the API Gateway API, stage, access-log group and
	// backend VPC Link while the ALB is on.
	RemoveAPIGateway bool `yaml:"remove_api_gateway,omitempty"`
}

// EFSConfig is one entry of the top-level `efs` list, shaped by the efs_configs
// variable in modules/efs/variable.tf.
//
// UID and GID are pointers because 0 and absent mean different things here.
// Both reach only root_directory.creation_info, which AWS applies when it has to
// create the access point's root directory; the access point's own posix_user is
// uid 0 / gid 0 in modules/efs/main.tf whatever the YAML says. An absent uid
// takes the variable's default of 1000, uid: 0 asks for a root-owned directory.
// A plain int would break one way or the other — with omitempty it drops an
// explicit 0 and hands the directory to 1000, without omitempty it writes 0 into
// every config that never set one. Either way a filesystem created at the next
// apply gets a differently owned root directory, and in the second case every
// untuned config also gains a spurious diff at the next plan.
//
// Permissions and Path are plain strings: neither "" nor absent is a POSIX mode
// or a root directory anybody can mean, so the two collapse onto the module's
// defaults ("755" and "/") and nothing is lost — the call ComputePool makes for
// NetworkMode.
type EFSConfig struct {
	Name        string `yaml:"name"`
	UID         *int   `yaml:"uid,omitempty"`
	GID         *int   `yaml:"gid,omitempty"`
	Permissions string `yaml:"permissions,omitempty"`
	Path        string `yaml:"path,omitempty"`
}

// EFSMount is one entry of workload.efs: a filesystem the backend container
// mounts. Name has to be the Name of an EFSConfig in the same environment,
// which is how the template resolves the mount to module.efs.efs_configs.
//
// MountPoint is omitempty because modules/workloads/variables.tf declares
// mount_point = optional(string, "/"), so a mount that omits the key has to stay
// omitted. Without it a save writes mount_point: "" into every config that never
// set one, and the YAML then claims a mount path nobody chose.
//
// ReadOnly is a *bool, the shape Workload.BackendRemoteAccess has, because the
// module declares read_only = optional(bool, false): absent has to stay absent.
// A plain bool cannot tell "the user asked for a read-write mount" from "the
// user said nothing", and a save would then write read_only: false into every
// config that never set one, so the YAML claims a choice about a mount nobody
// tuned.
type EFSMount struct {
	Name       string `yaml:"name"`
	MountPoint string `yaml:"mount_point,omitempty"`
	ReadOnly   *bool  `yaml:"read_only,omitempty"`
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
	ContainerCommand []string `yaml:"container_command"`
	// EnvironmentVariables are the container's own environment, rendered as the
	// task's custom_env_vars. ENV is not a usable name: meroku creates
	// <path>/env for every workload and ECS refuses a name present in both.
	// See app/env_reserved.go.
	EnvironmentVariables map[string]string `yaml:"environment_variables,omitempty"`
	CPU                  int               `yaml:"cpu,omitempty"`
	Memory               int               `yaml:"memory,omitempty"`
	ECRConfig            *ECRConfig        `yaml:"ecr_config,omitempty"` // Schema v9
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
	// Policy is the user's own `policy` block: statements meroku adds to this
	// task's role on top of the grants it builds itself.
	Policy []IAMStatement `yaml:"policy,omitempty"`
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
	ExternalDockerImage string   `yaml:"docker_image,omitempty"`
	ContainerCommand    []string `yaml:"container_command,omitempty"`
	// EnvironmentVariables are the container's own environment, rendered as the
	// task's custom_env_vars. ENV is not a usable name: meroku creates
	// <path>/env for every workload and ECS refuses a name present in both.
	// See app/env_reserved.go.
	EnvironmentVariables map[string]string `yaml:"environment_variables,omitempty"`
	CPU                  int               `yaml:"cpu,omitempty"`
	Memory               int               `yaml:"memory,omitempty"`
	ECRConfig            *ECRConfig        `yaml:"ecr_config,omitempty"` // Schema v9
	// Policy is the user's own `policy` block: statements meroku adds to this
	// task's role on top of the grants it builds itself.
	Policy []IAMStatement `yaml:"policy,omitempty"`
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

	// Runtime (schema v26) is fargate | ec2; ComputePool names the pool and is
	// required when Runtime is "ec2". An empty Runtime and "fargate" render
	// identically, which is what keeps a pre-v26 config at a zero plan diff.
	Runtime     string `yaml:"runtime,omitempty"`
	ComputePool string `yaml:"compute_pool,omitempty"`

	// NetworkAccess (schema v30) is who may open a connection to the container
	// port: public | internal | full_open | none (see migration_v30.go). Empty
	// renders as public. Outbound traffic and the public IP never change.
	NetworkAccess string `yaml:"network_access,omitempty"`
	// HealthCheckCommand is the container health check: a shell command string,
	// or a raw ECS list such as ["CMD", "/app/healthcheck"] for images with no
	// shell. See health_check_command.go. It runs inside the container, so the
	// tool must exist in the image.
	HealthCheckCommand HealthCheckCommand `yaml:"health_check_command,omitempty"`
	// Neon is this service's Neon database access. nil means none.
	Neon *NeonAccess `yaml:"neon,omitempty"`

	// Policy is the user's own `policy` block: statements meroku adds to this
	// service's role on top of the grants it builds itself.
	Policy []IAMStatement `yaml:"policy,omitempty"`
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
	// ForwardImageEvents sends this registry's image-push events to the
	// account and region above.
	ForwardImageEvents bool `yaml:"forward_image_events,omitempty"`
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
	// CustomRules are rewrites and redirects applied before the SPA rule, in
	// order. See amplify_rewrites.go.
	CustomRules []AmplifyRewriteRule `yaml:"custom_rules,omitempty"`
}

// AmplifyRewriteRule is one amplify_apps[].custom_rules entry. Exactly one of
// Target and Workload is set. Workload is "backend" or a public service, and
// the request is proxied to that workload's base URL plus Path (or the source
// path when Path is empty). Status is 200, 301, 302 or 404-200; empty means 200.
type AmplifyRewriteRule struct {
	Source   string `yaml:"source"`
	Target   string `yaml:"target,omitempty"`
	Workload string `yaml:"workload,omitempty"`
	Path     string `yaml:"path,omitempty"`
	Status   string `yaml:"status,omitempty"`
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
// GITHUB ACTIONS RUNNERS (CodeBuild-hosted)
// ============================================================================

// GitHubRunners is the github_runners block: one CodeBuild project per runner,
// each registered with GitHub through a webhook that fires on queued workflow
// jobs. Generation reads the raw map, not this struct; the struct exists so a
// TUI or API save round-trips the block instead of dropping it.
type GitHubRunners struct {
	Enabled bool `yaml:"enabled"`
	// TokenSecret is the NAME (or ARN) of a Secrets Manager secret holding the
	// GitHub token CodeBuild uses. It is created outside Terraform.
	TokenSecret string `yaml:"token_secret,omitempty"`
	// Owner is the GitHub user or organization the runners' webhooks live
	// under. Usually left unset: generate derives it from the runners.
	Owner string `yaml:"owner,omitempty"`
	// VPCAccess runs the builds inside the environment's private subnets.
	// Requires a NAT egress strategy; see validateGitHubRunnersConfigMap.
	VPCAccess bool           `yaml:"vpc_access,omitempty"`
	Runners   []GitHubRunner `yaml:"runners,omitempty"`
}

// GitHubRunner is one CodeBuild project serving one repository or one
// organization. Exactly one of Repository and Organization is set.
type GitHubRunner struct {
	Name         string `yaml:"name"`
	Repository   string `yaml:"repository,omitempty"`
	Organization string `yaml:"organization,omitempty"`
	ComputeType  string `yaml:"compute_type,omitempty"`
	Image        string `yaml:"image,omitempty"`
	Docker       bool   `yaml:"docker,omitempty"`
	// TimeoutMinutes is a pointer so an explicit value survives a round-trip
	// distinguishable from an absent key, which the template defaults to 60.
	TimeoutMinutes *int                 `yaml:"timeout_minutes,omitempty"`
	EnvVariables   []EnvVariable        `yaml:"env_variables,omitempty"`
	Secrets        []GitHubRunnerSecret `yaml:"secrets,omitempty"`
}

// GitHubRunnerSecret is an environment variable CodeBuild resolves at build
// start. Exactly one of SSM (a Parameter Store name) and SecretsManager
// (name-or-arn[:json-key]) is set.
type GitHubRunnerSecret struct {
	Name           string `yaml:"name"`
	SSM            string `yaml:"ssm,omitempty"`
	SecretsManager string `yaml:"secrets_manager,omitempty"`
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

// createEnv builds a new environment config. vpcCIDR is written explicitly
// into the YAML; callers get one from newEnvVPCCIDR so new environments do not
// all share 10.0.0.0/16.
func createEnv(name, env, vpcCIDR string) Env {
	policy := policyForEnvName(env)
	return Env{
		SchemaVersion: CurrentSchemaVersion, // Always create with latest schema version
		Project:       name,
		Env:           env,
		IsProd:        policy.IsProd,
		AutoDeploy:    boolPtr(policy.AutoDeploy),
		Region:        "", // Will be filled when AWS profile is selected
		AccountID:     "", // Will be filled when AWS profile is selected
		AWSProfile:    "", // Will be filled when AWS profile is selected
		StateBucket:   fmt.Sprintf("sate-bucket-%s-%s-%s", name, env, generateRandomString(5)),
		StateFile:     "state.tfstate",
		// VPC Configuration (schema v6)
		// Default to custom VPC for new projects (simpler, more control)
		// Always creates 2 AZs with public subnets only, no NAT gateway
		UseDefaultVPC: false,   // Use custom VPC by default
		VPCCIDR:       vpcCIDR, // Random free /16 from newEnvVPCCIDR, never re-randomised
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
			// A placeholder, and it has to be all three of these at once.
			//
			// Valid IAM: an empty list renders `"sub": []`, which AWS rejects at
			// apply with MalformedPolicyDocument, so "no default" is not an
			// option here the way it is in the Terraform module.
			//
			// Matching no real token: the previous default was
			// "repo:MadAppGang/*", which matches every token issued to a workflow
			// in MadAppGang's own repositories. For anyone who is not MadAppGang
			// that shipped default granted a third-party organisation the ability
			// to assume their deploy role — iam:PassRole over their task roles,
			// ECR push, ecs:UpdateService — in their own AWS account.
			//
			// Obviously unfinished: nobody can mistake this for a working value,
			// which is the whole point of a placeholder that must be edited.
			//
			// EnableGithubOIDC is false above, so a new project never meets this
			// unintentionally; it is read only once somebody turns the feature on.
			GithubOIDCSubjects:      []string{"repo:YOUR-GITHUB-ORG/YOUR-REPO:ref:refs/heads/main"},
			BackendContainerCommand: nil,
			InstallPgAdmin:          false,
			PgAdminEmail:            "",
			XrayEnabled:             false,
			BackendEnvVariables:     map[string]string{"TEST": "passed"},
			BackendPolicies:         []Policy{},
			// Backend scaling defaults (schema v4)
			BackendDesiredCount:           1,
			BackendAutoscalingEnabled:     false,
			BackendAutoscalingMinCapacity: 1,
			BackendAutoscalingMaxCapacity: 4,
			BackendCPU:                    "256",
			BackendMemory:                 "512",
			BackendALBDomainName:          "",
			// Compute defaults (schema v26). Fargate, and no compute block at
			// all: a new project has no capacity pools, which is the correct
			// empty state rather than a placeholder someone has to delete.
			BackendRuntime:     "fargate",
			BackendComputePool: "",
		},
		Domain: Domain{
			Enabled:            false,
			CreateDomainZone:   true,
			DomainName:         "",
			AddEnvDomainPrefix: policy.domainPrefix(),
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
	if err := checkSchemaNotNewer(name, detectSchemaVersion(e)); err != nil {
		return nil, err
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
