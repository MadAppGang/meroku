package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRegionDriftWarningOnlyWhenTheyDisagree(t *testing.T) {
	cases := []struct {
		name       string
		yaml       string
		ambient    string
		wantWarned bool
	}{
		{"agree", "us-east-1", "us-east-1", false},
		{"disagree", "us-east-1", "eu-west-1", true},
		{"nothing ambient", "us-east-1", "", false},
		{"no yaml region", "", "eu-west-1", false},
		{"whitespace still agrees", " us-east-1 ", "us-east-1", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := regionDriftWarning(tc.yaml, tc.ambient, "AWS_REGION")
			if warned := got != ""; warned != tc.wantWarned {
				t.Fatalf("warned=%v, want %v (warning: %q)", warned, tc.wantWarned, got)
			}
		})
	}
}

// The warning has one job: make the operator read the plan before an existing
// stack is moved. It has to name both regions and say the move is destructive.
func TestRegionDriftWarningNamesBothRegionsAndTheRisk(t *testing.T) {
	got := regionDriftWarning("us-east-1", "eu-west-1", "AWS_REGION")

	for _, want := range []string{"us-east-1", "eu-west-1", "AWS_REGION", "destroy"} {
		if !strings.Contains(got, want) {
			t.Errorf("warning does not mention %q:\n%s", want, got)
		}
	}
}

func TestResolveAmbientRegionPrecedence(t *testing.T) {
	// A config file with a region, so the env vars have something to outrank.
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	if err := os.WriteFile(cfg, []byte("[profile staging]\nregion = ap-southeast-2\n\n[default]\nregion = us-west-1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AWS_CONFIG_FILE", cfg)

	t.Run("AWS_REGION wins", func(t *testing.T) {
		t.Setenv("AWS_REGION", "eu-west-1")
		t.Setenv("AWS_DEFAULT_REGION", "eu-central-1")
		region, source := resolveAmbientRegion("staging")
		if region != "eu-west-1" || source != "AWS_REGION" {
			t.Fatalf("got (%q, %q), want (eu-west-1, AWS_REGION)", region, source)
		}
	})

	t.Run("AWS_DEFAULT_REGION is next", func(t *testing.T) {
		t.Setenv("AWS_REGION", "")
		t.Setenv("AWS_DEFAULT_REGION", "eu-central-1")
		region, source := resolveAmbientRegion("staging")
		if region != "eu-central-1" || source != "AWS_DEFAULT_REGION" {
			t.Fatalf("got (%q, %q), want (eu-central-1, AWS_DEFAULT_REGION)", region, source)
		}
	})

	t.Run("named profile last", func(t *testing.T) {
		t.Setenv("AWS_REGION", "")
		t.Setenv("AWS_DEFAULT_REGION", "")
		region, _ := resolveAmbientRegion("staging")
		if region != "ap-southeast-2" {
			t.Fatalf("got %q, want ap-southeast-2 from the staging profile", region)
		}
	})

	t.Run("falls back to default section", func(t *testing.T) {
		t.Setenv("AWS_REGION", "")
		t.Setenv("AWS_DEFAULT_REGION", "")
		t.Setenv("AWS_PROFILE", "")
		region, _ := resolveAmbientRegion("")
		if region != "us-west-1" {
			t.Fatalf("got %q, want us-west-1 from [default]", region)
		}
	})

	t.Run("unknown profile resolves to nothing", func(t *testing.T) {
		t.Setenv("AWS_REGION", "")
		t.Setenv("AWS_DEFAULT_REGION", "")
		region, _ := resolveAmbientRegion("nope")
		if region != "" {
			t.Fatalf("got %q, want empty for a profile that is not in the config", region)
		}
	})
}

// The whole point of the warning is that the template pins the region. If this
// fails, the split-brain the warning describes is back and the warning lies.
func TestDefaultProviderPinsTheConfiguredRegion(t *testing.T) {
	rendered := renderMainHBS(t, map[string]interface{}{"region": "ap-southeast-2"})

	start := strings.Index(rendered, `provider "aws" {`)
	if start < 0 {
		t.Fatal(`no default provider "aws" block in the rendered template`)
	}
	rest := rendered[start:]
	end := strings.Index(rest, "\n}\n")
	if end < 0 {
		t.Fatal("default provider block is unterminated")
	}
	block := rest[:end]

	if !strings.Contains(block, `region = "ap-southeast-2"`) {
		t.Errorf("default provider does not pin the config region:\n%s", block)
	}

	// The block exists to carry default_tags; pinning the region must not have
	// displaced them.
	if !strings.Contains(block, "default_tags") {
		t.Errorf("default provider lost its default_tags:\n%s", block)
	}
}
