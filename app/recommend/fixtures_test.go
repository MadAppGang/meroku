package recommend

// Synthetic fixtures. CON-5: this repository is public, so no value here comes
// from a live account. Instance-type names and their vCPU/memory shapes are
// public catalog facts; every price is an obviously round invented number, and
// r7i-wide.xlarge is an invented record that exists only to give the catalog a
// 16.0 GiB/vCPU ceiling for the C-10 clamp test.

func fp(v float64) *float64 { return &v }
func ip(v int) *int         { return &v }

// baseCatalog is the everyday fixture: eight x86 candidates across the c, m
// and r families, two Graviton types, two burstable types, one GPU type, one
// bare-metal type and one previous-generation type.
//
// Ratios over the al2023 (x86) pre-filter survivors run from 2.0 (c-family) to
// 8.0 (r-family), so R_eff clamps to [2.0, 8.0] on this catalog.
func baseCatalog() []InstanceType {
	return []InstanceType{
		{
			Name: "c7i.large", VCPU: 2, MemoryMiB: 4096,
			Architectures: []string{ArchX8664}, CurrentGeneration: true,
			MaxNetworkInterfaces: 3, SupportsSpot: true,
			OnDemandHourly: fp(0.0900), SpotMedianHourly: fp(0.0400),
		},
		{
			Name: "c7i.xlarge", VCPU: 4, MemoryMiB: 8192,
			Architectures: []string{ArchX8664}, CurrentGeneration: true,
			MaxNetworkInterfaces: 4, SupportsSpot: true,
			OnDemandHourly: fp(0.1800), SpotMedianHourly: fp(0.0800),
		},
		{
			Name: "m7i.large", VCPU: 2, MemoryMiB: 8192,
			Architectures: []string{ArchX8664}, CurrentGeneration: true,
			MaxNetworkInterfaces: 3, SupportsSpot: true,
			OnDemandHourly: fp(0.1000), SpotMedianHourly: fp(0.0400),
		},
		{
			Name: "m7i.xlarge", VCPU: 4, MemoryMiB: 16384,
			Architectures: []string{ArchX8664}, CurrentGeneration: true,
			MaxNetworkInterfaces: 4, SupportsSpot: true,
			OnDemandHourly: fp(0.2000), SpotMedianHourly: fp(0.0800),
		},
		{
			Name: "m6i.large", VCPU: 2, MemoryMiB: 8192,
			Architectures: []string{ArchX8664}, CurrentGeneration: true,
			MaxNetworkInterfaces: 3, SupportsSpot: true,
			OnDemandHourly: fp(0.0960), SpotMedianHourly: fp(0.0380),
		},
		{
			Name: "r7i.large", VCPU: 2, MemoryMiB: 16384,
			Architectures: []string{ArchX8664}, CurrentGeneration: true,
			MaxNetworkInterfaces: 3, SupportsSpot: true,
			OnDemandHourly: fp(0.1400), SpotMedianHourly: fp(0.0500),
		},
		{
			Name: "r7i.xlarge", VCPU: 4, MemoryMiB: 32768,
			Architectures: []string{ArchX8664}, CurrentGeneration: true,
			MaxNetworkInterfaces: 4, SupportsSpot: true,
			OnDemandHourly: fp(0.2800), SpotMedianHourly: fp(0.1000),
		},
		{
			Name: "r6i.large", VCPU: 2, MemoryMiB: 16384,
			Architectures: []string{ArchX8664}, CurrentGeneration: true,
			MaxNetworkInterfaces: 3, SupportsSpot: true,
			OnDemandHourly: fp(0.1340), SpotMedianHourly: fp(0.0480),
		},
		{
			Name: "m7g.large", VCPU: 2, MemoryMiB: 8192,
			Architectures: []string{ArchARM64}, CurrentGeneration: true,
			MaxNetworkInterfaces: 3, SupportsSpot: true,
			OnDemandHourly: fp(0.0850), SpotMedianHourly: fp(0.0350),
		},
		{
			Name: "r7g.large", VCPU: 2, MemoryMiB: 16384,
			Architectures: []string{ArchARM64}, CurrentGeneration: true,
			MaxNetworkInterfaces: 3, SupportsSpot: true,
			OnDemandHourly: fp(0.1200), SpotMedianHourly: fp(0.0450),
		},
		{
			Name: "t3.medium", VCPU: 2, MemoryMiB: 4096,
			Architectures: []string{ArchX8664}, CurrentGeneration: true,
			Burstable: true, MaxNetworkInterfaces: 3, SupportsSpot: true,
			OnDemandHourly: fp(0.0500), SpotMedianHourly: fp(0.0150),
		},
		{
			Name: "t4g.medium", VCPU: 2, MemoryMiB: 4096,
			Architectures: []string{ArchARM64}, CurrentGeneration: true,
			Burstable: true, MaxNetworkInterfaces: 3, SupportsSpot: true,
			OnDemandHourly: fp(0.0400), SpotMedianHourly: fp(0.0120),
		},
		{
			Name: "g5.xlarge", VCPU: 4, MemoryMiB: 16384,
			Architectures: []string{ArchX8664}, CurrentGeneration: true,
			GPUCount: 1, MaxNetworkInterfaces: 4, SupportsSpot: true,
			OnDemandHourly: fp(1.0000), SpotMedianHourly: fp(0.4000),
		},
		{
			Name: "m5.metal", VCPU: 96, MemoryMiB: 393216,
			Architectures: []string{ArchX8664}, CurrentGeneration: true,
			BareMetal: true, MaxNetworkInterfaces: 15,
			OnDemandHourly: fp(5.0000),
		},
		{
			Name: "m4.large", VCPU: 2, MemoryMiB: 8192,
			Architectures: []string{ArchX8664}, CurrentGeneration: false,
			MaxNetworkInterfaces: 3, OnDemandHourly: fp(0.1100),
		},
	}
}

// wideCatalog adds an invented 16.0 GiB/vCPU record, so that the catalog's
// ratio ceiling is 16.0 and C-10's clamp has something to clamp to.
//
// The record is named r7i-wide.xlarge and not x2i.xlarge because rule R10's
// family allowlist runs in preFilter, AHEAD of the ratio range in pipeline
// step 3. An x-family record would be excluded before the range was taken, so
// the ceiling it exists to provide would never reach the clamp -- which is
// exactly the property R10's placement is supposed to have, and is asserted
// directly in family_test.go. The r prefix keeps it inside the allowlist; the
// "-wide" suffix keeps it obviously invented, in the shape AWS itself uses for
// m7i-flex.
func wideCatalog() []InstanceType {
	return append(baseCatalog(), InstanceType{
		Name: "r7i-wide.xlarge", VCPU: 4, MemoryMiB: 65536,
		Architectures: []string{ArchX8664}, CurrentGeneration: true,
		MaxNetworkInterfaces: 4, SupportsSpot: true,
		OnDemandHourly: fp(1.2000), SpotMedianHourly: fp(0.5000),
	})
}

// uniformServices is the internal reviewer's workload, and the one every
// worked example in architecture.md 4.5 is computed against: six tasks of
// 0.5 vCPU / 2 GiB. T = 6, ConfiguredVCPU = 3.0, ConfiguredMemGiB = 12.0,
// R_cfg = 4.0.
func uniformServices() []ServiceDemand {
	return []ServiceDemand{{
		Name: "backend", VCPU: 0.5, MemGiB: 2, Count: 6,
	}}
}

// measuredServices is the same fleet with CloudWatch data: 15 % CPU peak,
// 80 % memory peak, a full 14-day window.
func measuredServices() []ServiceDemand {
	return []ServiceDemand{{
		Name: "backend", VCPU: 0.5, MemGiB: 2, Count: 6,
		CPUAvg: 8, CPUPeak: 15, MemAvg: 61, MemPeak: 80, Datapoints: 336,
	}}
}

// baseInput is the everyday recommendation request: the uniform workload
// against the base catalog, bridge networking, no pinned pool bounds.
func baseInput() Input {
	return Input{
		Region:   "ap-southeast-2",
		Catalog:  baseCatalog(),
		Services: uniformServices(),
		Pool:     PoolConstraints{Name: "general"},
		Posture:  PostureBalanced,
		Limit:    5,
	}
}

// baseDemand is ConfiguredShape over uniformServices, precomputed for the
// filter and density tests.
func baseDemand() Demand { return ConfiguredShape(uniformServices()) }

// mixedServices has a small mean per task and one very fat task, which is the
// only shape that reaches rule R2: R9a runs first and would otherwise exclude
// on density before the per-task fit is ever tested.
func mixedServices(fatVCPU, fatMemGiB float64) []ServiceDemand {
	return []ServiceDemand{
		{Name: "worker", VCPU: 0.25, MemGiB: 0.5, Count: 20},
		{Name: "fat", VCPU: fatVCPU, MemGiB: fatMemGiB, Count: 1},
	}
}
