package main

import (
	"fmt"
	"math/rand"
	"os"

	"gopkg.in/yaml.v2"
)

type Env struct {
	SchemaVersion       int                  `yaml:"schema_version,omitempty"`
	Project             string               `yaml:"project"`
	Env                 string               `yaml:"env"`
	IsProd              bool                 `yaml:"is_prod"`
	Region              string               `yaml:"region"`
	AccountID           string               `yaml:"account_id"`
	AWSProfile          string               `yaml:"aws_profile"`
	StateBucket         string               `yaml:"state_bucket"`
	StateFile           string               `yaml:"state_file"`
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
}

type AppSync struct {
	Enabled    bool `yaml:"enabled"`
	Schema     bool `yaml:"schema"`
	AuthLambda bool `yaml:"auth_lambda"`
	Resolvers  bool `yaml:"resolvers"`
}

type Workload struct {
	BackendHealthEndpoint      string            `yaml:"backend_health_endpoint"`
	BackendExternalDockerImage string            `yaml:"backend_external_docker_image"`
	BackendContainerCommand    string            `yaml:"backend_container_command"`
	BucketPostfix              string            `yaml:"bucket_postfix"`
	BucketPublic               bool              `yaml:"bucket_public"`
	BackendImagePort           int               `yaml:"backend_image_port"`
	SetupFCNSNS                bool              `yaml:"setup_fcnsns"`
	XrayEnabled                bool              `yaml:"xray_enabled"`
	BackendEnvVariables        map[string]string `yaml:"backend_env_variables"`
	Policies                   []string          `yaml:"policies"`
	BackendPolicies            []Policy          `yaml:"backend_policies"`
	EnvFilesS3                 []S3EnvFile       `yaml:"env_files_s3"`

	SlackWebhook       string   `yaml:"slack_webhook"`
	EnableGithubOIDC   bool     `yaml:"enable_github_oidc"`
	GithubOIDCSubjects []string `yaml:"github_oidc_subjects"`

	InstallPgAdmin bool   `yaml:"install_pg_admin"`
	PgAdminEmail   string `yaml:"pg_admin_email"`
	
	BackendALBDomainName string `yaml:"backend_alb_domain_name"`
	
	// Backend scaling configuration
	BackendDesiredCount              int32  `yaml:"backend_desired_count"`
	BackendAutoscalingEnabled        bool   `yaml:"backend_autoscaling_enabled"`
	BackendAutoscalingMinCapacity    int32  `yaml:"backend_autoscaling_min_capacity"`
	BackendAutoscalingMaxCapacity    int32  `yaml:"backend_autoscaling_max_capacity"`
	BackendCPU                       string `yaml:"backend_cpu"`
	BackendMemory                    string `yaml:"backend_memory"`
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

type Domain struct {
	// EXISTING FIELDS - DON'T TOUCH
	Enabled            bool   `yaml:"enabled"`
	CreateDomainZone   bool   `yaml:"create_domain_zone"`
	DomainName         string `yaml:"domain_name"`  // Keep as-is - always root
	IsDNSRoot          bool   `yaml:"is_dns_root"`
	DNSRootAccountID   string `yaml:"dns_root_account_id"`
	DelegationRoleArn  string `yaml:"delegation_role_arn"`
	
	// Additional fields from original structure (if missing)
	APIDomainPrefix    string `yaml:"api_domain_prefix,omitempty"`
	AddEnvDomainPrefix bool   `yaml:"add_env_domain_prefix,omitempty"`
	
	// NEW DNS MANAGEMENT FIELDS
	ZoneID        string `yaml:"zone_id,omitempty"`         // For existing zones
	RootZoneID    string `yaml:"root_zone_id,omitempty"`    // For subdomain delegation
	RootAccountID string `yaml:"root_account_id,omitempty"` // For cross-account access
}

type PostgresEngineVersion string

type Postgres struct {
	Enabled            bool    `yaml:"enabled"`
	Dbname             string  `yaml:"dbname"`
	Username           string  `yaml:"username"`
	PublicAccess       bool    `yaml:"public_access"`
	EngineVersion      string  `yaml:"engine_version"`
	Aurora             bool    `yaml:"aurora"`
	MinCapacity        float64 `yaml:"min_capacity"`
	MaxCapacity        float64 `yaml:"max_capacity"`
	// RDS-specific fields (when aurora is false)
	InstanceClass                     string `yaml:"instance_class"`
	AllocatedStorage                  int    `yaml:"allocated_storage"`
	StorageType                       string `yaml:"storage_type"`
	MultiAZ                           bool   `yaml:"multi_az"`
	StorageEncrypted                  bool   `yaml:"storage_encrypted"`
	DeletionProtection                bool   `yaml:"deletion_protection"`
	SkipFinalSnapshot                 bool   `yaml:"skip_final_snapshot"`
	IAMDatabaseAuthenticationEnabled  bool   `yaml:"iam_database_authentication_enabled"`
}

type Cognito struct {
	Enabled                bool     `yaml:"enabled"`
	EnableWebClient        bool     `yaml:"enable_web_client"`
	EnableDashboardClient  bool     `yaml:"enable_dashboard_client"`
	DashboardCallbackURLs  []string `yaml:"dashboard_callback_ur_ls"`
	EnableUserPoolDomain   bool     `yaml:"enable_user_pool_domain"`
	UserPoolDomainPrefix   string   `yaml:"user_pool_domain_prefix"`
	BackendConfirmSignup   bool     `yaml:"backend_confirm_signup"`
	AutoVerifiedAttributes []string `yaml:"auto_verified_attributes"`
}

type Ses struct {
	Enabled    bool     `yaml:"enabled"`
	DomainName string   `yaml:"domain_name"`
	TestEmails []string `yaml:"test_emails"`
}

type Sqs struct {
	Enabled bool   `yaml:"enabled"`
	Name    string `yaml:"name"`
}

type ALB struct {
	Enabled bool `yaml:"enabled"`
}

type ScheduledTask struct {
	Name                string     `yaml:"name"`
	Schedule            string     `yaml:"schedule"`
	ExternalDockerImage string     `yaml:"docker_image"`
	ContainerCommand    string     `yaml:"container_command"`
	ECRConfig           *ECRConfig `yaml:"ecr_config,omitempty"` // Schema v9
}

type EventProcessorTask struct {
	Name                string     `yaml:"name"`
	RuleName            string     `yaml:"rule_name"`
	DetailTypes         []string   `yaml:"detail_types"`
	Sources             []string   `yaml:"sources"`
	ExternalDockerImage string     `yaml:"docker_image"`
	ContainerCommand    []string   `yaml:"container_command"`
	ECRConfig           *ECRConfig `yaml:"ecr_config,omitempty"` // Schema v9
}

type EnvVariable struct {
	Name  string `yaml:"name"`
	Value string `yaml:"value"`
}

type Service struct {
	Name             string            `yaml:"name"`
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
	RootDomain     string           `yaml:"root_domain"`
	RootAccount    DNSRootAccount   `yaml:"root_account"`
	DelegatedZones []DelegatedZone  `yaml:"delegated_zones"`
}

type DNSRootAccount struct {
	AccountID         string `yaml:"account_id"`
	ZoneID            string `yaml:"zone_id"`
	DelegationRoleArn string `yaml:"delegation_role_arn"`
}

type DelegatedZone struct {
	Subdomain  string   `yaml:"subdomain"`
	AccountID  string   `yaml:"account_id"`
	ZoneID     string   `yaml:"zone_id"`
	NSRecords  []string `yaml:"ns_records"`
	Status     string   `yaml:"status"`
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
	Name             string                 `yaml:"name"`
	GitHubRepository string                 `yaml:"github_repository"`
	GitHubOAuthToken string                 `yaml:"github_oauth_token,omitempty"`
	Branches         []AmplifyBranch        `yaml:"branches"`
	SubdomainPrefix  string                 `yaml:"subdomain_prefix,omitempty"`    // NEW: Auto-constructs domain
	CustomDomain     string                 `yaml:"custom_domain,omitempty"`       // For manual override
	EnvVariables     map[string]string      `yaml:"environment_variables,omitempty"` // App-level env vars
}

// AmplifyBranch represents a branch configuration for an Amplify app
type AmplifyBranch struct {
	Name                      string            `yaml:"name"`
	Stage                     string            `yaml:"stage,omitempty"`                           // PRODUCTION, DEVELOPMENT, BETA, EXPERIMENTAL
	EnableAutoBuild           bool              `yaml:"enable_auto_build,omitempty"`
	EnablePullRequestPreview  bool              `yaml:"enable_pull_request_preview,omitempty"`
	EnvironmentVariables      map[string]string `yaml:"environment_variables,omitempty"`
	CustomSubdomains          []string          `yaml:"custom_subdomains,omitempty"`              // For branch-specific subdomains
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
		ECRStrategy:         "local",
		ECRAccountID:        "",
		ECRAccountRegion:    "",
		ECRTrustedAccounts:  []ECRTrustedAccount{},
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
			BackendContainerCommand:    "",
			InstallPgAdmin:             false,
			PgAdminEmail:               "",
			XrayEnabled:                false,
			BackendEnvVariables:        map[string]string{"TEST": "passed"},
			BackendPolicies:            []Policy{},
			// Backend scaling defaults (schema v4)
			BackendDesiredCount:              1,
			BackendAutoscalingEnabled:        false,
			BackendAutoscalingMinCapacity:    1,
			BackendAutoscalingMaxCapacity:    4,
			BackendCPU:                       "256",
			BackendMemory:                    "512",
			BackendALBDomainName:             "",
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
		},
		Buckets:             []BucketConfig{},
		Services:            []Service{},
		ScheduledTasks:      []ScheduledTask{},
		EventProcessorTasks: []EventProcessorTask{},
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

func loadEnvToMap(name string) (map[string]interface{}, error) {
	var e map[string]interface{}

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
