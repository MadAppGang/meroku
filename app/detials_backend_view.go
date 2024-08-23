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
					title:             "Project name",
					placeholder:       "Facebook",
					description:       "The name of the project",
					validator:         regexp.MustCompile(`^[a-z][a-z0-9-]{4,}$`),
					validationMessage: "minimum 5 characters, all lowercases, only letters from a-z",
				}, stringValue{e.project}),
				newTextFieldModel(baseInputModel{
					title:             "Environment",
					placeholder:       "dev",
					description:       "You can use any name, 2 letter minimum, 'prod' is special",
					validator:         regexp.MustCompile(`^[a-z]{2,}$`),
					validationMessage: "minimum 2 characters, all lowercases, only letters from a-z",
				}, stringValue{e.env}),
				newTextFieldModel(baseInputModel{
					title:             "State Bucket",
					placeholder:       "my-bucket-1",
					description:       "Infrastructure state bucket, it's better to keep infrastructure state in a separate bucket",
					validator:         regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9-]{5,}$`),
					validationMessage: "minimum 5 characters, only letters from a-z, numbers and dash",
				}, stringValue{e.sateBucket}),
				newTextFieldModel(baseInputModel{
					title:             "State File name",
					placeholder:       "state.tfstate",
					description:       "Infrastructure state file name, you can keep default value",
					validator:         regexp.MustCompile(`^[\w\-. ]+\.[A-Za-z0-9]{1,10}$`),
					validationMessage: "Just a regular final file name, no spaces, no special characters",
				}, stringValue{e.stateFile}),
			},
		},
		w: e.workload,
	}
}
