package main

import (
	"fmt"

	"github.com/charmbracelet/bubbles/list"
	"golang.org/x/exp/rand"
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

func menuListFromEnv(env env) []list.Item {
	scheduledTasks := []item{}
	for _, task := range env.scheduledTasks {
		scheduledTasks = append(scheduledTasks, item{title: task.name, desc: fmt.Sprintf("Scheduled task with schedule: %s", task.schedule), isChild: true, detailView: newScheduledTaskView(task)})
	}
	scheduledTasks = append(scheduledTasks, item{title: ADD_NEW_SCHEDULED_TASK, desc: "Add a new scheduled task", isChild: true})

	eventProcessorTasks := []item{}
	for _, task := range env.eventProcessorTasks {
		eventProcessorTasks = append(eventProcessorTasks, item{title: task.name, desc: task.ruleName, isChild: true, detailView: NewEventProcessorTaskView(task)})
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
	}
	return items
}

type env struct {
	project             string
	env                 string
	isProd              bool
	region              string
	sateBucket          string
	stateFile           string
	workload            workload
	domain              domain
	postgres            postgres
	cognito             cognito
	ses                 ses
	scheduledTasks      []scheduledTask
	eventProcessorTasks []eventProcessorTask
}

type workload struct {
	backendHealthEndpoint      string
	backendExternalDockerImage string
	backendContainerCommand    string
	bucketPostfix              string
	bucketPublic               bool
	backendImagePort           int
	setupFCNSNS                bool
	xrayEnabled                bool
	backendEnvVariables        map[string]string

	slackWebhook       string
	enableGithubOIDC   bool
	githubOIDCSubjects []string

	installPgAdmin bool
	pgAdminEmail   string
}

type SetupDomainType string

type domain struct {
	enabled     bool
	useExistent bool
	domainName  string
}

type postgresEngineVersion string

const (
	postgresEngineVersion11 postgresEngineVersion = "11"
	postgresEngineVersion12 postgresEngineVersion = "12"
	postgresEngineVersion13 postgresEngineVersion = "13"
	postgresEngineVersion14 postgresEngineVersion = "14"
	postgresEngineVersion15 postgresEngineVersion = "15"
	postgresEngineVersion16 postgresEngineVersion = "16"
)

type postgres struct {
	enabled       bool
	dbname        string
	username      string
	publicAccess  bool
	engineVersion postgresEngineVersion
}

type cognito struct {
	enabled                bool
	enableWebClient        bool
	enableDashboardClient  bool
	dashboardCallbackURLs  []string
	enableUserPoolDomain   bool
	userPoolDomainPrefix   string
	backendConfirmSignup   bool
	autoVerifiedAttributes []string
}

type ses struct {
	enabled    bool
	domainName string
	testEmails []string
}

type scheduledTask struct {
	name     string
	schedule string
}

type eventProcessorTask struct {
	name        string
	ruleName    string
	detailTypes []string
	sources     []string
}

// create function which generate random string
func generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, length)
	for i := 0; i < length; i++ {
		result[i] = charset[rand.Intn(len(charset))]
	}
	return string(result)
}

func createEnv(name string) env {
	return env{
		project:    name,
		env:        "dev",
		isProd:     false,
		region:     "us-east-1",
		sateBucket: fmt.Sprintf("sate-bucket-%s-%s-%s", name, "dev", generateRandomString(5)),
		stateFile:  "state.tfstate",
		workload: workload{
			slackWebhook:               "",
			bucketPostfix:              generateRandomString(5),
			bucketPublic:               true,
			backendHealthEndpoint:      "",
			backendExternalDockerImage: "",
			setupFCNSNS:                false,
			backendImagePort:           8080,
			enableGithubOIDC:           false,
			githubOIDCSubjects:         []string{"repo:MadAppGang/*", "repo:MadAppGang/project_backend:ref:refs/heads/main"},
			backendContainerCommand:    "",
			installPgAdmin:             false,
			pgAdminEmail:               "",
			xrayEnabled:                false,
			backendEnvVariables:        map[string]string{"TEST": "passed"},
		},
		domain: domain{
			enabled:     false,
			useExistent: false,
			domainName:  "",
		},
		postgres: postgres{
			enabled:       false,
			dbname:        "",
			username:      "",
			publicAccess:  false,
			engineVersion: postgresEngineVersion11,
		},
		cognito: cognito{
			enabled:                false,
			enableWebClient:        false,
			enableDashboardClient:  false,
			dashboardCallbackURLs:  []string{},
			enableUserPoolDomain:   false,
			userPoolDomainPrefix:   "",
			backendConfirmSignup:   false,
			autoVerifiedAttributes: []string{},
		},
		ses: ses{
			enabled:    false,
			domainName: "",
			testEmails: []string{"i@madappgang.com"},
		},
		scheduledTasks:      []scheduledTask{},
		eventProcessorTasks: []eventProcessorTask{},
	}
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
