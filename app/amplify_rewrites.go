package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Amplify custom rewrite rules (amplify_apps[].custom_rules).
//
// A rule either rewrites to a fixed `target` (a path or a URL), or names a
// `workload` — `backend` or a service — and meroku fills in the workload's
// public base URL. A workload rule is a reverse proxy: Amplify answers the
// browser on the app's own host and fetches the response from the API, so the
// frontend can call `/api/...` without CORS.
//
// The base URL depends on the ingress mode:
//
//	alb.enabled: true                         https://<api domain>
//	alb.enabled: false, domain + api prefix   https://<api domain>
//	alb.enabled: false, otherwise             the execute-api endpoint with its stage
//
// The ALB's alias record and certificate always exist under the API domain, and
// its own ELB hostname never matches the certificate, so ALB mode needs a
// domain. In API Gateway mode the API domain has a DNS record only when
// api_domain_prefix is set.
//
// The target path is joined here, not in Terraform: preprocessing stores it as
// `_target_path` on each workload rule, and the module only concatenates
// `workload_urls[workload] + target_path`.

const (
	amplifyRulesKey      = "custom_rules"
	amplifyTargetPathKey = "_target_path"
	// amplifyWorkloadRuleKey tells the template which form a rule has.
	amplifyWorkloadRuleKey = "_workload_rule"
	// amplifyRuleIndexKey is the rule's index in the YAML, kept for messages
	// after rules naming switched-off workloads are left out.
	amplifyRuleIndexKey  = "_rule_index"
	amplifyWildcard      = "<*>"
	amplifyCatchAll      = "/<*>"
	amplifyStatusRewrite = "200"

	// amplifyWorkloadURLsKey lists the workloads named by any rule, each with
	// the path appended to the base URL: "" for the backend, "/<service>" for a
	// service, which is where both ingresses route a service's paths.
	amplifyWorkloadURLsKey = "_amplify_workload_urls"
	// amplifyBaseViaDomainKey is true when the base URL is the API domain and
	// false when it is the API Gateway execute-api endpoint.
	amplifyBaseViaDomainKey = "_amplify_base_via_domain"
)

// amplifyRuleStatuses are the statuses a rule may use. 404-200 serves the
// target with a 200 only when the source path does not exist.
var amplifyRuleStatuses = map[string]bool{"200": true, "301": true, "302": true, "404-200": true}

// prepareAmplifyRewriteRules validates every amplify_apps[].custom_rules entry
// and, when they are valid, normalises them for the template: `status` becomes
// a string (default 200), workload rules gain `_target_path`, and the root map
// gains the list of workload URLs to render and how the base URL is built.
//
// A rule naming a switched-off workload (enabled: false) is dropped with a
// warning; a rule naming no workload at all is an error. Both are resolved
// against the names validateWorkloadNamesConfigMap records, so this runs after
// it.
func prepareAmplifyRewriteRules(envMap map[string]interface{}) error {
	apps, problems := amplifyAppMaps(envMap)
	if len(apps) == 0 && len(problems) == 0 {
		return nil
	}

	services := amplifyPublicServices(envMap)
	viaDomain, apiHost := amplifyAPIBase(envMap)
	albEnabled := amplifyALBEnabled(envMap)
	domainEnabled := amplifyDomainEnabled(envMap)

	used := map[string]bool{}

	for _, app := range apps {
		appName := computeStringField(app, "name")
		amplifyIsolateBranchEnv(app)

		rules, ruleProblems := amplifyRuleMaps(app, appName)
		problems = append(problems, ruleProblems...)
		kept := make([]interface{}, 0, len(rules))

		for _, entry := range rules {
			i, rule := entry.index, entry.rule
			where := fmt.Sprintf("amplify_apps '%s' custom_rules[%d]", appName, i)
			source := computeStringField(rule, "source")
			target := computeStringField(rule, "target")
			workload := computeStringField(rule, "workload")
			path := computeStringField(rule, "path")
			status, statusOK := amplifyRuleStatus(rule["status"])
			rule[amplifyRuleIndexKey] = i

			if source == "" {
				problems = append(problems, where+": source is required")
			} else if p := amplifyWildcardProblem(where, "source", source); p != "" {
				problems = append(problems, p)
			}
			for _, field := range []struct{ name, value string }{{"target", target}, {"path", path}} {
				if !strings.Contains(field.value, amplifyWildcard) {
					continue
				}
				if p := amplifyWildcardProblem(where, field.name, field.value); p != "" {
					problems = append(problems, p)
				} else if !strings.Contains(source, amplifyWildcard) {
					problems = append(problems, fmt.Sprintf(
						"%s: %s %q uses %s, but source %q has none for it to stand for", where, field.name, field.value, amplifyWildcard, source))
				}
			}

			switch {
			case target == "" && workload == "":
				problems = append(problems, where+": set either target or workload")
			case target != "" && workload != "":
				problems = append(problems, where+": set target or workload, not both")
			}

			if !statusOK {
				problems = append(problems, fmt.Sprintf(
					"%s: status %v is not one of 200, 301, 302, 404-200", where, rule["status"]))
			}

			if path != "" && workload == "" {
				problems = append(problems, where+": path applies only to a workload rule; put the full destination in target")
			}

			// Always set, true or false: inside {{#each}} raymond resolves a key
			// the rule lacks against the parent context, and the root map has a
			// `workload` key of its own.
			rule[amplifyWorkloadRuleKey] = workload != ""
			if workload == "" {
				rule["status"] = status
				kept = append(kept, rule)
				continue
			}

			if workload != backendWorkloadName {
				switch resolveWorkloadReference(envMap, workload) {
				case workloadReferenceDisabled:
					addGenerateWarning(envMap, fmt.Sprintf(
						"%s names workload '%s', which is switched off (enabled: false); the rule is left out until it is switched on", where, workload))
					continue
				case workloadReferenceUnknown:
					problems = append(problems, fmt.Sprintf(
						"%s: workload '%s' is not a workload; use backend or the name of a service with network_access public or full_open", where, workload))
					continue
				}
				if _, ok := services[workload]; !ok {
					problems = append(problems, fmt.Sprintf(
						"%s: workload '%s' must be backend or a service with network_access public or full_open", where, workload))
				}
			}

			if statusOK && status != amplifyStatusRewrite {
				problems = append(problems, fmt.Sprintf(
					"%s: a workload rule proxies the request and must use status 200, not %s", where, status))
			}
			if path != "" && !strings.HasPrefix(path, "/") {
				problems = append(problems, fmt.Sprintf("%s: path %q must start with /", where, path))
			}
			if path == "" && source != "" && !strings.HasPrefix(source, "/") {
				problems = append(problems, fmt.Sprintf(
					"%s: source %q is not a path, so set path to the destination path on the workload", where, source))
			}
			if albEnabled && !domainEnabled {
				problems = append(problems, where+": a workload rule with alb.enabled needs domain.enabled, because the ALB has no hostname its certificate matches without a domain")
			}
			if viaDomain && apiHost != "" {
				for _, other := range apps {
					if host := amplifyAppHost(envMap, other); host != "" && strings.EqualFold(host, apiHost) {
						problems = append(problems, fmt.Sprintf(
							"%s: the API host %s is also the host of amplify_apps '%s', so the rewrite would send the app's requests back to itself; give the API its own domain.api_domain_prefix",
							where, apiHost, computeStringField(other, "name")))
					}
				}
			}

			rule["status"] = amplifyStatusRewrite
			rule[amplifyTargetPathKey] = amplifyTargetPath(workload, source, path)
			used[workload] = true
			kept = append(kept, rule)
		}
		if _, has := app[amplifyRulesKey]; has {
			app[amplifyRulesKey] = kept
		}
	}

	if len(problems) > 0 {
		return fmt.Errorf("amplify custom_rules:\n  %s", strings.Join(problems, "\n  "))
	}

	if len(used) == 0 {
		return nil
	}
	names := make([]string, 0, len(used))
	for name := range used {
		names = append(names, name)
	}
	sort.Strings(names)
	urls := make([]interface{}, 0, len(names))
	for _, name := range names {
		suffix := ""
		if name != "backend" {
			suffix = "/" + name
		}
		urls = append(urls, map[string]interface{}{"name": name, "path": suffix})
	}
	envMap[amplifyWorkloadURLsKey] = urls
	envMap[amplifyBaseViaDomainKey] = viaDomain
	return nil
}

// amplifyRewriteWarnings names every rule that follows a `/<*>` catch-all in
// the same app. Amplify applies the first rule that matches, so such a rule
// never runs.
func amplifyRewriteWarnings(envMap map[string]interface{}) []string {
	var warnings []string
	apps, _ := amplifyAppMaps(envMap)
	for _, app := range apps {
		appName := computeStringField(app, "name")
		catchAll := -1
		rules, _ := amplifyRuleMaps(app, appName)
		for _, entry := range rules {
			// The index the user wrote. Once prepareAmplifyRewriteRules has left
			// out a rule naming a switched-off workload, the stored list is
			// shorter, so the index it recorded on each rule is the one to use.
			i, rule := entry.index, entry.rule
			if written, ok := rule[amplifyRuleIndexKey].(int); ok {
				i = written
			}
			if catchAll >= 0 {
				warnings = append(warnings, fmt.Sprintf(
					"amplify_apps '%s' custom_rules[%d] comes after the catch-all custom_rules[%d] (%s) and never matches; move it above",
					appName, i, catchAll, amplifyCatchAll))
				continue
			}
			if computeStringField(rule, "source") == amplifyCatchAll {
				catchAll = i
			}
		}
	}
	return warnings
}

// amplifyTargetPath is the path appended to a workload's base URL.
//
// An explicit path wins. The backend's base URL is the API root, so the source
// path is kept as written. A service's base URL already ends in /<service>, so
// a source that starts with that segment drops it rather than doubling it:
// /reports/<*> on service reports becomes <base>/reports/<*>.
func amplifyTargetPath(workload, source, path string) string {
	if path != "" {
		return path
	}
	if workload == "backend" {
		return source
	}
	prefix := "/" + workload
	if source == prefix {
		return ""
	}
	if strings.HasPrefix(source, prefix+"/") {
		return strings.TrimPrefix(source, prefix)
	}
	return source
}

// amplifyRuleStatus reads a rule's status as YAML gives it back — a string or,
// for an unquoted 301, a number — and defaults an absent status to 200.
func amplifyRuleStatus(raw interface{}) (string, bool) {
	var s string
	switch v := raw.(type) {
	case nil:
		return amplifyStatusRewrite, true
	case string:
		s = strings.TrimSpace(v)
		if s == "" {
			return amplifyStatusRewrite, true
		}
	case int:
		s = strconv.Itoa(v)
	case int64:
		s = strconv.FormatInt(v, 10)
	case float64:
		if v != float64(int64(v)) {
			return "", false
		}
		s = strconv.FormatInt(int64(v), 10)
	default:
		return "", false
	}
	return s, amplifyRuleStatuses[s]
}

// amplifyAPIBase reports whether the rewrite base URL is the API domain, and
// that domain's host. False means the API Gateway execute-api endpoint, whose
// host is never an Amplify app's.
func amplifyAPIBase(envMap map[string]interface{}) (bool, string) {
	if !amplifyDomainEnabled(envMap) {
		return false, ""
	}
	domain, _ := yamlSubMap(envMap, "domain")
	prefix := computeStringField(domain, "api_domain_prefix")
	if !amplifyALBEnabled(envMap) && prefix == "" {
		return false, ""
	}
	envDomain := amplifyEnvDomain(envMap)
	if envDomain == "" {
		return true, ""
	}
	if prefix == "" {
		return true, envDomain
	}
	return true, prefix + "." + envDomain
}

// amplifyEnvDomain mirrors module "domain" in env/main.hbs: a delegated
// subdomain is always <env>.<domain>; otherwise add_env_domain_prefix, which
// defaults to true, decides.
func amplifyEnvDomain(envMap map[string]interface{}) string {
	domain, _ := yamlSubMap(envMap, "domain")
	name := computeStringField(domain, "domain_name")
	if name == "" {
		return ""
	}
	env := stringFromMap(envMap, "env")
	if computeStringField(domain, "root_zone_id") != "" || amplifyAddEnvPrefix(domain) {
		return env + "." + name
	}
	return name
}

// amplifyAppHost mirrors local.app_domains in modules/amplify: custom_domain
// wins, then subdomain_prefix under the environment's domain. That domain is
// amplifyEnvDomain's, because env/main.hbs passes module "amplify" the same
// add_env_domain_prefix rule as module "domain": always true for a delegated
// subdomain (root_zone_id). Empty when the app only has its *.amplifyapp.com
// host.
func amplifyAppHost(envMap map[string]interface{}, app map[string]interface{}) string {
	if custom := computeStringField(app, "custom_domain"); custom != "" {
		return strings.TrimSuffix(custom, ".")
	}
	prefix := computeStringField(app, "subdomain_prefix")
	envDomain := amplifyEnvDomain(envMap)
	if prefix == "" || envDomain == "" {
		return ""
	}
	return prefix + "." + envDomain
}

func amplifyAddEnvPrefix(domain map[string]interface{}) bool {
	if v, ok := domain["add_env_domain_prefix"].(bool); ok {
		return v
	}
	return true
}

func amplifyDomainEnabled(envMap map[string]interface{}) bool {
	domain, ok := yamlSubMap(envMap, "domain")
	if !ok {
		return false
	}
	enabled, _ := domain["enabled"].(bool)
	return enabled
}

func amplifyALBEnabled(envMap map[string]interface{}) bool {
	alb, ok := yamlSubMap(envMap, "alb")
	if !ok {
		return false
	}
	enabled, _ := alb["enabled"].(bool)
	return enabled
}

// amplifyPublicServices is the set of services a workload rule may name: those
// whose paths the API Gateway or ALB routes, network_access public (the
// default) or full_open.
func amplifyPublicServices(envMap map[string]interface{}) map[string]struct{} {
	out := map[string]struct{}{}
	list, _ := envMap["services"].([]interface{})
	for _, raw := range list {
		svc, ok := computeNodeMap(raw)
		if !ok {
			continue
		}
		switch computeStringField(svc, "network_access") {
		case "", NetworkAccessPublic, NetworkAccessFullOpen:
			if name := computeStringField(svc, "name"); name != "" {
				out[name] = struct{}{}
			}
		}
	}
	return out
}

// amplifyAppMaps returns amplify_apps as the string-keyed maps stored in
// envMap. yaml.v2 decodes a mapping as map[interface{}]interface{}; such an
// entry is replaced in the list by its string-keyed copy, so fields written to
// it later reach the template. An entry that is not a mapping is a problem.
func amplifyAppMaps(envMap map[string]interface{}) ([]map[string]interface{}, []string) {
	raw, present := envMap["amplify_apps"]
	if !present || raw == nil {
		return nil, nil
	}
	list, ok := raw.([]interface{})
	if !ok {
		return nil, []string{fmt.Sprintf("amplify_apps must be a list of apps, got %s", formatYAMLValue(raw))}
	}
	var problems []string
	out := make([]map[string]interface{}, 0, len(list))
	for i, item := range list {
		app, ok := computeNodeMap(item)
		if !ok {
			problems = append(problems, fmt.Sprintf(
				"amplify_apps[%d] must be a mapping with name, github_repository and branches, got %s", i, formatYAMLValue(item)))
			continue
		}
		list[i] = app
		out = append(out, app)
	}
	return out, problems
}

// amplifyRuleEntry is one custom_rules mapping and its index in the list it
// was read from, so a message names the entry the user wrote even when an
// earlier entry was not a mapping and is missing from the result.
type amplifyRuleEntry struct {
	index int
	rule  map[string]interface{}
}

// amplifyRuleMaps returns the app's rules as the string-keyed maps stored in
// envMap, replacing a yaml.v2 map in the list as amplifyAppMaps does, so the
// normalised fields written to them reach the template. A rules value that is
// not a list, and a rule that is not a mapping, are problems.
func amplifyRuleMaps(app map[string]interface{}, appName string) ([]amplifyRuleEntry, []string) {
	raw, present := app[amplifyRulesKey]
	if !present || raw == nil {
		return nil, nil
	}
	list, ok := raw.([]interface{})
	if !ok {
		return nil, []string{fmt.Sprintf(
			"amplify_apps '%s' custom_rules must be a list of rules, got %s", appName, formatYAMLValue(raw))}
	}
	var problems []string
	out := make([]amplifyRuleEntry, 0, len(list))
	for i, item := range list {
		rule, ok := computeNodeMap(item)
		if !ok {
			problems = append(problems, fmt.Sprintf(
				"amplify_apps '%s' custom_rules[%d] must be a mapping with source and target or workload, got %s",
				appName, i, formatYAMLValue(item)))
			continue
		}
		list[i] = rule
		out = append(out, amplifyRuleEntry{index: i, rule: rule})
	}
	return out, problems
}

// amplifyIsolateBranchEnv gives every branch without environment_variables an
// empty map. Inside {{#each branches}} raymond resolves a key the branch lacks
// against the enclosing app, so the app's own environment_variables would
// otherwise render a second time as each such branch's.
func amplifyIsolateBranchEnv(app map[string]interface{}) {
	branches, _ := app["branches"].([]interface{})
	for i, item := range branches {
		branch, ok := computeNodeMap(item)
		if !ok {
			continue
		}
		if branch["environment_variables"] == nil {
			branch["environment_variables"] = map[string]interface{}{}
		}
		branches[i] = branch
	}
}

// amplifyWildcardProblem reports a value that uses <*> more than once or
// anywhere but at the end; Amplify matches and substitutes it only there.
func amplifyWildcardProblem(where, field, value string) string {
	n := strings.Count(value, amplifyWildcard)
	if n > 1 || (n == 1 && !strings.HasSuffix(value, amplifyWildcard)) {
		return fmt.Sprintf("%s: %s %q may use %s only once, at the end", where, field, value, amplifyWildcard)
	}
	return ""
}
