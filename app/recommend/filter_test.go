package recommend

import (
	"math"
	"testing"
)

func missFor(misses []Miss, name string) (Miss, bool) {
	for _, m := range misses {
		if m.InstanceType == name {
			return m, true
		}
	}
	return Miss{}, false
}

func survived(survivors []InstanceType, name string) bool {
	for _, it := range survivors {
		if it.Name == name {
			return true
		}
	}
	return false
}

func eligibleFor(list []eligible, name string) (eligible, bool) {
	for _, e := range list {
		if e.it.Name == name {
			return e, true
		}
	}
	return eligible{}, false
}

// TestEligible walks every hard rule: one candidate per rule, each asserted to
// be EXCLUDED (FR-21 never down-ranks) with the right Miss.FailedRule.
func TestEligible(t *testing.T) {
	catalog := baseCatalog()

	t.Run("pre-filter rules", func(t *testing.T) {
		extra := append(catalog,
			InstanceType{Name: "m7i-broken.large", VCPU: 0, MemoryMiB: 8192,
				Architectures: []string{ArchX8664}, CurrentGeneration: true,
				MaxNetworkInterfaces: 3, OnDemandHourly: fp(0.10)},
			InstanceType{Name: "m7i-unpriced.large", VCPU: 2, MemoryMiB: 8192,
				Architectures: []string{ArchX8664}, CurrentGeneration: true,
				MaxNetworkInterfaces: 3},
			InstanceType{Name: "m7i-free.large", VCPU: 2, MemoryMiB: 8192,
				Architectures: []string{ArchX8664}, CurrentGeneration: true,
				MaxNetworkInterfaces: 3, OnDemandHourly: fp(0)},
			InstanceType{Name: "m7i-infinite.large", VCPU: 2, MemoryMiB: 8192,
				Architectures: []string{ArchX8664}, CurrentGeneration: true,
				MaxNetworkInterfaces: 3, OnDemandHourly: fp(math.Inf(1))},
		)
		pool := NormalizePool(PoolConstraints{})
		survivors, misses := preFilter(normalizeCatalog(extra), pool, false)

		cases := []struct {
			instance string
			rule     string
			why      string
		}{
			{"m7i-broken.large", RuleMalformedRecord, "R0: a catalog record with zero vCPU is a defect, never scored"},
			{"m4.large", RuleGeneration, "R1: previous generation"},
			{"m5.metal", RuleBareMetal, "R3: bare metal"},
			{"m7g.large", RuleArchitecture, "R4: arm64 against the x86-only al2023 default"},
			{"g5.xlarge", RuleFamilyNotEligible, "R10: g is an accelerator family, and R10 runs ahead of R5"},
			{"m7i-unpriced.large", RuleUnpriced, "R7: no price at all"},
			{"m7i-free.large", RuleUnpriced, "R7: a zero price is never treated as free"},
			{"m7i-infinite.large", RuleUnpriced, "R7: a non-finite price"},
		}
		for _, tc := range cases {
			t.Run(tc.instance, func(t *testing.T) {
				if survived(survivors, tc.instance) {
					t.Fatalf("%s survived; %s", tc.instance, tc.why)
				}
				m, ok := missFor(misses, tc.instance)
				if !ok {
					t.Fatalf("%s produced no Miss; every exclusion feeds nearestMisses", tc.instance)
				}
				if m.FailedRule != tc.rule {
					t.Errorf("%s failed %q, want %q", tc.instance, m.FailedRule, tc.rule)
				}
			})
		}

		if !survived(survivors, "m7i.large") {
			t.Error("m7i.large should survive every pre-filter rule")
		}

		// R5's own coverage under a NON-gpu request. R10 now catches every
		// accelerator family first, so the only way to reach R5 on this path
		// is to pin an accelerator type in the pool's instance_types without
		// asking for a GPU -- and "an idle GPU is never recommended" still has
		// to bite there. The two rules are belt and braces in that order.
		pinned := NormalizePool(PoolConstraints{InstanceTypes: []string{"g5.xlarge"}})
		_, pinnedMisses := preFilter(normalizeCatalog(extra), pinned, false)
		if m, ok := missFor(pinnedMisses, "g5.xlarge"); !ok || m.FailedRule != RuleGPU {
			t.Errorf("pinned g5.xlarge without gpu=true failed %+v, want %q", m, RuleGPU)
		}
	})

	t.Run("gpu class demands a gpu", func(t *testing.T) {
		pool := NormalizePool(PoolConstraints{AMIFamily: AMIFamilyAL2023GPU})
		survivors, misses := preFilter(normalizeCatalog(catalog), pool, true)
		if !survived(survivors, "g5.xlarge") {
			t.Error("g5.xlarge should survive when a GPU is requested")
		}
		m, ok := missFor(misses, "m7i.large")
		if !ok || m.FailedRule != RuleGPU {
			t.Errorf("m7i.large under a GPU request failed %+v, want rule %q", m, RuleGPU)
		}
	})

	t.Run("class-filter rules", func(t *testing.T) {
		pool := NormalizePool(PoolConstraints{})
		survivors, _ := preFilter(normalizeCatalog(catalog), pool, false)

		// R8 both ways.
		_, misses := classFilter(survivors, classFilterInput{demand: baseDemand(), class: ClassBalanced, pool: pool})
		if m, ok := missFor(misses, "t3.medium"); !ok || m.FailedRule != RuleBurstableClass {
			t.Errorf("t3.medium under class balanced failed %+v, want %q", m, RuleBurstableClass)
		}
		_, misses = classFilter(survivors, classFilterInput{demand: baseDemand(), class: ClassBurstable, pool: pool})
		if m, ok := missFor(misses, "m7i.large"); !ok || m.FailedRule != RuleBurstableClass {
			t.Errorf("m7i.large under class burstable failed %+v, want %q", m, RuleBurstableClass)
		}

		// R9a: a task nothing can hold at all.
		huge := ConfiguredShape([]ServiceDemand{{Name: "huge", VCPU: 1, MemGiB: 100, Count: 1}})
		_, misses = classFilter(survivors, classFilterInput{demand: huge, class: ClassBalanced, pool: pool})
		if m, ok := missFor(misses, "r7i.large"); !ok || m.FailedRule != RuleZeroDensity {
			t.Errorf("r7i.large against a 100 GiB task failed %+v, want %q", m, RuleZeroDensity)
		} else if m.Unit != UnitTasks || m.Needed != 1 {
			t.Errorf("zero_density Miss = %+v, want needed 1 tasks", m)
		}

		// R2 vCPU: reachable only when the MEAN task is small and the largest
		// one is not, because R9a runs first and tests the mean.
		fatCPU := ConfiguredShape(mixedServices(8, 1))
		_, misses = classFilter(survivors, classFilterInput{demand: fatCPU, class: ClassBalanced, pool: pool})
		if m, ok := missFor(misses, "m7i.large"); !ok || m.FailedRule != RuleTaskFitVCPU {
			t.Errorf("m7i.large against an 8 vCPU task failed %+v, want %q", m, RuleTaskFitVCPU)
		} else if m.Needed != 8 || m.Available != 2 || m.Unit != UnitVCPU {
			t.Errorf("task_fit_vcpu Miss = %+v, want needed 8 available 2 vCPU", m)
		}

		// R2 memory, and the 0.85 reserve is what it is checked against.
		fatMem := ConfiguredShape(mixedServices(2, 200))
		_, misses = classFilter(survivors, classFilterInput{demand: fatMem, class: ClassBalanced, pool: pool})
		if m, ok := missFor(misses, "r7i.xlarge"); !ok || m.FailedRule != RuleTaskFitMemory {
			t.Errorf("r7i.xlarge against a 200 GiB task failed %+v, want %q", m, RuleTaskFitMemory)
		} else if m.Needed != 200 || !nearlyEqual(m.Available, 27.2, 1e-9) || m.Unit != UnitGiB {
			t.Errorf("task_fit_memory Miss = %+v, want needed 200 available 27.2 GiB", m)
		}
	})

	t.Run("eni density is an awsvpc rule", func(t *testing.T) {
		thin := InstanceType{
			Name: "m7i-thin.large", VCPU: 8, MemoryMiB: 32768,
			Architectures: []string{ArchX8664}, CurrentGeneration: true,
			MaxNetworkInterfaces: 2, OnDemandHourly: fp(0.30),
		}
		pool := NormalizePool(PoolConstraints{NetworkMode: NetworkModeAWSVPC})
		survivors, _ := preFilter(normalizeCatalog([]InstanceType{thin}), pool, false)
		_, misses := classFilter(survivors, classFilterInput{demand: baseDemand(), class: ClassBalanced, pool: pool})
		m, ok := missFor(misses, "m7i-thin.large")
		if !ok || m.FailedRule != RuleENIDensity {
			t.Fatalf("m7i-thin.large under awsvpc failed %+v, want %q", m, RuleENIDensity)
		}
		if m.Needed != 3 || m.Available != 2 || m.Unit != UnitENIs {
			t.Errorf("eni_density Miss = %+v, want needed 3 available 2 ENIs", m)
		}

		// Under bridge the same record is fine: there are no task ENIs.
		bridgePool := NormalizePool(PoolConstraints{})
		survivors, _ = preFilter(normalizeCatalog([]InstanceType{thin}), bridgePool, false)
		got, misses := classFilter(survivors, classFilterInput{demand: baseDemand(), class: ClassBalanced, pool: bridgePool})
		if len(got) != 1 {
			t.Errorf("under bridge m7i-thin.large was excluded: %+v", misses)
		}
	})
}

// TestEligible_ArchitectureBothDirections is C-11. The one-directional rule,
// combined with an AMIFamily defaulting to "", ranked Graviton types for a pool
// whose AMI was x86-only; the ASG then failed every launch with a message
// visible only in scaling activities, while terraform apply reported success.
func TestEligible_ArchitectureBothDirections(t *testing.T) {
	// The default is al2023 and NOT "": this is the half of the finding that
	// makes the second rejection happen without anyone setting a family.
	if got := NormalizePool(PoolConstraints{}).AMIFamily; got != AMIFamilyAL2023 {
		t.Fatalf("default AMIFamily = %q, want %q", got, AMIFamilyAL2023)
	}

	catalog := normalizeCatalog(baseCatalog())

	arm := NormalizePool(PoolConstraints{AMIFamily: AMIFamilyAL2023ARM64})
	survivors, misses := preFilter(catalog, arm, false)
	if m, ok := missFor(misses, "m7i.large"); !ok || m.FailedRule != RuleArchitecture {
		t.Errorf("al2023_arm64 accepted x86 m7i.large: %+v", m)
	}
	if !survived(survivors, "m7g.large") {
		t.Error("al2023_arm64 rejected arm64 m7g.large")
	}

	// The other direction, with the family left entirely unset.
	survivors, misses = preFilter(catalog, NormalizePool(PoolConstraints{}), false)
	if m, ok := missFor(misses, "m7g.large"); !ok || m.FailedRule != RuleArchitecture {
		t.Errorf("the default family accepted arm64 m7g.large: %+v", m)
	}
	if !survived(survivors, "m7i.large") {
		t.Error("the default family rejected x86 m7i.large")
	}

	// And through the whole pipeline, because that is where the ASG failure
	// actually came from.
	in := baseInput()
	in.Pool.AMIFamily = AMIFamilyAL2023
	for _, c := range Recommend(in).Ranked {
		if c.Architecture != ArchX8664 {
			t.Errorf("al2023 recommended %s (%s)", c.InstanceType, c.Architecture)
		}
	}
	in.Pool.AMIFamily = AMIFamilyAL2023ARM64
	for _, c := range Recommend(in).Ranked {
		if c.Architecture != ArchARM64 {
			t.Errorf("al2023_arm64 recommended %s (%s)", c.InstanceType, c.Architecture)
		}
	}
}

// TestEligible_ExcludesUnholdablePool is C-8 as D-8 resolved it.
//
// It pins MaxSize, NOT MinSize. A too-small floor is unrepresentable, because
// n_floor = max(ceilToInt(n_exact), MinSize, 1) lets demand raise the floor, so
// h >= 1 for every candidate and R9b excluded nothing. The pool's CEILING is
// the quantity demand does not raise.
func TestEligible_ExcludesUnholdablePool(t *testing.T) {
	catalog := normalizeCatalog(baseCatalog())
	d := baseDemand() // 6 tasks; m7i.large holds 3 of them under bridge

	pool := NormalizePool(PoolConstraints{MaxSize: ip(1)})
	survivors, _ := preFilter(catalog, pool, false)
	got, misses := classFilter(survivors, classFilterInput{demand: d, class: ClassBalanced, pool: pool})

	if _, ok := eligibleFor(got, "m7i.large"); ok {
		t.Fatal("m7i.large is eligible with max_size 1, which holds 3 of 6 tasks")
	}
	m, ok := missFor(misses, "m7i.large")
	if !ok || m.FailedRule != RuleMaxSizeTooSmall {
		t.Fatalf("m7i.large failed %+v, want %q", m, RuleMaxSizeTooSmall)
	}
	if m.Needed != 6 || m.Available != 3 || m.Unit != UnitTasks {
		t.Errorf("max_size_too_small Miss = %+v, want needed 6 available 3 tasks", m)
	}

	// The boundary is >=, not >: max_size x tasksPerInstance == T holds the
	// fleet exactly, and an off-by-one here would refuse a correct pool.
	exact := NormalizePool(PoolConstraints{MaxSize: ip(2)}) // 2 x 3 == 6
	survivors, _ = preFilter(catalog, exact, false)
	got, misses = classFilter(survivors, classFilterInput{demand: d, class: ClassBalanced, pool: exact})
	if _, ok := eligibleFor(got, "m7i.large"); !ok {
		m, _ := missFor(misses, "m7i.large")
		t.Errorf("m7i.large excluded at max_size 2 x 3 tasks == 6 configured: %+v", m)
	}
	// And one instance short is refused.
	short := NormalizePool(PoolConstraints{MaxSize: ip(1)})
	survivors, _ = preFilter(catalog, short, false)
	if _, misses := classFilter(survivors, classFilterInput{demand: d, class: ClassBalanced, pool: short}); true {
		if m, ok := missFor(misses, "m7i.large"); !ok || m.FailedRule != RuleMaxSizeTooSmall {
			t.Errorf("m7i.large at max_size 1 failed %+v, want %q", m, RuleMaxSizeTooSmall)
		}
	}

	// Excluded, not merely down-weighted -- under all three postures,
	// including cost-first, where headroom weighs 0.05 and a down-weight would
	// be invisible.
	for _, posture := range []Posture{PosturePerformance, PostureBalanced, PostureCost} {
		in := baseInput()
		in.Posture = posture
		in.Pool.MaxSize = ip(1)
		res := Recommend(in)
		for _, c := range res.Ranked {
			if c.InstanceType == "m7i.large" {
				t.Errorf("%s ranked m7i.large despite max_size 1", posture)
			}
			if c.TasksPerInstance*1 < d.TaskCount {
				t.Errorf("%s ranked %s, which holds %d of %d tasks at max_size 1",
					posture, c.InstanceType, c.TasksPerInstance, d.TaskCount)
			}
		}
	}
}

// TestEligible_NoFloorConstraint is the other half of D-8: there is no rule
// reading MinSize, and no Miss can ever say cannot_hold_peak.
func TestEligible_NoFloorConstraint(t *testing.T) {
	catalog := normalizeCatalog(baseCatalog())
	d := baseDemand()

	// n_exact for m7i.large is 1.765, so a pinned min_size of 1 is "too
	// small" in the sense the removed rule imagined. It is still eligible.
	pool := NormalizePool(PoolConstraints{MinSize: ip(1)})
	survivors, misses := preFilter(catalog, pool, false)
	got, classMisses := classFilter(survivors, classFilterInput{demand: d, class: ClassBalanced, pool: pool})
	if _, ok := eligibleFor(got, "m7i.large"); !ok {
		m, _ := missFor(classMisses, "m7i.large")
		t.Fatalf("m7i.large was excluded by a floor constraint: %+v", m)
	}

	for _, m := range append(misses, classMisses...) {
		if m.FailedRule == "cannot_hold_peak" {
			t.Errorf("a rule produced cannot_hold_peak, which D-8 removed: %+v", m)
		}
	}

	// And through Recommend, over every posture: MinSize never excludes.
	for _, posture := range []Posture{PosturePerformance, PostureBalanced, PostureCost} {
		in := baseInput()
		in.Posture = posture
		in.Pool.MinSize = ip(1)
		res := Recommend(in)
		if res.Unsatisfiable || len(res.Ranked) == 0 {
			t.Errorf("%s: a pinned min_size of 1 made the request unsatisfiable", posture)
		}
		for _, m := range res.NearestMisses {
			if m.FailedRule == "cannot_hold_peak" {
				t.Errorf("%s produced cannot_hold_peak", posture)
			}
		}
	}
}
