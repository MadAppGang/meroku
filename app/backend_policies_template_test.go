package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aymerick/raymond"
)

// A custom/pre module can add statements to the backend task role through a
// `backend_policies` output, merged with workload.policy into backend_policy.
//
// The reference is direct, never wrapped in try(). try() returns a wholly
// unknown value as soon as its argument is partly unknown, and a statement
// whose resources name something created in the same apply is partly unknown
// at plan. The whole list, and with it the count of the backend's custom
// policy, would be unknown, and Terraform stops with "Invalid count argument".
// So the generator sets custom_pre_backend_policies only when custom/pre
// declares the output, and the template references it only then.

// renderMainTemplateWithCustomPre is renderMainTemplate with the custom/pre
// switches under the caller's control: whether custom/pre exists, and whether
// it declares a backend_policies output. renderMainTemplate sets neither.
func renderMainTemplateWithCustomPre(t *testing.T, envMap map[string]interface{}, hasCustomPre, declaresBackendPolicies bool) string {
	t.Helper()

	registerCustomHelpers()

	raw, err := os.ReadFile(filepath.Join("..", "env", "main.hbs"))
	if err != nil {
		t.Fatalf("reading env/main.hbs: %v", err)
	}

	envMap["modules"] = "../../infrastructure/modules"
	envMap["custom_modules"] = "../../custom"
	envMap["has_custom_pre"] = hasCustomPre
	envMap[customPreBackendPoliciesKey] = declaresBackendPolicies
	envMap["has_custom_post"] = false

	tmpl, err := raymond.Parse(string(raw))
	if err != nil {
		t.Fatalf("parsing env/main.hbs: %v", err)
	}
	out, err := tmpl.Exec(envMap)
	if err != nil {
		t.Fatalf("executing env/main.hbs: %v", err)
	}
	return out
}

// backendPolicyLine returns the rendered `backend_policy = ...` line, failing
// the test if there is none.
func backendPolicyLine(t *testing.T, rendered string) string {
	t.Helper()
	for _, line := range strings.Split(rendered, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "backend_policy =") {
			return strings.TrimSpace(line)
		}
	}
	t.Fatal("rendered main.tf has no `backend_policy =` argument")
	return ""
}

func TestTemplate_CustomPreBackendPoliciesAreMergedDirectly(t *testing.T) {
	const ref = "module.custom_pre.backend_policies"

	t.Run("custom/pre declares backend_policies", func(t *testing.T) {
		env := minimalEnvMap("")
		env["workload"] = map[string]interface{}{
			"policy": []interface{}{
				map[string]interface{}{
					"actions":   []interface{}{"dynamodb:GetItem"},
					"resources": []interface{}{"arn:aws:dynamodb:us-east-1:000000000000:table/example"},
				},
			},
		}
		line := backendPolicyLine(t, renderMainTemplateWithCustomPre(t, env, true, true))

		if !strings.Contains(line, ref) {
			t.Errorf("backend_policy does not merge %s: %s", ref, line)
		}
		if strings.Contains(line, "try(") {
			t.Errorf("backend_policy wraps the custom/pre output in try(), which makes the policy count unknown at plan: %s", line)
		}
		if !strings.Contains(line, `"dynamodb:GetItem"`) {
			t.Errorf("backend_policy dropped workload.policy when merging custom/pre: %s", line)
		}
	})

	t.Run("custom/pre without a backend_policies output", func(t *testing.T) {
		out := renderMainTemplateWithCustomPre(t, minimalEnvMap(""), true, false)
		if strings.Contains(out, "custom_pre.backend_policies") {
			t.Error("a custom/pre module without a backend_policies output must not be referenced for it")
		}
	})

	t.Run("without custom/pre", func(t *testing.T) {
		out := renderMainTemplateWithCustomPre(t, minimalEnvMap(""), false, false)
		if strings.Contains(out, "custom_pre.backend_policies") {
			t.Error("without custom/pre the template must not reference module.custom_pre.backend_policies")
		}
	})
}

// workload.backend_policies is moved onto workload.policy by migration v33 and
// refused at generate; the template must never read it as a second source.
func TestTemplate_BackendPoliciesKeyIsNotRendered(t *testing.T) {
	env := minimalEnvMap("")
	env["workload"] = map[string]interface{}{
		"backend_policies": []interface{}{
			map[string]interface{}{
				"actions":   []interface{}{"sqs:SendMessage"},
				"resources": []interface{}{"arn:aws:sqs:us-east-1:000000000000:legacy"},
			},
		},
		"policies": []interface{}{"arn:aws:iam::aws:policy/ReadOnlyAccess"},
	}
	out := renderMainTemplate(t, env)

	if line := backendPolicyLine(t, out); strings.Contains(line, "sqs:SendMessage") {
		t.Errorf("backend_policy rendered workload.backend_policies: %s", line)
	}
	if strings.Contains(out, "ReadOnlyAccess") {
		t.Error("the template rendered a workload.policies ARN")
	}
}
