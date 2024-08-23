package main

import "regexp"

type backendSettingsView struct {
	detailViewModel

	w workload
}

func newBackendSettingsView(e env) *backendSettingsView {
	return &backendSettingsView{
		detailViewModel: detailViewModel{
			title:       "Backend settings",
			description: "Backend and main workload settings",
			inputs: []inputModel{
				newTextFieldModel(baseInputModel{
					title:       "Health endpoint",
					placeholder: "/health/live",
					description: "Optional health endpoint, if it is not responding with status 200, the application load balancer will consider the service unhealthy and redeploy it",
				}, stringValue{e.workload.backendHealthEndpoint}),
				newTextFieldModel(baseInputModel{
					title:       "External Docker image",
					placeholder: "madappgang/aooth",
					description: "Optional Docker hub image name, by default we use private ECR registry",
				}, stringValue{e.workload.backendExternalDockerImage}),
				newTextFieldModel(baseInputModel{
					title:       "custom docker container command",
					placeholder: "[\"aooth\", \"--flag\"]",
					description: "Optional overwrite default docker container command",
				}, sliceValue{e.workload.backendContainerCommand}),
				newTextFieldModel(baseInputModel{
					title:             "Bucket postfix",
					placeholder:       "hidden",
					description:       "Backend has it's own S3 bucket with specific name, you can add postfix to this name",
					validator:         regexp.MustCompile(`^[a-zA-Z0-9-]{0,30}$`),
					validationMessage: "Letters, numbers and dash only, max 30 characters",
				}, stringValue{e.workload.bucketPostfix}),
			},
		},
		w: e.workload,
	}
}
