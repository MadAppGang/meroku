package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCustomPreDeclaresBackendPolicies(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  bool
		// wantWarning names the file a warning must mention; "" means none.
		wantWarning string
	}{
		{name: "no custom/pre directory", want: false},
		{
			name:  "outputs without backend_policies",
			files: map[string]string{"outputs.tf": "output \"backend_env_vars\" {\n  value = []\n}\n"},
			want:  false,
		},
		{
			name:  "backend_policies only in a comment or another name",
			files: map[string]string{"outputs.tf": "# output \"backend_policies\" {\noutput \"backend_policies_extra\" {\n  value = []\n}\n"},
			want:  false,
		},
		{
			name:  "declared in outputs.tf",
			files: map[string]string{"outputs.tf": "output \"backend_policies\" {\n  value = []\n}\n"},
			want:  true,
		},
		{
			name: "declared indented, with extra spaces, in another file",
			files: map[string]string{
				"main.tf":  "resource \"terraform_data\" \"x\" {}\n",
				"extra.tf": "  output   \"backend_policies\"   {\n  value = []\n}\n",
			},
			want: true,
		},
		{
			name:  "a non-.tf file is ignored",
			files: map[string]string{"notes.md": "output \"backend_policies\" {\n"},
			want:  false,
		},
		{
			name: "the declaration inside a block comment",
			files: map[string]string{"outputs.tf": "/*\noutput \"backend_policies\" {\n  value = []\n}\n*/\n" +
				"output \"backend_env_vars\" {\n  value = []\n}\n"},
			want: false,
		},
		{
			name:  "the declaration inside a heredoc string",
			files: map[string]string{"outputs.tf": "output \"readme\" {\n  value = <<EOT\noutput \"backend_policies\" {\n  value = []\n}\nEOT\n}\n"},
			want:  false,
		},
		{
			name:  "a comment between output and the label",
			files: map[string]string{"outputs.tf": "output /* statements for the backend */ \"backend_policies\" {\n  value = []\n}\n"},
			want:  true,
		},
		{
			name:  "an unquoted label",
			files: map[string]string{"outputs.tf": "output backend_policies {\n  value = []\n}\n"},
			want:  true,
		},
		{
			name:        "an unparsable file is not a declaration and is named in a warning",
			files:       map[string]string{"broken.tf": "output \"backend_policies\" {\n  value = [\n"},
			want:        false,
			wantWarning: "broken.tf",
		},
		{
			name: "an unparsable file does not hide a declaration in another file",
			files: map[string]string{
				"broken.tf":  "resource \"terraform_data\" {{{\n",
				"outputs.tf": "output \"backend_policies\" {\n  value = []\n}\n",
			},
			want:        true,
			wantWarning: "broken.tf",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			if test.files != nil {
				pre := filepath.Join(dir, "custom", "pre")
				if err := os.MkdirAll(pre, 0o755); err != nil {
					t.Fatal(err)
				}
				for name, content := range test.files {
					if err := os.WriteFile(filepath.Join(pre, name), []byte(content), 0o644); err != nil {
						t.Fatal(err)
					}
				}
			}
			chdir(t, dir)

			got, warnings := customPreDeclaresBackendPolicies()
			if got != test.want {
				t.Errorf("customPreDeclaresBackendPolicies() = %v, want %v", got, test.want)
			}
			switch {
			case test.wantWarning == "" && len(warnings) > 0:
				t.Errorf("unexpected warnings: %q", warnings)
			case test.wantWarning != "":
				if len(warnings) != 1 || !strings.Contains(warnings[0], test.wantWarning) {
					t.Errorf("want one warning naming %s, got %q", test.wantWarning, warnings)
				}
			}
		})
	}
}

// workload.policy is the backend's one statement key. Empty legacy keys are
// accepted: they grant nothing, and older struct saves wrote `policies: []`
// into every file.
func TestLegacyBackendPolicyKeys_AcceptsPolicyAndEmptyLegacyKeys(t *testing.T) {
	for name, workload := range map[string]interface{}{
		"policy only": map[string]interface{}{
			"policy": []interface{}{
				map[string]interface{}{
					"actions":   []interface{}{"s3:GetObject"},
					"resources": []interface{}{"arn:aws:s3:::example-bucket/*"},
				},
			},
		},
		"empty legacy keys": map[interface{}]interface{}{
			"policies":         []interface{}{},
			"backend_policies": nil,
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateLegacyBackendPolicyKeysConfigMap(map[string]interface{}{"workload": workload}); err != nil {
				t.Fatalf("refused: %v", err)
			}
		})
	}
	if err := validateLegacyBackendPolicyKeysConfigMap(map[string]interface{}{}); err != nil {
		t.Fatalf("config without workload refused: %v", err)
	}
}

// A backend_policies the migration could not move is refused, naming the key
// to use instead.
func TestLegacyBackendPolicyKeys_RefusesBackendPoliciesNamingPolicy(t *testing.T) {
	for name, workload := range map[string]interface{}{
		"string-keyed, not a list": map[string]interface{}{"backend_policies": "s3:GetObject"},
		"yaml mapping, a list": map[interface{}]interface{}{"backend_policies": []interface{}{
			map[interface{}]interface{}{"actions": []interface{}{"s3:GetObject"}, "resources": []interface{}{"*"}},
		}},
	} {
		t.Run(name, func(t *testing.T) {
			err := validateLegacyBackendPolicyKeysConfigMap(map[string]interface{}{"workload": workload})
			if err == nil {
				t.Fatal("a non-empty workload.backend_policies must be refused")
			}
			for _, want := range []string{"workload.backend_policies", "workload.policy"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not mention %q", err, want)
				}
			}
		})
	}
}

// Managed policy ARNs do not belong in an env YAML. The refusal says what to
// do instead: policy statements, or an attachment from custom/post.
func TestLegacyBackendPolicyKeys_RefusesManagedPolicyARNs(t *testing.T) {
	err := validateLegacyBackendPolicyKeysConfigMap(map[string]interface{}{
		"workload": map[string]interface{}{
			"policies": []interface{}{"arn:aws:iam::aws:policy/ReadOnlyAccess"},
		},
	})
	if err == nil {
		t.Fatal("a non-empty workload.policies must be refused")
	}
	for _, want := range []string{"workload.policies", "workload.policy", "custom/post"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

func TestPreprocessEnvMap_RefusesLegacyBackendPolicyKeys(t *testing.T) {
	m := minimalEnvMap("")
	m["workload"] = map[string]interface{}{"policies": []interface{}{"arn:aws:iam::aws:policy/ReadOnlyAccess"}}
	if err := preprocessEnvMap(m); err == nil || !strings.Contains(err.Error(), "workload.policies") {
		t.Fatalf("preprocessEnvMap must refuse workload.policies, got %v", err)
	}
}
