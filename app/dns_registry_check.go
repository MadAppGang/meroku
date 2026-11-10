package main

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/miekg/dns"
)

// Public resolvers cache a delegation for as long as its TTL says, and a TLD
// hands out NS records with a TTL of two days. After the nameservers of a domain
// change, resolvers that saw the old set keep answering with it for up to that
// long, while the registry has published the new set within minutes. Asking the
// parent's own servers reads the delegation itself, with no cache in between.

// errPort53Intercepted means something on this network answers DNS queries
// itself instead of passing them to the server they were sent to. Registries and
// Route53 nameservers never offer recursion, so an answer that offers it came
// from a caching proxy, and is no evidence of what the real server holds.
var errPort53Intercepted = errors.New("DNS on port 53 is intercepted on this network; the real nameservers cannot be asked directly")

// delegationLookup returns the nameservers the parent zone delegates zone to.
// A variable so tests can replace it.
var delegationLookup = delegationWithRDAPFallback

// delegationWithRDAPFallback asks the parent over DNS, and when port 53 is
// intercepted asks the registry over RDAP instead. RDAP covers only a domain
// whose parent is a TLD; a delegated subdomain stays unchecked there.
func delegationWithRDAPFallback(zone string) ([]string, error) {
	ns, err := delegationAtParent(zone)
	if !errors.Is(err, errPort53Intercepted) {
		return ns, err
	}
	if rdapNS, rdapErr := rdapNameservers(zone); rdapErr == nil {
		return rdapNS, nil
	}
	return nil, err
}

// delegationAtParent asks the authoritative servers of zone's parent (the TLD
// registry for a root domain) which nameservers zone is delegated to.
func delegationAtParent(zone string) ([]string, error) {
	zone = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(zone)), ".")
	i := strings.Index(zone, ".")
	if i < 0 {
		return nil, fmt.Errorf("%q has no parent", zone)
	}
	parent := zone[i+1:]

	parentNS, err := net.LookupNS(parent)
	if err != nil || len(parentNS) == 0 {
		return nil, fmt.Errorf("looking up the nameservers of %s: %w", parent, err)
	}

	q := new(dns.Msg)
	q.SetQuestion(dns.Fqdn(zone), dns.TypeNS)
	q.RecursionDesired = false

	var lastErr error
	for _, proto := range []string{"udp", "tcp"} {
		c := &dns.Client{Net: proto, Timeout: 4 * time.Second}
		for _, ns := range parentNS {
			resp, _, err := c.Exchange(q, strings.TrimSuffix(ns.Host, ".")+":53")
			if err != nil {
				lastErr = err
				continue
			}
			if resp.RecursionAvailable {
				return nil, errPort53Intercepted
			}
			if resp.Rcode == dns.RcodeNameError {
				// The parent does not delegate zone at all (yet).
				return nil, nil
			}
			if found := delegatedNameservers(resp, zone); len(found) > 0 {
				return found, nil
			}
		}
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, nil
}

// delegatedNameservers reads the NS records for zone from a referral (authority
// section) or a direct answer.
func delegatedNameservers(resp *dns.Msg, zone string) []string {
	var out []string
	for _, rr := range append(append([]dns.RR{}, resp.Answer...), resp.Ns...) {
		ns, ok := rr.(*dns.NS)
		if !ok || !strings.EqualFold(strings.TrimSuffix(ns.Hdr.Name, "."), zone) {
			continue
		}
		out = append(out, strings.TrimSuffix(ns.Ns, "."))
	}
	return out
}
