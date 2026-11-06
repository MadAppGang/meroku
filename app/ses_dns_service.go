package main

// What the web app needs to edit one SES domain's DNS records: which kind of
// zone the domain lives in, what the apex publishes today, the sender (SPF)
// record meroku suggests, and the problems the YAML values have.
//
// The live read is a seam (sesDNSLiveReader) so tests never reach AWS. A read
// failure is reported in Live.ReadError and never fails the request: the
// values can still be edited and checked without it.

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	r53types "github.com/aws/aws-sdk-go-v2/service/route53/types"
)

const (
	sesDefaultSPFRecord = "v=spf1 include:amazonses.com ~all"
	sesSPFInclude       = "include:amazonses.com"
	sesDNSAWSTimeout    = 15 * time.Second
)

// sesSPFAllAtEnd matches a trailing all mechanism with an optional qualifier.
var sesSPFAllAtEnd = regexp.MustCompile(` ([-~?+]?all)$`)

// sesLiveApex is what the domain's apex publishes now.
type sesLiveApex struct {
	ApexTTL   *int64   `json:"apex_ttl"`
	SPF       []string `json:"spf"`
	ApexOther []string `json:"apex_other"`
	ReadError string   `json:"read_error,omitempty"`
}

// sesDNSReport is the body of GET /api/ses-dns.
type sesDNSReport struct {
	Domain       string        `json:"domain"`
	ZoneOrigin   string        `json:"zone_origin"`
	ZoneID       string        `json:"zone_id"`
	Live         *sesLiveApex  `json:"live"`
	SuggestedSPF string        `json:"suggested_spf"`
	Issues       []sesDNSIssue `json:"issues"`
}

// sesDNSLiveReader reads the TXT values at name. zoneID may be empty, in
// which case the reader finds the public zone that holds name.
type sesDNSLiveReader func(ctx context.Context, profile, region, zoneID, name string) (ttl *int64, values []string, err error)

// suggestSESSPF returns the sender record meroku proposes for a domain whose
// apex publishes live (one SPF value, or "" for none). It mirrors the
// spf_suggestion local in modules/ses/main.tf.
func suggestSESSPF(live string) string {
	switch {
	case live == "":
		return sesDefaultSPFRecord
	case strings.Contains(live, sesSPFInclude):
		return live
	case sesSPFAllAtEnd.MatchString(live):
		return sesSPFAllAtEnd.ReplaceAllString(live, " "+sesSPFInclude+" $1")
	default:
		return live + " " + sesSPFInclude
	}
}

// joinTXTValue turns a Route53 TXT value ("part1" "part2") into its text.
func joinTXTValue(v string) string {
	return strings.ReplaceAll(strings.Trim(v, `"`), sesTXTDelimiter, "")
}

// splitLiveApex separates the sender records from the other apex values.
func splitLiveApex(values []string) (spf, other []string) {
	for _, v := range values {
		text := joinTXTValue(v)
		if sesSPFPrefixAnyCase.MatchString(text) {
			spf = append(spf, text)
		} else {
			other = append(other, text)
		}
	}
	sort.Strings(other)
	return spf, other
}

// findSESDomain returns the ses.domains[] entry for domain, or nil.
func findSESDomain(envMap map[string]interface{}, domain string) (ses, entry map[string]interface{}) {
	ses, _ = envMap["ses"].(map[string]interface{})
	if ses == nil {
		return nil, nil
	}
	list, _ := ses["domains"].([]interface{})
	want := strings.ToLower(strings.TrimSuffix(domain, "."))
	for _, raw := range list {
		d, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if strings.ToLower(computeStringField(d, "domain")) == want {
			return ses, d
		}
	}
	return ses, nil
}

// isLegacySESDomain reports whether domain is the one domain of the legacy
// single-domain form (no ses.domains list).
func isLegacySESDomain(envMap, ses map[string]interface{}, domain string) bool {
	if list, _ := ses["domains"].([]interface{}); len(list) > 0 {
		return false
	}
	primary, _ := envMap[sesPrimaryDomainKey].(string)
	return primary != "" && strings.EqualFold(primary, strings.TrimSuffix(domain, "."))
}

// inspectSESDNS builds the report for one domain of an already-loaded env
// map. candidate, when non-nil, is checked as the spf_record in place of the
// YAML's value. read is nil to skip the live read.
func inspectSESDNS(ctx context.Context, envMap map[string]interface{}, domain string, candidate *string, read sesDNSLiveReader) (*sesDNSReport, error) {
	assignSESPrimaryDomain(envMap)
	resolveSESZones(envMap)

	ses, entry := findSESDomain(envMap, domain)
	var zone sesZone
	switch {
	case entry != nil:
		zone = sesZoneForDomain(envMap, entry)
	case ses != nil && isLegacySESDomain(envMap, ses, domain):
		// The legacy single-domain form: its zone is module.domain's.
		entry = map[string]interface{}{"domain": envMap[sesPrimaryDomainKey]}
		origin, _ := envMap[sesLegacyZoneOriginKey].(string)
		zone = sesZone{Origin: origin}
	default:
		return nil, fmt.Errorf("ses.domains has no entry for %s", domain)
	}

	checked := entry
	if candidate != nil {
		checked = make(map[string]interface{}, len(entry)+1)
		for k, v := range entry {
			checked[k] = v
		}
		checked["spf_record"] = *candidate
	}

	report := &sesDNSReport{
		Domain:       computeStringField(entry, "domain"),
		ZoneOrigin:   zone.Origin,
		ZoneID:       zone.ZoneID,
		SuggestedSPF: sesDefaultSPFRecord,
		Issues:       validateSESDomainDNS(ses, checked),
	}
	if report.Issues == nil {
		report.Issues = []sesDNSIssue{}
	}
	if read == nil {
		return report, nil
	}

	profile, _ := envMap["aws_profile"].(string)
	region, _ := envMap["region"].(string)
	live := &sesLiveApex{SPF: []string{}, ApexOther: []string{}}
	ttl, values, err := read(ctx, profile, region, zone.ZoneID, report.Domain)
	if err != nil {
		live.ReadError = err.Error()
	} else {
		live.ApexTTL = ttl
		spf, other := splitLiveApex(values)
		if spf != nil {
			live.SPF = spf
		}
		if other != nil {
			live.ApexOther = other
		}
		if len(spf) == 1 {
			report.SuggestedSPF = suggestSESSPF(spf[0])
		}
	}
	report.Live = live
	return report, nil
}

// readRoute53TXT is the real sesDNSLiveReader.
func readRoute53TXT(ctx context.Context, profile, region, zoneID, name string) (*int64, []string, error) {
	opts := []func(*config.LoadOptions) error{}
	if profile != "" {
		opts = append(opts, config.WithSharedConfigProfile(profile))
	}
	if region != "" {
		opts = append(opts, config.WithRegion(region))
	}
	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load AWS config for profile %q: %w", profile, err)
	}
	client := route53.NewFromConfig(cfg)

	if zoneID == "" {
		zoneID, err = findZoneHolding(ctx, client, name)
		if err != nil {
			return nil, nil, err
		}
	}

	fqdn := strings.TrimSuffix(name, ".") + "."
	out, err := client.ListResourceRecordSets(ctx, &route53.ListResourceRecordSetsInput{
		HostedZoneId:    aws.String(zoneID),
		StartRecordName: aws.String(fqdn),
		StartRecordType: r53types.RRTypeTxt,
		MaxItems:        aws.Int32(1),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read TXT records of %s: %w", name, err)
	}
	for _, rs := range out.ResourceRecordSets {
		if rs.Name == nil || !strings.EqualFold(*rs.Name, fqdn) || rs.Type != r53types.RRTypeTxt {
			continue
		}
		values := make([]string, 0, len(rs.ResourceRecords))
		for _, rr := range rs.ResourceRecords {
			if rr.Value != nil {
				values = append(values, *rr.Value)
			}
		}
		return rs.TTL, values, nil
	}
	return nil, nil, nil
}

// findZoneHolding returns the id of the public zone whose name is the
// longest suffix of name.
func findZoneHolding(ctx context.Context, client *route53.Client, name string) (string, error) {
	labels := strings.Split(strings.ToLower(strings.TrimSuffix(name, ".")), ".")
	for i := 0; i < len(labels)-1; i++ {
		candidate := strings.Join(labels[i:], ".")
		resp, err := client.ListHostedZonesByName(ctx, &route53.ListHostedZonesByNameInput{
			DNSName:  aws.String(candidate),
			MaxItems: aws.Int32(1),
		})
		if err != nil {
			return "", fmt.Errorf("failed to list hosted zones: %w", err)
		}
		for _, z := range resp.HostedZones {
			if z.Name == nil || z.Id == nil || (z.Config != nil && z.Config.PrivateZone) {
				continue
			}
			if strings.TrimSuffix(strings.ToLower(*z.Name), ".") == candidate {
				return strings.TrimPrefix(*z.Id, "/hostedzone/"), nil
			}
		}
	}
	return "", fmt.Errorf("no public hosted zone in this account holds %s", name)
}
