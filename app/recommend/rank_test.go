package recommend

import (
	"math"
	"testing"
)

// rankItem builds a RankItem for the comparator tests. The float after maxENI
// is the second tie-break key. That key was headroom; it is now FIT, for the
// reasons in rankLess's doc comment. The comparator LEVEL is unchanged -- one
// sub-score between the score bucket and the family lean -- so every ordering
// case below still exercises the same five levels.
func rankItem(name, family string, gen, maxENI int, total, fit, hourly float64) RankItem {
	return RankItem{
		Candidate: Candidate{
			InstanceType:    name,
			Total:           total,
			EffectiveHourly: hourly,
			Scores:          SubScores{Fit: fit},
		},
		Family:     family,
		Generation: gen,
		MaxENI:     maxENI,
	}
}

// TestRank_TieBreakOrder walks FR-25's five levels in one 0.01 bucket. Every
// total below quantizes to bucket 80, so nothing is decided by the raw score.
func TestRank_TieBreakOrder(t *testing.T) {
	items := []RankItem{
		// deliberately shuffled
		rankItem("m5.large", "m", 4, 2, 0.7999, 0.50, 0.10),
		rankItem("m3.large", "m", 8, 1, 0.7960, 0.50, 0.10),
		rankItem("a9.large", "m", 4, 2, 0.8000, 0.50, 0.10),
		rankItem("c2.large", "c", 1, 1, 0.8049, 0.50, 0.10),
		rankItem("m4.large", "m", 4, 9, 0.8040, 0.50, 0.10),
		rankItem("m1.large", "m", 9, 9, 0.7951, 0.90, 0.10),
	}
	for _, it := range items {
		if got := bucketOf(it.Total); got != 80 {
			t.Fatalf("fixture drift: %s quantizes to bucket %d, want 80", it.InstanceType, got)
		}
	}

	ranked, dropped := Rank(items, LeanCompute)
	if len(dropped) != 0 {
		t.Fatalf("dropped %d finite candidates", len(dropped))
	}
	want := []string{
		"m1.large", // 1. higher fit
		"c2.large", // 2. the posture's family lean
		"m3.large", // 3. newer generation
		"m4.large", // 4. higher maximumNetworkInterfaces
		"a9.large", // 5. lexicographically smaller
		"m5.large",
	}
	for i, w := range want {
		if ranked[i].InstanceType != w {
			names := make([]string, len(ranked))
			for j, r := range ranked {
				names[j] = r.InstanceType
			}
			t.Fatalf("order = %v, want %v", names, want)
		}
	}
}

// TestRank_CheapestLean is the cost-first half of FR-24's family lean.
func TestRank_CheapestLean(t *testing.T) {
	items := []RankItem{
		rankItem("b.large", "m", 7, 3, 0.80, 0.5, 0.20),
		rankItem("a.large", "m", 7, 3, 0.80, 0.5, 0.30),
		rankItem("c.large", "m", 7, 3, 0.80, 0.5, 0.10),
	}
	ranked, _ := Rank(items, LeanCheapest)
	want := []string{"c.large", "b.large", "a.large"}
	for i, w := range want {
		if ranked[i].InstanceType != w {
			t.Fatalf("position %d = %s, want %s", i, ranked[i].InstanceType, w)
		}
	}
	// With no lean, the same three fall through to the lexicographic rule.
	ranked, _ = Rank(items, LeanNone)
	want = []string{"a.large", "b.large", "c.large"}
	for i, w := range want {
		if ranked[i].InstanceType != w {
			t.Fatalf("position %d = %s, want %s", i, ranked[i].InstanceType, w)
		}
	}
}

// twelveCandidates spreads totals, fits, families, generations and ENI
// counts so that every tie-break level is exercised by some triple.
func twelveCandidates() []RankItem {
	names := []string{"a1.large", "b2.large", "c3.large", "d4.large", "e5.large", "f6.large",
		"g7.large", "h8.large", "i9.large", "j1.large", "k2.large", "l3.large"}
	families := []string{"c", "m", "r", "c", "m", "r", "c", "m", "r", "c", "m", "r"}
	totals := []float64{0.800, 0.7999, 0.795, 0.7951, 0.60, 0.60, 0.601, 0.42, 0.42, 0.42, 0.9, 0.9}
	fits := []float64{0.5, 0.5, 0.5, 0.9, 0.2, 0.2, 0.2, 1.0, 1.0, 0.0, 0.3, 0.3}
	gens := []int{7, 7, 6, 7, 5, 5, 5, 8, 7, 7, 6, 6}
	enis := []int{3, 4, 3, 3, 8, 2, 2, 4, 4, 4, 15, 15}
	out := make([]RankItem, 0, len(names))
	for i := range names {
		out = append(out, rankItem(names[i], families[i], gens[i], enis[i], totals[i], fits[i], 0.1+0.01*float64(i%5)))
	}
	return out
}

// TestRank_ComparatorIsATotalOrder is DEV-10. sort.Slice is pdqsort, which
// requires a strict weak ordering; given one that violates it, which tie-break
// fires depends on pivot choice.
func TestRank_ComparatorIsATotalOrder(t *testing.T) {
	items := twelveCandidates()
	for _, lean := range []string{LeanCompute, LeanNone, LeanCheapest} {
		less := func(a, b RankItem) bool { return rankLess(a, b, lean) }

		for _, a := range items {
			if less(a, a) {
				t.Errorf("%s: less(a,a) is true -- the relation is not irreflexive", lean)
			}
			for _, b := range items {
				if less(a, b) && less(b, a) {
					t.Errorf("%s: less(%s,%s) and less(%s,%s) both true -- not asymmetric",
						lean, a.InstanceType, b.InstanceType, b.InstanceType, a.InstanceType)
				}
				for _, c := range items {
					if less(a, b) && less(b, c) && !less(a, c) {
						t.Errorf("%s: transitivity fails on (%s,%s,%s)",
							lean, a.InstanceType, b.InstanceType, c.InstanceType)
					}
					// Incomparability must be transitive too, which is the
					// property FR-25's literal "within 0.01" wording breaks
					// and the quantized bucket restores.
					eqAB := !less(a, b) && !less(b, a)
					eqBC := !less(b, c) && !less(c, b)
					eqAC := !less(a, c) && !less(c, a)
					if eqAB && eqBC && !eqAC {
						t.Errorf("%s: incomparability is not transitive on (%s,%s,%s)",
							lean, a.InstanceType, b.InstanceType, c.InstanceType)
					}
				}
			}
		}
	}
}

// TestRank_DropsNonFinite is C-4's gate. sort.SliceStable over
// [0.7, NaN, 0.5, NaN, 0.9] with a > comparator returns the slice UNSORTED,
// without panicking, so a NaN total silently decides the primary.
func TestRank_DropsNonFinite(t *testing.T) {
	items := []RankItem{
		rankItem("good.large", "m", 7, 3, 0.7, 0.5, 0.1),
		rankItem("nan.large", "m", 7, 3, math.NaN(), 0.5, 0.1),
		rankItem("posinf.large", "m", 7, 3, math.Inf(1), 0.5, 0.1),
		rankItem("neginf.large", "m", 7, 3, math.Inf(-1), 0.5, 0.1),
		rankItem("better.large", "m", 7, 3, 0.9, 0.5, 0.1),
	}
	ranked, dropped := Rank(items, LeanNone)

	if len(ranked) != 2 {
		t.Fatalf("ranked %d candidates, want 2", len(ranked))
	}
	if ranked[0].InstanceType != "better.large" || ranked[1].InstanceType != "good.large" {
		t.Errorf("ranked = [%s %s], want [better.large good.large]",
			ranked[0].InstanceType, ranked[1].InstanceType)
	}
	if len(dropped) != 3 {
		t.Fatalf("dropped %d candidates, want 3: %+v", len(dropped), dropped)
	}
	for _, d := range dropped {
		if d.FailedRule != RuleNonFiniteScore {
			t.Errorf("dropped %s with rule %q, want %q", d.InstanceType, d.FailedRule, RuleNonFiniteScore)
		}
	}
	// Deterministic even in the failure path.
	if dropped[0].InstanceType != "nan.large" || dropped[2].InstanceType != "posinf.large" {
		t.Errorf("dropped order = %+v, want it sorted by name", dropped)
	}
}

// TestRank_NaNCannotSurviveIntoPrimary is the executed half of note 5's
// observation: a comparator alone does not sort NaN out.
func TestRank_NaNCannotSurviveIntoPrimary(t *testing.T) {
	items := []RankItem{
		rankItem("first.large", "m", 7, 3, 0.7, 0.5, 0.1),
		rankItem("nan.large", "m", 7, 3, math.NaN(), 0.9, 0.1),
		rankItem("third.large", "m", 7, 3, 0.5, 0.5, 0.1),
		rankItem("nan2.large", "m", 7, 3, math.NaN(), 0.9, 0.1),
		rankItem("best.large", "m", 7, 3, 0.9, 0.5, 0.1),
	}
	ranked, _ := Rank(items, LeanNone)
	if ranked[0].InstanceType != "best.large" {
		t.Errorf("primary = %s, want best.large", ranked[0].InstanceType)
	}
	for _, r := range ranked {
		if !isFinite(r.Total) {
			t.Errorf("%s reached the ranking with total %v", r.InstanceType, r.Total)
		}
	}
}

func TestRank_BucketQuantization(t *testing.T) {
	cases := []struct {
		total float64
		want  int
	}{
		{0.9110, 91},
		{0.9149, 91},
		{0.9150, 92},
		{0.0, 0},
		{1.0, 100},
		{math.NaN(), 0},
		{math.Inf(1), 0},
	}
	for _, tc := range cases {
		if got := bucketOf(tc.total); got != tc.want {
			t.Errorf("bucketOf(%v) = %d, want %d", tc.total, got, tc.want)
		}
	}
}
