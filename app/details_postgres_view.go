package main

import (
	"regexp"

	"github.com/charmbracelet/bubbles/viewport"
)

type backendPostgresView struct {
	detailViewModel

	p postgres
}

func newBackendPostgresView(e env) *backendPostgresView {
	m := &backendPostgresView{
		detailViewModel: detailViewModel{
			title:       "Postgres RDS settings",
			description: "Optional Postgres RDS settings",
			inputs: []inputModel{
				newBoolFieldModel(baseInputModel{
					title:       "Enable Postgres in RDS",
					description: "Enable Postgres in RDS and set up database and admin user",
				}, boolValue{e.postgres.enabled}),
				newTextFieldModel(baseInputModel{
					title:             "Database name",
					placeholder:       "MyDatabase1",
					description:       "The name of the database will be forwarded as PG_DATABASE_NAME environment variable to backend",
					validator:         regexp.MustCompile(`^([a-zA-Z_][a-zA-Z0-9_$]*)?$`),
					validationMessage: "Valid database name",
				}, stringValue{e.postgres.dbname}),
				newTextFieldModel(baseInputModel{
					title:             "Admin user name",
					placeholder:       "pgadmin",
					description:       "The password will be generated automatically and saved in AWS SSM parameter store",
					validator:         regexp.MustCompile(`^([a-z_][a-z0-9_$]*)?$`),
					validationMessage: "Valid postgres user name",
				}, stringValue{e.postgres.dbname}),
			},
		},
		p: e.postgres,
	}

	m.viewport = viewport.New(0, 0)
	m.updateViewportContent()
	return m
}
