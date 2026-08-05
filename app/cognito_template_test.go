package main

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// cognitoModuleBlock returns the body of `module "cognito" { ... }`.
func cognitoModuleBlock(t *testing.T, rendered string) string {
	t.Helper()

	start := strings.Index(rendered, `module "cognito" {`)
	if start < 0 {
		t.Fatal(`module "cognito" not found in the rendered template`)
	}
	rest := rendered[start:]
	end := strings.Index(rest, "\n}\n")
	if end < 0 {
		t.Fatal(`module "cognito" block is unterminated`)
	}
	return rest[:end]
}

// The end-to-end shape of the defect: a Cognito-enabled config with neither
// array key set.
//
// This is not an exotic case. cognito.dashboard_callback_urls was unreachable
// from the Go struct for its whole life (the yaml tag read
// dashboard_callback_ur_ls), so every config meroku has ever written is missing
// it — and the helper panicked on the nil that produced. Cognito being off
// everywhere is the only reason nobody hit it.
func TestMainHBS_CognitoEnabledWithNoArrayKeys(t *testing.T) {
	rendered := renderMainHBS(t, map[string]interface{}{
		"cognito": map[string]interface{}{
			"enabled":                 true,
			"enable_web_client":       true,
			"enable_dashboard_client": true,
			"enable_user_pool_domain": false,
			"backend_confirm_signup":  false,
			// dashboard_callback_urls and auto_verified_attributes deliberately absent.
		},
	})

	block := cognitoModuleBlock(t, rendered)
	for _, want := range []string{
		"dashboard_callback_urls = []",
		"auto_verified_attributes = []",
	} {
		if !strings.Contains(block, want) {
			t.Errorf("expected %q in:\n%s", want, block)
		}
	}
	if strings.Contains(block, "null") {
		t.Errorf("terraform rejects null where it wants list(string):\n%s", block)
	}
}

// The values a user sets have to survive the trip. The misspelt yaml tag meant
// they did not: loading dropped them, so generation saw nothing whatever the
// file said.
func TestMainHBS_CognitoCallbackURLsReachTheModule(t *testing.T) {
	rendered := renderMainHBS(t, map[string]interface{}{
		"cognito": map[string]interface{}{
			"enabled":                  true,
			"enable_dashboard_client":  true,
			"dashboard_callback_urls":  []interface{}{"https://admin.example.com/cb"},
			"auto_verified_attributes": []interface{}{"email"},
		},
	})

	block := cognitoModuleBlock(t, rendered)
	if !strings.Contains(block, `dashboard_callback_urls = ["https://admin.example.com/cb"]`) {
		t.Errorf("configured callback URLs did not reach the module:\n%s", block)
	}
	if !strings.Contains(block, `auto_verified_attributes = ["email"]`) {
		t.Errorf("configured attributes did not reach the module:\n%s", block)
	}
}

// Every key env/main.hbs reads out of the cognito block must exist on the
// Cognito struct under exactly that name, or the value a user sets is silently
// dropped on load and written back under a name nothing reads. That is the shape
// of the dashboard_callback_ur_ls defect, and this is the test that would have
// caught it.
func TestCognitoStructCoversEveryTemplateKey(t *testing.T) {
	// Read from the template rather than restating it, so a new {{cognito.x}}
	// call site fails here until the struct gains the matching field.
	raw, err := os.ReadFile(filepath.Join("..", "env", "main.hbs"))
	if err != nil {
		t.Fatalf("reading env/main.hbs: %v", err)
	}

	// Only inside a {{...}} expression: `module.cognito.user_pool_id` is HCL the
	// template emits, not a config key it reads.
	refs := regexp.MustCompile(`\{\{[^{}]*?\bcognito\.([a-z0-9_]+)`)

	keys := map[string]bool{}
	for _, match := range refs.FindAllStringSubmatch(string(raw), -1) {
		keys[match[1]] = true
	}
	if len(keys) == 0 {
		t.Fatal("found no cognito.* references in env/main.hbs — the scan is broken")
	}

	tagged := map[string]bool{}
	cognitoType := reflect.TypeOf(Cognito{})
	for i := 0; i < cognitoType.NumField(); i++ {
		tag := cognitoType.Field(i).Tag.Get("yaml")
		tagged[strings.Split(tag, ",")[0]] = true
	}

	for key := range keys {
		if !tagged[key] {
			t.Errorf("env/main.hbs reads cognito.%s but no Cognito struct field is tagged %q — "+
				"a value set there is dropped on load and never reaches generation", key, key)
		}
	}
}
