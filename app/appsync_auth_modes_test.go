package main

import (
	"strings"
	"testing"
)

// modules/appsync used to hardcode authentication_type = "AWS_LAMBDA" and attach
// an API_KEY provider to every deployment, which meant the Lambda authorizer -
// and every hardening applied to it - could be skipped by anyone holding the
// key. Authorization is now a per-environment choice between three modes, and
// the API key is opt-in.
//
// These tests pin the meroku half of that: each mode's required field is
// demanded by name before any HCL is produced, and the template passes the right
// arguments (and only those) for the mode in play.

// --- validation, per mode ----------------------------------------------------

func TestValidateAppSyncAuth_PerModeRequirements(t *testing.T) {
	tests := []struct {
		name           string
		enabled        bool
		mode           string
		cognitoEnabled bool
		oidcIssuer     string
		jwksURI        string
		wantErr        bool
		// wantIn asserts the message names the YAML key to edit. An error that
		// says "invalid configuration" costs the reader a grep.
		wantIn []string
	}{
		{
			name:    "disabled appsync needs nothing from any mode",
			enabled: false,
			mode:    "cognito",
		},
		{
			name:    "cognito mode without a user pool is rejected",
			enabled: true,
			mode:    "cognito",
			wantErr: true,
			wantIn: []string{
				`pubsub_appsync.auth_mode is "cognito"`,
				"cognito.enabled is false",
				"cognito:\n    enabled: true",
			},
		},
		{
			name:           "cognito mode with the pool enabled passes",
			enabled:        true,
			mode:           "cognito",
			cognitoEnabled: true,
		},
		{
			name:    "oidc mode without an issuer is rejected",
			enabled: true,
			mode:    "oidc",
			wantErr: true,
			wantIn: []string{
				"pubsub_appsync.oidc_issuer is required",
				`pubsub_appsync.auth_mode is "oidc"`,
				"oidc_client_id",
			},
		},
		{
			name:       "oidc issuer over plaintext http is rejected",
			enabled:    true,
			mode:       "oidc",
			oidcIssuer: "http://idp.example.com",
			wantErr:    true,
			wantIn:     []string{"pubsub_appsync.oidc_issuer must be an https:// URL"},
		},
		{
			name:       "oidc mode with an https issuer passes",
			enabled:    true,
			mode:       "oidc",
			oidcIssuer: "https://idp.example.com",
		},
		{
			name:    "lambda mode without a jwks_uri is rejected",
			enabled: true,
			mode:    "lambda",
			wantErr: true,
			wantIn: []string{
				"pubsub_appsync.jwks_uri is required",
				`pubsub_appsync.auth_mode is "lambda"`,
				// The message should also point at the cheaper modes, because
				// most people reading it do not need a Lambda at all.
				`auth_mode: "oidc"`,
				`auth_mode: "cognito"`,
			},
		},
		{
			name:    "lambda mode with an https jwks_uri passes",
			enabled: true,
			mode:    "lambda",
			jwksURI: "https://idp.example.com/.well-known/jwks.json",
		},
		{
			// Absent must mean what the module did before auth_mode existed,
			// which is lambda. Anything else would change deployed behaviour on
			// a file that has not been migrated yet.
			name:    "empty mode falls back to lambda and demands its field",
			enabled: true,
			mode:    "",
			wantErr: true,
			wantIn:  []string{"pubsub_appsync.jwks_uri is required"},
		},
		{
			name:    "empty mode with a jwks_uri passes",
			enabled: true,
			mode:    "",
			jwksURI: "https://idp.example.com/.well-known/jwks.json",
		},
		{
			name:    "case and whitespace are tolerated",
			enabled: true,
			mode:    "  Cognito ",
			// Deliberately no cognito pool, so a rejection here proves the value
			// was understood as cognito rather than falling through to unknown.
			wantErr: true,
			wantIn:  []string{"cognito.enabled is false"},
		},
		{
			name:    "an unknown mode is named with the valid set",
			enabled: true,
			mode:    "passflow",
			wantErr: true,
			wantIn: []string{
				"pubsub_appsync.auth_mode must be one of",
				`"cognito", "oidc", "lambda"`,
				`got "passflow"`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAppSyncAuth(tt.enabled, tt.mode, tt.cognitoEnabled, tt.oidcIssuer, tt.jwksURI, false)
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

// Cognito mode is the one requirement that reaches outside the appsync block, so
// the map path has to read a second section. Getting this wrong would generate a
// main.tf referencing module.cognito when no such module was emitted.
func TestValidateAppSyncConfigMap_CognitoModeReadsTheCognitoSection(t *testing.T) {
	tests := []struct {
		name    string
		envMap  map[string]interface{}
		wantErr bool
	}{
		{
			name: "cognito mode, no cognito section at all",
			envMap: map[string]interface{}{
				"pubsub_appsync": map[string]interface{}{"enabled": true, "auth_mode": "cognito"},
			},
			wantErr: true,
		},
		{
			name: "cognito mode, cognito disabled",
			envMap: map[string]interface{}{
				"cognito":        map[string]interface{}{"enabled": false},
				"pubsub_appsync": map[string]interface{}{"enabled": true, "auth_mode": "cognito"},
			},
			wantErr: true,
		},
		{
			name: "cognito mode, cognito enabled",
			envMap: map[string]interface{}{
				"cognito":        map[string]interface{}{"enabled": true},
				"pubsub_appsync": map[string]interface{}{"enabled": true, "auth_mode": "cognito"},
			},
		},
		{
			// Raw gopkg.in/yaml.v2 output, in case a caller skips
			// convertToJSONCompatible.
			name: "interface-keyed maps are read the same way",
			envMap: map[string]interface{}{
				"cognito":        map[interface{}]interface{}{"enabled": true},
				"pubsub_appsync": map[interface{}]interface{}{"enabled": true, "auth_mode": "cognito"},
			},
		},
		{
			name: "oidc mode, issuer present",
			envMap: map[string]interface{}{
				"pubsub_appsync": map[string]interface{}{
					"enabled": true, "auth_mode": "oidc", "oidc_issuer": "https://idp.example.com",
				},
			},
		},
		{
			name: "oidc mode, issuer missing",
			envMap: map[string]interface{}{
				"pubsub_appsync": map[string]interface{}{"enabled": true, "auth_mode": "oidc"},
			},
			wantErr: true,
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

func TestValidateAppSyncConfig_StructUsesEnvironmentCognito(t *testing.T) {
	env := createEnv("testproject", "dev")
	env.AppSyncPubSub.Enabled = true
	env.AppSyncPubSub.AuthMode = AppSyncAuthCognito

	if err := validateAppSyncConfig(&env); err == nil {
		t.Fatal("cognito mode must be refused while cognito.enabled is false")
	}

	env.Cognito.Enabled = true
	if err := validateAppSyncConfig(&env); err != nil {
		t.Fatalf("cognito mode with the pool enabled must validate: %v", err)
	}

	env.AppSyncPubSub.AuthMode = AppSyncAuthOIDC
	if err := validateAppSyncConfig(&env); err == nil {
		t.Fatal("oidc mode must be refused without an issuer")
	}

	env.AppSyncPubSub.OIDCIssuer = "https://idp.example.com"
	if err := validateAppSyncConfig(&env); err != nil {
		t.Fatalf("oidc mode with an issuer must validate: %v", err)
	}
}

// --- claim policy ------------------------------------------------------------
//
// required_claims is the one capability that genuinely selects lambda mode.
// Accepting it in a mode that cannot enforce it would hand someone a rule they
// believe is running when it is not.

func TestValidateAppSyncRequiredClaims(t *testing.T) {
	tests := []struct {
		name    string
		in      appSyncAuthInput
		wantErr bool
		wantIn  []string
	}{
		{
			name: "claims in lambda mode are fine",
			in: appSyncAuthInput{
				Enabled: true, Mode: "lambda",
				JWKSURI:        "https://idp.example.com/.well-known/jwks.json",
				RequiredClaims: map[string][]string{"role": {"admin"}, "tenant_id": {}},
			},
		},
		{
			name: "claims in cognito mode are refused",
			in: appSyncAuthInput{
				Enabled: true, Mode: "cognito", CognitoEnabled: true,
				RequiredClaims: map[string][]string{"role": {"admin"}},
			},
			wantErr: true,
			wantIn: []string{
				"pubsub_appsync.required_claims is set",
				"AMAZON_COGNITO_USER_POOLS verifies the signature",
				"silently ignored",
				// and it should say what you do NOT need lambda mode for
				`cognito_app_id_client_regex: "1F4G9H|1J6L4B"`,
			},
		},
		{
			name: "claims in oidc mode are refused",
			in: appSyncAuthInput{
				Enabled: true, Mode: "oidc", OIDCIssuer: "https://idp.example.com",
				RequiredClaims: map[string][]string{"tenant_id": {}},
			},
			wantErr: true,
			wantIn:  []string{"OPENID_CONNECT verifies the signature"},
		},
		{
			name: "an empty claim name is refused",
			in: appSyncAuthInput{
				Enabled: true, Mode: "lambda",
				JWKSURI:        "https://idp.example.com/.well-known/jwks.json",
				RequiredClaims: map[string][]string{"": {"admin"}},
			},
			wantErr: true,
			wantIn:  []string{"empty claim name"},
		},
		{
			name: "an empty accepted value is refused",
			in: appSyncAuthInput{
				Enabled: true, Mode: "lambda",
				JWKSURI:        "https://idp.example.com/.well-known/jwks.json",
				RequiredClaims: map[string][]string{"role": {""}},
			},
			wantErr: true,
			wantIn:  []string{`required_claims["role"] contains an empty accepted value`},
		},
		{
			// sub names one caller. A fixed list of accepted subs is a
			// deployment-wide user allowlist, not authorization.
			name: "pinning sub to fixed values is refused",
			in: appSyncAuthInput{
				Enabled: true, Mode: "lambda",
				JWKSURI:        "https://idp.example.com/.well-known/jwks.json",
				RequiredClaims: map[string][]string{"sub": {"user-abc-123"}},
			},
			wantErr: true,
			wantIn: []string{
				`must not pin "sub"`,
				"resolverContext.sub",
			},
		},
		{
			name: "requiring sub to be present is allowed",
			in: appSyncAuthInput{
				Enabled: true, Mode: "lambda",
				JWKSURI:        "https://idp.example.com/.well-known/jwks.json",
				RequiredClaims: map[string][]string{"sub": {}},
			},
		},
		{
			name: "a disabled appsync ignores the claim policy entirely",
			in: appSyncAuthInput{
				Enabled: false, Mode: "cognito",
				RequiredClaims: map[string][]string{"role": {"admin"}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAppSyncAuthFull(tt.in)
			if tt.wantErr && err == nil {
				t.Fatal("expected an error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("expected no error, got: %v", err)
			}
			for _, want := range tt.wantIn {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error should contain %q, got:\n%v", want, err)
				}
			}
		})
	}
}

// The map path is the one generation actually runs through, and YAML hands it
// shapes the struct path never sees.
func TestValidateAppSyncConfigMap_RequiredClaims(t *testing.T) {
	base := func(claims interface{}) map[string]interface{} {
		return map[string]interface{}{
			"pubsub_appsync": map[string]interface{}{
				"enabled":         true,
				"auth_mode":       "lambda",
				"jwks_uri":        "https://idp.example.com/.well-known/jwks.json",
				"required_claims": claims,
			},
		}
	}

	if err := validateAppSyncConfigMap(base(map[interface{}]interface{}{
		"role":      []interface{}{"admin", "ops"},
		"tenant_id": []interface{}{},
	})); err != nil {
		t.Fatalf("a well-formed claim policy must validate: %v", err)
	}

	// yaml.v2 renders `role:` with no value as nil, which means "must be present".
	if err := validateAppSyncConfigMap(base(map[interface{}]interface{}{"tenant_id": nil})); err != nil {
		t.Fatalf("a presence-only claim must validate: %v", err)
	}

	// A claim policy written as something other than a mapping must be rejected,
	// not silently dropped — a dropped policy is an unenforced one.
	for _, malformed := range []interface{}{"role", []interface{}{"role"}} {
		if err := validateAppSyncConfigMap(base(malformed)); err == nil {
			t.Errorf("required_claims=%#v must be rejected rather than ignored", malformed)
		}
	}

	// And the mode check has to survive the map path too.
	envMap := map[string]interface{}{
		"cognito": map[string]interface{}{"enabled": true},
		"pubsub_appsync": map[string]interface{}{
			"enabled":         true,
			"auth_mode":       "cognito",
			"required_claims": map[interface{}]interface{}{"role": []interface{}{"admin"}},
		},
	}
	if err := validateAppSyncConfigMap(envMap); err == nil {
		t.Error("required_claims in cognito mode must be refused at generate time")
	}
}

// --- template rendering, per mode -------------------------------------------

// appsyncEnvMap builds the template input for one mode.
func appsyncEnvMap(appsync map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"project":        "testproject",
		"env":            "dev",
		"pubsub_appsync": appsync,
	}
}

func TestMainTemplate_CognitoMode(t *testing.T) {
	block := appsyncModuleBlock(t, renderAppSyncTemplate(t, appsyncEnvMap(map[string]interface{}{
		"enabled":                     true,
		"auth_mode":                   "cognito",
		"cognito_app_id_client_regex": "1F4G9H|1J6L4B",
	})))

	if block == "" {
		t.Fatal("appsync module block was not rendered")
	}
	t.Logf("cognito mode renders:\n%s", block)

	for _, want := range []string{
		`auth_mode = "cognito"`,
		`cognito_user_pool_id = module.cognito.user_pool_id`,
		// Without this the API accepts a token minted for any app client on the
		// pool, and meroku's cognito module makes three of them.
		`cognito_app_id_client_regex = "1F4G9H|1J6L4B"`,
		`api_key_enabled = false`,
	} {
		if !strings.Contains(block, want) {
			t.Errorf("generated HCL is missing %s\n\ngot:\n%s", want, block)
		}
	}

	// Passing lambda-mode arguments in cognito mode is not merely redundant:
	// jwks_uri would be an empty string, which the module rejects.
	for _, unwanted := range []string{"jwks_uri", "jwt_issuer", "jwt_audience", "auth_lambda_path", "oidc_issuer"} {
		if strings.Contains(block, unwanted) {
			t.Errorf("%s must not be passed in cognito mode:\n%s", unwanted, block)
		}
	}
}

func TestMainTemplate_OIDCMode(t *testing.T) {
	block := appsyncModuleBlock(t, renderAppSyncTemplate(t, appsyncEnvMap(map[string]interface{}{
		"enabled":        true,
		"auth_mode":      "oidc",
		"oidc_issuer":    "https://idp.example.com",
		"oidc_client_id": "example-client-id",
	})))

	if block == "" {
		t.Fatal("appsync module block was not rendered")
	}
	t.Logf("oidc mode renders:\n%s", block)

	for _, want := range []string{
		`auth_mode = "oidc"`,
		`oidc_issuer = "https://idp.example.com"`,
		`oidc_client_id = "example-client-id"`,
		`api_key_enabled = false`,
	} {
		if !strings.Contains(block, want) {
			t.Errorf("generated HCL is missing %s\n\ngot:\n%s", want, block)
		}
	}

	for _, unwanted := range []string{"jwks_uri", "cognito_user_pool_id", "auth_lambda_path"} {
		if strings.Contains(block, unwanted) {
			t.Errorf("%s must not be passed in oidc mode:\n%s", unwanted, block)
		}
	}
}

// oidc_client_id is AppSync's audience check and is optional. Emitting it empty
// would tell the module "accept any audience" in a way that looks configured.
func TestMainTemplate_OIDCModeOmitsEmptyClientID(t *testing.T) {
	block := appsyncModuleBlock(t, renderAppSyncTemplate(t, appsyncEnvMap(map[string]interface{}{
		"enabled":        true,
		"auth_mode":      "oidc",
		"oidc_issuer":    "https://idp.example.com",
		"oidc_client_id": "",
	})))

	if strings.Contains(block, "oidc_client_id") {
		t.Errorf("an empty oidc_client_id should be omitted:\n%s", block)
	}
}

func TestMainTemplate_LambdaMode(t *testing.T) {
	block := appsyncModuleBlock(t, renderAppSyncTemplate(t, appsyncEnvMap(map[string]interface{}{
		"enabled":      true,
		"auth_mode":    "lambda",
		"auth_lambda":  true,
		"schema":       true,
		"resolvers":    true,
		"jwks_uri":     "https://idp.example.com/.well-known/jwks.json",
		"jwt_issuer":   "https://idp.example.com/",
		"jwt_audience": "test-api",
	})))

	if block == "" {
		t.Fatal("appsync module block was not rendered")
	}
	t.Logf("lambda mode renders:\n%s", block)

	for _, want := range []string{
		`auth_mode = "lambda"`,
		`jwks_uri = "https://idp.example.com/.well-known/jwks.json"`,
		`jwt_issuer = "https://idp.example.com/"`,
		`jwt_audience = "test-api"`,
		`auth_lambda_path = "../../custom/appsync/auth_lambda"`,
		`vtl_templates_yaml = "../../custom/appsync/vtl_templates.yaml"`,
		`api_key_enabled = false`,
	} {
		if !strings.Contains(block, want) {
			t.Errorf("generated HCL is missing %s\n\ngot:\n%s", want, block)
		}
	}

	for _, unwanted := range []string{"cognito_user_pool_id", "oidc_issuer"} {
		if strings.Contains(block, unwanted) {
			t.Errorf("%s must not be passed in lambda mode:\n%s", unwanted, block)
		}
	}
}

// The claim policy has to survive the trip from YAML mapping to HCL
// map(list(string)), sorted so that re-generating produces no diff.
func TestMainTemplate_LambdaModeRendersRequiredClaims(t *testing.T) {
	block := appsyncModuleBlock(t, renderAppSyncTemplate(t, appsyncEnvMap(map[string]interface{}{
		"enabled":   true,
		"auth_mode": "lambda",
		"jwks_uri":  "https://idp.example.com/.well-known/jwks.json",
		"required_claims": map[interface{}]interface{}{
			"role":      []interface{}{"admin", "ops"},
			"tenant_id": []interface{}{},
			"scope":     "graphql", // scalar promoted to a one-element list
		},
	})))

	t.Logf("lambda mode with a claim policy renders:\n%s", block)

	for _, want := range []string{
		`"role" = ["admin", "ops"]`,
		`"tenant_id" = []`,
		`"scope" = ["graphql"]`,
	} {
		if !strings.Contains(block, want) {
			t.Errorf("generated HCL is missing %s\n\ngot:\n%s", want, block)
		}
	}

	// Sorted, because Go map order is randomised and an unstable render would
	// make every `generate` produce a spurious diff.
	roleAt, scopeAt, tenantAt := strings.Index(block, `"role"`), strings.Index(block, `"scope"`), strings.Index(block, `"tenant_id"`)
	if !(roleAt < scopeAt && scopeAt < tenantAt) {
		t.Errorf("claims must render in sorted order for reproducible generation:\n%s", block)
	}
}

// A mode that cannot enforce claims must never be handed any, even if a config
// somehow reaches the template — validation is the gate, this is the backstop.
func TestMainTemplate_RequiredClaimsOnlyInLambdaMode(t *testing.T) {
	for _, mode := range []string{"cognito", "oidc"} {
		block := appsyncModuleBlock(t, renderAppSyncTemplate(t, appsyncEnvMap(map[string]interface{}{
			"enabled":         true,
			"auth_mode":       mode,
			"oidc_issuer":     "https://idp.example.com",
			"required_claims": map[interface{}]interface{}{"role": []interface{}{"admin"}},
		})))

		if strings.Contains(block, "required_claims") {
			t.Errorf("%s mode must not be passed required_claims it cannot enforce:\n%s", mode, block)
		}
	}
}

// An un-migrated file has no auth_mode. It must keep deploying what it deploys
// today: AWS_LAMBDA, no API key beyond whatever its own api_key_enabled says.
func TestMainTemplate_MissingAuthModeRendersLambda(t *testing.T) {
	block := appsyncModuleBlock(t, renderAppSyncTemplate(t, appsyncEnvMap(map[string]interface{}{
		"enabled":  true,
		"jwks_uri": "https://idp.example.com/.well-known/jwks.json",
	})))

	if !strings.Contains(block, `auth_mode = "lambda"`) {
		t.Errorf("an absent auth_mode must render lambda:\n%s", block)
	}
	if !strings.Contains(block, `jwks_uri = "https://idp.example.com/.well-known/jwks.json"`) {
		t.Errorf("lambda-mode arguments must still be passed:\n%s", block)
	}
	if !strings.Contains(block, `api_key_enabled = false`) {
		t.Errorf("an absent api_key_enabled must render false:\n%s", block)
	}
}

// The API key is the reason the authorizer was decorative. It must never appear
// unless the YAML asks for it, and it must appear when it does - a migrated
// environment relies on that to keep its existing key.
func TestMainTemplate_APIKeyIsOptIn(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value interface{}
		want  string
	}{
		{name: "absent", value: nil, want: "api_key_enabled = false"},
		{name: "explicit false", value: false, want: "api_key_enabled = false"},
		{name: "explicit true", value: true, want: "api_key_enabled = true"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			appsync := map[string]interface{}{
				"enabled":   true,
				"auth_mode": "cognito",
			}
			if tc.value != nil {
				appsync["api_key_enabled"] = tc.value
			}

			block := appsyncModuleBlock(t, renderAppSyncTemplate(t, appsyncEnvMap(appsync)))
			if !strings.Contains(block, tc.want) {
				t.Errorf("expected %s, got:\n%s", tc.want, block)
			}
		})
	}
}

// The end-to-end guarantee, one per mode: a config that would render an
// unplannable or unauthenticated module block is refused before any HCL exists.
func TestAppSyncModeMisconfigurationIsCaughtBeforeRendering(t *testing.T) {
	for _, tc := range []struct {
		name   string
		envMap map[string]interface{}
	}{
		{
			name: "cognito without a pool",
			envMap: map[string]interface{}{
				"pubsub_appsync": map[string]interface{}{"enabled": true, "auth_mode": "cognito"},
			},
		},
		{
			name: "oidc without an issuer",
			envMap: map[string]interface{}{
				"pubsub_appsync": map[string]interface{}{"enabled": true, "auth_mode": "oidc"},
			},
		},
		{
			name: "lambda without a jwks_uri",
			envMap: map[string]interface{}{
				"pubsub_appsync": map[string]interface{}{"enabled": true, "auth_mode": "lambda"},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateAppSyncConfigMap(tc.envMap); err == nil {
				t.Fatal("expected generation to be refused")
			}
		})
	}
}
