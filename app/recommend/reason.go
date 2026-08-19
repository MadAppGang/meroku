package recommend

import (
	"math"
	"strconv"
	"strings"
)

// FR-30's one-sentence reason, including the C-10 clamp note and the EC-6
// spot-downgrade note.
//
// Explain may not state a number that is absent from Signals. That is not a
// style rule: a reason quoting a figure the UI cannot show beside it is a
// figure nobody can check. The clamp bound, the spot downgrade and the density
// basis are all in Signals, so all three are sayable.

// fmt1 formats a ratio-like quantity to one decimal.
func fmt1(v float64) string {
	if !isFinite(v) {
		return "0.0"
	}
	return strconv.FormatFloat(v, 'f', 1, 64)
}

// fmtPct formats a utilisation percentage to a whole number.
func fmtPct(v float64) string {
	if !isFinite(v) {
		return "0"
	}
	return strconv.FormatFloat(math.Round(v), 'f', 0, 64)
}

// classDisplay is the plain-English name of a classification. None of these
// contain a digit, so none of them can be mistaken for a cited number.
func classDisplay(class string) string {
	switch class {
	case ClassMemoryHeavy:
		return "memory-heavy"
	case ClassCPUHeavy:
		return "cpu-heavy"
	case ClassGPU:
		return "GPU"
	case ClassBurstable:
		return "burstable"
	case ClassBalanced:
		return "balanced"
	default:
		return "unclassified"
	}
}

// measuredPeaks returns the highest memory peak and the highest CPU average
// any in-scope service reported. Both are values present in Signals.Services,
// which is what keeps Explain citable.
func measuredPeaks(services []ServiceSignal) (memPeak float64, cpuAvg float64, ok bool) {
	for _, s := range services {
		if s.Datapoints <= 0 {
			continue
		}
		memPeak = maxf(memPeak, s.MemPeak)
		cpuAvg = maxf(cpuAvg, s.CPUAvg)
		ok = true
	}
	return memPeak, cpuAvg, ok
}

// Explain writes the FR-30 sentence for one candidate.
//
// class is the class the answer was built under. inferredClass is non-empty
// only when the two differ -- when FR-18 inferred a class the region could not
// serve and sizing fell back -- and it is stated, because a user watching a
// low-CPU fleet be sized as memory-heavy has no other way to learn that the
// tool did read the utilisation and did try burstable first.
func Explain(c Candidate, runnerUp string, class string, inferredClass string, s Signals, downgraded bool) string {
	var b strings.Builder

	b.WriteString(classDisplay(class))
	b.WriteString(" — your services request ")
	b.WriteString(fmt1(s.Configured.Ratio))
	b.WriteString(" GiB per vCPU")

	memPeak, cpuAvg, haveMeasured := measuredPeaks(s.Services)
	if s.Actual != nil && haveMeasured {
		b.WriteString(" and measured memory peaks at ")
		b.WriteString(fmtPct(memPeak))
		b.WriteString("% while CPU averages ")
		b.WriteString(fmtPct(cpuAvg))
		b.WriteString("%")
	} else {
		b.WriteString(" and nothing has reported utilisation yet")
	}

	b.WriteString(", so ")
	b.WriteString(c.InstanceType)
	if runnerUp != "" {
		b.WriteString(" fits better than ")
		b.WriteString(runnerUp)
		b.WriteString(" and costs less than over-provisioning compute.")
	} else {
		b.WriteString(" is the best fit this region's catalog offers.")
	}

	if inferredClass != "" && inferredClass != class {
		b.WriteString(" Utilisation reads as ")
		b.WriteString(classDisplay(inferredClass))
		b.WriteString(", but no ")
		b.WriteString(classDisplay(inferredClass))
		b.WriteString(" type this region offers can hold a task this size, so the pool is sized as ")
		b.WriteString(classDisplay(class))
		b.WriteString(" instead.")
	}

	if s.Ratio.ClampedTo == ClampMax {
		b.WriteString(" Utilisation implies a shape no instance family provides (")
		b.WriteString(fmt1(s.Ratio.Raw))
		b.WriteString(" GiB per vCPU); capped at ")
		b.WriteString(fmt1(s.Ratio.CatalogMax))
		b.WriteString(".")
	} else if s.Ratio.ClampedTo == ClampMin {
		b.WriteString(" Utilisation implies a shape no instance family provides (")
		b.WriteString(fmt1(s.Ratio.Raw))
		b.WriteString(" GiB per vCPU); raised to ")
		b.WriteString(fmt1(s.Ratio.CatalogMin))
		b.WriteString(".")
	}

	if downgraded {
		b.WriteString(" Spot has no published median for this type, so the pool falls back to on-demand.")
	}

	return b.String()
}
