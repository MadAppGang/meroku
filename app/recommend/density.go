package recommend

import "strings"

// Task density, and the ENI cap that only applies to awsvpc (C-13, D-6).
//
// Under awsvpc, maximumNetworkInterfaces - 1 is 2 for every .large type, which
// is what made fit and waste structurally oppose each other: a memory_heavy
// classification steers toward memory-rich instances, and waste then penalises
// the memory the cap itself made unusable. Under bridge that cap is gone --
// tasks share the instance's primary interface -- and an m7i.large holds as
// many tasks as fit in CPU and memory.
//
// The trunking table and the ENI cap are both retained verbatim, scoped to
// awsvpc pools.

// trunkedTaskLimits is the documented ECS task limit per container instance
// with the awsvpcTrunking account setting enabled, keyed by the size token.
// Consistent across m7i m7a m7g m8g c7i c7a c7g c8g r7i r7a r7g r8g.
//
// Only these four sizes are documented, so any other size reports ok=false and
// falls back to maximumNetworkInterfaces - 1. Under-promising density is the
// safe direction.
var trunkedTaskLimits = map[string]int{
	"medium":  4,
	"large":   10,
	"xlarge":  20,
	"2xlarge": 40,
}

// trunkingExcludedTypes are individually named as unsupported by AWS even
// though their family is otherwise supported.
var trunkingExcludedTypes = map[string]bool{
	"a1.metal":      true,
	"c5.metal":      true,
	"c5a.8xlarge":   true,
	"c5ad.8xlarge":  true,
	"c5d.metal":     true,
	"m5.metal":      true,
	"p3dn.24xlarge": true,
	"r5.metal":      true,
	"r5.8xlarge":    true,
	"r5d.metal":     true,
}

// trunkingExcludedFamilies are whole instance families AWS documents as
// unsupported for trunking.
var trunkingExcludedFamilies = map[string]bool{
	"c5n": true, "d3": true, "d3en": true, "g3": true, "g3s": true,
	"g4dn": true, "i3": true, "i3en": true, "inf1": true, "m5dn": true,
	"m5n": true, "m5zn": true, "mac1": true, "r5b": true, "r5n": true,
	"r5dn": true, "u-12tb1": true, "u-6tb1": true, "u-9tb1": true,
	"z1d": true,
}

// TrunkedTaskLimit reports the documented trunked task limit for an instance
// type, and whether trunking applies to it at all.
//
// ok is false for every t* type -- burstable families appear nowhere in the
// supported-instance-types table -- for the documented exclusion list, and for
// any size AWS has not published a figure for.
func TrunkedTaskLimit(instanceType string) (int, bool) {
	prefix, size, found := strings.Cut(instanceType, ".")
	if !found || prefix == "" || size == "" {
		return 0, false
	}
	if trunkingExcludedTypes[instanceType] || trunkingExcludedFamilies[prefix] {
		return 0, false
	}
	if family, _ := ParseFamilyGeneration(instanceType); family == "t" {
		return 0, false
	}
	limit, ok := trunkedTaskLimits[size]
	if !ok {
		return 0, false
	}
	return limit, true
}

// ParseFamilyGeneration splits an instance type name into its family prefix
// and its generation digit: "m7i.large" -> ("m", 7), "inf1.xlarge" ->
// ("inf", 1), "u-6tb1.metal" -> ("u", 0) because no digit follows the letters.
//
// It never fails: an unparseable generation is 0, which modernity scores 0 and
// which no rule excludes on.
func ParseFamilyGeneration(instanceType string) (string, int) {
	prefix, _, _ := strings.Cut(instanceType, ".")
	i := 0
	for i < len(prefix) && prefix[i] >= 'a' && prefix[i] <= 'z' {
		i++
	}
	family := prefix[:i]
	gen := 0
	digits := 0
	for i < len(prefix) && prefix[i] >= '0' && prefix[i] <= '9' {
		gen = gen*10 + int(prefix[i]-'0')
		digits++
		i++
	}
	if digits == 0 {
		return family, 0
	}
	return family, gen
}

// UsableMemGiB is the memory a task may actually be scheduled against:
// FR-21.2's 15 % ECS agent + OS reserve.
func UsableMemGiB(memoryMiB int64) float64 {
	return (float64(memoryMiB) / 1024.0) * usableMemoryFactor // constant divisor
}

// TasksPerInstance is how many tasks of the demand-weighted mean shape fit on
// one instance, and which rule decided it.
//
// vcpuPerTask and memGiBPerTask are FR-22's demand-weighted means. If either is
// <= 0 the function returns (0, "invalid_demand") WITHOUT dividing, and rule
// R9a excludes the candidate -- the second belt behind compute_signals.go's
// boundary guard (C-9).
func TasksPerInstance(it InstanceType, vcpuPerTask, memGiBPerTask float64, networkMode string, trunkingEnabled bool) (int, string) {
	if !(vcpuPerTask > 0) || !(memGiBPerTask > 0) || !isFinite(vcpuPerTask) || !isFinite(memGiBPerTask) {
		return 0, DensityInvalidDemand
	}
	if it.VCPU <= 0 || it.MemoryMiB <= 0 {
		return 0, DensityInvalidDemand
	}

	byCPUf, okCPU := ratio(float64(it.VCPU), vcpuPerTask)
	byMemf, okMem := ratio(UsableMemGiB(it.MemoryMiB), memGiBPerTask)
	if !okCPU || !okMem {
		return 0, DensityInvalidDemand
	}
	byCPU := floorToInt(byCPUf)
	byMem := floorToInt(byMemf)
	k := min(byCPU, byMem)

	if networkMode != NetworkModeAWSVPC {
		// bridge -- THE DEFAULT (D-6). No task ENIs, so no cap.
		return k, DensityCPUMemoryOnly
	}

	if trunkingEnabled {
		if limit, ok := TrunkedTaskLimit(it.Name); ok {
			return min(k, limit), DensityTrunkedTable
		}
	}
	// One ENI is the host's primary interface; every task consumes another.
	return min(k, it.MaxNetworkInterfaces-1), DensityMaxENIsMinus1
}

// densityBasisFor reports the basis TasksPerInstance would use for a pool,
// independent of any particular candidate. It is what Signals.DensityBasis
// reads when there is no primary candidate to read it from.
func densityBasisFor(networkMode string, trunkingEnabled bool) string {
	if networkMode != NetworkModeAWSVPC {
		return DensityCPUMemoryOnly
	}
	if trunkingEnabled {
		return DensityTrunkedTable
	}
	return DensityMaxENIsMinus1
}
