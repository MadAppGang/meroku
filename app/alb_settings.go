package main

import (
	"fmt"
	"regexp"
	"strings"
)

// sslPolicyPattern is the shape of an ELB security policy name. It also keeps a
// quote or a newline out of the HCL string main.hbs renders the value into.
var sslPolicyPattern = regexp.MustCompile(`^[A-Za-z0-9-]+$`)

const (
	maxDeregistrationDelay = 3600
	maxGracePeriod         = 2147483647
)

// validateALBSettingsConfigMap checks the optional ALB tuning keys and refuses
// alb.remove_api_gateway wherever something still needs API Gateway. It collects
// every problem and returns them together.
//
// It runs in preprocessEnvMap after filterDisabledItems, so a disabled full_open
// service does not block remove_api_gateway. It is separate from
// validateALBConfigMap, which returns early with the ALB off, and the remove
// check must run exactly then.
//
// ssl_policy, deregistration_delay and the grace period are not refused while the
// ALB is off: modules/workloads ignores them there, and an operator may set them
// before turning the ALB on.
func validateALBSettingsConfigMap(envMap map[string]interface{}) error {
	var problems []string

	alb, _ := yamlSubMap(envMap, "alb")
	workload, _ := yamlSubMap(envMap, "workload")

	if raw, present := alb["ssl_policy"]; present && raw != nil {
		policy, isString := raw.(string)
		if !isString || !sslPolicyPattern.MatchString(policy) {
			problems = append(problems, fmt.Sprintf(
				"alb: ssl_policy is %q; it must be an ELB security policy name, e.g. ELBSecurityPolicy-TLS13-1-2-2021-06. Remove the key to keep ELBSecurityPolicy-2016-08",
				fmt.Sprint(raw)))
		}
	}

	if raw, present := alb["deregistration_delay"]; present && raw != nil {
		if n, ok := wholeNumber(raw); !ok || n < 0 || n > maxDeregistrationDelay {
			problems = append(problems, fmt.Sprintf(
				"alb: deregistration_delay is %v; it must be a whole number of seconds from 0 to %d. Remove the key to keep 300",
				raw, maxDeregistrationDelay))
		}
	}

	if raw, present := workload["backend_health_check_grace_period"]; present && raw != nil {
		if n, ok := wholeNumber(raw); !ok || n < 0 || n > maxGracePeriod {
			problems = append(problems, fmt.Sprintf(
				"workload: backend_health_check_grace_period is %v; it must be a whole number of seconds, 0 or more. Remove the key or set 0 for none",
				raw))
		}
	}

	if raw, present := alb["remove_api_gateway"]; present && raw != nil {
		remove, isBool := raw.(bool)
		if !isBool {
			problems = append(problems, fmt.Sprintf(
				"alb: remove_api_gateway is %v; it must be true or false", raw))
		} else if remove {
			problems = append(problems, removeAPIGatewayProblems(envMap, alb)...)
		}
	}

	if len(problems) > 0 {
		return fmt.Errorf("invalid ALB settings:\n  %s", strings.Join(problems, "\n  "))
	}
	return nil
}

// removeAPIGatewayProblems lists what still needs API Gateway when
// alb.remove_api_gateway is true.
func removeAPIGatewayProblems(envMap, alb map[string]interface{}) []string {
	var problems []string

	if enabled, _ := alb["enabled"].(bool); !enabled {
		problems = append(problems,
			"alb: remove_api_gateway needs alb.enabled: true. With the ALB off, API Gateway is the only way in. Turn the ALB on, or remove the key")
	}

	if services, ok := envMap["services"].([]interface{}); ok {
		for i, raw := range services {
			service, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			if access, _ := service["network_access"].(string); access != NetworkAccessFullOpen {
				continue
			}
			name, _ := service["name"].(string)
			if name == "" {
				name = fmt.Sprintf("#%d", i+1)
			}
			problems = append(problems, fmt.Sprintf(
				"service '%s': network_access: full_open always has an API Gateway route, so alb.remove_api_gateway cannot be on. Set network_access: public, which serves it from the ALB",
				name))
		}
	}

	if extensions, ok := yamlSubMap(envMap, "extensions"); ok {
		if topics, ok := extensions["sns_topics"].([]interface{}); ok {
			for i, raw := range topics {
				topic, ok := raw.(map[string]interface{})
				if !ok {
					continue
				}
				if webhooks, _ := topic["webhooks"].([]interface{}); len(webhooks) == 0 {
					continue
				}
				name, _ := topic["name"].(string)
				if name == "" {
					name = fmt.Sprintf("#%d", i+1)
				}
				problems = append(problems, fmt.Sprintf(
					"extensions.sns_topics '%s': its webhooks are delivered to the API Gateway URL, so alb.remove_api_gateway cannot be on. Remove the webhooks, or keep the gateway",
					name))
			}
		}
	}

	return problems
}
