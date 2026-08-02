package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// zoneTargetAddress is passed to `terraform apply -target=...` for phase 1 of a
// two-phase deploy. Terraform does NOT error when a target matches nothing — it
// prints "No changes" and a generic targeting warning, then exits 0 (verified
// against terraform 1.x with a deliberately bogus address). So a drifted address
// fails silently, and this test is the only cheap guard against that.
//
// runDNSBootstrapAndDelegate additionally verifies the zone exists in AWS after
// phase 1, which catches drift at runtime.
func TestZoneTargetAddress_MatchesDomainModule(t *testing.T) {
	path := filepath.Join("..", "modules", "domain", "main.tf")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("cannot read %s: %v", path, err)
	}
	src := string(body)

	// The address is module.domain.aws_route53_zone.domain[0]; each part must hold.
	if !strings.Contains(zoneTargetAddress, "aws_route53_zone.domain") {
		t.Fatalf("unexpected target address %q", zoneTargetAddress)
	}

	zoneBlock := regexp.MustCompile(`(?s)resource\s+"aws_route53_zone"\s+"domain"\s*\{(.*?)\n\}`)
	m := zoneBlock.FindStringSubmatch(src)
	if m == nil {
		t.Fatalf(`modules/domain/main.tf no longer declares resource "aws_route53_zone" "domain" — `+
			`the phase-1 target %q will silently match nothing`, zoneTargetAddress)
	}

	// The [0] index is only correct while the resource uses count.
	hasCount := strings.Contains(m[1], "count")
	hasIndex := strings.HasSuffix(zoneTargetAddress, "[0]")

	switch {
	case hasCount && !hasIndex:
		t.Errorf("aws_route53_zone.domain uses count, so the target must end in [0]; got %q", zoneTargetAddress)
	case !hasCount && hasIndex:
		t.Errorf("aws_route53_zone.domain no longer uses count, so the target must not end in [0]; got %q", zoneTargetAddress)
	}
}

// Phase 1 must target the zone alone. Targeting the whole module would pull in
// the ACM certificates and their validation — the exact resources that cannot
// succeed until delegation exists, which is what the split is for.
func TestZoneTargetAddress_IsZoneOnlyNotWholeModule(t *testing.T) {
	if zoneTargetAddress == "module.domain" {
		t.Fatal("phase 1 must not target the whole domain module: it would include " +
			"aws_acm_certificate_validation, which cannot complete before delegation")
	}
	if !strings.HasPrefix(zoneTargetAddress, "module.domain.") {
		t.Errorf("expected a resource inside module.domain, got %q", zoneTargetAddress)
	}
	for _, forbidden := range []string{"acm", "certificate", "record"} {
		if strings.Contains(strings.ToLower(zoneTargetAddress), forbidden) {
			t.Errorf("phase 1 target must be the hosted zone only, got %q", zoneTargetAddress)
		}
	}
}

// The zone must remain dependency-free, or -target would drag extra resources in.
func TestDomainZone_HasNoDependencies(t *testing.T) {
	path := filepath.Join("..", "modules", "domain", "main.tf")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("cannot read %s: %v", path, err)
	}

	zoneBlock := regexp.MustCompile(`(?s)resource\s+"aws_route53_zone"\s+"domain"\s*\{(.*?)\n\}`)
	m := zoneBlock.FindStringSubmatch(string(body))
	if m == nil {
		t.Fatal("could not find the aws_route53_zone.domain block")
	}
	block := m[1]

	// References to other resources/data/modules would widen the -target set.
	for _, ref := range []string{"aws_acm_", "data.aws_route53", "module.", "depends_on"} {
		if strings.Contains(block, ref) {
			t.Errorf("aws_route53_zone.domain now references %q; phase 1 -target would no "+
				"longer create just the zone:\n%s", ref, block)
		}
	}
}
