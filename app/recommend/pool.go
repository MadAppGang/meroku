package recommend

import "sort"

// FR-24/26/28 SuggestedPool synthesis, including network_mode and the DERIVED
// ami_family (C-11).

const (
	// defaultRootVolumeGB matches variables.tf's optional default.
	defaultRootVolumeGB = 30
	// siblingTolerance is FR-26's +/-25 % window on vCPU and memory for the
	// two companion instance types.
	siblingTolerance = 0.25
	// maxSuggestedTypes is FR-26's cap: primary plus up to two more. Fewer
	// qualifying types yields a shorter list, never a padded one.
	maxSuggestedTypes = 3
)

// zeroConfigTypes is the plan doc's zero-config default (FR-28), intersected
// with the region's catalog before it is returned.
var zeroConfigTypes = []string{"m7i-flex.large", "m6i.large", "m6a.large"}

// poolContext is what SuggestedPool synthesis reads beyond the ranked list.
type poolContext struct {
	pool       PoolConstraints
	params     PostureParams
	class      string
	demand     Demand
	peakVCPU   float64
	peakMemGiB float64
}

// poolName is the name the suggestion carries. An existing pool keeps its own.
func poolName(pool PoolConstraints, fallback string) string {
	if pool.Name != "" {
		return pool.Name
	}
	return fallback
}

// deriveAMIFamily is C-11's third change: ami_family is DERIVED from the
// chosen types, never echoed from the input. FR-26 let those two fields
// disagree, and an ASG whose AMI architecture disagrees with its instance types
// fails every launch with a message visible only in scaling activities --
// terraform apply reports success and capacity never appears.
func deriveAMIFamily(types []RankItem, class string) string {
	if len(types) > 0 {
		allARM := true
		for _, t := range types {
			if t.Architecture != ArchARM64 {
				allARM = false
				break
			}
		}
		if allARM {
			return AMIFamilyAL2023ARM64
		}
	}
	if class == ClassGPU {
		return AMIFamilyAL2023GPU
	}
	return AMIFamilyAL2023
}

// withinTolerance reports whether v sits inside +/-tolerance of base.
func withinTolerance(v, base, tolerance float64) bool {
	if !(base > 0) {
		return false
	}
	d, ok := ratio(v-base, base)
	if !ok {
		return false
	}
	if d < 0 {
		d = -d
	}
	return d <= tolerance
}

// selectTypes is FR-26: the primary plus up to two further candidates that
// share the primary's architecture and sit within +/-25 % of its vCPU and
// memory. The ranked order decides which two, so the result is deterministic.
func selectTypes(ranked []RankItem) []RankItem {
	if len(ranked) == 0 {
		return nil
	}
	primary := ranked[0]
	out := []RankItem{primary}
	for _, c := range ranked[1:] {
		if len(out) >= maxSuggestedTypes {
			break
		}
		if c.Architecture != primary.Architecture {
			continue
		}
		if !withinTolerance(float64(c.VCPU), float64(primary.VCPU), siblingTolerance) {
			continue
		}
		if !withinTolerance(float64(c.MemoryMiB), float64(primary.MemoryMiB), siblingTolerance) {
			continue
		}
		out = append(out, c)
	}
	return out
}

// buildSuggestedPool is FR-24/26 for a real recommendation.
func buildSuggestedPool(ranked []RankItem, ctx poolContext) SuggestedPool {
	selected := selectTypes(ranked)
	names := make([]string, 0, len(selected))
	for _, s := range selected {
		names = append(names, s.InstanceType)
	}

	sp := SuggestedPool{
		Name:           poolName(ctx.pool, "general"),
		InstanceTypes:  names,
		CapacityType:   ctx.params.CapacityType,
		OnDemandBase:   ctx.params.OnDemandBase,
		TargetCapacity: ctx.params.TargetCapacity,
		NetworkMode:    ctx.pool.NetworkMode,
		AMIFamily:      deriveAMIFamily(selected, ctx.class),
		RootVolumeGB:   defaultRootVolumeGB,
	}
	if sp.CapacityType != CapacitySpotWithBase {
		sp.OnDemandBase = 0
	}

	primary := ranked[0]

	// FR-24's min_size. DEV-26: cost-first derives it from demand exactly as
	// balanced does, instead of FR-24's literal 1.
	//
	// primary.InstancesAtFloor already carries the placement floor as well as
	// the resource one -- see instancesAtFloor in score.go -- so a pool floor
	// that cannot hold the configured fleet is unrepresentable here.
	minSize := primary.InstancesAtFloor
	if ctx.params.MinSizeBasis == MinSizePeak {
		nPeak := exactInstances(ctx.peakVCPU, ctx.peakMemGiB, primary.VCPU, UsableMemGiB(primary.MemoryMiB))
		minSize = max(minSize, instancesAtFloor(nPeak, ctx.demand.TaskCount, primary.TasksPerInstance, ctx.pool.MinSize))
	}
	sp.MinSize = max(1, minSize)

	// FR-24's max_size, with a floor at whatever it takes to hold the fleet --
	// suggesting a ceiling that rule R9c would then reject is not a
	// suggestion.
	maxSize := ceilToInt(float64(sp.MinSize) * ctx.params.MaxSizeFactor)
	if n := FleetInstances(ctx.demand.TaskCount, primary.TasksPerInstance); n > maxSize {
		maxSize = n
	}
	sp.MaxSize = max(sp.MinSize, maxSize)

	// EC-6: spot was asked for, spot has no usable median for the primary
	// type, so the pool falls back to on-demand and says so.
	if !primary.SpotUsable && (sp.CapacityType == CapacitySpot || sp.CapacityType == CapacitySpotWithBase) {
		sp.CapacityType = CapacityOnDemand
		sp.OnDemandBase = 0
		sp.Downgraded = true
	}
	return sp
}

// defaultSuggestedPool is FR-28: zero configured services and zero deployed
// services means there is no demand vector at all, so the answer is the
// zero-config default intersected with the region's catalog (FR-20). No
// instance type is ever suggested that the region does not report.
func defaultSuggestedPool(catalog []InstanceType, pool PoolConstraints, gpuRequested bool) SuggestedPool {
	present := make(map[string]bool, len(catalog))
	for _, it := range catalog {
		present[it.Name] = true
	}
	var names []string
	for _, want := range zeroConfigTypes {
		if present[want] {
			names = append(names, want)
		}
	}
	if len(names) == 0 {
		names = fallbackDefaultTypes(catalog, pool, gpuRequested)
	}

	family := AMIFamilyAL2023
	if pool.AMIFamily == AMIFamilyAL2023ARM64 || pool.AMIFamily == AMIFamilyAL2023GPU {
		family = pool.AMIFamily
	} else if gpuRequested {
		family = AMIFamilyAL2023GPU
	}

	return SuggestedPool{
		Name:           poolName(pool, "default"),
		InstanceTypes:  names,
		CapacityType:   CapacitySpotWithBase,
		OnDemandBase:   1,
		MinSize:        1,
		MaxSize:        6,
		TargetCapacity: 100,
		NetworkMode:    pool.NetworkMode,
		AMIFamily:      family,
		RootVolumeGB:   defaultRootVolumeGB,
	}
}

// fallbackDefaultTypes runs when the region carries none of the three
// zero-config types. It draws from the catalog the caller passed, honouring
// the same architecture and GPU rules the recommendation would, so the pool it
// writes can actually launch. Ordering is lexicographic, so it is
// deterministic.
func fallbackDefaultTypes(catalog []InstanceType, pool PoolConstraints, gpuRequested bool) []string {
	survivors, _ := preFilter(catalog, pool, gpuRequested)
	names := make([]string, 0, len(survivors))
	for _, it := range survivors {
		if it.Burstable {
			continue
		}
		names = append(names, it.Name)
	}
	if len(names) == 0 {
		for _, it := range survivors {
			names = append(names, it.Name)
		}
	}
	sort.Strings(names)
	if len(names) > maxSuggestedTypes {
		names = names[:maxSuggestedTypes]
	}
	return names
}
