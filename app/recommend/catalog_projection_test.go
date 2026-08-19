package recommend

import "testing"

// The DescribeInstanceTypes projection contract, pinned from the side that
// consumes it.
//
// N-5 / aws-verified-contracts.md section 1: AWS returns GpuInfo as **null**,
// not as an empty object, for roughly 870 of a region's 903 instance types. Nil
// safety is therefore the common path, and *it.GpuInfo.TotalGpuMemoryInMiB
// panics on m7i.large.
//
// app/compute_catalog.go owns the real projection; this package cannot import
// package main, so the shapes below stand in for the SDK records and the
// helper stands in for the projection. What the test actually pins is the
// contract app/recommend depends on: recommend.InstanceType.GPUCount must be a
// plain 0 for a nil GpuInfo, reached without dereferencing anything, and the
// classifier must then treat the type as non-GPU.

type sdkGPUDevice struct {
	Name      *string
	Count     *int32
	MemoryMiB *int32
}

type sdkGPUInfo struct {
	Gpus                []sdkGPUDevice
	TotalGpuMemoryInMiB *int32
}

type sdkInstanceTypeInfo struct {
	InstanceType         string
	VCPU                 int32
	MemoryMiB            int64
	Architectures        []string
	CurrentGeneration    bool
	BareMetal            bool
	Burstable            bool
	MaxNetworkInterfaces int32
	GpuInfo              *sdkGPUInfo // null for ~870 of 903 types
}

type projectedGPU struct {
	Count     int
	MemoryMiB *int32
	Name      *string
}

// projectGPU is the nil-safe projection app/compute_catalog.go must implement.
func projectGPU(rec sdkInstanceTypeInfo) projectedGPU {
	out := projectedGPU{}
	if rec.GpuInfo == nil {
		return out
	}
	out.MemoryMiB = rec.GpuInfo.TotalGpuMemoryInMiB
	for _, g := range rec.GpuInfo.Gpus {
		if g.Count != nil {
			out.Count += int(*g.Count)
		}
		if out.Name == nil && g.Name != nil {
			out.Name = g.Name
		}
	}
	return out
}

func projectInstanceType(rec sdkInstanceTypeInfo, onDemand *float64) InstanceType {
	gpu := projectGPU(rec)
	family, generation := ParseFamilyGeneration(rec.InstanceType)
	return InstanceType{
		Name:                 rec.InstanceType,
		Family:               family,
		Generation:           generation,
		VCPU:                 int(rec.VCPU),
		MemoryMiB:            rec.MemoryMiB,
		Architectures:        rec.Architectures,
		CurrentGeneration:    rec.CurrentGeneration,
		BareMetal:            rec.BareMetal,
		Burstable:            rec.Burstable,
		GPUCount:             gpu.Count,
		MaxNetworkInterfaces: int(rec.MaxNetworkInterfaces),
		OnDemandHourly:       onDemand,
	}
}

func TestComputeCatalog_ProjectsNilGpuInfo(t *testing.T) {
	name := "NVIDIA A10G"
	count := int32(1)
	deviceMem := int32(24576)
	totalMem := int32(24576)

	cases := []struct {
		name        string
		record      sdkInstanceTypeInfo
		wantCount   int
		wantMemNil  bool
		wantNameNil bool
	}{
		{
			name: "nil GpuInfo is the common path",
			record: sdkInstanceTypeInfo{
				InstanceType: "m7i.large", VCPU: 2, MemoryMiB: 8192,
				Architectures: []string{ArchX8664}, CurrentGeneration: true,
				MaxNetworkInterfaces: 3, GpuInfo: nil,
			},
			wantCount: 0, wantMemNil: true, wantNameNil: true,
		},
		{
			name: "an empty GpuInfo is not the same shape and must not panic either",
			record: sdkInstanceTypeInfo{
				InstanceType: "m7i-flex.large", VCPU: 2, MemoryMiB: 8192,
				Architectures: []string{ArchX8664}, CurrentGeneration: true,
				MaxNetworkInterfaces: 3, GpuInfo: &sdkGPUInfo{},
			},
			wantCount: 0, wantMemNil: true, wantNameNil: true,
		},
		{
			name: "a populated GpuInfo projects all three",
			record: sdkInstanceTypeInfo{
				InstanceType: "g5.xlarge", VCPU: 4, MemoryMiB: 16384,
				Architectures: []string{ArchX8664}, CurrentGeneration: true,
				MaxNetworkInterfaces: 4,
				GpuInfo: &sdkGPUInfo{
					TotalGpuMemoryInMiB: &totalMem,
					Gpus:                []sdkGPUDevice{{Name: &name, Count: &count, MemoryMiB: &deviceMem}},
				},
			},
			wantCount: 1, wantMemNil: false, wantNameNil: false,
		},
		{
			name: "a GPU device with a nil count contributes nothing",
			record: sdkInstanceTypeInfo{
				InstanceType: "g5.2xlarge", VCPU: 8, MemoryMiB: 32768,
				Architectures: []string{ArchX8664}, CurrentGeneration: true,
				MaxNetworkInterfaces: 4,
				GpuInfo:              &sdkGPUInfo{Gpus: []sdkGPUDevice{{Name: &name}}},
			},
			wantCount: 0, wantMemNil: true, wantNameNil: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := projectGPU(tc.record) // must not panic
			if got.Count != tc.wantCount {
				t.Errorf("gpuCount = %d, want %d", got.Count, tc.wantCount)
			}
			if (got.MemoryMiB == nil) != tc.wantMemNil {
				t.Errorf("gpuMemoryMiB nil = %v, want %v", got.MemoryMiB == nil, tc.wantMemNil)
			}
			if (got.Name == nil) != tc.wantNameNil {
				t.Errorf("gpuName nil = %v, want %v", got.Name == nil, tc.wantNameNil)
			}

			it := projectInstanceType(tc.record, fp(0.10))
			if it.GPUCount != tc.wantCount {
				t.Errorf("recommend.InstanceType.GPUCount = %d, want %d", it.GPUCount, tc.wantCount)
			}
		})
	}

	t.Run("a nil-GpuInfo type is recommendable and a GPU type is not", func(t *testing.T) {
		in := baseInput()
		in.Catalog = []InstanceType{
			projectInstanceType(sdkInstanceTypeInfo{
				InstanceType: "m7i.large", VCPU: 2, MemoryMiB: 8192,
				Architectures: []string{ArchX8664}, CurrentGeneration: true,
				MaxNetworkInterfaces: 3, GpuInfo: nil,
			}, fp(0.10)),
			projectInstanceType(sdkInstanceTypeInfo{
				InstanceType: "g5.xlarge", VCPU: 4, MemoryMiB: 16384,
				Architectures: []string{ArchX8664}, CurrentGeneration: true,
				MaxNetworkInterfaces: 4,
				GpuInfo: &sdkGPUInfo{
					TotalGpuMemoryInMiB: &totalMem,
					Gpus:                []sdkGPUDevice{{Name: &name, Count: &count}},
				},
			}, fp(1.00)),
		}
		res := Recommend(in)
		if res.Primary == nil {
			t.Fatal("no primary")
		}
		if res.Primary.InstanceType != "m7i.large" {
			t.Errorf("primary = %s, want m7i.large", res.Primary.InstanceType)
		}
		for _, c := range res.Ranked {
			if c.InstanceType == "g5.xlarge" {
				t.Error("an idle GPU was recommended")
			}
		}
	})
}
