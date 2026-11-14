package main

import (
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2/hclsyntax"
)

func qaCapabilityObjectItems(t *testing.T, expr hclsyntax.Expression) map[string]hclsyntax.Expression {
	t.Helper()
	object, ok := expr.(*hclsyntax.ObjectConsExpr)
	if !ok {
		t.Fatalf("expected generated object, got %T", expr)
	}
	result := map[string]hclsyntax.Expression{}
	for _, item := range object.Items {
		key, diagnostics := item.KeyExpr.Value(nil)
		if diagnostics.HasErrors() {
			t.Fatalf("object key is not literal: %s", diagnostics.Error())
		}
		result[key.AsString()] = item.ValueExpr
	}
	return result
}

func TestQACapabilityTasksRenderQueueURLsAndKeepOverrides(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		for _, override := range []bool{false, true} {
			name := "SQS off"
			if enabled {
				name = "SQS on"
			}
			if override {
				name += " with YAML override"
			}
			t.Run(name, func(t *testing.T) {
				m := qaCapabilityEnv()
				m["sqs"] = qaCapabilityMap{"enabled": enabled, "name": "jobs"}
				for _, kind := range []string{"scheduled_task", "event_task"} {
					if override {
						qaCapabilityOwner(m, kind)["environment_variables"] = qaCapabilityMap{
							"SQS_QUEUE_URL": "https://sqs.us-east-1.amazonaws.com/000000000000/myapp-custom",
						}
					}
				}
				rendered := renderMainHBS(t, m)
				body := qaCapabilityParse(t, rendered)
				for _, moduleName := range []string{"schedule_task_nightly", "event_bus_task_mailer"} {
					module := qaCapabilityModule(t, body, moduleName)
					// The module adds its default queue URL; YAML overrides use custom_env_vars.
					hasSQSEnable := false
					if attr := module.Attributes["sqs_enable"]; attr != nil {
						qaCapabilityEqual(t, qaCapabilityJSON(t, attr.Expr, nil), enabled)
						hasSQSEnable = true
					}
					if enabled && !hasSQSEnable {
						t.Fatalf("%s must enable SQS", moduleName)
					}
					if enabled {
						expr := qaCapabilityAttr(t, module, "sqs_queue_url").Expr
						source := qaCapabilityExprText(expr, rendered)
						qaCapabilityContains(t, source, "module.sqs.")
						if len(expr.Variables()) == 0 {
							t.Errorf("%s queue URL must reference the real SQS module, got %s", moduleName, source)
						}
					} else if attr := module.Attributes["sqs_queue_url"]; attr != nil {
						qaCapabilityEqual(t, qaCapabilityJSON(t, attr.Expr, nil), "")
					}

					var queueValues []hclsyntax.Expression
					if attr := module.Attributes["custom_env_vars"]; attr != nil {
						tuple, ok := attr.Expr.(*hclsyntax.TupleConsExpr)
						if !ok {
							t.Fatalf("custom_env_vars must be a list, got %T", attr.Expr)
						}
						for _, element := range tuple.Exprs {
							fields := qaCapabilityObjectItems(t, element)
							if qaCapabilityJSON(t, fields["name"], nil) == "SQS_QUEUE_URL" {
								queueValues = append(queueValues, fields["value"])
							}
						}
						if !enabled && strings.Contains(qaCapabilityExprText(attr.Expr, rendered), "module.sqs.") {
							t.Errorf("%s references SQS while it is disabled", moduleName)
						}
					}
					wantCount := 0
					if override {
						wantCount = 1
					}
					if len(queueValues) != wantCount {
						t.Fatalf("%s has %d custom SQS_QUEUE_URL entries, want %d", moduleName, len(queueValues), wantCount)
					}
					if override {
						qaCapabilityEqual(t, qaCapabilityJSON(t, queueValues[0], nil),
							"https://sqs.us-east-1.amazonaws.com/000000000000/myapp-custom")
					}
				}
			})
		}
	}
}

func TestQACapabilityTasksRenderCrossAccountImages(t *testing.T) {
	for _, strategy := range []string{"cross_account", "local"} {
		t.Run(strategy, func(t *testing.T) {
			m := qaCapabilityEnv()
			m["ecr_strategy"] = strategy
			m["ecr_account_id"] = "000000000000"
			m["ecr_account_region"] = "us-west-2"
			rendered := renderMainHBS(t, m)
			body := qaCapabilityParse(t, rendered)
			for moduleName, name := range map[string]string{
				"schedule_task_nightly": "nightly", "event_bus_task_mailer": "mailer",
			} {
				module := qaCapabilityModule(t, body, moduleName)
				// Local mode creates its own repository and does not consume ecr_url.
				if attr := module.Attributes["ecr_strategy"]; attr != nil {
					qaCapabilityEqual(t, qaCapabilityJSON(t, attr.Expr, nil), strategy)
				} else if strategy != "local" {
					t.Fatalf("%s must select cross-account ECR", moduleName)
				}
				attr := module.Attributes["ecr_url"]
				if strategy == "cross_account" {
					if attr == nil {
						t.Fatalf("%s has no cross-account ecr_url", moduleName)
					}
					qaCapabilityEqual(t, qaCapabilityJSON(t, attr.Expr, nil),
						"000000000000.dkr.ecr.us-west-2.amazonaws.com/myapp_task_"+name)
				}
			}
		})
	}
}
