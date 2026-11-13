package main

import "testing"

func qaCapabilityPolicyStatement() qaCapabilityMap {
	return qaCapabilityMap{
		"actions":   []interface{}{"s3:GetObject"},
		"resources": []interface{}{"arn:aws:s3:::myapp-data/*"},
	}
}

func TestQACapabilityPolicyRejectsInvalidStatements(t *testing.T) {
	tests := []struct {
		name, field string
		policy      func() interface{}
	}{
		{"policy is not a list", "policy", func() interface{} { return "s3:GetObject" }},
		{"statement is not an object", "policy", func() interface{} { return []interface{}{"s3:GetObject"} }},
		{"actions absent", "actions", func() interface{} {
			s := qaCapabilityPolicyStatement()
			delete(s, "actions")
			return qaCapabilityList(s)
		}},
		{"actions empty", "actions", func() interface{} {
			s := qaCapabilityPolicyStatement()
			s["actions"] = []interface{}{}
			return qaCapabilityList(s)
		}},
		{"blank action", "actions", func() interface{} {
			s := qaCapabilityPolicyStatement()
			s["actions"] = []interface{}{""}
			return qaCapabilityList(s)
		}},
		{"resources empty", "resources", func() interface{} {
			s := qaCapabilityPolicyStatement()
			s["resources"] = []interface{}{}
			return qaCapabilityList(s)
		}},
		{"resources absent", "resources", func() interface{} {
			s := qaCapabilityPolicyStatement()
			delete(s, "resources")
			return qaCapabilityList(s)
		}},
		{"later invalid statement", "resources", func() interface{} {
			s := qaCapabilityPolicyStatement()
			s["resources"] = []interface{}{}
			return qaCapabilityList(qaCapabilityPolicyStatement(), s)
		}},
		{"invalid effect", "effect", func() interface{} {
			s := qaCapabilityPolicyStatement()
			s["effect"] = "deny"
			return qaCapabilityList(s)
		}},
		{"condition test absent", "test", func() interface{} {
			s := qaCapabilityPolicyStatement()
			s["conditions"] = qaCapabilityList(qaCapabilityMap{"variable": "aws:RequestedRegion", "values": []interface{}{"us-east-1"}})
			return qaCapabilityList(s)
		}},
		{"condition variable absent", "variable", func() interface{} {
			s := qaCapabilityPolicyStatement()
			s["conditions"] = qaCapabilityList(qaCapabilityMap{"test": "StringEquals", "values": []interface{}{"us-east-1"}})
			return qaCapabilityList(s)
		}},
		{"condition values empty", "values", func() interface{} {
			s := qaCapabilityPolicyStatement()
			s["conditions"] = qaCapabilityList(qaCapabilityMap{"test": "StringEquals", "variable": "aws:RequestedRegion", "values": []interface{}{}})
			return qaCapabilityList(s)
		}},
		{"duplicate condition pair", "aws:RequestedRegion", func() interface{} {
			s := qaCapabilityPolicyStatement()
			s["conditions"] = qaCapabilityList(
				qaCapabilityMap{"test": "StringEquals", "variable": "aws:RequestedRegion", "values": []interface{}{"us-east-1"}},
				qaCapabilityMap{"test": "StringEquals", "variable": "aws:RequestedRegion", "values": []interface{}{"us-west-2"}},
			)
			return qaCapabilityList(s)
		}},
	}
	for _, kind := range qaCapabilityKinds() {
		for _, tt := range tests {
			t.Run(kind+"/"+tt.name, func(t *testing.T) {
				m := qaCapabilityEnv()
				qaCapabilityOwner(m, kind)["policy"] = tt.policy()
				subject := qaCapabilityOwnerName(kind)
				if kind == "backend" {
					subject = "workload"
				}
				qaCapabilityRequireError(t, validateWorkloadPolicyConfigMap(m), subject, tt.field)
			})
		}
	}
}

func TestQACapabilityPolicyAcceptsValidStatementsAndWarnsForBroadActions(t *testing.T) {
	for _, kind := range qaCapabilityKinds() {
		for _, empty := range []interface{}{nil, []interface{}{}} {
			t.Run(kind+"/empty", func(t *testing.T) {
				m := qaCapabilityEnv()
				qaCapabilityOwner(m, kind)["policy"] = empty
				qaCapabilityNoError(t, validateWorkloadPolicyConfigMap(m))
			})
		}
		for _, action := range []string{"s3:GetObject", "*", "*:*"} {
			t.Run(kind+"/"+action, func(t *testing.T) {
				m := qaCapabilityEnv()
				statement := qaCapabilityPolicyStatement()
				statement["actions"] = []interface{}{action}
				statement["effect"] = "Deny"
				statement["conditions"] = qaCapabilityList(
					qaCapabilityMap{"test": "StringEquals", "variable": "aws:RequestedRegion", "values": []interface{}{"us-east-1"}},
					qaCapabilityMap{"test": "StringEquals", "variable": "aws:PrincipalTag/team", "values": []interface{}{"payments"}},
				)
				qaCapabilityOwner(m, kind)["policy"] = qaCapabilityList(statement)
				err, warnings := qaCapabilityDiagnostics(t, m, func() error { return validateWorkloadPolicyConfigMap(m) })
				qaCapabilityNoError(t, err)
				if action != "s3:GetObject" {
					subject := qaCapabilityOwnerName(kind)
					if kind == "backend" {
						subject = "workload"
					}
					qaCapabilityContains(t, warnings, action, subject)
				}
			})
		}
	}
}
