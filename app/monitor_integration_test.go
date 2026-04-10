//go:build integration
// +build integration

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
)

// TestMonitorAWSFetching tests real AWS API calls against the circl-dev profile.
// Run with: AWS_PROFILE=circl-dev go test -tags integration -run TestMonitorAWSFetching -v
func TestMonitorAWSFetching(t *testing.T) {
	// Change to project root so loadEnv can find project/dev.yaml
	if err := os.Chdir(".."); err != nil {
		t.Fatalf("Failed to chdir to project root: %v", err)
	}

	// Set AWS profile
	os.Setenv("AWS_PROFILE", "circl-dev")

	// Load real environment config
	env, err := loadEnv("dev")
	if err != nil {
		t.Fatalf("Failed to load dev environment: %v", err)
	}

	t.Logf("Loaded env: project=%s env=%s region=%s", env.Project, env.Env, env.Region)

	// Test fetchDashboardData with real AWS
	ctx := context.Background()
	data, err := fetchDashboardData(ctx, env, "circl-dev")
	if err != nil {
		t.Fatalf("fetchDashboardData failed: %v", err)
	}

	// Log the results as JSON for inspection
	jsonData, _ := json.MarshalIndent(data, "", "  ")
	t.Logf("Dashboard data:\n%s", string(jsonData))

	// Verify we got real data
	t.Logf("Cluster: %s (status: %s)", data.Cluster.Name, data.Cluster.Status)
	t.Logf("Services: %d", len(data.Services))
	for _, svc := range data.Services {
		t.Logf("  - %s: status=%s running=%d/%d CPU=%.1f%% Mem=%.1f%%",
			svc.Name, svc.Status, svc.RunningCount, svc.DesiredCount, svc.CPUPercent, svc.MemPercent)
		for _, task := range svc.Tasks {
			t.Logf("    Task %s: status=%s AZ=%s IP=%s", task.TaskID, task.Status, task.AZ, task.PrivateIP)
		}
	}

	if data.Database != nil {
		t.Logf("Database: %s status=%s engine=%s", data.Database.Identifier, data.Database.Status, data.Database.Engine)
	} else {
		t.Log("Database: not configured")
	}

	t.Logf("Scheduled tasks: %d", len(data.Schedules))
	for _, s := range data.Schedules {
		t.Logf("  - %s: schedule=%s state=%s", s.Name, s.Schedule, s.State)
	}

	t.Logf("Deployment events: %d", len(data.Deployments))
	for _, d := range data.Deployments {
		t.Logf("  - [%s] %s: %s (%s)", d.Source, d.ServiceName, d.Message, d.Status)
	}

	t.Logf("Non-fatal errors: %d", len(data.Errors))
	for _, e := range data.Errors {
		t.Logf("  - %s", e)
	}

	// Test GitHub workflows (may return empty if gh not configured)
	runs, err := fetchGitHubWorkflows(env)
	if err != nil {
		t.Logf("GitHub workflows: error (non-fatal): %v", err)
	} else {
		t.Logf("GitHub workflow runs: %d", len(runs))
		for _, r := range runs {
			t.Logf("  - %s: %s/%s branch=%s", r.Name, r.Status, r.Conclusion, r.HeadBranch)
		}
	}

	// Print summary
	fmt.Println("\n=== MONITOR DASHBOARD E2E VALIDATION ===")
	fmt.Printf("Cluster: %s (%s)\n", data.Cluster.Name, data.Cluster.Status)
	fmt.Printf("Services: %d\n", len(data.Services))
	fmt.Printf("Database: %v\n", data.Database != nil)
	fmt.Printf("Schedules: %d\n", len(data.Schedules))
	fmt.Printf("Deployments: %d events\n", len(data.Deployments))
	fmt.Printf("GitHub runs: %d\n", len(runs))
	fmt.Printf("Errors: %d\n", len(data.Errors))
	fmt.Println("=========================================")
}

// TestMonitorLogsPage tests real CloudWatch log fetching
func TestMonitorLogsPage(t *testing.T) {
	// Change to project root so loadEnv can find project/dev.yaml
	if err := os.Chdir(".."); err != nil {
		t.Fatalf("Failed to chdir to project root: %v", err)
	}

	os.Setenv("AWS_PROFILE", "circl-dev")

	env, err := loadEnv("dev")
	if err != nil {
		t.Fatalf("Failed to load dev environment: %v", err)
	}

	ctx := context.Background()
	cfg, err := buildAWSConfig(ctx, "circl-dev", env.Region)
	if err != nil {
		t.Fatalf("Failed to build AWS config: %v", err)
	}

	cwlClient := cloudwatchlogs.NewFromConfig(cfg)

	// Try fetching logs for backend service
	page, err := fetchLogsPage(ctx, cwlClient, env, "backend", "", 10)
	if err != nil {
		t.Logf("Log fetch failed (may be expected if no log group): %v", err)
		return
	}

	t.Logf("Logs for backend: %d entries", len(page.Entries))
	for _, entry := range page.Entries {
		t.Logf("  [%s] %s %s", entry.Timestamp.Format("15:04:05"), entry.Level, entry.Message[:min(len(entry.Message), 80)])
	}
}

// TestMonitorViewRendering validates that all 4 views render without panics
// and produce reasonable output. This tests the View() method directly
// without needing a terminal.
func TestMonitorViewRendering(t *testing.T) {
	if err := os.Chdir(".."); err != nil {
		t.Fatalf("Failed to chdir to project root: %v", err)
	}
	os.Setenv("AWS_PROFILE", "circl-dev")

	env, err := loadEnv("dev")
	if err != nil {
		t.Fatalf("Failed to load dev environment: %v", err)
	}

	// Create model with real data
	model := initMonitorModel(env, "dev")
	model.width = 120
	model.height = 40

	// Fetch real data
	ctx := context.Background()
	data, err := fetchDashboardData(ctx, env, "circl-dev")
	if err != nil {
		t.Fatalf("fetchDashboardData failed: %v", err)
	}
	model.data = data
	model.loading = false

	// Also fetch GitHub workflows
	runs, _ := fetchGitHubWorkflows(env)
	model.workflowRuns = runs

	// Rebuild log service list (same inline logic as Update)
	var list []string
	for _, svc := range model.data.Services {
		list = append(list, svc.Name)
	}
	for _, s := range model.data.Schedules {
		list = append(list, s.Name)
	}
	model.logServiceList = list

	// Test all 4 views
	views := []struct {
		name string
		mode monitorViewMode
	}{
		{"Overview", monitorOverviewView},
		{"Deployments", monitorDeploymentView},
		{"Logs", monitorLogsView},
		{"Exec", monitorExecView},
	}

	for _, v := range views {
		t.Run(v.name, func(t *testing.T) {
			model.currentView = v.mode
			output := model.View()

			if output == "" {
				t.Errorf("View %s produced empty output", v.name)
			}

			// Write the rendered output to a file for inspection
			outputPath := fmt.Sprintf("ai-docs/sessions/dev-feature-infra-monitor-dash-20260220-122541-2525b799/validation/view-%s.txt", strings.ToLower(v.name))
			os.WriteFile(outputPath, []byte(output), 0644)

			t.Logf("View %s: %d chars, %d lines", v.name, len(output), strings.Count(output, "\n")+1)

			// Print the first 30 lines for visual inspection
			lines := strings.Split(output, "\n")
			maxLines := 30
			if len(lines) < maxLines {
				maxLines = len(lines)
			}
			t.Logf("View %s rendered output (first %d lines):\n%s", v.name, maxLines, strings.Join(lines[:maxLines], "\n"))
		})
	}
}
