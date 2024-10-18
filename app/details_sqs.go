package main

import (
	"regexp"

	"github.com/charmbracelet/bubbles/viewport"
)

type sqsView struct {
	detailViewModel

	sqs Sqs
}

func newSqsView(e Env) *sqsView {
	m := &sqsView{
		detailViewModel: detailViewModel{
			title:       "AWS SQS settings",
			description: "Setup AWS SQS for your project",
			inputs: []inputModel{
				newBoolFieldModel(baseInputModel{
					title:       "Enable AWS SQS",
					description: "If you want use AWS SQS, enable it.",
				}, boolValue{e.Sqs.Enabled}),
				newTextFieldModel(baseInputModel{
					title:             "SQS queue name",
					description:       "Wht is the name for SQS service",
					placeholder:       "default-queue",
					validator:         regexp.MustCompile(`^[a-zA-Z0-9_-]{1,80}$`),
					validationMessage: "use alphanumeric characters, hyphens (-), and underscores ( _ )",
				}, stringValue{e.Sqs.Name}),
			},
		},
		sqs: e.Sqs,
	}

	m.viewport = viewport.New(0, 0)
	m.updateViewportContent()
	return m
}

func (m *sqsView) env(e Env) Env {
	s := Sqs{}
	s.Enabled = m.inputs[0].value().Bool()
	s.Name = m.inputs[1].value().String()
	e.Sqs = s
	return e
}
