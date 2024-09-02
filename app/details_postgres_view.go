package main

import (
	"regexp"

	"github.com/charmbracelet/bubbles/viewport"
)

type backendPostgresView struct {
	detailViewModel

	p Postgres
}

func newBackendPostgresView(e Env) *backendPostgresView {
	m := &backendPostgresView{
		detailViewModel: detailViewModel{
			title:       "Postgres RDS settings",
			description: "Optional Postgres RDS settings",
			inputs: []inputModel{
				newBoolFieldModel(baseInputModel{
					title:       "Enable Postgres in RDS",
					description: "Enable Postgres in RDS and set up database and admin user",
				}, boolValue{e.Postgres.Enabled}),
				newTextFieldModel(baseInputModel{
					title:             "Database name",
					placeholder:       "MyDatabase1",
					description:       "The name of the database will be forwarded as PG_DATABASE_NAME environment variable to backend",
					validator:         regexp.MustCompile(`^([a-zA-Z_][a-zA-Z0-9_$]*)?$`),
					validationMessage: "Valid database name",
				}, stringValue{e.Postgres.Dbname}),
				newTextFieldModel(baseInputModel{
					title:             "Admin user name",
					placeholder:       "pgadmin",
					description:       "The password will be generated automatically and saved in AWS SSM parameter store",
					validator:         regexp.MustCompile(`^([a-z_][a-z0-9_$]*)?$`),
					validationMessage: "Valid postgres user name",
				}, stringValue{e.Postgres.Dbname}),
				newTextFieldModel(baseInputModel{
					title:       "Enable public access",
					description: "Database is available from public internet",
				}, boolValue{e.Postgres.PublicAccess}),
				newTextFieldModel(baseInputModel{
					title:       "Engine version",
					description: "Postgres engine version",
				}, sliceSelectValue{index: 0, value: []string{"11.x", "12.x", "13.x", "14.x", "15.x", "16.x"}}),
			},
		},
		p: e.Postgres,
	}

	m.viewport = viewport.New(0, 0)
	m.updateViewportContent()
	return m
}

func (m *backendPostgresView) env(e Env) Env {
	p := Postgres{}
	p.Enabled = m.inputs[0].value().Bool()
	p.Dbname = m.inputs[1].value().String()
	p.Username = m.inputs[2].value().String()
	p.PublicAccess = m.inputs[3].value().Bool()
	p.EngineVersion = m.inputs[4].value().String()
	e.Postgres = p
	return e
}
