package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// kmsTestEnv is the synthetic env with two keys, one retired.
func kmsTestEnv() qaCapabilityMap {
	m := qaCapabilityEnv()
	m["kms_keys"] = qaCapabilityList(
		qaCapabilityMap{"name": "auth"},
		qaCapabilityMap{"name": "webhooks", "key_spec": "RSA_2048"},
		qaCapabilityMap{"name": "legacy", "key_spec": "ECC_NIST_P384", "retired": true},
	)
	return m
}

func TestKMSKeysNormalisesKeysAndConnectionsForEveryKind(t *testing.T) {
	m := kmsTestEnv()
	accesses := map[string]string{"backend": "both", "service": "sign", "scheduled_task": "verify", "event_task": "sign"}
	for _, kind := range qaCapabilityKinds() {
		qaCapabilityOwner(m, kind)["kms_keys"] = qaCapabilityList(qaCapabilityMap{"name": "auth", "access": accesses[kind]})
	}
	qaCapabilityOwner(m, "service")["kms_keys"] = qaCapabilityList(
		qaCapabilityMap{"name": "webhooks", "access": "verify"},
		qaCapabilityMap{"name": "auth", "access": "sign"},
	)

	qaCapabilityNoError(t, preprocessEnvMap(m))

	qaCapabilityEqual(t, m[kmsKeysKey], map[string]interface{}{
		"auth":     map[string]interface{}{"key_spec": "ECC_NIST_P256"},
		"webhooks": map[string]interface{}{"key_spec": "RSA_2048"},
		// Retired keys stay deployed.
		"legacy": map[string]interface{}{"key_spec": "ECC_NIST_P384"},
	})
	qaCapabilityEqual(t, m[kmsConnectionsKey], []interface{}{
		map[string]interface{}{"workload": "backend", "key": "auth", "access": "both"},
		map[string]interface{}{"workload": "api", "key": "webhooks", "access": "verify"},
		map[string]interface{}{"workload": "api", "key": "auth", "access": "sign"},
		map[string]interface{}{"workload": "nightly", "key": "auth", "access": "verify"},
		map[string]interface{}{"workload": "mailer", "key": "auth", "access": "sign"},
	})
	if _, still := qaCapabilityOwner(m, "service")["kms_keys"]; still {
		t.Error("the service map still carries kms_keys after normalising")
	}
}

func TestKMSKeysAbsentBlockRendersNothing(t *testing.T) {
	m := qaCapabilityEnv()
	qaCapabilityNoError(t, preprocessEnvMap(m))
	qaCapabilityEqual(t, m[kmsKeysKey], map[string]interface{}{})
	qaCapabilityEqual(t, m[kmsConnectionsKey], []interface{}{})
}

func TestKMSKeysRefusals(t *testing.T) {
	tests := []struct {
		name      string
		keys      []interface{}
		backend   []interface{}
		fragments []string
	}{
		{
			name:      "bad name",
			keys:      qaCapabilityList(qaCapabilityMap{"name": "Auth-Key"}),
			fragments: []string{"kms_keys[0]", `"Auth-Key"`, "lower-case"},
		},
		{
			name:      "name starting with a digit",
			keys:      qaCapabilityList(qaCapabilityMap{"name": "1auth"}),
			fragments: []string{"kms_keys[0]", `"1auth"`},
		},
		{
			name:      "name too long",
			keys:      qaCapabilityList(qaCapabilityMap{"name": strings.Repeat("a", 32)}),
			fragments: []string{"kms_keys[0]", "1 to 31"},
		},
		{
			name:      "missing name",
			keys:      qaCapabilityList(qaCapabilityMap{"key_spec": "RSA_2048"}),
			fragments: []string{"kms_keys[0] has no name"},
		},
		{
			name:      "duplicate name",
			keys:      qaCapabilityList(qaCapabilityMap{"name": "auth"}, qaCapabilityMap{"name": "auth"}),
			fragments: []string{"kms_keys[1]", `"auth"`, "already used by kms_keys[0]"},
		},
		{
			name:      "unknown key_spec",
			keys:      qaCapabilityList(qaCapabilityMap{"name": "auth", "key_spec": "SYMMETRIC_DEFAULT"}),
			fragments: []string{"kms_keys[0] key_spec", "SYMMETRIC_DEFAULT", "ECC_NIST_P256", "RSA_4096", "HMAC_224", "HMAC_512"},
		},
		{
			name:      "retired not a boolean",
			keys:      qaCapabilityList(qaCapabilityMap{"name": "auth", "retired": "yes"}),
			fragments: []string{"kms_keys[0] retired", "true or false"},
		},
		{
			name:      "connection to an unknown key",
			keys:      qaCapabilityList(qaCapabilityMap{"name": "auth"}),
			backend:   qaCapabilityList(qaCapabilityMap{"name": "missing", "access": "sign"}),
			fragments: []string{"workload.kms_keys[0]", `"missing"`, "add it under kms_keys first"},
		},
		{
			name:      "connection to a retired key",
			keys:      qaCapabilityList(qaCapabilityMap{"name": "auth", "retired": true}),
			backend:   qaCapabilityList(qaCapabilityMap{"name": "auth", "access": "sign"}),
			fragments: []string{`kms key "auth" is retired: remove the connection, or set retired: false`},
		},
		{
			name:      "bad access",
			keys:      qaCapabilityList(qaCapabilityMap{"name": "auth"}),
			backend:   qaCapabilityList(qaCapabilityMap{"name": "auth", "access": "encrypt"}),
			fragments: []string{"workload.kms_keys[0] access", "encrypt", "sign, verify or both"},
		},
		{
			name:      "missing access",
			keys:      qaCapabilityList(qaCapabilityMap{"name": "auth"}),
			backend:   qaCapabilityList(qaCapabilityMap{"name": "auth"}),
			fragments: []string{"workload.kms_keys[0] access", "sign, verify or both"},
		},
		{
			name: "same key twice on one workload",
			keys: qaCapabilityList(qaCapabilityMap{"name": "auth"}),
			backend: qaCapabilityList(
				qaCapabilityMap{"name": "auth", "access": "sign"},
				qaCapabilityMap{"name": "auth", "access": "verify"},
			),
			fragments: []string{"workload.kms_keys[1]", "already connected at index 0", "both"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := qaCapabilityEnv()
			m["kms_keys"] = tt.keys
			if tt.backend != nil {
				qaCapabilityOwner(m, "backend")["kms_keys"] = tt.backend
			}
			qaCapabilityRequireError(t, preprocessEnvMap(m), tt.fragments...)
		})
	}
}

func TestKMSKeysConnectionFieldNamesTheWorkload(t *testing.T) {
	m := qaCapabilityEnv()
	m["kms_keys"] = qaCapabilityList(qaCapabilityMap{"name": "auth"})
	qaCapabilityOwner(m, "scheduled_task")["kms_keys"] = qaCapabilityList(qaCapabilityMap{"name": "nope", "access": "sign"})
	qaCapabilityRequireError(t, preprocessEnvMap(m), "scheduled_tasks[0].kms_keys ('nightly')[0]", `"nope"`)
}

func TestKMSKeysReservedEnvNames(t *testing.T) {
	for _, kind := range qaCapabilityKinds() {
		for _, variable := range []string{"KMS_AUTH_KEY_ARN", "KMS_AUTH_PUBLIC_KEY"} {
			t.Run(kind+"/"+variable, func(t *testing.T) {
				m := kmsTestEnv()
				owner := qaCapabilityOwner(m, kind)
				owner["kms_keys"] = qaCapabilityList(qaCapabilityMap{"name": "auth", "access": "sign"})
				owner[qaCapabilityEnvField(kind)] = qaCapabilityMap{variable: "mine"}
				qaCapabilityRequireError(t, preprocessEnvMap(m), qaCapabilityEnvField(kind), variable, `kms key "auth"`)
			})
		}
	}

	t.Run("an unconnected workload may set the name", func(t *testing.T) {
		m := kmsTestEnv()
		qaCapabilityOwner(m, "service")["kms_keys"] = qaCapabilityList(qaCapabilityMap{"name": "auth", "access": "sign"})
		qaCapabilityOwner(m, "backend")["backend_env_variables"] = qaCapabilityMap{"KMS_AUTH_KEY_ARN": "mine"}
		qaCapabilityNoError(t, preprocessEnvMap(m))
	})

	t.Run("a vault secret with the name is refused", func(t *testing.T) {
		m := kmsTestEnv()
		m["vaults"] = qaCapabilityList(qaCapabilityMap{"name": "shared", "secrets": []interface{}{"kms_auth_public_key"}})
		service := qaCapabilityOwner(m, "service")
		service["kms_keys"] = qaCapabilityList(qaCapabilityMap{"name": "auth", "access": "verify"})
		service["vaults"] = qaCapabilityList(qaCapabilityMap{"name": "shared"})
		qaCapabilityRequireError(t, preprocessEnvMap(m), "KMS_AUTH_PUBLIC_KEY", `kms key "auth"`)
	})
}

func TestKMSKeysAcceptsEveryHMACSpec(t *testing.T) {
	for _, spec := range []string{"HMAC_224", "HMAC_256", "HMAC_384", "HMAC_512"} {
		t.Run(spec, func(t *testing.T) {
			m := qaCapabilityEnv()
			m["kms_keys"] = qaCapabilityList(qaCapabilityMap{"name": "requests", "key_spec": spec})
			for kind, access := range map[string]string{"backend": "sign", "service": "verify", "scheduled_task": "both"} {
				qaCapabilityOwner(m, kind)["kms_keys"] = qaCapabilityList(qaCapabilityMap{"name": "requests", "access": access})
			}
			qaCapabilityNoError(t, preprocessEnvMap(m))
			qaCapabilityEqual(t, m[kmsKeysKey], map[string]interface{}{
				"requests": map[string]interface{}{"key_spec": spec},
			})
		})
	}
}

// kmsReservedNames is the set of KMS variable names reserved for one workload.
func kmsReservedNames(envMap map[string]interface{}, workload string) map[string]bool {
	names := map[string]bool{}
	for _, row := range kmsReservedWorkloadEnv(envMap) {
		if row.appliesTo(workloadEnvContext{envMap: envMap, name: workload}) {
			names[row.name] = true
		}
	}
	return names
}

func TestKMSKeysReservedRowsForHMACKeys(t *testing.T) {
	m := kmsTestEnv()
	m["kms_keys"] = append(m["kms_keys"].([]interface{}), qaCapabilityMap{"name": "requests", "key_spec": "HMAC_256"})
	qaCapabilityOwner(m, "service")["kms_keys"] = qaCapabilityList(
		qaCapabilityMap{"name": "auth", "access": "sign"},
		qaCapabilityMap{"name": "requests", "access": "both"},
	)
	qaCapabilityNoError(t, preprocessEnvMap(m))

	qaCapabilityEqual(t, kmsReservedNames(m, "api"), map[string]bool{
		"KMS_AUTH_KEY_ARN":     true,
		"KMS_AUTH_PUBLIC_KEY":  true,
		"KMS_REQUESTS_KEY_ARN": true,
	})
}

func TestKMSKeysHMACKeyReservesOnlyTheARN(t *testing.T) {
	for _, kind := range qaCapabilityKinds() {
		t.Run(kind, func(t *testing.T) {
			m := qaCapabilityEnv()
			m["kms_keys"] = qaCapabilityList(qaCapabilityMap{"name": "x", "key_spec": "HMAC_512"})
			owner := qaCapabilityOwner(m, kind)
			owner["kms_keys"] = qaCapabilityList(qaCapabilityMap{"name": "x", "access": "verify"})
			owner[qaCapabilityEnvField(kind)] = qaCapabilityMap{"KMS_X_PUBLIC_KEY": "mine"}
			qaCapabilityNoError(t, preprocessEnvMap(m))
		})
	}

	t.Run("the ARN variable is still refused", func(t *testing.T) {
		m := qaCapabilityEnv()
		m["kms_keys"] = qaCapabilityList(qaCapabilityMap{"name": "x", "key_spec": "HMAC_256"})
		backend := qaCapabilityOwner(m, "backend")
		backend["kms_keys"] = qaCapabilityList(qaCapabilityMap{"name": "x", "access": "sign"})
		backend[qaCapabilityEnvField("backend")] = qaCapabilityMap{"KMS_X_KEY_ARN": "mine"}
		qaCapabilityRequireError(t, preprocessEnvMap(m), "KMS_X_KEY_ARN", `kms key "x"`)
	})
}

func TestKMSKeysRenderModuleAndExtraEnv(t *testing.T) {
	m := kmsTestEnv()
	qaCapabilityOwner(m, "service")["kms_keys"] = qaCapabilityList(qaCapabilityMap{"name": "auth", "access": "both"})
	rendered := renderMainHBS(t, m)
	body := qaCapabilityParse(t, rendered)
	module := qaCapabilityModule(t, body, "kms_keys")

	qaCapabilityWantJSON(t, qaCapabilityLiteral(t, module, "keys"),
		`{"auth":{"key_spec":"ECC_NIST_P256"},"legacy":{"key_spec":"ECC_NIST_P384"},"webhooks":{"key_spec":"RSA_2048"}}`)
	qaCapabilityWantJSON(t, qaCapabilityLiteral(t, module, "connections"),
		`[{"workload":"api","key":"auth","access":"both"}]`)
	qaCapabilityContains(t, qaCapabilityExprText(qaCapabilityAttr(t, module, "workloads").Expr, rendered), "local.workload_roles")

	var extraEnv string
	for _, block := range body.Blocks {
		if block.Type == "locals" && block.Body.Attributes["workload_extra_env"] != nil {
			extraEnv = qaCapabilityExprText(block.Body.Attributes["workload_extra_env"].Expr, rendered)
		}
	}
	qaCapabilityContains(t, extraEnv, "module.kms_keys.environment[name]")

	serviceExpr := qaCapabilityAttr(t, qaCapabilityModule(t, body, "workloads"), "services").Expr
	if strings.Contains(qaCapabilityExprText(serviceExpr, rendered), "kms_keys") {
		t.Error("service payload still contains kms key connection data")
	}
}

func TestKMSKeysRenderWithoutKeys(t *testing.T) {
	body := qaCapabilityParse(t, renderMainHBS(t, qaCapabilityEnv()))
	module := qaCapabilityModule(t, body, "kms_keys")
	qaCapabilityWantJSON(t, qaCapabilityLiteral(t, module, "keys"), `{}`)
	qaCapabilityWantJSON(t, qaCapabilityLiteral(t, module, "connections"), `[]`)
}

func TestKMSKeysBridge(t *testing.T) {
	for _, withKeys := range []bool{false, true} {
		t.Run(map[bool]string{false: "no keys", true: "keys"}[withKeys], func(t *testing.T) {
			t.Chdir(t.TempDir())
			m := qaCapabilityEnv()
			if withKeys {
				m = kmsTestEnv()
			}
			qaCapabilityNoError(t, preprocessEnvMap(m))
			if err := os.MkdirAll(filepath.Join("env", "dev"), 0755); err != nil {
				t.Fatal(err)
			}
			generateBridgeFile("dev", m)
			raw, err := os.ReadFile(filepath.Join("env", "dev", "_bridge.tf"))
			qaCapabilityNoError(t, err)
			rendered := string(raw)
			qaCapabilityParse(t, rendered)
			if withKeys {
				qaCapabilityContains(t, rendered, "kms_key_arns = module.kms_keys.key_arns")
			} else {
				qaCapabilityContains(t, rendered, "kms_key_arns = {}")
			}
		})
	}
}

func TestKMSKeysRoundTripKeepsAbsentKeysAbsent(t *testing.T) {
	raw := []byte("project: myapp\nenv: dev\nregion: us-east-1\naccount_id: \"000000000000\"\nworkload: {}\nservices:\n  - name: api\nscheduled_tasks:\n  - name: nightly\nevent_processor_tasks:\n  - name: mailer\n")
	got := qaCapabilitySaveYAML(t, raw).(map[interface{}]interface{})
	if _, exists := got["kms_keys"]; exists {
		t.Error("save invented kms_keys")
	}
	if _, exists := got["workload"].(map[interface{}]interface{})["kms_keys"]; exists {
		t.Error("save invented workload.kms_keys")
	}
	for _, kind := range []string{"services", "scheduled_tasks", "event_processor_tasks"} {
		owner := got[kind].([]interface{})[0].(map[interface{}]interface{})
		if _, exists := owner["kms_keys"]; exists {
			t.Errorf("save invented %s[0].kms_keys", kind)
		}
	}
}
