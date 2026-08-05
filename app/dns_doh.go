package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Propagation checks over DNS-over-HTTPS.
//
// The obvious way to ask "does 8.8.8.8 see this delegation yet" is to send a UDP
// query to 8.8.8.8:53. On any network that intercepts port 53 — corporate DNS,
// many ISPs, some VPNs, plenty of hotel and café wifi — that packet never
// reaches Google. The local resolver answers instead, from its own cache, and
// the reply is indistinguishable from a real one.
//
// That is not hypothetical. Measured on the machine this was written on, for the
// same name at the same moment:
//
//	port 53 to 8.8.8.8 -> ns-1555 / ns-213  / ns-839  / ns-1058   (a zone deleted hours earlier)
//	DoH to dns.google  -> ns-1310 / ns-745  / ns-1739 / ns-239    (the live delegation)
//
// The stale answer had earlier convinced meroku a delegation existed when the
// parent zone had no such record, and later convinced it none existed when the
// record was live worldwide. Both directions, same cause.
//
// DoH runs over 443 to a named host with TLS, so an interception has to be a
// deliberate MITM with a trusted certificate rather than a transparent port
// redirect. It is the only way to ask a specific public resolver a question and
// know the answer came from it.

// dohResolver is one public resolver reachable over HTTPS.
type dohResolver struct {
	// Name is what the operator sees. Deliberately the provider rather than an
	// IP: with DoH there is no single address being queried, and showing 8.8.8.8
	// would imply a UDP query we are specifically not making.
	Name string
	// URL is a format string taking the query name.
	URL string
}

// dohResolvers are the resolvers propagation is measured against.
//
// Four independent operators on three continents' worth of anycast. Quad9's DoH
// endpoint does not serve the JSON API these use, so AdGuard and NextDNS stand
// in — the point is independence of operator, not any particular brand.
var dohResolvers = []dohResolver{
	{Name: "Google", URL: "https://dns.google/resolve?name=%s&type=NS"},
	{Name: "Cloudflare", URL: "https://cloudflare-dns.com/dns-query?name=%s&type=NS"},
	{Name: "AdGuard", URL: "https://dns.adguard-dns.com/resolve?name=%s&type=NS"},
	{Name: "NextDNS", URL: "https://dns.nextdns.io/dns-query?name=%s&type=NS"},
}

func dohResolverNames() []string {
	names := make([]string, 0, len(dohResolvers))
	for _, r := range dohResolvers {
		names = append(names, r.Name)
	}
	return names
}

// dohVerdict is what one resolver's answer means for a delegation.
//
// Three states rather than two, because "this resolver does not see it" hid the
// distinction that decides whether waiting is worth anything:
//
//   - a resolver with nothing cached picks the delegation up within minutes;
//   - a resolver still holding a previous incarnation of the zone is stuck for
//     as long as that answer lives, which is up to two days.
//
// Route53 assigns a fresh random nameserver set to every hosted zone, and
// publishes the zone's own apex NS records with a 172800s TTL. A resolver caches
// that RRset from the child's authoritative answer rather than from the parent's
// referral, so after a zone is deleted and recreated it keeps querying the old
// set — which now answers REFUSED, because those servers no longer host the
// zone. The name goes SERVFAIL rather than merely stale, and the short TTL on
// the delegation record in the parent does nothing to shorten it.
//
// Observed during development: two of four resolvers were still querying an
// entirely different nameserver set hours after the zone was recreated, while
// the two with cold caches saw the correct delegation immediately. Rendered as
// "not yet" that reads as ordinary propagation lag, and the operator waits for
// something that cannot happen.
type dohVerdict int

const (
	// dohNotYet is the zero value so a resolver that has not been asked reads as
	// "not yet" rather than as a problem.
	dohNotYet dohVerdict = iota
	// dohResolved means it returned exactly the expected nameservers.
	dohResolved
	// dohStale means it holds an answer that is not ours and will not clear on
	// any timescale a deploy can wait for.
	dohStale
)

// DNS response codes, as the JSON API reports them verbatim in Status.
const (
	rcodeNoError  = 0
	rcodeServFail = 2
)

// dohAnswer is one resolver's response, kept whole rather than reduced to a list
// of nameservers so the caller can tell an empty answer from a failed one.
type dohAnswer struct {
	// Status is the DNS rcode. SERVFAIL is the signature of a delegation whose
	// nameservers do not answer for the zone.
	Status int
	// NS is the nameserver set the resolver returned, without trailing dots.
	NS []string
}

// classifyNSAnswer decides what one answer means for the delegation we wrote.
//
// The two stale signals are different observations of the same fault. A resolver
// that returns nameservers which are not ours is looking at another delegation
// outright; one that returns SERVFAIL has a delegation it cannot use, because
// every server in it refused. Both mean this resolver is not going to agree with
// us until a cache measured in days expires.
//
// SERVFAIL has other causes — a DNSSEC validation failure, an upstream outage —
// and this deliberately does not try to tell them apart. Every one of them is a
// resolver that cannot answer for the zone, which is worth showing plainly
// rather than folding into the same "not yet" as an empty cache.
func classifyNSAnswer(a dohAnswer, expected []string) dohVerdict {
	switch {
	case nameserverSetsMatch(expected, a.NS):
		return dohResolved
	case len(a.NS) > 0:
		return dohStale
	case a.Status == rcodeServFail:
		return dohStale
	default:
		// NXDOMAIN and NOERROR-with-no-answer are the same thing here: no
		// delegation cached, so the next check may well find one.
		return dohNotYet
	}
}

// queryNameserversDoH asks one resolver over HTTPS for a name's NS records.
func queryNameserversDoH(client *http.Client, r dohResolver, domain string) (dohAnswer, error) {
	domain = normalizeDomain(domain)
	if domain == "" {
		return dohAnswer{}, fmt.Errorf("no domain")
	}

	req, err := http.NewRequest("GET", fmt.Sprintf(r.URL, url.QueryEscape(domain)), nil)
	if err != nil {
		return dohAnswer{}, err
	}
	// Every one of these endpoints needs the JSON accept header; without it some
	// return wire-format bytes that parse as garbage rather than erroring.
	req.Header.Set("Accept", "application/dns-json")

	resp, err := client.Do(req)
	if err != nil {
		return dohAnswer{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return dohAnswer{}, fmt.Errorf("%s returned HTTP %d", r.Name, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return dohAnswer{}, err
	}

	var parsed struct {
		Status int `json:"Status"`
		Answer []struct {
			Type int    `json:"type"`
			Data string `json:"data"`
		} `json:"Answer"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return dohAnswer{}, fmt.Errorf("%s returned unparseable JSON: %w", r.Name, err)
	}

	// The rcode is carried out even when there are answers. It is the only way to
	// tell "no delegation cached" from "a delegation whose servers all refused",
	// and reducing the response to its NS list threw that away.
	answer := dohAnswer{Status: parsed.Status}
	for _, a := range parsed.Answer {
		if a.Type == 2 { // NS
			answer.NS = append(answer.NS, strings.TrimSuffix(a.Data, "."))
		}
	}
	return answer, nil
}

// checkPropagationDoH asks every resolver whether a zone is delegated to the
// expected nameservers, concurrently.
//
// A resolver that errors counts as "not yet" rather than failing the whole check
// — one provider being unreachable should not stall a deploy, and the operator
// can see which one is missing from the list. That is a transport failure, not a
// DNS answer, so it is deliberately not reported as stale: we learned nothing
// about what that resolver holds.
func checkPropagationDoH(domain string, expected []string) map[string]dohVerdict {
	client := &http.Client{Timeout: 8 * time.Second}

	results := make(map[string]dohVerdict, len(dohResolvers))
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, r := range dohResolvers {
		wg.Add(1)
		go func(r dohResolver) {
			defer wg.Done()
			verdict := dohNotYet
			if observed, err := queryNameserversDoH(client, r, domain); err == nil {
				verdict = classifyNSAnswer(observed, expected)
			}

			mu.Lock()
			results[r.Name] = verdict
			mu.Unlock()
		}(r)
	}
	wg.Wait()

	return results
}

// countVerdicts tallies a result set. Both counts are wanted at once everywhere
// they are wanted at all — the agreement meter needs the first, the wording
// around it needs the second.
func countVerdicts(results map[string]dohVerdict) (resolved, stale int) {
	for _, v := range results {
		switch v {
		case dohResolved:
			resolved++
		case dohStale:
			stale++
		}
	}
	return resolved, stale
}
