package main

import (
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"
)

func TestQACapabilityEventsRendersConsumersProducersAndRuleBuses(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		for _, legacy := range []bool{false, true} {
			name := map[bool]string{false: "off", true: "on"}[enabled] + map[bool]string{false: " rules", true: " legacy"}[legacy]
			t.Run(name, func(t *testing.T) {
				m := qaCapabilityEnv()
				m["event_bus"] = qaCapabilityMap{"enabled": enabled}
				qaCapabilityOwner(m, "backend")["events"] = qaCapabilityMap{
					"subscribes": qaCapabilityList(qaCapabilityMap{"detail_type": "order.created"}),
					"produces":   qaCapabilityList(qaCapabilityMap{"detail_types": []interface{}{"backend.ready"}}),
				}
				qaCapabilityOwner(m, "service")["events"] = qaCapabilityMap{
					"subscribes": qaCapabilityList(qaCapabilityMap{"name": "users", "detail_type_prefix": "user.", "source": "myapp.identity", "bus": "default"}),
					"queue":      qaCapabilityMap{"visibility_timeout": 60, "max_receive_count": 7},
					"produces":   qaCapabilityList(qaCapabilityMap{"any": true, "own_source_only": false, "bus": "default"}),
				}
				qaCapabilityOwner(m, "scheduled_task")["events"] = qaCapabilityMap{"produces": qaCapabilityList(qaCapabilityMap{"detail_types": []interface{}{"job.done"}})}
				task := qaCapabilityOwner(m, "event_task")
				task["events"] = qaCapabilityMap{"produces": qaCapabilityList(qaCapabilityMap{"detail_types": []interface{}{"mail.sent"}})}
				if legacy {
					delete(task, "rules")
					task["rule_name"] = "ingest"
					task["sources"] = []interface{}{"myapp.api"}
					task["detail_types"] = []interface{}{"order.created"}
				} else {
					task["rules"] = qaCapabilityList(
						qaCapabilityMap{"name": "ingest", "sources": []interface{}{"myapp.api"}, "detail_types": []interface{}{"order.created"}},
						qaCapabilityMap{"name": "fixed", "sources": []interface{}{"myapp.api"}, "detail_types": []interface{}{"order.deleted"}, "bus": "default"},
					)
				}
				rendered := renderMainHBS(t, m)
				body := qaCapabilityParse(t, rendered)
				events := qaCapabilityModule(t, body, "events")
				qaCapabilityEqual(t, qaCapabilityLiteral(t, events, "bus_enabled"), enabled)
				consumers := qaCapabilityLiteral(t, events, "consumers")
				backend := qaCapabilityNamed(t, consumers, "backend")
				service := qaCapabilityNamed(t, consumers, "api")
				wantBus := "default"
				wantBusName := "default"
				if enabled {
					wantBus, wantBusName = "project", "myapp-dev"
				}
				backendSub := qaCapabilityObject(t, qaCapabilityArray(t, backend["subscriptions"])[0])
				qaCapabilityEqual(t, backendSub["bus"], wantBus)
				qaCapabilityEqual(t, backendSub["detail_type"], "order.created")
				serviceSub := qaCapabilityObject(t, qaCapabilityArray(t, service["subscriptions"])[0])
				qaCapabilityEqual(t, serviceSub["bus"], "default")
				qaCapabilityEqual(t, serviceSub["key"], "users")
				qaCapabilityEqual(t, serviceSub["detail_type_prefix"], "user.")
				qaCapabilityEqual(t, serviceSub["source"], "myapp.identity")
				qaCapabilityEqual(t, service["visibility_timeout"], float64(60))
				qaCapabilityEqual(t, service["max_receive_count"], float64(7))
				producers := qaCapabilityLiteral(t, events, "producers")
				if len(qaCapabilityArray(t, producers)) != 4 {
					t.Fatal("all four workload kinds must retain their produces entries")
				}
				for workload, source := range map[string]string{"backend": "myapp.backend", "api": "myapp.api", "nightly": "myapp.task.nightly", "mailer": "myapp.event.mailer"} {
					producer := qaCapabilityNamed(t, producers, workload)
					qaCapabilityEqual(t, producer["source"], source)
					entry := qaCapabilityObject(t, qaCapabilityArray(t, producer["entries"])[0])
					if workload == "api" {
						qaCapabilityEqual(t, entry["bus"], "default")
						qaCapabilityEqual(t, entry["any"], true)
						qaCapabilityEqual(t, entry["own_source_only"], false)
					} else {
						qaCapabilityEqual(t, entry["bus"], wantBus)
						qaCapabilityEqual(t, entry["own_source_only"], true)
					}
				}
				ctx := &hcl.EvalContext{Variables: map[string]cty.Value{
					"module": cty.ObjectVal(map[string]cty.Value{"events": cty.ObjectVal(map[string]cty.Value{"bus_name": cty.StringVal(wantBusName)})}),
				}}
				for _, moduleName := range []string{"workloads", "schedule_task_nightly", "event_bus_task_mailer"} {
					attr := qaCapabilityAttr(t, qaCapabilityModule(t, body, moduleName), "event_bus_name")
					qaCapabilityEqual(t, qaCapabilityJSON(t, attr.Expr, ctx), wantBusName)
					if enabled {
						qaCapabilityContains(t, qaCapabilityExprText(attr.Expr, rendered), "module.events.bus_name")
					}
				}
				if !legacy {
					rules := qaCapabilityObject(t, qaCapabilityJSON(t, qaCapabilityAttr(t, qaCapabilityModule(t, body, "event_bus_task_mailer"), "rules").Expr, ctx))
					qaCapabilityEqual(t, len(rules), 2)
					qaCapabilityEqual(t, qaCapabilityObject(t, rules["ingest"])["event_bus_name"], wantBusName)
					qaCapabilityEqual(t, qaCapabilityObject(t, rules["fixed"])["event_bus_name"], "default")
				}
			})
		}
	}
}
