package main

import (
	"fmt"
	"strings"
)

// A `policy` block is the user's own IAM statements for one workload. The same
// shape is accepted on workload (the backend), services[], scheduled_tasks[]
// and event_processor_tasks[]:
//
//	policy:
//	  - actions: ["dynamodb:GetItem"]
//	    resources: ["arn:aws:dynamodb:us-east-1:000000000000:table/example"]
//	  - effect: Deny
//	    actions: ["s3:DeleteObject"]
//	    resources: ["*"]
//	    conditions:
//	      - test: StringNotEquals
//	        variable: aws:RequestedRegion
//	        values: [us-east-1]
//
// Each workload's statements become one customer-managed IAM policy attached to
// its task role. See ai_docs/WORKLOAD_IAM.md.

// servicePoliciesKey is the top-level envMap key the template reads the
// services' statements from, keyed by service name.
const servicePoliciesKey = "_service_policies"

// workloadPolicyEffects are the two values IAM accepts for Effect. IAM compares
// them case-sensitively, so "allow" is refused here rather than by AWS partway
// through an apply.
var workloadPolicyEffects = map[string]bool{"Allow": true, "Deny": true}

// workloadPolicyOwner is one place a policy block can be written.
type workloadPolicyOwner struct {
	subject string // how the message names the owner: "workload", "services 'api'"
	raw     interface{}
}

// validateWorkloadPolicyConfigMap refuses a policy block IAM or the Terraform
// modules would reject, naming the workload, the statement and the field.
//
// Errors: a policy that is not a list, a statement that is not a map, empty
// actions or resources, an effect other than Allow or Deny, a condition missing
// its test, variable or values, and two conditions in one statement with the
// same test and variable (IAM keeps only one of them).
//
// Warnings, printed and not returned: an action of "*" or "*:*", which grants
// every action on the listed resources.
func validateWorkloadPolicyConfigMap(envMap map[string]interface{}) error {
	var problems, warnings []string
	for _, owner := range workloadPolicyOwners(envMap) {
		p, w := workloadPolicyProblems(owner.subject, owner.raw)
		problems = append(problems, p...)
		warnings = append(warnings, w...)
	}

	for _, w := range warnings {
		fmt.Printf("⚠️  %s\n", w)
	}
	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf("invalid policy configuration:\n  %s", strings.Join(problems, "\n  "))
}

// workloadPolicyOwners lists every policy block in the environment, in YAML
// order. Items without a policy key are skipped.
func workloadPolicyOwners(envMap map[string]interface{}) []workloadPolicyOwner {
	var owners []workloadPolicyOwner

	if workload, ok := envMap["workload"].(map[string]interface{}); ok {
		if raw, exists := workload["policy"]; exists {
			owners = append(owners, workloadPolicyOwner{subject: "workload", raw: raw})
		}
	}

	for _, list := range []string{"services", "scheduled_tasks", "event_processor_tasks"} {
		items, ok := envMap[list].([]interface{})
		if !ok {
			continue
		}
		for i, rawItem := range items {
			item, ok := rawItem.(map[string]interface{})
			if !ok {
				continue
			}
			raw, exists := item["policy"]
			if !exists {
				continue
			}
			name, _ := item["name"].(string)
			if name == "" {
				name = fmt.Sprintf("#%d", i+1)
			}
			owners = append(owners, workloadPolicyOwner{
				subject: fmt.Sprintf("%s '%s'", list, name),
				raw:     raw,
			})
		}
	}
	return owners
}

// workloadPolicyProblems checks one policy block. A nil block (`policy:` with
// nothing after it) means no statements and is accepted.
func workloadPolicyProblems(subject string, raw interface{}) (problems, warnings []string) {
	if raw == nil {
		return nil, nil
	}
	statements, ok := raw.([]interface{})
	if !ok {
		return []string{fmt.Sprintf("%s: policy must be a list of statements", subject)}, nil
	}

	for i, rawStatement := range statements {
		where := fmt.Sprintf("%s: policy[%d]", subject, i)
		statement, ok := rawStatement.(map[string]interface{})
		if !ok {
			problems = append(problems, fmt.Sprintf("%s must be a map with actions and resources", where))
			continue
		}

		if effect, exists := statement["effect"]; exists && effect != nil {
			s, isString := effect.(string)
			if !isString || !workloadPolicyEffects[s] {
				problems = append(problems, fmt.Sprintf(
					"%s.effect is %s; use Allow or Deny, or remove the key for Allow", where, formatYAMLValue(effect)))
			}
		}

		actions, problem := workloadPolicyStrings(where, "actions", statement["actions"])
		if problem != "" {
			problems = append(problems, problem)
		}
		for _, action := range actions {
			if action == "*" || action == "*:*" {
				warnings = append(warnings, fmt.Sprintf(
					"%s.actions contains %q, which allows every action on the listed resources", where, action))
			}
		}
		if _, problem := workloadPolicyStrings(where, "resources", statement["resources"]); problem != "" {
			problems = append(problems, problem)
		}

		problems = append(problems, workloadPolicyConditionProblems(where, statement["conditions"])...)
	}
	return problems, warnings
}

// workloadPolicyStrings checks that a statement field is a non-empty list of
// non-empty strings and returns it.
func workloadPolicyStrings(where, field string, raw interface{}) ([]string, string) {
	list, ok := raw.([]interface{})
	if !ok || len(list) == 0 {
		return nil, fmt.Sprintf("%s.%s must be a non-empty list", where, field)
	}
	out := make([]string, 0, len(list))
	for j, item := range list {
		s, isString := item.(string)
		if !isString || strings.TrimSpace(s) == "" {
			return nil, fmt.Sprintf("%s.%s[%d] must be a non-empty string, got %s",
				where, field, j, formatYAMLValue(item))
		}
		out = append(out, s)
	}
	return out, ""
}

// workloadPolicyConditionProblems checks a statement's conditions. Each one
// needs a test, a variable and at least one value. IAM groups conditions by
// test and then by variable, so two conditions with the same pair would leave
// only one of them in the deployed document.
func workloadPolicyConditionProblems(where string, raw interface{}) []string {
	if raw == nil {
		return nil
	}
	conditions, ok := raw.([]interface{})
	if !ok {
		return []string{fmt.Sprintf("%s.conditions must be a list", where)}
	}

	var problems []string
	seen := make(map[string]int, len(conditions))
	for k, rawCondition := range conditions {
		at := fmt.Sprintf("%s.conditions[%d]", where, k)
		condition, ok := rawCondition.(map[string]interface{})
		if !ok {
			problems = append(problems, fmt.Sprintf("%s must be a map with test, variable and values", at))
			continue
		}

		test, _ := condition["test"].(string)
		variable, _ := condition["variable"].(string)
		var missing []string
		if strings.TrimSpace(test) == "" {
			missing = append(missing, "test")
		}
		if strings.TrimSpace(variable) == "" {
			missing = append(missing, "variable")
		}
		if _, problem := workloadPolicyStrings(at, "values", condition["values"]); problem != "" {
			missing = append(missing, "values")
		}
		if len(missing) > 0 {
			problems = append(problems, fmt.Sprintf(
				"%s is incomplete: %s must be set (values as a non-empty list of strings)",
				at, strings.Join(missing, ", ")))
			continue
		}

		pair := test + "\x00" + variable
		if first, dup := seen[pair]; dup {
			problems = append(problems, fmt.Sprintf(
				"%s repeats test %s on variable %s from conditions[%d]; put every value in one condition",
				at, test, variable, first))
			continue
		}
		seen[pair] = k
	}
	return problems
}

// moveServicePoliciesOut moves each service's policy into the top-level
// _service_policies map, keyed by service name, and deletes it from the
// service map.
//
// The services list renders through {{{array services}}}, which does not
// escape HCL template openers, and HCL parses a quoted string's template
// before object conversion drops an undeclared attribute. A resource such as
// arn:aws:s3:::example/${aws:username}/* left in the service map would be an
// interpolation in the generated main.tf. _service_policies renders through
// hclArray instead, which escapes it.
//
// A service with an empty policy gets no entry. Running it twice on one map
// keeps what the first run moved. It writes to the in-memory map only, never to
// the user's YAML.
func moveServicePoliciesOut(envMap map[string]interface{}) {
	policies, ok := envMap[servicePoliciesKey].(map[string]interface{})
	if !ok {
		policies = map[string]interface{}{}
	}
	if services, ok := envMap["services"].([]interface{}); ok {
		for _, raw := range services {
			service, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			policy, exists := service["policy"]
			if !exists {
				continue
			}
			delete(service, "policy")
			name, _ := service["name"].(string)
			if statements, ok := policy.([]interface{}); ok && len(statements) > 0 && name != "" {
				policies[name] = statements
			}
		}
	}
	envMap[servicePoliciesKey] = policies
}
