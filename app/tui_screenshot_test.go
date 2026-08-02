package main

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// Visual QA harness.
//
// The apply view can only be reached by running a real terraform apply that
// fails, which is not something a test can arrange. This builds the same model
// state directly and writes the rendered frames — with ANSI colour intact — so
// they can be converted to PNG and looked at.
//
// Not a Test* function: it is driven by TestRenderTUIScreens below, which is
// gated on MEROKU_TUI_SHOTS=1 so ordinary `go test` stays quiet.

// buildFailedApplyModel reproduces the state from the reported failure: a
// coretechx dev apply into an account that already hosts circl, stalled on ACM
// validation with IAM collisions.
func buildFailedApplyModel(width, height int) *modernPlanModel {
	m := &modernPlanModel{
		width:  width,
		height: height,
		keys:   modernKeys,
		stats:  changeStats{totalChanges: 86},
		// Same as the real constructor, or the progress bar renders empty.
		progress: progress.New(progress.WithDefaultGradient()),
	}
	m.currentView = applyView
	m.initApplyState()

	st := m.applyState
	st.startTime = time.Now().Add(-32 * time.Minute)
	st.isApplying = true
	st.totalResources = 86

	// 76 done, matching the 88% in the screenshot.
	for i := 0; i < 70; i++ {
		st.completed = append(st.completed, completedResource{
			Address:   fmt.Sprintf("module.workloads.aws_iam_role_policy_attachment.svc_%02d", i),
			Action:    "create",
			Success:   true,
			Duration:  time.Duration(400+i*7) * time.Millisecond,
			Timestamp: time.Now(),
		})
	}
	for _, addr := range []string{
		"module.workloads.aws_apigatewayv2_vpc_link.services[\"magento-bridge\"]",
		"module.workloads.aws_apigatewayv2_integration.services[\"magento-bridge\"]",
		"module.workloads.aws_apigatewayv2_route.services[\"magento-bridge\"]",
		"module.workloads.aws_ecs_task_definition.backend",
	} {
		st.completed = append(st.completed, completedResource{
			Address: addr, Action: "create", Success: true,
			Duration: 1200 * time.Millisecond, Timestamp: time.Now(),
		})
	}

	// The two collisions the operator actually saw, with the real AWS text that
	// used to be thrown away.
	failures := []struct{ addr, summary, detail string }{
		{
			"module.workloads.aws_iam_role.lambda_deploy_iam",
			"creating IAM Role (lambda_deploy_iam_dev): EntityAlreadyExists",
			"operation error IAM: CreateRole, https response error StatusCode: 409, " +
				"api error EntityAlreadyExists: Role with name lambda_deploy_iam_dev already exists.",
		},
		{
			"module.workloads.aws_iam_policy.lambda_kms",
			"creating IAM Policy (LambdaKMSPolicy_dev): EntityAlreadyExists",
			"operation error IAM: CreatePolicy, https response error StatusCode: 409, " +
				"api error EntityAlreadyExists: A policy called LambdaKMSPolicy_dev already exists.",
		},
	}
	for _, f := range failures {
		st.completed = append(st.completed, completedResource{
			Address:      f.addr,
			Action:       "create",
			Success:      false,
			Error:        f.addr + ": Creation errored after 1s",
			ErrorSummary: f.summary,
			ErrorDetail:  f.detail,
			Duration:     time.Second,
			Timestamp:    time.Now(),
		})
		st.diagnostics[f.addr] = &DiagnosticInfo{
			Severity: "error", Summary: f.summary, Detail: f.detail, Address: f.addr,
		}
	}
	st.errorCount = len(failures)
	st.hasErrors = true

	// Long-running ACM validations — the resources that stall the apply.
	st.currentOps["module.domain.aws_acm_certificate_validation.api_domain"] = &currentOperation{
		Address: "module.domain.aws_acm_certificate_validation.api_domain",
		Action:  "create", StartTime: time.Now().Add(-1921 * time.Second),
	}
	st.currentOps["module.domain.aws_acm_certificate_validation.subdomains"] = &currentOperation{
		Address: "module.domain.aws_acm_certificate_validation.subdomains",
		Action:  "create", StartTime: time.Now().Add(-1919 * time.Second),
	}

	st.pending = []pendingResource{
		{Address: "module.workloads.aws_apigatewayv2_api_mapping.backend[0]", Action: "create"},
		{Address: "module.workloads.aws_apigatewayv2_domain_name.backend[0]", Action: "create"},
		{Address: "module.workloads.aws_lambda_function.lambda_deploy", Action: "create"},
		{Address: "module.workloads.aws_cloudwatch_event_rule.ecr_event", Action: "create"},
	}

	for i, msg := range []string{
		"Creating module.workloads.aws_iam_role.lambda_deploy_iam",
		"Failed create on module.workloads.aws_iam_role.lambda_deploy_iam (1s)",
		"Creating module.domain.aws_acm_certificate_validation.subdomains",
		"Still creating... [31m40s elapsed]",
	} {
		level := "info"
		if i == 1 {
			level = "error"
		}
		st.logs = append(st.logs, logEntry{
			Timestamp: time.Now(), Level: level, Message: msg,
		})
	}

	m.progress.Width = width - 30
	m.applyProgress = 76.0 / 86.0
	m.calculateApplyLayout(height)
	m.logViewport = viewport.New(width-4, 6)
	m.updateApplyLogViewport()
	return m
}

// TestRenderTUIScreens writes colour ANSI captures of the apply screens.
//
//	MEROKU_TUI_SHOTS=1 go test -run TestRenderTUIScreens ./app
func TestRenderTUIScreens(t *testing.T) {
	if os.Getenv("MEROKU_TUI_SHOTS") != "1" {
		t.Skip("set MEROKU_TUI_SHOTS=1 to render TUI screenshots")
	}

	// Lip Gloss strips colour when it thinks stdout is not a TTY, which is
	// exactly the case under `go test` — force full colour or the capture is
	// monochrome and the whole exercise is pointless.
	lipglossForceTrueColor()

	shots := []struct {
		name          string
		w, h          int
		errorDetails  bool
		applyComplete bool
	}{
		{"apply-wide", 160, 50, false, false},
		{"apply-narrow", 100, 34, false, false},
		{"error-details", 160, 50, true, true},
	}

	for _, s := range shots {
		m := buildFailedApplyModel(s.w, s.h)
		m.applyState.showErrorDetails = s.errorDetails
		if s.applyComplete {
			m.applyState.applyComplete = true
			m.applyState.isApplying = false
		}

		out := m.View()
		path := "/tmp/meroku-tui-" + s.name + ".ansi"
		if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
		t.Logf("wrote %s (%d bytes)", path, len(out))
	}
}

func lipglossForceTrueColor() {
	r := lipgloss.DefaultRenderer()
	r.SetColorProfile(termenv.TrueColor)
	r.SetHasDarkBackground(true)
}
