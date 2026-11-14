package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/ext/tryfunc"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
	ctyjson "github.com/zclconf/go-cty/cty/json"
)

// Supply synthetic module outputs and honor whether a public module call uses
// count. Expressions are evaluated, so harmless HCL formatting or try() wrappers
// do not change the assertion.
func qaCapabilityBridgeContext(fields map[string]hclsyntax.Expression, outputs map[string][2]string) (*hcl.EvalContext, map[string]cty.Value) {
	values := map[string]cty.Value{
		"data_bucket_name":         cty.StringVal("myapp-data-dev-000000000000"),
		"data_bucket_arn":          cty.StringVal("arn:aws:s3:::myapp-data-dev-000000000000"),
		"backend_bucket_arn":       cty.StringVal("arn:aws:s3:::myapp-backend-dev"),
		"event_bus_name":           cty.StringVal("myapp-dev"),
		"event_bus_arn":            cty.StringVal("arn:aws:events:us-east-1:000000000000:event-bus/myapp-dev"),
		"event_queue_urls":         cty.MapVal(map[string]cty.Value{"api": cty.StringVal("https://sqs.us-east-1.amazonaws.com/000000000000/myapp-events-api-dev")}),
		"alerts_topic_arn":         cty.StringVal("arn:aws:sns:us-east-1:000000000000:myapp-dev-alerts"),
		"valkey_address":           cty.StringVal("myapp-cache.example.com"),
		"valkey_port":              cty.NumberIntVal(6379),
		"valkey_security_group_id": cty.StringVal("sg-00000000000000000"),
	}
	moduleFields := map[string]map[string]cty.Value{}
	for key, output := range outputs {
		if moduleFields[output[0]] == nil {
			moduleFields[output[0]] = map[string]cty.Value{}
		}
		moduleFields[output[0]][output[1]] = values[key]
	}
	indexed := map[string]bool{}
	for _, expr := range fields {
		for _, traversal := range expr.Variables() {
			if len(traversal) < 3 || traversal.RootName() != "module" {
				continue
			}
			module, isAttribute := traversal[1].(hcl.TraverseAttr)
			_, isIndex := traversal[2].(hcl.TraverseIndex)
			if isAttribute && isIndex {
				indexed[module.Name] = true
			}
		}
	}
	modules := map[string]cty.Value{}
	for name, fields := range moduleFields {
		value := cty.ObjectVal(fields)
		if indexed[name] {
			value = cty.TupleVal([]cty.Value{value})
		}
		modules[name] = value
	}
	ctx := &hcl.EvalContext{
		Variables: map[string]cty.Value{"module": cty.ObjectVal(modules)},
		Functions: map[string]function.Function{"try": tryfunc.TryFunc},
	}
	return ctx, values
}

func TestQACapabilityBridgeExposesOnAndOffValues(t *testing.T) {
	outputs := map[string][2]string{
		"data_bucket_name":         {"data_bucket", "bucket_name"},
		"data_bucket_arn":          {"data_bucket", "bucket_arn"},
		"backend_bucket_arn":       {"workloads", "backend_bucket_arn"},
		"event_bus_name":           {"events", "bus_name"},
		"event_bus_arn":            {"events", "bus_arn"},
		"event_queue_urls":         {"events", "queue_urls"},
		"alerts_topic_arn":         {"events", "alerts_topic_arn"},
		"valkey_address":           {"valkey", "address"},
		"valkey_port":              {"valkey", "port"},
		"valkey_security_group_id": {"valkey", "security_group_id"},
	}
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "optional features off", true: "all features on"}[enabled], func(t *testing.T) {
			t.Chdir(t.TempDir())
			m := qaCapabilityEnv()
			if enabled {
				m["event_bus"] = qaCapabilityMap{"enabled": true}
				m["sqs"] = qaCapabilityMap{"enabled": true, "name": "jobs"}
				m["data_bucket"] = qaCapabilityMap{"shared": qaCapabilityList(qaCapabilityMap{"name": "uploads", "read_write": []interface{}{"backend", "api"}, "read": []interface{}{"nightly", "mailer"}})}
				m["vaults"] = qaCapabilityList(qaCapabilityMap{"name": "shared"})
				m["valkey"] = qaCapabilityMap{"enabled": true, "engine_version": "8", "max_storage_gb": 1, "access": []interface{}{"api", "nightly"}}
				for _, kind := range qaCapabilityKinds() {
					owner := qaCapabilityOwner(m, kind)
					owner["vaults"] = qaCapabilityList(qaCapabilityMap{"name": "shared", "prefix": "PAY"})
					owner["policy"] = qaCapabilityList(qaCapabilityPolicyStatement())
					owner["events"] = qaCapabilityMap{"produces": qaCapabilityList(qaCapabilityMap{"detail_types": []interface{}{"order.created"}})}
				}
				qaCapabilityOwner(m, "service")["events"].(qaCapabilityMap)["subscribes"] = qaCapabilityList(qaCapabilityMap{"detail_type": "order.created"})
				app := qaCapabilityAmplifyApp(qaCapabilityMap{"source": "/backend-api/<*>", "workload": "backend"})
				app["environment_variables"] = qaCapabilityMap{"AMPLIFY_MONOREPO_APP_ROOT": "apps/web"}
				m["amplify_apps"] = qaCapabilityList(app)
			}
			qaCapabilityNoError(t, preprocessEnvMap(m))
			if err := os.MkdirAll(filepath.Join("env", "dev"), 0755); err != nil {
				t.Fatal(err)
			}
			generateBridgeFile("dev", m)
			raw, err := os.ReadFile(filepath.Join("env", "dev", "_bridge.tf"))
			qaCapabilityNoError(t, err)
			rendered := string(raw)
			root := qaCapabilityParse(t, rendered)
			var bridgeFound bool
			var bridgeFields map[string]hclsyntax.Expression
			// Enabled fields reference outputs whose real values are only
			// known to Terraform; disabled optional fields must be literals.
			for _, block := range root.Blocks {
				if block.Type != "locals" {
					continue
				}
				attr := block.Body.Attributes["bridge"]
				if attr == nil {
					continue
				}
				bridgeFound = true
				bridgeFields = qaCapabilityObjectItems(t, attr.Expr)
			}
			if !bridgeFound {
				t.Fatal("generated bridge local is missing")
			}
			ctx, expectedValues := qaCapabilityBridgeContext(bridgeFields, outputs)
			for key := range outputs {
				expr, exists := bridgeFields[key]
				if !exists {
					t.Errorf("bridge key %q is missing", key)
					continue
				}
				alwaysOn := key == "data_bucket_name" || key == "data_bucket_arn" || key == "backend_bucket_arn"
				if enabled || alwaysOn {
					expected, err := ctyjson.Marshal(expectedValues[key], expectedValues[key].Type())
					qaCapabilityNoError(t, err)
					qaCapabilityWantJSON(t, qaCapabilityJSON(t, expr, ctx), string(expected))
				} else if key == "event_queue_urls" {
					qaCapabilityWantJSON(t, qaCapabilityJSON(t, expr, nil), `{}`)
				} else {
					qaCapabilityEqual(t, qaCapabilityJSON(t, expr, nil), "")
				}
			}
			var outputNames []string
			for _, block := range root.Blocks {
				if block.Type == "output" {
					outputNames = append(outputNames, block.Labels[0])
					if block.Labels[0] == "bridge" {
						qaCapabilityContains(t, qaCapabilityExprText(qaCapabilityAttr(t, block.Body, "value").Expr, rendered), "local.bridge")
					}
				}
			}
			qaCapabilityEqual(t, outputNames, []string{"bridge"})
		})
	}
}
