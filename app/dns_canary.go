package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	r53types "github.com/aws/aws-sdk-go-v2/service/route53/types"
)

// Nameservers matching is evidence about the delegation, not about whether a
// resolver can fetch records from the zone. The canary tests the second
// directly: meroku writes a record only it knows the value of, and asks the
// public resolvers for it over HTTPS. A resolver that returns the value has
// walked registry → delegation → this zone and read it.
//
// The record is a wildcard under one label, and every check asks for a new name
// beneath it. A resolver that asked before the record existed caches "no such
// name" for that name only (up to the SOA minimum), so a fresh name each round
// is never blocked by an earlier miss.

const canaryLabel = "_meroku-check"

// dnsCanary is the wildcard TXT record written into the environment's zone.
type dnsCanary struct {
	Profile string
	ZoneID  string
	Zone    string
	Token   string
}

func (c dnsCanary) wildcard() string {
	return "*." + canaryLabel + "." + strings.TrimSuffix(c.Zone, ".")
}

// freshName is a name under the wildcard that no resolver has asked for yet.
func (c dnsCanary) freshName() string {
	return fmt.Sprintf("q%d.%s.%s", time.Now().UnixNano(), canaryLabel, strings.TrimSuffix(c.Zone, "."))
}

func newCanaryToken() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "meroku-" + hex.EncodeToString(b)
}

// canaryRecordWriter upserts or deletes the canary and waits until Route53 has
// applied the change, so no resolver can ask before the record exists. A
// variable so tests can replace it.
var canaryRecordWriter = func(ctx context.Context, c dnsCanary, del bool) error {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithSharedConfigProfile(c.Profile))
	if err != nil {
		return err
	}
	client := route53.NewFromConfig(cfg)
	action := r53types.ChangeActionUpsert
	if del {
		action = r53types.ChangeActionDelete
	}
	out, err := client.ChangeResourceRecordSets(ctx, &route53.ChangeResourceRecordSetsInput{
		HostedZoneId: aws.String(c.ZoneID),
		ChangeBatch: &r53types.ChangeBatch{Changes: []r53types.Change{{
			Action: action,
			ResourceRecordSet: &r53types.ResourceRecordSet{
				Name:            aws.String(c.wildcard()),
				Type:            r53types.RRTypeTxt,
				TTL:             aws.Int64(60),
				ResourceRecords: []r53types.ResourceRecord{{Value: aws.String(`"` + c.Token + `"`)}},
			},
		}}},
	})
	if err != nil || del {
		return err
	}
	return route53.NewResourceRecordSetsChangedWaiter(client).Wait(ctx,
		&route53.GetChangeInput{Id: out.ChangeInfo.Id}, 2*time.Minute)
}

// dohLookup asks one resolver over HTTPS for name's records of rrtype, and
// returns the rcode and the record values.
func dohLookup(client *http.Client, r dohResolver, name, rrtype string) (int, []string, error) {
	u := strings.Replace(fmt.Sprintf(r.URL, url.QueryEscape(normalizeDomain(name))), "type=NS", "type="+rrtype, 1)
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Accept", "application/dns-json")
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, nil, fmt.Errorf("%s returned HTTP %d", r.Name, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return 0, nil, err
	}
	var parsed struct {
		Status int `json:"Status"`
		Answer []struct {
			Data string `json:"data"`
		} `json:"Answer"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return 0, nil, err
	}
	var values []string
	for _, a := range parsed.Answer {
		v := strings.Trim(strings.TrimSpace(a.Data), `"`)
		values = append(values, strings.TrimSuffix(strings.ToLower(v), "."))
	}
	return parsed.Status, values, nil
}

// checkRecordDoH asks every resolver for name and classifies the answer: the
// expected value is resolved, SERVFAIL is a resolver that cannot reach the zone
// (an older delegation), anything else is not yet.
func checkRecordDoH(name, rrtype, want string) map[string]dohVerdict {
	client := &http.Client{Timeout: 8 * time.Second}
	want = strings.TrimSuffix(strings.ToLower(want), ".")
	results := make(map[string]dohVerdict, len(dohResolvers))
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, r := range dohResolvers {
		wg.Add(1)
		go func(r dohResolver) {
			defer wg.Done()
			verdict := dohNotYet
			if status, values, err := dohLookup(client, r, name, rrtype); err == nil {
				switch {
				case containsValue(values, want):
					verdict = dohResolved
				case status == rcodeServFail:
					verdict = dohStale
				}
			}
			mu.Lock()
			results[r.Name] = verdict
			mu.Unlock()
		}(r)
	}
	wg.Wait()
	return results
}

func containsValue(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}

// canaryCheck asks the resolvers for a fresh name under the canary. A variable
// so tests can replace it.
var canaryCheck = func(c dnsCanary) map[string]dohVerdict {
	return checkRecordDoH(c.freshName(), "TXT", c.Token)
}
