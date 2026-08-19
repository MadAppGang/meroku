package recommend

import (
	"math"
	"testing"
)

// TestNum_Saturates is C-9's regression test. int(+Inf) is 9223372036854775807
// on darwin/arm64 and -9223372036854775808 on linux/amd64; either would make
// tasksPerInstance meaningless and the recommender architecture-dependent.
func TestNum_Saturates(t *testing.T) {
	cases := []struct {
		name      string
		in        float64
		wantFloor int
		wantCeil  int
	}{
		{"positive infinity", math.Inf(1), maxTaskSlots, maxInstances},
		{"negative infinity", math.Inf(-1), 0, 0},
		{"NaN", math.NaN(), 0, 0},
		{"zero", 0, 0, 0},
		{"negative one", -1, 0, 0},
		{"1e300", 1e300, maxTaskSlots, maxInstances},
		{"one point nine", 1.9, 1, 2},
		{"exactly two", 2.0, 2, 2},
		{"just under the floor cap", float64(maxTaskSlots) - 0.5, maxTaskSlots - 1, maxTaskSlots},
		{"exactly the ceil cap", float64(maxInstances), maxTaskSlots, maxInstances},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := floorToInt(tc.in); got != tc.wantFloor {
				t.Errorf("floorToInt(%v) = %d, want %d", tc.in, got, tc.wantFloor)
			}
			if got := ceilToInt(tc.in); got != tc.wantCeil {
				t.Errorf("ceilToInt(%v) = %d, want %d", tc.in, got, tc.wantCeil)
			}
			if got := floorToInt(tc.in); got < 0 || got > maxTaskSlots {
				t.Errorf("floorToInt(%v) = %d escaped [0,%d]", tc.in, got, maxTaskSlots)
			}
			if got := ceilToInt(tc.in); got < 0 || got > maxInstances {
				t.Errorf("ceilToInt(%v) = %d escaped [0,%d]", tc.in, got, maxInstances)
			}
		})
	}
}

func TestNum_ClampIsTotal(t *testing.T) {
	cases := []struct {
		name            string
		v, lo, hi, want float64
	}{
		{"inside", 0.5, 0, 1, 0.5},
		{"below", -3, 0, 1, 0},
		{"above", 9, 0, 1, 1},
		{"NaN clamps to lo", math.NaN(), 0, 1, 0},
		{"positive infinity clamps to hi", math.Inf(1), 0, 1, 1},
		{"negative infinity clamps to lo", math.Inf(-1), 0, 1, 0},
		{"at lo", 0, 0, 1, 0},
		{"at hi", 1, 0, 1, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := clamp(tc.v, tc.lo, tc.hi); got != tc.want {
				t.Errorf("clamp(%v,%v,%v) = %v, want %v", tc.v, tc.lo, tc.hi, got, tc.want)
			}
		})
	}
}

// TestNum_RatioRefusesDegenerateDivisors pins the guard that stands behind all
// ten divisions of note 5: a missing quotient is reported, never invented.
func TestNum_RatioRefusesDegenerateDivisors(t *testing.T) {
	cases := []struct {
		name     string
		num, den float64
		want     float64
		wantOK   bool
	}{
		{"ordinary", 12, 4, 3, true},
		{"zero divisor", 12, 0, 0, false},
		{"zero over zero", 0, 0, 0, false},
		{"NaN divisor", 12, math.NaN(), 0, false},
		{"NaN numerator", math.NaN(), 4, 0, false},
		{"infinite divisor", 12, math.Inf(1), 0, false},
		{"infinite numerator", math.Inf(1), 4, 0, false},
		{"overflowing quotient", 1e308, 1e-10, 0, false},
		{"negative divisor", 12, -4, -3, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ratio(tc.num, tc.den)
			if ok != tc.wantOK || got != tc.want {
				t.Errorf("ratio(%v,%v) = (%v,%v), want (%v,%v)", tc.num, tc.den, got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

func TestNum_MinMaxAreNaNSafe(t *testing.T) {
	nan := math.NaN()
	if got := minf(nan, 2); got != 2 {
		t.Errorf("minf(NaN,2) = %v, want 2", got)
	}
	if got := minf(2, nan); got != 2 {
		t.Errorf("minf(2,NaN) = %v, want 2", got)
	}
	if got := maxf(nan, 2); got != 2 {
		t.Errorf("maxf(NaN,2) = %v, want 2", got)
	}
	if got := maxf(2, nan); got != 2 {
		t.Errorf("maxf(2,NaN) = %v, want 2", got)
	}
	if got := minf(nan, nan); !math.IsNaN(got) {
		t.Errorf("minf(NaN,NaN) = %v, want NaN", got)
	}
}
