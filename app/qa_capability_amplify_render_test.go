package main

import (
	"testing"
)

func TestQACapabilityAmplifyRendersIngressBaseURLs(t *testing.T) {
	tests := []struct {
		name                      string
		alb, domain               bool
		prefix, apiHost, wantBase string
	}{
		{"ALB with prefix", true, true, "api", "api.dev.example.com", "https://api.dev.example.com"},
		{"ALB without prefix", true, true, "", "dev.example.com", "https://dev.example.com"},
		{"API Gateway domain and prefix", false, true, "api", "api.dev.example.com", "https://api.dev.example.com"},
		{"API Gateway without prefix", false, true, "", "dev.example.com", "https://example.execute-api.us-east-1.amazonaws.com/dev"},
		{"API Gateway without domain", false, false, "api", "", "https://example.execute-api.us-east-1.amazonaws.com/dev"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := qaCapabilityEnv()
			m["alb"] = qaCapabilityMap{"enabled": tt.alb}
			domain := m["domain"].(qaCapabilityMap)
			domain["enabled"] = tt.domain
			domain["api_domain_prefix"] = tt.prefix
			m["amplify_apps"] = qaCapabilityList(qaCapabilityAmplifyApp(
				qaCapabilityMap{"source": "/backend-api/<*>", "workload": "backend"},
				qaCapabilityMap{"source": "/reports/<*>", "workload": "api"},
			))
			rendered := renderMainHBS(t, m)
			module := qaCapabilityModule(t, qaCapabilityParse(t, rendered), "amplify")
			urls := qaCapabilityObject(t, qaCapabilityJSON(t, qaCapabilityAttr(t, module, "workload_urls").Expr, qaCapabilityURLContext(tt.apiHost)))
			qaCapabilityEqual(t, urls["backend"], tt.wantBase)
			qaCapabilityEqual(t, urls["api"], tt.wantBase+"/api")
		})
	}
}

func TestQACapabilityAmplifyRendersRulesAndAppEnvironment(t *testing.T) {
	m := qaCapabilityEnv()
	app := qaCapabilityAmplifyApp(
		qaCapabilityMap{"source": "/backend-api/<*>", "workload": "backend"},
		qaCapabilityMap{"source": "/api/<*>", "workload": "api"},
		qaCapabilityMap{"source": "/r/<*>", "workload": "api", "path": "/<*>"},
		qaCapabilityMap{"source": "/alias/<*>", "workload": "api"},
		qaCapabilityMap{"source": "/old-pricing", "target": "/pricing", "status": "301"},
	)
	app["environment_variables"] = qaCapabilityMap{"AMPLIFY_MONOREPO_APP_ROOT": "apps/web", "APP_STAGE": "dev"}
	m["amplify_apps"] = qaCapabilityList(app)
	rendered := renderMainHBS(t, m)
	module := qaCapabilityModule(t, qaCapabilityParse(t, rendered), "amplify")
	result := qaCapabilityNamed(t, qaCapabilityLiteral(t, module, "amplify_apps"), "web")
	qaCapabilityWantJSON(t, result["environment_variables"], `{"AMPLIFY_MONOREPO_APP_ROOT":"apps/web","APP_STAGE":"dev"}`)
	rules := qaCapabilityArray(t, result["custom_rules"])
	if len(rules) != 5 {
		t.Fatalf("got %d rules, want 5", len(rules))
	}
	for i, path := range []string{"/backend-api/<*>", "/<*>", "/<*>", "/alias/<*>"} {
		rule := qaCapabilityObject(t, rules[i])
		qaCapabilityEqual(t, rule["target_path"], path)
		qaCapabilityEqual(t, rule["status"], "200")
	}
	redirect := qaCapabilityObject(t, rules[4])
	qaCapabilityEqual(t, redirect["source"], "/old-pricing")
	qaCapabilityEqual(t, redirect["target"], "/pricing")
	qaCapabilityEqual(t, redirect["status"], "301")
}
