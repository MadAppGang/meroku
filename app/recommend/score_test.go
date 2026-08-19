package recommend

import (
	"math"
	"testing"
)

// TestScore_EffectiveHourlyBlend is C-5/DEV-25.
//
// FR-22's blend divided INSTANCES by TASKS and produced weights outside [0,1]:
// with b = 2, k = 1, p_od = 0.10 and p_sp = 0.04 it returned 0.16, i.e. 60 %
// above the on-demand price for a pool that is at most half on-demand. The
// corrected blend weighs instances against instances.
func TestScore_EffectiveHourlyBlend(t *testing.T) {
	const (
		pOD = 0.10
		pSP = 0.04
		T   = 6
	)

	cases := []struct {
		name         string
		capacityType string
		base         int
		tasks        int
		wantN        int
		want         float64
	}{
		{"on_demand pays the on-demand price", CapacityOnDemand, 0, 1, 6, pOD},
		{"spot pays the spot median", CapacitySpot, 0, 1, 6, pSP},
		{"spot_with_base at k=1 blends 2 of 6 instances", CapacitySpotWithBase, 2, 1, 6, 0.060},
		{"spot_with_base at k=3 blends 2 of 2 instances", CapacitySpotWithBase, 2, 3, 2, 0.100},
		{"a base larger than the fleet cannot exceed it", CapacitySpotWithBase, 10, 3, 2, pOD},
		{"a zero base is pure spot", CapacitySpotWithBase, 0, 1, 6, pSP},
		{"a negative base is treated as zero", CapacitySpotWithBase, -4, 1, 6, pSP},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			n := FleetInstances(T, tc.tasks)
			if n != tc.wantN {
				t.Fatalf("N = %d, want %d", n, tc.wantN)
			}
			got := EffectiveHourly(pOD, pSP, tc.capacityType, tc.base, n)
			if !nearlyEqual(got, tc.want, 1e-9) {
				t.Errorf("effectiveHourly = %v, want %v", got, tc.want)
			}
			// Note the direction, which is the opposite of the old formula's:
			// at higher density fewer instances run, so a fixed on-demand base
			// covers a LARGER fraction of the fleet and the price rises toward
			// on-demand.
			if got < minf(pOD, pSP)-1e-12 || got > maxf(pOD, pSP)+1e-12 {
				t.Errorf("effectiveHourly %v is outside [%v,%v] -- not a price that can exist", got, pSP, pOD)
			}
		})
	}

	t.Run("a null spot median falls back to on-demand", func(t *testing.T) {
		// EC-6: the caller substitutes p_od for a missing median before the
		// blend, so every capacity type collapses to the on-demand price.
		for _, capType := range []string{CapacityOnDemand, CapacitySpot, CapacitySpotWithBase} {
			got := EffectiveHourly(pOD, pOD, capType, 1, FleetInstances(T, 3))
			if !nearlyEqual(got, pOD, 1e-12) {
				t.Errorf("%s with no spot median = %v, want %v", capType, got, pOD)
			}
		}
	})

	t.Run("the weights stay in [0,1] over a sweep", func(t *testing.T) {
		prices := [][2]float64{{0.10, 0.04}, {0.05, 0.0499}, {1.0, 0.25}, {0.10, 0.10}}
		for _, taskCount := range []int{1, 2, 6, 17, 400} {
			for _, k := range []int{1, 2, 3, 8, 64} {
				for _, b := range []int{0, 1, 2, 10, 1000} {
					for _, capType := range []string{CapacityOnDemand, CapacitySpot, CapacitySpotWithBase} {
						for _, p := range prices {
							n := FleetInstances(taskCount, k)
							eff := EffectiveHourly(p[0], p[1], capType, b, n)
							lo, hi := minf(p[0], p[1]), maxf(p[0], p[1])
							if eff < lo-1e-12 || eff > hi+1e-12 {
								t.Fatalf("T=%d k=%d b=%d %s p=%v: eff %v outside [%v,%v]",
									taskCount, k, b, capType, p, eff, lo, hi)
							}
							if p[0] == p[1] {
								continue
							}
							// eff = w_od*p_od + (1-w_od)*p_sp, so w_od is
							// recoverable and must be a real weight.
							wOD := (eff - p[1]) / (p[0] - p[1])
							if wOD < -1e-9 || wOD > 1+1e-9 {
								t.Fatalf("T=%d k=%d b=%d %s: w_od = %v, outside [0,1]",
									taskCount, k, b, capType, wOD)
							}
						}
					}
				}
			}
		}
	})
}

// TestScore_Utilisation replaces TestScore_Headroom, which tested a function
// that no longer exists.
//
// WHY THE OLD TEST WENT RATHER THAN BEING ADAPTED. It pinned three worked rows
// of note 4's h -- 1.25, 2.125, 1.307 -- and a fourth sub-test executed D-8's
// proof that h >= 1 for every candidate. All four asserted properties OF
// headroomScore. The pair it belonged to was removed for the reasons in
// score.go, so there is nothing left for those rows to describe. D-8's
// conclusion is preserved where it still applies: TestEligible_NoFloor-
// Constraint in filter_test.go still asserts that no rule reads MinSize, and
// R9b is still absent from the rule table.
//
// What replaces it is a test of the property the new term exists to have:
// achieved occupancy, and a score that peaks at the posture's target.
func TestScore_Utilisation(t *testing.T) {
	d := ConfiguredShape(uniformServices())
	if !nearlyEqual(d.VCPU, 3.0, eps) || !nearlyEqual(d.MemGiB, 12.0, eps) {
		t.Fatalf("fixture drift: configured = (%v,%v), want (3.0,12.0)", d.VCPU, d.MemGiB)
	}

	t.Run("achieved occupancy is T/N, not the packing capacity", func(t *testing.T) {
		cases := []struct {
			name      string
			taskCount int
			capacity  int
			wantN     int
			wantKEff  float64
		}{
			{"k divides T exactly", 6, 3, 2, 3},
			{"k above T wastes the surplus", 6, 163, 1, 6},
			{"k above T, moderately", 6, 13, 1, 6},
			{"k does not divide T", 6, 4, 2, 3},
			{"a single task", 1, 40, 1, 1},
			{"no tasks", 0, 3, 0, 0},
			{"no capacity", 6, 0, 0, 0},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				if got := FleetInstances(tc.taskCount, tc.capacity); got != tc.wantN {
					t.Errorf("FleetInstances = %d, want %d", got, tc.wantN)
				}
				got := achievedOccupancy(tc.taskCount, tc.capacity)
				if !nearlyEqual(got, tc.wantKEff, 1e-12) {
					t.Errorf("achievedOccupancy = %v, want %v", got, tc.wantKEff)
				}
			})
		}
	})

	// The defect the achieved measurement exists to close: an instance far
	// larger than the fleet used to score as though it were full.
	t.Run("an oversized instance is not rated as though it were full", func(t *testing.T) {
		const vt, mt = 0.5, 2.0
		small := achievedUtilisation(achievedOccupancy(6, 3), vt, mt, 2, UsableMemGiB(8192))
		huge := achievedUtilisation(achievedOccupancy(6, 163), vt, mt, 96, UsableMemGiB(393216))
		if !(small > 0.75) {
			t.Errorf("m7i.large-shaped utilisation = %v, want it well filled", small)
		}
		if !(huge < 0.10) {
			t.Errorf("24xlarge-shaped utilisation = %v, want it near empty for a six-task fleet", huge)
		}
		if !(small > huge) {
			t.Errorf("the near-empty instance (%v) outscored the full one (%v)", huge, small)
		}
	})

	t.Run("the score peaks at the target and falls off both sides", func(t *testing.T) {
		const target = 0.70
		cases := []struct{ u, want float64 }{
			{0.00, 0.0},
			{0.35, 0.5},
			{0.70, 1.0},
			{0.85, 0.5},
			{1.00, 0.0},
		}
		for _, tc := range cases {
			if got := utilisationScore(tc.u, target); !nearlyEqual(got, tc.want, 1e-12) {
				t.Errorf("utilisationScore(%v, %v) = %v, want %v", tc.u, target, got, tc.want)
			}
		}
	})

	t.Run("a target of 1.0 makes the score the achieved utilisation itself", func(t *testing.T) {
		for _, u := range []float64{0, 0.25, 0.5, 0.816, 1.0} {
			if got := utilisationScore(u, 1.0); !nearlyEqual(got, u, 1e-12) {
				t.Errorf("utilisationScore(%v, 1.0) = %v, want %v", u, got, u)
			}
		}
	})

	t.Run("the overshoot slope is steeper the nearer the target sits to full", func(t *testing.T) {
		// Not a tuned asymmetry: each side is normalised by its own distance
		// to the bound, so 1/target below and 1/(1-target) above.
		lowUnder := 1 - utilisationScore(0.60-0.10, 0.60)
		lowOver := 1 - utilisationScore(0.60+0.10, 0.60)
		if !(lowOver > lowUnder) {
			t.Errorf("at target 0.60 the overshoot penalty %v is not above the undershoot penalty %v",
				lowOver, lowUnder)
		}
		if !nearlyEqual(lowUnder, 0.10/0.60, 1e-12) || !nearlyEqual(lowOver, 0.10/0.40, 1e-12) {
			t.Errorf("slopes = (%v,%v), want (1/0.60, 1/0.40) scaled by 0.10", lowUnder, lowOver)
		}
	})

	t.Run("it never escapes the unit interval", func(t *testing.T) {
		for _, target := range []float64{-1, 0, 0.7, 0.85, 1.0} {
			for _, u := range []float64{-5, 0, 0.5, 1, 12, math.Inf(1), math.NaN()} {
				got := utilisationScore(u, target)
				if got < 0 || got > 1 || !isFinite(got) {
					t.Errorf("utilisationScore(%v,%v) = %v, outside [0,1]", u, target, got)
				}
			}
		}
	})

	t.Run("it does not go dead on a fleet with no CloudWatch history", func(t *testing.T) {
		// The precise defect finding N-6 recorded for headroom: with no
		// coverage, PeakDemand falls back to configured demand, which made h
		// exactly 1.0 -- and headroom exactly 0.0 -- for EVERY candidate, so
		// 0.40 of performance-first's table ranked nothing on the default path.
		in := baseInput()
		in.Posture = PosturePerformance
		in.Limit = 20
		res := Recommend(in)
		if res.Signals.Coverage != 0 {
			t.Fatalf("fixture drift: coverage = %v, want 0", res.Signals.Coverage)
		}
		seen := map[float64]bool{}
		for _, c := range res.Ranked {
			seen[c.Scores.Utilisation] = true
		}
		if len(seen) < 2 {
			t.Errorf("utilisation took %d distinct values over %d candidates on an idle fleet; "+
				"a dimension that cannot discriminate is dead weight", len(seen), len(res.Ranked))
		}
	})
}

// TestParams_CostFirstMinSizeIsDerived is DEV-26, the root fix for C-8.
//
// FR-24 gave cost-first a literal min_size of 1 while both other postures
// derived it from demand. A pool holding 2 of 6 tasks was then "correct" by
// construction, and four tasks sat in PROVISIONING for the 2-5 minutes managed
// scaling needs to boot more, on every scale-to-min.
func TestParams_CostFirstMinSizeIsDerived(t *testing.T) {
	if got, want := Params(PostureCost).MinSizeBasis, Params(PostureBalanced).MinSizeBasis; got != want {
		t.Errorf("cost-first min_size basis = %q, want balanced's %q", got, want)
	}

	in := baseInput()
	in.Posture = PostureCost
	cost := Recommend(in)
	in.Posture = PostureBalanced
	balanced := Recommend(in)

	if cost.SuggestedPool.MinSize <= 1 {
		t.Errorf("cost-first min_size = %d; demand needs %d instances of %s",
			cost.SuggestedPool.MinSize, cost.Primary.InstancesAtFloor, cost.Primary.InstanceType)
	}
	if cost.SuggestedPool.MinSize != cost.Primary.InstancesAtFloor {
		t.Errorf("cost-first min_size = %d, want the primary's n_floor %d",
			cost.SuggestedPool.MinSize, cost.Primary.InstancesAtFloor)
	}
	if balanced.SuggestedPool.MinSize != balanced.Primary.InstancesAtFloor {
		t.Errorf("balanced min_size = %d, want the primary's n_floor %d",
			balanced.SuggestedPool.MinSize, balanced.Primary.InstancesAtFloor)
	}
	if cost.SuggestedPool.MinSize != balanced.SuggestedPool.MinSize {
		t.Errorf("cost-first min_size %d != balanced's %d for the same input",
			cost.SuggestedPool.MinSize, balanced.SuggestedPool.MinSize)
	}
	// A min_size that cannot hold the fleet is the failure DEV-26 removes.
	if held := cost.SuggestedPool.MinSize * cost.Primary.TasksPerInstance; held < cost.Signals.ConfiguredTaskCount {
		t.Errorf("cost-first min_size holds %d of %d configured tasks", held, cost.Signals.ConfiguredTaskCount)
	}

	// FR-24's other cost-first differences survive DEV-26.
	if got := Params(PostureCost).MaxSizeFactor; got != 2 {
		t.Errorf("cost-first max_size factor = %v, want 2", got)
	}
	if got := Params(PostureCost).TargetCapacity; got != 100 {
		t.Errorf("cost-first target_capacity = %d, want 100", got)
	}
	if got := Params(PostureCost).CapacityType; got != CapacitySpot {
		t.Errorf("cost-first capacity_type = %q, want %q", got, CapacitySpot)
	}
}

func TestParams_WeightsSumToOne(t *testing.T) {
	for _, p := range []Posture{PosturePerformance, PostureBalanced, PostureCost} {
		w := Params(p).Weights
		sum := w.Fit + w.Utilisation + w.Cost + w.Modernity
		if !nearlyEqual(sum, 1.0, 1e-12) {
			t.Errorf("%s weights sum to %v, want 1.0", p, sum)
		}
	}
}

func TestScore_FitIsZeroAtAFourfoldMismatch(t *testing.T) {
	cases := []struct {
		name                string
		instanceRatio, rEff float64
		want                float64
	}{
		{"exact match", 4, 4, 1},
		{"four times too rich", 16, 4, 0},
		{"four times too lean", 1, 4, 0},
		{"twice too rich", 8, 4, 0.5},
		{"eight times off is still zero, never negative", 32, 4, 0},
		{"a zero R_eff cannot divide", 4, 0, 0},
		{"a zero instance ratio cannot log", 0, 4, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := fitScore(tc.instanceRatio, tc.rEff)
			if !nearlyEqual(got, tc.want, 1e-9) {
				t.Errorf("fitScore(%v,%v) = %v, want %v", tc.instanceRatio, tc.rEff, got, tc.want)
			}
			if got < 0 || got > 1 {
				t.Errorf("fit %v escaped [0,1]", got)
			}
		})
	}
}

func TestScore_Modernity(t *testing.T) {
	newest := map[string]int{"m": 7, "r": 7, "t": 4}
	cases := []struct {
		family string
		gen    int
		want   float64
	}{
		{"m", 7, 1.0},
		{"m", 6, 0.7},
		{"m", 5, 0.4},
		{"m", 4, 0.0},
		{"t", 4, 1.0},
		{"m", 0, 0.0},   // unparseable generation
		{"zzz", 3, 0.0}, // family absent from the catalog
	}
	for _, tc := range cases {
		if got := modernityScore(tc.family, tc.gen, newest); !nearlyEqual(got, tc.want, 1e-12) {
			t.Errorf("modernityScore(%q,%d) = %v, want %v", tc.family, tc.gen, got, tc.want)
		}
	}
}

// TestScore_AchievedUtilisationNeverEscapesTheUnitInterval is the old
// TestScore_WasteNeverEscapesTheUnitInterval, retargeted at the function that
// replaced wasteScore. The rows are the same shapes; the first argument is now
// k_eff (achieved) rather than k (capacity), which is the entire change.
func TestScore_AchievedUtilisationNeverEscapesTheUnitInterval(t *testing.T) {
	cases := []struct {
		name                    string
		kEff                    float64
		vcpuPerTask, memPerTask float64
		vcpu                    int
		usableMem               float64
		want                    float64
	}{
		{"m7i.large holding three tasks", 3, 0.5, 2, 2, 6.8, (0.75 + 6.0/6.8) / 2},
		{"a perfect pack", 4, 0.5, 2, 2, 8, 1.0},
		{"a fractional share, k_eff = 2.5", 2.5, 0.5, 2, 2, 6.8, (0.625 + 5.0/6.8) / 2},
		{"zero vcpu cannot divide", 3, 0.5, 2, 0, 6.8, 0},
		{"zero usable memory cannot divide", 3, 0.5, 2, 2, 0, 0},
		{"an over-full dimension is clamped, never lent to the other", 100, 0.5, 2, 2, 6.8, 1.0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := achievedUtilisation(tc.kEff, tc.vcpuPerTask, tc.memPerTask, tc.vcpu, tc.usableMem)
			if !nearlyEqual(got, tc.want, 1e-9) {
				t.Errorf("achievedUtilisation = %v, want %v", got, tc.want)
			}
			if got < 0 || got > 1 {
				t.Errorf("utilisation %v escaped [0,1]", got)
			}
		})
	}
}

func TestScore_TotalIsBoundedByBadSubScores(t *testing.T) {
	// Sub-scores are clamped before the weighted sum, so a single bad one
	// cannot make Total unbounded.
	w := Params(PostureBalanced).Weights
	bad := SubScores{Fit: math.Inf(1), Utilisation: -5, Cost: 12, Modernity: 1}
	got := totalScore(bad, w)
	if got < 0 || got > 1 {
		t.Errorf("totalScore over out-of-range sub-scores = %v, want it inside [0,1]", got)
	}
	if !isFinite(got) {
		t.Errorf("totalScore = %v, want a finite value", got)
	}
}
