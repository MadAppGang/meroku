package main

import (
	"strings"
	"testing"

	"github.com/miekg/dns"
)

// The apex NS and SOA must never be copied into the new zone.
//
// This is the migration's one unrecoverable mistake: writing the old provider's
// NS records into the new zone points it straight back at the old host. The
// switch then "succeeds" — the registrar delegates to Route53, Route53 delegates
// back — and the domain resolves to whatever the old provider still holds, or to
// nothing. Route53 creates its own NS and SOA; ours must not overwrite them.
func TestRecordsToCopy_ExcludesApexNSAndSOA(t *testing.T) {
	snap := zoneSnapshot{
		Domain: "example.com",
		Records: []dnsRecord{
			{Name: "example.com", Type: "NS", Values: []string{"ns1.hover.com."}},
			{Name: "example.com", Type: "SOA", Values: []string{"ns1.hover.com. root. 1 2 3 4 5"}},
			{Name: "example.com", Type: "A", Values: []string{"192.0.2.1"}},
			{Name: "example.com", Type: "MX", Values: []string{"10 mail.example.com."}},
			{Name: "www.example.com", Type: "CNAME", Values: []string{"example.com."}},
			// A delegated subdomain that already exists must be carried across.
			{Name: "legacy.example.com", Type: "NS", Values: []string{"ns1.other.net."}},
		},
	}

	got := recordsToCopy(snap)

	for _, r := range got {
		if r.Type == "SOA" {
			t.Errorf("SOA must never be copied: %+v", r)
		}
		if r.Type == "NS" && normalizeDomain(r.Name) == "example.com" {
			t.Errorf("apex NS must never be copied — it would point the new zone at the old host: %+v", r)
		}
	}

	var kinds []string
	for _, r := range got {
		kinds = append(kinds, r.Name+"/"+r.Type)
	}
	for _, want := range []string{
		"example.com/A", "example.com/MX",
		"www.example.com/CNAME", "legacy.example.com/NS",
	} {
		if !contains(kinds, want) {
			t.Errorf("%s should have been kept, got %v", want, kinds)
		}
	}
}

func contains(hay []string, needle string) bool {
	for _, h := range hay {
		if h == needle {
			return true
		}
	}
	return false
}

// A trailing dot on the apex must not smuggle the NS record past the filter.
func TestRecordsToCopy_ApexMatchIgnoresTrailingDot(t *testing.T) {
	snap := zoneSnapshot{
		Domain: "example.com.",
		Records: []dnsRecord{
			{Name: "example.com.", Type: "NS", Values: []string{"ns1.hover.com."}},
		},
	}
	if got := recordsToCopy(snap); len(got) != 0 {
		t.Errorf("apex NS should have been dropped, got %+v", got)
	}
}

// A probe sweep can never prove absence, so it must say so.
func TestZoneSnapshot_WarningOnlyWhenIncomplete(t *testing.T) {
	complete := zoneSnapshot{Complete: true, Method: "zone transfer (AXFR)"}
	if complete.Warning() != "" {
		t.Error("a zone transfer is authoritative; it should carry no caveat")
	}

	partial := zoneSnapshot{
		Records:     []dnsRecord{{Name: "example.com", Type: "A"}},
		NamesProbed: 60,
	}
	w := partial.Warning()
	for _, want := range []string{"cannot be listed", "60", "before switching nameservers"} {
		if !strings.Contains(w, want) {
			t.Errorf("warning should mention %q, got: %s", want, w)
		}
	}
}

func TestDedupeRecords_MergesValuesAndIsStable(t *testing.T) {
	in := []dnsRecord{
		{Name: "example.com", Type: "MX", TTL: 300, Values: []string{"20 mx2.example.com."}},
		{Name: "example.com", Type: "MX", TTL: 300, Values: []string{"10 mx1.example.com."}},
		{Name: "example.com", Type: "MX", TTL: 300, Values: []string{"10 mx1.example.com."}},
		{Name: "www.example.com", Type: "A", TTL: 60, Values: []string{"192.0.2.1"}},
	}

	first := dedupeRecords(in)
	if len(first) != 2 {
		t.Fatalf("expected 2 record sets, got %d: %+v", len(first), first)
	}

	var mx dnsRecord
	for _, r := range first {
		if r.Type == "MX" {
			mx = r
		}
	}
	if len(mx.Values) != 2 {
		t.Errorf("duplicate MX values should collapse, got %v", mx.Values)
	}

	// Same input, same output — the operator compares this list against their
	// provider's, so it must not reshuffle between runs.
	second := dedupeRecords(in)
	for i := range first {
		if first[i].key() != second[i].key() {
			t.Errorf("ordering is not stable at %d: %s vs %s", i, first[i].key(), second[i].key())
		}
	}
}

// Route53 rejects types it does not know; catching them here produces a precise
// message instead of an opaque API error.
func TestRoute53RRType(t *testing.T) {
	for _, ok := range []string{"A", "aaaa", "CNAME", "MX", "TXT", "SRV", "CAA", "NS", "SPF"} {
		if _, valid := route53RRType(ok); !valid {
			t.Errorf("%s should be supported", ok)
		}
	}
	for _, bad := range []string{"SOA", "DNSKEY", "RRSIG", "", "NOTAREALTYPE"} {
		if _, valid := route53RRType(bad); valid {
			t.Errorf("%s should be rejected before it reaches the API", bad)
		}
	}
}

func TestSameValues(t *testing.T) {
	cases := []struct {
		name string
		a, b []string
		want bool
	}{
		{"identical", []string{"192.0.2.1"}, []string{"192.0.2.1"}, true},
		{"order differs", []string{"a", "b"}, []string{"b", "a"}, true},
		{"case differs", []string{"MX1.example.com."}, []string{"mx1.example.com."}, true},
		{"whitespace", []string{" 10 mx.example.com. "}, []string{"10 mx.example.com."}, true},
		{"different", []string{"192.0.2.1"}, []string{"192.0.2.2"}, false},
		{"missing on one side", []string{"192.0.2.1"}, nil, false},
		{"both empty", nil, nil, true},
		{"count differs", []string{"a"}, []string{"a", "b"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sameValues(tc.a, tc.b); got != tc.want {
				t.Errorf("sameValues(%v, %v) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

func TestCountMismatches(t *testing.T) {
	diffs := []resolutionDiff{
		{Name: "example.com", Type: "A", Match: true},
		{Name: "example.com", Type: "MX", Match: false},
		{Name: "www.example.com", Type: "CNAME", Match: false},
		{}, // an entry the comparison never filled in must not count
	}
	if got := countMismatches(diffs); got != 2 {
		t.Errorf("expected 2 mismatches, got %d", got)
	}
}

// The value written to Route53 must be the zone-file form, because that is what
// the API expects — TXT quoting in particular is easy to mangle by hand.
func TestRRToRecord_UsesZoneFileValues(t *testing.T) {
	cases := []struct {
		zone      string
		wantType  string
		wantValue string
	}{
		{"example.com.\t300\tIN\tA\t192.0.2.1", "A", "192.0.2.1"},
		{"example.com.\t300\tIN\tMX\t10 mx1.example.com.", "MX", "10 mx1.example.com."},
		{"example.com.\t300\tIN\tTXT\t\"v=spf1 include:_spf.google.com ~all\"", "TXT", `"v=spf1 include:_spf.google.com ~all"`},
		{"www.example.com.\t60\tIN\tCNAME\texample.com.", "CNAME", "example.com."},
	}

	for _, tc := range cases {
		rr, err := dns.NewRR(tc.zone)
		if err != nil {
			t.Fatalf("could not parse %q: %v", tc.zone, err)
		}
		got, ok := rrToRecord(rr)
		if !ok {
			t.Fatalf("failed to convert %q", tc.zone)
		}
		if got.Type != tc.wantType {
			t.Errorf("type = %q, want %q", got.Type, tc.wantType)
		}
		if len(got.Values) != 1 || got.Values[0] != tc.wantValue {
			t.Errorf("value = %v, want [%q]", got.Values, tc.wantValue)
		}
		if strings.HasSuffix(got.Name, ".") {
			t.Errorf("name should have no trailing dot, got %q", got.Name)
		}
	}
}

// SPF policy lives in a TXT record and losing it silently sends mail to spam.
// This pins that a realistic mail setup survives the filter intact.
func TestRecordsToCopy_KeepsMailRecords(t *testing.T) {
	snap := zoneSnapshot{
		Domain: "example.com",
		Records: []dnsRecord{
			{Name: "example.com", Type: "NS", Values: []string{"ns1.hover.com."}},
			{Name: "example.com", Type: "MX", Values: []string{"1 aspmx.l.google.com."}},
			{Name: "example.com", Type: "TXT", Values: []string{`"v=spf1 include:_spf.google.com ~all"`}},
			{Name: "_dmarc.example.com", Type: "TXT", Values: []string{`"v=DMARC1; p=quarantine"`}},
			{Name: "google._domainkey.example.com", Type: "TXT", Values: []string{`"v=DKIM1; k=rsa; p=MIGf"`}},
		},
	}

	got := recordsToCopy(snap)
	if len(got) != 4 {
		t.Fatalf("expected the 4 mail records kept and the NS dropped, got %d: %+v", len(got), got)
	}
	for _, want := range []string{"MX", "TXT"} {
		found := false
		for _, r := range got {
			if r.Type == want {
				found = true
			}
		}
		if !found {
			t.Errorf("no %s record survived: %+v", want, got)
		}
	}
}

// The wildcard filter must only suppress the types the wildcard actually
// answers for.
//
// A zone serving a *.example.com A record still has real NS, MX, TXT and SRV
// records underneath it. Suppressing a subdomain NS would silently drop an
// existing delegation during the migration — the one record whose loss breaks
// exactly the thing the operator came here to set up.
func TestWildcardAnswer_OnlySuppressesTypesItAnswers(t *testing.T) {
	w := wildcardAnswer{
		present: true,
		byType:  map[uint16][]string{dns.TypeA: {"216.40.34.41"}},
		ttl:     map[uint16]int64{dns.TypeA: 300},
	}

	if !w.matches(dns.TypeA, []string{"216.40.34.41"}) {
		t.Error("an A answer identical to the wildcard should be suppressed")
	}
	if w.matches(dns.TypeA, []string{"192.0.2.9"}) {
		t.Error("an A record that differs from the wildcard is real and must be kept")
	}
	for _, rrtype := range []uint16{dns.TypeNS, dns.TypeMX, dns.TypeTXT, dns.TypeSRV, dns.TypeCAA} {
		if w.matches(rrtype, []string{"anything"}) {
			t.Errorf("%s must never be suppressed by an A-only wildcard",
				dns.TypeToString[rrtype])
		}
	}
}

// With no wildcard nothing is suppressed at all.
func TestWildcardAnswer_AbsentSuppressesNothing(t *testing.T) {
	var w wildcardAnswer
	if w.matches(dns.TypeA, []string{"192.0.2.1"}) {
		t.Error("no wildcard means no suppression")
	}
	if got := w.records("example.com"); got != nil {
		t.Errorf("no wildcard means no synthesised record, got %+v", got)
	}
	if w.describe("example.com") != "" {
		t.Error("no wildcard means nothing to describe")
	}
}

// The wildcard is itself a record and must be carried across, or every name it
// covered stops resolving after the switch.
func TestWildcardAnswer_IsCarriedAcrossAsARecord(t *testing.T) {
	w := wildcardAnswer{
		present: true,
		byType: map[uint16][]string{
			dns.TypeA:  {"216.40.34.41"},
			dns.TypeMX: {"10 mx.example.net."},
		},
		ttl: map[uint16]int64{dns.TypeA: 300, dns.TypeMX: 300},
	}

	got := w.records("example.com")
	if len(got) != 2 {
		t.Fatalf("expected both wildcard types, got %+v", got)
	}
	for _, r := range got {
		if r.Name != "*.example.com" {
			t.Errorf("wildcard record should be named *.example.com, got %q", r.Name)
		}
	}
	// Stable ordering, since this list is read against a provider's own.
	if got[0].Type != "A" || got[1].Type != "MX" {
		t.Errorf("wildcard records should be ordered by type, got %s then %s", got[0].Type, got[1].Type)
	}
}
