package main

import (
	"regexp"

	"github.com/charmbracelet/bubbles/viewport"
)

type backendSettingsView struct {
	detailViewModel

	w Workload
}

func newBackendSettingsView(e Env) *backendSettingsView {
	m := &backendSettingsView{
		detailViewModel: detailViewModel{
			title:       "Backend settings",
			description: "Backend and main workload settings",
			inputs: []inputModel{
				newTextFieldModel(baseInputModel{
					title:       "Health endpoint",
					placeholder: "/health/live",
					description: "Optional health endpoint, if it is not responding with status 200, the application load balancer will consider the service unhealthy and redeploy it",
				}, stringValue{e.Workload.BackendHealthEndpoint}),
				newTextFieldModel(baseInputModel{
					title:       "External Docker image",
					placeholder: "madappgang/aooth",
					description: "Optional Docker hub image name, by default we use private ECR registry",
				}, stringValue{e.Workload.BackendExternalDockerImage}),
				newTextFieldModel(baseInputModel{
					title:             "custom docker container command",
					placeholder:       "[\"aooth\", \"--flag\"]",
					description:       "Optional overwrite default docker container command",
					validator:         regexp.MustCompile(`(^\s*$|\[(\s*"[^"]*"\s*,?\s*)*\])`),
					validationMessage: "Container command is JSON  array of strings format",
				}, stringValue{e.Workload.BackendContainerCommand}),
				newTextFieldModel(baseInputModel{
					title:             "Bucket postfix",
					placeholder:       "hidden",
					description:       "Backend has it's own S3 bucket with specific name, you can add postfix to this name",
					validator:         regexp.MustCompile(`^[a-zA-Z0-9-]{0,30}$`),
					validationMessage: "Letters, numbers and dash only, max 30 characters",
				}, stringValue{e.Workload.BucketPostfix}),
				newTextFieldModel(baseInputModel{
					title:             "Backend docker image port",
					placeholder:       "8000",
					description:       "Backend docker image port",
					validator:         regexp.MustCompile(`^($|([1-9]\d{0,3}|[1-5]\d{4}|6[0-4]\d{3}|65[0-4]\d{2}|655[0-2]\d|6553[0-5]))$`),
					validationMessage: "Port number from 1 to 65535",
				}, intValue{e.Workload.BackendImagePort}),
				newBoolFieldModel(baseInputModel{
					title:       "setupFCNSNS",
					description: "Optional you can setup SNS topic for push notifications",
				}, boolValue{e.Workload.SetupFCNSNS}),
				newBoolFieldModel(baseInputModel{
					title:       "Enable XRay",
					description: "Setup Xray daemon as a service in ECS",
				}, boolValue{e.Workload.XrayEnabled}),
				newTextFieldModel(baseInputModel{
					title:             "Slack deployment webhook",
					placeholder:       "https://hooks.slack.com/services/T00000000/B00000000/XXXXXXXXXXXXXXXXXXXXXXXX",
					description:       "Deployment script will send slack message with deployment status",
					validator:         regexp.MustCompile(`^$|(https?:\/\/)?([\da-z\.-]+)\.([a-z\.]{2,6})([\/\w \.-]*)*\/?$`),
					validationMessage: "Valid URL",
				}, stringValue{e.Workload.SlackWebhook}),
				newBoolFieldModel(baseInputModel{
					title:       "Enable Github OIDC",
					description: "This will allow github actions to have access to AWS infrastructure to push ECR images and deploy services",
				}, boolValue{e.Workload.EnableGithubOIDC}),
				newTextFieldModel(baseInputModel{
					title:       "Github OIDC subjects",
					placeholder: "repo:MadAppGang/*",
					description: "The list of github subject, usually it is a list of repositories, like repo:MadAppGang/project_backend:ref:refs/heads/main",
				}, sliceValue{e.Workload.GithubOIDCSubjects}),
				newBoolFieldModel(baseInputModel{
					title:       "Install PgAdmin",
					description: "Install PgAdmin as a service in ECS",
				}, boolValue{e.Workload.InstallPgAdmin}),
				newTextFieldModel(baseInputModel{
					title:             "PgAdmin admin email",
					placeholder:       "admin@admin.com",
					description:       "PgAdmin login email credentials, the password will be generated automatically",
					validator:         regexp.MustCompile(`^$|^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`),
					validationMessage: "Valid email address",
				}, stringValue{e.Workload.PgAdminEmail}),
			},
		},
		w: e.Workload,
	}

	m.viewport = viewport.New(0, 0)
	m.updateViewportContent()
	return m
}
