package main

import (
	"strings"
	"testing"
)

// scheduleTaskBlock returns the body of `module "schedule_task_<name>" { ... }`.
func scheduleTaskBlock(t *testing.T, rendered, name string) string {
	t.Helper()

	header := `module "schedule_task_` + name + `" {`
	start := strings.Index(rendered, header)
	if start < 0 {
		t.Fatalf("%s not found in the rendered template", header)
	}
	rest := rendered[start:]
	end := strings.Index(rest, "\n}\n")
	if end < 0 {
		t.Fatalf("%s block is unterminated", header)
	}
	return rest[:end]
}

func schedulingOverlay(extra map[string]interface{}) map[string]interface{} {
	task := map[string]interface{}{
		"name":     "cleanup",
		"schedule": "cron(0 9 * * ? *)",
	}
	for k, v := range extra {
		task[k] = v
	}
	return map[string]interface{}{
		"scheduled_tasks": []interface{}{task},
	}
}

// max_retry_attempts is three-state and the middle state is the dangerous one.
//
// AWS's own default is 185. The module omits the retry_policy block entirely
// when the variable is null, so an unset value leaves that default alone. If an
// absent key rendered as a number instead, every existing scheduled task would
// have its retry budget cut on the next apply — silently, with nobody having
// asked for it. And 0 has to survive as 0, because "never retry" is a real
// choice that a truthiness check would swallow.
func TestScheduledTaskRetryAttemptsThreeStates(t *testing.T) {
	tests := []struct {
		name    string
		overlay map[string]interface{}
		want    string // "" means the argument must be absent
	}{
		{
			name:    "absent omits the argument so AWS keeps 185",
			overlay: schedulingOverlay(nil),
			want:    "",
		},
		{
			name:    "zero renders as zero, meaning never retry",
			overlay: schedulingOverlay(map[string]interface{}{"max_retry_attempts": 0}),
			want:    "max_retry_attempts = 0",
		},
		{
			name:    "a real value renders",
			overlay: schedulingOverlay(map[string]interface{}{"max_retry_attempts": 3}),
			want:    "max_retry_attempts = 3",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			block := scheduleTaskBlock(t, renderMainHBS(t, tc.overlay), "cleanup")

			if tc.want == "" {
				if strings.Contains(block, "max_retry_attempts") {
					t.Errorf("max_retry_attempts should be absent when unset, got:\n%s", block)
				}
				return
			}
			if !strings.Contains(block, tc.want) {
				t.Errorf("want %q, got:\n%s", tc.want, block)
			}
		})
	}
}

func TestScheduledTaskTimezoneAndDLQ(t *testing.T) {
	t.Run("both render when set", func(t *testing.T) {
		block := scheduleTaskBlock(t, renderMainHBS(t, schedulingOverlay(map[string]interface{}{
			"timezone": "Australia/Sydney",
			"dlq_arn":  "arn:aws:sqs:us-east-1:000000000000:cleanup-dlq",
		})), "cleanup")

		for _, want := range []string{
			`schedule_expression_timezone = "Australia/Sydney"`,
			`dlq_arn = "arn:aws:sqs:us-east-1:000000000000:cleanup-dlq"`,
		} {
			if !strings.Contains(block, want) {
				t.Errorf("want %q, got:\n%s", want, block)
			}
		}
	})

	// Both are plain optional strings, so absent must render nothing and let
	// the module defaults (UTC, and no DLQ or IAM grant) apply.
	t.Run("both absent when unset", func(t *testing.T) {
		block := scheduleTaskBlock(t, renderMainHBS(t, schedulingOverlay(nil)), "cleanup")

		for _, unwanted := range []string{"schedule_expression_timezone", "dlq_arn"} {
			if strings.Contains(block, unwanted) {
				t.Errorf("%s should be absent when unset, got:\n%s", unwanted, block)
			}
		}
	})
}

// The list conversion from v25, checked at the render boundary rather than only
// at the migration.
func TestScheduledTaskContainerCommandRendersAsHCLList(t *testing.T) {
	block := scheduleTaskBlock(t, renderMainHBS(t, schedulingOverlay(map[string]interface{}{
		"container_command": []interface{}{"npm", "run", "cron"},
	})), "cleanup")

	const want = `container_command = ["npm","run","cron"]`
	if !strings.Contains(block, want) {
		t.Errorf("want %s, got:\n%s", want, block)
	}
	if strings.Contains(block, "npmruncron") {
		t.Errorf("container_command rendered as a concatenated bare token:\n%s", block)
	}
}
