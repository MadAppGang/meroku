package recommend_test

// Black-box tests for the pure recommendation core.
//
// This file is in package recommend_test on purpose: it may touch nothing but
// the exported surface, which is the same surface api_compute.go and any future
// caller has. Every expectation below is derived from requirements.md
// (FR-17...FR-30, EC-2, EC-6, EC-11, EC-12, NFR-6, NFR-9), from decisions.md
// (D-1, D-6, D-8) and from architecture.md section 3.3's response contract --
// never from what the implementation happens to do. Where two documents
// disagree the comment names both and the test asserts the higher-precedence
// one, so a failure can be classified without re-litigating the spec.
//
// CON-5: every fixture value here is synthetic. No account, invented prices,
// and two invented instance types (m7i-metal.metal, m7i-thin.large) that do not exist in
// AWS, so it is obvious no number was captured from a live account.

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"

	"madappgang.com/meroku/recommend"
)

func fp(v float64) *float64 { return &v }
func ip(v int) *int         { return &v }

// ---------------------------------------------------------------------------
// CAT-A -- the shared synthetic catalog (test plan section 1.1)
// ---------------------------------------------------------------------------
//
// Sixteen records for ap-southeast-2. Every row exists to be the sole survivor
// or the sole casualty of one rule: m7i-thin.large carries 2 ENIs (the D-6 /
// FR-21.6 contradiction), m7i-metal.metal is bare metal, m5.large is previous
// generation, r7a.large is unpriced, m6i.large has no spot market, g5.xlarge is
// the only GPU, t3.medium the only burstable, m7g.large the only arm64.
//
// Over the al2023 (x86, non-GPU) pre-filter survivors the ratio range is
// [2.0, 8.0], so a configured ratio inside that band is never clamped.
func catA() []recommend.InstanceType {
	x86 := []string{recommend.ArchX8664}
	arm := []string{recommend.ArchARM64}
	return []recommend.InstanceType{
		{Name: "c7i.large", VCPU: 2, MemoryMiB: 4096, Architectures: x86, CurrentGeneration: true,
			MaxNetworkInterfaces: 3, SupportsSpot: true, OnDemandHourly: fp(0.1000), SpotMedianHourly: fp(0.0400)},
		{Name: "c7i.xlarge", VCPU: 4, MemoryMiB: 8192, Architectures: x86, CurrentGeneration: true,
			MaxNetworkInterfaces: 4, SupportsSpot: true, OnDemandHourly: fp(0.2000), SpotMedianHourly: fp(0.0800)},
		{Name: "c6i.large", VCPU: 2, MemoryMiB: 4096, Architectures: x86, CurrentGeneration: true,
			MaxNetworkInterfaces: 3, SupportsSpot: true, OnDemandHourly: fp(0.0950), SpotMedianHourly: fp(0.0380)},
		{Name: "m7i.large", VCPU: 2, MemoryMiB: 8192, Architectures: x86, CurrentGeneration: true,
			MaxNetworkInterfaces: 3, SupportsSpot: true, OnDemandHourly: fp(0.1200), SpotMedianHourly: fp(0.0480)},
		{Name: "m7i.xlarge", VCPU: 4, MemoryMiB: 16384, Architectures: x86, CurrentGeneration: true,
			MaxNetworkInterfaces: 4, SupportsSpot: true, OnDemandHourly: fp(0.2400), SpotMedianHourly: fp(0.0960)},
		// No spot market at all (EC-6): SupportsSpot false and no median.
		{Name: "m6i.large", VCPU: 2, MemoryMiB: 8192, Architectures: x86, CurrentGeneration: true,
			MaxNetworkInterfaces: 3, OnDemandHourly: fp(0.1150)},
		{Name: "m7g.large", VCPU: 2, MemoryMiB: 8192, Architectures: arm, CurrentGeneration: true,
			MaxNetworkInterfaces: 3, SupportsSpot: true, OnDemandHourly: fp(0.1000), SpotMedianHourly: fp(0.0400)},
		{Name: "r7i.large", VCPU: 2, MemoryMiB: 16384, Architectures: x86, CurrentGeneration: true,
			MaxNetworkInterfaces: 3, SupportsSpot: true, OnDemandHourly: fp(0.1600), SpotMedianHourly: fp(0.0640)},
		{Name: "r7i.xlarge", VCPU: 4, MemoryMiB: 32768, Architectures: x86, CurrentGeneration: true,
			MaxNetworkInterfaces: 4, SupportsSpot: true, OnDemandHourly: fp(0.3200), SpotMedianHourly: fp(0.1280)},
		{Name: "r6i.large", VCPU: 2, MemoryMiB: 16384, Architectures: x86, CurrentGeneration: true,
			MaxNetworkInterfaces: 3, SupportsSpot: true, OnDemandHourly: fp(0.1550), SpotMedianHourly: fp(0.0620)},
		{Name: "t3.medium", VCPU: 2, MemoryMiB: 4096, Architectures: x86, CurrentGeneration: true,
			Burstable: true, MaxNetworkInterfaces: 3, SupportsSpot: true,
			OnDemandHourly: fp(0.0500), SpotMedianHourly: fp(0.0200)},
		{Name: "m5.large", VCPU: 2, MemoryMiB: 8192, Architectures: x86, CurrentGeneration: false,
			MaxNetworkInterfaces: 3, SupportsSpot: true, OnDemandHourly: fp(0.1100), SpotMedianHourly: fp(0.0440)},
		{Name: "g5.xlarge", VCPU: 4, MemoryMiB: 16384, Architectures: x86, CurrentGeneration: true,
			GPUCount: 1, MaxNetworkInterfaces: 4, SupportsSpot: true,
			OnDemandHourly: fp(1.0000), SpotMedianHourly: fp(0.4000)},
		{Name: "m7i-metal.metal", VCPU: 96, MemoryMiB: 786432, Architectures: x86, CurrentGeneration: true,
			BareMetal: true, MaxNetworkInterfaces: 15, SupportsSpot: true,
			OnDemandHourly: fp(5.0000), SpotMedianHourly: fp(2.0000)},
		// Two ENIs: eligible under bridge (D-6), excluded under awsvpc (FR-21.6).
		{Name: "m7i-thin.large", VCPU: 2, MemoryMiB: 8192, Architectures: x86, CurrentGeneration: true,
			MaxNetworkInterfaces: 2, SupportsSpot: true, OnDemandHourly: fp(0.0900), SpotMedianHourly: fp(0.0360)},
		// Unpriced (EC-2): in the catalog, never in the ranking.
		{Name: "r7a.large", VCPU: 2, MemoryMiB: 16384, Architectures: x86, CurrentGeneration: true,
			MaxNetworkInterfaces: 3, SupportsSpot: true, SpotMedianHourly: fp(0.0600)},
	}
}

// catAZeroPriced adds a row priced at exactly 0.00. A zero price is not "free",
// it is a bad record: it would make the cost normaliser's minimum 0 and every
// cost sub-score 0/0 (EC-2).
func catAZeroPriced() []recommend.InstanceType {
	return append(catA(), recommend.InstanceType{
		Name: "c7a.large", VCPU: 2, MemoryMiB: 4096, Architectures: []string{recommend.ArchX8664},
		CurrentGeneration: true, MaxNetworkInterfaces: 3, SupportsSpot: true,
		OnDemandHourly: fp(0), SpotMedianHourly: fp(0),
	})
}

// catASmall is CAT-A-small: two candidates, for the nearest-miss length
// question (EC-11 says three, and a catalog of two cannot supply three).
func catASmall() []recommend.InstanceType {
	full := catA()
	return []recommend.InstanceType{full[0], full[3]} // c7i.large, m7i.large
}

func catOnly(name string) []recommend.InstanceType {
	for _, it := range catA() {
		if it.Name == name {
			return []recommend.InstanceType{it}
		}
	}
	panic("no such fixture type: " + name)
}

func catalogByName(catalog []recommend.InstanceType) map[string]recommend.InstanceType {
	out := make(map[string]recommend.InstanceType, len(catalog))
	for _, it := range catalog {
		out[it.Name] = it
	}
	return out
}

// ---------------------------------------------------------------------------
// Environment fixtures (test plan section 1.2)
// ---------------------------------------------------------------------------

type envFixture struct {
	name     string
	services []recommend.ServiceDemand
}

// mib converts a MiB figure to GiB exactly: 1024 is a power of two, so
// 3072/1024 is 3.0 with no rounding at all. That exactness is the whole point
// of the FR-18 boundary rows -- an epsilon comparison would hide a > written
// where a >= was meant.
func mib(v float64) float64 { return v / 1024.0 }

func envFixtures() []envFixture {
	return []envFixture{
		{"ENV-CPU2", []recommend.ServiceDemand{
			{Name: "api", VCPU: 1, MemGiB: 2, Count: 3},
			{Name: "worker", VCPU: 1, MemGiB: 2, Count: 3},
		}},
		{"ENV-BAL3", []recommend.ServiceDemand{{Name: "api", VCPU: 1, MemGiB: mib(3072), Count: 4}}},
		{"ENV-BAL3-", []recommend.ServiceDemand{{Name: "api", VCPU: 1, MemGiB: mib(3071), Count: 4}}},
		{"ENV-MEM6", []recommend.ServiceDemand{{Name: "api", VCPU: 1, MemGiB: mib(6144), Count: 4}}},
		{"ENV-MEM6-", []recommend.ServiceDemand{{Name: "api", VCPU: 1, MemGiB: mib(6143), Count: 4}}},
		{"ENV-MEM8", envMem8()},
		{"ENV-MEM8-measured", envMem8Measured()},
		{"ENV-BURST", []recommend.ServiceDemand{{Name: "api", VCPU: 1, MemGiB: 2, Count: 2}}},
		{"ENV-MIXED", envMixed()},
		{"ENV-ZERO", nil},
		{"ENV-ZEROCPU", []recommend.ServiceDemand{{Name: "api", VCPU: 0, MemGiB: 4, Count: 2}}},
		{"ENV-HUGE", []recommend.ServiceDemand{{Name: "api", VCPU: 4, MemGiB: 200, Count: 1}}},
		{"ENV-CAP1", envCap1()},
	}
}

func envMem8() []recommend.ServiceDemand {
	return []recommend.ServiceDemand{
		{Name: "api", VCPU: 0.5, MemGiB: 4, Count: 3},
		{Name: "worker", VCPU: 0.5, MemGiB: 4, Count: 3},
	}
}

// envMem8Measured is ENV-MEM8 with CW-FULL: a complete 14-day window on every
// in-scope service, so coverage is 1.0 and FR-17's 0.60 cap is the only thing
// keeping the actuals from dominating.
func envMem8Measured() []recommend.ServiceDemand {
	out := envMem8()
	for i := range out {
		out[i].CPUAvg, out[i].CPUPeak = 9.4, 12.0
		out[i].MemAvg, out[i].MemPeak = 61.2, 78.0
		out[i].Datapoints = 336
	}
	return out
}

// envMixed is FR-17's demand-weighting question: one large service with a full
// window, three trivial sidecars with none. Count-weighting would report
// coverage 0.25; demand-weighting reports 4.0/4.75.
func envMixed() []recommend.ServiceDemand {
	out := []recommend.ServiceDemand{{
		Name: "backend", VCPU: 4, MemGiB: 8, Count: 1,
		CPUAvg: 9.4, CPUPeak: 12.0, MemAvg: 61.2, MemPeak: 78.0, Datapoints: 336,
	}}
	for _, n := range []string{"sidecar1", "sidecar2", "sidecar3"} {
		out = append(out, recommend.ServiceDemand{Name: n, VCPU: 0.25, MemGiB: 0.5, Count: 1})
	}
	return out
}

// envCap1 is 20 tasks of 2 vCPU / 4 GiB -- enough demand that a min_size of 1
// cannot hold it (D-8 / DEV-26).
func envCap1() []recommend.ServiceDemand {
	var out []recommend.ServiceDemand
	for _, n := range []string{"a", "b", "c", "d"} {
		out = append(out, recommend.ServiceDemand{Name: n, VCPU: 2, MemGiB: 4, Count: 5})
	}
	return out
}

func postures() []recommend.Posture {
	return []recommend.Posture{
		recommend.PosturePerformance, recommend.PostureBalanced, recommend.PostureCost,
	}
}

// input is the everyday request against CAT-A: bridge networking, the default
// x86 AMI family, no pinned pool bounds, and enough limit to see the whole
// eligible set.
func input(services []recommend.ServiceDemand, posture recommend.Posture) recommend.Input {
	return recommend.Input{
		Region:   "ap-southeast-2",
		Catalog:  catA(),
		Services: services,
		Pool:     recommend.PoolConstraints{Name: "general"},
		Posture:  posture,
		Limit:    50,
	}
}

// ---------------------------------------------------------------------------
// Finiteness -- the headline property
// ---------------------------------------------------------------------------

// nonFiniteFields walks a value and reports the path of every NaN or Inf it
// carries. encoding/json refuses to marshal either, so a single one turns a
// 200 into a truncated body no client can parse -- and NFR-6 is explicit that
// "a blank screen, a spinner that never resolves, or a 500 is a defect".
// Walking the struct rather than relying on json.Marshal's error is what makes
// the failure message name the offending field.
func nonFiniteFields(v reflect.Value, path string, out *[]string) {
	switch v.Kind() {
	case reflect.Float64, reflect.Float32:
		f := v.Float()
		if math.IsNaN(f) || math.IsInf(f, 0) {
			*out = append(*out, fmt.Sprintf("%s = %v", path, f))
		}
	case reflect.Pointer, reflect.Interface:
		if !v.IsNil() {
			nonFiniteFields(v.Elem(), path, out)
		}
	case reflect.Struct:
		t := v.Type()
		for i := 0; i < v.NumField(); i++ {
			if !t.Field(i).IsExported() {
				continue
			}
			nonFiniteFields(v.Field(i), path+"."+t.Field(i).Name, out)
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			nonFiniteFields(v.Index(i), fmt.Sprintf("%s[%d]", path, i), out)
		}
	case reflect.Map:
		for _, k := range v.MapKeys() {
			nonFiniteFields(v.MapIndex(k), fmt.Sprintf("%s[%v]", path, k.Interface()), out)
		}
	}
}

// assertEncodable is the contract every compute endpoint rests on: the answer
// serialises. It checks the fields first so the message names the culprit, then
// the encoder, because only the encoder proves it end to end.
func assertEncodable(t *testing.T, result recommend.Result, what string) {
	t.Helper()

	var bad []string
	nonFiniteFields(reflect.ValueOf(result), "result", &bad)
	if len(bad) > 0 {
		t.Errorf("%s: non-finite values reached the response:\n  %s", what, strings.Join(bad, "\n  "))
	}

	body, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("%s: the recommendation cannot be encoded as JSON: %v\n"+
			"NFR-6 requires every compute endpoint to answer with a body the UI can render. "+
			"encoding/json aborts mid-object on a non-finite float, so the client gets HTTP 200 "+
			"and a truncated body, with nothing in the logs naming the cause.", what, err)
	}
	for _, token := range []string{"NaN", "Inf", "inf"} {
		if strings.Contains(string(body), token) {
			t.Errorf("%s: the encoded body contains the token %q: %s", what, token, body)
		}
	}
}

// ---------------------------------------------------------------------------
// A-43 / A-23 -- the catch-all over the whole fixture matrix
// ---------------------------------------------------------------------------

// TestBlackBox_MatrixIsEncodableDroppedIsEmptyAndRulesHold walks every
// catalog x environment x posture x network mode in the plan and asserts the
// three properties that hold unconditionally:
//
//   - the result encodes as JSON (NFR-6)
//   - signals.dropped is empty (architecture 3.3: a non-empty dropped is a bug,
//     not a state a user is ever meant to see)
//   - every ranked entry satisfies every hard rule of FR-21, with the ENI rule
//     scoped to awsvpc per D-6
//
// Contradiction X-1: FR-21.6 and AC-23 state the >= 3 ENI floor
// unconditionally; D-6 scopes it to awsvpc. Decisions outrank requirements, so
// a low-ENI type excluded under bridge is an IMPLEMENTATION_ISSUE.
func TestBlackBox_MatrixIsEncodableDroppedIsEmptyAndRulesHold(t *testing.T) {
	catalogs := map[string][]recommend.InstanceType{
		"CAT-A":            catA(),
		"CAT-A-zeropriced": catAZeroPriced(),
		"CAT-A-small":      catASmall(),
	}
	modes := []string{recommend.NetworkModeBridge, recommend.NetworkModeAWSVPC}

	for catName, catalog := range catalogs {
		byName := catalogByName(catalog)
		for _, env := range envFixtures() {
			for _, posture := range postures() {
				for _, mode := range modes {
					name := fmt.Sprintf("%s/%s/%s/%s", catName, env.name, posture, mode)
					t.Run(name, func(t *testing.T) {
						result := recommend.Recommend(recommend.Input{
							Region:   "ap-southeast-2",
							Catalog:  catalog,
							Services: env.services,
							Pool:     recommend.PoolConstraints{Name: "general", NetworkMode: mode},
							Posture:  posture,
							Limit:    50,
						})

						assertEncodable(t, result, name)

						if len(result.Signals.Dropped) != 0 {
							t.Errorf("signals.dropped is not empty: %+v\n"+
								"architecture 3.3 records a non-empty dropped as a defect: a candidate "+
								"vanished for a non-finite score and no user-visible field says why",
								result.Signals.Dropped)
						}

						assertHardRules(t, result, byName, mode)
					})
				}
			}
		}
	}
}

// assertHardRules is FR-21 restated as a property over the answer rather than
// over the filter: only a property test catches a rule applied during filtering
// and then undone downstream by the pool builder or the nearest-miss path.
func assertHardRules(t *testing.T, result recommend.Result, byName map[string]recommend.InstanceType, mode string) {
	t.Helper()

	for i, c := range result.Ranked {
		it, known := byName[c.InstanceType]
		if !known {
			t.Errorf("ranked[%d] is %q, which is not in the catalog (FR-20)", i, c.InstanceType)
			continue
		}
		if !it.CurrentGeneration {
			t.Errorf("ranked[%d] %q is previous generation (FR-21.1)", i, c.InstanceType)
		}
		if it.BareMetal {
			t.Errorf("ranked[%d] %q is bare metal (FR-21.3)", i, c.InstanceType)
		}
		if it.OnDemandHourly == nil || *it.OnDemandHourly <= 0 {
			t.Errorf("ranked[%d] %q has no usable on-demand price (FR-21.7, EC-2)", i, c.InstanceType)
		}
		if c.Architecture != recommend.ArchX8664 {
			t.Errorf("ranked[%d] %q is %s under the x86-only al2023 AMI family (FR-21.4)",
				i, c.InstanceType, c.Architecture)
		}
		if it.GPUCount != 0 && result.Classification != recommend.ClassGPU {
			t.Errorf("ranked[%d] %q carries a GPU for a %s workload (FR-21.5)",
				i, c.InstanceType, result.Classification)
		}
		if it.Burstable != (result.Classification == recommend.ClassBurstable) {
			t.Errorf("ranked[%d] %q burstable=%v for classification %q (FR-21.8)",
				i, c.InstanceType, it.Burstable, result.Classification)
		}
		if c.TasksPerInstance < 1 {
			t.Errorf("ranked[%d] %q reports tasksPerInstance=%d; a candidate that holds no task "+
				"divides by zero in both cost and waste", i, c.InstanceType, c.TasksPerInstance)
		}
		// X-1: the ENI floor is D-6-scoped to awsvpc. Under bridge, tasks share
		// the instance's primary interface and the rule does not exist.
		if mode == recommend.NetworkModeAWSVPC && it.MaxNetworkInterfaces < 3 {
			t.Errorf("ranked[%d] %q has %d ENIs under awsvpc (FR-21.6 requires >= 3)",
				i, c.InstanceType, it.MaxNetworkInterfaces)
		}
	}

	if result.Primary != nil && len(result.Ranked) > 0 &&
		result.Primary.InstanceType != result.Ranked[0].InstanceType {
		t.Errorf("primary is %q but ranked[0] is %q; FR-26 makes them the same candidate",
			result.Primary.InstanceType, result.Ranked[0].InstanceType)
	}
}

// ---------------------------------------------------------------------------
// A-42 / A-43 -- the two most reachable non-finites
// ---------------------------------------------------------------------------

// TestBlackBox_SpotMedianIsSanitisedBeforeItIsPublished is the headline
// scenario. SpotMedianHourly is the one number in the response that the core
// copies from its input straight into the answer, and it is a *float64 the
// caller populates from DescribeSpotPriceHistory.
//
// FR-21.7 as implemented already refuses a non-finite ON-DEMAND price, which
// establishes that a non-finite price is a bad record rather than an
// impossibility. The same defence has to reach the field that is published:
// EC-5 says an unavailable spot price is `median: null` and "never a 0 % or a
// nominal saving", and NFR-6 says every endpoint answers with a body the UI can
// render. A +Inf median satisfies neither -- encoding/json aborts on it, the
// browser receives a truncated 200, and nothing in the logs says why.
func TestBlackBox_SpotMedianIsSanitisedBeforeItIsPublished(t *testing.T) {
	poison := []struct {
		name  string
		value float64
	}{
		{"positive infinity", math.Inf(1)},
		{"negative infinity", math.Inf(-1)},
		{"NaN", math.NaN()},
	}

	for _, p := range poison {
		t.Run(p.name, func(t *testing.T) {
			catalog := catA()
			for i := range catalog {
				if catalog[i].Name == "m7i.large" {
					catalog[i].SpotMedianHourly = fp(p.value)
				}
			}

			result := recommend.Recommend(recommend.Input{
				Region:   "ap-southeast-2",
				Catalog:  catalog,
				Services: envMem8(),
				Pool:     recommend.PoolConstraints{Name: "general"},
				Posture:  recommend.PostureBalanced,
				Limit:    50,
			})

			assertEncodable(t, result, "spot median "+p.name)

			for _, c := range result.Ranked {
				if c.SpotMedianHourly == nil {
					continue
				}
				v := *c.SpotMedianHourly
				if math.IsNaN(v) || math.IsInf(v, 0) {
					t.Errorf("ranked candidate %q publishes spotMedianHourly = %v; EC-5 requires "+
						"an unusable spot price to be reported as null, never as a value",
						c.InstanceType, v)
				}
			}
		})
	}
}

// TestBlackBox_ZeroConfiguredVCPUProducesNoNonFinite is EC-12's reachable
// division by zero: task-level cpu is optional on EC2, so `cpu: 0` is legal
// YAML, and memGiB/0 is the shortest path to a NaN in the whole pipeline.
//
// Two documents disagree about the answer and this test asserts the invariant
// they share plus the resolution the later one records:
//
//   - requirements.md EC-12 says fall back to memory-only sizing, classify
//     memory_heavy and fix fit at 1.0 for every candidate.
//   - architecture.md C-14 replaces that row, having shown that with
//     R_eff = +Inf, FR-22's fit evaluates to 0 and not 1: a zero-vCPU service is
//     dropped from the demand vector instead, and if it is the only service the
//     result is the FR-28 default (Basis "default", Primary nil).
//
// C-14 is the later, reasoned correction, so it is what is asserted. What both
// require, and what actually protects the user, is asserted unconditionally:
// no NaN anywhere and a body that encodes.
func TestBlackBox_ZeroConfiguredVCPUProducesNoNonFinite(t *testing.T) {
	for _, posture := range postures() {
		t.Run(string(posture), func(t *testing.T) {
			result := recommend.Recommend(input(
				[]recommend.ServiceDemand{{Name: "api", VCPU: 0, MemGiB: 4, Count: 2}}, posture))

			assertEncodable(t, result, "ENV-ZEROCPU/"+string(posture))

			if result.Signals.Configured.Ratio != 0 {
				t.Errorf("signals.configured.ratio = %v; with no CPU requested the ratio is "+
					"undefined and must be reported as 0 or null, never computed",
					result.Signals.Configured.Ratio)
			}
			// architecture.md C-14, replacing requirements.md EC-12.
			if result.Basis != recommend.BasisDefault {
				t.Errorf("basis = %q, want %q (architecture C-14: a zero-vCPU service is dropped "+
					"from the demand vector, and a lone dropped service leaves no demand at all)",
					result.Basis, recommend.BasisDefault)
			}
			if result.Primary != nil {
				t.Errorf("primary = %q, want null", result.Primary.InstanceType)
			}
			if result.Unsatisfiable {
				t.Error("unsatisfiable = true for an environment with no demand; FR-28's default " +
					"and EC-11's refusal must stay distinguishable")
			}
			if len(result.Signals.Services) != 1 || result.Signals.Services[0].Status != recommend.StatusNoData {
				t.Errorf("signals.services = %+v; the dropped service must still be reported by "+
					"name with status no_data (FR-29)", result.Signals.Services)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// A-01 / A-14 -- boundaries that are exact numbers in the requirements
// ---------------------------------------------------------------------------

// TestBlackBox_ClassificationBoundariesAreExact is FR-18's 3.0 and 6.0
// GiB-per-vCPU thresholds, asserted AT the boundary rather than near it.
// 3072/1024 and 6144/1024 are exact in IEEE-754, so an epsilon here would hide
// a > written where a >= was meant -- which is the entire point of the rows.
func TestBlackBox_ClassificationBoundariesAreExact(t *testing.T) {
	tests := []struct {
		name      string
		services  []recommend.ServiceDemand
		wantRatio float64
		wantClass string
	}{
		{"ENV-CPU2 (2.0)", []recommend.ServiceDemand{{Name: "api", VCPU: 1, MemGiB: 2, Count: 3}},
			2.0, recommend.ClassCPUHeavy},
		{"ENV-BAL3- (just under 3.0)", []recommend.ServiceDemand{{Name: "api", VCPU: 1, MemGiB: mib(3071), Count: 4}},
			mib(3071), recommend.ClassCPUHeavy},
		{"ENV-BAL3 (exactly 3.0)", []recommend.ServiceDemand{{Name: "api", VCPU: 1, MemGiB: mib(3072), Count: 4}},
			3.0, recommend.ClassBalanced},
		{"ENV-MEM6- (just under 6.0)", []recommend.ServiceDemand{{Name: "api", VCPU: 1, MemGiB: mib(6143), Count: 4}},
			mib(6143), recommend.ClassBalanced},
		{"ENV-MEM6 (exactly 6.0)", []recommend.ServiceDemand{{Name: "api", VCPU: 1, MemGiB: mib(6144), Count: 4}},
			6.0, recommend.ClassMemoryHeavy},
		{"ENV-MEM8 (8.0)", envMem8(), 8.0, recommend.ClassMemoryHeavy},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := recommend.Recommend(input(tc.services, recommend.PostureBalanced))

			if result.Signals.Ratio.Effective != tc.wantRatio {
				t.Errorf("signals.ratio.effective = %v, want exactly %v (CAT-A's purchasable "+
					"range is [2.0, 8.0], so nothing here is clamped)",
					result.Signals.Ratio.Effective, tc.wantRatio)
			}
			if result.Classification != tc.wantClass {
				t.Errorf("classification = %q, want %q at R_eff = %v (FR-18)",
					result.Classification, tc.wantClass, result.Signals.Ratio.Effective)
			}
		})
	}
}

// TestBlackBox_UsableMemoryBoundaryIsExact pins FR-21.2's 15 % ECS agent + OS
// reserve at the boundary. m7i.large is 8192 MiB, so 0.85 of it is exactly
// 6963.2 MiB: a task of 6963 MiB places, a task of 6964 MiB does not.
//
// Three plausible implementations of "0.85" -- in GiB, in MiB, or rounded --
// differ by about 1 MiB and only one of them is right here. The consequence of
// getting it wrong is a task that sits in PROVISIONING forever with no error
// surfaced anywhere, which is exactly what FR-52.6 exists to prevent.
func TestBlackBox_UsableMemoryBoundaryIsExact(t *testing.T) {
	run := func(memMiB float64) recommend.Result {
		return recommend.Recommend(recommend.Input{
			Region:   "ap-southeast-2",
			Catalog:  catOnly("m7i.large"),
			Services: []recommend.ServiceDemand{{Name: "api", VCPU: 1, MemGiB: mib(memMiB), Count: 2}},
			Pool:     recommend.PoolConstraints{Name: "general"},
			Posture:  recommend.PostureBalanced,
			Limit:    50,
		})
	}

	t.Run("6963 MiB fits", func(t *testing.T) {
		result := run(6963)
		if result.Primary == nil || result.Primary.InstanceType != "m7i.large" {
			t.Fatalf("m7i.large was excluded for a task of 6963 MiB, which is below its usable "+
				"6963.2 MiB: primary = %v, constraint = %q", result.Primary, result.Constraint)
		}
		if result.Primary.TasksPerInstance != 1 {
			t.Errorf("tasksPerInstance = %d, want 1", result.Primary.TasksPerInstance)
		}
	})

	t.Run("6964 MiB does not fit", func(t *testing.T) {
		result := run(6964)
		for _, c := range result.Ranked {
			if c.InstanceType == "m7i.large" {
				t.Fatalf("m7i.large was ranked for a task of 6964 MiB, which is above its usable " +
					"6963.2 MiB; the task would never place (FR-21.2)")
			}
		}
		if !result.Unsatisfiable {
			t.Error("unsatisfiable = false with no eligible candidate; EC-11 requires an " +
				"explained refusal, never an empty list with no explanation")
		}
		if result.Constraint == "" {
			t.Error("constraint is empty; EC-11 requires it to name the rule and the numbers")
		}
	})
}

// ---------------------------------------------------------------------------
// A-25 -- density is a floor per dimension, and never zero
// ---------------------------------------------------------------------------

// TestBlackBox_TasksPerInstanceFloorsPerDimensionAndIsNeverZero pins FR-22's
// density on c7i.large (2 vCPU, 4 GiB -> 3.4 GiB usable) under bridge.
//
// tasksPerInstance == 0 is the other reachable divide-by-zero: it divides both
// cost (costPerTask) and utilisation, produces +Inf, and encoding/json then
// refuses the whole body. A candidate that holds no task must be excluded, not
// ranked with a zero.
func TestBlackBox_TasksPerInstanceFloorsPerDimensionAndIsNeverZero(t *testing.T) {
	tests := []struct {
		name        string
		vcpuPerTask float64
		memPerTask  float64
		wantTasks   int // 0 means "the candidate must be excluded entirely"
	}{
		{"0.25 vCPU / 0.5 GiB -> memory binds at 6", 0.25, 0.5, 6},
		{"0.75 vCPU / 0.5 GiB -> cpu binds at 2, floored not rounded", 0.75, 0.5, 2},
		{"2.0 vCPU / 3.4 GiB -> exactly one", 2.0, 3.4, 1},
		{"2.0 vCPU / 3.5 GiB -> excluded, never zero", 2.0, 3.5, 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := recommend.Recommend(recommend.Input{
				Region:  "ap-southeast-2",
				Catalog: catOnly("c7i.large"),
				Services: []recommend.ServiceDemand{
					{Name: "api", VCPU: tc.vcpuPerTask, MemGiB: tc.memPerTask, Count: 4},
				},
				Pool:    recommend.PoolConstraints{Name: "general"},
				Posture: recommend.PostureBalanced,
				Limit:   50,
			})

			assertEncodable(t, result, tc.name)

			for _, c := range result.Ranked {
				if c.TasksPerInstance == 0 {
					t.Fatalf("%q is ranked with tasksPerInstance = 0; costPerTask and utilisation "+
						"both divide by it, and +Inf cannot be encoded as JSON", c.InstanceType)
				}
			}

			if tc.wantTasks == 0 {
				if len(result.Ranked) != 0 {
					t.Fatalf("a task of %.1f GiB exceeds c7i.large's usable 3.4 GiB but it was "+
						"still ranked (FR-21.2)", tc.memPerTask)
				}
				if !result.Unsatisfiable {
					t.Error("unsatisfiable = false with nothing eligible (EC-11)")
				}
				if !strings.Contains(result.Constraint, "3.5") {
					t.Errorf("constraint = %q; AC-20 requires the numbers that caused the refusal",
						result.Constraint)
				}
				return
			}

			if result.Primary == nil {
				t.Fatalf("no primary; constraint = %q", result.Constraint)
			}
			if result.Primary.TasksPerInstance != tc.wantTasks {
				t.Errorf("tasksPerInstance = %d, want %d (floor per dimension, then the minimum)",
					result.Primary.TasksPerInstance, tc.wantTasks)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// A-07 -- zero coverage is exactly the configured ratio
// ---------------------------------------------------------------------------

// TestBlackBox_ZeroCoverageIsBitIdenticalToConfigured is FR-17's final
// sentence and FR-27. The blend is w_cfg*R_cfg + w_act*R_act, and R_act is
// 0/0 on an environment nothing has reported for -- which is the state of every
// new environment, the most common case there is. Evaluate it before applying
// the zero weight and R_eff is NaN, classification falls through to cpu_heavy,
// and every new project is handed a c-family recommendation for a memory-heavy
// workload.
func TestBlackBox_ZeroCoverageIsBitIdenticalToConfigured(t *testing.T) {
	result := recommend.Recommend(input(envMem8(), recommend.PosturePerformance))

	assertEncodable(t, result, "ENV-MEM8/no CloudWatch")

	if result.Signals.WeightActual != 0 || result.Signals.WeightConfigured != 1 {
		t.Errorf("weights = {configured %v, actual %v}, want {1, 0} (FR-17)",
			result.Signals.WeightConfigured, result.Signals.WeightActual)
	}
	if result.Signals.Actual != nil {
		t.Errorf("signals.actual = %+v, want null: nothing was measured, and a guessed "+
			"utilisation is a number the user will believe (FR-27)", result.Signals.Actual)
	}
	if result.Signals.Ratio.Effective != 8.0 {
		t.Errorf("signals.ratio.effective = %v, want exactly 8.0 -- not 8.000000000000002",
			result.Signals.Ratio.Effective)
	}
	if result.Signals.Ratio.Raw != 8.0 {
		t.Errorf("signals.ratio.raw = %v, want exactly 8.0", result.Signals.Ratio.Raw)
	}
	if result.Classification != recommend.ClassMemoryHeavy {
		t.Errorf("classification = %q, want memory_heavy", result.Classification)
	}
	if result.Basis != recommend.BasisConfigured {
		t.Errorf("basis = %q, want %q", result.Basis, recommend.BasisConfigured)
	}
	if result.Primary == nil || !strings.HasPrefix(result.Primary.InstanceType, "r") {
		t.Errorf("primary = %v, want an r-family type for a workload asking 8 GiB per vCPU",
			result.Primary)
	}
}

// ---------------------------------------------------------------------------
// The ten spec contradictions, X-1 ... X-10 (the four that live in this package)
// ---------------------------------------------------------------------------

// TestBlackBox_X1_ENIRuleAppliesOnlyUnderAWSVPC.
//
// Contradiction X-1: FR-21.6 and AC-23 require maximumNetworkInterfaces >= 3
// unconditionally; D-6 scopes the ENI limit to awsvpc pools, where each task
// takes its own interface. Decisions outrank requirements, so D-6 wins.
//
// The same fixture must give a ~3x density difference between the two modes on
// identical YAML. If it does not, the cost the tool shows is wrong by 3x with
// nothing in the response to reveal it -- and it presents to the user as a
// pricing bug.
func TestBlackBox_X1_ENIRuleAppliesOnlyUnderAWSVPC(t *testing.T) {
	services := []recommend.ServiceDemand{{Name: "api", VCPU: 0.25, MemGiB: 0.5, Count: 8}}

	run := func(mode string) recommend.Result {
		return recommend.Recommend(recommend.Input{
			Region:   "ap-southeast-2",
			Catalog:  catA(),
			Services: services,
			Pool:     recommend.PoolConstraints{Name: "general", NetworkMode: mode},
			Posture:  recommend.PostureCost,
			Limit:    50,
		})
	}
	ranked := func(r recommend.Result, name string) (recommend.Candidate, bool) {
		for _, c := range r.Ranked {
			if c.InstanceType == name {
				return c, true
			}
		}
		return recommend.Candidate{}, false
	}

	t.Run("bridge (the default) neither excludes nor caps", func(t *testing.T) {
		result := run(recommend.NetworkModeBridge)

		if result.Signals.NetworkMode != recommend.NetworkModeBridge {
			t.Errorf("signals.networkMode = %q, want bridge", result.Signals.NetworkMode)
		}
		if result.Signals.DensityBasis != recommend.DensityCPUMemoryOnly {
			t.Errorf("signals.densityBasis = %q, want %q under bridge",
				result.Signals.DensityBasis, recommend.DensityCPUMemoryOnly)
		}
		if result.Signals.Trunking != recommend.TrunkingNotApplicable {
			t.Errorf("signals.trunking = %q, want %q under bridge -- there are no task ENIs to trunk",
				result.Signals.Trunking, recommend.TrunkingNotApplicable)
		}
		if _, ok := ranked(result, "m7i-thin.large"); !ok {
			t.Errorf("m7i-thin.large (2 ENIs) is absent from ranked under bridge.\n"+
				"D-6 removed the ENI ceiling for bridge pools, and bridge is the DEFAULT: "+
				"excluding a type here discards it for a limit that does not apply.\n"+
				"ranked = %v", names(result.Ranked))
		}
		c, ok := ranked(result, "c7i.large")
		if !ok {
			t.Fatalf("c7i.large is absent from ranked: %v", names(result.Ranked))
		}
		if c.TasksPerInstance != 6 {
			t.Errorf("c7i.large tasksPerInstance = %d under bridge, want 6 "+
				"(memory binds: floor(3.4 / 0.5)); an ENI cap applied here deflates density ~3x",
				c.TasksPerInstance)
		}
	})

	t.Run("awsvpc both excludes and caps", func(t *testing.T) {
		result := run(recommend.NetworkModeAWSVPC)

		if result.Signals.DensityBasis != recommend.DensityMaxENIsMinus1 {
			t.Errorf("signals.densityBasis = %q, want %q under awsvpc without trunking",
				result.Signals.DensityBasis, recommend.DensityMaxENIsMinus1)
		}
		if _, ok := ranked(result, "m7i-thin.large"); ok {
			t.Error("m7i-thin.large (2 ENIs) is ranked under awsvpc; FR-21.6 requires >= 3")
		}
		c, ok := ranked(result, "c7i.large")
		if !ok {
			t.Fatalf("c7i.large is absent from ranked: %v", names(result.Ranked))
		}
		if c.TasksPerInstance != 2 {
			t.Errorf("c7i.large tasksPerInstance = %d under awsvpc, want 2 (maxENIs - 1)",
				c.TasksPerInstance)
		}
	})
}

func names(cands []recommend.Candidate) []string {
	out := make([]string, 0, len(cands))
	for _, c := range cands {
		out = append(out, c.InstanceType)
	}
	return out
}

// TestBlackBox_X2_ZeroConfigPoolIsConsistentAcrossPostures.
//
// Contradiction X-2: FR-28 fixes the zero-config pool at spot_with_base /
// min_size 1 / max_size 6 / target_capacity 100 regardless of posture, while
// FR-24 and AC-16 make target_capacity 80 and capacity_type on_demand under
// performance-first -- which is the DEFAULT posture. The two cannot both hold.
//
// Rather than pick a winner, this asserts what is not in dispute: the answer is
// the same whichever posture asked (a posture cannot change a pool derived from
// no demand at all), and every type in it exists in the region (FR-20, NFR-7).
// A failure here is AMBIGUOUS and should be escalated, not "fixed".
func TestBlackBox_X2_ZeroConfigPoolIsConsistentAcrossPostures(t *testing.T) {
	present := catalogByName(catA())

	var first recommend.SuggestedPool
	for i, posture := range postures() {
		result := recommend.Recommend(input(nil, posture))

		if result.Primary != nil {
			t.Errorf("%s: primary = %q, want null for an environment with no services",
				posture, result.Primary.InstanceType)
		}
		if result.Basis != recommend.BasisDefault {
			t.Errorf("%s: basis = %q, want %q (FR-28)", posture, result.Basis, recommend.BasisDefault)
		}
		if result.Unsatisfiable {
			t.Errorf("%s: unsatisfiable = true; an empty project must never be told its workload "+
				"cannot be satisfied", posture)
		}
		if len(result.NearestMisses) != 0 {
			t.Errorf("%s: nearestMisses = %+v, want none: nothing failed", posture, result.NearestMisses)
		}
		if len(result.SuggestedPool.InstanceTypes) == 0 {
			t.Errorf("%s: suggestedPool.instance_types is empty (FR-28, NFR-7)", posture)
		}
		for _, name := range result.SuggestedPool.InstanceTypes {
			if _, ok := present[name]; !ok {
				t.Errorf("%s: suggestedPool names %q, which this region's catalog does not carry; "+
					"the ASG could not launch it (NFR-7, FR-20)", posture, name)
			}
		}

		if i == 0 {
			first = result.SuggestedPool
			continue
		}
		if !reflect.DeepEqual(first, result.SuggestedPool) {
			t.Errorf("the zero-config pool differs by posture:\n  %s: %+v\n  %s: %+v\n"+
				"FR-28 fixes it regardless of posture; FR-24/AC-16 vary it. One of the two has to "+
				"give, and until it does this is AMBIGUOUS rather than a defect.",
				postures()[0], first, posture, result.SuggestedPool)
		}
	}
}

// TestBlackBox_X3_RatioRawIsTheBlendNotTheActual.
//
// Contradiction X-3: architecture 3.3's worked example prints
// signals.ratio.raw = 22.28, which is R_act; FR-17's formula with that
// example's own weights yields 0.55*4.0 + 0.45*22.28 = 12.226. FR-17 is
// asserted.
//
// If raw reported R_act instead of R_eff, the response would tell the user the
// clamp fired at a value it never saw, and every "why did it pick r-family?"
// investigation would start from a wrong number.
func TestBlackBox_X3_RatioRawIsTheBlendNotTheActual(t *testing.T) {
	// float64 variables, not untyped constants: 0.60*0.75 folded at compile
	// time is exactly 0.45, while the same product evaluated at runtime is
	// 0.44999999999999996. Comparing the two would test the Go compiler.
	var (
		rCfg     = 4.0
		rAct     = 22.28
		coverage = 0.75
		cap60    = 0.60
	)

	raw, wCfg, wAct := recommend.Blend(rCfg, rAct, coverage)

	if wAct != cap60*coverage {
		t.Errorf("weights.actual = %v, want 0.60 * coverage = %v (FR-17 caps the actuals at 60 %%)",
			wAct, cap60*coverage)
	}
	if wCfg+wAct != 1.0 {
		t.Errorf("weights sum to %v, want exactly 1.0", wCfg+wAct)
	}
	want := wCfg*rCfg + wAct*rAct
	if math.Abs(raw-want) > 1e-12 {
		t.Errorf("blended raw = %v, want %v (= %v). architecture 3.3's example prints %v, "+
			"which is R_act and not the blend.", raw, want, 12.226, rAct)
	}
	if math.Abs(raw-rAct) < 1e-9 {
		t.Errorf("blended raw = %v, which is R_act itself: the configured shape contributed "+
			"nothing despite carrying %v of the weight", raw, wCfg)
	}

	// End to end: signals.ratio.raw is that same blend, and the clamp reports
	// itself rather than firing silently (C-10).
	//
	// The utilisation figures sit just outside FR-18's burstable window
	// (cpuAvg < 20 AND cpuPeak < 60) on purpose. Inside it this fleet
	// classifies burstable, and the burstable class is a different scenario --
	// see TestBlackBox_BurstableClassMustNotRefuseASatisfiableWorkload.
	measured := envMem8()
	for i := range measured {
		measured[i].CPUPeak, measured[i].MemPeak = 60, 100
		measured[i].CPUAvg, measured[i].MemAvg = 25, 90
		measured[i].Datapoints = 336
	}
	result := recommend.Recommend(input(measured, recommend.PostureBalanced))
	assertEncodable(t, result, "extreme actual ratio")

	if result.Signals.Ratio.ClampedTo != recommend.ClampMax {
		t.Errorf("clampedTo = %q, want %q: R_act is far above CAT-A's 8.0 ceiling",
			result.Signals.Ratio.ClampedTo, recommend.ClampMax)
	}
	if result.Signals.Ratio.Effective != result.Signals.Ratio.CatalogMax {
		t.Errorf("effective = %v, want catalogMax = %v", result.Signals.Ratio.Effective,
			result.Signals.Ratio.CatalogMax)
	}
	if result.Signals.Ratio.Raw <= result.Signals.Ratio.CatalogMax {
		t.Errorf("raw = %v, which is inside the catalog range even though the clamp fired; "+
			"raw must be the pre-clamp blend", result.Signals.Ratio.Raw)
	}
	// The clamp exists so fit keeps discriminating: unclamped, every candidate
	// scores 0 and a 0.20-weighted term silently stops contributing.
	best := 0.0
	for _, c := range result.Ranked {
		best = math.Max(best, c.Scores.Fit)
	}
	if best < 0.99 {
		t.Errorf("the best fit sub-score is %v; after clamping to the catalog ceiling at least "+
			"one candidate must sit at the boundary with fit ~1.0", best)
	}
}

// TestBlackBox_BurstableClassMustNotRefuseASatisfiableWorkload.
//
// EC-11 defines `unsatisfiable` as "no available instance type satisfies the
// demand", and gives as its example a 200 GiB task in a region whose largest
// type is 128 GiB. It is a statement about capacity.
//
// The fixture below is six quiet tasks of 0.5 vCPU / 4 GiB. Eight
// current-generation, priced, x86 types in CAT-A can hold such a task with room
// to spare. But FR-18 classifies it `burstable` (max task <= 2 vCPU, cpuAvg
// < 20, cpuPeak < 60, coverage >= 0.5) and FR-21.8's rule R8 then admits only
// burstable types -- and the only one in the catalog, t3.medium, has 3.4 GiB
// usable against a 4 GiB task. Every candidate is excluded and the user is told
// their workload cannot be satisfied.
//
// The proof that this is a classification artefact rather than a capacity fact
// is the second half: performance-first suppresses the burstable class (FR-19)
// and the identical fleet then gets a full recommendation from the identical
// catalog.
//
// architecture.md C-15 records this as "the external reviewer's separate
// 'burstable unsatisfiable' concern" and states that making the class per-task
// "removes most of the practical force" behind it. It narrows it; it does not
// close it. A classification is a preference and must not be able to refuse a
// deployment that the region can host.
func TestBlackBox_BurstableClassMustNotRefuseASatisfiableWorkload(t *testing.T) {
	quiet := envMem8() // 6 tasks of 0.5 vCPU / 4 GiB
	for i := range quiet {
		quiet[i].CPUAvg, quiet[i].CPUPeak = 9.4, 12.0
		quiet[i].MemAvg, quiet[i].MemPeak = 61.2, 78.0
		quiet[i].Datapoints = 336
	}

	fallback := recommend.Recommend(input(quiet, recommend.PosturePerformance))
	if fallback.Primary == nil {
		t.Fatalf("performance-first found nothing either, so this fixture does not isolate the "+
			"classification: constraint = %q", fallback.Constraint)
	}

	result := recommend.Recommend(input(quiet, recommend.PostureBalanced))
	assertEncodable(t, result, "quiet memory-heavy fleet under balanced")

	// This asserts on classificationSuppressed and NOT on classification, and
	// the difference is the fix rather than an accommodation of it.
	//
	// After a capacity fallback the two fields deliberately hold different
	// values, and the response publishes both:
	//
	//	classification           what sizing actually ran under -- memory_heavy
	//	classificationSuppressed what FR-18 read off the utilisation but the
	//	                         region carries no type able to serve -- burstable
	//
	// classification has to name the class the ranking was filtered under,
	// because that is what makes FR-21.8 checkable: assertHardRules (line 340)
	// and recommend_test.go's TestRecommend_RankedObeysEveryHardRule both read
	// classification to decide which candidates are legal, and a field naming
	// a class the ranking did not use would leave them nothing to check. It is
	// also what performance-first has always reported for this same fleet,
	// where FR-19 suppresses the same class -- the two postures agreeing is
	// the whole point of the scenario below.
	//
	// So the fixture precondition moves to the field that now carries the
	// inference. It pins exactly what it always meant to pin -- "FR-18 reads
	// this fleet as burstable" -- and still fails loudly if that stops being
	// true. Asserting classification == burstable here could only hold while
	// a satisfiable workload was being refused, i.e. only while the bug was
	// present. Do not move it back.
	if result.ClassificationSuppressed != recommend.ClassBurstable {
		t.Fatalf("classificationSuppressed = %q, want burstable for this fixture (FR-18); the "+
			"scenario below only makes sense if FR-18 inferred burstable and the region could "+
			"not serve it. classification = %q.",
			result.ClassificationSuppressed, result.Classification)
	}
	if result.Unsatisfiable {
		t.Errorf("unsatisfiable = true with constraint %q, yet the SAME fleet against the SAME "+
			"catalog is served %q under performance-first, which differs only in suppressing the "+
			"burstable class (FR-19).\n"+
			"EC-11's unsatisfiable is a statement about capacity; here it is a statement about a "+
			"classification the user never chose, and the message points at a knob "+
			"(the burstable class) that no YAML field can change.",
			result.Constraint, fallback.Primary.InstanceType)
	}
	if result.Primary == nil {
		t.Error("primary is null for a workload eight catalog types can hold")
	}
}

// TestBlackBox_X4_EffectiveHourlyStaysBetweenSpotAndOnDemand.
//
// Contradiction X-4: FR-22's blend divides on_demand_base -- INSTANCES, per
// section 5.1 -- by tasksPerInstance, which counts TASKS. The quotient is not
// dimensionless and exceeds 1 whenever the base exceeds the density, at which
// point the spot term goes negative and effectiveHourly rises ABOVE the
// on-demand price. The recommender would then report spot as dearer than
// on-demand and rank it last.
//
// The invariant below holds under either reading of the formula, which is why
// it is the assertion rather than a particular number.
func TestBlackBox_X4_EffectiveHourlyStaysBetweenSpotAndOnDemand(t *testing.T) {
	byName := catalogByName(catA())

	type row struct {
		name         string
		services     []recommend.ServiceDemand
		capacityType string
		onDemandBase int
	}
	rows := []row{
		{"balanced default base of 1", envMem8(), "", 0},
		// The adversarial case: a base of 2 instances against candidates that
		// hold one task each.
		{"base 2 with one task per instance", []recommend.ServiceDemand{
			{Name: "api", VCPU: 2, MemGiB: 3.4, Count: 4},
		}, recommend.CapacitySpotWithBase, 2},
		{"base 8, far above any density", envCap1(), recommend.CapacitySpotWithBase, 8},
	}

	for _, r := range rows {
		for _, posture := range postures() {
			t.Run(r.name+"/"+string(posture), func(t *testing.T) {
				result := recommend.Recommend(recommend.Input{
					Region:   "ap-southeast-2",
					Catalog:  catA(),
					Services: r.services,
					Pool: recommend.PoolConstraints{
						Name:         "general",
						CapacityType: r.capacityType,
						OnDemandBase: r.onDemandBase,
					},
					Posture: posture,
					Limit:   50,
				})
				assertEncodable(t, result, r.name)

				for _, c := range result.Ranked {
					it := byName[c.InstanceType]
					od := *it.OnDemandHourly
					sp := od
					if it.SupportsSpot && it.SpotMedianHourly != nil && *it.SpotMedianHourly > 0 {
						sp = *it.SpotMedianHourly
					}
					lo, hi := math.Min(sp, od), od
					if c.EffectiveHourly <= 0 {
						t.Errorf("%s: effectiveHourly = %v; a price of zero or less cannot exist",
							c.InstanceType, c.EffectiveHourly)
					}
					if c.EffectiveHourly < lo-1e-12 || c.EffectiveHourly > hi+1e-12 {
						t.Errorf("%s: effectiveHourly = %v is outside [%v, %v]. A blended price "+
							"above the on-demand rate means the units in the blend disagree: "+
							"on_demand_base counts instances and tasksPerInstance counts tasks.",
							c.InstanceType, c.EffectiveHourly, lo, hi)
					}
					// costPerTask is a FLEET figure: what the pool this
					// candidate implies costs per hour, per task it runs.
					// It was effectiveHourly/tasksPerInstance, which priced
					// slots no task occupies and made an 8 vCPU instance the
					// better buy for a one-task fleet.
					if c.TasksPerInstance > 0 && result.Signals.ConfiguredTaskCount > 0 {
						n := math.Ceil(float64(result.Signals.ConfiguredTaskCount) / float64(c.TasksPerInstance))
						want := c.EffectiveHourly * n / float64(result.Signals.ConfiguredTaskCount)
						if math.Abs(c.CostPerTask-want) > 1e-12 {
							t.Errorf("%s: costPerTask = %v, want effectiveHourly*N/T = %v",
								c.InstanceType, c.CostPerTask, want)
						}
					}
				}
			})
		}
	}
}

// TestBlackBox_X5_CoverageIsDemandWeighted.
//
// Contradiction X-5: FR-17 says coverage is a "demand-weighted fraction"
// without naming the dimension (vCPU, memGiB, or vCPU x count). What is not
// ambiguous is that it must not be service-count-weighted: three trivial
// sidecars would then outvote the one service that actually consumes the
// cluster, and the recommendation would size for the sidecars.
//
// ENV-MIXED is one 4-vCPU service with a full window and three 0.25-vCPU
// sidecars with none. Count-weighting gives 0.25; vCPU-weighting gives
// 4.0/4.75 = 0.842.
func TestBlackBox_X5_CoverageIsDemandWeighted(t *testing.T) {
	result := recommend.Recommend(input(envMixed(), recommend.PostureBalanced))
	assertEncodable(t, result, "ENV-MIXED")

	cov := result.Signals.Coverage
	if math.Abs(cov-0.25) < 1e-9 {
		t.Errorf("coverage = %v, which is the service-COUNT fraction (1 of 4). FR-17 requires a "+
			"demand-weighted fraction; count-weighting lets three trivial sidecars outvote the "+
			"one service that consumes the cluster.", cov)
	}
	if want := 4.0 / 4.75; math.Abs(cov-want) > 1e-9 {
		t.Errorf("coverage = %v, want %v (the vCPU-weighted fraction). If the implementation "+
			"weights by a different demand dimension this is AMBIGUOUS under X-5 rather than a "+
			"defect -- but the weight below must still be consistent with whatever it reports.",
			cov, want)
	}
	if want := 0.60 * cov; math.Abs(result.Signals.WeightActual-want) > 1e-12 {
		t.Errorf("weights.actual = %v, want 0.60 * coverage = %v (FR-17)",
			result.Signals.WeightActual, want)
	}
	if want := 1 - result.Signals.WeightActual; math.Abs(result.Signals.WeightConfigured-want) > 1e-12 {
		t.Errorf("weights.configured = %v, want %v; the two must sum to 1",
			result.Signals.WeightConfigured, want)
	}
	if len(result.Signals.Services) != 4 {
		t.Errorf("signals.services has %d entries, want 4: FR-29 requires every in-scope service "+
			"to be named, including the silent ones", len(result.Signals.Services))
	}
	silent := 0
	for _, s := range result.Signals.Services {
		if s.Datapoints == 0 {
			silent++
			if s.Status != recommend.StatusNoData {
				t.Errorf("service %q has no datapoints but status %q", s.Name, s.Status)
			}
			if s.Name == "" {
				t.Error("a service with no data is reported without a name; the UI cannot say " +
					"which chart is empty")
			}
		}
	}
	if silent != 3 {
		t.Errorf("%d services reported no data, want 3", silent)
	}
}

// TestBlackBox_X6_ModernityIsRelativeToTheRegionsCatalog.
//
// Contradiction X-6: FR-22's modernity parses a generation digit, and whether
// m7i-flex is the same family as m7i is undefined -- which matters, because the
// zero-config default pool (FR-28) is built on m7i-flex.large. If -flex were a
// family of its own it would score 1.0 permanently, no matter how old it got.
//
// This pins the documented behaviour so a change to it has to be deliberate.
func TestBlackBox_X6_ModernityIsRelativeToTheRegionsCatalog(t *testing.T) {
	flexFamily, flexGen := recommend.ParseFamilyGeneration("m7i-flex.large")
	plainFamily, plainGen := recommend.ParseFamilyGeneration("m7i.large")
	if flexFamily != plainFamily || flexGen != plainGen {
		t.Errorf("m7i-flex.large parses as (%q, %d) and m7i.large as (%q, %d). If -flex is a "+
			"family of its own it is permanently the newest generation of itself and modernity "+
			"stops discriminating for the type the zero-config default is built on (X-6).",
			flexFamily, flexGen, plainFamily, plainGen)
	}

	result := recommend.Recommend(input(
		[]recommend.ServiceDemand{{Name: "api", VCPU: 1, MemGiB: mib(3072), Count: 4}},
		recommend.PostureBalanced))

	want := map[string]float64{
		"m7i.large": 1.0, // newest m in this catalog
		"m6i.large": 0.7, // one generation back
		"c7i.large": 1.0,
		"c6i.large": 0.7,
		"t3.medium": 0.0, // excluded by class, asserted below instead
	}
	for _, c := range result.Ranked {
		w, tracked := want[c.InstanceType]
		if !tracked || c.InstanceType == "t3.medium" {
			continue
		}
		if c.Scores.Modernity != w {
			t.Errorf("%s modernity = %v, want %v. Scoring 'newest absolutely' rather than "+
				"'newest present in this region' flattens a 0.15-weighted term to noise in "+
				"every region that lacks the latest generation.", c.InstanceType, c.Scores.Modernity, w)
		}
	}
	for _, c := range result.Ranked {
		if c.InstanceType == "m5.large" {
			t.Error("m5.large is scored at all; a previous-generation type is excluded before " +
				"scoring (FR-21.1)")
		}
	}
}

// TestBlackBox_X9_CostFirstMinSizeIsDerivedFromDemand.
//
// Contradiction X-9: FR-24 gives cost-first a literal min_size of 1; D-8
// (DEV-26) replaces it with a demand-derived floor. Decisions outrank
// requirements.
//
// With 20 tasks of 2 vCPU / 4 GiB, a pool with min_size 1 cannot start the
// workload: the tasks sit in PROVISIONING for the two to five minutes managed
// scaling needs to boot more instances, on every scale-to-min, and the failure
// reads as an AWS capacity problem rather than a recommender bug.
func TestBlackBox_X9_CostFirstMinSizeIsDerivedFromDemand(t *testing.T) {
	for _, posture := range postures() {
		t.Run(string(posture), func(t *testing.T) {
			result := recommend.Recommend(input(envCap1(), posture))
			assertEncodable(t, result, "ENV-CAP1/"+string(posture))

			if result.Primary == nil {
				t.Fatalf("no primary; constraint = %q", result.Constraint)
			}
			pool := result.SuggestedPool

			if pool.MinSize < result.Primary.InstancesAtFloor {
				t.Errorf("suggestedPool.min_size = %d but the primary needs %d instances at the "+
					"floor; the pool cannot start the configured workload",
					pool.MinSize, result.Primary.InstancesAtFloor)
			}
			if pool.MinSize <= 1 {
				t.Errorf("suggestedPool.min_size = %d against 20 tasks of 2 vCPU / 4 GiB. "+
					"FR-24's literal 1 was superseded by D-8/DEV-26 precisely because a pool that "+
					"holds a fraction of the fleet is 'correct' by construction and leaves the "+
					"rest in PROVISIONING.", pool.MinSize)
			}
			if pool.MaxSize < pool.MinSize {
				t.Errorf("suggestedPool.max_size = %d is below min_size = %d; AWS rejects the ASG "+
					"outright", pool.MaxSize, pool.MinSize)
			}
			if pool.MaxSize*result.Primary.TasksPerInstance < result.Signals.ConfiguredTaskCount {
				t.Errorf("suggestedPool.max_size = %d x %d tasks per instance cannot reach the %d "+
					"configured tasks; suggesting a ceiling that the pool's own rule would reject "+
					"is not a suggestion", pool.MaxSize, result.Primary.TasksPerInstance,
					result.Signals.ConfiguredTaskCount)
			}
		})
	}
}

// TestBlackBox_MaxSizeTooSmallIsAnExplainedRefusal is D-8's R9c: max_size is
// the one capacity number demand cannot raise. A pool that cannot reach the
// configured task count deploys successfully and then silently runs fewer tasks
// than asked for.
func TestBlackBox_MaxSizeTooSmallIsAnExplainedRefusal(t *testing.T) {
	result := recommend.Recommend(recommend.Input{
		Region:   "ap-southeast-2",
		Catalog:  catA(),
		Services: envCap1(), // 20 tasks of 2 vCPU / 4 GiB
		Pool:     recommend.PoolConstraints{Name: "general", MaxSize: ip(1)},
		Posture:  recommend.PostureBalanced,
		Limit:    50,
	})

	assertEncodable(t, result, "ENV-CAP1 with max_size 1")

	if len(result.Ranked) != 0 {
		t.Fatalf("candidates survived a max_size of 1 against 20 tasks: %v", names(result.Ranked))
	}
	if !result.Unsatisfiable {
		t.Error("unsatisfiable = false; EC-11 requires an explained refusal")
	}
	if !strings.Contains(result.Constraint, "20") {
		t.Errorf("constraint = %q; AC-20 requires the shortfall stated with both numbers",
			result.Constraint)
	}
	if len(result.NearestMisses) == 0 {
		t.Fatal("nearestMisses is empty; EC-11 requires the closest failures with the margin " +
			"quantified")
	}
	for _, m := range result.NearestMisses {
		if m.FailedRule != recommend.RuleMaxSizeTooSmall {
			t.Errorf("nearest miss %q failed rule %q, want %q",
				m.InstanceType, m.FailedRule, recommend.RuleMaxSizeTooSmall)
		}
	}
	// D-8 retired R9b as vacuous. The reason string it used must never come back:
	// it points the user at a knob that cannot fix anything.
	if strings.Contains(result.Constraint, "cannot_hold_peak") {
		t.Error("the retired cannot_hold_peak reason was emitted (D-8)")
	}
}

// TestBlackBox_NearestMissesAreNotPadded is EC-11 against a catalog that
// cannot supply three. AC-20 states the length as 3 unconditionally; a literal
// implementation either indexes out of range -- a 500 on an endpoint that must
// never fail -- or invents a type the user might act on.
func TestBlackBox_NearestMissesAreNotPadded(t *testing.T) {
	result := recommend.Recommend(recommend.Input{
		Region:   "ap-southeast-2",
		Catalog:  catASmall(), // two types
		Services: []recommend.ServiceDemand{{Name: "api", VCPU: 4, MemGiB: 200, Count: 1}},
		Pool:     recommend.PoolConstraints{Name: "general"},
		Posture:  recommend.PostureBalanced,
		Limit:    50,
	})

	assertEncodable(t, result, "ENV-HUGE against a two-type catalog")

	if !result.Unsatisfiable {
		t.Fatal("unsatisfiable = false for a 200 GiB task against 4 and 8 GiB instances")
	}
	if len(result.NearestMisses) != 2 {
		t.Errorf("nearestMisses has %d entries, want 2 -- the catalog holds two candidates and "+
			"the list must be neither padded nor truncated to a literal 3: %+v",
			len(result.NearestMisses), result.NearestMisses)
	}
	for _, m := range result.NearestMisses {
		if m.InstanceType == "" || m.FailedRule == "" {
			t.Errorf("nearest miss %+v is missing its identity or its rule", m)
		}
		if m.Needed <= 0 && m.Available <= 0 {
			t.Errorf("nearest miss %+v quantifies nothing; AC-20 requires the margin", m)
		}
	}
	if result.Primary != nil {
		t.Errorf("primary = %q, want null", result.Primary.InstanceType)
	}
}

// ---------------------------------------------------------------------------
// FR-17's inclusive datapoint threshold, and NFR-9's determinism
// ---------------------------------------------------------------------------

// TestBlackBox_CoverageThresholdIsInclusiveAt24 is FR-17's "at least 24
// datapoints". An environment a day old must count; one just under it must not
// have a day of noise treated as a fortnight of evidence.
func TestBlackBox_CoverageThresholdIsInclusiveAt24(t *testing.T) {
	withPoints := func(n int) recommend.Result {
		return recommend.Recommend(input([]recommend.ServiceDemand{{
			Name: "api", VCPU: 1, MemGiB: 4, Count: 4,
			CPUAvg: 30, CPUPeak: 55, MemAvg: 50, MemPeak: 70, Datapoints: n,
		}}, recommend.PostureBalanced))
	}

	at24 := withPoints(24)
	if at24.Signals.Coverage != 1.0 {
		t.Errorf("coverage at exactly 24 datapoints = %v, want 1.0 (FR-17's threshold is "+
			"inclusive)", at24.Signals.Coverage)
	}

	at23 := withPoints(23)
	if at23.Signals.Coverage != 0 {
		t.Errorf("coverage at 23 datapoints = %v, want 0", at23.Signals.Coverage)
	}
	if at23.Signals.WeightActual != 0 {
		t.Errorf("weights.actual = %v with nothing qualifying, want 0", at23.Signals.WeightActual)
	}
	if at23.Signals.Actual != nil {
		t.Errorf("signals.actual = %+v with nothing qualifying, want null", at23.Signals.Actual)
	}
	if at23.Signals.Services[0].Status == recommend.StatusOK {
		t.Error("a service with 23 datapoints reports status ok; it did not qualify")
	}
}

// TestBlackBox_TenIdenticalCallsAreByteIdentical is NFR-9 and AC-13. Without
// FR-25's final lexicographic tie-break, Go map iteration order picks the
// user's instance type and the same YAML recommends differently on each page
// load. signals.services is included because it is the field most likely to be
// built from a map, and a reorder there alone is enough to make the payload
// unstable.
func TestBlackBox_TenIdenticalCallsAreByteIdentical(t *testing.T) {
	for _, posture := range postures() {
		t.Run(string(posture), func(t *testing.T) {
			var first []byte
			for i := 0; i < 10; i++ {
				result := recommend.Recommend(input(envMem8Measured(), posture))
				body, err := json.Marshal(result)
				if err != nil {
					t.Fatalf("call %d does not encode: %v", i, err)
				}
				if i == 0 {
					first = body
					continue
				}
				if string(body) != string(first) {
					t.Fatalf("call %d differs from call 0:\n  first: %s\n  now:   %s", i, first, body)
				}
			}
		})
	}
}

// TestBlackBox_PostureWeightsSumToOne is FR-23. A mistyped weight changes every
// recommendation the tool will ever make and is invisible in any single-case
// eyeball check.
func TestBlackBox_PostureWeightsSumToOne(t *testing.T) {
	for _, posture := range postures() {
		w := recommend.Params(posture).Weights
		sum := w.Fit + w.Utilisation + w.Cost + w.Modernity
		if math.Abs(sum-1.0) > 1e-12 {
			t.Errorf("%s weights sum to %v, want exactly 1.0: %+v", posture, sum, w)
		}
	}
}

// TestBlackBox_SubScoresStayInsideTheUnitInterval is FR-22: each of the four
// dimensions is defined on [0,1]. An unclamped fit goes negative on a large
// mismatch and can drag a total below a strictly worse candidate's, inverting
// the ranking exactly where the recommendation matters most.
func TestBlackBox_SubScoresStayInsideTheUnitInterval(t *testing.T) {
	for _, env := range envFixtures() {
		for _, posture := range postures() {
			result := recommend.Recommend(input(env.services, posture))
			for _, c := range result.Ranked {
				for label, v := range map[string]float64{
					"fit":         c.Scores.Fit,
					"utilisation": c.Scores.Utilisation,
					"cost":        c.Scores.Cost,
					"modernity":   c.Scores.Modernity,
					"total":       c.Total,
				} {
					if v < 0 || v > 1 {
						t.Errorf("%s/%s: %s of %s = %v, outside [0,1]",
							env.name, posture, label, c.InstanceType, v)
					}
				}
			}
		}
	}
}
