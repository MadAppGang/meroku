package main

import (
	"encoding/json"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

// REQ-18 / TEST-15.
func TestNeonContractMigrationV32AddsOnlyAbsentDefaults(t *testing.T) {
	for _, tc := range []struct{ name, input, want string }{
		{"AWS-only", `{"schema_version":31,"project":"myapp","env":"dev"}`, `{"schema_version":31,"project":"myapp","env":"dev"}`},
		{"create defaults", `{"neon":{"mode":"create","project":{"name":"fake-project"},"databases":[{"name":"app"}]}}`, `{"neon":{"mode":"create","owner_role":"neondb_owner","project":{"name":"fake-project","pg_version":17},"databases":[{"name":"app","create":false}]}}`},
		{"reuse defaults", `{"neon":{"mode":"reuse","project_id":"fake-project-000000","databases":[{"name":"app"}]}}`, `{"neon":{"mode":"reuse","project_id":"fake-project-000000","owner_role":"neondb_owner","databases":[{"name":"app","create":false}]}}`},
		{"explicit choices", `{"neon":{"mode":"create","owner_role":"custom_owner","project":{"pg_version":16},"databases":[{"name":"app","create":true},{"name":"other","create":false}]}}`, `{"neon":{"mode":"create","owner_role":"custom_owner","project":{"pg_version":16},"databases":[{"name":"app","create":true},{"name":"other","create":false}]}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := qaNeonMap(t, tc.input)
			if err := migrateToV32(m); err != nil {
				t.Fatal(err)
			}
			// JSON normalizes Go/YAML integer representations without changing observable values.
			got := qaNeonMap(t, qaNeonJSON(t, m))
			want := qaNeonMap(t, tc.want)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("migrated=%s want %s", qaNeonJSON(t, m), tc.want)
			}
			once := qaNeonJSON(t, m)
			if err := migrateToV32(m); err != nil {
				t.Fatal(err)
			}
			if qaNeonJSON(t, m) != once {
				t.Fatal("migration is not idempotent")
			}
		})
	}
}

func TestNeonContractForwardSchemaGuardIsActionable(t *testing.T) {
	// REQ-18: loader integration is not available in the declaration contract.
	for _, v := range []int{0, 31, 32} {
		if err := checkSchemaNotNewer("fake-env.yaml", v); err != nil {
			t.Fatalf("supported v%d refused: %v", v, err)
		}
	}
	err := checkSchemaNotNewer("fake-env.yaml", 99)
	if err == nil {
		t.Fatal("future schema accepted")
	}
	for _, s := range []string{"fake-env.yaml", "99", "32", "Update meroku", "not changed"} {
		if !strings.Contains(err.Error(), s) {
			t.Errorf("guard missing %q: %v", s, err)
		}
	}
}

// REQ-4 / TEST-5.
func TestNeonContractRedactsRawAndURLEncodedSecrets(t *testing.T) {
	secret := "fake-redaction-p@ss/word?do-not-use"
	registerSecret(secret)
	input := "public " + secret + " " + url.QueryEscape(secret) + " " + url.PathEscape(secret)
	got := redactSecrets(input)
	for _, s := range []string{secret, url.QueryEscape(secret), url.PathEscape(secret)} {
		if strings.Contains(got, s) {
			t.Error("redaction left a secret representation")
		}
	}
	if !strings.HasPrefix(got, "public ") || !strings.Contains(got, "[REDACTED]") {
		t.Fatalf("wrong redaction shape: %q", got)
	}
}

func TestNeonContractPlanMaskHidesNestedSecretsWithoutMutatingRawChanges(t *testing.T) {
	// REQ-4 / TEST-5: maps and slices are shared by a shallow ResourceChange copy.
	var rc ResourceChange
	raw := `{"address":"module.neon_access_neon_app.postgresql_role.svc[\"backend\"]","change":{
 "actions":["delete","create"],
 "before":{"name":"public-name","password":"fake-before-password","nested":{"private":"fake-before-nested","visible":"before-visible"},"list":[{"secret":"fake-list-secret","visible":"item-visible"}],"optional":null},
 "after":{"name":"public-name","password":"fake-after-password","nested":{"private":"fake-after-nested","visible":"after-visible"},"list":[{"secret":"fake-new-list-secret","visible":"item-visible"}],"optional":null},
 "before_sensitive":{"password":true,"nested":{"private":true},"list":[{"secret":true}],"optional":true},
 "after_sensitive":{"password":true,"nested":{"private":true},"list":[{"secret":true}],"optional":true}
 }}`
	if err := json.Unmarshal([]byte(raw), &rc); err != nil {
		t.Fatal(err)
	}
	untouched := rc
	before := qaNeonJSON(t, untouched)
	maskPlanSensitive(&rc)
	masked := qaNeonJSON(t, rc)
	for _, s := range []string{"fake-before-password", "fake-before-nested", "fake-list-secret", "fake-after-password", "fake-after-nested", "fake-new-list-secret"} {
		if strings.Contains(masked, s) {
			t.Errorf("masked plan contains %q", s)
		}
	}
	for _, s := range []string{"public-name", "before-visible", "after-visible", "item-visible"} {
		if !strings.Contains(masked, s) {
			t.Errorf("public plan value %q was removed", s)
		}
	}
	if qaNeonJSON(t, untouched) != before {
		t.Fatal("mask mutated raw changes shared by another consumer")
	}
	if rc.Change.Before["optional"] != nil || rc.Change.After["optional"] != nil {
		t.Error("null must remain null")
	}
	if !reflect.DeepEqual(rc.Change.BeforeSensitive, untouched.Change.BeforeSensitive) || !reflect.DeepEqual(rc.Change.AfterSensitive, untouched.Change.AfterSensitive) {
		t.Fatal("sensitivity masks changed")
	}
	once := qaNeonJSON(t, rc)
	maskPlanSensitive(&rc)
	if qaNeonJSON(t, rc) != once {
		t.Error("masking is not idempotent")
	}
}

func TestNeonContractWholeObjectSensitiveMaskPreservesNullAndInput(t *testing.T) {
	// REQ-4 / TEST-5.
	obj := map[string]interface{}{"password": "fake-whole-object-password", "list": []interface{}{"fake-list-value"}, "unset": nil}
	before := qaNeonJSON(t, obj)
	got := maskSensitiveObject(obj, true)
	text := qaNeonJSON(t, got)
	if strings.Contains(text, "fake-whole-object-password") || strings.Contains(text, "fake-list-value") {
		t.Fatal("top-level sensitivity leaked values")
	}
	if got["unset"] != nil {
		t.Error("null masked into a value")
	}
	if qaNeonJSON(t, obj) != before {
		t.Error("whole-object mask mutated input")
	}
}
