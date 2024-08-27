package main

import (
	"regexp"

	"github.com/charmbracelet/bubbles/viewport"
)

type eventProcessorTaskView struct {
	detailViewModel

	t eventProcessorTask
}

func NewEventProcessorTaskView(t eventProcessorTask) *eventProcessorTaskView {
	m := &eventProcessorTaskView{
		detailViewModel: detailViewModel{
			title:       "Scheduled ECS task",
			description: "ECR Repository will be crated for the service",
			inputs: []inputModel{
				newTextFieldModel(baseInputModel{
					title:             "Event processor task name",
					description:       "The ECS task which will be run on specific event in event bus.",
					placeholder:       "on_new_order",
					validator:         regexp.MustCompile(`^($|[a-zA-Z][\w-]{3,254})$`),
					validationMessage: "Valid ECS service name, letter, numbers and dash only, min 3 and max 255 characters",
				}, stringValue{t.name}),
				newTextFieldModel(baseInputModel{
					title:             "Rule name",
					description:       "The Cloudwatch event rule name which will be triggered by the event.",
					placeholder:       "new_order_rule",
					validator:         regexp.MustCompile(`^$|^[a-zA-Z0-9][-_.a-zA-Z0-9]{3,63}$`),
					validationMessage: "Valid Cloudwatch event rule name, letter, numbers and dash only, min 3 and max 63 characters",
				}, stringValue{t.ruleName}),
				newTextFieldModel(baseInputModel{
					title:       "Detail types",
					description: "Optional filter by details types.",
				}, sliceValue{t.detailTypes}),
				newTextFieldModel(baseInputModel{
					title:       "Sources to catch",
					description: "Optional filter by sources of messages.",
				}, sliceValue{t.sources}),
			},
		},
		t: t,
	}

	m.viewport = viewport.New(0, 0)
	m.updateViewportContent()
	return m
}
