package main

import (
	"strings"
	"testing"
)

// The validators read YAML two ways: the generate path converts every mapping
// to map[string]interface{}, while a plain yaml.v2 Unmarshal leaves
// map[interface{}]interface{}. Each check below must behave the same for both,
// and a value of the wrong type must be refused, not read as its zero value.

func v2Map(kv ...interface{}) map[interface{}]interface{} {
	m := map[interface{}]interface{}{}
	for i := 0; i+1 < len(kv); i += 2 {
		m[kv[i]] = kv[i+1]
	}
	return m
}

func TestWorkloadPolicy_YAMLv2MapsAreValidated(t *testing.T) {
	badStatement := v2Map("actions", []interface{}{}, "resources", []interface{}{"*"})

	tests := []struct {
		name   string
		envMap map[string]interface{}
		want   string
	}{
		{
			name:   "backend",
			envMap: map[string]interface{}{"workload": v2Map("policy", []interface{}{badStatement})},
			want:   "workload: policy[0].actions must be a non-empty list",
		},
		{
			name: "service",
			envMap: map[string]interface{}{"services": []interface{}{
				v2Map("name", "api", "policy", []interface{}{badStatement}),
			}},
			want: "services 'api': policy[0].actions must be a non-empty list",
		},
		{
			name: "condition",
			envMap: map[string]interface{}{"scheduled_tasks": []interface{}{
				v2Map("name", "nightly", "policy", []interface{}{v2Map(
					"actions", []interface{}{"s3:GetObject"},
					"resources", []interface{}{"*"},
					"conditions", []interface{}{v2Map("test", "StringEquals")},
				)}),
			}},
			want: "scheduled_tasks 'nightly': policy[0].conditions[0] is incomplete: variable, values",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateWorkloadPolicyConfigMap(tc.envMap)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want an error containing %q, got %v", tc.want, err)
			}
		})
	}
}

// A service's policy holds ${aws:...} values that would be interpolations in
// the {{{array services}}} render, so it must leave the service map whichever
// map type that is.
func TestMoveServicePoliciesOut_YAMLv2Service(t *testing.T) {
	statements := []interface{}{v2Map(
		"actions", []interface{}{"s3:GetObject"},
		"resources", []interface{}{"arn:aws:s3:::example/${aws:username}/*"},
	)}
	service := v2Map("name", "api", "policy", statements)
	envMap := map[string]interface{}{"services": []interface{}{service}}

	moveServicePoliciesOut(envMap)

	if _, still := service["policy"]; still {
		t.Error("policy is still in the yaml.v2 service map")
	}
	policies, _ := envMap[servicePoliciesKey].(map[string]interface{})
	if _, ok := policies["api"]; !ok {
		t.Errorf("_service_policies has no entry for api: %#v", policies)
	}
}

// Warnings go through addGenerateWarning, like every other validator's, so
// applyTemplate prints them once and render tests can read them.
func TestWorkloadPolicy_WildcardActionIsAGenerateWarning(t *testing.T) {
	envMap := map[string]interface{}{"workload": map[string]interface{}{"policy": []interface{}{
		map[string]interface{}{"actions": []interface{}{"*"}, "resources": []interface{}{"*"}},
	}}}
	if err := validateWorkloadPolicyConfigMap(envMap); err != nil {
		t.Fatalf("validateWorkloadPolicyConfigMap: %v", err)
	}
	warnings := generateWarnings(envMap)
	if len(warnings) != 1 || !strings.Contains(warnings[0], `workload: policy[0].actions contains "*"`) {
		t.Errorf("want one generate warning about the * action, got %v", warnings)
	}
}

func TestFilterDisabledItems_YAMLv2Items(t *testing.T) {
	envMap := map[string]interface{}{"services": []interface{}{
		v2Map("name", "on"),
		v2Map("name", "off", "enabled", false),
		v2Map("name", "explicit", "enabled", true),
	}}
	filterDisabledItems(envMap, "services")

	var names []string
	for _, raw := range envMap["services"].([]interface{}) {
		item, _ := computeNodeMap(raw)
		names = append(names, item["name"].(string))
	}
	if got := strings.Join(names, ","); got != "on,explicit" {
		t.Errorf("kept %s, want on,explicit", got)
	}
}

func TestValkey_EnabledMustBeABoolean(t *testing.T) {
	for _, raw := range []interface{}{"true", "yes", 1} {
		envMap := map[string]interface{}{"valkey": map[string]interface{}{"enabled": raw}}
		err := validateValkeyConfigMap(envMap)
		if err == nil || !strings.Contains(err.Error(), "valkey.enabled is") {
			t.Errorf("enabled: %#v: want a type error, got %v", raw, err)
		}
	}

	for _, raw := range []interface{}{nil, false} {
		envMap := map[string]interface{}{"valkey": map[string]interface{}{"enabled": raw}}
		if err := validateValkeyConfigMap(envMap); err != nil {
			t.Errorf("enabled: %#v: want off without error, got %v", raw, err)
		}
		if _, on := envMap[valkeyKey]; on {
			t.Errorf("enabled: %#v: valkey must stay off", raw)
		}
	}
}

func TestVaults_AccessMustBeAString(t *testing.T) {
	for _, raw := range []interface{}{1, true, []interface{}{"read"}} {
		envMap := map[string]interface{}{
			"vaults":   []interface{}{map[string]interface{}{"name": "shared", "secrets": []interface{}{"api_key"}}},
			"workload": map[string]interface{}{"vaults": []interface{}{map[string]interface{}{"name": "shared", "access": raw}}},
		}
		err := validateVaultsConfigMap(envMap)
		if err == nil || !strings.Contains(err.Error(), "use read or read_write") {
			t.Errorf("access: %#v: want an error, got %v", raw, err)
		}
	}

	envMap := map[string]interface{}{
		"vaults":   []interface{}{map[string]interface{}{"name": "shared", "secrets": []interface{}{"api_key"}}},
		"workload": map[string]interface{}{"vaults": []interface{}{map[string]interface{}{"name": "shared", "access": ""}}},
	}
	if err := validateVaultsConfigMap(envMap); err != nil {
		t.Errorf("an empty access is the default read, got %v", err)
	}
}
