package recommend

// FR-21's hard constraints, split across the two pipeline stages of
// architecture.md 4.4 -- eleven rules, all of which EXCLUDE rather than
// down-rank, and every one of which produces a Miss for EC-11's nearest-miss
// list.
//
// The split into PreFilter (demand-independent) and ClassFilter
// (demand-dependent) is about evaluation order, not about two different
// filters: the catalog's ratio range must be computed over a candidate set,
// the classification needs that range's clamp, and the class rules need the
// classification. That is the circle 4.4 breaks.

// eligible is a candidate that survived every hard rule, carrying the density
// figures the scorer would otherwise recompute.
type eligible struct {
	it           InstanceType
	tasks        int
	densityBasis string
}

// hasArch reports whether the catalog record advertises an architecture.
func hasArch(archs []string, want string) bool {
	for _, a := range archs {
		if a == want {
			return true
		}
	}
	return false
}

// archOK is rule R4, and it is SYMMETRIC (C-11).
//
// The one-directional version ("al2023_arm64 -> arm64 only, when one is set")
// combined with an AMIFamily defaulting to "" meant a new pool ranked Graviton
// types, the user accepted them, and the ASG failed every launch with "The
// architecture 'arm64' of the specified instance type does not match the
// architecture 'x86_64' of the AMI" -- visible only in scaling activities,
// with terraform apply reporting success.
//
// The empty family behaves as al2023 here as well as in NormalizePool, so an
// un-normalised zero value cannot re-open the hole.
func archOK(amiFamily string, archs []string) bool {
	if amiFamily == AMIFamilyAL2023ARM64 {
		return hasArch(archs, ArchARM64)
	}
	return hasArch(archs, ArchX8664)
}

// preFilter is pipeline step 2: the demand-INDEPENDENT hard rules, over the
// whole catalog.
//
// Rules, in evaluation order: R0 malformed record, R10 family suitability,
// R1 current generation, R3 bare metal, R4 architecture (both directions),
// R5 GPU presence/absence for the requested class, R7 priced.
//
// R10 sits directly after the catalog-defect check and ahead of everything
// else because the ratio range in pipeline step 3 is taken over THESE
// survivors, and C-10 clamps R_eff into that range on the argument that it is
// the set of shapes that can actually be purchased. See family.go.
func preFilter(catalog []InstanceType, pool PoolConstraints, gpuRequested bool) ([]InstanceType, []Miss) {
	survivors := make([]InstanceType, 0, len(catalog))
	var misses []Miss

	pinned := pinnedTypeSet(pool.InstanceTypes)

	for _, it := range catalog {
		// R0 -- a catalog defect, never scored. Guards divisions 3 and 9.
		if it.VCPU <= 0 || it.MemoryMiB <= 0 {
			misses = append(misses, Miss{
				InstanceType: it.Name, FailedRule: RuleMalformedRecord,
				Needed: 1, Available: float64(it.VCPU), Unit: UnitVCPU,
			})
			continue
		}
		// R10 -- the family allowlist. An accelerator, storage, HPC, FPGA,
		// high-memory or Mac family is not a machine this tool sizes, and
		// inf1/inf2 in particular are invisible to every other rule because
		// DescribeInstanceTypes reports no GpuInfo and no AcceleratorInfo for
		// them. The rule has no numeric dimension, so Needed and Available
		// stay zero, exactly as they do for R1, R3 and R4.
		//
		// It does not run when a GPU was REQUESTED (ami_family: al2023_gpu, or
		// gpu=true). That request is itself an explicit opt-out of the
		// general-purpose scope, and R5 below then demands GPUCount >= 1 --
		// strictly narrower than the allowlist, and a bar that inf and trn
		// cannot clear precisely because they report no GpuInfo. So R10 has
		// nothing left to protect on that path and would only exclude the g
		// and p families the request exists to reach.
		if !gpuRequested && !familyEligible(it.Name, pinned) {
			misses = append(misses, Miss{InstanceType: it.Name, FailedRule: RuleFamilyNotEligible})
			continue
		}
		// R1
		if !it.CurrentGeneration && !pool.IncludePreviousGeneration {
			misses = append(misses, Miss{InstanceType: it.Name, FailedRule: RuleGeneration})
			continue
		}
		// R3
		if it.BareMetal {
			misses = append(misses, Miss{InstanceType: it.Name, FailedRule: RuleBareMetal})
			continue
		}
		// R4
		if !archOK(pool.AMIFamily, it.Architectures) {
			misses = append(misses, Miss{InstanceType: it.Name, FailedRule: RuleArchitecture})
			continue
		}
		// R5 -- an idle GPU is never recommended, and a GPU class never gets a
		// type without one.
		if gpuRequested && it.GPUCount < 1 {
			misses = append(misses, Miss{
				InstanceType: it.Name, FailedRule: RuleGPU,
				Needed: 1, Available: float64(it.GPUCount),
			})
			continue
		}
		if !gpuRequested && it.GPUCount != 0 {
			misses = append(misses, Miss{
				InstanceType: it.Name, FailedRule: RuleGPU,
				Needed: 0, Available: float64(it.GPUCount),
			})
			continue
		}
		// R7 -- strengthened past FR-21.7's "!= null" to "> 0 and finite", so
		// that effectiveHourly > 0, costPerTaskSlot > 0 and minCost > 0. A
		// zero price is never treated as free: it would make minCost 0 and
		// every cost sub-score 0/0. An unpriced candidate is EXCLUDED, never
		// scored cost = 1.0 -- 1.0 is the best score, and it would win the one
		// dimension it has no data for (EC-2).
		if it.OnDemandHourly == nil || !(*it.OnDemandHourly > 0) || !isFinite(*it.OnDemandHourly) {
			m := Miss{InstanceType: it.Name, FailedRule: RuleUnpriced, Unit: UnitUSDPerHr}
			if it.OnDemandHourly != nil && isFinite(*it.OnDemandHourly) {
				m.Available = *it.OnDemandHourly
			}
			misses = append(misses, m)
			continue
		}
		survivors = append(survivors, it)
	}
	return survivors, misses
}

// classFilterInput carries the demand-dependent facts the step-6 rules read.
type classFilterInput struct {
	demand   Demand
	class    string
	pool     PoolConstraints
	trunking bool
}

// classFilter is pipeline step 6: the demand-dependent hard rules, in the
// order 4.4 fixes -- R8, R9a, R6, R2, R9c. The order decides which Miss a
// candidate reports when it fails more than one rule.
func classFilter(survivors []InstanceType, in classFilterInput) ([]eligible, []Miss) {
	out := make([]eligible, 0, len(survivors))
	var misses []Miss

	awsvpc := in.pool.NetworkMode == NetworkModeAWSVPC

	for _, it := range survivors {
		// R8 -- class burstable takes only burstable types; every other class
		// refuses them, because CPU credits make steady-state throughput
		// unpredictable.
		if (in.class == ClassBurstable) != it.Burstable {
			misses = append(misses, Miss{InstanceType: it.Name, FailedRule: RuleBurstableClass})
			continue
		}

		k, basis := TasksPerInstance(it, in.demand.VCPUPerTask, in.demand.MemGiBPerTask,
			in.pool.NetworkMode, in.trunking)

		// R9a -- C-9's boundary made a hard rule. k == 0 is also what
		// TasksPerInstance returns for invalid demand, without dividing.
		if k < 1 {
			misses = append(misses, Miss{
				InstanceType: it.Name, FailedRule: RuleZeroDensity,
				Needed: 1, Available: 0, Unit: UnitTasks,
			})
			continue
		}

		// R6 -- ENI density, ONLY under awsvpc (C-13). Under bridge there are
		// no task ENIs, so the rule does not exist. FR-21.6's >= 3 relaxes to
		// >= 2 when trunking actually applies to this type: the trunk
		// interface replaces the per-task ones.
		if awsvpc {
			needENIs := 3
			if basis == DensityTrunkedTable {
				needENIs = 2
			}
			if it.MaxNetworkInterfaces < needENIs {
				misses = append(misses, Miss{
					InstanceType: it.Name, FailedRule: RuleENIDensity,
					Needed: float64(needENIs), Available: float64(it.MaxNetworkInterfaces),
					Unit: UnitENIs,
				})
				continue
			}
		}

		// R2 -- one task must fit on one instance. Memory is checked against
		// the usable figure: a task requesting the instance's full memory
		// never places.
		if float64(it.VCPU) < in.demand.MaxTaskVCPU {
			misses = append(misses, Miss{
				InstanceType: it.Name, FailedRule: RuleTaskFitVCPU,
				Needed: in.demand.MaxTaskVCPU, Available: float64(it.VCPU), Unit: UnitVCPU,
			})
			continue
		}
		usable := UsableMemGiB(it.MemoryMiB)
		if usable < in.demand.MaxTaskMemGiB {
			misses = append(misses, Miss{
				InstanceType: it.Name, FailedRule: RuleTaskFitMemory,
				Needed: in.demand.MaxTaskMemGiB, Available: usable, Unit: UnitGiB,
			})
			continue
		}

		// R9c -- the pool's CEILING can hold the fleet (C-8, D-8).
		//
		// This is the constraint that can genuinely fail. There is no rule
		// reading MinSize: h >= 1.0 was proved vacuous, because
		// n_floor = max(ceilToInt(n_exact), MinSize, 1) lets demand raise the
		// floor, so a too-small pinned min_size is unrepresentable. MaxSize is
		// a user-set ceiling that demand does NOT raise.
		if in.pool.MaxSize != nil {
			available := float64(*in.pool.MaxSize) * float64(k)
			if float64(*in.pool.MaxSize)*float64(k) < float64(in.demand.TaskCount) {
				misses = append(misses, Miss{
					InstanceType: it.Name, FailedRule: RuleMaxSizeTooSmall,
					Needed: float64(in.demand.TaskCount), Available: available, Unit: UnitTasks,
				})
				continue
			}
		}

		out = append(out, eligible{it: it, tasks: k, densityBasis: basis})
	}
	return out, misses
}
