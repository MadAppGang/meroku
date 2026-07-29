package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// AWS names that are unique across an entire account (IAM roles and policies) or an entire
// region (EventBridge rules, Lambda functions) must carry both ${var.project} and ${var.env},
// per the naming convention in CLAUDE.md. Names scoped to a parent resource (an ECS service
// inside a project's cluster, a target group, a container) are exempt and not checked here.
//
// This is not cosmetic. The ECR CI/CD rule used to be the bare literal "ecr_events_cicd",
// shared by every environment in an AWS account. PutRule is an upsert, so Terraform adopted
// another environment's rule rather than failing: every env's CI/CD lambda ended up a target
// of the same rule (each lambda receiving every other env's ECR/ECS events), and destroying
// any one env deleted the shared rule out from under the others.
func TestGlobalAwsNamesAreNamespaced(t *testing.T) {
	globalTypes := map[string]bool{
		"aws_iam_role":              true,
		"aws_iam_policy":            true,
		"aws_cloudwatch_event_rule": true,
		"aws_lambda_function":       true,
	}

	resourceHeader := regexp.MustCompile(`^resource\s+"([^"]+)"\s+"([^"]+)"\s*{`)
	// Only TOP-LEVEL attributes (exactly two spaces of indent). Nested blocks — container
	// definitions, env vars, health checks — have deeper indentation and are not AWS names.
	topLevelName := regexp.MustCompile(`^  (?:name|function_name)\s+=\s+"([^"]+)"`)

	files, err := filepath.Glob(filepath.Join("..", "modules", "workloads", "*.tf"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no workloads terraform files found: %v", err)
	}

	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}

		currentType := ""
		for _, line := range strings.Split(string(content), "\n") {
			if m := resourceHeader.FindStringSubmatch(line); m != nil {
				currentType = m[1]
				continue
			}
			if line == "}" {
				currentType = ""
				continue
			}
			if !globalTypes[currentType] {
				continue
			}
			m := topLevelName.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			value := m[1]
			if !strings.Contains(value, "${var.project}") || !strings.Contains(value, "${var.env}") {
				t.Errorf("%s: %s name %q lacks ${var.project} and/or ${var.env} — this name is "+
					"global to the AWS account/region and will collide with another meroku "+
					"deployment", filepath.Base(file), currentType, value)
			}
		}
	}
}
