package main

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
)

// Moving a whole domain into Route53, for the case where the current DNS host
// will not let you add an NS record for a subdomain.
//
// Plenty of registrars and budget DNS panels only offer A/CNAME/MX/TXT and have
// no way to delegate a subdomain at all. The way out is to host the apex zone in
// Route53 too: changing a domain's nameservers is a registrar operation that
// every registrar supports, unlike subdomain delegation. Once the apex is ours,
// writing dev.example.com NS is trivial — it is the same code path as any other
// parent zone we control.
//
// This is the most dangerous thing meroku can do to a domain. Repointing
// nameservers moves *everything* — mail, the marketing site, SSO, verification
// records — to a zone that only contains what we managed to copy across. So the
// design is built around one rule: never claim to know the full record set
// unless a zone transfer actually gave it to us, and never switch nameservers
// without showing what would change.

// dnsRecord is one record set, as discovered or as it will be written.
type dnsRecord struct {
	Name   string // fully qualified, no trailing dot
	Type   string // "A", "MX", "TXT", ...
	TTL    int64
	Values []string
}

func (r dnsRecord) key() string { return strings.ToLower(r.Name) + "/" + r.Type }

// zoneSnapshot is what a domain currently serves, and how confident we are that
// it is the whole picture.
type zoneSnapshot struct {
	Domain  string
	Records []dnsRecord

	// Complete is true only when a zone transfer succeeded. A probe sweep can
	// never prove absence, so anything else is a lower bound on what exists.
	Complete bool

	// Method describes how the records were obtained, for display.
	Method string

	// NamesProbed is how many names the sweep asked about, so the UI can say
	// what was and was not looked at.
	NamesProbed int

	// HasWildcard reports that the zone answers for names that do not exist.
	// Sweeping such a zone would otherwise "find" a record at every name asked
	// about, so what is listed here is only what differs from the wildcard.
	HasWildcard  bool
	WildcardHint string
}

// Warning renders the honesty caveat that must accompany an incomplete sweep.
func (s zoneSnapshot) Warning() string {
	if s.Complete {
		return ""
	}
	return fmt.Sprintf(
		"DNS cannot be listed — only queried. These %d records came from probing %d "+
			"common names against the current nameservers, so anything on an unusual "+
			"name is not here. Check against your DNS provider's own record list "+
			"before switching nameservers.",
		len(s.Records), s.NamesProbed)
}

// probeNames are the names swept when a zone transfer is refused.
//
// Ordered by how loudly their absence would be noticed: mail first, because a
// missing MX or SPF record bounces mail silently and is the failure people
// discover days later; then the web presence; then the long tail.
var probeNames = []string{
	// apex is queried separately as ""
	"www", "mail", "smtp", "imap", "pop", "webmail", "mx", "mx1", "mx2",
	"autodiscover", "autoconfig", "mta-sts",
	"_dmarc", "_domainkey", "default._domainkey", "google._domainkey",
	"selector1._domainkey", "selector2._domainkey", "k1._domainkey",
	"s1._domainkey", "s2._domainkey", "mail._domainkey", "dkim._domainkey",
	"_mta-sts", "_smtp._tls", "_acme-challenge",
	"api", "app", "admin", "portal", "dashboard", "console",
	"blog", "shop", "store", "docs", "help", "support", "status",
	"cdn", "assets", "static", "img", "media", "files", "downloads",
	"dev", "staging", "stage", "test", "demo", "beta", "preview",
	"vpn", "remote", "git", "ci", "jenkins", "grafana", "metrics",
	"m", "mobile", "auth", "login", "sso", "id", "account",
	"ns1", "ns2", "ftp", "cpanel", "webdisk", "whm",
}

// apexTypes are queried at the domain root.
var apexTypes = []uint16{
	dns.TypeA, dns.TypeAAAA, dns.TypeMX, dns.TypeTXT, dns.TypeCAA, dns.TypeSRV,
}

// subTypes are queried at every probed name. NS is included so an existing
// subdomain delegation is carried across rather than silently dropped.
var subTypes = []uint16{
	dns.TypeA, dns.TypeAAAA, dns.TypeCNAME, dns.TypeTXT, dns.TypeMX, dns.TypeNS, dns.TypeSRV,
}

// discoverZoneRecords works out what a domain currently serves.
//
// Queries go to the domain's own authoritative nameservers rather than a
// recursive resolver: a recursive answer is filtered by what happens to be
// cached and hides records with a zero-length TTL, and we want the zone as its
// operator sees it.
func discoverZoneRecords(ctx context.Context, domain string, authNS []string) (zoneSnapshot, error) {
	domain = normalizeDomain(domain)
	if domain == "" {
		return zoneSnapshot{}, fmt.Errorf("no domain given")
	}
	if len(authNS) == 0 {
		return zoneSnapshot{}, fmt.Errorf("no authoritative nameservers known for %s", domain)
	}

	// A zone transfer is the only way to get a provably complete answer. Almost
	// every provider refuses, but it costs one query to find out and turns a
	// best-effort sweep into a certainty when it works.
	if records, err := attemptZoneTransfer(domain, authNS); err == nil && len(records) > 0 {
		return zoneSnapshot{
			Domain:   domain,
			Records:  dedupeRecords(records),
			Complete: true,
			Method:   "zone transfer (AXFR)",
		}, nil
	}

	server := authNS[0]

	// Find out whether the zone answers for names that do not exist before
	// believing anything the sweep returns. Many providers serve a wildcard or a
	// parking page, and then every probe "succeeds": sweeping sploty.app on Hover
	// reported 76 records where only a handful were real, and copying those would
	// have created sixty explicit A records that had never existed — freezing
	// today's wildcard target into place so a later change to it silently stopped
	// applying.
	wildcard := detectWildcard(server, domain)

	var (
		mu      sync.Mutex
		found   []dnsRecord
		wg      sync.WaitGroup
		sem     = make(chan struct{}, 16)
		queries []dnsQuery
	)

	queries = append(queries, buildQueries(domain, "", apexTypes)...)
	for _, n := range probeNames {
		queries = append(queries, buildQueries(domain, n, subTypes)...)
	}

	for _, q := range queries {
		wg.Add(1)
		go func(q dnsQuery) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()

			rec, ok := queryRecord(server, q.name, q.rrtype)
			if !ok {
				return
			}
			// The apex is always real. Below it, an answer identical to what a
			// nonexistent name returns is the wildcard, not a record.
			if q.name != domain && wildcard.matches(q.rrtype, rec.Values) {
				return
			}
			mu.Lock()
			found = append(found, rec)
			mu.Unlock()
		}(q)
	}
	wg.Wait()

	// The wildcard itself is a real record and belongs in the new zone — it is
	// what keeps every name it covers resolving after the switch.
	found = append(found, wildcard.records(domain)...)

	return zoneSnapshot{
		Domain:       domain,
		Records:      dedupeRecords(found),
		Complete:     false,
		Method:       "probe sweep",
		NamesProbed:  len(probeNames) + 1,
		HasWildcard:  wildcard.present,
		WildcardHint: wildcard.describe(domain),
	}, nil
}

// wildcardAnswer records what a zone returns for names that do not exist.
type wildcardAnswer struct {
	present bool
	byType  map[uint16][]string
	ttl     map[uint16]int64
}

func (w wildcardAnswer) matches(rrtype uint16, values []string) bool {
	if !w.present {
		return false
	}
	got, ok := w.byType[rrtype]
	return ok && sameValues(got, values)
}

// records renders the wildcard back into explicit `*.domain` record sets.
func (w wildcardAnswer) records(domain string) []dnsRecord {
	if !w.present {
		return nil
	}
	out := make([]dnsRecord, 0, len(w.byType))
	for rrtype, values := range w.byType {
		out = append(out, dnsRecord{
			Name:   "*." + domain,
			Type:   dns.TypeToString[rrtype],
			TTL:    w.ttl[rrtype],
			Values: values,
		})
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].Type < out[b].Type })
	return out
}

func (w wildcardAnswer) describe(domain string) string {
	if !w.present {
		return ""
	}
	types := make([]string, 0, len(w.byType))
	for rrtype := range w.byType {
		types = append(types, dns.TypeToString[rrtype])
	}
	sort.Strings(types)
	return fmt.Sprintf(
		"%s answers for every name (a *.%s wildcard, %s). Names covered by it are "+
			"not listed separately — only records that differ from the wildcard are.",
		domain, domain, strings.Join(types, "/"))
}

// detectWildcard asks for two names that cannot plausibly exist. Requiring both
// to answer identically avoids mistaking one unlucky real record for a wildcard.
func detectWildcard(server, domain string) wildcardAnswer {
	probes := []string{
		"meroku-wildcard-probe-zzq7x." + domain,
		"meroku-wildcard-probe-4mn2v." + domain,
	}

	answer := wildcardAnswer{byType: map[uint16][]string{}, ttl: map[uint16]int64{}}
	for _, rrtype := range []uint16{dns.TypeA, dns.TypeAAAA, dns.TypeCNAME, dns.TypeMX, dns.TypeTXT} {
		first, ok1 := queryRecord(server, probes[0], rrtype)
		if !ok1 {
			continue
		}
		second, ok2 := queryRecord(server, probes[1], rrtype)
		if !ok2 || !sameValues(first.Values, second.Values) {
			continue
		}
		answer.present = true
		answer.byType[rrtype] = first.Values
		answer.ttl[rrtype] = first.TTL
	}
	return answer
}

type dnsQuery struct {
	name   string
	rrtype uint16
}

func buildQueries(domain, sub string, types []uint16) []dnsQuery {
	name := domain
	if sub != "" {
		name = sub + "." + domain
	}
	out := make([]dnsQuery, 0, len(types))
	for _, t := range types {
		out = append(out, dnsQuery{name: name, rrtype: t})
	}
	return out
}

// attemptZoneTransfer tries AXFR against each nameserver until one answers.
func attemptZoneTransfer(domain string, servers []string) ([]dnsRecord, error) {
	for _, server := range servers {
		msg := new(dns.Msg)
		msg.SetAxfr(dns.Fqdn(domain))

		tr := &dns.Transfer{DialTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second}
		ch, err := tr.In(msg, net.JoinHostPort(strings.TrimSuffix(server, "."), "53"))
		if err != nil {
			continue
		}

		var records []dnsRecord
		failed := false
		for env := range ch {
			if env.Error != nil {
				failed = true
				break
			}
			for _, rr := range env.RR {
				if rec, ok := rrToRecord(rr); ok {
					records = append(records, rec)
				}
			}
		}
		if !failed && len(records) > 0 {
			return records, nil
		}
	}
	return nil, fmt.Errorf("no nameserver allowed a zone transfer")
}

// queryRecord asks one authoritative server for one name and type.
func queryRecord(server, name string, rrtype uint16) (dnsRecord, bool) {
	c := &dns.Client{Timeout: 4 * time.Second}
	msg := new(dns.Msg)
	msg.SetQuestion(dns.Fqdn(name), rrtype)
	msg.RecursionDesired = false

	resp, _, err := c.Exchange(msg, net.JoinHostPort(strings.TrimSuffix(server, "."), "53"))
	if err != nil || resp == nil || resp.Rcode != dns.RcodeSuccess {
		return dnsRecord{}, false
	}

	var out dnsRecord
	for _, rr := range resp.Answer {
		// Only take answers of the type we asked for. A CNAME chase would
		// otherwise record the target's A record under the alias's name.
		if rr.Header().Rrtype != rrtype {
			continue
		}
		rec, ok := rrToRecord(rr)
		if !ok {
			continue
		}
		out.Name, out.Type, out.TTL = rec.Name, rec.Type, rec.TTL
		out.Values = append(out.Values, rec.Values...)
	}
	return out, len(out.Values) > 0
}

// rrToRecord converts one miekg RR into our shape.
func rrToRecord(rr dns.RR) (dnsRecord, bool) {
	h := rr.Header()
	rec := dnsRecord{
		Name: strings.TrimSuffix(h.Name, "."),
		Type: dns.TypeToString[h.Rrtype],
		TTL:  int64(h.Ttl),
	}

	// The value is the RR's string form with the header removed — this is exactly
	// the zone-file representation Route53 expects, including the quoting rules
	// for TXT, which are easy to get wrong by hand.
	full := rr.String()
	if i := strings.Index(full, "\t"+rec.Type+"\t"); i >= 0 {
		rec.Values = []string{strings.TrimSpace(full[i+len(rec.Type)+2:])}
	} else {
		parts := strings.Fields(full)
		if len(parts) < 5 {
			return dnsRecord{}, false
		}
		rec.Values = []string{strings.Join(parts[4:], " ")}
	}
	if rec.Type == "" || len(rec.Values) == 0 {
		return dnsRecord{}, false
	}
	return rec, true
}

// dedupeRecords merges records sharing a name and type, and orders the result
// so the same zone always renders the same way.
func dedupeRecords(in []dnsRecord) []dnsRecord {
	byKey := map[string]*dnsRecord{}
	var order []string

	for _, r := range in {
		k := r.key()
		if existing, ok := byKey[k]; ok {
			existing.Values = append(existing.Values, r.Values...)
			continue
		}
		copied := r
		byKey[k] = &copied
		order = append(order, k)
	}

	out := make([]dnsRecord, 0, len(order))
	for _, k := range order {
		r := byKey[k]
		r.Values = dedupeStrings(r.Values)
		sort.Strings(r.Values)
		out = append(out, *r)
	}

	sort.SliceStable(out, func(a, b int) bool {
		if out[a].Name != out[b].Name {
			// Apex first, then alphabetically — the apex is what people check.
			return len(out[a].Name) < len(out[b].Name)
		}
		return out[a].Type < out[b].Type
	})
	return out
}

func dedupeStrings(in []string) []string {
	seen := map[string]bool{}
	out := in[:0]
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// recordsToCopy filters a snapshot down to what should actually be written into
// a new Route53 hosted zone.
//
// The apex NS and SOA are excluded: Route53 creates its own, and overwriting
// them with the old provider's would point the new zone back at the old host —
// which is precisely the loop that makes a migration look successful and
// resolve to nothing.
func recordsToCopy(snap zoneSnapshot) []dnsRecord {
	apex := normalizeDomain(snap.Domain)
	out := make([]dnsRecord, 0, len(snap.Records))

	for _, r := range snap.Records {
		if normalizeDomain(r.Name) == apex && (r.Type == "NS" || r.Type == "SOA") {
			continue
		}
		if r.Type == "SOA" {
			continue
		}
		out = append(out, r)
	}
	return out
}

// resolutionDiff is one name compared between the old and new nameservers.
type resolutionDiff struct {
	Name  string
	Type  string
	Old   []string
	New   []string
	Match bool
}

// compareZones resolves every record against both the current nameservers and
// the new Route53 zone, so the operator can see what a nameserver switch would
// change before making it.
//
// This is the gate that turns "trust the copy worked" into "look at what would
// break". A mismatch on an MX or TXT record here is mail going down tomorrow.
func compareZones(ctx context.Context, records []dnsRecord, oldNS, newNS []string) []resolutionDiff {
	if len(oldNS) == 0 || len(newNS) == 0 {
		return nil
	}

	diffs := make([]resolutionDiff, len(records))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 12)

	for i, r := range records {
		wg.Add(1)
		go func(i int, r dnsRecord) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()

			rrtype, ok := dns.StringToType[r.Type]
			if !ok {
				return
			}
			oldRec, _ := queryRecord(oldNS[0], r.Name, rrtype)
			newRec, _ := queryRecord(newNS[0], r.Name, rrtype)

			diffs[i] = resolutionDiff{
				Name:  r.Name,
				Type:  r.Type,
				Old:   oldRec.Values,
				New:   newRec.Values,
				Match: sameValues(oldRec.Values, newRec.Values),
			}
		}(i, r)
	}
	wg.Wait()

	return diffs
}

func sameValues(a, b []string) bool {
	if len(a) != len(b) || len(a) == 0 {
		return len(a) == len(b)
	}
	x := append([]string(nil), a...)
	y := append([]string(nil), b...)
	sort.Strings(x)
	sort.Strings(y)
	for i := range x {
		if !strings.EqualFold(strings.TrimSpace(x[i]), strings.TrimSpace(y[i])) {
			return false
		}
	}
	return true
}

// countMismatches reports how many comparisons would change on a switch.
func countMismatches(diffs []resolutionDiff) int {
	n := 0
	for _, d := range diffs {
		if d.Name != "" && !d.Match {
			n++
		}
	}
	return n
}
