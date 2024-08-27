package main

import (
	"regexp"

	"github.com/charmbracelet/bubbles/viewport"
)

type cognitoView struct {
	detailViewModel

	c cognito
}

func newCognitoView(e env) *cognitoView {
	m := &cognitoView{
		detailViewModel: detailViewModel{
			title:       "AWS Cognito settings",
			description: "Optional AWS Cognito settings",
			inputs: []inputModel{
				newBoolFieldModel(baseInputModel{
					title:       "Enable Cognito",
					description: "If you want use cognito as you authentication provider, enable it. But we recommend look to the Aooth first.",
				}, boolValue{e.cognito.enabled}),
				newBoolFieldModel(baseInputModel{
					title:       "Cognito Web client enabled",
					description: "Enable cognito web client, which will be used for authentication in web apps",
				}, boolValue{e.cognito.enableWebClient}),
				newBoolFieldModel(baseInputModel{
					title:       "Cognito API client enabled",
					description: "Enable API client to authenticate with API calls to Cognito",
				}, boolValue{e.cognito.enableDashboardClient}),
				newTextFieldModel(baseInputModel{
					title:       "Allowed Callback URLS",
					description: "Which Callbacks allowed for API client",
				}, sliceValue{e.cognito.dashboardCallbackURLs}),
				newBoolFieldModel(baseInputModel{
					title:       "Enable User Pool Domain",
					description: "Enable Domain specific for Cognito User Pool",
				}, boolValue{e.cognito.enableUserPoolDomain}),
				newTextFieldModel(baseInputModel{
					title:             "User Pool domain prefix",
					description:       "Prefix for the user pool",
					placeholder:       "dev",
					validator:         regexp.MustCompile(`^[a-z][a-zA-Z0-9]{1,}$`),
					validationMessage: "letter, and numbers only, min 1 character",
				}, stringValue{e.workload.pgAdminEmail}),
				newBoolFieldModel(baseInputModel{
					title:       "Backend confirm sign up",
					description: "Backend should confirm sign up",
				}, boolValue{e.cognito.backendConfirmSignup}),
				newTextFieldModel(baseInputModel{
					title:       "Auto verified attributes",
					description: "The list of attributes that will be verified by Cognito",
				}, sliceValue{e.cognito.autoVerifiedAttributes}),
			},
		},
		c: e.cognito,
	}

	m.viewport = viewport.New(0, 0)
	m.updateViewportContent()
	return m
}
