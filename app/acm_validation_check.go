package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/acm"
	acmtypes "github.com/aws/aws-sdk-go-v2/service/acm/types"
	"github.com/miekg/dns"
)

// ACM looks the validation record up through AWS's own resolvers, which nothing
// outside AWS can query. Two things come close: ACM's own verdict, read from
// DescribeCertificate, and the lookup a resolver with an empty cache makes —
// the registry for the delegation, then those nameservers for the record. When
// ACM still says pending, the lookup says whether the record is the reason.

// acmValidation is one domain of one certificate, as ACM reports it.
type acmValidation struct {
	CertARN       string
	Region        string
	Domain        string
	Status        string // PENDING_VALIDATION, SUCCESS, FAILED
	FailureReason string
	RecordName    string
	RecordValue   string
}

type certVerdict int

const (
	certIssued certVerdict = iota
	certFailed
	// certUnchecked: ACM is pending and this network cannot reach the
	// nameservers directly to say why.
	certUnchecked
	// certWaitingOnACM: the record is correct at the authoritative servers and
	// ACM has not looked again yet.
	certWaitingOnACM
	certRecordMissing
	certRecordWrong
	certNotDelegated
	// certOutsideZone: the record lies outside this environment's zone, so
	// meroku does not know which nameservers to ask.
	certOutsideZone
)

// certCheck is the verdict for one acmValidation.
type certCheck struct {
	acmValidation
	Verdict certVerdict
	Detail  string
}

// acmValidationLookup returns the validations of the account's certificates for
// zone that are pending, plus those of the certificates in also. A variable so
// tests can replace it.
var acmValidationLookup = func(ctx context.Context, profile string, regions []string, zone string, also []string) ([]acmValidation, error) {
	var out []acmValidation
	wanted := map[string]bool{}
	for _, arn := range also {
		wanted[arn] = true
	}
	for _, region := range regions {
		cfg, err := config.LoadDefaultConfig(ctx,
			config.WithSharedConfigProfile(profile), config.WithRegion(region))
		if err != nil {
			return nil, err
		}
		client := acm.NewFromConfig(cfg)
		pages := acm.NewListCertificatesPaginator(client, &acm.ListCertificatesInput{
			CertificateStatuses: []acmtypes.CertificateStatus{acmtypes.CertificateStatusPendingValidation},
		})
		arns := []string{}
		for pages.HasMorePages() {
			page, err := pages.NextPage(ctx)
			if err != nil {
				return nil, fmt.Errorf("listing certificates in %s: %w", region, err)
			}
			for _, c := range page.CertificateSummaryList {
				if c.CertificateArn != nil && c.DomainName != nil && inZone(*c.DomainName, zone) {
					arns = append(arns, *c.CertificateArn)
				}
			}
		}
		for arn := range wanted {
			if strings.Contains(arn, ":"+region+":") && !slices.Contains(arns, arn) {
				arns = append(arns, arn)
			}
		}
		for _, arn := range arns {
			d, err := client.DescribeCertificate(ctx, &acm.DescribeCertificateInput{CertificateArn: aws.String(arn)})
			if err != nil || d.Certificate == nil {
				continue
			}
			for _, o := range d.Certificate.DomainValidationOptions {
				v := acmValidation{
					CertARN: arn, Region: region,
					Domain: aws.ToString(o.DomainName),
					Status: string(o.ValidationStatus),
				}
				if d.Certificate.FailureReason != "" {
					v.FailureReason = string(d.Certificate.FailureReason)
				}
				if o.ResourceRecord != nil {
					v.RecordName = aws.ToString(o.ResourceRecord.Name)
					v.RecordValue = aws.ToString(o.ResourceRecord.Value)
				}
				out = append(out, v)
			}
		}
	}
	return out, nil
}

// inZone reports whether name is zone or a name under it (a wildcard counts).
func inZone(name, zone string) bool {
	name = strings.TrimPrefix(strings.TrimSuffix(strings.ToLower(name), "."), "*.")
	zone = strings.TrimSuffix(strings.ToLower(zone), ".")
	return zone != "" && (name == zone || strings.HasSuffix(name, "."+zone))
}

// authoritativeCNAME asks nameservers directly, without recursion, for name's
// CNAME. It returns the first answer any of them gives.
var authoritativeCNAME = func(name string, nameservers []string) (string, bool, error) {
	q := new(dns.Msg)
	q.SetQuestion(dns.Fqdn(name), dns.TypeCNAME)
	q.RecursionDesired = false
	c := &dns.Client{Timeout: 4 * time.Second}

	var lastErr error
	answered := false
	for _, ns := range nameservers {
		resp, _, err := c.Exchange(q, net.JoinHostPort(strings.TrimSuffix(ns, "."), "53"))
		if err != nil {
			lastErr = err
			continue
		}
		if resp.RecursionAvailable || !resp.Authoritative {
			return "", false, errPort53Intercepted
		}
		answered = true
		for _, rr := range resp.Answer {
			if cname, ok := rr.(*dns.CNAME); ok {
				return strings.TrimSuffix(cname.Target, "."), true, nil
			}
		}
	}
	if !answered && lastErr != nil {
		return "", false, lastErr
	}
	return "", false, nil
}

// judgeValidation decides what one validation's state means.
func judgeValidation(v acmValidation, zone string) certCheck {
	c := certCheck{acmValidation: v}
	switch v.Status {
	case "SUCCESS":
		c.Verdict, c.Detail = certIssued, "ACM validated "+v.Domain
		return c
	case "FAILED":
		c.Verdict = certFailed
		c.Detail = fmt.Sprintf("ACM gave up on %s (%s). Only a new certificate request validates again.",
			v.Domain, strings.ToLower(strings.ReplaceAll(v.FailureReason, "_", " ")))
		return c
	}
	if v.RecordName == "" {
		c.Verdict, c.Detail = certWaitingOnACM, "ACM has not issued the validation record for "+v.Domain+" yet"
		return c
	}
	if !inZone(v.RecordName, zone) {
		c.Verdict = certOutsideZone
		c.Detail = fmt.Sprintf("%s lies outside %s; meroku cannot tell which nameservers to ask", v.RecordName, zone)
		return c
	}

	delegated, err := delegationLookup(zone)
	if errors.Is(err, errPort53Intercepted) {
		c.Verdict = certUnchecked
		c.Detail = "ACM is still checking; this network intercepts DNS, so meroku cannot ask the nameservers directly"
		return c
	}
	if err != nil || len(delegated) == 0 {
		c.Verdict = certNotDelegated
		c.Detail = fmt.Sprintf("the registry does not delegate %s to any nameservers yet", zone)
		return c
	}
	got, found, err := authoritativeCNAME(v.RecordName, delegated)
	switch {
	case errors.Is(err, errPort53Intercepted):
		c.Verdict = certUnchecked
		c.Detail = "ACM is still checking; this network intercepts DNS, so meroku cannot ask the nameservers directly"
	case err != nil:
		c.Verdict = certNotDelegated
		c.Detail = fmt.Sprintf("the nameservers %s is delegated to did not answer: %s", zone, shortError(err))
	case !found:
		c.Verdict = certRecordMissing
		c.Detail = fmt.Sprintf("%s's nameservers (%s) have no %s record — the zone ACM reads is not the one meroku wrote to",
			zone, normalizeNameservers(delegated)[0], strings.TrimSuffix(v.RecordName, "."))
	case !strings.EqualFold(got, strings.TrimSuffix(v.RecordValue, ".")):
		c.Verdict = certRecordWrong
		c.Detail = fmt.Sprintf("%s points at %s, ACM expects %s",
			strings.TrimSuffix(v.RecordName, "."), got, strings.TrimSuffix(v.RecordValue, "."))
	default:
		c.Verdict = certWaitingOnACM
		c.Detail = "the record is correct at " + zone + "'s nameservers; ACM has not checked again yet"
	}
	return c
}

// checkCertificateValidation reads ACM's state for the environment's
// certificates and judges each validation.
func checkCertificateValidation(ctx context.Context, e Env, also []string) ([]certCheck, error) {
	zone := expectedZoneName(e)
	if zone == "" {
		return nil, nil
	}
	regions := []string{e.Region}
	if e.Region != "us-east-1" {
		// CloudFront and Amplify certificates live in us-east-1.
		regions = append(regions, "us-east-1")
	}
	vals, err := acmValidationLookup(ctx, e.AWSProfile, regions, zone, also)
	if err != nil {
		return nil, err
	}
	// The zone's own nameservers, to tell resolvers holding them from resolvers
	// holding an older set.
	var zoneNS []string
	if _, ns, err := findHostedZoneByName(ctx, e.AWSProfile, zone); err == nil {
		zoneNS = ns
	}

	checks := make([]certCheck, 0, len(vals))
	stale := -1
	for _, v := range vals {
		c := judgeValidation(v, zone)
		if (c.Verdict == certWaitingOnACM || c.Verdict == certUnchecked) && len(zoneNS) > 0 {
			if stale < 0 {
				_, stale = countVerdicts(propagationCheck(zone, zoneNS))
			}
			if stale > 0 {
				c.Detail += fmt.Sprintf(". %d of %d public resolvers still hold an older delegation of %s "+
					"and fail the lookup; resolvers AWS uses may too, until that cache expires",
					stale, len(dohResolvers), zone)
			}
		}
		checks = append(checks, c)
	}
	sort.SliceStable(checks, func(a, b int) bool {
		return verdictSeverity(checks[a].Verdict) > verdictSeverity(checks[b].Verdict)
	})
	return checks, nil
}

// propagationCheck asks the public resolvers over HTTPS. A variable so tests
// can replace it.
var propagationCheck = checkPropagationDoH

// verdictSeverity orders verdicts by how much they need attention.
func verdictSeverity(v certVerdict) int {
	switch v {
	case certFailed:
		return 7
	case certNotDelegated:
		return 6
	case certRecordMissing:
		return 5
	case certRecordWrong:
		return 4
	case certOutsideZone:
		return 3
	case certUnchecked:
		return 2
	case certWaitingOnACM:
		return 1
	default:
		return 0
	}
}

// summarizeCertChecks is a one-line state for the apply screen: the worst
// verdict first, because it is the one that decides what happens next.
func summarizeCertChecks(checks []certCheck) string {
	if len(checks) == 0 {
		return ""
	}
	issued := 0
	for _, c := range checks {
		if c.Verdict == certIssued {
			issued++
		}
	}
	if issued == len(checks) {
		return fmt.Sprintf("ACM: all %d validated", issued)
	}
	worst := checks[0]
	return fmt.Sprintf("ACM: %d/%d validated · %s", issued, len(checks), worst.Detail)
}
