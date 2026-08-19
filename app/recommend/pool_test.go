package recommend

import "testing"

// TestSuggestedPool_AMIFamilyMatchesTypes is C-11's third change: ami_family is
// DERIVED from the chosen types. FR-26 let those two fields disagree, and an
// ASG whose AMI architecture disagrees with its instance types fails every
// launch with a message visible only in scaling activities.
func TestSuggestedPool_AMIFamilyMatchesTypes(t *testing.T) {
	t.Run("an all-arm64 selection sets al2023_arm64", func(t *testing.T) {
		in := baseInput()
		in.Pool.AMIFamily = AMIFamilyAL2023ARM64
		got := Recommend(in)
		if got.Primary == nil {
			t.Fatal("no primary")
		}
		if got.SuggestedPool.AMIFamily != AMIFamilyAL2023ARM64 {
			t.Errorf("ami_family = %q, want %q", got.SuggestedPool.AMIFamily, AMIFamilyAL2023ARM64)
		}
		for _, name := range got.SuggestedPool.InstanceTypes {
			for _, c := range got.Ranked {
				if c.InstanceType == name && c.Architecture != ArchARM64 {
					t.Errorf("%s is %s but the pool renders an arm64 AMI", name, c.Architecture)
				}
			}
		}
	})

	t.Run("an x86 selection sets al2023", func(t *testing.T) {
		got := Recommend(baseInput())
		if got.SuggestedPool.AMIFamily != AMIFamilyAL2023 {
			t.Errorf("ami_family = %q, want %q", got.SuggestedPool.AMIFamily, AMIFamilyAL2023)
		}
	})

	t.Run("a gpu classification sets al2023_gpu", func(t *testing.T) {
		in := baseInput()
		in.Pool.ForceGPU = true
		got := Recommend(in)
		if got.Classification != ClassGPU {
			t.Fatalf("classification = %q, want %q", got.Classification, ClassGPU)
		}
		if got.SuggestedPool.AMIFamily != AMIFamilyAL2023GPU {
			t.Errorf("ami_family = %q, want %q", got.SuggestedPool.AMIFamily, AMIFamilyAL2023GPU)
		}
	})

	t.Run("a mixed-architecture selection never occurs", func(t *testing.T) {
		// Rule R4 has already excluded one architecture, so the selection
		// cannot straddle. Asserted over every fixture rather than argued.
		for name, in := range allFixtureInputs() {
			res := Recommend(in)
			arch := ""
			for _, want := range res.SuggestedPool.InstanceTypes {
				for _, c := range res.Ranked {
					if c.InstanceType != want {
						continue
					}
					if arch == "" {
						arch = c.Architecture
					} else if c.Architecture != arch {
						t.Errorf("%s: suggested pool mixes %s and %s", name, arch, c.Architecture)
					}
				}
			}
		}
	})
}

// TestSuggestedPool_TypesAreSiblings is FR-26: the primary plus up to two
// further candidates within +/-25 % of its vCPU and memory. Fewer qualifying
// types yields a shorter list, never a padded one.
func TestSuggestedPool_TypesAreSiblings(t *testing.T) {
	for name, in := range allFixtureInputs() {
		t.Run(name, func(t *testing.T) {
			res := Recommend(in)
			if res.Primary == nil {
				return
			}
			types := res.SuggestedPool.InstanceTypes
			if len(types) == 0 || len(types) > maxSuggestedTypes {
				t.Fatalf("instance_types = %v, want between 1 and %d entries", types, maxSuggestedTypes)
			}
			if types[0] != res.Primary.InstanceType {
				t.Errorf("instance_types[0] = %s, want the primary %s", types[0], res.Primary.InstanceType)
			}
			byName := byType(res)
			for _, n := range types[1:] {
				c := byName[n]
				if !withinTolerance(float64(c.VCPU), float64(res.Primary.VCPU), siblingTolerance) {
					t.Errorf("%s has %d vCPU against the primary's %d", n, c.VCPU, res.Primary.VCPU)
				}
				if !withinTolerance(float64(c.MemoryMiB), float64(res.Primary.MemoryMiB), siblingTolerance) {
					t.Errorf("%s has %d MiB against the primary's %d", n, c.MemoryMiB, res.Primary.MemoryMiB)
				}
			}
		})
	}
}

// TestSuggestedPool_SizesHoldTheFleet: a suggestion whose own ceiling rule R9c
// would reject is not a suggestion.
func TestSuggestedPool_SizesHoldTheFleet(t *testing.T) {
	for name, in := range allFixtureInputs() {
		t.Run(name, func(t *testing.T) {
			res := Recommend(in)
			if res.Primary == nil {
				return
			}
			sp := res.SuggestedPool
			if sp.MinSize < 1 {
				t.Errorf("min_size = %d", sp.MinSize)
			}
			if sp.MaxSize < sp.MinSize {
				t.Errorf("max_size %d < min_size %d", sp.MaxSize, sp.MinSize)
			}
			if held := sp.MaxSize * res.Primary.TasksPerInstance; held < res.Signals.ConfiguredTaskCount {
				t.Errorf("max_size %d holds %d of %d tasks", sp.MaxSize, held, res.Signals.ConfiguredTaskCount)
			}
			if sp.RootVolumeGB != defaultRootVolumeGB {
				t.Errorf("root_volume_gb = %d, want %d", sp.RootVolumeGB, defaultRootVolumeGB)
			}
			if sp.NetworkMode != NormalizePool(in.Pool).NetworkMode {
				t.Errorf("network_mode = %q, want %q", sp.NetworkMode, NormalizePool(in.Pool).NetworkMode)
			}
			if sp.CapacityType != CapacitySpotWithBase && sp.OnDemandBase != 0 {
				t.Errorf("on_demand_base = %d on a %q pool", sp.OnDemandBase, sp.CapacityType)
			}
		})
	}
}

// TestSuggestedPool_SpotDowngrade is EC-6: spot asked for, spot unavailable for
// the primary type, so the pool falls back to on_demand and says so.
func TestSuggestedPool_SpotDowngrade(t *testing.T) {
	catalog := baseCatalog()
	for i := range catalog {
		catalog[i].SupportsSpot = false
		catalog[i].SpotMedianHourly = nil
	}
	in := baseInput()
	in.Catalog = catalog
	in.Posture = PostureCost // suggests spot

	got := Recommend(in)
	if got.Primary == nil {
		t.Fatal("no primary")
	}
	if got.SuggestedPool.CapacityType != CapacityOnDemand {
		t.Errorf("capacity_type = %q, want %q after the downgrade", got.SuggestedPool.CapacityType, CapacityOnDemand)
	}
	if !got.SuggestedPool.Downgraded {
		t.Error("Downgraded = false; the downgrade must be visible, not silent")
	}
	if got.SuggestedPool.OnDemandBase != 0 {
		t.Errorf("on_demand_base = %d after the downgrade", got.SuggestedPool.OnDemandBase)
	}
	// And the reason says so, because Explain may only cite what Signals and
	// the ranking know.
	if !containsFold(got.Primary.Reason, "on-demand") {
		t.Errorf("reason does not mention the downgrade: %q", got.Primary.Reason)
	}

	// With spot available the pool keeps its posture's capacity type.
	normal := Recommend(baseInput())
	if normal.SuggestedPool.Downgraded {
		t.Error("Downgraded = true with spot medians present")
	}
}

func containsFold(haystack, needle string) bool {
	return len(haystack) >= len(needle) && indexFold(haystack, needle) >= 0
}

func indexFold(haystack, needle string) int {
	lower := func(b byte) byte {
		if b >= 'A' && b <= 'Z' {
			return b + 32
		}
		return b
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		match := true
		for j := 0; j < len(needle); j++ {
			if lower(haystack[i+j]) != lower(needle[j]) {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

func TestDeriveAMIFamily(t *testing.T) {
	arm := []RankItem{{Candidate: Candidate{Architecture: ArchARM64}}}
	x86 := []RankItem{{Candidate: Candidate{Architecture: ArchX8664}}}
	mixed := []RankItem{{Candidate: Candidate{Architecture: ArchARM64}}, {Candidate: Candidate{Architecture: ArchX8664}}}

	cases := []struct {
		name  string
		types []RankItem
		class string
		want  string
	}{
		{"all arm64", arm, ClassBalanced, AMIFamilyAL2023ARM64},
		{"all x86", x86, ClassBalanced, AMIFamilyAL2023},
		{"gpu class on x86", x86, ClassGPU, AMIFamilyAL2023GPU},
		{"mixed falls back to x86", mixed, ClassBalanced, AMIFamilyAL2023},
		{"no types at all", nil, ClassBalanced, AMIFamilyAL2023},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := deriveAMIFamily(tc.types, tc.class); got != tc.want {
				t.Errorf("deriveAMIFamily = %q, want %q", got, tc.want)
			}
		})
	}
}
