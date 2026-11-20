package main

import (
	"strings"
	"testing"
)

// A scheduled task's dead-letter queue is `dlq: true`: meroku creates the
// queue and wires it. `dlq_arn` still works for a queue someone made by hand,
// with a warning that points at `dlq: true`; both at once is refused.

func scheduledTaskDLQEnv(extra map[string]interface{}) map[string]interface{} {
	return schedulingOverlay(extra)
}

func TestScheduledTaskDLQ_TrueRendersCreateDLQ(t *testing.T) {
	envMap := scheduledTaskDLQEnv(map[string]interface{}{"dlq": true})
	if err := preprocessEnvMap(envMap); err != nil {
		t.Fatalf("preprocessEnvMap: %v", err)
	}
	if warnings := generateWarnings(envMap); len(warnings) != 0 {
		t.Errorf("dlq: true should warn about nothing, got %v", warnings)
	}

	block := scheduleTaskBlock(t, renderMainHBS(t, scheduledTaskDLQEnv(map[string]interface{}{"dlq": true})), "cleanup")
	if !strings.Contains(block, "create_dlq = true") {
		t.Errorf("want create_dlq = true, got:\n%s", block)
	}
	if strings.Contains(block, "dlq_arn") {
		t.Errorf("dlq: true must not render dlq_arn, got:\n%s", block)
	}
}

func TestScheduledTaskDLQ_FalseRendersNothing(t *testing.T) {
	block := scheduleTaskBlock(t, renderMainHBS(t, scheduledTaskDLQEnv(map[string]interface{}{"dlq": false})), "cleanup")
	if strings.Contains(block, "create_dlq") || strings.Contains(block, "dlq_arn") {
		t.Errorf("dlq: false must render no DLQ argument, got:\n%s", block)
	}
}

func TestScheduledTaskDLQ_ArnStillWorksAndWarns(t *testing.T) {
	const arn = "arn:aws:sqs:us-east-1:000000000000:cleanup-dlq"
	envMap := scheduledTaskDLQEnv(map[string]interface{}{"dlq_arn": arn})
	if err := preprocessEnvMap(envMap); err != nil {
		t.Fatalf("dlq_arn alone must not stop generate: %v", err)
	}

	warnings := generateWarnings(envMap)
	if len(warnings) != 1 {
		t.Fatalf("want one warning, got %v", warnings)
	}
	w := warnings[0]
	if w.Code != warnCodeScheduledTaskDLQArn {
		t.Errorf("code = %q, want %q", w.Code, warnCodeScheduledTaskDLQArn)
	}
	if w.Item != (WarningItem{Kind: WarningKindScheduledTask, Name: "cleanup"}) {
		t.Errorf("item = %+v, want scheduled_task cleanup", w.Item)
	}
	if !strings.Contains(w.Title, "cleanup") || !strings.Contains(w.Detail, "dlq: true") {
		t.Errorf("the warning must name the task and point at dlq: true, got title %q, detail %q", w.Title, w.Detail)
	}

	block := scheduleTaskBlock(t, renderMainHBS(t, scheduledTaskDLQEnv(map[string]interface{}{"dlq_arn": arn})), "cleanup")
	if !strings.Contains(block, `dlq_arn = "`+arn+`"`) {
		t.Errorf("dlq_arn must still render, got:\n%s", block)
	}
}

func TestScheduledTaskDLQ_Refused(t *testing.T) {
	tests := []struct {
		name  string
		extra map[string]interface{}
		want  []string
	}{
		{
			name: "dlq and dlq_arn together",
			extra: map[string]interface{}{
				"dlq":     true,
				"dlq_arn": "arn:aws:sqs:us-east-1:000000000000:cleanup-dlq",
			},
			want: []string{"cleanup", "dlq", "dlq_arn"},
		},
		{
			// The template's {{#if dlq_arn}} renders any non-empty string, so
			// a blank one would reach Terraform as dlq_arn = " ".
			name:  "dlq_arn that is only spaces",
			extra: map[string]interface{}{"dlq_arn": "  "},
			want:  []string{"cleanup", "dlq_arn", "blank"},
		},
		{
			name:  "dlq that is not a boolean",
			extra: map[string]interface{}{"dlq": "yes"},
			want:  []string{"cleanup", "dlq", "true or false"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := preprocessEnvMap(scheduledTaskDLQEnv(tc.extra))
			if err == nil {
				t.Fatal("want an error, got none")
			}
			for _, want := range tc.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error should mention %q, got: %v", want, err)
				}
			}
		})
	}
}
