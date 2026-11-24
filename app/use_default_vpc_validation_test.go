package main

import (
	"strings"
	"testing"
)

func TestValidateUseDefaultVPCMap(t *testing.T) {
	tests := []struct {
		name   string
		envMap map[string]interface{}
		// Empty means the value is accepted; otherwise the value and type the
		// refusal must show.
		wantShown string
	}{
		{name: "true", envMap: map[string]interface{}{"use_default_vpc": true}},
		{name: "false", envMap: map[string]interface{}{"use_default_vpc": false}},
		{name: "absent", envMap: map[string]interface{}{}},
		{name: "null", envMap: map[string]interface{}{"use_default_vpc": nil}},
		{name: `quoted "true"`, envMap: map[string]interface{}{"use_default_vpc": "true"}, wantShown: `"true" (string)`},
		{name: `quoted "false"`, envMap: map[string]interface{}{"use_default_vpc": "false"}, wantShown: `"false" (string)`},
		{name: "empty string", envMap: map[string]interface{}{"use_default_vpc": ""}, wantShown: `"" (string)`},
		{name: "number", envMap: map[string]interface{}{"use_default_vpc": 1}, wantShown: `1 (int)`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateUseDefaultVPCMap(tc.envMap)
			if tc.wantShown == "" {
				if err != nil {
					t.Fatalf("expected no error, got: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected an error, got nil")
			}
			for _, want := range []string{"use_default_vpc", tc.wantShown, "Write true or false without quotes"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error should mention %q, got: %v", want, err)
				}
			}
		})
	}
}

// The type check must run before the checks that read use_default_vpc. The
// placement check would also refuse this map, with a different message.
func TestValidateConfigBeforePreprocess_RefusesNonBooleanUseDefaultVPC(t *testing.T) {
	for _, value := range []string{"true", "false"} {
		t.Run(value, func(t *testing.T) {
			m := minimalEnvMap("public_ip")
			m["postgres"] = map[string]interface{}{"enabled": true, "private_subnets": true}
			m["use_default_vpc"] = value

			err := validateConfigBeforePreprocess("dev", m)
			if err == nil {
				t.Fatal("expected an error, got nil")
			}
			if !strings.Contains(err.Error(), "use_default_vpc must be a boolean") {
				t.Errorf("expected the use_default_vpc type error, got: %v", err)
			}
		})
	}
}
