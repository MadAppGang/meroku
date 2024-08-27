package main

import (
	"regexp"

	"github.com/charmbracelet/bubbles/viewport"
)

type scheduledTaskView struct {
	detailViewModel

	t scheduledTask
}

func newScheduledTaskView(t scheduledTask) *scheduledTaskView {
	m := &scheduledTaskView{
		detailViewModel: detailViewModel{
			title:       "Scheduled ECS task",
			description: "ECR Repository will be crated for the service",
			inputs: []inputModel{
				newTextFieldModel(baseInputModel{
					title:             "Scheduled task name",
					description:       "The ECS task which will be run on schedule.",
					placeholder:       "send_notifications",
					validator:         regexp.MustCompile(`^($|[a-zA-Z][\w-]{3,254})$`),
					validationMessage: "Valid ECS service name, letter, numbers and dash only, min 3 and max 255 characters",
				}, stringValue{t.name}),
				newTextFieldModel(baseInputModel{
					title:             "Scheduled task name",
					description:       "cron(Minutes Hours Day-of-month Month Day-of-week Year)",
					placeholder:       "cron(0 6 * * ? *)",
					validator:         regexp.MustCompile(`^cron\(([\d\*\-\,\/]+)\s+([\d\*\-\,\/]+)\s+([\d\*\-\,\/\?LW]+)\s+([\d\*\-\,\/]+|JAN|FEB|MAR|APR|MAY|JUN|JUL|AUG|SEP|OCT|NOV|DEC)\s+([\d\*\-\,\/L\?]+|SUN|MON|TUE|WED|THU|FRI|SAT)\s+([\d\*\-\,\/]+|\*)\)$`),
					validationMessage: "Valid cron expression",
				}, stringValue{t.schedule}),
			},
		},
		t: t,
	}

	m.viewport = viewport.New(0, 0)
	m.updateViewportContent()
	return m
}
