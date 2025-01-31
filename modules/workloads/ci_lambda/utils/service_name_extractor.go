package utils

import (
	"errors"
	"fmt"
	"regexp"
)

// GetServiceNameFromRepoName extracts service name from repository name
func GetServiceNameFromRepoName(str string, projectName string) (string, error) {
	// backend service
	if str == fmt.Sprintf("%s_backend", projectName) {
		return "", nil
	}
	re := regexp.MustCompile(`\w+_service_(?P<service>\w+(?:_\w+)*)`)

	match := re.FindStringSubmatch(str)
	if len(match) == 2 {
		return match[1], nil
	}
	return "", errors.New("Unable to extract service name")
}
