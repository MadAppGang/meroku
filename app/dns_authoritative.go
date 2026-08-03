package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/aws/aws-sdk-go-v2/service/route53/types"
)

// Authoritative delegation checks.
//
// Everything else in this package asks a recursive resolver whether a
// delegation exists. That answer cannot distinguish "the parent delegates this
// subdomain" from "a resolver still remembers that it used to", and the
// difference is the whole question.
//
// It bit us exactly that way. sploty.app was hosted at Hover with a
// dev.sploty.app NS record; the domain moved to Route53 and the record did not
// come with it. Resolvers kept serving the old delegation from cache, meroku
// asked a resolver, saw the expected nameservers, and reported the subdomain
// delegated. It was not: the parent zone held five records and none of them was
// that NS. The certificate then failed validation for the entirely correct
// reason that the name did not resolve for anyone without a warm cache.
//
// A DNS lookup also cannot be trusted to reach the server it names. On a network
// that intercepts port 53 — which is common, and was true of the machine this
// was diagnosed on — `dig @some.authoritative.server` is answered by the local
// resolver instead, so even querying the parent's own nameservers returns cache.
// The only check immune to both problems is reading the zone through the
// Route53 API.

// delegationCheck is the outcome of asking the parent zone directly.
type delegationCheck struct {
	// Present is true when the parent zone contains an NS record for the
	// subdomain at all.
	Present bool
	// Matches is true when that record points at the expected nameservers.
	Matches bool
	// Observed is what the parent zone actually delegates to.
	Observed []string
}

// verifyDelegationInParentZone reads the parent hosted zone and reports whether
// it really delegates the subdomain to the expected nameservers.
//
// This is the only delegation check that means anything when we own the parent:
// it reads the zone rather than asking what the internet currently believes.
func verifyDelegationInParentZone(ctx context.Context, profile, parentZoneID, subdomain string, expected []string) (delegationCheck, error) {
	if strings.TrimSpace(profile) == "" {
		return delegationCheck{}, fmt.Errorf("no AWS profile for the parent zone")
	}
	if strings.TrimSpace(parentZoneID) == "" {
		return delegationCheck{}, fmt.Errorf("no parent zone id")
	}

	cfg, err := config.LoadDefaultConfig(ctx, config.WithSharedConfigProfile(profile))
	if err != nil {
		return delegationCheck{}, fmt.Errorf("failed to load AWS config for %q: %w", profile, err)
	}
	client := route53.NewFromConfig(cfg)

	want := normalizeDomain(subdomain)

	// StartRecordName positions the paginated listing at the name we care about
	// rather than walking the whole zone, but the API can still return records
	// before it, so the name is checked rather than assumed.
	out, err := client.ListResourceRecordSets(ctx, &route53.ListResourceRecordSetsInput{
		HostedZoneId:    aws.String(parentZoneID),
		StartRecordName: aws.String(subdomain),
		StartRecordType: types.RRTypeNs,
		MaxItems:        aws.Int32(10),
	})
	if err != nil {
		return delegationCheck{}, fmt.Errorf("could not read parent zone %s: %w", parentZoneID, err)
	}

	for _, rr := range out.ResourceRecordSets {
		if rr.Type != types.RRTypeNs || rr.Name == nil {
			continue
		}
		if normalizeDomain(*rr.Name) != want {
			continue
		}

		observed := make([]string, 0, len(rr.ResourceRecords))
		for _, v := range rr.ResourceRecords {
			observed = append(observed, aws.ToString(v.Value))
		}
		return delegationCheck{
			Present:  true,
			Matches:  nameserverSetsMatch(expected, observed),
			Observed: observed,
		}, nil
	}

	return delegationCheck{}, nil
}

// describeDelegationCheck turns the result into something worth showing.
func describeDelegationCheck(c delegationCheck, subdomain, parentDomain string) string {
	switch {
	case c.Present && c.Matches:
		return ""
	case c.Present:
		return fmt.Sprintf(
			"%s is delegated in %s, but to different nameservers (%s) — the record "+
				"needs updating, not adding",
			subdomain, parentDomain, strings.Join(normalizeNameservers(c.Observed), ", "))
	default:
		return fmt.Sprintf(
			"the %s zone contains no NS record for %s. Resolvers may still answer "+
				"for it from cache, but that expires and nothing new can resolve it",
			parentDomain, subdomain)
	}
}
