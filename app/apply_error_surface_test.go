package main

import (
	"strings"
	"testing"
)

// newErrorTestModel builds the minimal model needed to exercise the error
// surfacing helpers.
func newErrorTestModel() *modernPlanModel {
	return &modernPlanModel{
		applyState: &applyState{
			diagnostics: map[string]*DiagnosticInfo{},
		},
	}
}

// The regression this guards: handleApplyError copies the diagnostic into the
// completedResource, but it can run before terraform emits the matching
// "diagnostic" message. In that ordering ErrorSummary/ErrorDetail are empty and
// handleDiagnostic's back-fill can also miss, because the resource is still in
// flight as a resourceCompleteMsg and is not yet in applyState.completed.
//
// The operator then saw only "Creation errored after 1s" and had no way to learn
// the actual AWS error. The renderer must fall back to the diagnostics map.
func TestErrorSummaryFor_FallsBackToDiagnosticsMap(t *testing.T) {
	m := newErrorTestModel()

	const addr = "module.workloads.aws_iam_role.lambda_deploy_iam"
	m.applyState.diagnostics[addr] = &DiagnosticInfo{
		Severity: "error",
		Summary:  "creating IAM Role (lambda_deploy_iam_dev): EntityAlreadyExists",
		Detail:   "IAM Role lambda_deploy_iam_dev already exists.",
		Address:  addr,
	}

	// ErrorSummary/ErrorDetail empty: exactly the lost-back-fill case.
	res := completedResource{
		Address: addr,
		Success: false,
		Error:   addr + ": Creation errored after 1s",
	}

	if got := m.errorSummaryFor(res); !strings.Contains(got, "EntityAlreadyExists") {
		t.Errorf("expected summary recovered from diagnostics map, got %q", got)
	}
	if got := m.errorDetailFor(res); !strings.Contains(got, "already exists") {
		t.Errorf("expected detail recovered from diagnostics map, got %q", got)
	}
}

// When the resource already carries the diagnostic, that copy wins and the map
// is not consulted.
func TestErrorSummaryFor_PrefersResourceCopy(t *testing.T) {
	m := newErrorTestModel()

	const addr = "module.workloads.aws_iam_policy.lambda_kms"
	m.applyState.diagnostics[addr] = &DiagnosticInfo{Summary: "from map", Detail: "from map"}

	res := completedResource{
		Address:      addr,
		Success:      false,
		ErrorSummary: "from resource",
		ErrorDetail:  "from resource",
	}

	if got := m.errorSummaryFor(res); got != "from resource" {
		t.Errorf("expected resource copy to win, got %q", got)
	}
	if got := m.errorDetailFor(res); got != "from resource" {
		t.Errorf("expected resource copy to win, got %q", got)
	}
}

// No diagnostic anywhere: return empty so the caller falls back to res.Error.
func TestErrorSummaryFor_EmptyWhenNoDiagnostic(t *testing.T) {
	m := newErrorTestModel()

	res := completedResource{Address: "aws_s3_bucket.x", Success: false, Error: "boom"}

	if got := m.errorSummaryFor(res); got != "" {
		t.Errorf("expected empty summary, got %q", got)
	}
	if got := m.errorDetailFor(res); got != "" {
		t.Errorf("expected empty detail, got %q", got)
	}
}

// The six colliding resources in modules/workloads/lambda.tf surface as these
// AWS error codes. Each must produce the "another project owns this name" hint.
func TestCollisionHint_RecognisesNameClashes(t *testing.T) {
	clashes := []string{
		"creating IAM Role (lambda_deploy_iam_dev): EntityAlreadyExists: Role with name lambda_deploy_iam_dev already exists.",
		"creating Lambda Function (ci_lambda_dev): ResourceConflictException: Function already exist",
		"creating CloudWatch Log Group: ResourceAlreadyExistsException: The specified log group already exists",
		"AlreadyExistsException: resource exists",
	}

	for _, text := range clashes {
		if collisionHint(text) == "" {
			t.Errorf("expected a collision hint for %q", text)
		}
	}
}

// Unrelated failures must not be mislabelled as name collisions.
func TestCollisionHint_IgnoresUnrelatedErrors(t *testing.T) {
	unrelated := []string{
		"AccessDenied: User is not authorized to perform iam:CreateRole",
		"waiting for ACM Certificate validation: timeout while waiting for state to become 'ISSUED'",
		"InvalidParameterValue: The subnet ID is malformed",
		"",
	}

	for _, text := range unrelated {
		if hint := collisionHint(text); hint != "" {
			t.Errorf("expected no collision hint for %q, got %q", text, hint)
		}
	}
}
