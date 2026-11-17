package main

import "testing"

// The cognito module block in env/main.hbs takes three booleans that a YAML
// with cognito enabled may leave out. Each needs its own fallback, matching the
// module's default of false; without one the line renders as `name = ` and the
// generated main.tf is not valid HCL.

// cognitoDefaultLines lists each attribute and the YAML key under cognito that
// feeds it.
var cognitoDefaultLines = []struct {
	attr    string
	yamlKey string
}{
	{"enable_dashboard_client", "enable_dashboard_client"},
	{"enable_user_pool_domain", "enable_user_pool_domain"},
	{"allow_backend_task_to_confirm_signup", "backend_confirm_signup"},
}

// cognitoEnvMap returns a minimal env map whose cognito block holds only
// enabled: true plus the given extra keys.
func cognitoEnvMap(extra map[string]interface{}) map[string]interface{} {
	cognito := map[string]interface{}{"enabled": true}
	for k, v := range extra {
		cognito[k] = v
	}
	m := minimalEnvMap("")
	m["cognito"] = cognito
	return m
}

func TestTemplate_CognitoDefaults_EnabledOnlyRendersValidHCL(t *testing.T) {
	out := renderMainTemplate(t, cognitoEnvMap(nil))

	if err := validateGeneratedHCL("main.tf", out); err != nil {
		t.Fatalf("a cognito map with only enabled: true must render valid HCL:\n%v", err)
	}
}

func TestTemplate_CognitoDefaults_AbsentKeysRenderFalse(t *testing.T) {
	out := renderMainTemplate(t, cognitoEnvMap(nil))

	for _, tc := range cognitoDefaultLines {
		t.Run(tc.attr, func(t *testing.T) {
			if !attrLine(tc.attr, "false").MatchString(out) {
				t.Errorf("with cognito.%s absent, want `%s = false`, got `%s %s`",
					tc.yamlKey, tc.attr, tc.attr, renderedValue(out, tc.attr))
			}
		})
	}
}

func TestTemplate_CognitoDefaults_ExplicitValuesSurvive(t *testing.T) {
	for _, value := range []bool{true, false} {
		want := "false"
		if value {
			want = "true"
		}
		t.Run(want, func(t *testing.T) {
			extra := map[string]interface{}{}
			for _, tc := range cognitoDefaultLines {
				extra[tc.yamlKey] = value
			}
			out := renderMainTemplate(t, cognitoEnvMap(extra))

			for _, tc := range cognitoDefaultLines {
				if !attrLine(tc.attr, want).MatchString(out) {
					t.Errorf("with cognito.%s: %v, want `%s = %s`, got `%s %s`",
						tc.yamlKey, value, tc.attr, want, tc.attr, renderedValue(out, tc.attr))
				}
			}
		})
	}
}
