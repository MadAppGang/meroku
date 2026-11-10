package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// RDAP is the registries' HTTPS interface to their own records, the successor
// to WHOIS. Its nameserver list for a domain is the delegation the registry
// publishes, read over HTTPS, so a network that intercepts port 53 cannot
// substitute a cached answer for it.

// rdapBootstrapURL is IANA's map from TLD to that registry's RDAP service.
const rdapBootstrapURL = "https://data.iana.org/rdap/dns.json"

var (
	rdapBootstrapOnce sync.Once
	rdapBootstrap     map[string]string
	rdapBootstrapErr  error
	rdapClient        = &http.Client{Timeout: 10 * time.Second}
)

// rdapBaseFor returns the RDAP service URL for a TLD.
func rdapBaseFor(tld string) (string, error) {
	rdapBootstrapOnce.Do(func() {
		resp, err := rdapClient.Get(rdapBootstrapURL)
		if err != nil {
			rdapBootstrapErr = err
			return
		}
		defer resp.Body.Close()
		var doc struct {
			Services [][][]string `json:"services"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
			rdapBootstrapErr = err
			return
		}
		rdapBootstrap = map[string]string{}
		for _, svc := range doc.Services {
			if len(svc) < 2 || len(svc[1]) == 0 {
				continue
			}
			for _, t := range svc[0] {
				rdapBootstrap[strings.ToLower(t)] = svc[1][0]
			}
		}
	})
	if rdapBootstrapErr != nil {
		return "", rdapBootstrapErr
	}
	base, ok := rdapBootstrap[strings.ToLower(tld)]
	if !ok {
		return "", fmt.Errorf("no RDAP service for .%s", tld)
	}
	return base, nil
}

// rdapNameservers returns the nameservers the registry lists for a domain
// directly under a TLD.
var rdapNameservers = func(domain string) ([]string, error) {
	domain = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(domain)), ".")
	labels := strings.Split(domain, ".")
	if len(labels) != 2 {
		return nil, fmt.Errorf("%s is not directly under a TLD", domain)
	}
	base, err := rdapBaseFor(labels[1])
	if err != nil {
		return nil, err
	}
	resp, err := rdapClient.Get(strings.TrimSuffix(base, "/") + "/domain/" + domain)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("RDAP for %s: HTTP %d", domain, resp.StatusCode)
	}
	var doc struct {
		Nameservers []struct {
			LDHName string `json:"ldhName"`
		} `json:"nameservers"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return nil, err
	}
	var out []string
	for _, ns := range doc.Nameservers {
		if ns.LDHName != "" {
			out = append(out, strings.TrimSuffix(strings.ToLower(ns.LDHName), "."))
		}
	}
	return out, nil
}
