package main

import (
	"regexp"
	"strings"
	"testing"
)

// eventConsumersLine finds the consumers argument of module "events" in a
// rendered main.tf.
var eventConsumersLine = regexp.MustCompile(`(?m)^\s*consumers\s*=\s*(.*)$`)

func renderedEventConsumers(t *testing.T, rendered string) string {
	t.Helper()
	start := strings.Index(rendered, `module "events" {`)
	if start < 0 {
		t.Fatal(`rendered main.tf has no module "events"`)
	}
	m := eventConsumersLine.FindStringSubmatch(rendered[start:])
	if m == nil {
		t.Fatal(`module "events" has no consumers argument`)
	}
	return m[1]
}

// Subscriptions written before wildcards and filters existed must render the
// consumers argument byte for byte as they did, so generating with the new
// binary changes nothing in main.tf for them.
func TestEventsExistingSubscriptionsRenderUnchanged(t *testing.T) {
	m := qaCapabilityEnv()
	m["event_bus"] = qaCapabilityMap{"enabled": true}
	qaCapabilityOwner(m, "backend")["events"] = qaCapabilityMap{
		"subscribes": qaCapabilityList(qaCapabilityMap{"detail_type": "order.created"}),
	}
	qaCapabilityOwner(m, "service")["events"] = qaCapabilityMap{
		"subscribes": qaCapabilityList(
			qaCapabilityMap{"detail_type": "payment.completed", "source": "example.payments"},
			qaCapabilityMap{"name": "users", "detail_type_prefix": "user.", "bus": "default"},
		),
	}

	got := renderedEventConsumers(t, renderMainHBS(t, m))
	want := `[{"max_receive_count":5,"name":"backend","subscriptions":[{"bus":"project","detail_type":"order.created","detail_type_prefix":"","key":"order-created","source":""}],"visibility_timeout":30},` +
		`{"max_receive_count":5,"name":"api","subscriptions":[{"bus":"project","detail_type":"payment.completed","detail_type_prefix":"","key":"payment-completed","source":"example.payments"},` +
		`{"bus":"default","detail_type":"","detail_type_prefix":"user.","key":"users","source":""}],"visibility_timeout":30}]`
	if got != want {
		t.Errorf("consumers changed for subscriptions without wildcard or filters:\n got: %s\nwant: %s", got, want)
	}
}

func TestEventsWildcardAndFiltersRender(t *testing.T) {
	m := qaCapabilityEnv()
	m["event_bus"] = qaCapabilityMap{"enabled": true}
	qaCapabilityOwner(m, "service")["events"] = qaCapabilityMap{
		"subscribes": qaCapabilityList(
			qaCapabilityMap{"detail_type": "job.completed"},
			qaCapabilityMap{
				"detail_type": "job.*",
				"source":      "example.jobs",
				"filters": qaCapabilityList(
					qaCapabilityMap{"field": "detail.status", "values": []interface{}{"done", "failed"}},
					qaCapabilityMap{"field": "detail.order.type", "values": []interface{}{"retail"}},
				),
			},
		),
	}

	got := renderedEventConsumers(t, renderMainHBS(t, m))
	want := `[{"max_receive_count":5,"name":"api","subscriptions":[{"bus":"project","detail_type":"job.completed","detail_type_prefix":"","key":"job-completed","source":""},` +
		`{"bus":"project","detail_type":"job.*","detail_type_prefix":"","filters":[{"field":"detail.status","values":["done","failed"]},{"field":"detail.order.type","values":["retail"]}],"key":"job","source":"example.jobs"}],"visibility_timeout":30}]`
	if got != want {
		t.Errorf("consumers:\n got: %s\nwant: %s", got, want)
	}
}

func TestEventsWildcardKeyDerivation(t *testing.T) {
	tests := []struct{ exact, bus, want string }{
		{"job.*", "project", "job"},
		{"job.*", "default", "job-default"},
		{"*.created", "project", "created"},
		{"job.*.done", "project", "job-done"},
		{"*", "project", ""},
	}
	for _, tt := range tests {
		t.Run(tt.bus+"/"+tt.exact, func(t *testing.T) {
			qaCapabilityEqual(t, deriveEventSubscriptionKey(tt.exact, "", tt.bus), tt.want)
		})
	}
}

func TestEventsSubscriptionFilterValidation(t *testing.T) {
	filter := func(field string, values ...interface{}) qaCapabilityMap {
		return qaCapabilityMap{"field": field, "values": values}
	}
	sixFilters := qaCapabilityList(filter("detail.a", "x"), filter("detail.b", "x"), filter("detail.c", "x"),
		filter("detail.d", "x"), filter("detail.e", "x"), filter("detail.f", "x"))
	elevenValues := make([]interface{}, 11)
	for i := range elevenValues {
		elevenValues[i] = "v"
	}

	tests := []struct {
		name      string
		entry     qaCapabilityMap
		fragments []string
	}{
		{"star in prefix", qaCapabilityMap{"detail_type_prefix": "job.*"}, []string{"detail_type_prefix", "contains *", `"job.*"`}},
		{"field outside detail", qaCapabilityMap{"detail_type": "job.done", "filters": qaCapabilityList(filter("status", "done"))}, []string{"filters[0].field", "path under detail"}},
		{"bare detail", qaCapabilityMap{"detail_type": "job.done", "filters": qaCapabilityList(filter("detail", "done"))}, []string{"filters[0].field"}},
		{"empty segment", qaCapabilityMap{"detail_type": "job.done", "filters": qaCapabilityList(filter("detail..status", "done"))}, []string{"filters[0].field"}},
		{"no values", qaCapabilityMap{"detail_type": "job.done", "filters": qaCapabilityList(filter("detail.status"))}, []string{"filters[0].values"}},
		{"empty value", qaCapabilityMap{"detail_type": "job.done", "filters": qaCapabilityList(filter("detail.status", ""))}, []string{"filters[0].values"}},
		{"number value", qaCapabilityMap{"detail_type": "job.done", "filters": qaCapabilityList(filter("detail.status", 3))}, []string{"filters[0].values"}},
		{"eleven values", qaCapabilityMap{"detail_type": "job.done", "filters": qaCapabilityList(filter("detail.status", elevenValues...))}, []string{"1 to 10"}},
		{"duplicate field", qaCapabilityMap{"detail_type": "job.done", "filters": qaCapabilityList(filter("detail.status", "a"), filter("detail.status", "b"))}, []string{"filters[1]", "filters[0]", "detail.status"}},
		{"field inside another", qaCapabilityMap{"detail_type": "job.done", "filters": qaCapabilityList(filter("detail.order", "a"), filter("detail.order.type", "b"))}, []string{"detail.order", "detail.order.type"}},
		{"six filters", qaCapabilityMap{"detail_type": "job.done", "filters": sixFilters}, []string{"at most 5"}},
		{"unknown filter key", qaCapabilityMap{"detail_type": "job.done", "filters": qaCapabilityList(qaCapabilityMap{"field": "detail.a", "values": []interface{}{"x"}, "op": "eq"})}, []string{`"op"`}},
		{"filters not a list", qaCapabilityMap{"detail_type": "job.done", "filters": qaCapabilityMap{"field": "detail.a"}}, []string{"filters must be a list"}},
		{"wildcard key collides with exact", nil, []string{"subscribes[1]", "the key \"job-default\"", "distinct name"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := qaCapabilityEnv()
			subs := qaCapabilityList(tt.entry)
			if tt.entry == nil {
				subs = qaCapabilityList(qaCapabilityMap{"detail_type": "job"}, qaCapabilityMap{"detail_type": "job.*"})
			}
			qaCapabilityOwner(m, "service")["events"] = qaCapabilityMap{"subscribes": subs}
			qaCapabilityRequireError(t, normalizeEvents(m), tt.fragments...)
		})
	}
}

func TestEventsSubscriptionFiltersAccepted(t *testing.T) {
	m := qaCapabilityEnv()
	tenValues := make([]interface{}, 10)
	for i := range tenValues {
		tenValues[i] = "v" + string(rune('a'+i))
	}
	qaCapabilityOwner(m, "service")["events"] = qaCapabilityMap{"subscribes": qaCapabilityList(qaCapabilityMap{
		"detail_type": "job.done",
		"filters": qaCapabilityList(
			qaCapabilityMap{"field": "detail.order.type", "values": tenValues},
			qaCapabilityMap{"field": "detail.order-x", "values": []interface{}{"a"}},
			qaCapabilityMap{"field": "detail.order_id", "values": []interface{}{"a"}},
			qaCapabilityMap{"field": "detail.Region", "values": []interface{}{"eu"}},
			qaCapabilityMap{"field": "detail.order.id", "values": []interface{}{"a"}},
		),
	})}
	qaCapabilityNoError(t, normalizeEvents(m))
	sub := qaCapabilityObject(t, qaCapabilityArray(t, qaCapabilityNamed(t, m[eventConsumersKey], "api")["subscriptions"])[0])
	filters := qaCapabilityArray(t, sub["filters"])
	qaCapabilityEqual(t, len(filters), 5)
	qaCapabilityEqual(t, qaCapabilityObject(t, filters[0])["field"], "detail.order.type")
}

func TestEventsMatchEverythingWarns(t *testing.T) {
	m := qaCapabilityEnv()
	qaCapabilityOwner(m, "service")["events"] = qaCapabilityMap{"subscribes": qaCapabilityList(
		qaCapabilityMap{"name": "everything", "detail_type": "*", "bus": "default"},
		qaCapabilityMap{"detail_type": "job.*", "bus": "default"},
	)}
	qaCapabilityNoError(t, normalizeEvents(m))
	warnings := strings.Join(generateWarnings(m), "\n")
	qaCapabilityContains(t, warnings, `detail_type "*"`, "every event on the default bus")
	if strings.Contains(warnings, "subscribes[1]") {
		t.Errorf("a narrowed wildcard must not warn: %s", warnings)
	}

	t.Run("without a name it needs one", func(t *testing.T) {
		m := qaCapabilityEnv()
		qaCapabilityOwner(m, "service")["events"] = qaCapabilityMap{"subscribes": qaCapabilityList(qaCapabilityMap{"detail_type": "*"})}
		qaCapabilityRequireError(t, normalizeEvents(m), "give the entry a name")
	})
}
