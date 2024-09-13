package main

import (
	"log/slog"
	"os"
	"path/filepath"

	"github.com/charmbracelet/bubbles/viewport"
)

type appSyncView struct {
	detailViewModel

	c AppSync
}

func newAppSyncView(e Env) *appSyncView {
	m := &appSyncView{
		detailViewModel: detailViewModel{
			title:       "AWS AppSync settings for pubsub and other",
			description: "The settings for AppSync to handle pubsub and other resolvers",
			inputs: []inputModel{
				newBoolFieldModel(baseInputModel{
					title:       "Enable AppSync",
					description: "If you want to enable AppSync for your project, it is amazing way to have serverless pubsub for public clients.",
				}, boolValue{e.AppSyncPubSub.Enabled}),
				newTextFieldModel(baseInputModel{
					title:       "Rewrite Graphql schema",
					description: "If you want custom Graphql schema, the new file will be located here: ./custom/appsync/schema.graphql",
				}, boolValue{e.AppSyncPubSub.Schema}),
				newBoolFieldModel(baseInputModel{
					title:       "Customize Auth lambda implementation",
					description: "If you want to secure pub/sub (and you probably do), use custom authenticator. The location of new implementation will be in ./custom/appsync/auth_lambda/ folder",
				}, boolValue{e.AppSyncPubSub.AuthLambda}),
				newBoolFieldModel(baseInputModel{
					title:       "Custom VTL resolvers",
					description: "YAML implementation of VTL resolvers, the custom implementation will be located in ./custom/appsync/vtl_template.yaml file.",
				}, boolValue{e.AppSyncPubSub.Resolvers}),
			},
		},
		c: e.AppSyncPubSub,
	}

	m.viewport = viewport.New(0, 0)
	m.updateViewportContent()
	return m
}

func (m *appSyncView) env(e Env) Env {
	c := AppSync{}
	c.Enabled = m.inputs[0].value().Bool()
	c.Schema = m.inputs[1].value().Bool()
	c.AuthLambda = m.inputs[2].value().Bool()
	c.Resolvers = m.inputs[3].value().Bool()
	if c.Enabled {
		if c.Schema {
			createCustomAppSyncSchema()
		}
		if c.AuthLambda {
			createCustomAppSyncAuthLambda()
		}
		if c.Resolvers {
			createCustomAppSyncResolvers()
		}
	}
	return e
}

func createCustomAppSyncSchema() {
	customSchemaPath := "./custom/appsync/schema.graphql"
	moduleSchemaPath := "./infrastructure/modules/appsync/schema.graphql"

	// Check if the custom schema file already exists
	if _, err := os.Stat(customSchemaPath); os.IsNotExist(err) {
		// Create the directory if it doesn't exist
		err := os.MkdirAll(filepath.Dir(customSchemaPath), 0o755)
		if err != nil {
			slog.Error("Failed to create directory for custom schema", "error", err)
			panic(1)
		}

		// Copy the schema file from the modules directory using CopyFile
		err = CopyFile(moduleSchemaPath, customSchemaPath)
		if err != nil {
			slog.Error("Failed to copy schema file", "error", err)
			panic(1)
		}

		slog.Info("Custom schema file created successfully")
	}
}

func createCustomAppSyncAuthLambda() {
	customAuthLambdaPath := "./custom/appsync/auth_lambda"
	moduleAuthLambdaPath := "./infrastructure/modules/appsync/auth_lambda"

	// Check if the custom auth lambda folder exists
	if _, err := os.Stat(customAuthLambdaPath); os.IsNotExist(err) {
		// Create the directory if it doesn't exist
		err := os.MkdirAll(customAuthLambdaPath, 0o755)
		if err != nil {
			slog.Error("Failed to create custom auth lambda directory", "error", err)
			panic(1)
		}

		// Copy the contents of the module auth lambda folder to the custom folder
		err = CopyFolder(moduleAuthLambdaPath, customAuthLambdaPath)
		if err != nil {
			slog.Error("Failed to copy auth lambda files", "error", err)
			panic(1)
		}
		slog.Info("Custom auth lambda folder created and files copied successfully")
	}
}

func createCustomAppSyncResolvers() {
	customResolversPath := "./custom/appsync/vtl_template.yaml"
	moduleResolversPath := "./infrastructure/modules/appsync/vtl_template.yaml"

	// Check if the custom resolvers file already exists
	if _, err := os.Stat(customResolversPath); os.IsNotExist(err) {
		// Create the directory if it doesn't exist
		err := os.MkdirAll(filepath.Dir(customResolversPath), 0o755)
		if err != nil {
			slog.Error("Failed to create directory for custom resolvers", "error", err)
			panic(1)
		}

		// Copy the resolvers file from the modules directory using CopyFile
		err = CopyFile(moduleResolversPath, customResolversPath)
		if err != nil {
			slog.Error("Failed to copy resolvers file", "error", err)
			panic(1)
		}

		slog.Info("Custom resolvers file created successfully")
	}
}
