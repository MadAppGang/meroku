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

// queryNameserversDoH asks one resolver over HTTPS for a name's NS records.
func queryNameserversDoH(client *http.Client, r dohResolver, domain string) ([]string, error) {
	domain = normalizeDomain(domain)
	if domain == "" {
		return nil, fmt.Errorf("no domain")
	}

	req, err := http.NewRequest("GET", fmt.Sprintf(r.URL, url.QueryEscape(domain)), nil)
	if err != nil {
		return nil, err
	}
	// Every one of these endpoints needs the JSON accept header; without it some
	// return wire-format bytes that parse as garbage rather than erroring.
	req.Header.Set("Accept", "application/dns-json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s returned HTTP %d", r.Name, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}

	var parsed struct {
		Status int `json:"Status"`
		Answer []struct {
			Type int    `json:"type"`
			Data string `json:"data"`
		} `json:"Answer"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("%s returned unparseable JSON: %w", r.Name, err)
	}

	var ns []string
	for _, a := range parsed.Answer {
		if a.Type == 2 { // NS
			ns = append(ns, strings.TrimSuffix(a.Data, "."))
		}
	}
	if len(ns) == 0 {
		// NXDOMAIN and "delegated but not here yet" are the same answer for our
		// purposes: this resolver does not see it.
		return nil, nil
	}
	return ns, nil
}

// checkPropagationDoH asks every resolver whether a zone is delegated to the
// expected nameservers, concurrently.
//
// A resolver that errors counts as "does not see it" rather than failing the
// whole check — one provider being unreachable should not stall a deploy, and
// the operator can see which one is missing from the list.
func checkPropagationDoH(domain string, expected []string) map[string]bool {
	client := &http.Client{Timeout: 8 * time.Second}

	results := make(map[string]bool, len(dohResolvers))
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, r := range dohResolvers {
		wg.Add(1)
		go func(r dohResolver) {
			defer wg.Done()
			observed, err := queryNameserversDoH(client, r, domain)
			ok := err == nil && len(observed) > 0 && nameserverSetsMatch(expected, observed)

			mu.Lock()
			results[r.Name] = ok
			mu.Unlock()
		}(r)
	}
	wg.Wait()

	return results
}
