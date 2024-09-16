package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/charmbracelet/bubbles/list"
	"golang.org/x/exp/rand"
	"gopkg.in/yaml.v3"
)

// main list items
type item struct {
	title      string
	desc       string
	detailView detailView
	children   []item
	isParent   bool
	isExpanded bool
	isChild    bool
}

func (i item) Title() string       { return i.title }
func (i item) Description() string { return i.desc }
func (i item) FilterValue() string { return i.title }

func menuListFromEnv(env Env) []list.Item {
	scheduledTasks := []item{}
	for _, task := range env.ScheduledTasks {
		scheduledTasks = append(scheduledTasks, item{title: task.Name, desc: fmt.Sprintf("Scheduled task with schedule: %s", task.Schedule), isChild: true, detailView: newScheduledTaskView(task)})
	}
	scheduledTasks = append(scheduledTasks, item{title: ADD_NEW_SCHEDULED_TASK, desc: "Add a new scheduled task", isChild: true})

	eventProcessorTasks := []item{}
	for _, task := range env.EventProcessorTasks {
		eventProcessorTasks = append(eventProcessorTasks, item{title: task.Name, desc: task.RuleName, isChild: true, detailView: NewEventProcessorTaskView(task)})
	}
	eventProcessorTasks = append(eventProcessorTasks, item{title: ADD_NEW_EVENT_TASK, desc: "Add a new event processor task", isChild: true})

	items := []list.Item{
		item{title: "Main settings", desc: "Main settings", detailView: newMainSettingsView(env)},
		item{title: "Backend settings", desc: "Backend and environment settings", detailView: newBackendSettingsView(env)},
		item{title: "Domain", desc: "Domain settings", detailView: newBackendDomainView(env)},
		item{title: "Postgres", desc: "Postgres database in RDS settings", detailView: newBackendPostgresView(env)},
		item{title: "Cognito", desc: "Cognito settings", detailView: newCognitoView(env)},
		item{title: "SES Email", desc: "Simple email service settings", detailView: newSesView(env)},
		item{title: "Scheduled ECS Task", desc: "mange list of scheduled ECS tasks", detailView: nil, isParent: true, children: scheduledTasks},
		item{title: "Event Processor Task", desc: "mange list of event processor tasks EventBridge", detailView: nil, isParent: true, children: eventProcessorTasks},
		item{title: "PubSub with AppSync", desc: "Create AppSync for PubSum", detailView: newAppSyncView(env)},
	}
	return items
}

type Env struct {
	Project             string               `yaml:"project"`
	Env                 string               `yaml:"env"`
	IsProd              bool                 `yaml:"is_prod"`
	Region              string               `yaml:"region"`
	StateBucket         string               `yaml:"state_bucket"`
	StateFile           string               `yaml:"state_file"`
	Workload            Workload             `yaml:"workload"`
	Domain              Domain               `yaml:"domain"`
	Postgres            Postgres             `yaml:"postgres"`
	Cognito             Cognito              `yaml:"cognito"`
	Ses                 Ses                  `yaml:"ses"`
	ScheduledTasks      []ScheduledTask      `yaml:"scheduled_tasks"`
	EventProcessorTasks []EventProcessorTask `yaml:"event_processor_tasks"`
	AppSyncPubSub       AppSync              `yaml:"pubsub_appsync"`
}

type AppSync struct {
	Enabled    bool `yaml:"enabled"`
	Schema     bool `yaml:"schema"`
	AuthLambda bool `yaml:"auth_lambda"`
	Resolvers  bool `yaml:"resolvers"`
}

type Workload struct {
	BackendHealthEndpoint      string `yaml:"backend_health_endpoint"`
	BackendExternalDockerImage string `yaml:"backend_external_docker_image"`
	BackendContainerCommand    string `yaml:"backend_container_command"`
	BucketPostfix              string `yaml:"bucket_postfix"`
	BucketPublic               bool   `yaml:"bucket_public"`
	BackendImagePort           int    `yaml:"backend_image_port"`
	SetupFCNSNS                bool   `yaml:"setup_fcnsns"`
	XrayEnabled                bool   `yaml:"xray_enabled"`
	BackendEnvVariables        string `yaml:"backend_env_variables"`

	SlackWebhook       string   `yaml:"slack_webhook"`
	EnableGithubOIDC   bool     `yaml:"enable_github_oidc"`
	GithubOIDCSubjects []string `yaml:"github_oidc_subjects"`

	InstallPgAdmin bool   `yaml:"install_pg_admin"`
	PgAdminEmail   string `yaml:"pg_admin_email"`
}

type SetupDomainType string

type Domain struct {
	Enabled     bool   `yaml:"enabled"`
	UseExistent bool   `yaml:"use_existent"`
	DomainName  string `yaml:"domain_name"`
}

type PostgresEngineVersion string

type Postgres struct {
	Enabled       bool   `yaml:"enabled"`
	Dbname        string `yaml:"dbname"`
	Username      string `yaml:"username"`
	PublicAccess  bool   `yaml:"public_access"`
	EngineVersion string `yaml:"engine_version"`
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

type ScheduledTask struct {
	Name                string `yaml:"name"`
	Schedule            string `yaml:"schedule"`
	ExternalDockerImage string `yaml:"docker_image"`
	ContainerCommand    string `yaml:"container_command"`
}

type EventProcessorTask struct {
	Name                string   `yaml:"name"`
	RuleName            string   `yaml:"rule_name"`
	DetailTypes         []string `yaml:"detail_types"`
	Sources             []string `yaml:"sources"`
	ExternalDockerImage string   `yaml:"docker_image"`
	ContainerCommand    string   `yaml:"container_command"`
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
		Project:     name,
		Env:         env,
		IsProd:      false,
		Region:      "us-east-1",
		StateBucket: fmt.Sprintf("sate-bucket-%s-%s-%s", name, env, generateRandomString(5)),
		StateFile:   "state.tfstate",
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
			BackendEnvVariables:        `TEST=passed`,
		},
		Domain: Domain{
			Enabled:     false,
			UseExistent: false,
			DomainName:  "",
		},
		Postgres: Postgres{
			Enabled:       false,
			Dbname:        "",
			Username:      "",
			PublicAccess:  false,
			EngineVersion: "16.x",
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
		ScheduledTasks:      []ScheduledTask{},
		EventProcessorTasks: []EventProcessorTask{},
	}
}

func loadEnv(name string) (Env, error) {
	var e Env

	data, err := os.ReadFile(name + ".yaml")
	if err != nil {
		wd, err := os.Getwd()
		if err != nil {
			return e, fmt.Errorf("error getting current working directory: %v", err)
		}
		return e, fmt.Errorf("error reading YAML file: %v, current folder: %s", err, wd)
	}

	err = yaml.Unmarshal(data, &e)
	if err != nil {
		return e, fmt.Errorf("error unmarshaling YAML: %v", err)
	}

	return e, nil
}

func loadEnvToMap(name string) (map[string]interface{}, error) {
	var e map[string]interface{}

	data, err := os.ReadFile(name)
	if err != nil {
		return nil, fmt.Errorf("error reading YAML file: %v", err)
	}

	err = yaml.Unmarshal(data, &e)
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling YAML: %v", err)
	}

	return e, nil
}

func saveEnv(e Env) error {
	yamlData, err := yaml.Marshal(e)
	if err != nil {
		slog.Error("saveEnv", "error", err)
		return err
	}
	filename := e.Env + ".yaml"
	return os.WriteFile(filename, yamlData, 0o644)
}

var AWSRegions = []string{
	"us-east-1",
	"us-east-2",
	"us-west-1",
	"us-west-2",
	"af-south-1",
	"ap-east-1",
	"ap-south-1",
	"ap-northeast-1",
	"ap-northeast-2",
	"ap-northeast-3",
	"ap-southeast-1",
	"ap-southeast-2",
	"ap-northeast-3",
	"ca-central-1",
	"eu-central-1",
	"eu-west-1",
	"eu-west-2",
	"eu-south-1",
	"eu-west-3",
	"eu-north-1",
	"me-south-1",
	"sa-east-1",
}
