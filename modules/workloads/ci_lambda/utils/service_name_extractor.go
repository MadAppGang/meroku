package utils

import (
	"errors"
	"fmt"
	"regexp"
)

// GetServiceNameFromRepoName extracts service name from repository name.
//
// Returns:
//   - "" (empty string) for the backend service repo: {project}_backend
//   - "{name}" for named service repos: {project}_service_{name}
//   - "task:{name}" for scheduled task repos: {project}_task_{name}
//   - error if the repo name does not match any known pattern
func GetServiceNameFromRepoName(str string, projectName string) (string, error) {
	// backend service: {project}_backend → ""
	if str == fmt.Sprintf("%s_backend", projectName) {
		return "", nil
	}

	// named ECS service: {project}_service_{name} → "{name}"
	serviceRe := regexp.MustCompile(`\w+_service_(?P<service>\w+(?:_\w+)*)`)
	if match := serviceRe.FindStringSubmatch(str); len(match) == 2 {
		return match[1], nil
	}

	// scheduled task: {project}_task_{name} → "task:{name}"
	taskRe := regexp.MustCompile(`\w+_task_(?P<task>\w+(?:_\w+)*)`)
	if match := taskRe.FindStringSubmatch(str); len(match) == 2 {
		return "task:" + match[1], nil
	}

	return "", errors.New("unable to extract service name")
}
