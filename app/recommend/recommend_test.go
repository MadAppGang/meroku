package recommend

import (
	"fmt"
	"strings"
	"testing"
)

// fingerprint renders a Result deterministically, dereferencing every pointer,
// so that two runs can be compared byte for byte. %+v alone would print
// pointer addresses and pass regardless.
func fingerprint(r Result) string {
	var b strings.Builder
	fmt.Fprintf(&b, "class=%s basis=%s unsat=%v constraint=%q\n", r.Classification, r.Basis, r.Unsatisfiable, r.Constraint)
	if r.Primary == nil {
		b.WriteString("primary=<nil>\n")
	} else {
		fmt.Fprintf(&b, "primary=%s\n", candidateLine(*r.Primary))
	}
	for i, c := range r.Ranked {
		fmt.Fprintf(&b, "ranked[%d]=%s\n", i, candidateLine(c))
	}
	for i, m := range r.NearestMisses {
		fmt.Fprintf(&b, "miss[%d]=%+v\n", i, m)
	}
	s := r.Signals
	fmt.Fprintf(&b, "signals cfg=%+v T=%d cov=%.9f w=(%.9f,%.9f) ratio=%+v nm=%s trunk=%s density=%s cw=%s\n",
		s.Configured, s.ConfiguredTaskCount, s.Coverage, s.WeightConfigured, s.WeightActual,
		s.Ratio, s.NetworkMode, s.Trunking, s.DensityBasis, s.CloudWatch)
	if s.Actual == nil {
		b.WriteString("actual=<nil>\n")
	} else {
		fmt.Fprintf(&b, "actual=%+v\n", *s.Actual)
	}
	for i, d := range s.Dropped {
		fmt.Fprintf(&b, "dropped[%d]=%+v\n", i, d)
	}
	for i, sv := range s.Services {
		fmt.Fprintf(&b, "service[%d]=%+v\n", i, sv)
	}
	fmt.Fprintf(&b, "pool=%+v\n", r.SuggestedPool)
	return b.String()
}

func candidateLine(c Candidate) string {
	spot := "<nil>"
	if c.SpotMedianHourly != nil {
		spot = fmt.Sprintf("%.9f", *c.SpotMedianHourly)
	}
	return fmt.Sprintf("%s vcpu=%d mem=%d arch=%s scores=%+v total=%.12f eff=%.12f cps=%.12f k=%d floor=%d spot=%s reason=%q",
		c.InstanceType, c.VCPU, c.MemoryMiB, c.Architecture, c.Scores, c.Total,
		c.EffectiveHourly, c.CostPerTask, c.TasksPerInstance, c.InstancesAtFloor, spot, c.Reason)
}

// assertNoDrops is C-4's package-wide invariant: a non-empty Signals.Dropped
// is a bug, so every fixture asserts it empty and the drop stays visible.
func assertNoDrops(t *testing.T, r Result) {
	t.Helper()
	if len(r.Signals.Dropped) != 0 {
		t.Errorf("Signals.Dropped = %+v, want empty", r.Signals.Dropped)
	}
	for _, c := range r.Ranked {
		if !isFinite(c.Total) || !isFinite(c.Scores.Fit) || !isFinite(c.Scores.Utilisation) ||
			!isFinite(c.Scores.Cost) || !isFinite(c.Scores.Modernity) {
			t.Errorf("%s carries a non-finite score: %+v total=%v", c.InstanceType, c.Scores, c.Total)
		}
		if !isFinite(c.EffectiveHourly) || !isFinite(c.CostPerTask) {
			t.Errorf("%s carries a non-finite price: eff=%v cpt=%v", c.InstanceType, c.EffectiveHourly, c.CostPerTask)
		}
	}
}

// allFixtureInputs is every input shape this package's tests exercise. C-4's
// "for every other fixture, len(Signals.Dropped) == 0" is asserted over it.
func allFixtureInputs() map[string]Input {
	out := map[string]Input{}

	for _, posture := range []Posture{PosturePerformance, PostureBalanced, PostureCost} {
		in := baseInput()
		in.Posture = posture
		out["base/"+string(posture)] = in

		measured := baseInput()
		measured.Posture = posture
		measured.Services = measuredServices()
		out["measured/"+string(posture)] = measured
	}

	awsvpc := baseInput()
	awsvpc.Pool.NetworkMode = NetworkModeAWSVPC
	out["awsvpc"] = awsvpc

	trunked := awsvpc
	trunked.TrunkingEnabled = true
	out["awsvpc-trunked"] = trunked

	arm := baseInput()
	arm.Pool.AMIFamily = AMIFamilyAL2023ARM64
	out["arm64"] = arm

	gpu := baseInput()
	gpu.Pool.ForceGPU = true
	out["gpu"] = gpu

	burst := baseInput()
	burst.Services = []ServiceDemand{{Name: "idle", VCPU: 0.5, MemGiB: 1, Count: 4,
		CPUAvg: 5, CPUPeak: 20, MemAvg: 30, MemPeak: 40, Datapoints: 336}}
	out["burstable"] = burst

	wide := baseInput()
	wide.Catalog = wideCatalog()
	wide.Posture = PosturePerformance
	wide.Services = []ServiceDemand{{Name: "jvm", VCPU: 0.5, MemGiB: 2, Count: 6,
		CPUAvg: 5, CPUPeak: 6, MemAvg: 70, MemPeak: 80, Datapoints: 336}}
	out["clamped"] = wide

	none := baseInput()
	none.Services = nil
	out["no-demand"] = none

	zero := baseInput()
	zero.Services = []ServiceDemand{{Name: "broken", VCPU: 0, MemGiB: 2, Count: 3}}
	out["zero-vcpu"] = zero

	unsat := baseInput()
	unsat.Services = mixedServices(2, 200)
	out["unsatisfiable"] = unsat

	pinned := baseInput()
	pinned.Pool.MaxSize = ip(2)
	pinned.Pool.MinSize = ip(1)
	out["pinned-bounds"] = pinned

	partial := baseInput()
	partial.Services = []ServiceDemand{
		{Name: "backend", VCPU: 0.5, MemGiB: 2, Count: 3, CPUAvg: 8, CPUPeak: 15, MemAvg: 61, MemPeak: 78, Datapoints: 336},
		{Name: "worker", VCPU: 0.5, MemGiB: 2, Count: 3},
	}
	out["partial-coverage"] = partial

	idle := baseInput()
	idle.Services = []ServiceDemand{{Name: "idle", VCPU: 0.5, MemGiB: 2, Count: 6,
		CPUAvg: 0, CPUPeak: 0, MemAvg: 40, MemPeak: 55, Datapoints: 336}}
	out["zero-cpu-peak"] = idle

	return out
}

func TestRecommend_DroppedIsEmptyForEveryFixture(t *testing.T) {
	for name, in := range allFixtureInputs() {
		t.Run(name, func(t *testing.T) {
			assertNoDrops(t, Recommend(in))
		})
	}
}

// TestRecommend_IsDeterministic is AC-13/NFR-9. Run on linux/amd64 in CI it is
// also the C-9 regression: int(+Inf) differs there from the darwin/arm64
// development machine.
func TestRecommend_IsDeterministic(t *testing.T) { assertDeterministic(t) }

// TestRecommend_Determinism is the same body under the name CI selects with
// `go test -run Determinism`. Two entry points, because the architecture names
// the first and the build gate greps the second.
func TestRecommend_Determinism(t *testing.T) { assertDeterministic(t) }

func assertDeterministic(t *testing.T) {
	t.Helper()
	for name, in := range allFixtureInputs() {
		t.Run(name, func(t *testing.T) {
			want := fingerprint(Recommend(in))
			for i := 0; i < 10; i++ {
				if got := fingerprint(Recommend(in)); got != want {
					t.Fatalf("run %d differed:\n--- want ---\n%s\n--- got ---\n%s", i, want, got)
				}
			}
			// Input order must not leak into the answer either.
			shuffled := in
			shuffled.Catalog = reverseCatalog(in.Catalog)
			if got := fingerprint(Recommend(shuffled)); got != want {
				t.Fatalf("reversing the catalog changed the answer:\n--- want ---\n%s\n--- got ---\n%s", want, got)
			}
			// And Recommend must not mutate its input.
			if len(in.Catalog) > 0 && in.Catalog[0].Family != "" && in.Catalog[0].Generation != 0 {
				t.Errorf("Recommend mutated the caller's catalog: %+v", in.Catalog[0])
			}
		})
	}
}

func reverseCatalog(in []InstanceType) []InstanceType {
	out := make([]InstanceType, 0, len(in))
	for i := len(in) - 1; i >= 0; i-- {
		out = append(out, in[i])
	}
	return out
}

// TestRecommend_NoDemand is FR-28/AC-19.
func TestRecommend_NoDemand(t *testing.T) {
	in := baseInput()
	in.Services = nil
	got := Recommend(in)

	if got.Primary != nil {
		t.Errorf("primary = %+v, want nil", *got.Primary)
	}
	if got.Basis != BasisDefault {
		t.Errorf("basis = %q, want %q", got.Basis, BasisDefault)
	}
	if got.Unsatisfiable {
		t.Error("unsatisfiable = true; no demand is not an impossible demand")
	}
	if len(got.SuggestedPool.InstanceTypes) == 0 {
		t.Fatal("suggestedPool.instance_types is empty")
	}
	present := map[string]bool{}
	for _, it := range in.Catalog {
		present[it.Name] = true
	}
	for _, name := range got.SuggestedPool.InstanceTypes {
		if !present[name] {
			t.Errorf("suggested %s, which the region's catalog does not report", name)
		}
	}
	if got.SuggestedPool.CapacityType != CapacitySpotWithBase || got.SuggestedPool.OnDemandBase != 1 {
		t.Errorf("capacity = (%q,%d), want (%q,1)", got.SuggestedPool.CapacityType, got.SuggestedPool.OnDemandBase, CapacitySpotWithBase)
	}
	if got.SuggestedPool.MinSize != 1 || got.SuggestedPool.MaxSize != 6 || got.SuggestedPool.TargetCapacity != 100 {
		t.Errorf("sizes = (%d,%d,%d), want (1,6,100)",
			got.SuggestedPool.MinSize, got.SuggestedPool.MaxSize, got.SuggestedPool.TargetCapacity)
	}
	if got.SuggestedPool.NetworkMode != NetworkModeBridge {
		t.Errorf("network_mode = %q, want %q", got.SuggestedPool.NetworkMode, NetworkModeBridge)
	}
	assertNoDrops(t, got)

	t.Run("a catalog without the three defaults still answers", func(t *testing.T) {
		bare := baseInput()
		bare.Services = nil
		bare.Catalog = []InstanceType{
			{Name: "m9i.large", VCPU: 2, MemoryMiB: 8192, Architectures: []string{ArchX8664},
				CurrentGeneration: true, MaxNetworkInterfaces: 3, OnDemandHourly: fp(0.11)},
			{Name: "m9i.xlarge", VCPU: 4, MemoryMiB: 16384, Architectures: []string{ArchX8664},
				CurrentGeneration: true, MaxNetworkInterfaces: 4, OnDemandHourly: fp(0.22)},
		}
		res := Recommend(bare)
		if len(res.SuggestedPool.InstanceTypes) == 0 {
			t.Fatal("instance_types is empty even though the catalog is not")
		}
		for _, name := range res.SuggestedPool.InstanceTypes {
			if name != "m9i.large" && name != "m9i.xlarge" {
				t.Errorf("suggested %s, which is not in the passed catalog", name)
			}
		}
	})
}

// TestRecommend_Unsatisfiable is EC-11/AC-20.
func TestRecommend_Unsatisfiable(t *testing.T) {
	in := baseInput()
	in.Services = mixedServices(2, 200) // one 200 GiB task nothing can hold
	got := Recommend(in)

	if !got.Unsatisfiable {
		t.Fatalf("unsatisfiable = false; primary = %+v", got.Primary)
	}
	if got.Primary != nil {
		t.Errorf("primary = %+v, want nil", *got.Primary)
	}
	if len(got.NearestMisses) != nearestMissCount {
		t.Errorf("len(nearestMisses) = %d, want %d: %+v", len(got.NearestMisses), nearestMissCount, got.NearestMisses)
	}
	if !strings.Contains(got.Constraint, "200.0") {
		t.Errorf("constraint %q does not name the shortfall in numbers", got.Constraint)
	}
	if !strings.Contains(got.Constraint, "27.2") {
		t.Errorf("constraint %q does not name what the catalog could offer", got.Constraint)
	}
	// The nearest miss is the one that came closest, and it says by how much.
	if got.NearestMisses[0].InstanceType != "r7i.xlarge" {
		t.Errorf("nearest miss = %s, want r7i.xlarge (27.2 of 200 GiB)", got.NearestMisses[0].InstanceType)
	}
	if got.NearestMisses[0].Unit != UnitGiB || got.NearestMisses[0].Needed != 200 {
		t.Errorf("nearest miss = %+v, want needed 200 GiB", got.NearestMisses[0])
	}
	assertNoDrops(t, got)
}

// TestRecommend_AllUnpriced is C-4's explicit case: an unpriced candidate is
// EXCLUDED, never scored cost = 1.0. 1.0 is the best score, and it would win
// the one dimension the candidate has no data for.
func TestRecommend_AllUnpriced(t *testing.T) {
	in := baseInput()
	catalog := baseCatalog()
	for i := range catalog {
		catalog[i].OnDemandHourly = nil
	}
	in.Catalog = catalog
	got := Recommend(in)

	if !got.Unsatisfiable {
		t.Fatalf("unsatisfiable = false with nothing priced; primary = %+v", got.Primary)
	}
	if got.Primary != nil || len(got.Ranked) != 0 {
		t.Errorf("scored %d candidates with no prices at all", len(got.Ranked))
	}
	if !strings.Contains(got.Constraint, "price") {
		t.Errorf("constraint %q does not name pricing", got.Constraint)
	}
	assertNoDrops(t, got)

	t.Run("a zero price is not a free instance", func(t *testing.T) {
		zero := baseInput()
		zeroCatalog := baseCatalog()
		for i := range zeroCatalog {
			if zeroCatalog[i].Name == "m7i.large" {
				zeroCatalog[i].OnDemandHourly = fp(0)
			}
		}
		zero.Catalog = zeroCatalog
		res := Recommend(zero)
		for _, c := range res.Ranked {
			if c.InstanceType == "m7i.large" {
				t.Errorf("a zero-priced type was ranked with cost %v", c.Scores.Cost)
			}
			if c.Scores.Cost > 1 {
				t.Errorf("%s cost = %v, outside [0,1]", c.InstanceType, c.Scores.Cost)
			}
		}
		assertNoDrops(t, res)
	})
}

// TestRecommend_ZeroVCPUServiceIsExcluded is EC-12 as C-14 corrected it.
//
// The previous test row asserted Fit == 1.0 for this case. With R_eff = +Inf,
// FR-22's fit evaluates to 0, not 1 -- and an implementer writing the test from
// that row would have "fixed" the formula to match the document.
func TestRecommend_ZeroVCPUServiceIsExcluded(t *testing.T) {
	t.Run("alone", func(t *testing.T) {
		in := baseInput()
		in.Services = []ServiceDemand{{Name: "broken", VCPU: 0, MemGiB: 2, Count: 3}}
		got := Recommend(in)

		if got.Basis != BasisDefault {
			t.Errorf("basis = %q, want %q", got.Basis, BasisDefault)
		}
		if got.Primary != nil {
			t.Errorf("primary = %+v, want nil", *got.Primary)
		}
		if got.Signals.ConfiguredTaskCount != 0 {
			t.Errorf("taskCount = %d, want 0 -- the service carries no demand", got.Signals.ConfiguredTaskCount)
		}
		if len(got.Signals.Services) != 1 || got.Signals.Services[0].Status != StatusNoData {
			t.Errorf("services = %+v, want one entry with status %q", got.Signals.Services, StatusNoData)
		}
		assertNoDrops(t, got)
	})

	t.Run("beside a healthy service", func(t *testing.T) {
		in := baseInput()
		in.Services = []ServiceDemand{
			{Name: "broken", VCPU: 0, MemGiB: 2, Count: 3},
			{Name: "backend", VCPU: 0.5, MemGiB: 2, Count: 6},
		}
		got := Recommend(in)

		if got.Primary == nil {
			t.Fatal("primary = nil; the healthy service still carries demand")
		}
		if got.Signals.ConfiguredTaskCount != 6 {
			t.Errorf("taskCount = %d, want 6 -- the broken service must not contribute", got.Signals.ConfiguredTaskCount)
		}
		if !nearlyEqual(got.Signals.Configured.Ratio, 4.0, eps) {
			t.Errorf("R_cfg = %v, want 4.0", got.Signals.Configured.Ratio)
		}
		for _, s := range got.Signals.Services {
			if s.Name == "broken" && s.Status != StatusNoData {
				t.Errorf("broken service status = %q, want %q", s.Status, StatusNoData)
			}
		}
		assertNoDrops(t, got)
	})
}

// TestRecommend_BridgeChangesCostPerTask is C-13, asserted rather than
// claimed: costPerTask falls and the utilisation sub-score improves for
// exactly the memory-rich types the classification wanted.
//
// The waste assertion became a utilisation assertion because waste no longer
// exists. The claim is unchanged and, if anything, stronger: under awsvpc the
// ENI cap strands memory the classification asked for, so lifting the cap
// raises the ACHIEVED occupancy of a memory-rich type, not merely its
// hypothetical density.
func TestRecommend_BridgeChangesCostPerTask(t *testing.T) {
	bridgeIn := baseInput()
	bridgeIn.Posture = PosturePerformance
	bridgeIn.Limit = 20 // compare the whole ranking, not the top five of each
	awsvpcIn := bridgeIn
	awsvpcIn.Pool.NetworkMode = NetworkModeAWSVPC

	bridge := byType(Recommend(bridgeIn))
	awsvpc := byType(Recommend(awsvpcIn))

	memoryRich := []string{"r7i.large", "r7i.xlarge", "r6i.large"}
	for _, name := range memoryRich {
		b, okB := bridge[name]
		a, okA := awsvpc[name]
		if !okB || !okA {
			t.Fatalf("%s missing from one of the rankings (bridge=%v awsvpc=%v)", name, okB, okA)
		}
		if !(b.CostPerTask < a.CostPerTask) {
			t.Errorf("%s costPerTask: bridge %v is not below awsvpc %v", name, b.CostPerTask, a.CostPerTask)
		}
		if !(b.Scores.Utilisation > a.Scores.Utilisation) {
			t.Errorf("%s utilisation: bridge %v is not better than awsvpc %v",
				name, b.Scores.Utilisation, a.Scores.Utilisation)
		}
		if !(b.TasksPerInstance > a.TasksPerInstance) {
			t.Errorf("%s density: bridge %d is not above awsvpc %d", name, b.TasksPerInstance, a.TasksPerInstance)
		}
	}

	if got := Recommend(bridgeIn).Signals.DensityBasis; got != DensityCPUMemoryOnly {
		t.Errorf("bridge densityBasis = %q, want %q", got, DensityCPUMemoryOnly)
	}
	if got := Recommend(awsvpcIn).Signals.DensityBasis; got != DensityMaxENIsMinus1 {
		t.Errorf("awsvpc densityBasis = %q, want %q", got, DensityMaxENIsMinus1)
	}
	if got := Recommend(bridgeIn).Signals.Trunking; got != TrunkingNotApplicable {
		t.Errorf("bridge trunking = %q, want %q", got, TrunkingNotApplicable)
	}
	// TrunkingEnabled is read only under awsvpc.
	ignored := bridgeIn
	ignored.TrunkingEnabled = true
	if fingerprint(Recommend(ignored)) != fingerprint(Recommend(bridgeIn)) {
		t.Error("TrunkingEnabled changed a bridge recommendation")
	}
}

func byType(r Result) map[string]Candidate {
	out := make(map[string]Candidate, len(r.Ranked))
	for _, c := range r.Ranked {
		out[c.InstanceType] = c
	}
	return out
}

// TestRecommend_PostureMovesTheOutput is AC-14 clause 1.
func TestRecommend_PostureMovesTheOutput(t *testing.T) {
	results := map[Posture]Result{}
	for _, p := range []Posture{PosturePerformance, PostureBalanced, PostureCost} {
		in := baseInput()
		in.Posture = p
		res := Recommend(in)
		if res.Primary == nil {
			t.Fatalf("%s produced no primary", p)
		}
		results[p] = res
		assertNoDrops(t, res)
	}

	pairs := [][2]Posture{
		{PosturePerformance, PostureBalanced},
		{PostureBalanced, PostureCost},
		{PosturePerformance, PostureCost},
	}
	for _, pair := range pairs {
		a, b := results[pair[0]], results[pair[1]]
		moved := a.Primary.InstanceType != b.Primary.InstanceType ||
			a.SuggestedPool.CapacityType != b.SuggestedPool.CapacityType ||
			a.SuggestedPool.MinSize != b.SuggestedPool.MinSize
		if !moved {
			t.Errorf("%s and %s produced the same primary, capacity_type and min_size", pair[0], pair[1])
		}
	}
}

// TestRecommend_CostFirstIsNeverDearer is AC-14 clause 2. The weight table
// does NOT guarantee this, so this test is the enforcement: a failure retunes
// the weights rather than relaxing the test.
func TestRecommend_CostFirstIsNeverDearer(t *testing.T) {
	inputs := allFixtureInputs()
	for name, in := range inputs {
		t.Run(name, func(t *testing.T) {
			cheap := in
			cheap.Posture = PostureCost
			fast := in
			fast.Posture = PosturePerformance

			cheapRes, fastRes := Recommend(cheap), Recommend(fast)
			if cheapRes.Unsatisfiable || fastRes.Unsatisfiable {
				t.Skip("unsatisfiable inputs have no primary to compare")
			}
			if cheapRes.Primary == nil || fastRes.Primary == nil {
				t.Skip("no demand, so no primary")
			}
			if cheapRes.Primary.CostPerTask > fastRes.Primary.CostPerTask+1e-12 {
				t.Errorf("cost-first picked %s at %v per task slot; performance-first picked %s at %v",
					cheapRes.Primary.InstanceType, cheapRes.Primary.CostPerTask,
					fastRes.Primary.InstanceType, fastRes.Primary.CostPerTask)
			}
		})
	}
}

// TestRecommend_LimitAndBasis pins the two envelope fields the caller reads
// straight through.
func TestRecommend_LimitAndBasis(t *testing.T) {
	in := baseInput()
	in.Limit = 2
	got := Recommend(in)
	if len(got.Ranked) != 2 {
		t.Errorf("len(ranked) = %d, want 2", len(got.Ranked))
	}
	if got.Primary == nil || got.Primary.InstanceType != got.Ranked[0].InstanceType {
		t.Error("primary is not the top-ranked entry")
	}
	if got.Basis != BasisConfigured {
		t.Errorf("basis = %q, want %q with no CloudWatch data", got.Basis, BasisConfigured)
	}

	in.Limit = 0
	if got := Recommend(in); len(got.Ranked) > defaultLimit {
		t.Errorf("len(ranked) = %d with no limit set, want at most %d", len(got.Ranked), defaultLimit)
	}

	measured := baseInput()
	measured.Services = measuredServices()
	res := Recommend(measured)
	if res.Basis != BasisMeasured {
		t.Errorf("basis = %q, want %q with a full CloudWatch window", res.Basis, BasisMeasured)
	}
	if res.Signals.Actual == nil {
		t.Fatal("signals.actual = nil with a full CloudWatch window")
	}
	// ActualVCPU = 3.0 * 0.15, ActualMemGiB = 12.0 * 0.80.
	if !nearlyEqual(res.Signals.Actual.VCPU, 0.45, eps) || !nearlyEqual(res.Signals.Actual.MemGiB, 9.6, eps) {
		t.Errorf("actual = %+v, want (0.45, 9.6)", *res.Signals.Actual)
	}
	if !nearlyEqual(res.Signals.WeightActual, 0.60, eps) {
		t.Errorf("w_act = %v, want 0.60 at full coverage", res.Signals.WeightActual)
	}
}

// TestRecommend_EveryRankedEntrySatisfiesTheHardRules is AC-23.
func TestRecommend_EveryRankedEntrySatisfiesTheHardRules(t *testing.T) {
	for name, in := range allFixtureInputs() {
		t.Run(name, func(t *testing.T) {
			res := Recommend(in)
			pool := NormalizePool(in.Pool)
			catalog := map[string]InstanceType{}
			for _, it := range normalizeCatalog(in.Catalog) {
				catalog[it.Name] = it
			}
			for _, c := range res.Ranked {
				it, ok := catalog[c.InstanceType]
				if !ok {
					t.Fatalf("%s is not in the passed catalog", c.InstanceType)
				}
				if it.BareMetal {
					t.Errorf("%s is bare metal", c.InstanceType)
				}
				if !it.CurrentGeneration && !pool.IncludePreviousGeneration {
					t.Errorf("%s is previous generation", c.InstanceType)
				}
				if !archOK(pool.AMIFamily, it.Architectures) {
					t.Errorf("%s does not match ami_family %s", c.InstanceType, pool.AMIFamily)
				}
				if it.OnDemandHourly == nil || *it.OnDemandHourly <= 0 {
					t.Errorf("%s is unpriced yet ranked", c.InstanceType)
				}
				if c.TasksPerInstance < 1 {
					t.Errorf("%s holds %d tasks", c.InstanceType, c.TasksPerInstance)
				}
				gpuWanted := res.Classification == ClassGPU
				if gpuWanted != (it.GPUCount >= 1) {
					t.Errorf("%s has %d GPUs under classification %s", c.InstanceType, it.GPUCount, res.Classification)
				}
				if (res.Classification == ClassBurstable) != it.Burstable {
					t.Errorf("%s burstable=%v under classification %s", c.InstanceType, it.Burstable, res.Classification)
				}
			}
		})
	}
}

func TestNormalizePool_Defaults(t *testing.T) {
	got := NormalizePool(PoolConstraints{})
	if got.AMIFamily != AMIFamilyAL2023 {
		t.Errorf("AMIFamily = %q, want %q", got.AMIFamily, AMIFamilyAL2023)
	}
	if got.NetworkMode != NetworkModeBridge {
		t.Errorf("NetworkMode = %q, want %q", got.NetworkMode, NetworkModeBridge)
	}
	if got := NormalizePool(PoolConstraints{NetworkMode: "host"}).NetworkMode; got != NetworkModeBridge {
		t.Errorf("an unrecognised network mode became %q, want %q", got, NetworkModeBridge)
	}
	if got := NormalizePosture("nonsense"); got != PosturePerformance {
		t.Errorf("NormalizePosture(nonsense) = %q, want %q", got, PosturePerformance)
	}
}
