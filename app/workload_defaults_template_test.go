package main

import (
	"regexp"
	"testing"
)

// The workloads module block in env/main.hbs takes several workload values that
// older or hand-written YAML files leave out. The template renders straight from
// the YAML map, so each of these lines needs its own fallback; without one the
// line renders as `name = ` and the generated main.tf is not valid HCL.

// workloadDefaultLines lists each attribute, the YAML key under workload that
// feeds it, and the value it must render when that key is absent.
var workloadDefaultLines = []struct {
	attr     string
	yamlKey  string
	fallback string
}{
	{"backend_bucket_public", "bucket_public", "false"},
	{"setup_FCM_SNS", "setup_fcnsns", "false"},
	{"backend_image_port", "backend_image_port", "8080"},
	{"github_oidc_enabled", "enable_github_oidc", "false"},
	{"xray_enabled", "xray_enabled", "false"},
	{"pgadmin_enabled", "install_pg_admin", "false"},
}

// attrLine matches `attr = value` on its own line, tolerating indentation and
// any alignment whitespace around the equals sign.
func attrLine(attr, value string) *regexp.Regexp {
	return regexp.MustCompile(`(?m)^[ \t]*` + regexp.QuoteMeta(attr) + `[ \t]*=[ \t]*` + regexp.QuoteMeta(value) + `[ \t]*$`)
}

// renderedValue returns what the template rendered for attr, or "<missing>"
// when no such line exists, so a failure shows the actual output.
func renderedValue(out, attr string) string {
	re := regexp.MustCompile(`(?m)^[ \t]*` + regexp.QuoteMeta(attr) + `[ \t]*=(.*)$`)
	m := re.FindStringSubmatch(out)
	if m == nil {
		return "<missing>"
	}
	return "=" + m[1]
}

func TestTemplate_WorkloadDefaults_BareWorkloadRendersValidHCL(t *testing.T) {
	out := renderMainTemplate(t, minimalEnvMap(""))

	if err := validateGeneratedHCL("main.tf", out); err != nil {
		t.Fatalf("a workload map with none of the optional keys must render valid HCL:\n%v", err)
	}
}

func TestTemplate_WorkloadDefaults_AbsentKeysRenderFallback(t *testing.T) {
	out := renderMainTemplate(t, minimalEnvMap(""))

	for _, tc := range workloadDefaultLines {
		t.Run(tc.attr, func(t *testing.T) {
			if !attrLine(tc.attr, tc.fallback).MatchString(out) {
				t.Errorf("with workload.%s absent, want `%s = %s`, got `%s %s`",
					tc.yamlKey, tc.attr, tc.fallback, tc.attr, renderedValue(out, tc.attr))
			}
		})
	}
}

func TestTemplate_WorkloadDefaults_ExplicitValuesSurvive(t *testing.T) {
	cases := []struct {
		name     string
		workload map[string]interface{}
		want     map[string]string
	}{
		{
			name: "enabled",
			workload: map[string]interface{}{
				"bucket_public":      true,
				"setup_fcnsns":       true,
				"backend_image_port": 3000,
				"enable_github_oidc": true,
				"xray_enabled":       true,
				"install_pg_admin":   true,
			},
			want: map[string]string{
				"backend_bucket_public": "true",
				"setup_FCM_SNS":         "true",
				"backend_image_port":    "3000",
				"github_oidc_enabled":   "true",
				"xray_enabled":          "true",
				"pgadmin_enabled":       "true",
			},
		},
		{
			// false and 0 are real values, not missing ones, and must not be
			// replaced by the fallback.
			name: "explicit false and zero",
			workload: map[string]interface{}{
				"bucket_public":      false,
				"setup_fcnsns":       false,
				"backend_image_port": 0,
				"enable_github_oidc": false,
				"xray_enabled":       false,
				"install_pg_admin":   false,
			},
			want: map[string]string{
				"backend_bucket_public": "false",
				"setup_FCM_SNS":         "false",
				"backend_image_port":    "0",
				"github_oidc_enabled":   "false",
				"xray_enabled":          "false",
				"pgadmin_enabled":       "false",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := minimalEnvMap("")
			m["workload"] = tc.workload
			out := renderMainTemplate(t, m)

			for attr, value := range tc.want {
				if !attrLine(attr, value).MatchString(out) {
					t.Errorf("want `%s = %s`, got `%s %s`", attr, value, attr, renderedValue(out, attr))
				}
			}
		})
	}
}
