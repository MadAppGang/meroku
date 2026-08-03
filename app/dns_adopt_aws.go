package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/aws/aws-sdk-go-v2/service/route53/types"
)

// The AWS half of adopting a domain into Route53: create the apex zone, copy the
// discovered records into it, and read back the nameservers the operator must
// set at their registrar.

// adoptedZone is the result of creating the apex hosted zone.
type adoptedZone struct {
	ZoneID      string
	Nameservers []string
	Created     bool // false when an existing zone was reused
}

// createOrFindApexZone creates a public hosted zone for the apex domain, or
// returns the existing one.
//
// Reusing an existing zone rather than creating a second is important: Route53
// happily hosts two zones with the same name, each with its own nameservers, and
// only the set the registrar points at is live. A duplicate is the kind of
// mistake that looks fine in the console and resolves to the wrong place.
func createOrFindApexZone(ctx context.Context, profile, domain string) (adoptedZone, error) {
	domain = normalizeDomain(domain)

	cfg, err := config.LoadDefaultConfig(ctx, config.WithSharedConfigProfile(profile))
	if err != nil {
		return adoptedZone{}, fmt.Errorf("failed to load AWS config for %q: %w", profile, err)
	}
	client := route53.NewFromConfig(cfg)

	if zoneID, ns, err := findHostedZoneByName(ctx, profile, domain); err == nil && zoneID != "" {
		return adoptedZone{ZoneID: zoneID, Nameservers: ns, Created: false}, nil
	}

	out, err := client.CreateHostedZone(ctx, &route53.CreateHostedZoneInput{
		Name: aws.String(domain),
		// Route53 requires a unique caller reference per creation attempt.
		CallerReference: aws.String(fmt.Sprintf("meroku-adopt-%s-%d", domain, time.Now().UnixNano())),
		HostedZoneConfig: &types.HostedZoneConfig{
			Comment: aws.String("Adopted by meroku so subdomains can be delegated"),
		},
	})
	if err != nil {
		return adoptedZone{}, fmt.Errorf("could not create hosted zone for %s: %w", domain, err)
	}

	zoneID := strings.TrimPrefix(aws.ToString(out.HostedZone.Id), "/hostedzone/")
	ns, err := getZoneNameservers(ctx, client, zoneID)
	if err != nil {
		return adoptedZone{ZoneID: zoneID}, fmt.Errorf("zone created but could not read its nameservers: %w", err)
	}
	return adoptedZone{ZoneID: zoneID, Nameservers: ns, Created: true}, nil
}

// writeRecordsToZone upserts records into a hosted zone, and reports which ones
// could not be written.
//
// Each record is submitted on its own rather than in one batch. A batch is
// atomic, so a single record Route53 rejects — an unsupported type, a malformed
// value copied verbatim from a permissive provider — would discard every other
// record with it. One at a time means a partial copy plus an exact list of what
// needs doing by hand, which is far more useful than all-or-nothing.
func writeRecordsToZone(ctx context.Context, profile, zoneID string, records []dnsRecord) ([]dnsRecord, []recordWriteFailure) {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithSharedConfigProfile(profile))
	if err != nil {
		return nil, []recordWriteFailure{{Err: err}}
	}
	client := route53.NewFromConfig(cfg)

	var written []dnsRecord
	var failures []recordWriteFailure

	for _, r := range records {
		rrType, ok := route53RRType(r.Type)
		if !ok {
			failures = append(failures, recordWriteFailure{
				Record: r,
				Err:    fmt.Errorf("Route53 does not support %s records", r.Type),
			})
			continue
		}

		ttl := r.TTL
		if ttl <= 0 {
			ttl = 300
		}

		values := make([]types.ResourceRecord, 0, len(r.Values))
		for _, v := range r.Values {
			values = append(values, types.ResourceRecord{Value: aws.String(v)})
		}

		_, err := client.ChangeResourceRecordSets(ctx, &route53.ChangeResourceRecordSetsInput{
			HostedZoneId: aws.String(zoneID),
			ChangeBatch: &types.ChangeBatch{
				Comment: aws.String("copied by meroku during domain adoption"),
				Changes: []types.Change{{
					Action: types.ChangeActionUpsert,
					ResourceRecordSet: &types.ResourceRecordSet{
						Name:            aws.String(r.Name),
						Type:            rrType,
						TTL:             aws.Int64(ttl),
						ResourceRecords: values,
					},
				}},
			},
		})
		if err != nil {
			failures = append(failures, recordWriteFailure{Record: r, Err: err})
			continue
		}
		written = append(written, r)
	}

	return written, failures
}

type recordWriteFailure struct {
	Record dnsRecord
	Err    error
}

// route53RRType maps a record type string onto the SDK enum, rejecting anything
// Route53 will not accept rather than letting the API reject it later with a
// less specific message.
func route53RRType(t string) (types.RRType, bool) {
	switch strings.ToUpper(t) {
	case "A":
		return types.RRTypeA, true
	case "AAAA":
		return types.RRTypeAaaa, true
	case "CNAME":
		return types.RRTypeCname, true
	case "MX":
		return types.RRTypeMx, true
	case "TXT":
		return types.RRTypeTxt, true
	case "SRV":
		return types.RRTypeSrv, true
	case "CAA":
		return types.RRTypeCaa, true
	case "NS":
		return types.RRTypeNs, true
	case "PTR":
		return types.RRTypePtr, true
	case "NAPTR":
		return types.RRTypeNaptr, true
	case "SPF":
		// Deprecated by RFC 7208; the value belongs in a TXT record and Route53
		// still accepts the type, so carry it across rather than dropping it.
		return types.RRTypeSpf, true
	}
	return "", false
}

// adoptionSummary is what the UI reports after the copy.
type adoptionSummary struct {
	Zone     adoptedZone
	Snapshot zoneSnapshot
	Written  []dnsRecord
	Failures []recordWriteFailure
}

// adoptDomainIntoRoute53 runs the whole copy: discover, create, write.
//
// It deliberately stops short of anything that changes where the domain
// resolves. Nothing here is visible to the internet until the operator changes
// the nameservers at their registrar, which means this step is reversible by
// simply deleting the new zone.
func adoptDomainIntoRoute53(ctx context.Context, profile, domain string) (adoptionSummary, error) {
	domain = normalizeDomain(domain)

	currentNS, err := queryNameservers(domain)
	if err != nil || len(currentNS) == 0 {
		return adoptionSummary{}, fmt.Errorf("could not resolve the current nameservers for %s", domain)
	}

	snap, err := discoverZoneRecords(ctx, domain, currentNS)
	if err != nil {
		return adoptionSummary{}, err
	}

	zone, err := createOrFindApexZone(ctx, profile, domain)
	if err != nil {
		return adoptionSummary{Snapshot: snap}, err
	}

	written, failures := writeRecordsToZone(ctx, profile, zone.ZoneID, recordsToCopy(snap))

	return adoptionSummary{
		Zone:     zone,
		Snapshot: snap,
		Written:  written,
		Failures: failures,
	}, nil
}
