package main

import (
	"strings"
	"testing"
)

func qaCapabilityAmplifyApp(rules ...qaCapabilityMap) qaCapabilityMap {
	return qaCapabilityMap{
		"name": "web", "subdomain_prefix": "app",
		"github_repository": "https://github.com/example/web",
		"branches":          qaCapabilityList(qaCapabilityMap{"name": "main"}),
		"custom_rules":      qaCapabilityList(rules...),
	}
}

func TestQACapabilityAmplifyRejectsInvalidRules(t *testing.T) {
	tests := []struct {
		name  string
		rule  qaCapabilityMap
		field string
	}{
		{"both destinations", qaCapabilityMap{"source": "/api/<*>", "target": "/other", "workload": "backend"}, "workload"},
		{"no destination", qaCapabilityMap{"source": "/api/<*>"}, "target"},
		{"unsupported status", qaCapabilityMap{"source": "/old", "target": "/new", "status": "307"}, "status"},
		{"workload redirect", qaCapabilityMap{"source": "/api/<*>", "workload": "backend", "status": "301"}, "status"},
		{"source wildcard in middle", qaCapabilityMap{"source": "/<*>/tail", "target": "/new"}, "source"},
		{"repeated source wildcard", qaCapabilityMap{"source": "/<*>/<*>", "target": "/new"}, "source"},
		{"target wildcard in middle", qaCapabilityMap{"source": "/old/<*>", "target": "/<*>/tail"}, "target"},
		{"repeated target wildcard", qaCapabilityMap{"source": "/old/<*>", "target": "/<*>/<*>"}, "target"},
		{"unknown workload", qaCapabilityMap{"source": "/api/<*>", "workload": "missing"}, "missing"},
		{"scheduled workload", qaCapabilityMap{"source": "/api/<*>", "workload": "nightly"}, "nightly"},
		{"event workload", qaCapabilityMap{"source": "/api/<*>", "workload": "mailer"}, "mailer"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := qaCapabilityEnv()
			m["amplify_apps"] = qaCapabilityList(qaCapabilityAmplifyApp(tt.rule))
			qaCapabilityPrepareNames(t, m)
			qaCapabilityRequireError(t, prepareAmplifyRewriteRules(m), "custom_rules", tt.field)
		})
	}
	for _, access := range []string{"internal", "none"} {
		t.Run(access+" service", func(t *testing.T) {
			m := qaCapabilityEnv()
			qaCapabilityOwner(m, "service")["network_access"] = access
			m["amplify_apps"] = qaCapabilityList(qaCapabilityAmplifyApp(qaCapabilityMap{"source": "/api/<*>", "workload": "api"}))
			qaCapabilityPrepareNames(t, m)
			qaCapabilityRequireError(t, prepareAmplifyRewriteRules(m), "api")
		})
	}
}

func TestQACapabilityAmplifyAcceptsSupportedRules(t *testing.T) {
	for _, status := range []interface{}{nil, "200", "301", 301, "302", "404-200"} {
		t.Run("status", func(t *testing.T) {
			m := qaCapabilityEnv()
			rule := qaCapabilityMap{"source": "/old/<*>", "target": "/new/<*>"}
			if status != nil {
				rule["status"] = status
			}
			m["amplify_apps"] = qaCapabilityList(qaCapabilityAmplifyApp(rule))
			qaCapabilityPrepareNames(t, m)
			qaCapabilityNoError(t, prepareAmplifyRewriteRules(m))
			want := "200"
			if status != nil {
				switch value := status.(type) {
				case string:
					want = value
				case int:
					want = "301"
				}
			}
			qaCapabilityEqual(t, rule["status"], want)
		})
	}
	for _, access := range []string{"", "public", "full_open"} {
		t.Run("service access "+access, func(t *testing.T) {
			m := qaCapabilityEnv()
			qaCapabilityOwner(m, "service")["network_access"] = access
			rule := qaCapabilityMap{"source": "/api/<*>", "workload": "api"}
			m["amplify_apps"] = qaCapabilityList(qaCapabilityAmplifyApp(rule))
			qaCapabilityPrepareNames(t, m)
			qaCapabilityNoError(t, prepareAmplifyRewriteRules(m))
			qaCapabilityEqual(t, rule["status"], "200")
		})
	}
	t.Run("disabled service rule is dropped with warning", func(t *testing.T) {
		m := qaCapabilityEnv()
		qaCapabilityOwner(m, "service")["enabled"] = false
		app := qaCapabilityAmplifyApp(
			qaCapabilityMap{"source": "/api/<*>", "workload": "api"},
			qaCapabilityMap{"source": "/old", "target": "/new", "status": "301"},
		)
		m["amplify_apps"] = qaCapabilityList(app)
		qaCapabilityPrepareNames(t, m)

		qaCapabilityNoError(t, prepareAmplifyRewriteRules(m))

		rules := qaCapabilityArray(t, app["custom_rules"])
		if len(rules) != 1 {
			t.Fatalf("got %d rules after dropping disabled service, want 1", len(rules))
		}
		remaining := qaCapabilityObject(t, rules[0])
		qaCapabilityEqual(t, remaining["source"], "/old")
		qaCapabilityEqual(t, remaining["target"], "/new")
		qaCapabilityEqual(t, remaining["status"], "301")
		qaCapabilityContains(t, strings.Join(generateWarnings(m), "\n"), "api", "custom_rules")
	})
}

// Backend rewrites must not resolve to a public service's route.
func TestQACapabilityAmplifyRejectsBackendPathsShadowedByServices(t *testing.T) {
	for _, alb := range []bool{false, true} {
		for _, access := range []string{"public", "full_open"} {
			for _, path := range []string{"/api", "/api/", "/api/orders/<*>", "/api<*>"} {
				for _, explicit := range []bool{false, true} {
					ingress := map[bool]string{false: "API Gateway", true: "ALB"}[alb]
					form := map[bool]string{false: "source-derived", true: "explicit path"}[explicit]
					t.Run(ingress+"/"+access+"/"+form+path, func(t *testing.T) {
						m := qaCapabilityEnv()
						m["alb"] = qaCapabilityMap{"enabled": alb}
						qaCapabilityOwner(m, "service")["network_access"] = access
						rule := qaCapabilityMap{"source": path, "workload": "backend"}
						if explicit {
							rule["source"] = "/frontend/<*>"
							rule["path"] = path
						}
						m["amplify_apps"] = qaCapabilityList(qaCapabilityAmplifyApp(
							qaCapabilityMap{"source": "/old", "target": "/new", "status": "301"},
							rule,
						))

						err := preprocessEnvMap(m)

						qaCapabilityRequireError(t, err, "amplify_apps 'web'", "custom_rules[1]",
							"target path \""+path+"\"", "backend reaches service 'api'")
					})
				}
			}
		}
	}
}

// Only the resolved backend path and routed services cause shadowing.
func TestQACapabilityAmplifyAcceptsUnshadowedBackendAndServicePaths(t *testing.T) {
	tests := []struct {
		name, access, workload, source, path, wantPath string
	}{
		{"internal exact path", "internal", "backend", "/api", "", "/api"},
		{"internal child path", "internal", "backend", "/api/orders/<*>", "", "/api/orders/<*>"},
		{"internal adjacent wildcard", "internal", "backend", "/api<*>", "", "/api<*>"},
		{"none exact path", "none", "backend", "/api", "", "/api"},
		{"none child path", "none", "backend", "/api/orders/<*>", "", "/api/orders/<*>"},
		{"none adjacent wildcard", "none", "backend", "/api<*>", "", "/api<*>"},
		{"public similar segment", "public", "backend", "/apiculture/<*>", "", "/apiculture/<*>"},
		{"full open similar segment", "full_open", "backend", "/apiculture/<*>", "", "/apiculture/<*>"},
		{"public later segment", "public", "backend", "/v1/api/<*>", "", "/v1/api/<*>"},
		{"full open root path", "full_open", "backend", "/", "", "/"},
		{"public service destination", "public", "api", "/api/<*>", "", "/<*>"},
		{"full open service destination", "full_open", "api", "/api/<*>", "", "/<*>"},
		{"explicit safe path overrides shadowed source", "public", "backend", "/api/<*>", "/health", "/health"},
	}
	for _, alb := range []bool{false, true} {
		for _, tt := range tests {
			ingress := map[bool]string{false: "API Gateway", true: "ALB"}[alb]
			t.Run(ingress+"/"+tt.name, func(t *testing.T) {
				m := qaCapabilityEnv()
				m["alb"] = qaCapabilityMap{"enabled": alb}
				qaCapabilityOwner(m, "service")["network_access"] = tt.access
				rule := qaCapabilityMap{"source": tt.source, "workload": tt.workload}
				if tt.path != "" {
					rule["path"] = tt.path
				}
				m["amplify_apps"] = qaCapabilityList(qaCapabilityAmplifyApp(rule))

				qaCapabilityNoError(t, preprocessEnvMap(m))

				qaCapabilityEqual(t, rule[amplifyTargetPathKey], tt.wantPath)
				qaCapabilityEqual(t, rule["status"], "200")
			})
		}
	}
}

func TestQACapabilityAmplifyWarnsAfterCatchAll(t *testing.T) {
	m := qaCapabilityEnv()
	m["amplify_apps"] = qaCapabilityList(qaCapabilityAmplifyApp(
		qaCapabilityMap{"source": "/before", "target": "/first"},
		qaCapabilityMap{"source": "/<*>", "target": "/index.html"},
		qaCapabilityMap{"source": "/after-one", "target": "/one"},
		qaCapabilityMap{"source": "/after-two", "target": "/two"},
	))
	qaCapabilityPrepareNames(t, m)
	qaCapabilityNoError(t, prepareAmplifyRewriteRules(m))
	warnings := amplifyRewriteWarnings(m)
	qaCapabilityEqual(t, len(warnings), 2)
	joined := strings.Join(warnings, "\n")
	for source, field := range map[string]string{
		"/after-one": "custom_rules[2]",
		"/after-two": "custom_rules[3]",
	} {
		if !strings.Contains(joined, source) && !strings.Contains(joined, field) {
			t.Errorf("warning must identify unreachable rule %s (%s): %s", source, field, joined)
		}
	}
}

func TestQACapabilityAmplifyRefusesSelfHostAndALBWithoutDomain(t *testing.T) {
	tests := []struct {
		name   string
		change func(qaCapabilityMap, qaCapabilityMap)
	}{
		{"ALB has no TLS domain", func(m, app qaCapabilityMap) {
			m["alb"] = qaCapabilityMap{"enabled": true}
			m["domain"].(qaCapabilityMap)["enabled"] = false
		}},
		{"API domain is the app prefix", func(m, app qaCapabilityMap) { app["subdomain_prefix"] = "api" }},
		{"custom domain is the API host", func(m, app qaCapabilityMap) { app["custom_domain"] = "api.dev.example.com" }},
		{"ALB empty prefix is app host", func(m, app qaCapabilityMap) {
			m["alb"] = qaCapabilityMap{"enabled": true}
			m["domain"].(qaCapabilityMap)["api_domain_prefix"] = ""
			app["custom_domain"] = "dev.example.com"
		}},
		{"another app owns API host", func(m, app qaCapabilityMap) {
			m["amplify_apps"] = append(m["amplify_apps"].([]interface{}), qaCapabilityMap{"name": "other", "custom_domain": "api.dev.example.com"})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := qaCapabilityEnv()
			app := qaCapabilityAmplifyApp(qaCapabilityMap{"source": "/backend-api/<*>", "workload": "backend"})
			m["amplify_apps"] = qaCapabilityList(app)
			tt.change(m, app)
			qaCapabilityPrepareNames(t, m)
			qaCapabilityRequireError(t, prepareAmplifyRewriteRules(m))
		})
	}
	t.Run("execute-api cannot self-host on env domain", func(t *testing.T) {
		m := qaCapabilityEnv()
		m["domain"].(qaCapabilityMap)["api_domain_prefix"] = ""
		app := qaCapabilityAmplifyApp(qaCapabilityMap{"source": "/backend-api/<*>", "workload": "backend"})
		app["custom_domain"] = "dev.example.com"
		m["amplify_apps"] = qaCapabilityList(app)
		qaCapabilityPrepareNames(t, m)
		qaCapabilityNoError(t, prepareAmplifyRewriteRules(m))
	})
}

func TestQACapabilityAmplifyJoinsWorkloadPaths(t *testing.T) {
	tests := []struct{ workload, source, path, want string }{
		{"backend", "/backend-api/<*>", "", "/backend-api/<*>"},
		{"api", "/api/<*>", "", "/<*>"},
		{"api", "/alias/<*>", "", "/alias/<*>"},
		{"api", "/apiculture/<*>", "", "/apiculture/<*>"},
		{"api", "/alias/<*>", "/<*>", "/<*>"},
		{"backend", "/api/<*>", "/health", "/health"},
	}
	for _, tt := range tests {
		t.Run(tt.workload+tt.source+tt.path, func(t *testing.T) {
			qaCapabilityEqual(t, amplifyTargetPath(tt.workload, tt.source, tt.path), tt.want)
			m := qaCapabilityEnv()
			rule := qaCapabilityMap{"source": tt.source, "workload": tt.workload}
			if tt.path != "" {
				rule["path"] = tt.path
			}
			m["amplify_apps"] = qaCapabilityList(qaCapabilityAmplifyApp(rule))
			qaCapabilityPrepareNames(t, m)
			qaCapabilityNoError(t, prepareAmplifyRewriteRules(m))
			qaCapabilityEqual(t, rule[amplifyTargetPathKey], tt.want)
		})
	}
}
