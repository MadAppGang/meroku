package main

import (
	"strings"
	"testing"
)

// The YAML -> HCL link for github_oidc_create_provider.
//
// This flag is the whole fix for a second project in one AWS account, and it is
// worth nothing if the value never reaches the module call. The failure would
// also be quiet: a missing line in main.hbs renders a valid module block that
// simply falls back to the variable default, so terraform stays happy and the
// apply fails on EntityAlreadyExists exactly as it did before.

// githubOIDCLine returns the github_oidc_create_provider assignment from the
// rendered workloads module block.
func githubOIDCLine(t *testing.T, rendered string) string {
	t.Helper()

	for _, line := range strings.Split(workloadsModuleBlock(t, rendered), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "github_oidc_create_provider") {
			return trimmed
		}
	}
	t.Fatal("github_oidc_create_provider is not in the workloads module block")
	return ""
}

func TestTemplate_GithubOIDCCreateProvider_AbsentRendersTrue(t *testing.T) {
	// The key is absent from every config until v28 writes it, and v28 only
	// writes it where OIDC is enabled. Absent has to render as true, or the
	// migration would silently stop existing projects creating their provider.
	rendered := renderMainHBS(t, workloadOverlay(t, map[string]interface{}{
		"enable_github_oidc": true,
	}))

	if got := githubOIDCLine(t, rendered); got != "github_oidc_create_provider = true" {
		t.Errorf("rendered %q, want %q — an absent key must keep today's behaviour",
			got, "github_oidc_create_provider = true")
	}
}

func TestTemplate_GithubOIDCCreateProvider_FalseSurvivesRendering(t *testing.T) {
	// The value meroku writes when another project owns the provider. If the
	// default helper swallowed false, the resolution would be discarded on every
	// generate and the deploy would fail again.
	rendered := renderMainHBS(t, workloadOverlay(t, map[string]interface{}{
		"enable_github_oidc":          true,
		"github_oidc_create_provider": false,
	}))

	if got := githubOIDCLine(t, rendered); got != "github_oidc_create_provider = false" {
		t.Errorf("rendered %q, want %q — an explicit false must not be treated as missing",
			got, "github_oidc_create_provider = false")
	}
}

func TestTemplate_GithubOIDCCreateProvider_ExplicitTrueRendersTrue(t *testing.T) {
	rendered := renderMainHBS(t, workloadOverlay(t, map[string]interface{}{
		"enable_github_oidc":          true,
		"github_oidc_create_provider": true,
	}))

	if got := githubOIDCLine(t, rendered); got != "github_oidc_create_provider = true" {
		t.Errorf("rendered %q, want %q", got, "github_oidc_create_provider = true")
	}
}

// The YAML -> HCL link for github_subjects, which is what makes removing the
// module's default safe.
//
// modules/workloads/variables.tf deliberately declares github_subjects with NO
// default, so a direct module consumer who omits it gets Terraform's "No value
// for required variable" rather than a silent org-wide grant. That is only safe
// because generated configurations always pass the key. If main.hbs ever emitted
// it conditionally, every generated env would stop planning — loudly, but for a
// reason nobody would connect back to this change.
//
// So the claim is asserted rather than assumed, including the two cases that
// would tempt somebody to add a conditional: the feature switched off, and no
// subjects at all.
func TestTemplate_GithubSubjects_AlwaysEmitted(t *testing.T) {
	tests := []struct {
		name     string
		oidc     bool
		subjects interface{} // nil means "delete the key entirely"
		want     string
	}{
		{"absent key, oidc on", true, nil, "github_subjects = []"},
		{"absent key, oidc off", false, nil, "github_subjects = []"},
		{"empty list", true, []interface{}{}, "github_subjects = []"},
		{
			"populated", true,
			[]interface{}{"repo:acme/api:ref:refs/heads/main"},
			`github_subjects = ["repo:acme/api:ref:refs/heads/main"]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			overlay := workloadOverlay(t, map[string]interface{}{
				"enable_github_oidc": tt.oidc,
			})
			workload, ok := overlay["workload"].(map[string]interface{})
			if !ok {
				t.Fatal("workloadOverlay did not return a workload mapping")
			}
			if tt.subjects == nil {
				delete(workload, "github_oidc_subjects")
			} else {
				workload["github_oidc_subjects"] = tt.subjects
			}

			block := workloadsModuleBlock(t, renderMainHBS(t, overlay))

			var got string
			for _, line := range strings.Split(block, "\n") {
				trimmed := strings.TrimSpace(line)
				if strings.HasPrefix(trimmed, "github_subjects") {
					got = trimmed
					break
				}
			}
			if got == "" {
				t.Fatalf("github_subjects is not in the workloads module block; the module "+
					"variable has no default, so this would render an unplannable config\n%s", block)
			}
			if got != tt.want {
				t.Errorf("rendered %q, want %q", got, tt.want)
			}
		})
	}
}
