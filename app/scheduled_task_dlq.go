package main

import (
	"fmt"
	"strings"
)

// validateScheduledTaskDLQConfigMap checks each scheduled task's dead-letter
// queue settings.
//
// `dlq: true` is the way to ask for one: modules/ecs_task creates the queue,
// makes it the schedule's dead-letter target and lets the scheduler role send
// to it. `dlq_arn` names an existing queue by ARN. It still renders, so a task
// deployed with one keeps its queue, but it is a warning: the YAML says what a
// workload needs, and meroku builds the resources and their ARNs. A schedule
// has one dead-letter target, so both at once is an error.
func validateScheduledTaskDLQConfigMap(envMap map[string]interface{}) error {
	tasks, _ := envMap["scheduled_tasks"].([]interface{})

	var problems []string
	for i, raw := range tasks {
		task, ok := computeNodeMap(raw)
		if !ok {
			continue
		}
		name := computeStringField(task, "name")
		if name == "" {
			name = fmt.Sprintf("#%d", i+1)
		}

		dlq := false
		if value, present := task["dlq"]; present && value != nil {
			b, isBool := value.(bool)
			if !isBool {
				problems = append(problems, fmt.Sprintf(
					"scheduled task '%s': dlq is %v; it must be true or false", name, value))
				continue
			}
			dlq = b
		}
		arn := strings.TrimSpace(computeStringField(task, "dlq_arn"))

		switch {
		case dlq && arn != "":
			problems = append(problems, fmt.Sprintf(
				"scheduled task '%s': set dlq or dlq_arn, not both. A schedule has one dead-letter target; remove dlq_arn to use the queue meroku creates", name))
		case arn != "":
			addGenerateWarning(envMap, scheduledTaskDLQArnWarning(name))
		}
	}

	if len(problems) > 0 {
		return fmt.Errorf("invalid scheduled task dead-letter settings:\n  %s", strings.Join(problems, "\n  "))
	}
	return nil
}

// scheduledTaskDLQArnWarning is the warning for a scheduled task that names
// its dead-letter queue by ARN.
func scheduledTaskDLQArnWarning(name string) ConfigWarning {
	return ConfigWarning{
		Code:  warnCodeScheduledTaskDLQArn,
		Item:  workloadItem(WarningKindScheduledTask, name),
		Title: fmt.Sprintf("scheduled task '%s' names its dead-letter queue by ARN (dlq_arn)", name),
		Detail: strings.Join([]string{
			"It still works: the schedule keeps sending failed launches to that queue.",
			"Replace dlq_arn with dlq: true to have meroku create the queue and wire it",
			"for you. That is a new, empty queue; read or move what is left in the old",
			"one before you delete it.",
		}, "\n"),
		Docs: docsLink("ai_docs/YAML_SPECIFICATION.md#scheduled-task-dead-letter-queue"),
	}
}
