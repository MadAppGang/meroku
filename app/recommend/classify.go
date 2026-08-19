package recommend

// Demand signals (FR-14, FR-16), the FR-17 blend, the C-10 catalog-range clamp
// on R_eff, and FR-18/19 classification with C-15's per-task burstable test.

// Demand is the configured shape: what YAML asks ECS to reserve.
//
// Every quantity here is derived from services that survived the guard --
// VCPU > 0, MemGiB > 0, Count > 0, all finite. compute_signals.go has already
// dropped the rest and told the user which field was wrong; this is the second
// belt (C-9).
type Demand struct {
	VCPU          float64 // ConfiguredVCPU = sum(vcpu_i * count_i)
	MemGiB        float64 // ConfiguredMemGiB = sum(memGiB_i * count_i)
	TaskCount     int     // T = sum(count_i)
	MaxTaskVCPU   float64 // the largest single task -- a task must fit on one instance
	MaxTaskMemGiB float64
	VCPUPerTask   float64 // demand-weighted mean = ConfiguredVCPU / T
	MemGiBPerTask float64
	Ratio         float64 // R_cfg = ConfiguredMemGiB / ConfiguredVCPU, GiB per vCPU

	// LargestTask* identify ONE service -- the fattest reservation in the
	// fleet -- rather than a per-dimension maximum. EC-11's refusal reads "no
	// instance type in {region} can hold {service} ({cpu}/{mem})", and it
	// names a service because the numbers beside it have to be findable: the
	// demand-weighted mean is a figure that appears in no config file and in
	// no signals field, so a user told their tasks need 67.33 GiB searches
	// their YAML for 67.33 and finds nothing.
	//
	// The fattest task is the one that binds, and memory is the dimension the
	// 0.85 reserve makes bind first, so memory picks it. VCPU and MemGiB below
	// are that service's OWN pair, never two different services' maxima.
	LargestTaskName   string
	LargestTaskVCPU   float64
	LargestTaskMemGiB float64
}

// Actual is the CloudWatch shape: what the fleet measurably consumes.
type Actual struct {
	VCPU   float64 // ActualVCPU = sum(vcpu_i * count_i * cpuPeak_i/100)
	MemGiB float64
	Ratio  float64 // R_act
	// Demand-weighted aggregates over the covered services, for FR-18's
	// burstable test.
	CPUAvg, CPUPeak, MemAvg, MemPeak float64
	// OK is false when ActualVCPU <= 0 -- every covered service's cpuPeak
	// rounded to 0.0, e.g. an idle JVM with datapoints >= 24. R_act is then
	// never computed, coverage is forced to 0 and signals.actual is null.
	OK bool
}

// usableService reports whether a service carries real demand. It is the guard
// behind every division in this file.
func usableService(s ServiceDemand) bool {
	return s.VCPU > 0 && s.MemGiB > 0 && s.Count > 0 &&
		isFinite(s.VCPU) && isFinite(s.MemGiB)
}

// covered reports whether a service has enough CloudWatch history to count
// toward FR-17's coverage: 24 hourly datapoints, one full day.
func covered(s ServiceDemand) bool {
	return usableService(s) && s.Datapoints >= coveredDatapoints
}

// ConfiguredShape computes FR-14's demand vector from YAML alone.
func ConfiguredShape(services []ServiceDemand) Demand {
	var d Demand
	for _, s := range services {
		if !usableService(s) {
			continue
		}
		n := float64(s.Count)
		d.VCPU += s.VCPU * n
		d.MemGiB += s.MemGiB * n
		d.TaskCount += s.Count
		d.MaxTaskVCPU = maxf(d.MaxTaskVCPU, s.VCPU)
		d.MaxTaskMemGiB = maxf(d.MaxTaskMemGiB, s.MemGiB)
		// Strictly greater, so the first service wins a tie and the identity
		// does not depend on input order beyond what the caller controls.
		if s.MemGiB > d.LargestTaskMemGiB ||
			(s.MemGiB == d.LargestTaskMemGiB && s.VCPU > d.LargestTaskVCPU) {
			d.LargestTaskName = s.Name
			d.LargestTaskVCPU = s.VCPU
			d.LargestTaskMemGiB = s.MemGiB
		}
	}
	if d.TaskCount > 0 {
		if v, ok := ratio(d.VCPU, float64(d.TaskCount)); ok {
			d.VCPUPerTask = v
		}
		if m, ok := ratio(d.MemGiB, float64(d.TaskCount)); ok {
			d.MemGiBPerTask = m
		}
	}
	// Division 1 of note 5: the divisor is zero only when every in-scope task
	// has cpu <= 0, and those services never reach this line.
	if r, ok := ratio(d.MemGiB, d.VCPU); ok {
		d.Ratio = r
	}
	return d
}

// Coverage is FR-17's demand-weighted fraction of in-scope services with at
// least 24 datapoints. The weight is the service's share of configured vCPU,
// because vCPU is R_cfg's denominator.
func Coverage(services []ServiceDemand) float64 {
	var total, withData float64
	for _, s := range services {
		if !usableService(s) {
			continue
		}
		w := s.VCPU * float64(s.Count)
		total += w
		if covered(s) {
			withData += w
		}
	}
	c, ok := ratio(withData, total)
	if !ok {
		return 0
	}
	return clamp(c, 0, 1)
}

// ActualShape computes FR-16's measured consumption over the covered services.
//
// Division 2 of note 5: ActualVCPU is zero when cpuPeak rounds to 0.0 for every
// covered service. R_act is then never computed and OK is false.
func ActualShape(services []ServiceDemand) Actual {
	var a Actual
	var cpuW, memW float64
	for _, s := range services {
		if !covered(s) {
			continue
		}
		n := float64(s.Count)
		cw := s.VCPU * n
		mw := s.MemGiB * n
		a.VCPU += cw * s.CPUPeak / 100.0   // constant divisor
		a.MemGiB += mw * s.MemPeak / 100.0 // constant divisor
		a.CPUAvg += cw * s.CPUAvg
		a.CPUPeak += cw * s.CPUPeak
		a.MemAvg += mw * s.MemAvg
		a.MemPeak += mw * s.MemPeak
		cpuW += cw
		memW += mw
	}
	if v, ok := ratio(a.CPUAvg, cpuW); ok {
		a.CPUAvg = v
	} else {
		a.CPUAvg = 0
	}
	if v, ok := ratio(a.CPUPeak, cpuW); ok {
		a.CPUPeak = v
	} else {
		a.CPUPeak = 0
	}
	if v, ok := ratio(a.MemAvg, memW); ok {
		a.MemAvg = v
	} else {
		a.MemAvg = 0
	}
	if v, ok := ratio(a.MemPeak, memW); ok {
		a.MemPeak = v
	} else {
		a.MemPeak = 0
	}

	if !(a.VCPU > 0) || !isFinite(a.VCPU) || !isFinite(a.MemGiB) {
		return Actual{OK: false}
	}
	r, ok := ratio(a.MemGiB, a.VCPU)
	if !ok {
		return Actual{OK: false}
	}
	a.Ratio = r
	a.OK = true
	return a
}

// PeakDemand is note 4's PeakVCPU / PeakMemGiB: the fleet's measured peak
// consumption, falling back to configured demand when coverage is zero.
//
// Division 8 of note 5 guards on the result rather than the input: a peak that
// comes out at or below zero is replaced by the configured figure, which is
// > 0 because T > 0 and every surviving service has vcpu > 0.
func PeakDemand(services []ServiceDemand, d Demand, coverage float64) (float64, float64) {
	if !(coverage > 0) {
		return d.VCPU, d.MemGiB
	}
	var pv, pm float64
	for _, s := range services {
		if !covered(s) {
			continue
		}
		n := float64(s.Count)
		pv += s.VCPU * n * s.CPUPeak / 100.0   // constant divisor
		pm += s.MemGiB * n * s.MemPeak / 100.0 // constant divisor
	}
	if !(pv > 0) || !isFinite(pv) {
		pv = d.VCPU
	}
	if !(pm > 0) || !isFinite(pm) {
		pm = d.MemGiB
	}
	return pv, pm
}

// Blend is FR-17: the two ratio signals combined with the actuals capped at
// 60 % weight. It returns R_raw before the C-10 catalog clamp, plus both
// weights for Signals.
//
// coverage == 0 yields wAct == 0 and rRaw == rCfg EXACTLY (FR-27) -- not
// approximately, which is why the zero case short-circuits instead of
// multiplying by 0.0.
func Blend(rCfg, rAct, coverage float64) (rRaw, wCfg, wAct float64) {
	c := clamp(coverage, 0, 1)
	if c == 0 || !isFinite(rAct) {
		return rCfg, 1, 0
	}
	wAct = actualWeightCap * c
	wCfg = 1 - wAct
	rRaw = wCfg*rCfg + wAct*rAct
	if !isFinite(rRaw) {
		return rCfg, 1, 0
	}
	return rRaw, wCfg, wAct
}

// ClampRatio is C-10: R_eff is confined to the range of ratios that can
// actually be purchased.
//
// For uniform tasks R_act = R_cfg * (memPeak / cpuPeak), so a low CPU peak
// inflates R_eff without bound. Past R_eff >= 32 with R_cfg = 4,
// abs(ln(instanceRatio / R_eff)) / ln 4 >= 1 for EVERY instance on the market:
// every candidate scores fit = 0, fit stops discriminating, and the recommender
// silently degrades to waste + cost + headroom + modernity while still claiming
// a shape argument. Clamping to the observed range guarantees at least one
// candidate sits at or beyond a boundary with fit near 1.0.
func ClampRatio(rRaw, catalogMin, catalogMax float64) (float64, string) {
	if !(catalogMin > 0) || !(catalogMax > 0) || catalogMin > catalogMax {
		return rRaw, ClampNone
	}
	switch {
	case rRaw < catalogMin:
		return catalogMin, ClampMin
	case rRaw > catalogMax:
		return catalogMax, ClampMax
	default:
		return rRaw, ClampNone
	}
}

// ClassifyInput carries exactly the quantities FR-18 reads.
type ClassifyInput struct {
	REff        float64
	MaxTaskVCPU float64 // C-15: the largest SINGLE task, not the fleet aggregate
	CPUAvg      float64
	CPUPeak     float64
	Coverage    float64
	Posture     Posture
	GPU         bool // gpu=true requested, or the pool's ami_family is a GPU family
}

// Classify is FR-18, evaluated in order, with FR-19's suppression and C-15's
// per-task burstable test.
//
// FR-18's burstable row tested ConfiguredVCPU <= 2, and ConfiguredVCPU is the
// FR-14 fleet aggregate. Six tasks of 0.5 vCPU total 3.0, so the textbook
// burstable workload was excluded and FR-21.8 then actively removed t3/t4g from
// every realistic recommendation. The condition is MaxTaskVCPU <= 2.
func Classify(in ClassifyInput) string {
	if in.GPU {
		return ClassGPU
	}
	burstable := in.MaxTaskVCPU > 0 && in.MaxTaskVCPU <= 2 &&
		in.CPUAvg < 20 && in.CPUPeak < 60 && in.Coverage >= 0.5
	// FR-19: burstable CPU credits make steady-state throughput
	// unpredictable, which contradicts performance-first.
	if burstable && in.Posture != PosturePerformance {
		return ClassBurstable
	}
	return ratioClass(in.REff)
}

// ratioClass is FR-18's shape rows on their own, with no utilisation and no
// posture: the classification the workload has by virtue of what it reserves.
//
// It is separate from Classify because it is what performance-first already
// falls back to when FR-19 suppresses burstable, and what recommend.go falls
// back to when the region carries no burstable type that can hold the
// workload. Both are the same fallback and must not be able to drift apart.
func ratioClass(rEff float64) string {
	switch {
	case rEff >= 6.0:
		return ClassMemoryHeavy
	case rEff >= 3.0:
		return ClassBalanced
	default:
		return ClassCPUHeavy
	}
}
