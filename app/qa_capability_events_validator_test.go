package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestQACapabilityEventsDeriveStableKeys(t *testing.T) {
	tests := []struct{ exact, prefix, bus, want string }{
		{"Order.Created", "", "project", "order-created"},
		{" ..ORDER__created!! ", "", "project", "order-created"},
		{"", "User.", "project", "user-prefix"},
		{"", "User.", "default", "user-prefix-default"},
		{"order.created", "", "default", "order-created-default"},
		{strings.Repeat("a", 40), "", "project", strings.Repeat("a", 32)},
		{strings.Repeat("a", 40), "", "default", strings.Repeat("a", 32) + "-default"},
		{"!!!", "", "project", ""},
	}
	for _, tt := range tests {
		t.Run(tt.bus+"/"+tt.exact+tt.prefix, func(t *testing.T) {
			qaCapabilityEqual(t, deriveEventSubscriptionKey(tt.exact, tt.prefix, tt.bus), tt.want)
		})
	}
}

func TestQACapabilityEventsValidateSubscriptionKeys(t *testing.T) {
	for _, subscriptions := range [][]interface{}{
		qaCapabilityList(qaCapabilityMap{}),
		qaCapabilityList(qaCapabilityMap{"detail_type": "one", "detail_type_prefix": "two"}),
		qaCapabilityList(qaCapabilityMap{"detail_type": "!!!"}),
		qaCapabilityList(qaCapabilityMap{"detail_type": "same", "source": "one"}, qaCapabilityMap{"detail_type": "same", "source": "two"}),
		qaCapabilityList(qaCapabilityMap{"detail_type": "Order.Created"}, qaCapabilityMap{"detail_type": "order_created"}),
		qaCapabilityList(qaCapabilityMap{"detail_type": strings.Repeat("a", 32) + "one"}, qaCapabilityMap{"detail_type": strings.Repeat("a", 32) + "two"}),
		qaCapabilityList(qaCapabilityMap{"name": "fixed", "detail_type": "one"}, qaCapabilityMap{"name": "fixed", "detail_type": "two"}),
	} {
		t.Run("invalid pattern or duplicate", func(t *testing.T) {
			m := qaCapabilityEnv()
			m["event_bus"] = qaCapabilityMap{"enabled": true}
			qaCapabilityOwner(m, "service")["events"] = qaCapabilityMap{"subscribes": subscriptions}
			qaCapabilityRequireError(t, normalizeEvents(m), "subscribes")
		})
	}
	for _, name := range []string{"Upper", "with_under", "-first", strings.Repeat("a", 33)} {
		t.Run("invalid name "+name, func(t *testing.T) {
			m := qaCapabilityEnv()
			qaCapabilityOwner(m, "service")["events"] = qaCapabilityMap{"subscribes": qaCapabilityList(qaCapabilityMap{"name": name, "detail_type": "order.created"})}
			qaCapabilityRequireError(t, normalizeEvents(m), "name")
		})
	}
	for _, name := range []string{"a", "0", strings.Repeat("a", 32)} {
		t.Run("explicit key "+name, func(t *testing.T) {
			m := qaCapabilityEnv()
			qaCapabilityOwner(m, "service")["events"] = qaCapabilityMap{"subscribes": qaCapabilityList(
				qaCapabilityMap{"name": name, "detail_type": "same", "source": "one"},
				qaCapabilityMap{"name": "other", "detail_type": "same", "source": "two"},
			)}
			qaCapabilityNoError(t, normalizeEvents(m))
			consumer := qaCapabilityNamed(t, m[eventConsumersKey], "api")
			subscriptions := qaCapabilityArray(t, consumer["subscriptions"])
			qaCapabilityEqual(t, qaCapabilityObject(t, subscriptions[0])["key"], name)
		})
	}
	t.Run("reordering preserves keys and consumers have separate namespaces", func(t *testing.T) {
		var keys [][]string
		for _, reverse := range []bool{false, true} {
			m := qaCapabilityEnv()
			m["event_bus"] = qaCapabilityMap{"enabled": true}
			subs := qaCapabilityList(qaCapabilityMap{"detail_type": "order.created"}, qaCapabilityMap{"detail_type_prefix": "user."})
			if reverse {
				subs[0], subs[1] = subs[1], subs[0]
			}
			qaCapabilityOwner(m, "service")["events"] = qaCapabilityMap{"subscribes": subs}
			qaCapabilityOwner(m, "backend")["events"] = qaCapabilityMap{"subscribes": qaCapabilityList(qaCapabilityMap{"detail_type": "order.created"})}
			qaCapabilityNoError(t, normalizeEvents(m))
			consumer := qaCapabilityNamed(t, m[eventConsumersKey], "api")
			var got []string
			for _, sub := range qaCapabilityArray(t, consumer["subscriptions"]) {
				got = append(got, qaCapabilityObject(t, sub)["key"].(string))
			}
			sort.Strings(got)
			keys = append(keys, got)
		}
		qaCapabilityEqual(t, keys[0], []string{"order-created", "user-prefix"})
		qaCapabilityEqual(t, keys[1], keys[0])
	})
}

func TestQACapabilityEventsResolveBusesInBothStates(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		for _, bus := range []string{"", "default", "project", "unknown"} {
			t.Run(map[bool]string{false: "off", true: "on"}[enabled]+"/"+bus, func(t *testing.T) {
				var raw interface{}
				if bus != "" {
					raw = bus
				}
				resolved, explicit, problem := resolveEventBus(raw, enabled, "events")
				invalid := bus == "unknown" || (bus == "project" && !enabled)
				if invalid {
					if problem == "" {
						t.Fatal("invalid bus was accepted")
					}
					return
				}
				qaCapabilityEqual(t, problem, "")
				want := bus
				if bus == "" {
					want = "default"
					if enabled {
						want = "project"
					}
				}
				qaCapabilityEqual(t, resolved, want)
				qaCapabilityEqual(t, explicit, bus != "")
				m := qaCapabilityEnv()
				m["event_bus"] = qaCapabilityMap{"enabled": enabled}
				sub := qaCapabilityMap{"detail_type": "order.created"}
				entry := qaCapabilityMap{"detail_types": []interface{}{"order.created"}}
				if bus != "" {
					sub["bus"], entry["bus"] = bus, bus
				}
				qaCapabilityOwner(m, "backend")["events"] = qaCapabilityMap{"subscribes": qaCapabilityList(sub), "produces": qaCapabilityList(entry)}
				qaCapabilityNoError(t, normalizeEvents(m))
				consumer := qaCapabilityNamed(t, m[eventConsumersKey], "backend")
				producer := qaCapabilityNamed(t, m[eventProducersKey], "backend")
				qaCapabilityEqual(t, qaCapabilityObject(t, qaCapabilityArray(t, consumer["subscriptions"])[0])["bus"], want)
				qaCapabilityEqual(t, qaCapabilityObject(t, qaCapabilityArray(t, producer["entries"])[0])["bus"], want)
			})
		}
	}
	for _, target := range []string{"subscription", "produce", "rule"} {
		t.Run("project bus off/"+target, func(t *testing.T) {
			m := qaCapabilityEnv()
			switch target {
			case "subscription":
				qaCapabilityOwner(m, "service")["events"] = qaCapabilityMap{"subscribes": qaCapabilityList(qaCapabilityMap{"detail_type": "one", "bus": "project"})}
			case "produce":
				qaCapabilityOwner(m, "scheduled_task")["events"] = qaCapabilityMap{"produces": qaCapabilityList(qaCapabilityMap{"detail_types": []interface{}{"one"}, "bus": "project"})}
			case "rule":
				qaCapabilityOwner(m, "event_task")["rules"] = qaCapabilityList(qaCapabilityMap{"name": "ingest", "detail_types": []interface{}{"one"}, "bus": "project"})
			}
			qaCapabilityRequireError(t, normalizeEvents(m), "project")
		})
	}
}

func TestQACapabilityEventsValidateProducesAndTaskSubscriptions(t *testing.T) {
	for _, kind := range []string{"scheduled_task", "event_task"} {
		t.Run(kind+" cannot consume", func(t *testing.T) {
			m := qaCapabilityEnv()
			qaCapabilityOwner(m, kind)["events"] = qaCapabilityMap{"subscribes": qaCapabilityList(qaCapabilityMap{"detail_type": "order.created"})}
			qaCapabilityRequireError(t, normalizeEvents(m), "subscribes")
		})
	}
	invalid := []qaCapabilityMap{
		{}, {"any": false}, {"detail_types": []interface{}{}},
		{"any": true, "detail_types": []interface{}{"one"}},
		{"detail_types": "one"}, {"detail_types": []interface{}{""}},
		{"detail_types": []interface{}{"one"}, "own_source_only": "false"},
	}
	for _, kind := range qaCapabilityKinds() {
		for _, entry := range invalid {
			t.Run(kind+"/invalid shape", func(t *testing.T) {
				m := qaCapabilityEnv()
				qaCapabilityOwner(m, kind)["events"] = qaCapabilityMap{"produces": qaCapabilityCloneValue(qaCapabilityList(entry))}
				qaCapabilityRequireError(t, normalizeEvents(m), "produces")
			})
		}
		for _, any := range []bool{false, true} {
			t.Run(kind+"/valid produce", func(t *testing.T) {
				m := qaCapabilityEnv()
				entry := qaCapabilityMap{"detail_types": []interface{}{"order.created"}}
				if any {
					entry = qaCapabilityMap{"any": true, "own_source_only": false}
				}
				qaCapabilityOwner(m, kind)["events"] = qaCapabilityMap{"produces": qaCapabilityCloneValue(qaCapabilityList(entry))}
				err, warnings := qaCapabilityDiagnostics(t, m, func() error { return normalizeEvents(m) })
				qaCapabilityNoError(t, err)
				producer := qaCapabilityNamed(t, m[eventProducersKey], qaCapabilityOwnerName(kind))
				wantSource := map[string]string{"backend": "myapp.backend", "service": "myapp.api", "scheduled_task": "myapp.task.nightly", "event_task": "myapp.event.mailer"}[kind]
				qaCapabilityEqual(t, producer["source"], wantSource)
				got := qaCapabilityObject(t, qaCapabilityArray(t, producer["entries"])[0])
				qaCapabilityEqual(t, got["own_source_only"], !any)
				if any {
					qaCapabilityContains(t, warnings, "any")
				}
			})
		}
	}
}

func TestQACapabilityEventsValidateQueueBounds(t *testing.T) {
	for _, tt := range []struct {
		name                 string
		queue                qaCapabilityMap
		visibility, receives int
	}{
		{"defaults", nil, 30, 5},
		{"minimum", qaCapabilityMap{"visibility_timeout": 0, "max_receive_count": 1}, 0, 1},
		{"maximum", qaCapabilityMap{"visibility_timeout": 43200, "max_receive_count": 1000}, 43200, 1000},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := qaCapabilityEnv()
			events := qaCapabilityMap{"subscribes": qaCapabilityList(qaCapabilityMap{"detail_type": "one"})}
			if tt.queue != nil {
				events["queue"] = tt.queue
			}
			qaCapabilityOwner(m, "service")["events"] = events
			qaCapabilityNoError(t, normalizeEvents(m))
			consumer := qaCapabilityNamed(t, m[eventConsumersKey], "api")
			raw, err := json.Marshal(consumer)
			qaCapabilityNoError(t, err)
			var got struct {
				Visibility int `json:"visibility_timeout"`
				Receives   int `json:"max_receive_count"`
			}
			qaCapabilityNoError(t, json.Unmarshal(raw, &got))
			qaCapabilityEqual(t, got.Visibility, tt.visibility)
			qaCapabilityEqual(t, got.Receives, tt.receives)
		})
	}
	for field, invalidValues := range map[string][]interface{}{
		"visibility_timeout": {-1, 43201, 0.5, "30", true},
		"max_receive_count":  {0, 1001, 1.5, "5", false},
	} {
		for _, value := range invalidValues {
			t.Run(field+"/invalid", func(t *testing.T) {
				m := qaCapabilityEnv()
				qaCapabilityOwner(m, "service")["events"] = qaCapabilityMap{
					"subscribes": qaCapabilityList(qaCapabilityMap{"detail_type": "one"}),
					"queue":      qaCapabilityMap{field: value},
				}
				qaCapabilityRequireError(t, normalizeEvents(m), field)
			})
		}
	}
}

func TestQACapabilityEventsRejectExtensionNameClashes(t *testing.T) {
	// Choosing this env makes the two documented naming orders coincide:
	// project-events-consumer-env and project-env-extension.
	for _, suffix := range []string{"", "-dlq", "-delivery-dlq"} {
		for _, consuming := range []bool{false, true} {
			t.Run("SQS "+suffix+map[bool]string{false: " absent", true: " consumer"}[consuming], func(t *testing.T) {
				m := qaCapabilityEnv()
				m["env"] = "events"
				extension := "api" + suffix + "-events"
				parts := []string{"events", "api"}
				if suffix != "" {
					parts = append(parts, strings.TrimPrefix(suffix, "-"))
				}
				eventName := awsName("myapp", "events", parts, 80, "-", "", "")
				extensionName := awsName("myapp", "events", []string{extension}, 80, "-", "", "myapp-events-"+extension)
				qaCapabilityEqual(t, extensionName, eventName)
				m["extensions"] = qaCapabilityMap{"sqs_queues": qaCapabilityList(qaCapabilityMap{"name": extension})}
				if consuming {
					qaCapabilityOwner(m, "service")["events"] = qaCapabilityMap{"subscribes": qaCapabilityList(qaCapabilityMap{"detail_type": "one"})}
				}
				err := normalizeEvents(m)
				if consuming {
					qaCapabilityRequireError(t, err, "sqs_queues", "api")
				} else {
					qaCapabilityNoError(t, err)
				}
			})
		}
	}
	for _, consuming := range []bool{false, true} {
		t.Run("SNS alerts"+map[bool]string{false: " absent", true: " consumer"}[consuming], func(t *testing.T) {
			m := qaCapabilityEnv()
			m["extensions"] = qaCapabilityMap{"sns_topics": qaCapabilityList(qaCapabilityMap{"name": "alerts"})}
			want := awsName("myapp", "dev", []string{"alerts"}, 256, "-", "", "myapp-dev-alerts")
			qaCapabilityEqual(t, want, "myapp-dev-alerts")
			if consuming {
				qaCapabilityOwner(m, "backend")["events"] = qaCapabilityMap{"subscribes": qaCapabilityList(qaCapabilityMap{"detail_type": "one"})}
			}
			err := normalizeEvents(m)
			if consuming {
				qaCapabilityRequireError(t, err, "sns_topics", "events")
			} else {
				qaCapabilityNoError(t, err)
			}
		})
	}
}

func TestQACapabilityEventsValidateTaskRuleNames(t *testing.T) {
	for _, format := range []string{"rules", "legacy", "mixed", "shortened"} {
		for _, differentBus := range []bool{false, true} {
			if differentBus && format == "legacy" {
				continue
			}
			t.Run(format+map[bool]string{false: " same bus", true: " different buses"}[differentBus], func(t *testing.T) {
				m := qaCapabilityEnv()
				m["event_bus"] = qaCapabilityMap{"enabled": true}
				if format == "shortened" {
					m["project"] = "myapp-" + strings.Repeat("x", 80)
				}
				first := qaCapabilityMap{"name": "mailer", "rules": qaCapabilityList(qaCapabilityMap{"name": "same", "detail_types": []interface{}{"one"}})}
				second := qaCapabilityMap{"name": "other-mailer", "rules": qaCapabilityList(qaCapabilityMap{"name": "same", "detail_types": []interface{}{"two"}})}
				if format == "legacy" || format == "mixed" {
					delete(first, "rules")
					first["rule_name"] = "same"
					first["detail_types"] = []interface{}{"one"}
				}
				if format == "legacy" {
					delete(second, "rules")
					second["rule_name"] = "same"
					second["detail_types"] = []interface{}{"two"}
				}
				if differentBus {
					second["rules"].([]interface{})[0].(qaCapabilityMap)["bus"] = "default"
				}
				m["event_processor_tasks"] = qaCapabilityList(first, second)
				err := normalizeEvents(m)
				if differentBus {
					qaCapabilityNoError(t, err)
				} else {
					qaCapabilityRequireError(t, err, "mailer", "other-mailer")
				}
			})
		}
	}
	qaCapabilityEqual(t, eventTaskRuleName("myapp", "dev", "same"), "myapp_rule_same_dev")
}

func TestQACapabilityEventsWarnWhenBusToggleMovesEntries(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		for _, changed := range []bool{false, true} {
			t.Run(map[bool]string{false: "off", true: "on"}[enabled]+map[bool]string{false: " unchanged", true: " changed"}[changed], func(t *testing.T) {
				m := qaCapabilityEnv()
				m["event_bus"] = qaCapabilityMap{"enabled": enabled}
				m[eventBusPreviousKey] = enabled != changed
				qaCapabilityOwner(m, "backend")["events"] = qaCapabilityMap{"subscribes": qaCapabilityList(qaCapabilityMap{"detail_type": "implicit-backend"})}
				qaCapabilityOwner(m, "service")["events"] = qaCapabilityMap{"produces": qaCapabilityList(
					qaCapabilityMap{"detail_types": []interface{}{"implicit-service"}},
					qaCapabilityMap{"detail_types": []interface{}{"fixed-service"}, "bus": "default"},
				)}
				qaCapabilityOwner(m, "scheduled_task")["events"] = qaCapabilityMap{"produces": qaCapabilityList(qaCapabilityMap{"detail_types": []interface{}{"implicit-task"}})}
				err, diagnostics := qaCapabilityDiagnostics(t, m, func() error { return normalizeEvents(m) })
				qaCapabilityNoError(t, err)
				if changed {
					qaCapabilityContains(t, diagnostics, "EVENT_BUS_NAME", "default", "myapp-dev", "subscribes", "produces", "api", "nightly", "mailer")
					if strings.Contains(diagnostics, "fixed-service") {
						t.Errorf("explicit default bus should not move: %s", diagnostics)
					}
				} else if strings.Contains(diagnostics, "EVENT_BUS_NAME") {
					t.Errorf("unchanged bus must not warn about moving entries: %s", diagnostics)
				}
			})
		}
	}
}

func TestQACapabilityEventsMatchSourceVectors(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "event_source_vectors.json"))
	qaCapabilityNoError(t, err)
	var fixture struct {
		Vectors []struct{ Name, Kind, Project, Workload, Want string }
	}
	qaCapabilityNoError(t, json.Unmarshal(raw, &fixture))
	if len(fixture.Vectors) == 0 {
		t.Fatal("event source vectors must not be empty")
	}
	seen := map[string]bool{}
	for _, vector := range fixture.Vectors {
		t.Run(vector.Name, func(t *testing.T) {
			qaCapabilityEqual(t, eventSource(vector.Kind, vector.Project, vector.Workload), vector.Want)
		})
		seen[vector.Kind] = true
	}
	for _, kind := range qaCapabilityKinds() {
		if !seen[kind] {
			t.Errorf("event source vectors do not cover %s", kind)
		}
	}
}
