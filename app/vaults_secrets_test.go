package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v2"
)

// vaultSecretsEnv is a synthetic env with two vaults: the backend connects
// shared without a prefix, the api service connects payments with PAY.
func vaultSecretsEnv(shared, payments interface{}) qaCapabilityMap {
	m := qaCapabilityEnv()
	sharedVault := qaCapabilityMap{"name": "shared"}
	if shared != nil {
		sharedVault["secrets"] = shared
	}
	paymentsVault := qaCapabilityMap{"name": "payments"}
	if payments != nil {
		paymentsVault["secrets"] = payments
	}
	m["vaults"] = qaCapabilityList(sharedVault, paymentsVault)
	qaCapabilityOwner(m, "backend")["vaults"] = qaCapabilityList(qaCapabilityMap{"name": "shared"})
	qaCapabilityOwner(m, "service")["vaults"] = qaCapabilityList(qaCapabilityMap{"name": "payments", "prefix": "PAY"})
	return m
}

func TestVaultSecrets_MissingListIsRefusedWithTheImportHint(t *testing.T) {
	m := vaultSecretsEnv(nil, []interface{}{})
	err := validateVaultsConfigMap(m)
	qaCapabilityRequireError(t, err,
		`vault "shared" has no secrets list: list its secret names under vaults[0].secrets; the Vaults tab can import the names already in SSM`)
	if strings.Contains(err.Error(), "not in the top-level vaults list") {
		t.Errorf("a vault without a list must not also be reported as undeclared: %v", err)
	}
	if strings.Contains(err.Error(), `"payments"`) {
		t.Errorf("an empty list is valid: %v", err)
	}
}

func TestVaultSecrets_NullListIsRefusedLikeAMissingOne(t *testing.T) {
	m := vaultSecretsEnv([]interface{}{}, nil)
	m["vaults"].([]interface{})[1].(qaCapabilityMap)["secrets"] = nil
	qaCapabilityRequireError(t, validateVaultsConfigMap(m), `vault "payments" has no secrets list`)
}

func TestVaultSecrets_EmptyListsAreValidAndRenderEmpty(t *testing.T) {
	m := vaultSecretsEnv([]interface{}{}, []interface{}{})
	qaCapabilityNoError(t, validateVaultsConfigMap(m))
	qaCapabilityEqual(t, m[vaultSecretsKey], map[string]interface{}{
		"shared": []interface{}{}, "payments": []interface{}{},
	})
}

func TestVaultSecrets_ListShapeAndNames(t *testing.T) {
	cases := []struct {
		name     string
		secrets  interface{}
		fragment string
	}{
		{"not a list", "database_url", "vaults[0].secrets must be a list of secret names"},
		{"a mapping", qaCapabilityMap{"a": "b"}, "vaults[0].secrets must be a list of secret names"},
		{"upper case", []interface{}{"Database_URL"}, `vaults[0].secrets[0] "Database_URL": use 1 to 63 lower-case letters`},
		{"digit first", []interface{}{"1token"}, `vaults[0].secrets[0] "1token"`},
		{"underscore first", []interface{}{"_token"}, `vaults[0].secrets[0] "_token"`},
		{"hyphen", []interface{}{"api-key"}, `vaults[0].secrets[0] "api-key"`},
		{"slash", []interface{}{"a/b"}, `vaults[0].secrets[0] "a/b"`},
		{"too long", []interface{}{"a" + strings.Repeat("b", 63)}, "vaults[0].secrets[0]"},
		{"not a string", []interface{}{42}, "vaults[0].secrets[0] 42"},
		{"duplicate", []interface{}{"token", "api_key", "token"}, `vaults[0].secrets[2] "token" is already listed at index 0`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := vaultSecretsEnv(tc.secrets, []interface{}{})
			qaCapabilityRequireError(t, validateVaultsConfigMap(m), tc.fragment)
		})
	}

	t.Run("longest and shortest valid names", func(t *testing.T) {
		m := vaultSecretsEnv([]interface{}{"a", "a" + strings.Repeat("b", 62), "key_2"}, []interface{}{})
		qaCapabilityNoError(t, validateVaultsConfigMap(m))
	})

	t.Run("a []string list from Go is read too", func(t *testing.T) {
		m := vaultSecretsEnv([]string{"api_key"}, []string{})
		qaCapabilityNoError(t, validateVaultsConfigMap(m))
	})
}

func TestVaultSecrets_EnvNeedsAPrefix(t *testing.T) {
	m := vaultSecretsEnv([]interface{}{"env"}, []interface{}{})
	qaCapabilityRequireError(t, validateVaultsConfigMap(m),
		`workload.vaults[0] connects vault "shared" without a prefix, and its secret env would be the variable ENV`)

	// payments is connected only with PAY, so env there is PAY_ENV.
	m = vaultSecretsEnv([]interface{}{}, []interface{}{"env"})
	qaCapabilityNoError(t, validateVaultsConfigMap(m))
}

func TestVaultSecrets_TwoVaultsGivingOneNameAreRefused(t *testing.T) {
	m := vaultSecretsEnv([]interface{}{"database_url"}, []interface{}{"database_url"})
	qaCapabilityOwner(m, "service")["vaults"] = qaCapabilityList(
		qaCapabilityMap{"name": "shared"},
		qaCapabilityMap{"name": "payments"},
	)
	qaCapabilityRequireError(t, validateVaultsConfigMap(m),
		`Workload "api" would get DATABASE_URL from vault "shared"`, `vault "payments"`)

	// Different prefixes make the names differ.
	qaCapabilityOwner(m, "service")["vaults"] = qaCapabilityList(
		qaCapabilityMap{"name": "shared"},
		qaCapabilityMap{"name": "payments", "prefix": "PAY"},
	)
	qaCapabilityNoError(t, validateVaultsConfigMap(m))
}

func TestVaultSecrets_YAMLVariableOfTheSameNameIsRefused(t *testing.T) {
	for _, kind := range qaCapabilityKinds() {
		t.Run(kind, func(t *testing.T) {
			m := vaultSecretsEnv([]interface{}{"api_key"}, []interface{}{})
			qaCapabilityOwner(m, kind)["vaults"] = qaCapabilityList(qaCapabilityMap{"name": "shared"})
			qaCapabilityOwner(m, kind)[qaCapabilityEnvField(kind)] = qaCapabilityMap{"API_KEY": "plain"}
			qaCapabilityRequireError(t, validateVaultsConfigMap(m), "secret api_key", "API_KEY", qaCapabilityEnvField(kind))
		})
	}

	// The lower-case variable is another name for ECS.
	m := vaultSecretsEnv([]interface{}{"api_key"}, []interface{}{})
	qaCapabilityOwner(m, "backend")["backend_env_variables"] = qaCapabilityMap{"api_key": "plain"}
	qaCapabilityNoError(t, validateVaultsConfigMap(m))
}

func TestVaultSecrets_NameMerokuSetsIsRefused(t *testing.T) {
	m := vaultSecretsEnv([]interface{}{"data_bucket_name"}, []interface{}{})
	qaCapabilityRequireError(t, validateVaultsConfigMap(m), "DATA_BUCKET_NAME", "which meroku sets for this workload")

	m = vaultSecretsEnv([]interface{}{"data_bucket_shared_x"}, []interface{}{})
	qaCapabilityRequireError(t, validateVaultsConfigMap(m), "DATA_BUCKET_SHARED_X", "DATA_BUCKET_SHARED_*")

	// AWS_S3_BUCKET is meroku's on the backend only.
	m = vaultSecretsEnv([]interface{}{}, []interface{}{"bucket"})
	qaCapabilityOwner(m, "service")["vaults"] = qaCapabilityList(qaCapabilityMap{"name": "payments", "prefix": "AWS_S3"})
	qaCapabilityNoError(t, validateVaultsConfigMap(m))
	qaCapabilityOwner(m, "backend")["vaults"] = qaCapabilityList(qaCapabilityMap{"name": "payments", "prefix": "AWS_S3"})
	qaCapabilityRequireError(t, validateVaultsConfigMap(m), "AWS_S3_BUCKET")
}

func TestVaultSecrets_RenderedAsTheModuleMap(t *testing.T) {
	m := vaultSecretsEnv([]interface{}{"database_url", "api_key"}, []interface{}{})
	rendered := renderMainHBS(t, m)
	body := qaCapabilityParse(t, rendered)
	vaults := qaCapabilityModule(t, body, "vaults")
	qaCapabilityWantJSON(t, qaCapabilityLiteral(t, vaults, "vaults"),
		`{"payments": [], "shared": ["database_url", "api_key"]}`)
}

func TestVaultSecrets_NoVaultsRendersAnEmptyMap(t *testing.T) {
	rendered := renderMainHBS(t, qaCapabilityEnv())
	body := qaCapabilityParse(t, rendered)
	qaCapabilityWantJSON(t, qaCapabilityLiteral(t, qaCapabilityModule(t, body, "vaults"), "vaults"), `{}`)
}

// A struct save keeps a missing list missing and an empty list empty: the first
// is what generate refuses, the second is a valid vault with nothing yet.
func TestVaultSecrets_StructSaveKeepsMissingAndEmptyApart(t *testing.T) {
	raw := []byte("project: myapp\nenv: dev\nvaults:\n  - name: legacy\n  - name: fresh\n    secrets: []\n  - name: full\n    secrets: [database_url, api_key]\n")
	var env Env
	qaCapabilityNoError(t, yaml.Unmarshal(raw, &env))
	if env.Vaults[0].Secrets != nil {
		t.Errorf("legacy: secrets %#v, want nil", env.Vaults[0].Secrets)
	}
	if env.Vaults[1].Secrets == nil || len(env.Vaults[1].Secrets) != 0 {
		t.Errorf("fresh: secrets %#v, want an empty non-nil list", env.Vaults[1].Secrets)
	}

	filename := filepath.Join(t.TempDir(), "dev.yaml")
	qaCapabilityNoError(t, saveEnvToFile(env, filename))
	saved, err := os.ReadFile(filename)
	qaCapabilityNoError(t, err)
	var doc map[string]interface{}
	qaCapabilityNoError(t, yaml.Unmarshal(saved, &doc))
	vaults := doc["vaults"].([]interface{})
	legacy := vaults[0].(map[interface{}]interface{})
	if _, has := legacy["secrets"]; has {
		t.Errorf("save added a secrets key to a vault without one: %s", saved)
	}
	fresh := vaults[1].(map[interface{}]interface{})
	if list, ok := fresh["secrets"].([]interface{}); !ok || len(list) != 0 {
		t.Errorf("save lost the empty secrets list: %s", saved)
	}
	full := vaults[2].(map[interface{}]interface{})
	qaCapabilityEqual(t, full["secrets"], []interface{}{"database_url", "api_key"})
}
