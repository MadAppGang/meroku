package main

import (
	"bytes"
	"strings"
	"testing"
)

// REQ-FR-1, REQ-FR-2, AC-1: only inaccessible services without a health check warn.
func TestConfigWarningsServiceHealthCheckVariants(t *testing.T) {
	tests := []struct {
		name          string
		network       string
		omitNetwork   bool
		healthCommand interface{}
		wantWarning   bool
	}{
		{name: "internal without command warns", network: "internal", wantWarning: true},
		{name: "none without command warns", network: "none", wantWarning: true},
		{name: "public without command is clean", network: "public"},
		{name: "absent network access is clean", omitNetwork: true},
		{name: "internal with command is clean", network: "internal", healthCommand: []interface{}{"CMD", "true"}},
		{name: "none with command is clean", network: "none", healthCommand: []interface{}{"CMD-SHELL", "true"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := map[string]interface{}{"name": "magento-bridge"}
			if !tt.omitNetwork {
				service["network_access"] = tt.network
			}
			if tt.healthCommand != nil {
				service["health_check_command"] = tt.healthCommand
			}
			envMap := map[string]interface{}{"services": []interface{}{service}}

			if err := preprocessEnvMap(envMap); err != nil {
				t.Fatalf("preprocessEnvMap() error = %v", err)
			}
			got := configWarnings(envMap, nil)
			var found []ConfigWarning
			for _, warning := range got {
				if warning.Code == warnCodeServiceNoHealthCheck {
					found = append(found, warning)
				}
			}
			if !tt.wantWarning {
				if len(found) != 0 {
					t.Fatalf("unexpected health-check warnings: %#v", found)
				}
				return
			}
			if len(found) != 1 {
				t.Fatalf("got %d health-check warnings, want 1: %#v", len(found), got)
			}
			warning := found[0]
			if warning.Item != (WarningItem{Kind: WarningKindService, Name: "magento-bridge"}) {
				t.Errorf("item = %#v, want the named service", warning.Item)
			}
			if !strings.Contains(warning.Title, tt.network) || strings.Contains(warning.Title, "\n") {
				t.Errorf("title must be one line and name network access %q: %q", tt.network, warning.Title)
			}
			if !strings.Contains(warning.Detail, "health_check_command") {
				t.Errorf("detail does not explain the fix: %q", warning.Detail)
			}
			if !strings.HasPrefix(warning.Docs, "https://") {
				t.Errorf("docs = %q, want an absolute HTTPS URL", warning.Docs)
			}
		})
	}
}

// REQ-FR-1; contract 8.1: an unnamed YAML entry still has a stable reference.
func TestConfigWarningsUnnamedServiceUsesOneBasedIndex(t *testing.T) {
	envMap := map[string]interface{}{"services": []interface{}{
		map[string]interface{}{"network_access": "internal"},
	}}
	if err := preprocessEnvMap(envMap); err != nil {
		t.Fatalf("preprocessEnvMap() error = %v", err)
	}
	got := configWarnings(envMap, nil)
	if len(got) != 1 {
		t.Fatalf("got %d warnings, want 1: %#v", len(got), got)
	}
	if got[0].Item.Name != "#1" {
		t.Fatalf("unnamed service item = %q, want #1", got[0].Item.Name)
	}
}

// Contract 8.1: ordering and de-duplication are report invariants.
func TestNormalizeConfigWarningsSortsAndDeduplicates(t *testing.T) {
	duplicate := ConfigWarning{
		Code: "z_rule", Item: WarningItem{Kind: "service", Name: "zeta"},
		Title: "Z", Detail: "same detail",
	}
	input := []ConfigWarning{
		duplicate,
		{Code: "z_rule", Item: WarningItem{Kind: "env"}, Title: "Z env", Detail: "z"},
		{Code: "a_rule", Item: WarningItem{Kind: "backend"}, Title: "Backend", Detail: "b"},
		{Code: "z_rule", Item: WarningItem{Kind: "service", Name: "alpha"}, Title: "Z alpha", Detail: "z"},
		{Code: "a_rule", Item: WarningItem{Kind: "service", Name: "alpha"}, Title: "A alpha", Detail: "z"},
		{Code: "a_rule", Item: WarningItem{Kind: "service", Name: "alpha"}, Title: "A alpha", Detail: "a"},
		{Code: "a_rule", Item: WarningItem{Kind: "env"}, Title: "A env", Detail: "a"},
		duplicate,
	}

	got := normalizeConfigWarnings(input)
	if len(got) != 7 {
		t.Fatalf("normalized length = %d, want 7 after de-duplication: %#v", len(got), got)
	}
	wantOrder := []struct {
		kind, name, code, detail string
	}{
		{kind: "env", code: "a_rule", detail: "a"},
		{kind: "env", code: "z_rule", detail: "z"},
		{kind: "backend", code: "a_rule", detail: "b"},
		{kind: "service", name: "alpha", code: "a_rule", detail: "a"},
		{kind: "service", name: "alpha", code: "a_rule", detail: "z"},
		{kind: "service", name: "alpha", code: "z_rule", detail: "z"},
		{kind: "service", name: "zeta", code: "z_rule", detail: "same detail"},
	}
	for i, want := range wantOrder {
		gotKey := []string{got[i].Item.Kind, got[i].Item.Name, got[i].Code, got[i].Detail}
		wantKey := []string{want.kind, want.name, want.code, want.detail}
		for field := range wantKey {
			if gotKey[field] != wantKey[field] {
				t.Errorf("warning %d = %#v, want kind/name/code/detail %#v", i, got[i], wantKey)
				break
			}
		}
	}
}

func TestDedupeConfigWarningsKeepsSameCodeAndItemWithDifferentDetail(t *testing.T) {
	base := ConfigWarning{Code: "rule", Item: WarningItem{Kind: "env"}, Title: "title"}
	first := base
	first.Detail = "first"
	second := base
	second.Detail = "second"
	got := dedupeConfigWarnings([]ConfigWarning{first, second, first})
	if len(got) != 2 || got[0].Detail != "first" || got[1].Detail != "second" {
		t.Fatalf("de-duplication must retain distinct details and first occurrence order: %#v", got)
	}
}

// REQ-FR-3, REQ-FR-8; contract 8.6: preserve the CLI's substance and indentation.
func TestPrintConfigWarningsFormat(t *testing.T) {
	t.Run("multiline detail and docs", func(t *testing.T) {
		var out bytes.Buffer
		printConfigWarnings(&out, []ConfigWarning{{
			Code: "rule", Item: envItem(), Title: "A warning title",
			Detail: "First detail line\nSecond detail line",
			Docs:   "https://example.com/fix",
		}})
		want := "⚠️  A warning title\n" +
			"   First detail line\n" +
			"   Second detail line\n" +
			"   Docs: https://example.com/fix\n"
		if !strings.Contains(out.String(), want) {
			t.Fatalf("printed warning mismatch:\n--- got ---\n%s--- required output ---\n%s", out.String(), want)
		}
	})

	t.Run("no docs line when docs are absent", func(t *testing.T) {
		var out bytes.Buffer
		printConfigWarnings(&out, []ConfigWarning{{
			Code: "rule", Item: envItem(), Title: "Title", Detail: "What to do",
		}})
		if strings.Contains(out.String(), "Docs:") {
			t.Fatalf("warning without docs printed a docs line: %q", out.String())
		}
		if !strings.Contains(out.String(), "⚠️  Title\n   What to do\n") {
			t.Fatalf("warning without docs has wrong title/detail format: %q", out.String())
		}
	})
}

func TestDocsLinkIsAbsoluteHTTPSRepositoryURL(t *testing.T) {
	got := docsLink("ai_docs/SERVICE_NETWORK_ACCESS.md#health-check")
	want := "https://github.com/MadAppGang/meroku/blob/main/ai_docs/SERVICE_NETWORK_ACCESS.md#health-check"
	if got != want {
		t.Fatalf("docsLink() = %q, want %q", got, want)
	}
}

// Contract 8.2/8.4: region drift belongs to the deploying shell, never the web report.
func TestEnvLevelWarningsRegionDriftInputBoundary(t *testing.T) {
	envMap := map[string]interface{}{"region": "us-east-1"}
	cli := warningInputs{
		EnvName: "dev", EnvFile: "dev.yaml",
		AmbientRegion: func(profile string) (string, string) {
			return "eu-west-1", "AWS_REGION"
		},
		Siblings: func(dir, self string) ([]vpcCIDRUse, error) { return nil, nil },
	}
	cliWarnings := envLevelWarnings(envMap, cli)
	if !hasConfigWarningCode(cliWarnings, warnCodeRegionDrift) {
		t.Fatalf("CLI warnings omit region drift: %#v", cliWarnings)
	}

	web := webWarningInputs("dev")
	web.EnvFile = "dev.yaml"
	web.Siblings = func(dir, self string) ([]vpcCIDRUse, error) { return nil, nil }
	if web.AmbientRegion != nil {
		t.Fatal("web warning inputs must not consult the web server's ambient region")
	}
	webWarnings := envLevelWarnings(envMap, web)
	if hasConfigWarningCode(webWarnings, warnCodeRegionDrift) {
		t.Fatalf("web warnings include region drift: %#v", webWarnings)
	}
}

func hasConfigWarningCode(warnings []ConfigWarning, code string) bool {
	for _, warning := range warnings {
		if warning.Code == code {
			return true
		}
	}
	return false
}
