package main

import "fmt"

// sesPrimaryDomainKey is the envMap key env/main.hbs reads for the SES domain
// that a pre-v17 state holds at unkeyed addresses.
const sesPrimaryDomainKey = "ses_primary_domain"

// assignSESPrimaryDomain resolves the one SES domain a pre-Schema-v17 state can
// hold and stores it in envMap for env/main.hbs.
//
// Before v17 the ses module took a single `domain`, so its resources sat at
// unkeyed addresses (`aws_ses_domain_identity.domain`). The module now keys
// every resource by domain name, and the template renders `moved` blocks from
// the old addresses to `["<this domain>"]`. The key has to be the exact string
// the module receives, so the template's legacy `domain =` line renders this
// value too: one derivation, not a Go copy of a Handlebars one.
//
// Resolution, in the template's own branch order:
//   - ses.domains set: its first entry. migrateToV17 turns the legacy
//     domain_name into a one-element list, so the domain the state was built
//     for is the first one unless someone reordered the list since.
//   - ses.domain_name set: that name.
//   - otherwise the name the template has always derived:
//     mail.<domain> in prod, mail.<env>.<domain> elsewhere.
//
// It writes the in-memory map only, never the user's YAML.
func assignSESPrimaryDomain(envMap map[string]interface{}) {
	ses, ok := computeNodeMap(envMap["ses"])
	if !ok {
		return
	}

	if domains, ok := ses["domains"].([]interface{}); ok && len(domains) > 0 {
		if first, ok := computeNodeMap(domains[0]); ok {
			envMap[sesPrimaryDomainKey] = computeStringField(first, "domain")
		}
		return
	}

	if name := computeStringField(ses, "domain_name"); name != "" {
		envMap[sesPrimaryDomainKey] = name
		return
	}

	var root string
	if domain, ok := computeNodeMap(envMap["domain"]); ok {
		root = computeStringField(domain, "domain_name")
	}
	env := fmt.Sprintf("%v", envMap["env"])
	if env == "prod" {
		envMap[sesPrimaryDomainKey] = "mail." + root
	} else {
		envMap[sesPrimaryDomainKey] = "mail." + env + "." + root
	}
}
