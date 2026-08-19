package recommend

import "math"

// FR-22's sub-scores (FOUR of them, not five -- see SubScores in types.go),
// FR-23's weights, FR-24's posture parameters, and the corrected
// effectiveHourly blend (C-5).

// Family lean values for FR-24's tie-break.
const (
	LeanCompute  = "c"        // performance-first prefers compute-optimized
	LeanNone     = ""         // balanced has no lean
	LeanCheapest = "cheapest" // cost-first prefers the lower effectiveHourly
)

// Min-size bases for FR-24.
const (
	MinSizePeak       = "peak"
	MinSizeConfigured = "configured"
)

// PostureParams are FR-23's weights plus FR-24's hard parameters.
type PostureParams struct {
	Weights SubScores
	// TargetUtilisation is the achieved packing this posture is aiming at --
	// the fraction of an instance its share of the fleet actually occupies.
	// It replaces TargetHeadroom, which was a FLEET-level slack multiple and
	// is documented as removed below.
	TargetUtilisation float64
	CapacityType      string
	OnDemandBase      int
	AllowBurstable    bool
	FamilyLean        string
	MinSizeBasis      string
	MaxSizeFactor     float64
	TargetCapacity    int
}

// Params returns the posture's weight table and hard parameters.
//
// ---------------------------------------------------------------------------
// What each weight means
// ---------------------------------------------------------------------------
//
//	Fit         SHAPE. How closely the instance's GiB-per-vCPU matches R_eff,
//	            the blended and catalog-clamped demand ratio. Reads measured
//	            CloudWatch data through FR-17's blend. Says nothing about size.
//	Utilisation FILL. How close the instance's ACHIEVED occupancy -- the tasks
//	            it really ends up holding, not the tasks it could hold -- lands
//	            to TargetUtilisation. Says nothing about price.
//	Cost        MONEY. The pool this candidate implies, per hour, per task it
//	            runs -- effectiveHourly * N / T -- normalised to 1.0 for the
//	            cheapest survivor. NOT cost per packing slot, which prices slots
//	            no task occupies; see CostPerTask in types.go.
//	Modernity   GENERATION. Newest generation of its family in this region's
//	            catalog scores 1.0, one back 0.7, two back 0.4, older 0.
//
// Cost and Utilisation overlap and do not cancel, which is the distinction
// that matters. With EC2's within-family linear pricing, fleet cost per task
// factors as (per-vCPU rate) x (task vCPU) / (achieved CPU utilisation):
// Utilisation carries the second factor and Cost carries the whole product, so
// what Cost adds on top is the per-vCPU RATE -- m6i against m7i against
// Graviton -- which Utilisation is blind to. Both rise with a better answer, so
// they reinforce. waste and headroom moved in OPPOSITE directions, which is
// why their weights had to be read as a difference to mean anything.
//
// ---------------------------------------------------------------------------
// Why the table changed, and how much
// ---------------------------------------------------------------------------
//
//	posture             fit   waste  head.  ->  fit   util   cost  modern.
//	performance-first   0.20  0.10   0.40   ->  0.30  0.35   0.15  0.20
//	balanced            0.25  0.20   0.20   ->  0.30  0.30   0.35  0.05
//	cost-first          0.20  0.30   0.05   ->  0.20  0.35   0.45  0.00
//
// The obvious move -- set the new Utilisation weight to the old Waste +
// Headroom and leave everything else alone -- was tried first and is WRONG,
// for a reason worth recording rather than discovering twice.
//
// Those two weights did not describe one emphasis split across two terms. On
// the default path -- a fleet with no CloudWatch history, which is every newly
// configured project -- headroom was identically 0 for every candidate (see
// the note above achievedOccupancy). So the LIVE weight on "how the instance
// is filled" was waste alone: 0.10 for performance-first, not 0.50. Merging
// arithmetically would have multiplied the live emphasis by five and called it
// neutral.
//
// Measured on the ENV-MEM8 fixture -- six tasks reserving 8 GiB per vCPU, so
// an r-family answer is the only defensible one -- the arithmetic merge scored
// r7i.xlarge 0.8020 against m7i.xlarge 0.7959. Both quantize into FR-25's same
// 0.01 bucket, so the right answer survived only because a tie-break happened
// to point at it. A 0.006 margin between a correctly shaped instance and one
// with half the memory per vCPU is not a recommendation, it is a coin toss
// with a reason attached.
//
// So the table is set from what each term means:
//
//   - FIT RISES EVERYWHERE (0.20/0.25/0.20 -> 0.30/0.30/0.20). Shape is the
//     only dimension where being wrong strands a resource permanently: a
//     memory-heavy fleet on a CPU-rich instance leaves memory unbuyable for
//     the life of the pool, and no amount of scaling recovers it. It was
//     under-weighted relative to a slack term that was measuring the wrong
//     thing.
//   - UTILISATION LANDS NEAR THE OLD LIVE COMBINED EMPHASIS, not the nominal
//     one, and is the largest or joint-largest non-cost term in every posture
//     because it is now the only term that reads fleet SIZE at all.
//   - COST IS UNCHANGED for performance-first and cost-first, and rises for
//     balanced (0.30 -> 0.35) so that the neutral posture has a term that
//     actually leans; with fit and utilisation equal at 0.30 it otherwise
//     decided nothing.
//   - MODERNITY rises for performance-first (0.15 -> 0.20) because generation
//     is the only lever that raises per-core throughput without changing size,
//     and it stays 0.00 for cost-first, where newer silicon is a cost.
//
// behaviour_matrix_test.go turns this table into an asserted workload x posture
// matrix, so the next person to move one of these numbers sees exactly which
// recommendations it changes.
//
// ---------------------------------------------------------------------------
// DEV-26
// ---------------------------------------------------------------------------
//
// Cost-first's min_size is derived from demand (MinSizeConfigured), not
// FR-24's literal 1. With a literal 1 a pool holding 2 of 6 tasks was
// "correct" by construction, and four tasks sat in PROVISIONING for the 2-5
// minutes managed scaling needs to boot more, on every scale-to-min.
// Cost-first still differs in max_size, target_capacity, capacity_type and its
// weights; it no longer differs by promising capacity it does not provision.
func Params(p Posture) PostureParams {
	switch p {
	case PostureCost:
		return PostureParams{
			Weights: SubScores{Fit: 0.20, Utilisation: 0.35, Cost: 0.45, Modernity: 0.00},
			// 1.00: every slot bought and not filled is money spent on
			// nothing, and cost-first has no competing objective to trade that
			// against. Because achieved utilisation cannot exceed 1.0, the
			// overshoot branch of utilisationScore is unreachable here and the
			// score degenerates to raw packing efficiency -- which is what
			// cost-first has always meant.
			TargetUtilisation: 1.00,
			CapacityType:      CapacitySpot,
			OnDemandBase:      0,
			AllowBurstable:    true,
			FamilyLean:        LeanCheapest,
			MinSizeBasis:      MinSizeConfigured,
			MaxSizeFactor:     2,
			TargetCapacity:    100,
		}
	case PostureBalanced:
		return PostureParams{
			Weights: SubScores{Fit: 0.30, Utilisation: 0.30, Cost: 0.35, Modernity: 0.05},
			// 0.85: unbought capacity is a CERTAIN cost, burst contention is a
			// PROBABILISTIC one, so the neutral posture sits nearer cost-first
			// than performance-first. On a .large holding three tasks it
			// leaves roughly one task's worth of slack.
			TargetUtilisation: 0.85,
			CapacityType:      CapacitySpotWithBase,
			OnDemandBase:      1,
			AllowBurstable:    true,
			FamilyLean:        LeanNone,
			MinSizeBasis:      MinSizeConfigured,
			MaxSizeFactor:     3,
			TargetCapacity:    100,
		}
	default: // PosturePerformance
		return PostureParams{
			Weights: SubScores{Fit: 0.30, Utilisation: 0.35, Cost: 0.15, Modernity: 0.20},
			// 0.70: about 30 % of every instance is left unsubscribed, and
			// that remainder is the burst pool the tasks sharing the instance
			// draw from when one of them exceeds its reservation. It replaces
			// TargetHeadroom = 2.0, which asked for twice the FLEET's measured
			// peak -- a quantity that was 1.0 for every candidate on a fleet
			// with no CloudWatch history, and therefore ranked nothing at all
			// on the default path.
			TargetUtilisation: 0.70,
			CapacityType:      CapacityOnDemand,
			OnDemandBase:      0,
			AllowBurstable:    false,
			FamilyLean:        LeanCompute,
			MinSizeBasis:      MinSizePeak,
			MaxSizeFactor:     3,
			TargetCapacity:    80,
		}
	}
}

// FleetInstances is N: the number of instances the pool runs to hold the
// fleet. It is at least 1 whenever there is any demand at all.
func FleetInstances(taskCount, tasksPerInstance int) int {
	if tasksPerInstance < 1 || taskCount < 1 {
		return 0
	}
	n, ok := ratio(float64(taskCount), float64(tasksPerInstance))
	if !ok {
		return 0
	}
	return max(1, ceilToInt(n))
}

// EffectiveHourly is the corrected FR-22 blend (C-5), computed at the level
// where the money is actually spent: instances.
//
// FR-22 blended with weights onDemandBase/tasksPerInstance and
// 1 - onDemandBase/tasksPerInstance. onDemandBase counts INSTANCES -- it
// renders as on_demand_base_capacity inside instances_distribution, which the
// ASG reads as a number of instances -- while tasksPerInstance counts TASKS.
// The ratio is instances / tasks: not dimensionless, not bounded by 1, and the
// complementary weight is negative whenever b > k. With b = 2, k = 1,
// p_od = 0.10 and p_sp = 0.04 it returned 0.16, i.e. 60 % ABOVE the on-demand
// price for a pool that is at most half on-demand. It also made w_od fall as k
// rose, so a denser candidate was scored cheaper twice -- once here and again
// through costPerTaskSlot = effectiveHourly / k.
//
// Here both weights are n_od/N and n_sp/N: instances over instances, so
// dimensionless; both in [0,1], because n_od = min(N,b) <= N forces n_sp >= 0;
// and they sum to exactly 1. The result is therefore always inside
// [min(p_od,p_sp), max(p_od,p_sp)] -- a price that can exist.
func EffectiveHourly(onDemand, spot float64, capacityType string, onDemandBase, instances int) float64 {
	if instances < 1 {
		return 0
	}
	var b int
	switch capacityType {
	case CapacitySpot:
		b = 0
	case CapacitySpotWithBase:
		b = max(0, onDemandBase)
	default: // on_demand, and anything unrecognised
		b = instances
	}
	nOD := min(instances, b)
	nSP := instances - nOD
	eff, ok := ratio(float64(nOD)*onDemand+float64(nSP)*spot, float64(instances))
	if !ok {
		return onDemand
	}
	return eff
}

// fitScore is FR-22's fit: a 4x shape mismatch scores 0.
//
// Division 4 of note 5: rEff cannot be <= 0 here, because it has been clamped
// into [catalogMin, catalogMax] and both bounds are > 0 (rule R0 removed
// zero-vCPU records before the range was taken).
func fitScore(instanceRatio, rEff float64) float64 {
	q, ok := ratio(instanceRatio, rEff)
	if !ok || !(q > 0) {
		return 0
	}
	d, ok := ratio(math.Abs(math.Log(q)), math.Log(4))
	if !ok {
		return 0
	}
	return clamp(1-minf(1, d), 0, 1)
}

// ---------------------------------------------------------------------------
// Utilisation: the replacement for FR-22's waste/headroom pair
// ---------------------------------------------------------------------------
//
// WHAT WAS WRONG WITH THE PAIR
//
// waste    = how full the instance is once whole tasks are packed onto it.
//            Higher is better packed.
// headroom = fleet capacity divided by measured peak demand, mapped through
//            (h-1)/(target-1). Higher is more slack.
//
// They are near-inverses of the same axis, so each posture expressed its real
// preference as the DIFFERENCE of two partly-cancelling terms:
// performance-first as 0.10 waste against 0.40 headroom, cost-first as 0.30
// against 0.05. The net direction was right; the decomposition was not, which
// is why the individual weights read as precise and behaved badly.
//
// Two concrete failures, both provable from the formula rather than observed
// once and generalised:
//
//  1. REVIEW FINDING N-6. Write P_v = ConfiguredVCPU/PeakVCPU and
//     P_m = ConfiguredMemGiB/PeakMemGiB, rho_i for the instance's usable
//     GiB-per-vCPU, rho_d for the demand's and rho_u = PeakMemGiB/PeakVCPU for
//     the measured one. Substituting n_exact = max(cfgV/v, cfgM/u) into h and
//     taking the min collapses to
//
//     rho_i < rho_d:  h = P_m * min(1, rho_u/rho_i)
//     rho_i > rho_d:  h = P_v * min(1, rho_i/rho_u)
//
//     Both branches RISE as rho_i moves away from rho_d, because a mismatched
//     shape forces the binding dimension to buy surplus in the other one. So a
//     0.40-weighted sub-score under the default posture paid for poor fit,
//     while `fit` at 0.20 paid for good fit. The pair did not merely overlap;
//     under performance-first it pointed backwards, with twice the weight.
//
//  2. IT RANKED NOTHING ON AN IDLE FLEET. With no CloudWatch coverage,
//     PeakDemand falls back to configured demand, so P_v = P_m = 1 and
//     rho_u = rho_d, and both branches give h = 1 EXACTLY for every candidate
//     -- hence headroom = 0 for every candidate. On the default path of a
//     newly configured project, 0.40 of performance-first's table was dead
//     weight, silently renormalising the posture to fit 0.33 / waste 0.17 /
//     cost 0.25 / modernity 0.25. Nobody chose that table.
//
// WHAT REPLACES IT
//
// One term, measured on ACHIEVED occupancy rather than on accidental slack:
// the tasks an instance really ends up holding once the fleet is spread across
// the pool. That quantity cannot reward mismatch (a mismatched shape underfills
// one dimension and drags the average down) and cannot go dead on an idle
// fleet (it reads configured demand, which always exists, and never reads a
// peak).

// achievedOccupancy is k_eff: the tasks a single instance of this type ACTUALLY
// carries, as opposed to the k it could carry.
//
// The distinction is the whole point. A pool of N = ceil(T/k) instances holds T
// tasks, so the average instance carries T/N -- which equals k only when k
// divides T. It is real-valued on purpose: the alternative is to model the
// fleet as (N-1) full instances and one part-full one, and then score a
// candidate on which of the two you happened to look at.
//
// Worked, for the six-task fixture of 0.5 vCPU / 2 GiB:
//
//	m7i.large    k = 3    N = 2   k_eff = 3.0   -- both instances full
//	m7i.xlarge   k = 6    N = 1   k_eff = 6.0   -- one instance full
//	m7i.2xlarge  k = 13   N = 1   k_eff = 6.0   -- one instance 46 % full
//	m7i.24xlarge k = 163  N = 1   k_eff = 6.0   -- one instance 3.7 % full
//
// The old waste score read k and rated all four highly, which is the mechanism
// by which an enormous instance won cost-first: it scored as though it were
// full of tasks that do not exist.
func achievedOccupancy(taskCount, tasksPerInstance int) float64 {
	n := FleetInstances(taskCount, tasksPerInstance)
	if n < 1 {
		return 0
	}
	k, ok := ratio(float64(taskCount), float64(n))
	if !ok {
		return 0
	}
	return k
}

// achievedUtilisation is U: the fraction of an instance its real share of the
// fleet occupies, averaged over the two dimensions ECS places on.
//
// Each dimension is clamped to 1 before averaging, so the binding dimension
// cannot lend capacity to the other one. Division 9 of note 5 is guarded by
// rule R0, which removed every vcpu == 0 and memory == 0 record.
func achievedUtilisation(kEff, vcpuPerTask, memGiBPerTask float64, vcpu int, usableMem float64) float64 {
	byCPU, okCPU := ratio(kEff*vcpuPerTask, float64(vcpu))
	byMem, okMem := ratio(kEff*memGiBPerTask, usableMem)
	if !okCPU || !okMem {
		return 0
	}
	return clamp((clamp(byCPU, 0, 1)+clamp(byMem, 0, 1))/2.0, 0, 1) // constant divisor
}

// utilisationScore rates U against the posture's target: 1.0 at the target,
// falling linearly to 0 at each end of the range.
//
// It is a tent and not a one-sided ramp because both directions are real
// costs, and they are different costs: undershooting means paying for capacity
// nothing occupies, overshooting means an instance with no room for a task to
// exceed its reservation.
//
// The asymmetry is NOT a tuned constant. Each side is normalised by the
// distance from the target to its own bound -- U/target below, and
// (1-U)/(1-target) above -- so the slopes are 1/target and 1/(1-target). A
// posture whose target sits near full therefore penalises the remaining slack
// far more steeply than it penalises underfill, and one whose target sits low
// does the reverse. That falls out of the geometry; there is no second knob.
//
// At target == 1.0 the upper branch is unreachable (U <= 1 by construction) and
// the score is U itself. At target <= 0 there is no meaningful target and the
// score is 0 for everyone, which ranks nothing rather than ranking wrongly.
func utilisationScore(u, target float64) float64 {
	if !(target > 0) {
		return 0
	}
	if u <= target {
		s, ok := ratio(u, target)
		if !ok {
			return 0
		}
		return clamp(s, 0, 1)
	}
	over, ok := ratio(u-target, 1-target)
	if !ok {
		// target >= 1 with u > target is unreachable while u is clamped to
		// [0,1]; answering "at target" rather than 0 keeps a future caller
		// that widens u from silently zeroing the whole dimension.
		return 1
	}
	return clamp(1-over, 0, 1)
}

// modernityScore is FR-22's modernity: newest generation present in the
// region's catalog for that family scores 1.0, one back 0.7, two back 0.4,
// older 0. Row 10 of note 5: no division, and a family with no parseable
// generation scores 0.
func modernityScore(family string, generation int, newest map[string]int) float64 {
	if generation <= 0 {
		return 0
	}
	top, ok := newest[family]
	if !ok || top <= 0 {
		return 0
	}
	switch top - generation {
	case 0:
		return 1.0
	case 1:
		return 0.7
	case 2:
		return 0.4
	default:
		return 0
	}
}

// exactInstances is n_exact: the real-valued number of instances of this size
// needed to hold configured demand, in the binding dimension.
func exactInstances(configuredVCPU, configuredMemGiB float64, vcpu int, usableMem float64) float64 {
	byCPU, okCPU := ratio(configuredVCPU, float64(vcpu))
	byMem, okMem := ratio(configuredMemGiB, usableMem)
	if !okCPU {
		byCPU = 0
	}
	if !okMem {
		byMem = 0
	}
	return maxf(byCPU, byMem)
}

// instancesAtFloor is n_floor: what SuggestedPool.MinSize reports, because you
// cannot run 1.76 instances.
//
// Note this is where the pool's pinned MinSize enters -- as a floor on the
// floor, never as a constraint. n_floor >= n_exact by construction, which is
// exactly why D-8's R9b (h >= 1.0) excludes nothing and was removed.
//
// THE fleet ARGUMENT, and why ceil(n_exact) alone is not enough.
//
// n_exact is a RESOURCE ratio: total demand over one instance's capacity. Task
// placement is not a resource ratio, it is an integer packing, and
// TasksPerInstance floors it -- so an instance whose usable memory is 6.8 GiB
// holds three 2 GiB tasks and strands 0.8 GiB. n_exact does not see that
// stranding, and the two answers diverge as soon as the remainder accumulates:
// for 16 tasks of 0.5 vCPU / 2 GiB on m7i.large, n_exact is 4.706 -> 5
// instances, while k = 3 means five instances hold FIFTEEN tasks.
//
// A min_size that provably cannot hold the fleet it was sized for is the same
// failure DEV-26 removed for cost-first, arriving by a different route: on
// every scale-to-min the leftover tasks sit in PROVISIONING for the two to
// five minutes managed scaling needs to boot another instance. DEV-26's own
// sentence -- that cost-first "no longer differs by promising capacity it does
// not provision" -- was true of the posture and false of the arithmetic, for
// all three postures.
//
// So the floor is the larger of the two: enough RESOURCE, and enough PLACES.
func instancesAtFloor(nExact float64, fleet int, tasksPerInstance int, minSize *int) int {
	n := ceilToInt(nExact)
	if byPlacement := FleetInstances(fleet, tasksPerInstance); byPlacement > n {
		n = byPlacement
	}
	if minSize != nil && *minSize > n {
		n = *minSize
	}
	if n < 1 {
		n = 1
	}
	return n
}

// NOTE ON THE REMOVED headroomScore.
//
// It is gone rather than repaired, and D-8's conclusion is unaffected: h was
// never a gate, and the rule that can genuinely fail to hold the workload is
// still R9c on the pool's CEILING in filter.go. R9b stays deleted -- it was
// proved vacuous because n_floor >= n_exact by construction, and nothing here
// re-opens that argument.
//
// The peak demand h used has not left the pipeline. It still drives
// performance-first's min_size through poolContext.peakVCPU / peakMemGiB in
// pool.go, which is the place a measured peak has a decision to make: how many
// instances to keep warm. Measured data reaches RANKING through FR-17's blend
// into R_eff, which `fit` reads. What it no longer does is enter the score a
// second time as a slack term that pointed the wrong way.

// scoreContext is everything scoring reads that is not the candidate itself.
//
// It no longer carries peakVCPU / peakMemGiB: the only consumer was
// headroomScore. They are still computed in Recommend, and still passed to
// buildSuggestedPool.
type scoreContext struct {
	rEff         float64
	demand       Demand
	params       PostureParams
	pool         PoolConstraints
	capacityType string
	onDemandBase int
	newestGen    map[string]int
}

// resolveCapacity picks the capacity type the blend prices against: the pool's
// own setting when the user has one, otherwise the posture's suggestion, which
// is what a not-yet-created pool will be written with.
func resolveCapacity(pool PoolConstraints, params PostureParams) (string, int) {
	switch pool.CapacityType {
	case CapacityOnDemand, CapacitySpot, CapacitySpotWithBase:
		return pool.CapacityType, pool.OnDemandBase
	default:
		return params.CapacityType, params.OnDemandBase
	}
}

// scoreCandidates computes every sub-score except cost, which needs the
// minimum over the whole surviving set (EC-2: over PRICED survivors only, and
// rule R7 has already guaranteed every survivor is priced).
func scoreCandidates(cands []eligible, ctx scoreContext) []RankItem {
	items := make([]RankItem, 0, len(cands))

	for _, c := range cands {
		it := c.it
		usable := UsableMemGiB(it.MemoryMiB)
		memGiB := float64(it.MemoryMiB) / 1024.0 // constant divisor
		// Division 3 of note 5: rule R0 removed every vcpu == 0 record.
		instRatio, _ := ratio(memGiB, float64(it.VCPU))
		nExact := exactInstances(ctx.demand.VCPU, ctx.demand.MemGiB, it.VCPU, usable)

		pOD := *it.OnDemandHourly
		pSP := pOD
		spotUsable := it.SupportsSpot && it.SpotMedianHourly != nil &&
			*it.SpotMedianHourly > 0 && isFinite(*it.SpotMedianHourly)
		// spotMedian is what the RESPONSE publishes, and it is gated on the
		// same test the blend is gated on. Copying it.SpotMedianHourly through
		// unconditionally was the one route by which a number the pricing
		// arithmetic had already rejected still reached the client: a +Inf
		// median aborts encoding/json mid-object, so the browser gets HTTP 200
		// and a truncated body while the server logs a success (NFR-6). Nil is
		// the answer EC-5 asks for -- "spot unavailable", never a value, and
		// never 0, which would read as free.
		var spotMedian *float64
		if spotUsable {
			pSP = *it.SpotMedianHourly
			spotMedian = it.SpotMedianHourly
		}

		n := FleetInstances(ctx.demand.TaskCount, c.tasks)
		eff := EffectiveHourly(pOD, pSP, ctx.capacityType, ctx.onDemandBase, n)
		// FLEET cost per task: what the pool this candidate implies costs per
		// hour, divided by the tasks it runs. See CostPerTask in types.go for
		// why this is not eff/k.
		cpt, ok := ratio(eff*float64(n), float64(ctx.demand.TaskCount))
		if !ok {
			cpt = 0
		}

		// Achieved occupancy, not the candidate's packing capacity: n is
		// already ceil(T/k), so T/n is what an instance of this type really
		// carries once the fleet is spread over the pool.
		kEff := achievedOccupancy(ctx.demand.TaskCount, c.tasks)
		util := achievedUtilisation(kEff, ctx.demand.VCPUPerTask, ctx.demand.MemGiBPerTask, it.VCPU, usable)

		item := RankItem{
			Candidate: Candidate{
				InstanceType:     it.Name,
				VCPU:             it.VCPU,
				MemoryMiB:        it.MemoryMiB,
				Architecture:     primaryArch(it.Architectures),
				TasksPerInstance: c.tasks,
				InstancesAtFloor: instancesAtFloor(nExact, ctx.demand.TaskCount, c.tasks, ctx.pool.MinSize),
				EffectiveHourly:  eff,
				CostPerTask:      cpt,
				SpotMedianHourly: spotMedian,
				Scores: SubScores{
					Fit:         fitScore(instRatio, ctx.rEff),
					Utilisation: utilisationScore(util, ctx.params.TargetUtilisation),
					Modernity:   modernityScore(it.Family, it.Generation, ctx.newestGen),
				},
			},
			Family:            it.Family,
			Generation:        it.Generation,
			MaxENI:            it.MaxNetworkInterfaces,
			DensityBasis:      c.densityBasis,
			SpotUsable:        spotUsable,
			NExact:            nExact,
			AchievedOccupancy: kEff,
			Utilisation:       util,
		}
		items = append(items, item)
	}

	// Division 7 of note 5: minCost is computed AFTER filtering, over priced
	// survivors only, and rule R7 has forced every costPerTask > 0.
	minCost := 0.0
	for _, it := range items {
		if it.CostPerTask > 0 && (minCost == 0 || it.CostPerTask < minCost) {
			minCost = it.CostPerTask
		}
	}
	for i := range items {
		cost, ok := ratio(minCost, items[i].CostPerTask)
		if !ok {
			cost = 0
		}
		items[i].Scores.Cost = clamp(cost, 0, 1)
		items[i].Total = totalScore(items[i].Scores, ctx.params.Weights)
	}
	return items
}

// totalScore is FR-23's weighted sum. Sub-scores are clamped to [0,1] before
// the sum, so a single bad sub-score cannot make Total unbounded.
func totalScore(s SubScores, w SubScores) float64 {
	return clamp(s.Fit, 0, 1)*w.Fit +
		clamp(s.Utilisation, 0, 1)*w.Utilisation +
		clamp(s.Cost, 0, 1)*w.Cost +
		clamp(s.Modernity, 0, 1)*w.Modernity
}

// primaryArch is the architecture reported for a candidate. Rule R4 has
// already forced every survivor to a single family's architecture, so a record
// advertising both reports the one the AMI will boot.
func primaryArch(archs []string) string {
	if hasArch(archs, ArchARM64) && !hasArch(archs, ArchX8664) {
		return ArchARM64
	}
	if hasArch(archs, ArchX8664) {
		return ArchX8664
	}
	if len(archs) > 0 {
		return archs[0]
	}
	return ""
}
