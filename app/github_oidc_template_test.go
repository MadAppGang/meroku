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
