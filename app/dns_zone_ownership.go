package main

// dns_zone_ownership.go decides which hosted zone the DNS pre-flight checks.
//
// A zone created by meroku belongs to terraform state at zoneTargetAddress,
// and every record the apply writes goes into that zone. Route53 lets several
// public zones share one name, so a lookup by name can return a zone the
// apply never writes to: one left behind when state was cleared, for example.
// The pre-flight then approves a delegation to that zone while terraform
// creates another and writes the certificate validation record into it.
//
// So the zone comes from state first. A same-name zone that state does not
// own is imported rather than duplicated, and when the registry or parent
// delegates to a zone other than the one in state, that zone is imported in
// place of it: the delegation is what is hard to move, the zone is not.

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/route53"
)

// hostedZone is one public Route53 zone and its delegation set.
type hostedZone struct {
	ID          string
	Nameservers []string
}

// stateZone is what terraform state says about the domain zone.
// Known is false when state could not be read; nothing is concluded then.
type stateZone struct {
	Known  bool
	ZoneID string
}

// zoneChoice is the zone the pre-flight checks and what must happen to state
// before the apply writes to it.
type zoneChoice struct {
	Zone hostedZone

	// Adopt means Zone is not in state and must be imported first.
	Adopt bool
	// Replaces is the zone state holds now, dropped from state before the
	// import. It stays in AWS.
	Replaces string

	// Strays are the other public zones with the same name.
	Strays []string

	// Blocked explains why no zone can be chosen safely.
	Blocked string
}

// chooseZone decides which zone this environment uses.
//
// delegatedNS is the registry's or parent's delegation, read without a
// resolver cache, or nil when it could not be read.
func chooseZone(state stateZone, zones []hostedZone, delegatedNS []string) zoneChoice {
	if len(zones) == 0 {
		return zoneChoice{}
	}

	delegated := -1
	if len(delegatedNS) > 0 {
		for i, z := range zones {
			if nameserverSetsMatch(z.Nameservers, delegatedNS) {
				delegated = i
				break
			}
		}
	}
	owned := -1
	for i, z := range zones {
		if state.Known && state.ZoneID != "" && z.ID == state.ZoneID {
			owned = i
		}
	}

	pick := func(i int) zoneChoice {
		c := zoneChoice{Zone: zones[i]}
		for j, z := range zones {
			if j != i {
				c.Strays = append(c.Strays, z.ID)
			}
		}
		return c
	}

	switch {
	case !state.Known:
		// State unreadable (not initialised yet): fall back to the zone the
		// delegation points at, and import nothing.
		if delegated >= 0 {
			return pick(delegated)
		}
		return pick(0)

	case owned >= 0 && (delegated < 0 || delegated == owned):
		return pick(owned)

	case delegated >= 0:
		// The delegation points at a zone state does not hold: import it, in
		// place of whatever state holds.
		c := pick(delegated)
		c.Adopt = true
		c.Replaces = state.ZoneID
		return c

	case len(zones) == 1:
		c := pick(0)
		c.Adopt = true
		c.Replaces = state.ZoneID
		return c
	}

	ids := make([]string, len(zones))
	for i, z := range zones {
		ids[i] = z.ID
	}
	return zoneChoice{Blocked: fmt.Sprintf(
		"%d public zones share this name (%s), terraform state holds none of them and the delegation points at none of them; delete the ones not in use",
		len(zones), strings.Join(ids, ", "))}
}

var hostedZonesLookup = findHostedZonesByName

// findHostedZonesByName lists every public zone named zoneName in the account.
func findHostedZonesByName(ctx context.Context, profile, zoneName string) ([]hostedZone, error) {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithSharedConfigProfile(profile))
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config for profile %q: %w", profile, err)
	}
	client := route53.NewFromConfig(cfg)

	want := strings.TrimSuffix(strings.ToLower(zoneName), ".")
	resp, err := client.ListHostedZonesByName(ctx, &route53.ListHostedZonesByNameInput{
		DNSName: aws.String(zoneName),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list hosted zones: %w", err)
	}

	var zones []hostedZone
	for _, zone := range resp.HostedZones {
		if zone.Name == nil || zone.Id == nil {
			continue
		}
		// Private zones never serve the public internet.
		if zone.Config != nil && zone.Config.PrivateZone {
			continue
		}
		if strings.TrimSuffix(strings.ToLower(*zone.Name), ".") != want {
			continue
		}
		ns, nsErr := getZoneNameservers(ctx, client, *zone.Id)
		if nsErr != nil {
			return nil, fmt.Errorf("found zone %s but could not read its nameservers: %w", want, nsErr)
		}
		zones = append(zones, hostedZone{ID: strings.TrimPrefix(*zone.Id, "/hostedzone/"), Nameservers: ns})
	}
	return zones, nil
}

// envTerraformDir is env/<env> relative to the working directory, which is
// the project root during the deploy pre-flight and env/<env> after it.
func envTerraformDir(env string) string {
	if wd, err := os.Getwd(); err == nil &&
		filepath.Base(wd) == env && filepath.Base(filepath.Dir(wd)) == "env" {
		return "."
	}
	return filepath.Join("env", env)
}

var zoneIDInState = regexp.MustCompile(`(?m)^\s*zone_id\s*=\s*"([^"]+)"`)

// terraformInEnv runs terraform in dir with the environment's profile.
func terraformInEnv(dir, profile string, args ...string) (string, error) {
	cmd := exec.Command("terraform", append([]string{"-chdir=" + dir}, args...)...)
	cmd.Env = os.Environ()
	if profile != "" {
		cmd.Env = append(cmd.Env, "AWS_PROFILE="+profile)
	}
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return out.String(), err
}

// stateZoneLookup reads the domain zone's ID from terraform state.
var stateZoneLookup = func(e Env) stateZone {
	dir := envTerraformDir(e.Env)
	if _, err := os.Stat(filepath.Join(dir, ".terraform")); err != nil {
		return stateZone{}
	}
	list, err := terraformInEnv(dir, e.AWSProfile, "state", "list")
	if err != nil {
		return stateZone{}
	}
	if !stateHasAddress(list, zoneTargetAddress) {
		return stateZone{Known: true}
	}
	show, err := terraformInEnv(dir, e.AWSProfile, "state", "show", "-no-color", zoneTargetAddress)
	if err != nil {
		return stateZone{}
	}
	m := zoneIDInState.FindStringSubmatch(show)
	if m == nil {
		return stateZone{}
	}
	return stateZone{Known: true, ZoneID: m[1]}
}

func stateHasAddress(list, address string) bool {
	for _, line := range strings.Split(list, "\n") {
		if strings.TrimSpace(line) == address {
			return true
		}
	}
	return false
}

// zoneImporter makes terraform state own zoneID, dropping replaces first.
var zoneImporter = func(e Env, zoneID, replaces string) error {
	dir := envTerraformDir(e.Env)
	if replaces != "" {
		if out, err := terraformInEnv(dir, e.AWSProfile, "state", "rm", zoneTargetAddress); err != nil {
			return fmt.Errorf("terraform state rm %s: %w\n%s", zoneTargetAddress, err, lastLines(out, 12))
		}
	}
	out, err := terraformInEnv(dir, e.AWSProfile, "import", "-input=false", "-no-color", zoneTargetAddress, zoneID)
	if err != nil {
		return fmt.Errorf("terraform import %s %s: %w\n%s", zoneTargetAddress, zoneID, err, lastLines(out, 12))
	}
	return nil
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

// adoptDomainZone imports the zone the pre-flight chose. It needs an
// initialised env/<env>, so it runs terraform init there first when needed.
func adoptDomainZone(e Env, res dnsPreflightResult) error {
	dir := envTerraformDir(e.Env)
	if _, err := os.Stat(filepath.Join(dir, ".terraform")); err != nil {
		if out, err := terraformInEnv(dir, e.AWSProfile, "init", "-input=false", "-no-color"); err != nil {
			return fmt.Errorf("terraform init: %w\n%s", err, lastLines(out, 12))
		}
	}
	if res.AdoptReplaces != "" {
		fmt.Printf("   terraform state rm %s   (was %s)\n", zoneTargetAddress, res.AdoptReplaces)
	}
	fmt.Printf("   terraform import %s %s\n", zoneTargetAddress, res.ZoneID)
	return zoneImporter(e, res.ZoneID, res.AdoptReplaces)
}
