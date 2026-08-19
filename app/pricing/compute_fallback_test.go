package pricing

import (
	"regexp"
	"strings"
	"testing"
)

// TestFallbackCatalog_CoversFR12Families is the FR-12 coverage contract: the
// three default-pool types, plus one representative of each of t, c, r and g in
// BOTH architectures. A user with no credentials must still see a usable set.
func TestFallbackCatalog_CoversFR12Families(t *testing.T) {
	cat := GetFallbackCatalog()

	byType := make(map[string]FallbackInstanceType, len(cat.InstanceTypes))
	for _, r := range cat.InstanceTypes {
		if _, dup := byType[r.InstanceType]; dup {
			t.Fatalf("duplicate record for %s", r.InstanceType)
		}
		byType[r.InstanceType] = r
	}

	for _, want := range FallbackDefaultPoolInstanceTypes() {
		if _, ok := byType[want]; !ok {
			t.Errorf("default pool type %s is missing from the fallback catalogue", want)
		}
	}

	// family letter -> architecture -> present
	seen := map[string]map[string]bool{}
	family := regexp.MustCompile(`^([a-z]+)[0-9]`)
	for _, r := range cat.InstanceTypes {
		m := family.FindStringSubmatch(r.InstanceType)
		if m == nil {
			t.Fatalf("cannot derive a family letter from %q", r.InstanceType)
		}
		f := m[1]
		for _, arch := range r.Architectures {
			if seen[f] == nil {
				seen[f] = map[string]bool{}
			}
			seen[f][arch] = true
		}
	}

	for _, f := range []string{"t", "c", "r", "g", "m"} {
		for _, arch := range []string{"x86_64", "arm64"} {
			if !seen[f][arch] {
				t.Errorf("no %s-family %s record in the fallback catalogue (FR-12)", f, arch)
			}
		}
	}
}

// TestFallbackCatalog_NeverClaimsAvailability is the NFR-7 / AS-4 marker: this
// table is region-agnostic, so it may never imply that a type exists in the
// region the user selected.
func TestFallbackCatalog_NeverClaimsAvailability(t *testing.T) {
	if FALLBACK_AVAILABILITY_VERIFIED {
		t.Fatal("FALLBACK_AVAILABILITY_VERIFIED is true — the fallback table would claim availability it never checked")
	}

	cat := GetFallbackCatalog()
	if cat.AvailabilityVerified {
		t.Error("FallbackCatalog.AvailabilityVerified is true")
	}
	if cat.InstanceDataDate != FALLBACK_INSTANCE_DATA_DATE {
		t.Errorf("InstanceDataDate = %q, want %q", cat.InstanceDataDate, FALLBACK_INSTANCE_DATA_DATE)
	}
	if cat.PricingDate != FALLBACK_PRICING_DATE {
		t.Errorf("PricingDate = %q, want %q", cat.PricingDate, FALLBACK_PRICING_DATE)
	}
	if cat.PricingRegion != FALLBACK_PRICING_REGION {
		t.Errorf("PricingRegion = %q, want %q", cat.PricingRegion, FALLBACK_PRICING_REGION)
	}
	if FALLBACK_PRICING_REGION != "us-east-1" {
		t.Errorf("FALLBACK_PRICING_REGION = %q — the fallback prices are us-east-1 list prices (C-18)", FALLBACK_PRICING_REGION)
	}
}

// TestFallbackDates_AreISODates: both dated constants are parseable YYYY-MM-DD,
// because they are rendered straight into the API envelope.
func TestFallbackDates_AreISODates(t *testing.T) {
	iso := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	for name, v := range map[string]string{
		"FALLBACK_INSTANCE_DATA_DATE": FALLBACK_INSTANCE_DATA_DATE,
		"FALLBACK_PRICING_DATE":       FALLBACK_PRICING_DATE,
	} {
		if !iso.MatchString(v) {
			t.Errorf("%s = %q, want YYYY-MM-DD", name, v)
		}
	}
}

// TestFallbackEC2Hourly_EveryTypePricedAboveZero: a zero price would make the
// recommender's minimum cost zero and collapse every cost sub-score, so "no
// price" is an absent key, never a 0.
func TestFallbackEC2Hourly_EveryTypePricedAboveZero(t *testing.T) {
	prices := GetFallbackEC2Hourly()
	if len(prices) == 0 {
		t.Fatal("the fallback price table is empty")
	}
	for typ, hourly := range prices {
		if hourly <= 0 {
			t.Errorf("%s priced at %v, want > 0", typ, hourly)
		}
	}

	for _, r := range GetFallbackCatalog().InstanceTypes {
		hourly, ok := FallbackEC2HourlyFor(r.InstanceType)
		if !ok {
			t.Errorf("%s has a hardware record but no fallback price", r.InstanceType)
			continue
		}
		if hourly <= 0 {
			t.Errorf("%s priced at %v, want > 0", r.InstanceType, hourly)
		}
	}

	for typ := range prices {
		found := false
		for _, r := range GetFallbackCatalog().InstanceTypes {
			if r.InstanceType == typ {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s has a fallback price but no hardware record", typ)
		}
	}

	if _, ok := FallbackEC2HourlyFor("zz9-plural.z-alpha"); ok {
		t.Error("an unknown instance type reported a fallback price")
	}
}

// TestFallbackCatalog_RecordsAreWellFormed guards the fields the recommender's
// hard constraints read. A malformed record here becomes a division by zero or
// a saturated int downstream.
func TestFallbackCatalog_RecordsAreWellFormed(t *testing.T) {
	for _, r := range GetFallbackCatalog().InstanceTypes {
		if r.InstanceType == "" {
			t.Fatal("a record has no instance type")
		}
		if r.VCPU <= 0 {
			t.Errorf("%s: vcpu = %d, want > 0", r.InstanceType, r.VCPU)
		}
		if r.MemoryMiB <= 0 {
			t.Errorf("%s: memoryMiB = %d, want > 0", r.InstanceType, r.MemoryMiB)
		}
		if len(r.Architectures) == 0 {
			t.Errorf("%s: no architectures", r.InstanceType)
		}
		for _, a := range r.Architectures {
			if a != "x86_64" && a != "arm64" {
				t.Errorf("%s: architecture %q is neither x86_64 nor arm64", r.InstanceType, a)
			}
		}
		if r.MaximumNetworkInterfaces <= 0 {
			t.Errorf("%s: maximumNetworkInterfaces = %d, want > 0", r.InstanceType, r.MaximumNetworkInterfaces)
		}
		if len(r.SupportedUsageClasses) == 0 {
			t.Errorf("%s: no supported usage classes", r.InstanceType)
		}
		if r.NetworkPerformance == "" {
			t.Errorf("%s: no networkPerformance string", r.InstanceType)
		}
		if r.BaselineBandwidthMbps != nil && *r.BaselineBandwidthMbps <= 0 {
			t.Errorf("%s: baselineBandwidthMbps = %v, want > 0 or null", r.InstanceType, *r.BaselineBandwidthMbps)
		}

		// GPU fields are consistent in both directions: a GPU record names its
		// GPU, a non-GPU record claims nothing.
		if r.GPUCount > 0 {
			if r.GPUMemoryMiB == nil || *r.GPUMemoryMiB <= 0 {
				t.Errorf("%s: gpuCount %d but no gpu memory", r.InstanceType, r.GPUCount)
			}
			if r.GPUName == nil || *r.GPUName == "" {
				t.Errorf("%s: gpuCount %d but no gpu name", r.InstanceType, r.GPUCount)
			}
			if !strings.HasPrefix(r.InstanceType, "g") && !strings.HasPrefix(r.InstanceType, "p") {
				t.Errorf("%s: has a GPU but is not in a GPU family", r.InstanceType)
			}
		} else {
			if r.GPUMemoryMiB != nil || r.GPUName != nil {
				t.Errorf("%s: gpuCount is 0 but GPU details are set", r.InstanceType)
			}
		}

		// The burstable flag is a per-task test input, so it has to be right on
		// the one family where it is true.
		if strings.HasPrefix(r.InstanceType, "t") != r.Burstable {
			t.Errorf("%s: burstable = %v, which disagrees with its family", r.InstanceType, r.Burstable)
		}
		if r.BareMetal {
			t.Errorf("%s: the fallback table carries no bare-metal types", r.InstanceType)
		}
		if !r.CurrentGeneration {
			t.Errorf("%s: the fallback table carries only current-generation types", r.InstanceType)
		}
	}
}

// TestFallbackCatalog_ReturnsADeepCopy: a handler that sorts, filters or edits
// what it got must not corrupt the table for the next request.
func TestFallbackCatalog_ReturnsADeepCopy(t *testing.T) {
	first := GetFallbackCatalog()
	if len(first.InstanceTypes) == 0 {
		t.Fatal("empty fallback catalogue")
	}

	victim := first.InstanceTypes[0]
	original := victim.InstanceType
	first.InstanceTypes[0].InstanceType = "mutated.type"
	first.InstanceTypes[0].Architectures[0] = "mutated_arch"
	first.InstanceTypes[0].SupportedUsageClasses[0] = "mutated_class"
	if victim.BaselineBandwidthMbps != nil {
		*first.InstanceTypes[0].BaselineBandwidthMbps = -1
	}
	first.InstanceTypes = first.InstanceTypes[:1]

	second := GetFallbackCatalog()
	if len(second.InstanceTypes) < 2 {
		t.Fatalf("second call returned %d records — the slice was shared", len(second.InstanceTypes))
	}
	if second.InstanceTypes[0].InstanceType != original {
		t.Errorf("record 0 = %q after a caller mutated its copy, want %q", second.InstanceTypes[0].InstanceType, original)
	}
	if second.InstanceTypes[0].Architectures[0] == "mutated_arch" {
		t.Error("architectures slice is shared between callers")
	}
	if second.InstanceTypes[0].SupportedUsageClasses[0] == "mutated_class" {
		t.Error("supportedUsageClasses slice is shared between callers")
	}
	if b := second.InstanceTypes[0].BaselineBandwidthMbps; b != nil && *b < 0 {
		t.Error("baselineBandwidthMbps pointer is shared between callers")
	}

	prices := GetFallbackEC2Hourly()
	for k := range prices {
		prices[k] = 0
	}
	delete(prices, "m6i.large")
	if v, ok := FallbackEC2HourlyFor("m6i.large"); !ok || v <= 0 {
		t.Errorf("price map is shared between callers: m6i.large = (%v, %v)", v, ok)
	}

	types := FallbackDefaultPoolInstanceTypes()
	types[0] = "mutated"
	if FallbackDefaultPoolInstanceTypes()[0] == "mutated" {
		t.Error("default pool type list is shared between callers")
	}
}
