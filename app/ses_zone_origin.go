package main

import (
	"fmt"
	"strings"
)

// Zone origin of an SES domain: whether meroku created the Route53 zone its
// records go into.
//
//   - created:  meroku's YAML is the whole truth for every record set it
//     manages in the zone. A value not listed is removed on apply.
//   - existing: the zone was set up outside meroku. meroku only adds its own
//     values to a record set, keeps every other value, and on destroy removes
//     only what it added (modules/ses, merge mode).
//
// A domain with no zone at all has no origin: its records are printed as
// manual instructions and nothing is written to Route53.
const (
	sesZoneCreated  = "created"
	sesZoneExisting = "existing"

	// Keys written onto each ses.domains[] entry of the in-memory map.
	sesZoneOriginKey = "_zone_origin"
	sesZoneExprKey   = "_zone_expr"

	// Top-level keys for the legacy single-domain form and for the
	// migration blocks.
	sesLegacyZoneOriginKey = "ses_legacy_zone_origin"
	sesMergeDomainsKey     = "ses_merge_domains"
	sesMergeAnyKey         = "ses_merge_any"

	sesMergeMigrationHeading = "# SES records in DNS zones meroku did not create"
	sesMergeMigrationEnd     = "# End of SES records in DNS zones meroku did not create\n"
)

// sesZone is where one SES domain's records go.
type sesZone struct {
	// Expr is the HCL expression env/main.hbs prints after `zone_id = `,
	// without its trailing comment. Empty when the domain has no zone.
	Expr string
	// Origin is sesZoneCreated, sesZoneExisting, or "" when there is no zone.
	Origin string
	// ZoneID is the literal zone id when the YAML names one, else "".
	ZoneID string
}

// sesZoneForDomain applies, in the same order, the branches env/main.hbs uses
// to pick the zone_id of a ses.domains[] entry:
//
//  1. the entry's own zone_id;
//  2. a domain.additional_domains[] entry equal to or a parent of the domain,
//     with zone_id (existing) or create_zone (created);
//  3. the main domain module, when domain.enabled and the SES domain is the
//     main domain or a subdomain of it. In prod the zone is the main zone;
//     outside prod it is the delegated <env>.<domain> zone meroku creates
//     when domain.root_zone_id is set, and otherwise the main zone.
//
// The template prints every branch that matches. Two matches render a
// duplicate zone_id that fails at generate; the first one is returned here.
// sesZoneTemplateAgreement in the tests pins this function to the template.
func sesZoneForDomain(envMap map[string]interface{}, sesDomain map[string]interface{}) sesZone {
	name := computeStringField(sesDomain, "domain")
	if id := computeStringField(sesDomain, "zone_id"); id != "" {
		return sesZone{Expr: fmt.Sprintf("%q", id), Origin: sesZoneExisting, ZoneID: id}
	}

	domain, _ := computeNodeMap(envMap["domain"])
	if domain != nil {
		if list, ok := domain["additional_domains"].([]interface{}); ok {
			for i, raw := range list {
				add, ok := computeNodeMap(raw)
				if !ok {
					continue
				}
				addName := computeStringField(add, "domain")
				if !sesIsSubdomainOf(name, addName) {
					continue
				}
				if id := computeStringField(add, "zone_id"); id != "" {
					return sesZone{Expr: fmt.Sprintf("%q", id), Origin: sesZoneExisting, ZoneID: id}
				}
				if isTruthy(add["create_zone"]) {
					return sesZone{Expr: fmt.Sprintf("aws_route53_zone.additional_%d.zone_id", i), Origin: sesZoneCreated}
				}
			}
		}
	}

	if domain == nil || !isTruthy(domain["enabled"]) {
		return sesZone{}
	}
	if !sesIsSubdomainOf(name, computeStringField(domain, "domain_name")) {
		return sesZone{}
	}
	return sesZone{Expr: "module.domain.zone_id", Origin: sesMainZoneOrigin(envMap, domain)}
}

// sesMainZoneOrigin is the origin of module.domain's zone.
func sesMainZoneOrigin(envMap map[string]interface{}, domain map[string]interface{}) string {
	if computeStringField(domain, "root_zone_id") != "" {
		// module.dns_delegation creates <env>.<domain> and module.domain
		// looks it up.
		return sesZoneCreated
	}
	if isTruthy(domain["create_domain_zone"]) {
		return sesZoneCreated
	}
	return sesZoneExisting
}

// sesIsSubdomainOf is the isSubdomainOf template helper.
func sesIsSubdomainOf(domain, parent string) bool {
	domain = strings.TrimSuffix(domain, ".")
	parent = strings.TrimSuffix(parent, ".")
	if domain == "" || parent == "" {
		return false
	}
	return domain == parent || strings.HasSuffix(domain, "."+parent)
}

// resolveSESZones writes each SES domain's zone origin onto the in-memory
// map for env/main.hbs, and the list of domains in existing zones for the
// migration blocks. It never touches the user's YAML.
//
// Runs after assignSESPrimaryDomain, whose ses_primary_domain names the
// domain of the legacy single-domain form.
func resolveSESZones(envMap map[string]interface{}) {
	ses, ok := computeNodeMap(envMap["ses"])
	if !ok {
		return
	}
	var merge []interface{}

	domains, _ := ses["domains"].([]interface{})
	if len(domains) > 0 {
		for _, raw := range domains {
			d, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			z := sesZoneForDomain(envMap, d)
			if z.Origin == "" {
				continue
			}
			d[sesZoneOriginKey] = z.Origin
			d[sesZoneExprKey] = z.Expr
			if z.Origin == sesZoneExisting {
				merge = append(merge, computeStringField(d, "domain"))
			}
		}
	} else if primary, _ := envMap[sesPrimaryDomainKey].(string); primary != "" {
		// The legacy form passes module.domain.zone_id whenever
		// domain.enabled, without the subdomain test.
		if domain, ok := computeNodeMap(envMap["domain"]); ok && isTruthy(domain["enabled"]) {
			origin := sesMainZoneOrigin(envMap, domain)
			envMap[sesLegacyZoneOriginKey] = origin
			if origin == sesZoneExisting {
				merge = append(merge, primary)
			}
		}
	}

	if len(merge) > 0 {
		envMap[sesMergeDomainsKey] = merge
		envMap[sesMergeAnyKey] = true
	}
}
