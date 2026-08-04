package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/aymerick/raymond"
)

// The AppSync Lambda authorizer used to fall back to a hardcoded third-party
// JWKS endpoint when nothing was configured, which meant anyone who could get a
// token from that provider could call the API. modules/appsync now declares
// jwks_uri with no default. These tests pin down the two halves of the fix on
// the meroku side: the value is actually wired through the template, and an
// empty one is refused before terraform is ever invoked.

func TestValidateAppSyncAuthorizer(t *testing.T) {
	tests := []struct {
		name       string
		enabled    bool
		authLambda bool
		jwksURI    string
		wantErr    bool
		wantIn     []string
	}{
		{
			name:    "appsync disabled needs nothing",
			enabled: false,
		},
		{
			name:       "disabled appsync ignores a missing jwks_uri",
			enabled:    false,
			authLambda: true,
		},
		{
			name:       "auth_lambda with empty jwks_uri is rejected",
			enabled:    true,
			authLambda: true,
			wantErr:    true,
			wantIn: []string{
				"pubsub_appsync.jwks_uri",
				"pubsub_appsync.auth_lambda is true",
				"https://<your-idp-host>/.well-known/jwks.json",
			},
		},
		{
			name:    "enabled appsync without auth_lambda still needs the key source",
			enabled: true,
			wantErr: true,
			wantIn:  []string{"pubsub_appsync.jwks_uri", "pubsub_appsync.enabled is true"},
		},
		{
			name:       "whitespace is not a value",
			enabled:    true,
			authLambda: true,
			jwksURI:    "   ",
			wantErr:    true,
			wantIn:     []string{"pubsub_appsync.jwks_uri"},
		},
		{
			name:       "plaintext http is rejected",
			enabled:    true,
			authLambda: true,
			jwksURI:    "http://idp.example.com/.well-known/jwks.json",
			wantErr:    true,
			wantIn:     []string{"must be an https:// URL"},
		},
		{
			name:       "https jwks_uri passes",
			enabled:    true,
			authLambda: true,
			jwksURI:    "https://idp.example.com/.well-known/jwks.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAppSyncAuthorizer(tt.enabled, tt.authLambda, tt.jwksURI)
			if tt.wantErr && err == nil {
				t.Fatal("expected an error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("expected no error, got: %v", err)
			}
			for _, want := range tt.wantIn {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error should name %q so the user knows what to fix, got:\n%v", want, err)
				}
			}
		})
	}
}

// Generation renders from the raw map, not from the Env struct, so the map path
// is the one that actually stands between a bad config and a generated main.tf.
func TestValidateAppSyncConfigMap(t *testing.T) {
	tests := []struct {
		name    string
		envMap  map[string]interface{}
		wantErr bool
	}{
		{
			name:   "no appsync section",
			envMap: map[string]interface{}{"project": "test"},
		},
		{
			name:   "nil appsync section",
			envMap: map[string]interface{}{"pubsub_appsync": nil},
		},
		{
			name: "string-keyed map, missing jwks_uri",
			envMap: map[string]interface{}{
				"pubsub_appsync": map[string]interface{}{"enabled": true, "auth_lambda": true},
			},
			wantErr: true,
		},
		{
			// Raw gopkg.in/yaml.v2 output, in case a caller skips convertToJSONCompatible.
			name: "interface-keyed map, missing jwks_uri",
			envMap: map[string]interface{}{
				"pubsub_appsync": map[interface{}]interface{}{"enabled": true, "auth_lambda": true},
			},
			wantErr: true,
		},
		{
			name: "configured jwks_uri passes",
			envMap: map[string]interface{}{
				"pubsub_appsync": map[string]interface{}{
					"enabled":     true,
					"auth_lambda": true,
					"jwks_uri":    "https://idp.example.com/.well-known/jwks.json",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAppSyncConfigMap(tt.envMap)
			if tt.wantErr != (err != nil) {
				t.Fatalf("wantErr=%v, got: %v", tt.wantErr, err)
			}
		})
	}
}

func TestValidateAppSyncConfig_Struct(t *testing.T) {
	env := createEnv("testproject", "dev")
	if err := validateAppSyncConfig(&env); err != nil {
		t.Fatalf("a fresh project has appsync disabled and must validate: %v", err)
	}

	env.AppSyncPubSub.Enabled = true
	env.AppSyncPubSub.AuthLambda = true
	if err := validateAppSyncConfig(&env); err == nil {
		t.Fatal("enabling the authorizer without a jwks_uri must fail")
	}

	env.AppSyncPubSub.JWKSURI = "https://idp.example.com/.well-known/jwks.json"
	if err := validateAppSyncConfig(&env); err != nil {
		t.Fatalf("a configured authorizer must validate: %v", err)
	}
}

// appsyncTemplateSource pulls the `{{#if pubsub_appsync.enabled}}` block out of
// the real env/main.hbs. Rendering the whole file would need a fully populated
// env map (helpers such as `array` panic on nil), and this keeps the assertions
// pinned to the template that actually ships rather than a copy in the test.
func appsyncTemplateSource(t *testing.T) string {
	t.Helper()

	path := filepath.Join("..", "env", "main.hbs")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	content := string(raw)

	const open = "{{#if pubsub_appsync.enabled}}"
	start := strings.Index(content, open)
	if start < 0 {
		t.Fatalf("%s no longer contains %s", path, open)
	}

	// Walk block tokens keeping a depth count, so nested {{#if}}s inside the
	// module block do not terminate it early.
	tokens := regexp.MustCompile(`\{\{#|\{\{/`).FindAllStringIndex(content[start:], -1)
	depth := 0
	for _, tok := range tokens {
		if content[start+tok[0]:start+tok[1]] == "{{#" {
			depth++
			continue
		}
		depth--
		if depth == 0 {
			end := strings.Index(content[start+tok[0]:], "}}")
			if end < 0 {
				t.Fatal("unterminated closing token in main.hbs")
			}
			return content[start : start+tok[0]+end+2]
		}
	}
	t.Fatalf("unbalanced %s block in %s", open, path)
	return ""
}

// renderAppSyncTemplate renders that block the same way applyTemplate does.
func renderAppSyncTemplate(t *testing.T, envMap map[string]interface{}) string {
	t.Helper()
	registerCustomHelpers()

	envMap["modules"] = "../../infrastructure/modules"
	envMap["custom_modules"] = "../../custom"

	out, err := raymond.Render(appsyncTemplateSource(t), envMap)
	if err != nil {
		t.Fatalf("rendering appsync block: %v", err)
	}
	return out
}

// appsyncModuleBlock extracts `module "appsync" { ... }` from generated HCL.
// The closing brace may carry indentation: raymond keeps the leading whitespace
// of a standalone {{#if}} line, so a skipped optional argument leaves it behind.
var closingBraceLine = regexp.MustCompile(`(?m)^[ \t]*\}[ \t]*$`)

func appsyncModuleBlock(t *testing.T, hcl string) string {
	t.Helper()
	start := strings.Index(hcl, `module "appsync"`)
	if start < 0 {
		return ""
	}
	loc := closingBraceLine.FindStringIndex(hcl[start:])
	if loc == nil {
		t.Fatalf("unterminated appsync module block:\n%s", hcl[start:])
	}
	return hcl[start : start+loc[1]]
}

func TestMainTemplate_RendersAppSyncAuthorizerConfig(t *testing.T) {
	out := renderAppSyncTemplate(t, map[string]interface{}{
		"project": "testproject",
		"env":     "dev",
		"pubsub_appsync": map[string]interface{}{
			"enabled":      true,
			"schema":       true,
			"resolvers":    true,
			"auth_lambda":  true,
			"jwks_uri":     "https://idp.example.com/.well-known/jwks.json",
			"jwt_issuer":   "https://idp.example.com/",
			"jwt_audience": "test-api",
		},
	})

	block := appsyncModuleBlock(t, out)
	if block == "" {
		t.Fatal("appsync module block was not rendered")
	}

	for _, want := range []string{
		`jwks_uri = "https://idp.example.com/.well-known/jwks.json"`,
		`jwt_issuer = "https://idp.example.com/"`,
		`jwt_audience = "test-api"`,
		// Regression: this used to be gated on pubsub_appsync.appsync.auth_lambda,
		// a path that never resolves, so the module silently used its bundled
		// authorizer instead of the project's own.
		`auth_lambda_path = "../../custom/appsync/auth_lambda"`,
		// Same class of bug: the gate read appsync.resolvers, not
		// pubsub_appsync.resolvers.
		`vtl_templates_yaml = "../../custom/appsync/vtl_templates.yaml"`,
	} {
		if !strings.Contains(block, want) {
			t.Errorf("generated HCL is missing %s\n\ngot:\n%s", want, block)
		}
	}
}

// jwt_issuer and jwt_audience are optional in the module. Emitting them empty
// would be harmless but noisy, and it hides whether a project pinned its claims.
func TestMainTemplate_OmitsEmptyOptionalClaims(t *testing.T) {
	out := renderAppSyncTemplate(t, map[string]interface{}{
		"project": "testproject",
		"env":     "dev",
		"pubsub_appsync": map[string]interface{}{
			"enabled":      true,
			"auth_lambda":  false,
			"jwks_uri":     "https://idp.example.com/.well-known/jwks.json",
			"jwt_issuer":   "",
			"jwt_audience": "",
		},
	})

	block := appsyncModuleBlock(t, out)
	if block == "" {
		t.Fatal("appsync module block was not rendered")
	}

	// jwks_uri has no default in the module, so it must be passed even when the
	// project does not override auth_lambda_path.
	if !strings.Contains(block, `jwks_uri = "https://idp.example.com/.well-known/jwks.json"`) {
		t.Errorf("jwks_uri must be passed on every instantiation:\n%s", block)
	}
	for _, unwanted := range []string{"jwt_issuer", "jwt_audience", "auth_lambda_path"} {
		if strings.Contains(block, unwanted) {
			t.Errorf("%s should be omitted when unset:\n%s", unwanted, block)
		}
	}
}

// The end-to-end guarantee: a config that would render an authorizer with no key
// source is rejected before any HCL is produced.
func TestAppSyncMisconfigurationIsCaughtBeforeRendering(t *testing.T) {
	envMap := map[string]interface{}{
		"project": "testproject",
		"env":     "dev",
		"pubsub_appsync": map[string]interface{}{
			"enabled":     true,
			"auth_lambda": true,
			"jwks_uri":    "",
		},
	}

	err := validateAppSyncConfigMap(envMap)
	if err == nil {
		t.Fatal("empty jwks_uri must be rejected")
	}

	// Without the guard this renders happily into an unplannable module block.
	block := appsyncModuleBlock(t, renderAppSyncTemplate(t, envMap))
	if !strings.Contains(block, `jwks_uri = ""`) {
		t.Errorf("sanity check: template should render an empty jwks_uri, which is exactly what validation prevents:\n%s", block)
	}
}
