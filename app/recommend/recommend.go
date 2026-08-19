package recommend

import (
	"fmt"
	"sort"
)

// The pipeline of architecture.md 4.4, in the order it fixes -- because two of
// the review findings were ordering bugs.
//
//	1. Demand        ConfiguredShape / ActualShape / Coverage; T == 0 stops here
//	2. PreFilter     demand-INDEPENDENT hard rules, over the whole catalog
//	3. Ratio range   catalogMin/catalogMax over the PreFilter survivors
//	4. Blend+clamp   R_raw from FR-17, R_eff clamped into the range (C-10)
//	5. Classify      from R_eff, per-task vCPU (C-15), utilisation, posture
//	6. Class filters the demand-dependent hard rules, R8 R9a R6 R2 R9c
//	7. Score         the five sub-scores
//	8. Rank          quantized total, FR-25 tie-breaks, non-finite drop gate
//	9. SuggestedPool FR-24/26/28, network_mode and a DERIVED ami_family
//
// The clamp needs a candidate set, the candidate set needs a classification,
// and the classification needs the clamp. Steps 2-5 break that circle.

// defaultLimit is FR-13's default number of ranked results.
const defaultLimit = 5

// nearestMissCount is EC-11's fixed nearest-miss list length.
const nearestMissCount = 3

// NormalizePosture maps an unset or unrecognised posture to FR-13's default.
// The HTTP layer rejects an unrecognised value with a 400 naming the three
// legal ones; the core simply cannot be handed one that changes its answer
// non-deterministically.
func NormalizePosture(p Posture) Posture {
	switch p {
	case PostureBalanced, PostureCost, PosturePerformance:
		return p
	default:
		return PosturePerformance
	}
}

// NormalizePool fills the pool defaults. AMIFamily defaults to "al2023" and
// NOT to "" (C-11): "" let the architecture filter accept every architecture
// while variables.tf and the template both rendered an x86-only AMI.
// NetworkMode defaults to "bridge" (D-6).
func NormalizePool(p PoolConstraints) PoolConstraints {
	switch p.NetworkMode {
	case NetworkModeBridge, NetworkModeAWSVPC:
	default:
		p.NetworkMode = NetworkModeBridge
	}
	switch p.AMIFamily {
	case AMIFamilyAL2023, AMIFamilyAL2023ARM64, AMIFamilyAL2023GPU:
	default:
		p.AMIFamily = AMIFamilyAL2023
	}
	return p
}

// normalizeCatalog copies the catalog and fills Family/Generation from the
// type name when the caller left them empty. It copies rather than mutates so
// that two calls with the same Input cannot observe each other -- purity is
// what makes NFR-9 testable.
func normalizeCatalog(catalog []InstanceType) []InstanceType {
	out := make([]InstanceType, len(catalog))
	copy(out, catalog)
	for i := range out {
		if out[i].Family == "" || out[i].Generation == 0 {
			family, gen := ParseFamilyGeneration(out[i].Name)
			if out[i].Family == "" {
				out[i].Family = family
			}
			if out[i].Generation == 0 {
				out[i].Generation = gen
			}
		}
	}
	return out
}

// newestGenerations is FR-22's modernity baseline: the newest generation the
// region's catalog carries for each family.
func newestGenerations(catalog []InstanceType) map[string]int {
	newest := make(map[string]int, len(catalog))
	for _, it := range catalog {
		if it.Generation > newest[it.Family] {
			newest[it.Family] = it.Generation
		}
	}
	return newest
}

// catalogRatioRange is pipeline step 3: the range of ratios that can actually
// be purchased. Rule R0 has removed every vcpu == 0 record, so both bounds are
// finite and > 0 whenever there is at least one survivor.
func catalogRatioRange(survivors []InstanceType) (float64, float64) {
	var lo, hi float64
	for _, it := range survivors {
		r, ok := ratio(float64(it.MemoryMiB)/1024.0, float64(it.VCPU)) // constant divisor
		if !ok || !(r > 0) {
			continue
		}
		if lo == 0 || r < lo {
			lo = r
		}
		if r > hi {
			hi = r
		}
	}
	return lo, hi
}

// serviceSignals is the per-service half of FR-29.
//
// degenerate is note 5's division-2 case: every covered service reported a
// cpuPeak of 0.0, so ActualShape refused to divide. Their statuses become
// no_data, because a measurement that cannot produce a shape is not a
// measurement.
func serviceSignals(services []ServiceDemand, degenerate bool) []ServiceSignal {
	out := make([]ServiceSignal, 0, len(services))
	for _, s := range services {
		// These four are the only numbers in the whole Result that are copied
		// from the input straight into the answer without arithmetic, and they
		// are CloudWatch percentages of RESERVED capacity that the boundary
		// fills from GetMetricStatistics. Every other path out of
		// ServiceDemand runs through ratio() or isFinite() and cannot carry a
		// NaN; this one could, and a single NaN here aborts the encode of the
		// entire response (NFR-6). clamp is NaN-safe and answers lo, and a
		// percentage outside [0,100] is a bad datapoint rather than a
		// measurement.
		sig := ServiceSignal{
			Name:       s.Name,
			Datapoints: s.Datapoints,
			CPUAvg:     clamp(s.CPUAvg, 0, 100),
			CPUPeak:    clamp(s.CPUPeak, 0, 100),
			MemAvg:     clamp(s.MemAvg, 0, 100),
			MemPeak:    clamp(s.MemPeak, 0, 100),
		}
		switch {
		case !usableService(s):
			// C-9: the boundary already told the user which field was wrong.
			// The core reports the service, never a demand figure for it.
			sig.Status = StatusNoData
		case degenerate:
			sig.Status = StatusNoData
		case s.Datapoints >= coveredDatapoints:
			sig.Status = StatusOK
		case s.Datapoints > 0:
			sig.Status = StatusPartial
		default:
			sig.Status = StatusNoData
		}
		out = append(out, sig)
	}
	return out
}

// rulePriority orders the failed rules for two deterministic decisions: which
// rule the unsatisfiable constraint names, and how nearest misses break ties.
// It runs from "most likely to be the real reason" to least.
var rulePriority = []string{
	RuleUnpriced,
	RuleTaskFitMemory,
	RuleTaskFitVCPU,
	RuleMaxSizeTooSmall,
	RuleENIDensity,
	RuleArchitecture,
	RuleGPU,
	RuleBurstableClass,
	RuleZeroDensity,
	RuleGeneration,
	RuleBareMetal,
	RuleMalformedRecord,
	RuleFamilyNotEligible,
	RuleNonFiniteScore,
}

func rulePriorityIndex(rule string) int {
	for i, r := range rulePriority {
		if r == rule {
			return i
		}
	}
	return len(rulePriority)
}

// scopeRule reports whether a rule describes what this TOOL will consider
// rather than a fact about the region, the catalog record or the pool.
//
// R10 is the only such rule. Every other rule names something the user can
// look at: a price AWS did not publish, an architecture their ami_family
// cannot boot, a max_size they set, a task they reserved. R10 names a decision
// this recommender made about its own scope, and on a real 903-type region it
// fires for roughly two thirds of the catalog -- so without this carve-out it
// would win dominantRule's count on almost every unsatisfiable answer and
// bury the constraint the user could actually act on. A fleet with one 500 GiB
// task would be told "no eligible family in ap-southeast-2" instead of "no
// instance type can hold a single task of 500.0 GiB".
//
// It is not dropped from the miss list: it still reaches Signals and the
// nearest-miss ranking, so a user asking where a type went can still see that
// it was skipped for its family. It is only barred from SPEAKING FOR the
// refusal while a rule about the workload is available.
func scopeRule(rule string) bool { return rule == RuleFamilyNotEligible }

// dominantRule picks the rule that excluded the most candidates, breaking ties
// by rulePriority so the answer never depends on map iteration or input order.
//
// Scope rules are considered only when nothing else fired, per scopeRule.
func dominantRule(misses []Miss) string {
	counts := make(map[string]int, len(misses))
	for _, m := range misses {
		counts[m.FailedRule]++
	}
	for _, skipScope := range []bool{true, false} {
		best, bestCount, bestPri := "", 0, len(rulePriority)+1
		for rule, n := range counts {
			if skipScope && scopeRule(rule) {
				continue
			}
			pri := rulePriorityIndex(rule)
			if n > bestCount || (n == bestCount && pri < bestPri) {
				best, bestCount, bestPri = rule, n, pri
			}
		}
		if best != "" {
			return best
		}
	}
	return ""
}

// bestAvailable is the largest Available any miss of this rule reported -- the
// closest the catalog came to satisfying it.
func bestAvailable(misses []Miss, rule string) (float64, bool) {
	var best float64
	found := false
	for _, m := range misses {
		if m.FailedRule != rule {
			continue
		}
		if !found || m.Available > best {
			best, found = m.Available, true
		}
	}
	return best, found
}

// constraintMessage names the shortfall with numbers (AC-20). Every figure it
// prints comes from a Miss or from the demand vector.
func constraintMessage(region string, misses []Miss, d Demand, pool PoolConstraints, gpuRequested bool) string {
	where := region
	if where == "" {
		where = "this region"
	}
	rule := dominantRule(misses)
	switch rule {
	case RuleUnpriced:
		return fmt.Sprintf("no instance type in %s has a price this tool could read", where)
	case RuleTaskFitMemory:
		best, _ := bestAvailable(misses, rule)
		return fmt.Sprintf("no instance type in %s can hold a single task of %.1f GiB; the largest usable memory on offer is %.1f GiB",
			where, d.MaxTaskMemGiB, best)
	case RuleTaskFitVCPU:
		best, _ := bestAvailable(misses, rule)
		return fmt.Sprintf("no instance type in %s can hold a single task of %.1f vCPU; the largest on offer is %.0f vCPU",
			where, d.MaxTaskVCPU, best)
	case RuleMaxSizeTooSmall:
		best, _ := bestAvailable(misses, rule)
		size := 0
		if pool.MaxSize != nil {
			size = *pool.MaxSize
		}
		return fmt.Sprintf("the pool's max_size of %d instances holds at most %.0f tasks, and %d are configured",
			size, best, d.TaskCount)
	case RuleENIDensity:
		best, _ := bestAvailable(misses, rule)
		return fmt.Sprintf("no instance type in %s carries enough network interfaces for awsvpc task networking; the best on offer has %.0f",
			where, best)
	case RuleArchitecture:
		return fmt.Sprintf("no instance type in %s matches ami_family %q", where, pool.AMIFamily)
	case RuleGPU:
		if gpuRequested {
			return fmt.Sprintf("no instance type in %s provides a GPU", where)
		}
		return fmt.Sprintf("every instance type in %s that passed the other rules carries a GPU, and an idle GPU is never recommended", where)
	case RuleBurstableClass:
		return fmt.Sprintf("no instance type in %s matches the workload's burstable classification", where)
	case RuleZeroDensity:
		// Rule R9a tests the demand-weighted MEAN task, because that is what
		// TasksPerInstance packs. The mean is not what the sentence may quote:
		// it is a figure the user cannot find in {env}.yaml, cannot find in
		// signals, and cannot change, so it sends them looking for a number
		// that does not exist anywhere. EC-11 names a service and its own
		// reservation instead.
		//
		// The substitution is sound, not just friendlier. The largest task is
		// at least as large as the mean in both dimensions, so a mean that no
		// instance can pack implies a largest task that no instance can place
		// -- and where R9a fired on the awsvpc ENI cap instead, no instance
		// places any task at all, so the sentence holds there too.
		if d.LargestTaskName != "" {
			return fmt.Sprintf("no instance type in %s can place %s (%.1f vCPU / %.1f GiB) on a single instance",
				where, d.LargestTaskName, d.LargestTaskVCPU, d.LargestTaskMemGiB)
		}
		return fmt.Sprintf("no instance type in %s can place a single task of %.1f vCPU / %.1f GiB",
			where, d.MaxTaskVCPU, d.MaxTaskMemGiB)
	case RuleGeneration:
		return fmt.Sprintf("no current-generation instance type in %s survived the other rules; set include_previous_generation to widen the search", where)
	case RuleFamilyNotEligible:
		// Reachable only when R10 is the ONLY rule that fired, i.e. the region
		// really does carry nothing outside the accelerator, storage, HPC,
		// FPGA, high-memory and Mac families. The sentence names the escape
		// hatch, because it is the one the user has.
		return fmt.Sprintf("no general-purpose, compute-optimised, memory-optimised or burstable instance type in %s is available; list a specific type in the pool's instance_types to size against a family this tool does not recommend by default", where)
	case RuleNonFiniteScore:
		return fmt.Sprintf("every candidate in %s produced a non-finite score, which is a defect rather than a constraint", where)
	case "":
		return fmt.Sprintf("no instance type in %s survived the hard constraints", where)
	default:
		return fmt.Sprintf("no instance type in %s survived rule %s", where, rule)
	}
}

// nearest picks EC-11's nearest misses: the candidates that came closest to
// passing. Closeness is available/needed where the rule has a numeric
// dimension; rules without one sort last. Ties break by rule priority and then
// by name, so the list is deterministic.
func nearest(misses []Miss, n int) []Miss {
	if len(misses) == 0 {
		return nil
	}
	sorted := make([]Miss, len(misses))
	copy(sorted, misses)
	closeness := func(m Miss) float64 {
		if !(m.Needed > 0) {
			return -1
		}
		c, ok := ratio(m.Available, m.Needed)
		if !ok {
			return -1
		}
		return c
	}
	sort.SliceStable(sorted, func(i, j int) bool {
		ci, cj := closeness(sorted[i]), closeness(sorted[j])
		if ci != cj {
			return ci > cj
		}
		pi, pj := rulePriorityIndex(sorted[i].FailedRule), rulePriorityIndex(sorted[j].FailedRule)
		if pi != pj {
			return pi < pj
		}
		return sorted[i].InstanceType < sorted[j].InstanceType
	})
	if len(sorted) > n {
		sorted = sorted[:n]
	}
	return sorted
}

// Recommend is the whole core: inputs in, ranking out. No context, no error,
// no clock, no goroutines, no logging, no randomness.
func Recommend(in Input) Result {
	pool := NormalizePool(in.Pool)
	posture := NormalizePosture(in.Posture)
	params := Params(posture)
	limit := in.Limit
	if limit <= 0 {
		limit = defaultLimit
	}
	catalog := normalizeCatalog(in.Catalog)
	gpuRequested := pool.ForceGPU || pool.AMIFamily == AMIFamilyAL2023GPU
	// TrunkingEnabled is READ ONLY under awsvpc: under bridge there are no
	// task ENIs to trunk and the field is ignored (C-13).
	trunking := pool.NetworkMode == NetworkModeAWSVPC && in.TrunkingEnabled

	sig := Signals{NetworkMode: pool.NetworkMode}
	if pool.NetworkMode == NetworkModeBridge {
		sig.Trunking = TrunkingNotApplicable
	}

	// ---- 1. Demand -------------------------------------------------------
	d := ConfiguredShape(in.Services)
	coverage := Coverage(in.Services)
	actual := ActualShape(in.Services)
	degenerate := coverage > 0 && !actual.OK
	if !actual.OK {
		coverage = 0
	}

	sig.Configured = Shape{VCPU: d.VCPU, MemGiB: d.MemGiB, Ratio: d.Ratio}
	sig.ConfiguredTaskCount = d.TaskCount
	sig.Coverage = coverage
	sig.Services = serviceSignals(in.Services, degenerate)
	if actual.OK {
		shape := Shape{VCPU: actual.VCPU, MemGiB: actual.MemGiB, Ratio: actual.Ratio}
		sig.Actual = &shape
	}

	if d.TaskCount == 0 || !(d.VCPU > 0) || !(d.MemGiB > 0) {
		sig.WeightConfigured = 1
		sig.WeightActual = 0
		sig.Ratio = RatioSignal{ClampedTo: ClampNone}
		sig.DensityBasis = densityBasisFor(pool.NetworkMode, trunking)
		return Result{
			Basis:         BasisDefault,
			Signals:       sig,
			SuggestedPool: defaultSuggestedPool(catalog, pool, gpuRequested),
		}
	}

	basis := BasisConfigured
	if coverage > 0 {
		basis = BasisMeasured
	}
	peakVCPU, peakMemGiB := PeakDemand(in.Services, d, coverage)

	// ---- 2. PreFilter ----------------------------------------------------
	survivors, misses := preFilter(catalog, pool, gpuRequested)

	// ---- 3. Ratio range --------------------------------------------------
	catalogMin, catalogMax := catalogRatioRange(survivors)

	// ---- 4. Blend + clamp ------------------------------------------------
	rRaw, wCfg, wAct := Blend(d.Ratio, actual.Ratio, coverage)
	sig.WeightConfigured = wCfg
	sig.WeightActual = wAct
	rEff, clampedTo := ClampRatio(rRaw, catalogMin, catalogMax)
	sig.Ratio = RatioSignal{
		Raw:        rRaw,
		Effective:  rEff,
		CatalogMin: catalogMin,
		CatalogMax: catalogMax,
		ClampedTo:  clampedTo,
	}
	sig.DensityBasis = densityBasisFor(pool.NetworkMode, trunking)

	if len(survivors) == 0 {
		return Result{
			Basis:         basis,
			Unsatisfiable: true,
			Constraint:    constraintMessage(in.Region, misses, d, pool, gpuRequested),
			NearestMisses: nearest(misses, nearestMissCount),
			Signals:       sig,
			SuggestedPool: defaultSuggestedPool(catalog, pool, gpuRequested),
		}
	}

	// ---- 5. Classify -----------------------------------------------------
	class := Classify(ClassifyInput{
		REff:        rEff,
		MaxTaskVCPU: d.MaxTaskVCPU,
		CPUAvg:      actual.CPUAvg,
		CPUPeak:     actual.CPUPeak,
		Coverage:    coverage,
		Posture:     posture,
		GPU:         gpuRequested,
	})

	// ---- 6. Class filters ------------------------------------------------
	eligibles, classMisses := classFilter(survivors, classFilterInput{
		demand:   d,
		class:    class,
		pool:     pool,
		trunking: trunking,
	})

	// C-15, continued past where it stopped: making the burstable test
	// per-task narrowed this hole, it did not close it.
	//
	// Rule R8 is the only rule that reads the class, and burstable is the only
	// class it narrows to a family. So burstable is the only classification
	// that can EMPTY an otherwise satisfiable candidate set -- and the
	// classification is an inference drawn from utilisation, not a choice: no
	// YAML field selects it and none can turn it off. Refusing here produced
	// "no instance type in {region} matches the workload's burstable
	// classification", which is EC-11's `unsatisfiable` -- a statement about
	// CAPACITY -- spent on a knob that does not exist, for a fleet the region
	// demonstrably can host: the identical services under performance-first,
	// where FR-19 suppresses the same class, are served a full recommendation
	// from the identical catalog.
	//
	// So it falls back the way FR-19 already does, to the ratio class, and
	// records the inference it could not honour. `unsatisfiable` is left to
	// mean what EC-11 says it means: nothing in the region can hold this.
	inferredClass := ""
	if len(eligibles) == 0 && class == ClassBurstable {
		alt := ratioClass(rEff)
		altEligibles, altMisses := classFilter(survivors, classFilterInput{
			demand:   d,
			class:    alt,
			pool:     pool,
			trunking: trunking,
		})
		// The swap is unconditional on purpose. With candidates it is the
		// fallback; without them the ratio class's misses are the ones that
		// name a capacity rule, and quoting R8 in a refusal that survives even
		// after the class is dropped would blame the wrong thing.
		inferredClass, class = class, alt
		eligibles, classMisses = altEligibles, altMisses
	}

	allMisses := append(append([]Miss{}, misses...), classMisses...)

	if len(eligibles) == 0 {
		return Result{
			Classification:           class,
			ClassificationSuppressed: inferredClass,
			Basis:                    basis,
			Unsatisfiable:            true,
			Constraint:               constraintMessage(in.Region, allMisses, d, pool, gpuRequested),
			NearestMisses:            nearest(allMisses, nearestMissCount),
			Signals:                  sig,
			SuggestedPool:            defaultSuggestedPool(catalog, pool, gpuRequested),
		}
	}

	// ---- 7. Score --------------------------------------------------------
	capacityType, onDemandBase := resolveCapacity(pool, params)
	// peakVCPU / peakMemGiB are deliberately NOT passed: scoring reads
	// measured data only through R_eff, and the peaks now have exactly one
	// consumer, buildSuggestedPool's performance-first min_size.
	items := scoreCandidates(eligibles, scoreContext{
		rEff:         rEff,
		demand:       d,
		params:       params,
		pool:         pool,
		capacityType: capacityType,
		onDemandBase: onDemandBase,
		newestGen:    newestGenerations(catalog),
	})

	// ---- 8. Rank ---------------------------------------------------------
	ranked, dropped := Rank(items, params.FamilyLean)
	sig.Dropped = dropped

	if len(ranked) == 0 {
		return Result{
			Classification:           class,
			ClassificationSuppressed: inferredClass,
			Basis:                    basis,
			Unsatisfiable:            true,
			Constraint:               constraintMessage(in.Region, append(allMisses, dropped...), d, pool, gpuRequested),
			NearestMisses:            nearest(append(allMisses, dropped...), nearestMissCount),
			Signals:                  sig,
			SuggestedPool:            defaultSuggestedPool(catalog, pool, gpuRequested),
		}
	}

	sig.DensityBasis = ranked[0].DensityBasis

	// ---- 9. SuggestedPool ------------------------------------------------
	suggested := buildSuggestedPool(ranked, poolContext{
		pool:       pool,
		params:     params,
		class:      class,
		demand:     d,
		peakVCPU:   peakVCPU,
		peakMemGiB: peakMemGiB,
	})

	// Reasons are written last, because the runner-up is a ranking fact and
	// the clamp note is a Signals fact.
	spotWanted := capacityType == CapacitySpot || capacityType == CapacitySpotWithBase
	out := make([]Candidate, 0, len(ranked))
	for i, item := range ranked {
		runnerUp := ""
		if i+1 < len(ranked) {
			runnerUp = ranked[i+1].InstanceType
		}
		c := item.Candidate
		c.Reason = Explain(c, runnerUp, class, inferredClass, sig, spotWanted && !item.SpotUsable)
		out = append(out, c)
	}
	if len(out) > limit {
		out = out[:limit]
	}
	primary := out[0]

	return Result{
		Primary:                  &primary,
		Ranked:                   out,
		Classification:           class,
		ClassificationSuppressed: inferredClass,
		Basis:                    basis,
		Signals:                  sig,
		SuggestedPool:            suggested,
	}
}
