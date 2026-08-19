package recommend

import "testing"

// Rule R10, the family allowlist (family.go).
//
// CON-5: every instance-type NAME below is a public catalog fact; no price,
// account or region detail here came from a live environment.

// TestFamily_ParsePrefix covers the shapes the real catalog actually carries.
// A parse that reads "mac1" as "m" or "trn1" as "t" would silently re-admit
// the exact families R10 exists to exclude, so the table is deliberately
// heavier on near-collisions than on ordinary types.
func TestFamily_ParsePrefix(t *testing.T) {
	cases := []struct {
		name       string
		wantFamily string
		wantGen    int
	}{
		// Ordinary, and the two suffix conventions AWS uses.
		{"m7i.large", "m", 7},
		{"m7i-flex.large", "m", 7},
		{"c7g.medium", "c", 7},
		{"c7gd.xlarge", "c", 7},
		{"r7iz.large", "r", 7},
		{"r6idn.32xlarge", "r", 6},
		{"t4g.small", "t", 4},
		{"m8g.24xlarge", "m", 8},

		// Near-collisions with the four eligible letters. Each of these must
		// parse to its FULL alphabetic run, never to its first character.
		{"mac1.metal", "mac", 1},
		{"mac2-m2.metal", "mac", 2},
		{"trn1.32xlarge", "trn", 1},
		{"trn2.48xlarge", "trn", 2},
		{"c5n.large", "c", 5},

		// The excluded categories.
		{"inf1.24xlarge", "inf", 1},
		{"inf2.xlarge", "inf", 2},
		{"p5.48xlarge", "p", 5},
		{"g5.xlarge", "g", 5},
		{"g5g.xlarge", "g", 5},
		{"dl1.24xlarge", "dl", 1},
		{"vt1.3xlarge", "vt", 1},
		{"f1.4xlarge", "f", 1},
		{"i4i.xlarge", "i", 4},
		{"i3en.large", "i", 3},
		{"im4gn.large", "im", 4},
		{"is4gen.medium", "is", 4},
		{"d3en.xlarge", "d", 3},
		{"h1.16xlarge", "h", 1},
		{"x2iedn.xlarge", "x", 2},
		{"x2gd.medium", "x", 2},
		{"z1d.large", "z", 1},
		{"hpc7a.48xlarge", "hpc", 7},

		// The hyphenated high-memory naming: the run stops at '-', so the
		// family is "u" and no generation parses at all.
		{"u-6tb1.56xlarge", "u", 0},
		{"u-24tb1.112xlarge", "u", 0},

		// Degenerate input. Both must be handled without panicking, and both
		// land outside the allowlist, which is the fail-closed direction.
		{"", "", 0},
		{"garbage", "garbage", 0},
		{".large", "", 0},
		{"7i.large", "", 7},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotFamily, gotGen := ParseFamilyGeneration(tc.name)
			if gotFamily != tc.wantFamily || gotGen != tc.wantGen {
				t.Errorf("ParseFamilyGeneration(%q) = (%q,%d), want (%q,%d)",
					tc.name, gotFamily, gotGen, tc.wantFamily, tc.wantGen)
			}
			if got := familyOf(tc.name); got != tc.wantFamily {
				t.Errorf("familyOf(%q) = %q, want %q", tc.name, got, tc.wantFamily)
			}
		})
	}
}

// TestFamily_Eligibility is the allowlist as a table. The eligible half is
// short by design; the ineligible half is long, because an allowlist's value
// is entirely in what it refuses.
func TestFamily_Eligibility(t *testing.T) {
	eligible := []string{
		"m7i.large", "m7i-flex.large", "m6a.2xlarge", "m8g.xlarge",
		"c7i.large", "c7g.medium", "c6in.4xlarge",
		"r7i.large", "r7iz.2xlarge", "r8g.xlarge",
		"t3.medium", "t3a.small", "t4g.nano",
	}
	for _, name := range eligible {
		if !familyEligible(name, nil) {
			t.Errorf("%s is not eligible; it is a general-purpose, compute-optimised, "+
				"memory-optimised or burstable type", name)
		}
	}

	ineligible := []string{
		// Accelerators. inf and trn are the ones no other rule can see.
		"inf1.24xlarge", "inf2.48xlarge", "trn1.32xlarge", "trn1n.32xlarge",
		"p4d.24xlarge", "p5.48xlarge", "g5.xlarge", "g6e.48xlarge",
		"dl1.24xlarge", "vt1.24xlarge", "f1.16xlarge", "f2.48xlarge",
		// Storage optimised.
		"i4i.32xlarge", "i3en.24xlarge", "im4gn.16xlarge", "is4gen.8xlarge",
		"d3en.12xlarge", "h1.16xlarge",
		// AWS files these under "memory optimized" and they are still out:
		// memory APPLIANCES, not container hosts.
		"x2iedn.32xlarge", "x2gd.16xlarge", "z1d.12xlarge",
		"u-6tb1.56xlarge", "u-24tb1.112xlarge",
		// HPC and Mac.
		"hpc7a.96xlarge", "hpc6id.32xlarge", "mac1.metal", "mac2-m2pro.metal",
		// Degenerate.
		"", "garbage", ".large",
		// A family AWS has not shipped. This is the whole argument for an
		// allowlist: a denylist would admit it, and nobody would find out
		// until it was already being recommended.
		"q1.large", "qq9zz.48xlarge",
	}
	for _, name := range ineligible {
		if familyEligible(name, nil) {
			t.Errorf("%s is eligible; R10 must fail closed on anything outside m/c/r/t", name)
		}
	}
}

// TestFamily_PinnedTypesOptIn is the escape hatch: an operator who names an
// exact type in the pool's instance_types gets it scored.
func TestFamily_PinnedTypesOptIn(t *testing.T) {
	pinned := pinnedTypeSet([]string{"inf1.24xlarge", "  X2GD.16XLARGE  ", "", "   "})

	if !familyEligible("inf1.24xlarge", pinned) {
		t.Error("a type named in instance_types is not eligible; the opt-in does nothing")
	}
	if !familyEligible("x2gd.16xlarge", pinned) {
		t.Error("the opt-in is case- and whitespace-sensitive; hand-edited YAML should still work")
	}
	// The opt-in is by TYPE, not by family: naming one inf1 size does not
	// admit the rest of the family the operator never mentioned.
	if familyEligible("inf1.6xlarge", pinned) {
		t.Error("pinning inf1.24xlarge admitted inf1.6xlarge; the opt-in must not widen to the family")
	}
	if familyEligible("x2gd.medium", pinned) {
		t.Error("pinning x2gd.16xlarge admitted x2gd.medium")
	}
	if pinnedTypeSet(nil) != nil || pinnedTypeSet([]string{}) != nil {
		t.Error("an empty instance_types must produce no pin set at all")
	}
}

// acceleratorCatalog is the base catalog plus the two records that motivated
// R10: an Inferentia type that reports NO GpuInfo and NO AcceleratorInfo (so
// FR-21.5 cannot see it), and a storage-optimised type. Both are current
// generation, x86, priced, and dense enough to win on cost per task slot --
// which is exactly why they got recommended.
//
// CON-5: prices are invented round numbers, not a live quote.
func acceleratorCatalog() []InstanceType {
	return append(baseCatalog(),
		InstanceType{
			// 96 vCPU / 192 GiB. GPUCount is 0 because the live API really
			// does report null GpuInfo for inf1 -- that is the finding.
			Name: "inf1.24xlarge", VCPU: 96, MemoryMiB: 196608,
			Architectures: []string{ArchX8664}, CurrentGeneration: true,
			GPUCount: 0, MaxNetworkInterfaces: 15, SupportsSpot: true,
			OnDemandHourly: fp(4.7200), SpotMedianHourly: fp(1.4200),
		},
		InstanceType{
			Name: "i4i.2xlarge", VCPU: 8, MemoryMiB: 65536,
			Architectures: []string{ArchX8664}, CurrentGeneration: true,
			GPUCount: 0, MaxNetworkInterfaces: 4, SupportsSpot: true,
			OnDemandHourly: fp(0.6860), SpotMedianHourly: fp(0.2100),
		},
	)
}

// TestFamily_AcceleratorNeverRanked is the regression for the reported defect:
// on the real ap-southeast-2 catalog, posture=cost-first returned
// inf1.24xlarge as the primary recommendation for a general web workload.
func TestFamily_AcceleratorNeverRanked(t *testing.T) {
	for _, posture := range []Posture{PosturePerformance, PostureBalanced, PostureCost} {
		t.Run(string(posture), func(t *testing.T) {
			in := baseInput()
			in.Catalog = acceleratorCatalog()
			in.Posture = posture
			in.Limit = 50
			res := Recommend(in)

			for _, c := range res.Ranked {
				if c.InstanceType == "inf1.24xlarge" || c.InstanceType == "i4i.2xlarge" {
					t.Errorf("%s is ranked at total %v; R10 must EXCLUDE, not down-weight",
						c.InstanceType, c.Total)
				}
			}
			if res.Primary == nil {
				t.Fatalf("no primary: %s", res.Constraint)
			}
			if fam := familyOf(res.Primary.InstanceType); !eligibleFamilies[fam] {
				t.Errorf("primary %s is in family %q, which is not on the allowlist",
					res.Primary.InstanceType, fam)
			}
			for _, name := range res.SuggestedPool.InstanceTypes {
				if fam := familyOf(name); !eligibleFamilies[fam] {
					t.Errorf("suggested_pool.instance_types carries %s (family %q)", name, fam)
				}
			}
		})
	}
}

// TestFamily_ExclusionIsVisible: the user has to be able to see WHY a type
// they expected is absent, rather than wondering.
func TestFamily_ExclusionIsVisible(t *testing.T) {
	pool := NormalizePool(PoolConstraints{})
	_, misses := preFilter(normalizeCatalog(acceleratorCatalog()), pool, false)

	for _, name := range []string{"inf1.24xlarge", "i4i.2xlarge"} {
		m, ok := missFor(misses, name)
		if !ok {
			t.Errorf("%s produced no Miss at all; an invisible exclusion is the thing "+
				"nearestMisses exists to prevent", name)
			continue
		}
		if m.FailedRule != RuleFamilyNotEligible {
			t.Errorf("%s failed %q, want %q", name, m.FailedRule, RuleFamilyNotEligible)
		}
		// No numeric dimension, exactly as for R1/R3/R4.
		if m.Needed != 0 || m.Available != 0 || m.Unit != "" {
			t.Errorf("%s Miss carries numbers it cannot justify: %+v", name, m)
		}
	}
}

// TestFamily_R10PrecedesTheRatioRange is why R10 lives in preFilter rather
// than anywhere later.
//
// catalogRatioRange is taken over the preFilter survivors, and C-10 clamps
// R_eff into it on the argument that the range is the set of shapes that can
// actually be purchased. An ineligible family inside that range would let
// R_eff be clamped to a shape no eligible candidate has, and C-10's guarantee
// -- that at least one candidate ends up near fit 1.0 -- would be false.
func TestFamily_R10PrecedesTheRatioRange(t *testing.T) {
	// x2i-shaped: 16.0 GiB/vCPU, far above the r-family ceiling of 8.0.
	wide := append(baseCatalog(), InstanceType{
		Name: "x2i.xlarge", VCPU: 4, MemoryMiB: 65536,
		Architectures: []string{ArchX8664}, CurrentGeneration: true,
		MaxNetworkInterfaces: 4, SupportsSpot: true,
		OnDemandHourly: fp(1.2000), SpotMedianHourly: fp(0.5000),
	})

	in := baseInput()
	in.Catalog = wide
	in.Posture = PosturePerformance
	in.Services = []ServiceDemand{{
		Name: "cache", VCPU: 0.5, MemGiB: 2, Count: 6,
		CPUAvg: 5, CPUPeak: 6, MemAvg: 70, MemPeak: 80, Datapoints: 336,
	}}
	res := Recommend(in)

	if got := res.Signals.Ratio.CatalogMax; !nearlyEqual(got, 8.0, eps) {
		t.Errorf("catalogMax = %v, want 8.0 -- the x-family record must not widen a "+
			"range that C-10 calls 'the ratios that can actually be purchased'", got)
	}
	if got := res.Signals.Ratio.Effective; !nearlyEqual(got, 8.0, eps) {
		t.Errorf("R_eff = %v, want it clamped to the ELIGIBLE ceiling 8.0", got)
	}
	best := 0.0
	for _, c := range res.Ranked {
		best = maxf(best, c.Scores.Fit)
	}
	if best <= 0.9 {
		t.Errorf("best fit after the clamp = %v; C-10's guarantee is broken when the "+
			"clamp targets a shape no eligible candidate has", best)
	}
}

// TestFamily_GPURequestSuspendsR10: asking for a GPU is itself an explicit
// opt-out of the general-purpose scope, and R5 is then strictly narrower than
// the allowlist.
func TestFamily_GPURequestSuspendsR10(t *testing.T) {
	pool := NormalizePool(PoolConstraints{AMIFamily: AMIFamilyAL2023GPU})
	survivors, misses := preFilter(normalizeCatalog(acceleratorCatalog()), pool, true)

	if !survived(survivors, "g5.xlarge") {
		t.Error("g5.xlarge did not survive a GPU request; R10 must not exclude the very " +
			"family the request exists to reach")
	}
	// And the accelerator that is NOT a GPU is still refused -- by R5, on the
	// GPUCount the API does report, which is 0.
	m, ok := missFor(misses, "inf1.24xlarge")
	if !ok || m.FailedRule != RuleGPU {
		t.Errorf("inf1.24xlarge under a GPU request failed %+v, want %q", m, RuleGPU)
	}

	in := baseInput()
	in.Catalog = acceleratorCatalog()
	in.Pool.AMIFamily = AMIFamilyAL2023GPU
	res := Recommend(in)
	if res.Classification != ClassGPU {
		t.Errorf("classification = %q, want %q", res.Classification, ClassGPU)
	}
	if res.Primary == nil || res.Primary.InstanceType != "g5.xlarge" {
		t.Errorf("primary = %v, want g5.xlarge", res.Primary)
	}
}

// TestFamily_PinnedTypeReachesTheRanking is the opt-in end to end: an operator
// who really does want an Inferentia pool can have one, and nothing else
// changes.
func TestFamily_PinnedTypeReachesTheRanking(t *testing.T) {
	in := baseInput()
	in.Catalog = acceleratorCatalog()
	in.Posture = PostureCost
	in.Limit = 50
	in.Pool.InstanceTypes = []string{"inf1.24xlarge"}

	res := Recommend(in)
	found := false
	for _, c := range res.Ranked {
		if c.InstanceType == "inf1.24xlarge" {
			found = true
		}
		if c.InstanceType == "i4i.2xlarge" {
			t.Error("pinning inf1.24xlarge also admitted i4i.2xlarge; the opt-in is per type")
		}
	}
	if !found {
		t.Errorf("inf1.24xlarge is absent even though it is pinned in instance_types: %v",
			res.Constraint)
	}
}

// TestFamily_ScopeRuleDoesNotSpeakForTheRefusal.
//
// dominantRule picks the rule that excluded the most candidates. On a real
// 903-type region R10 fires for roughly two thirds of the catalog, so without
// the scopeRule carve-out it would win that count on nearly every unsatisfiable
// answer and bury the constraint the user can act on.
func TestFamily_ScopeRuleDoesNotSpeakForTheRefusal(t *testing.T) {
	in := baseInput()
	in.Catalog = acceleratorCatalog()
	// One task larger than anything in the catalog can hold.
	in.Services = []ServiceDemand{{Name: "monolith", VCPU: 4, MemGiB: 500, Count: 1}}

	res := Recommend(in)
	if !res.Unsatisfiable {
		t.Fatalf("a 500 GiB task was satisfied by %v", res.Primary)
	}
	if got := res.Constraint; !contains(got, "500.0 GiB") {
		t.Errorf("constraint = %q, want it to name the task the user can act on, "+
			"not this tool's own family scope", got)
	}

	// The carve-out is a preference, not a suppression: when R10 is the ONLY
	// rule that fired, it does speak, and it names the escape hatch.
	only := baseInput()
	only.Catalog = []InstanceType{
		{Name: "inf1.24xlarge", VCPU: 96, MemoryMiB: 196608,
			Architectures: []string{ArchX8664}, CurrentGeneration: true,
			MaxNetworkInterfaces: 15, OnDemandHourly: fp(4.7200)},
		{Name: "i4i.2xlarge", VCPU: 8, MemoryMiB: 65536,
			Architectures: []string{ArchX8664}, CurrentGeneration: true,
			MaxNetworkInterfaces: 4, OnDemandHourly: fp(0.6860)},
	}
	res = Recommend(only)
	if !res.Unsatisfiable {
		t.Fatalf("an accelerator-only catalog produced %v", res.Primary)
	}
	if got := res.Constraint; !contains(got, "instance_types") {
		t.Errorf("constraint = %q, want it to name the instance_types opt-in", got)
	}
}

// contains is a local substring helper, so this file adds no import.
func contains(haystack, needle string) bool {
	if len(needle) > len(haystack) {
		return false
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
