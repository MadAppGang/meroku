package main

import (
	"strings"
	"testing"
)

func networkAccessEnv(services ...map[string]interface{}) map[string]interface{} {
	list := make([]interface{}, len(services))
	for i, s := range services {
		list[i] = s
	}
	return map[string]interface{}{"services": list}
}

func TestValidateNetworkAccess_AcceptsEveryValueAndAbsence(t *testing.T) {
	env := networkAccessEnv(
		map[string]interface{}{"name": "a", "network_access": "public"},
		map[string]interface{}{"name": "b", "network_access": "internal", "health_check_command": []interface{}{"CMD", "true"}},
		map[string]interface{}{"name": "c", "network_access": "full_open"},
		map[string]interface{}{"name": "d", "network_access": "none"},
		map[string]interface{}{"name": "e"},
	)
	if err := validateNetworkAccessConfigMap(env); err != nil {
		t.Fatalf("valid config refused: %v", err)
	}
}

// "private" is the likeliest typo: it is what other platforms call internal.
// The error must name the service, so the user knows which entry to fix.
func TestValidateNetworkAccess_RefusesAnUnknownValueNamingTheService(t *testing.T) {
	err := validateNetworkAccessConfigMap(networkAccessEnv(
		map[string]interface{}{"name": "bridge", "network_access": "private"},
	))
	if err == nil {
		t.Fatal("network_access: private must be refused")
	}
	for _, want := range []string{"bridge", "private", "public, internal, full_open, none"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

// A missing health check is a warning, not a refusal: workers often have no
// HTTP endpoint, and refusing would block a valid configuration.
func TestValidateNetworkAccess_MissingHealthCheckOnlyWarns(t *testing.T) {
	err := validateNetworkAccessConfigMap(networkAccessEnv(
		map[string]interface{}{"name": "worker", "network_access": "none"},
		map[string]interface{}{"name": "bridge", "network_access": "internal"},
	))
	if err != nil {
		t.Fatalf("a missing health check must warn, not fail: %v", err)
	}
}
