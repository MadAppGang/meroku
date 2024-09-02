package main

import (
	"regexp"

	"github.com/charmbracelet/bubbles/viewport"
)

type sesView struct {
	detailViewModel

	s Ses
}

func newSesView(e Env) *sesView {
	m := &sesView{
		detailViewModel: detailViewModel{
			title:       "AWS Simple Email Service settings",
			description: "Optional AWS Simple Email Service settings",
			inputs: []inputModel{
				newBoolFieldModel(baseInputModel{
					title:       "Enable AWS SES",
					description: "If you want use AWS SES as your email service, enable it.",
				}, boolValue{e.Ses.Enabled}),
				newTextFieldModel(baseInputModel{
					title:             "Domain name for AWS SES",
					description:       "What email domain name you want to use for AWS SES",
					placeholder:       "email.madappgang.com",
					validator:         regexp.MustCompile(`^(([a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,12})?$`),
					validationMessage: "Valid domain name, with no scheme and no query string",
				}, stringValue{e.Ses.DomainName}),
				newTextFieldModel(baseInputModel{
					title:       "The list of test emails",
					description: "By default your SES will be in sandbox mode, only verified emails could receive emails. You need to write to support team. Before that you can send emails to these email addresses.",
				}, sliceValue{e.Ses.TestEmails}),
			},
		},
		s: e.Ses,
	}

	m.viewport = viewport.New(0, 0)
	m.updateViewportContent()
	return m
}

func (m *sesView) env(e Env) Env {
	s := Ses{}
	s.Enabled = m.inputs[0].value().Bool()
	s.DomainName = m.inputs[1].value().String()
	s.TestEmails = m.inputs[2].value().Slice()
	e.Ses = s
	return e
}
