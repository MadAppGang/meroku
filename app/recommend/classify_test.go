package recommend

import (
	"math"
	"testing"
)

const eps = 1e-9

func nearlyEqual(a, b, tol float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d <= tol
}

// TestClassify pins FR-18's boundaries. 3.0 and 6.0 are the midpoints between
// the AWS family ratios (c = 2, m = 4, r = 8 GiB per vCPU).
func TestClassify(t *testing.T) {
	cases := []struct {
		name string
		in   ClassifyInput
		want string
	}{
		{"eight GiB per vCPU is memory heavy", ClassifyInput{REff: 8, Posture: PostureBalanced}, ClassMemoryHeavy},
		{"two GiB per vCPU is cpu heavy", ClassifyInput{REff: 2, Posture: PostureBalanced}, ClassCPUHeavy},
		{"exactly three is balanced", ClassifyInput{REff: 3.0, Posture: PostureBalanced}, ClassBalanced},
		{"just under three is cpu heavy", ClassifyInput{REff: 2.999, Posture: PostureBalanced}, ClassCPUHeavy},
		{"exactly six is memory heavy", ClassifyInput{REff: 6.0, Posture: PostureBalanced}, ClassMemoryHeavy},
		{"just under six is balanced", ClassifyInput{REff: 5.999, Posture: PostureBalanced}, ClassBalanced},
		{"gpu wins over every ratio", ClassifyInput{REff: 8, GPU: true, Posture: PostureBalanced}, ClassGPU},
		{
			"gpu wins over burstable",
			ClassifyInput{REff: 4, GPU: true, MaxTaskVCPU: 0.5, CPUAvg: 5, CPUPeak: 20, Coverage: 1, Posture: PostureBalanced},
			ClassGPU,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Classify(tc.in); got != tc.want {
				t.Errorf("Classify(%+v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestClassify_BurstableIsPerTask is C-15/DEV-28. FR-18's burstable row tested
// ConfiguredVCPU <= 2, which is the FR-14 FLEET aggregate: six tasks of
// 0.5 vCPU total 3.0, so the textbook burstable workload was excluded and
// FR-21.8 then actively removed t3/t4g from every realistic recommendation.
func TestClassify_BurstableIsPerTask(t *testing.T) {
	// Six tasks of 0.5 vCPU: fleet 3.0, MaxTaskVCPU 0.5.
	d := ConfiguredShape(uniformServices())
	if !nearlyEqual(d.VCPU, 3.0, eps) {
		t.Fatalf("fixture drift: ConfiguredVCPU = %v, want 3.0", d.VCPU)
	}
	if !nearlyEqual(d.MaxTaskVCPU, 0.5, eps) {
		t.Fatalf("fixture drift: MaxTaskVCPU = %v, want 0.5", d.MaxTaskVCPU)
	}

	base := ClassifyInput{REff: 4.0, CPUAvg: 15, CPUPeak: 40, Coverage: 1.0, Posture: PostureBalanced}

	fleetAggregate := base
	fleetAggregate.MaxTaskVCPU = d.MaxTaskVCPU
	if got := Classify(fleetAggregate); got != ClassBurstable {
		t.Errorf("six tasks of 0.5 vCPU at 15%% CPU classified %q, want %q", got, ClassBurstable)
	}

	cases := []struct {
		name        string
		maxTaskVCPU float64
		want        string
	}{
		{"exactly two vCPU per task is burstable", 2.0, ClassBurstable},
		{"a hair over two is not", 2.001, ClassBalanced},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := base
			in.MaxTaskVCPU = tc.maxTaskVCPU
			if got := Classify(in); got != tc.want {
				t.Errorf("MaxTaskVCPU %v classified %q, want %q", tc.maxTaskVCPU, got, tc.want)
			}
		})
	}

	// The other three conditions are unchanged and each one alone suppresses.
	suppressors := []struct {
		name string
		in   ClassifyInput
	}{
		{"cpuAvg at 20", ClassifyInput{REff: 4, MaxTaskVCPU: 0.5, CPUAvg: 20, CPUPeak: 40, Coverage: 1, Posture: PostureBalanced}},
		{"cpuPeak at 60", ClassifyInput{REff: 4, MaxTaskVCPU: 0.5, CPUAvg: 15, CPUPeak: 60, Coverage: 1, Posture: PostureBalanced}},
		{"coverage below half", ClassifyInput{REff: 4, MaxTaskVCPU: 0.5, CPUAvg: 15, CPUPeak: 40, Coverage: 0.49, Posture: PostureBalanced}},
	}
	for _, tc := range suppressors {
		t.Run(tc.name, func(t *testing.T) {
			if got := Classify(tc.in); got == ClassBurstable {
				t.Errorf("expected the burstable condition to be suppressed, got %q", got)
			}
		})
	}
}

// TestClassify_PerformanceFirstSuppressesBurstable is FR-19/AC-15: burstable
// CPU credits make steady-state throughput unpredictable.
func TestClassify_PerformanceFirstSuppressesBurstable(t *testing.T) {
	in := ClassifyInput{REff: 4.0, MaxTaskVCPU: 0.5, CPUAvg: 5, CPUPeak: 20, Coverage: 1.0}

	in.Posture = PostureBalanced
	if got := Classify(in); got != ClassBurstable {
		t.Fatalf("balanced classified %q, want %q", got, ClassBurstable)
	}
	in.Posture = PostureCost
	if got := Classify(in); got != ClassBurstable {
		t.Fatalf("cost-first classified %q, want %q", got, ClassBurstable)
	}
	in.Posture = PosturePerformance
	if got := Classify(in); got != ClassBalanced {
		t.Errorf("performance-first classified %q, want %q (reclassified by ratio)", got, ClassBalanced)
	}
}

// TestBlend pins FR-17's weights and FR-27's exactness.
func TestBlend(t *testing.T) {
	cases := []struct {
		name             string
		rCfg, rAct       float64
		coverage         float64
		wantRaw          float64
		wantCfg, wantAct float64
	}{
		{"no coverage yields R_cfg exactly", 4.0, 22.0, 0.0, 4.0, 1.0, 0.0},
		{"full coverage caps actuals at 0.60", 4.0, 24.0, 1.0, 0.4*4.0 + 0.6*24.0, 0.40, 0.60},
		{"half coverage halves the actual weight", 4.0, 24.0, 0.5, 0.7*4.0 + 0.3*24.0, 0.70, 0.30},
		{"coverage above one is clamped", 4.0, 24.0, 1.7, 0.4*4.0 + 0.6*24.0, 0.40, 0.60},
		{"negative coverage is clamped to zero", 4.0, 24.0, -1, 4.0, 1.0, 0.0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, wCfg, wAct := Blend(tc.rCfg, tc.rAct, tc.coverage)
			if !nearlyEqual(raw, tc.wantRaw, eps) {
				t.Errorf("R_raw = %v, want %v", raw, tc.wantRaw)
			}
			if !nearlyEqual(wCfg, tc.wantCfg, eps) || !nearlyEqual(wAct, tc.wantAct, eps) {
				t.Errorf("weights = (%v,%v), want (%v,%v)", wCfg, wAct, tc.wantCfg, tc.wantAct)
			}
			if !nearlyEqual(wCfg+wAct, 1.0, eps) {
				t.Errorf("weights sum to %v, want 1.0", wCfg+wAct)
			}
		})
	}

	// FR-27 says EXACTLY, so the zero case is asserted with == and not with a
	// tolerance.
	if raw, _, _ := Blend(4.3333333333333, 99, 0); raw != 4.3333333333333 {
		t.Errorf("coverage 0 returned %v, want the R_cfg bit pattern back unchanged", raw)
	}
}

// TestBlend_ClampsToCatalogRange is C-10/DEV-27, on the unit.
func TestBlend_ClampsToCatalogRange(t *testing.T) {
	cases := []struct {
		name        string
		raw, lo, hi float64
		wantEff     float64
		wantClamp   string
	}{
		{"inside the range", 4.0, 2.0, 16.0, 4.0, ClampNone},
		{"above the ceiling", 22.3, 0.5, 16.0, 16.0, ClampMax},
		{"below the floor", 0.25, 2.0, 16.0, 2.0, ClampMin},
		{"exactly the ceiling", 16.0, 2.0, 16.0, 16.0, ClampNone},
		{"exactly the floor", 2.0, 2.0, 16.0, 2.0, ClampNone},
		{"an empty range cannot clamp", 22.3, 0, 0, 22.3, ClampNone},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			eff, clamped := ClampRatio(tc.raw, tc.lo, tc.hi)
			if !nearlyEqual(eff, tc.wantEff, eps) || clamped != tc.wantClamp {
				t.Errorf("ClampRatio(%v,%v,%v) = (%v,%q), want (%v,%q)",
					tc.raw, tc.lo, tc.hi, eff, clamped, tc.wantEff, tc.wantClamp)
			}
		})
	}
}

// TestBlend_ClampsToCatalogRangeEndToEnd is the same finding through
// Recommend, including section 9's requirement that fit keeps discriminating:
// at least one candidate must score fit > 0.9 after the clamp.
func TestBlend_ClampsToCatalogRangeEndToEnd(t *testing.T) {
	// 80 % memory against 6 % CPU: a JVM with a large resident heap idling
	// between requests. R_act = 9.6/0.18 = 53.33, R_raw = 33.6, and no
	// instance on the market has that shape.
	in := baseInput()
	in.Catalog = wideCatalog()
	in.Posture = PosturePerformance // FR-19 keeps this off the burstable branch
	in.Services = []ServiceDemand{{
		Name: "backend", VCPU: 0.5, MemGiB: 2, Count: 6,
		CPUAvg: 5, CPUPeak: 6, MemAvg: 70, MemPeak: 80, Datapoints: 336,
	}}

	got := Recommend(in)
	if got.Signals.Ratio.ClampedTo != ClampMax {
		t.Fatalf("clampedTo = %q, want %q (raw %v, catalogMax %v)",
			got.Signals.Ratio.ClampedTo, ClampMax, got.Signals.Ratio.Raw, got.Signals.Ratio.CatalogMax)
	}
	if !nearlyEqual(got.Signals.Ratio.CatalogMax, 16.0, eps) {
		t.Errorf("catalogMax = %v, want 16.0", got.Signals.Ratio.CatalogMax)
	}
	if !nearlyEqual(got.Signals.Ratio.Effective, 16.0, eps) {
		t.Errorf("R_eff = %v, want 16.0", got.Signals.Ratio.Effective)
	}
	if got.Signals.Ratio.Raw <= got.Signals.Ratio.CatalogMax {
		t.Errorf("raw %v should exceed the catalog ceiling %v", got.Signals.Ratio.Raw, got.Signals.Ratio.CatalogMax)
	}
	best := 0.0
	for _, c := range got.Ranked {
		best = maxf(best, c.Scores.Fit)
	}
	if best <= 0.9 {
		t.Errorf("best fit after the clamp = %v; fit has stopped discriminating", best)
	}

	// The min case, end to end: a 1.0 GiB/vCPU workload against a catalog
	// whose floor is 2.0.
	low := baseInput()
	low.Services = []ServiceDemand{{Name: "backend", VCPU: 1, MemGiB: 1, Count: 2}}
	lowGot := Recommend(low)
	if lowGot.Signals.Ratio.ClampedTo != ClampMin {
		t.Errorf("clampedTo = %q, want %q (raw %v, catalogMin %v)",
			lowGot.Signals.Ratio.ClampedTo, ClampMin, lowGot.Signals.Ratio.Raw, lowGot.Signals.Ratio.CatalogMin)
	}
	if !nearlyEqual(lowGot.Signals.Ratio.Effective, lowGot.Signals.Ratio.CatalogMin, eps) {
		t.Errorf("R_eff = %v, want the catalog floor %v", lowGot.Signals.Ratio.Effective, lowGot.Signals.Ratio.CatalogMin)
	}

	// And the "none" case, so all three values of clampedTo are covered.
	plain := Recommend(baseInput())
	if plain.Signals.Ratio.ClampedTo != ClampNone {
		t.Errorf("clampedTo = %q, want %q", plain.Signals.Ratio.ClampedTo, ClampNone)
	}
}

// TestActualShape_ZeroCPUPeak is division 2 of note 5: an idle JVM with a full
// window of datapoints reports cpuPeak 0.0, and R_act would be +Inf.
func TestActualShape_ZeroCPUPeak(t *testing.T) {
	services := []ServiceDemand{
		{Name: "idle-a", VCPU: 0.5, MemGiB: 2, Count: 3, CPUAvg: 0, CPUPeak: 0, MemAvg: 40, MemPeak: 55, Datapoints: 336},
		{Name: "idle-b", VCPU: 1.0, MemGiB: 4, Count: 1, CPUAvg: 0, CPUPeak: 0, MemAvg: 30, MemPeak: 44, Datapoints: 336},
	}
	act := ActualShape(services)
	if act.OK {
		t.Fatalf("ActualShape reported ok with every cpuPeak at zero: %+v", act)
	}
	if math.IsInf(act.Ratio, 0) || math.IsNaN(act.Ratio) {
		t.Errorf("R_act = %v, want a finite zero value rather than an infinity", act.Ratio)
	}

	in := baseInput()
	in.Services = services
	got := Recommend(in)
	if got.Signals.Coverage != 0 {
		t.Errorf("coverage = %v, want 0 forced", got.Signals.Coverage)
	}
	if got.Signals.Actual != nil {
		t.Errorf("signals.actual = %+v, want nil", *got.Signals.Actual)
	}
	if got.Signals.WeightActual != 0 || got.Signals.WeightConfigured != 1 {
		t.Errorf("weights = (%v,%v), want (1,0)", got.Signals.WeightConfigured, got.Signals.WeightActual)
	}
	for _, s := range got.Signals.Services {
		if s.Status != StatusNoData {
			t.Errorf("service %q status = %q, want %q", s.Name, s.Status, StatusNoData)
		}
	}
	if !nearlyEqual(got.Signals.Ratio.Raw, got.Signals.Configured.Ratio, eps) {
		t.Errorf("R_raw = %v, want R_cfg %v", got.Signals.Ratio.Raw, got.Signals.Configured.Ratio)
	}
	if got.Basis != BasisConfigured {
		t.Errorf("basis = %q, want %q", got.Basis, BasisConfigured)
	}
}

// TestCoverage_IsDemandWeighted: a big service with data outweighs a small one
// without, because coverage feeds a weight on the ratio, not a headcount.
func TestCoverage_IsDemandWeighted(t *testing.T) {
	services := []ServiceDemand{
		{Name: "big", VCPU: 3, MemGiB: 6, Count: 1, CPUPeak: 40, MemPeak: 50, Datapoints: 336},
		{Name: "small", VCPU: 1, MemGiB: 2, Count: 1},
	}
	got := Coverage(services)
	want := 3.0 / 4.0
	if !nearlyEqual(got, want, eps) {
		t.Errorf("Coverage = %v, want %v", got, want)
	}

	// Under 24 datapoints a service does not count, however much demand it
	// carries.
	services[0].Datapoints = 23
	if got := Coverage(services); got != 0 {
		t.Errorf("Coverage with 23 datapoints = %v, want 0", got)
	}
}

// TestConfiguredShape_DropsMalformedServices is the core's second belt behind
// the boundary guard (C-9).
func TestConfiguredShape_DropsMalformedServices(t *testing.T) {
	d := ConfiguredShape([]ServiceDemand{
		{Name: "zero-cpu", VCPU: 0, MemGiB: 2, Count: 3},
		{Name: "zero-mem", VCPU: 0.5, MemGiB: 0, Count: 3},
		{Name: "zero-count", VCPU: 0.5, MemGiB: 2, Count: 0},
		{Name: "negative", VCPU: -1, MemGiB: 2, Count: 1},
		{Name: "nan", VCPU: math.NaN(), MemGiB: 2, Count: 1},
		{Name: "good", VCPU: 0.5, MemGiB: 2, Count: 6},
	})
	if d.TaskCount != 6 {
		t.Errorf("TaskCount = %d, want 6 (only the good service)", d.TaskCount)
	}
	if !nearlyEqual(d.VCPU, 3.0, eps) || !nearlyEqual(d.MemGiB, 12.0, eps) {
		t.Errorf("shape = (%v,%v), want (3.0,12.0)", d.VCPU, d.MemGiB)
	}
	if !nearlyEqual(d.Ratio, 4.0, eps) {
		t.Errorf("R_cfg = %v, want 4.0", d.Ratio)
	}
	if !nearlyEqual(d.VCPUPerTask, 0.5, eps) || !nearlyEqual(d.MemGiBPerTask, 2.0, eps) {
		t.Errorf("per task = (%v,%v), want (0.5,2.0)", d.VCPUPerTask, d.MemGiBPerTask)
	}
}
