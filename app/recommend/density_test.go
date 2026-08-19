package recommend

import "testing"

// TestTasksPerInstance_BridgeHasNoENICap is C-13, asserted rather than
// claimed. The architecture's own table:
//
//	type        | awsvpc (ENI-capped) | bridge | change
//	m7i.large   | 2                   | 3      | +50 %
//	r7i.large   | 2                   | 4      | +100 %
//	r7i.xlarge  | 3                   | 8      | +167 %
func TestTasksPerInstance_BridgeHasNoENICap(t *testing.T) {
	byName := map[string]InstanceType{}
	for _, it := range baseCatalog() {
		byName[it.Name] = it
	}
	const vcpuPerTask, memPerTask = 0.5, 2.0

	cases := []struct {
		name        string
		instance    string
		networkMode string
		trunking    bool
		wantTasks   int
		wantBasis   string
	}{
		{"m7i.large under bridge", "m7i.large", NetworkModeBridge, false, 3, DensityCPUMemoryOnly},
		{"m7i.large under awsvpc untrunked", "m7i.large", NetworkModeAWSVPC, false, 2, DensityMaxENIsMinus1},
		// The architecture's section 9 row expects 10 here, which the formula
		// in note 2 cannot produce: tasksPerInstance is
		// min(byCPU, byMem, TrunkedTaskLimit) = min(4, 3, 10) = 3. The
		// trunked limit does not bind on a .large at this task size; only the
		// BASIS changes. The formula is normative, so 3 is asserted and the
		// deviation is recorded in the implementation log.
		{"m7i.large under awsvpc trunked", "m7i.large", NetworkModeAWSVPC, true, 3, DensityTrunkedTable},
		{"r7i.large under bridge", "r7i.large", NetworkModeBridge, false, 4, DensityCPUMemoryOnly},
		{"r7i.large under awsvpc untrunked", "r7i.large", NetworkModeAWSVPC, false, 2, DensityMaxENIsMinus1},
		{"r7i.xlarge under bridge", "r7i.xlarge", NetworkModeBridge, false, 8, DensityCPUMemoryOnly},
		{"r7i.xlarge under awsvpc untrunked", "r7i.xlarge", NetworkModeAWSVPC, false, 3, DensityMaxENIsMinus1},
		// The trunked limit does bind once CPU and memory stop being the
		// constraint: an xlarge holds 20 trunked tasks, and 8 fit by shape.
		{"r7i.xlarge under awsvpc trunked", "r7i.xlarge", NetworkModeAWSVPC, true, 8, DensityTrunkedTable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotTasks, gotBasis := TasksPerInstance(byName[tc.instance], vcpuPerTask, memPerTask, tc.networkMode, tc.trunking)
			if gotTasks != tc.wantTasks {
				t.Errorf("tasks = %d, want %d", gotTasks, tc.wantTasks)
			}
			if gotBasis != tc.wantBasis {
				t.Errorf("basis = %q, want %q", gotBasis, tc.wantBasis)
			}
		})
	}
}

// TestTasksPerInstance_TrunkingIgnoredUnderBridge: under bridge there are no
// task ENIs to trunk, so the account setting cannot change the answer.
func TestTasksPerInstance_TrunkingIgnoredUnderBridge(t *testing.T) {
	for _, it := range baseCatalog() {
		off, offBasis := TasksPerInstance(it, 0.5, 2.0, NetworkModeBridge, false)
		on, onBasis := TasksPerInstance(it, 0.5, 2.0, NetworkModeBridge, true)
		if off != on || offBasis != onBasis {
			t.Errorf("%s: trunking changed the bridge answer: (%d,%q) -> (%d,%q)",
				it.Name, off, offBasis, on, onBasis)
		}
	}
}

// TestTasksPerInstance_InvalidDemandNeverDivides is note 2's zero-divisor
// branch and the second belt behind compute_signals.go's boundary guard.
func TestTasksPerInstance_InvalidDemandNeverDivides(t *testing.T) {
	it := InstanceType{Name: "m7i.large", VCPU: 2, MemoryMiB: 8192, MaxNetworkInterfaces: 3}
	cases := []struct {
		name         string
		vcpu, memGiB float64
	}{
		{"zero vcpu per task", 0, 2},
		{"zero memory per task", 0.5, 0},
		{"negative vcpu per task", -1, 2},
		{"both zero", 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			k, basis := TasksPerInstance(it, tc.vcpu, tc.memGiB, NetworkModeBridge, false)
			if k != 0 || basis != DensityInvalidDemand {
				t.Errorf("got (%d,%q), want (0,%q)", k, basis, DensityInvalidDemand)
			}
		})
	}
}

func TestTrunkedTaskLimit(t *testing.T) {
	cases := []struct {
		instance  string
		wantLimit int
		wantOK    bool
	}{
		{"m7i.medium", 4, true},
		{"m7i.large", 10, true},
		{"m7i.xlarge", 20, true},
		{"m7i.2xlarge", 40, true},
		{"c7g.large", 10, true},
		// Burstable families appear nowhere in the supported-instance-types
		// table, so they stay at maximumNetworkInterfaces - 1.
		{"t3.large", 0, false},
		{"t4g.medium", 0, false},
		// Documented individual and family exclusions.
		{"r5.8xlarge", 0, false},
		{"c5.metal", 0, false},
		{"g4dn.xlarge", 0, false},
		{"z1d.large", 0, false},
		// Sizes AWS has published no figure for stay uncapped by the table.
		{"m7i.8xlarge", 0, false},
		{"m7i.metal", 0, false},
		{"malformed", 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.instance, func(t *testing.T) {
			limit, ok := TrunkedTaskLimit(tc.instance)
			if limit != tc.wantLimit || ok != tc.wantOK {
				t.Errorf("TrunkedTaskLimit(%q) = (%d,%v), want (%d,%v)",
					tc.instance, limit, ok, tc.wantLimit, tc.wantOK)
			}
		})
	}
}

func TestParseFamilyGeneration(t *testing.T) {
	cases := []struct {
		instance   string
		wantFamily string
		wantGen    int
	}{
		{"m7i.large", "m", 7},
		{"m7i-flex.large", "m", 7},
		{"r7g.xlarge", "r", 7},
		{"c7a.2xlarge", "c", 7},
		{"t4g.small", "t", 4},
		{"inf1.xlarge", "inf", 1},
		{"g4dn.xlarge", "g", 4},
		{"m8g.large", "m", 8},
		{"u-6tb1.metal", "u", 0},
		{"nodot", "nodot", 0},
		{"", "", 0},
	}
	for _, tc := range cases {
		t.Run(tc.instance, func(t *testing.T) {
			family, gen := ParseFamilyGeneration(tc.instance)
			if family != tc.wantFamily || gen != tc.wantGen {
				t.Errorf("ParseFamilyGeneration(%q) = (%q,%d), want (%q,%d)",
					tc.instance, family, gen, tc.wantFamily, tc.wantGen)
			}
		})
	}
}

func TestUsableMemGiB(t *testing.T) {
	// FR-21.2's 15 % ECS agent + OS reserve, on the shapes every worked
	// example in note 4 uses.
	cases := []struct {
		memMiB int64
		want   float64
	}{
		{8192, 6.8},
		{16384, 13.6},
		{32768, 27.2},
	}
	for _, tc := range cases {
		if got := UsableMemGiB(tc.memMiB); got < tc.want-1e-9 || got > tc.want+1e-9 {
			t.Errorf("UsableMemGiB(%d) = %v, want %v", tc.memMiB, got, tc.want)
		}
	}
}
