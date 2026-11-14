package main

import (
	"strings"
	"testing"
)

// Both ingresses route /<service>/... to a routed service ahead of the
// backend's catch-all, so a backend rule whose target path starts with such a
// service's name reaches that service, not the backend.
func TestAmplifyRules_BackendPathShadowedByServiceRoute(t *testing.T) {
	envWith := func(rule map[string]interface{}, services ...interface{}) map[string]interface{} {
		envMap := amplifyTestEnv([]interface{}{amplifyApp(
			amplifyRule("source", "/<*>", "target", "/index.html", "status", "404-200"),
			rule,
		)})
		if services != nil {
			envMap["services"] = services
		}
		return envMap
	}
	svc := func(name, access string) map[string]interface{} {
		return map[string]interface{}{"name": name, "network_access": access}
	}
	albOn := func(envMap map[string]interface{}) map[string]interface{} {
		envMap["alb"] = map[string]interface{}{"enabled": true}
		return envMap
	}

	refused := []struct {
		name    string
		envMap  map[string]interface{}
		service string
	}{
		{"source under a public service", envWith(amplifyRule("source", "/api/<*>", "workload", "backend")), "api"},
		{"source equal to a public service", envWith(amplifyRule("source", "/api", "workload", "backend")), "api"},
		{"wildcard right after the service name", envWith(amplifyRule("source", "/api<*>", "workload", "backend")), "api"},
		{"explicit path under a public service", envWith(amplifyRule("source", "/v1/<*>", "workload", "backend", "path", "/api/<*>")), "api"},
		{"full_open service", envWith(amplifyRule("source", "/files/<*>", "workload", "backend"), svc("files", NetworkAccessFullOpen)), "files"},
		{"ALB mode", albOn(envWith(amplifyRule("source", "/api/<*>", "workload", "backend"))), "api"},
	}
	for _, tc := range refused {
		t.Run(tc.name, func(t *testing.T) {
			err := preprocessEnvMap(tc.envMap)
			if err == nil {
				t.Fatal("want an error, got nil")
			}
			for _, want := range []string{
				"custom_rules[1]",
				"reaches service '" + tc.service + "'",
				"target workload " + tc.service + " instead, or set path to one that does not start with /" + tc.service,
			} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error does not contain %q:\n%v", want, err)
				}
			}
		})
	}

	allowed := []struct {
		name   string
		envMap map[string]interface{}
	}{
		{"explicit path that avoids the service", envWith(amplifyRule("source", "/api/<*>", "workload", "backend", "path", "/v1/<*>"))},
		{"a longer first segment", envWith(amplifyRule("source", "/apis/<*>", "workload", "backend"))},
		{"internal service of that name gets no route", envWith(amplifyRule("source", "/api/<*>", "workload", "backend"), svc("api", NetworkAccessInternal))},
		{"service of that name with no inbound access", envWith(amplifyRule("source", "/api/<*>", "workload", "backend"), svc("api", NetworkAccessNone))},
		{"rule that targets the service itself", envWith(amplifyRule("source", "/api/<*>", "workload", "api"))},
	}
	for _, tc := range allowed {
		t.Run(tc.name, func(t *testing.T) {
			if err := preprocessEnvMap(tc.envMap); err != nil {
				t.Fatalf("preprocessEnvMap: %v", err)
			}
		})
	}
}
