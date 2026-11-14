package main

import (
	"strings"
	"testing"
)

func TestQACapabilityPolicyRendersLiteralConditionsForEveryKind(t *testing.T) {
	m := qaCapabilityEnv()
	for _, kind := range qaCapabilityKinds() {
		qaCapabilityOwner(m, kind)["policy"] = qaCapabilityList(
			qaCapabilityMap{
				"effect": "Deny", "actions": []interface{}{"s3:DeleteObject"},
				"resources": []interface{}{"arn:aws:s3:::myapp-data/${aws:PrincipalTag/team}/*"},
				"conditions": qaCapabilityList(
					qaCapabilityMap{"test": "StringEquals", "variable": "aws:PrincipalTag/team", "values": []interface{}{"${aws:PrincipalTag/team}"}},
					qaCapabilityMap{"test": "StringEquals", "variable": "aws:RequestedRegion", "values": []interface{}{"us-east-1", "literal-%{team}"}},
				),
			},
		)
	}
	m["vaults"] = qaCapabilityList(qaCapabilityMap{"name": "shared"})
	service := qaCapabilityOwner(m, "service")
	service["vaults"] = qaCapabilityList(qaCapabilityMap{"name": "shared", "prefix": "PAY"})
	service["events"] = qaCapabilityMap{
		"produces": qaCapabilityList(qaCapabilityMap{"detail_types": []interface{}{"order.created"}}),
	}
	rendered := renderMainHBS(t, m)
	body := qaCapabilityParse(t, rendered)
	workloads := qaCapabilityModule(t, body, "workloads")
	servicePolicies := qaCapabilityObject(t, qaCapabilityLiteral(t, workloads, "service_policies"))
	policies := map[string]interface{}{
		"backend":        qaCapabilityLiteral(t, workloads, "backend_policy"),
		"service":        servicePolicies["api"],
		"scheduled_task": qaCapabilityLiteral(t, qaCapabilityModule(t, body, "schedule_task_nightly"), "custom_policy"),
		"event_task":     qaCapabilityLiteral(t, qaCapabilityModule(t, body, "event_bus_task_mailer"), "custom_policy"),
	}
	for kind, policy := range policies {
		t.Run(kind, func(t *testing.T) {
			qaCapabilityWantJSON(t, policy, `[{
				"effect":"Deny","actions":["s3:DeleteObject"],
				"resources":["arn:aws:s3:::myapp-data/${aws:PrincipalTag/team}/*"],
				"conditions":[
					{"test":"StringEquals","variable":"aws:PrincipalTag/team","values":["${aws:PrincipalTag/team}"]},
					{"test":"StringEquals","variable":"aws:RequestedRegion","values":["us-east-1","literal-%{team}"]}
				]
			}]`)
		})
	}
	for _, pair := range []struct{ module, input string }{
		{"workloads", "backend_policy"}, {"workloads", "service_policies"},
		{"schedule_task_nightly", "custom_policy"}, {"event_bus_task_mailer", "custom_policy"},
	} {
		attr := qaCapabilityAttr(t, qaCapabilityModule(t, body, pair.module), pair.input)
		source := qaCapabilityExprText(attr.Expr, rendered)
		qaCapabilityContains(t, source, "$${aws:PrincipalTag/team}", "%%{team}")
	}
	services := qaCapabilityExprText(qaCapabilityAttr(t, workloads, "services").Expr, rendered)
	for _, forbidden := range []string{"${", "%{", "policy", "events", "vaults"} {
		if strings.Contains(services, forbidden) {
			t.Errorf("services payload contains %q:\n%s", forbidden, services)
		}
	}
}
