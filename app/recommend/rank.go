package recommend

import (
	"math"
	"sort"
)

// FR-25's ordering over a quantized total, and the non-finite drop gate (C-4).

// RankItem is a scored candidate plus the facts the tie-breaks read but the
// API response does not carry.
type RankItem struct {
	Candidate
	Family       string
	Generation   int
	MaxENI       int
	DensityBasis string
	SpotUsable   bool
	NExact       float64
	// AchievedOccupancy is k_eff and Utilisation is the raw U the
	// Utilisation SUB-SCORE was derived from. They are carried rather than
	// recomputed so a test can assert the measurement separately from the
	// posture target applied to it.
	AchievedOccupancy float64
	Utilisation       float64
}

// bucketOf quantizes a total to FR-25's tie width and makes the epsilon part
// of the sort key.
//
// FR-25's "ties are total scores within 0.01" is not transitive --
// 0.800 / 0.795 / 0.790 makes A~B and B~C tied but A vs C not -- and sort.Slice
// is pdqsort, which requires a strict weak ordering. Given one that violates
// it, which tie-break fires depends on pivot choice. Quantizing first makes
// every tie-break fire on every comparison and the order total. The one
// behavioural difference from FR-25's literal wording is that two candidates in
// adjacent buckets are never "tied".
//
// math.Round is exact here and the value cannot exceed 100, but it still goes
// through floorToInt: this package has no bare int() conversions.
func bucketOf(total float64) int {
	if !isFinite(total) {
		return 0
	}
	return floorToInt(math.Round(total / tieWidth)) // constant divisor
}

// rankLess is the comparator: bucket desc, FIT desc, family lean, generation
// desc, maxENI desc, name asc.
//
// The second term was headroom desc, and headroom is gone. Its replacement is
// fit and NOT the utilisation sub-score, which is the obvious heir and is the
// wrong choice:
//
//   - Whatever sits at this level is double-counted, because it is already
//     inside Total. The only question is which double-count does least harm.
//   - Utilisation is the term most likely to differ inside one 0.01 bucket
//     (it is the widest-ranging sub-score), so putting it here effectively
//     re-runs the ranking on that one dimension for every near-tie.
//   - Measured on ENV-MEM8 -- six tasks at 8 GiB per vCPU -- utilisation as
//     the tie-break promoted m7i.xlarge (4 GiB/vCPU, fit 0.50) over
//     r7i.xlarge (8 GiB/vCPU, fit 1.00) inside a single bucket, because the
//     m-type's fill happened to land nearer performance-first's 0.70 target.
//     Half the memory per vCPU, chosen on a rounding-width tie.
//
// Fit is the defensible tie-break for the same reason it is the heaviest new
// weight: within a scoring tie, prefer the candidate whose SHAPE matches the
// workload, because a shape error strands a resource for the life of the pool
// while a fill error is recovered by the next scaling event.
//
// Every term is a total preorder on its own key and the terms are applied in a
// fixed order, so the composition is a strict weak ordering. The final term is
// a strict total order on a unique key, so incomparability means identity.
func rankLess(a, b RankItem, lean string) bool {
	if ab, bb := bucketOf(a.Total), bucketOf(b.Total); ab != bb {
		return ab > bb
	}
	if a.Scores.Fit != b.Scores.Fit {
		return a.Scores.Fit > b.Scores.Fit
	}
	switch lean {
	case LeanCompute:
		af, bf := a.Family == LeanCompute, b.Family == LeanCompute
		if af != bf {
			return af
		}
	case LeanCheapest:
		if a.EffectiveHourly != b.EffectiveHourly {
			return a.EffectiveHourly < b.EffectiveHourly
		}
	}
	if a.Generation != b.Generation {
		return a.Generation > b.Generation
	}
	if a.MaxENI != b.MaxENI {
		return a.MaxENI > b.MaxENI
	}
	return a.InstanceType < b.InstanceType
}

// Rank drops every candidate whose Total is not finite, then orders the rest.
//
// sort.SliceStable over [0.7, NaN, 0.5, NaN, 0.9] with a > comparator returns
// the slice UNSORTED, without panicking -- NaN comparisons are always false, so
// primary becomes whichever candidate happened to be first, total serialises as
// JSON null, and FR-25's tie-break never fires because "within 0.01" is also
// false. The guards in num.go are a claim; this gate is the mechanism.
// Production degrades to a correct ordering over the healthy candidates, and
// every fixture asserts len(dropped) == 0 so the drop is visible rather than
// silent.
//
// SliceStable is used at no cost, so that a future sixth tie-break added
// incorrectly degrades to stable rather than arbitrary.
func Rank(items []RankItem, lean string) ([]RankItem, []Miss) {
	kept := make([]RankItem, 0, len(items))
	var dropped []Miss
	for _, it := range items {
		if !isFinite(it.Total) {
			dropped = append(dropped, Miss{InstanceType: it.InstanceType, FailedRule: RuleNonFiniteScore})
			continue
		}
		kept = append(kept, it)
	}
	sort.SliceStable(kept, func(i, j int) bool { return rankLess(kept[i], kept[j], lean) })
	sort.SliceStable(dropped, func(i, j int) bool { return dropped[i].InstanceType < dropped[j].InstanceType })
	return kept, dropped
}
