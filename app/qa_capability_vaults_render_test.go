package main

import (
	"strings"
	"testing"
)

func TestQACapabilityVaultsRenderConnectionsForEveryKind(t *testing.T) {
	m := qaCapabilityEnv()
	m["vaults"] = qaCapabilityList(qaCapabilityMap{"name": "shared"}, qaCapabilityMap{"name": "payments"})
	for _, kind := range qaCapabilityKinds() {
		qaCapabilityOwner(m, kind)["vaults"] = qaCapabilityList(qaCapabilityMap{"name": "shared"})
	}
	qaCapabilityOwner(m, "service")["vaults"] = qaCapabilityList(
		qaCapabilityMap{"name": "shared", "access": "read"},
		qaCapabilityMap{"name": "payments", "access": "read_write", "prefix": "PAY"},
	)
	rendered := renderMainHBS(t, m)
	body := qaCapabilityParse(t, rendered)
	vaults := qaCapabilityModule(t, body, "vaults")
	connections := qaCapabilityArray(t, qaCapabilityLiteral(t, vaults, "connections"))
	if len(connections) != 5 {
		t.Fatalf("got %d connections, want 5", len(connections))
	}
	byConnection := map[string]qaCapabilityMap{}
	for _, entry := range connections {
		c := qaCapabilityObject(t, entry)
		workload, workloadOK := c["workload"].(string)
		vault, vaultOK := c["vault"].(string)
		if !workloadOK || !vaultOK {
			t.Fatalf("invalid rendered connection: %#v", c)
		}
		key := workload + "/" + vault
		if _, exists := byConnection[key]; exists {
			t.Errorf("duplicate rendered connection %s", key)
		}
		byConnection[key] = c
	}
	for _, kind := range qaCapabilityKinds() {
		c := byConnection[qaCapabilityOwnerName(kind)+"/shared"]
		if c == nil {
			t.Fatalf("missing shared vault for %s", kind)
		}
		if c["access"] != nil {
			qaCapabilityEqual(t, c["access"], "read")
		}
		if c["prefix"] != nil {
			qaCapabilityEqual(t, c["prefix"], "")
		}
	}
	pay := byConnection["api/payments"]
	if pay == nil {
		t.Fatal("missing prefixed payments vault")
	}
	qaCapabilityEqual(t, pay["access"], "read_write")
	qaCapabilityEqual(t, pay["prefix"], "PAY")
	for _, moduleName := range []string{"workloads", "schedule_task_nightly", "event_bus_task_mailer"} {
		expr := qaCapabilityAttr(t, qaCapabilityModule(t, body, moduleName), "extra_secrets").Expr
		qaCapabilityContains(t, qaCapabilityExprText(expr, rendered), "local.workload_extra_secrets")
	}
	serviceExpr := qaCapabilityAttr(t, qaCapabilityModule(t, body, "workloads"), "services").Expr
	if strings.Contains(qaCapabilityExprText(serviceExpr, rendered), "vaults") {
		t.Error("service payload still contains vault connection data")
	}
}
