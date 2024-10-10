package main

import (
	"regexp"

	"github.com/charmbracelet/bubbles/viewport"
)

const ecsSchedule = `^(cron\(` +
	// Minutes: 0-59 or *
	`([0-5]?\d|\*)` +
	// Hours: 0-23 or *
	`\s([01]?\d|2[0-3]|\*)` +
	// Day-of-month: 1-31 or ? or *
	`\s(([1-9]|[12]\d|3[01])|\?|\*)` +
	// Month: 1-12 or JAN-DEC or *
	`\s((1[0-2]|[1-9])|(JAN|FEB|MAR|APR|MAY|JUN|JUL|AUG|SEP|OCT|NOV|DEC)|\*)` +
	// Day-of-week: 1-7 or SUN-SAT or ? or *
	`\s([1-7]|(MON|TUE|WED|THU|FRI|SAT|SUN)|\?|\*)` +
	// Year: 1970-2199 or *
	`\s((19[7-9]\d|2[01]\d{2}|2199)|\*)` +
	`\)` +
	`|rate\((\d+)\s(minute|minutes|hour|hours|day|days)\))$`

type scheduledTaskView struct {
	detailViewModel

	t ScheduledTask
}

func newScheduledTaskView(t ScheduledTask) *scheduledTaskView {
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
				}, stringValue{t.Name}),
				newTextFieldModel(baseInputModel{
					title:             "Scheduled task name",
					description:       "cron(Minutes Hours Day-of-month Month Day-of-week Year) or rate: rate(1 minute), rate(3 hours)",
					placeholder:       "cron(0 6 * * ? *)",
					validator:         regexp.MustCompile(ecsSchedule),
					validationMessage: "Valid cron or rate expression",
				}, stringValue{t.Schedule}),
				newTextFieldModel(baseInputModel{
					title:       "External Docker image",
					placeholder: "madappgang/aooth",
					description: "Optional Docker hub image name, by default we use private ECR registry for task",
				}, stringValue{t.ExternalDockerImage}),
				newTextFieldModel(baseInputModel{
					title:             "custom docker container command",
					placeholder:       "[\"aooth\", \"--flag\"]",
					description:       "Optional provide container command",
					validator:         regexp.MustCompile(`(^\s*$|\[(\s*"[^"]*"\s*,?\s*)*\])`),
					validationMessage: "Container command is JSON  array of strings format",
				}, stringValue{t.ContainerCommand}),
				newBoolFieldModel(baseInputModel{
					title:       "Allow public access",
					description: "Allow your task to access public internet, it you are using docker image from dockerhub, this should be true.",
				}, boolValue{t.AllowPublicAccess}),
			},
		},
		t: t,
	}

	m.viewport = viewport.New(0, 0)
	m.updateViewportContent()
	return m
}

func (m *scheduledTaskView) env(e Env) Env {
	t := ScheduledTask{}
	t.Name = m.inputs[0].value().String()
	t.Schedule = m.inputs[1].value().String()
	t.ExternalDockerImage = m.inputs[2].value().String()
	t.ContainerCommand = m.inputs[3].value().String()
	t.AllowPublicAccess = m.inputs[4].value().Bool()
	e.ScheduledTasks = append(e.ScheduledTasks, t)
	return e
}
