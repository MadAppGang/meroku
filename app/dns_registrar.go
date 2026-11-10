package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/aws/aws-sdk-go-v2/service/route53domains"
	r53dtypes "github.com/aws/aws-sdk-go-v2/service/route53domains/types"
	"github.com/aws/smithy-go"
)

// A root domain has no parent zone to hold an NS record. It is delegated by the
// nameserver list at its registrar. When the domain is registered with Route 53
// Domains in an account one of the local profiles reaches, meroku sets that list
// itself.

// route53DomainsRegion is the only region the Route 53 Domains API serves.
const route53DomainsRegion = "us-east-1"

// domainRegistration is a domain found registered with Route 53 Domains.
type domainRegistration struct {
	Profile   string
	AccountID string
	Domain    string

	// Nameservers is the list the registrar holds now.
	Nameservers []string

	// LiveRecords counts the records, other than NS and SOA, in the Route53 zone
	// the registrar points at now. They stop resolving once the nameservers
	// change. -1 means meroku could not tell: the nameservers are not a zone
	// in the registrant account.
	LiveRecords int

	// OldZoneID is that zone, when it was found.
	OldZoneID string
}

// registrarAction is what the screen does with a registration it found.
type registrarAction int

const (
	// registrarAlreadySet: the registrar already lists the zone's nameservers.
	registrarAlreadySet registrarAction = iota
	// registrarAutoSwitch: nothing resolves through the current nameservers, so
	// switching them loses nothing and needs no question.
	registrarAutoSwitch
	// registrarConfirmSwitch: the current nameservers serve records, or meroku
	// cannot tell, so the operator confirms.
	registrarConfirmSwitch
)

func decideRegistrarAction(reg domainRegistration, zoneNS []string) registrarAction {
	if nameserverSetsMatch(reg.Nameservers, zoneNS) {
		return registrarAlreadySet
	}
	if reg.LiveRecords == 0 {
		return registrarAutoSwitch
	}
	return registrarConfirmSwitch
}

// registrationLookup returns the domain's registration as profile sees it, or
// nil when the domain is not registered in that profile's account. A variable
// so tests can replace it.
var registrationLookup = func(ctx context.Context, profile, domain string) (*domainRegistration, error) {
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithSharedConfigProfile(profile), config.WithRegion(route53DomainsRegion))
	if err != nil {
		return nil, err
	}
	out, err := route53domains.NewFromConfig(cfg).GetDomainDetail(ctx,
		&route53domains.GetDomainDetailInput{DomainName: aws.String(domain)})
	if err != nil {
		if isDomainNotInAccount(err) {
			return nil, nil
		}
		return nil, err
	}
	reg := &domainRegistration{Profile: profile, Domain: domain, LiveRecords: -1}
	for _, ns := range out.Nameservers {
		if ns.Name != nil {
			reg.Nameservers = append(reg.Nameservers, *ns.Name)
		}
	}
	reg.AccountID = lookupAccountID(ctx, profile)
	reg.OldZoneID, reg.LiveRecords = countRecordsBehindNameservers(ctx, cfg, domain, reg.Nameservers)
	return reg, nil
}

// isDomainNotInAccount recognises Route 53 Domains' answer for a domain the
// account does not hold: InvalidInput, "Domain ... not found in account".
func isDomainNotInAccount(err error) bool {
	var apiErr smithy.APIError
	return errors.As(err, &apiErr) && apiErr.ErrorCode() == "InvalidInput" &&
		strings.Contains(apiErr.ErrorMessage(), "not found in account")
}

// countRecordsBehindNameservers finds the account's public zone for domain that
// nameservers names, and counts its records other than NS and SOA. The count is
// -1 when no such zone is found or it cannot be read.
func countRecordsBehindNameservers(ctx context.Context, cfg aws.Config, domain string, nameservers []string) (string, int) {
	client := route53.NewFromConfig(cfg)
	zones, err := client.ListHostedZonesByName(ctx, &route53.ListHostedZonesByNameInput{
		DNSName: aws.String(domain),
	})
	if err != nil {
		return "", -1
	}
	want := strings.TrimSuffix(strings.ToLower(domain), ".")
	for _, z := range zones.HostedZones {
		if z.Id == nil || z.Name == nil || (z.Config != nil && z.Config.PrivateZone) ||
			strings.TrimSuffix(strings.ToLower(*z.Name), ".") != want {
			continue
		}
		ns, err := getZoneNameservers(ctx, client, *z.Id)
		if err != nil || !nameserverSetsMatch(ns, nameservers) {
			continue
		}
		count := 0
		pages := route53.NewListResourceRecordSetsPaginator(client,
			&route53.ListResourceRecordSetsInput{HostedZoneId: z.Id})
		for pages.HasMorePages() {
			page, err := pages.NextPage(ctx)
			if err != nil {
				return *z.Id, -1
			}
			for _, rr := range page.ResourceRecordSets {
				if rr.Type != "NS" && rr.Type != "SOA" {
					count++
				}
			}
		}
		return *z.Id, count
	}
	return "", -1
}

// switchRegistrarNameservers points the registration at nameservers.
//
// The zone the registrar pointed at before is left alone. Resolvers that cached
// the old nameservers keep asking them until the TLD's TTL runs out, and get a
// failure either way: "no such record" while that zone exists, a refusal once it
// is deleted. Deleting it gains nothing.
func switchRegistrarNameservers(ctx context.Context, reg domainRegistration, nameservers []string) error {
	return registrarNameserverWriter(ctx, reg.Profile, reg.Domain, nameservers)
}

// registrarNameserverWriter replaces the domain's nameservers at Route 53
// Domains. A variable so tests can replace it.
var registrarNameserverWriter = func(ctx context.Context, profile, domain string, nameservers []string) error {
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithSharedConfigProfile(profile), config.WithRegion(route53DomainsRegion))
	if err != nil {
		return err
	}
	ns := make([]r53dtypes.Nameserver, 0, len(nameservers))
	for _, n := range nameservers {
		ns = append(ns, r53dtypes.Nameserver{Name: aws.String(strings.TrimSuffix(n, "."))})
	}
	_, err = route53domains.NewFromConfig(cfg).UpdateDomainNameservers(ctx,
		&route53domains.UpdateDomainNameserversInput{DomainName: aws.String(domain), Nameservers: ns})
	return err
}

// registrationScan is the result of asking every profile for the registration.
type registrationScan struct {
	Registration *domainRegistration
	// Unreachable lists the profiles that could not be asked (expired SSO, no
	// permission). The registration may sit behind one of them.
	Unreachable []string
}

// findDomainRegistration asks every profile, concurrently, whether its account
// holds domain's registration. A domain is registered exactly once, so the first
// registration found is the answer.
func findDomainRegistration(ctx context.Context, profiles []string, domain string) registrationScan {
	var (
		mu   sync.Mutex
		scan registrationScan
		wg   sync.WaitGroup
		sem  = make(chan struct{}, 6)
	)
	for _, profile := range profiles {
		wg.Add(1)
		go func(profile string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			pctx, cancel := context.WithTimeout(ctx, 20*time.Second)
			defer cancel()
			reg, err := registrationLookup(pctx, profile, domain)

			mu.Lock()
			defer mu.Unlock()
			switch {
			case err != nil:
				scan.Unreachable = append(scan.Unreachable, profile)
			case reg != nil && scan.Registration == nil:
				scan.Registration = reg
			}
		}(profile)
	}
	wg.Wait()
	return scan
}

// rootDomainManualReason explains why meroku cannot set the nameservers itself.
func rootDomainManualReason(domain string, unreachable []string) string {
	reason := fmt.Sprintf(
		"%s is a root domain, and none of your AWS profiles holds its registration. "+
			"Set these four nameservers at the registrar where it was bought.", domain)
	if len(unreachable) > 0 {
		reason += fmt.Sprintf(" %d profile(s) could not be checked (%s); if the domain is "+
			"registered there, log in and press r.", len(unreachable), strings.Join(unreachable, ", "))
	}
	return reason
}
