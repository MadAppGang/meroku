package recommend

import "math"

// Saturating numeric helpers -- the ONLY float->int conversions and the only
// variable-divisor divisions in this package (architecture.md 4.5 note 1, C-9;
// note 5, C-4).
//
// Go leaves int(x) implementation-defined when x is NaN or outside the target
// type's range. Executed, not reasoned about: on darwin/arm64 int(math.Inf(1))
// is 9223372036854775807, while on linux/amd64 the CVTTSD2SI instruction
// returns the integer-indefinite value -9223372036854775808. A min() over the
// two picks the negative one, tasksPerInstance goes hugely negative and every
// quantity derived from it inverts -- so the recommender would answer
// differently on a developer's Mac and on CI. NFR-9 forbids exactly that.
//
// A bare int(x) anywhere else in app/recommend is a review rejection.

const (
	// maxTaskSlots bounds tasksPerInstance. It sits above any purchasable
	// density: 192 vCPU / 0.25 vCPU-per-task = 768.
	maxTaskSlots = 1024
	// maxInstances bounds instance counts. It sits above any ASG max_size
	// this product will ever render.
	maxInstances = 4096
)

// floorToInt is total: every float64 maps to a value in [0, maxTaskSlots].
// NaN maps to 0 -- "no slots", which rule R9a then excludes -- rather than to
// whatever the hardware happens to produce.
func floorToInt(f float64) int {
	switch {
	case math.IsNaN(f):
		return 0
	case f <= 0:
		return 0
	case f >= float64(maxTaskSlots):
		return maxTaskSlots
	default:
		return int(math.Floor(f))
	}
}

// ceilToInt is total: every float64 maps to a value in [0, maxInstances].
func ceilToInt(f float64) int {
	switch {
	case math.IsNaN(f):
		return 0
	case f <= 0:
		return 0
	case f >= float64(maxInstances):
		return maxInstances
	default:
		return int(math.Ceil(f))
	}
}

// clamp confines v to [lo, hi]. NaN clamps to lo, because every caller uses
// the low bound as its "no information" value.
func clamp(v, lo, hi float64) float64 {
	switch {
	case math.IsNaN(v):
		return lo
	case v < lo:
		return lo
	case v > hi:
		return hi
	default:
		return v
	}
}

// ratio is division with the divisor-zero case answered once, here, instead of
// ten times across the package. It reports ok=false rather than returning
// +Inf/NaN, so a caller must decide what a missing quotient means -- the
// decision architecture.md 4.5 note 5 tabulates for all ten divisions.
func ratio(num, den float64) (float64, bool) {
	if den == 0 || math.IsNaN(den) || math.IsInf(den, 0) {
		return 0, false
	}
	if math.IsNaN(num) || math.IsInf(num, 0) {
		return 0, false
	}
	q := num / den
	if math.IsNaN(q) || math.IsInf(q, 0) {
		return 0, false
	}
	return q, true
}

// minf and maxf are NaN-safe: a NaN operand loses, so a single bad input
// cannot propagate through a chain of mins into the ranking.
func minf(a, b float64) float64 {
	switch {
	case math.IsNaN(a):
		return b
	case math.IsNaN(b):
		return a
	case a < b:
		return a
	default:
		return b
	}
}

func maxf(a, b float64) float64 {
	switch {
	case math.IsNaN(a):
		return b
	case math.IsNaN(b):
		return a
	case a > b:
		return a
	default:
		return b
	}
}

// isFinite reports whether f is a real number this package may keep.
func isFinite(f float64) bool {
	return !math.IsNaN(f) && !math.IsInf(f, 0)
}
