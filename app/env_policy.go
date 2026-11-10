package main

import (
	"fmt"
	"strings"
)

// envPolicy is what meroku decides from an environment's name. It is the only
// place Terraform-facing behaviour depends on that name: the values are written
// into the YAML, and generated Terraform reads the flags.
type envPolicy struct {
	// IsProd marks the environment as production.
	IsProd bool
	// AutoDeploy lets a new image or config redeploy targets on their own.
	AutoDeploy bool
	// RootDomain serves the environment from the domain itself, with no
	// <env>. prefix.
	RootDomain bool
}

// policyForEnvName returns the defaults for an environment with this name.
func policyForEnvName(name string) envPolicy {
	switch name {
	case "dev":
		return envPolicy{AutoDeploy: true}
	case "prod", "production":
		return envPolicy{IsProd: true, RootDomain: true}
	default:
		return envPolicy{}
	}
}

// domainPrefix returns the add_env_domain_prefix value to write: an explicit
// false for a root-domain environment, and nil to leave the default otherwise.
func (p envPolicy) domainPrefix() *bool {
	if p.RootDomain {
		return boolPtr(false)
	}
	return nil
}

func boolPtr(v bool) *bool { return &v }

// defaultProductionEnvName is the name meroku gives a production environment it
// creates when the project has none.
const defaultProductionEnvName = "prod"

// productionEnvName returns the project's environment flagged is_prod, or the
// name meroku would create one under.
func productionEnvName() string {
	names, _ := listEnvironmentNames()
	for _, name := range names {
		if env, err := loadEnv(name); err == nil && env.IsProd {
			return name
		}
	}
	return defaultProductionEnvName
}

// envZoneName is the hosted zone an environment's domain lives in, as
// env/main.hbs builds it: delegated <env>.<domain> when root_zone_id is set,
// otherwise <env>.<domain> unless add_env_domain_prefix is false.
func envZoneName(env, domain, rootZoneID string, prefix *bool) string {
	base := strings.TrimSuffix(strings.TrimSpace(domain), ".")
	if base == "" {
		return ""
	}
	if rootZoneID != "" || prefix == nil || *prefix {
		return env + "." + base
	}
	return base
}

// migrateToV31 writes the name-based policy into the YAML as flags.
//
// Every environment not named dev has automatic deploys turned off, including
// targets that had them on. Manual deploys are unaffected.
//
// The domain settings apply to "prod" only: the template used to single out
// that exact name, so writing them for any other environment would move its DNS.
func migrateToV31(data map[string]interface{}) error {
	fmt.Println("  → Migrating to v31: Writing name-based policy as flags")

	env, _ := data["env"].(string)
	policy := policyForEnvName(env)

	if policy.IsProd {
		data["is_prod"] = true
	}
	data["auto_deploy"] = policy.AutoDeploy
	fmt.Printf("    ℹ️  env=%q → auto_deploy=%v\n", env, policy.AutoDeploy)

	if !policy.AutoDeploy {
		if turnedOff := disableTargetAutoDeploy(data); turnedOff > 0 {
			fmt.Printf("    ⚠️  Automatic deploys are now OFF for %d target(s) in %q.\n", turnedOff, env)
			fmt.Println("       To turn them back on, set auto_deploy: true and re-apply.")
		}
	}

	if env == "prod" {
		if domain, ok := data["domain"].(map[interface{}]interface{}); ok {
			domain["add_env_domain_prefix"] = false
			delete(domain, "root_zone_id")
		}
	}

	return nil
}

// disableTargetAutoDeploy sets every target that has automatic deploys on to
// off and returns how many it changed.
func disableTargetAutoDeploy(data map[string]interface{}) int {
	changed := 0
	if workload, ok := data["workload"].(map[interface{}]interface{}); ok {
		if workload["backend_auto_deploy"] == true {
			workload["backend_auto_deploy"] = false
			changed++
		}
	}
	for _, key := range []string{"services", "scheduled_tasks"} {
		items, _ := data[key].([]interface{})
		for _, raw := range items {
			item, ok := raw.(map[interface{}]interface{})
			if ok && item["auto_deploy"] == true {
				item["auto_deploy"] = false
				changed++
			}
		}
	}
	return changed
}
