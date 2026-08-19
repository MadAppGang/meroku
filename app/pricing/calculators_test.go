package pricing

import (
	"testing"
	"time"
)

// getTestRates returns a PriceRates object with known test values
// Used for consistent testing across all calculator tests
func getTestRates() *PriceRates {
	return &PriceRates{
		Region:     "us-east-1",
		LastUpdate: time.Now(),
		Source:     "test",
		RDS: map[string]float64{
			"db.t4g.micro":  0.016,
			"db.t4g.small":  0.032,
			"db.t4g.medium": 0.065,
		},
		Aurora: AuroraPricing{
			ACUHourly:      0.12,
			StorageGBMonth: 0.10,
		},
		Fargate: FargatePricing{
			VCPUHourly:     0.04048,
			MemoryGBHourly: 0.004445,
		},
		Storage: StoragePricing{
			GP3PerGBMonth: 0.115,
		},
		S3: S3Pricing{
			StandardPerGBMonth: 0.023,
			RequestsPer1000:    0.0004,
		},
		CloudWatch: CloudWatchPricing{
			LogsIngestionPerGB: 0.50,
		},
	}
}

// TestCalculateRDSPrice tests RDS pricing calculations
func TestCalculateRDSPrice(t *testing.T) {
	rates := getTestRates()

	tests := []struct {
		name     string
		config   RDSConfig
		expected float64
	}{
		{
			name: "Basic t4g.micro single-AZ with 20GB storage",
			config: RDSConfig{
				InstanceClass:    "db.t4g.micro",
				AllocatedStorage: 20,
				MultiAZ:          false,
			},
			// Expected: (0.016 * 730) + (20 * 0.115) = 11.68 + 2.30 = 13.98
			expected: 13.98,
		},
		{
			name: "t4g.small multi-AZ with 100GB storage",
			config: RDSConfig{
				InstanceClass:    "db.t4g.small",
				AllocatedStorage: 100,
				MultiAZ:          true,
			},
			// Expected: (0.032 * 2 * 730) + (100 * 0.115) = 46.72 + 11.50 = 58.22
			expected: 58.22,
		},
		{
			name: "Unknown instance type falls back to t4g.micro",
			config: RDSConfig{
				InstanceClass:    "db.unknown.type",
				AllocatedStorage: 20,
				MultiAZ:          false,
			},
			// Should fallback to db.t4g.micro pricing
			expected: 13.98,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateRDSPrice(tt.config, rates)
			if !floatEquals(result, tt.expected, 0.01) {
				t.Errorf("CalculateRDSPrice() = %.2f, expected %.2f", result, tt.expected)
			}
		})
	}
}

// TestCalculateAuroraPrice tests Aurora Serverless v2 pricing calculations
// These calculations must match the frontend exactly
func TestCalculateAuroraPrice(t *testing.T) {
	rates := getTestRates()

	tests := []struct {
		name     string
		config   AuroraConfig
		expected float64
	}{
		{
			name: "Startup level: min=0, max=1 (can pause)",
			config: AuroraConfig{
				MinCapacity: 0,
				MaxCapacity: 1,
				Level:       "startup",
			},
			// avgACU = 0 + (1-0)*0.20 = 0.20
			// With pause time (75% active): 0.20 * 0.75 = 0.15
			// Monthly: 0.15 * 0.12 * 730 = 13.14
			expected: 13.14,
		},
		{
			name: "Startup level: min=0.5, max=1",
			config: AuroraConfig{
				MinCapacity: 0, // Will be treated as float 0.5 in real usage
				MaxCapacity: 1,
				Level:       "startup",
			},
			// This test uses MinCapacity=0 as int, so:
			// avgACU = 0 + (1-0)*0.20 * 0.75 = 0.15
			expected: 13.14,
		},
		{
			name: "Scaleup level: min=1, max=4",
			config: AuroraConfig{
				MinCapacity: 1,
				MaxCapacity: 4,
				Level:       "scaleup",
			},
			// avgACU = 1 + (4-1)*0.50 = 2.5
			// No pause time (min > 0): 2.5
			// Monthly: 2.5 * 0.12 * 730 = 219.00
			expected: 219.00,
		},
		{
			name: "Highload level: min=2, max=16",
			config: AuroraConfig{
				MinCapacity: 2,
				MaxCapacity: 16,
				Level:       "highload",
			},
			// avgACU = 2 + (16-2)*0.80 = 13.2
			// Monthly: 13.2 * 0.12 * 730 = 1156.32
			expected: 1156.32,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateAuroraPrice(tt.config, rates)
			if !floatEquals(result, tt.expected, 0.01) {
				t.Errorf("CalculateAuroraPrice() = %.2f, expected %.2f", result, tt.expected)
			}
		})
	}
}

// TestCalculateECSPrice tests ECS Fargate pricing calculations
func TestCalculateECSPrice(t *testing.T) {
	rates := getTestRates()

	tests := []struct {
		name     string
		config   ECSConfig
		expected float64
	}{
		{
			name: "Single task: 256 CPU, 512MB memory",
			config: ECSConfig{
				CPU:          256,
				Memory:       512,
				DesiredCount: 1,
			},
			// vCPU = 256/1024 = 0.25
			// Memory = 512/1024 = 0.5 GB
			// Hourly: (0.25 * 0.04048) + (0.5 * 0.004445) = 0.01012 + 0.0022225 = 0.0123425
			// Monthly: 0.0123425 * 730 = 9.01
			expected: 9.01,
		},
		{
			name: "Two tasks: 512 CPU, 1024MB memory",
			config: ECSConfig{
				CPU:          512,
				Memory:       1024,
				DesiredCount: 2,
			},
			// vCPU = 512/1024 = 0.5
			// Memory = 1024/1024 = 1.0 GB
			// Hourly per task: (0.5 * 0.04048) + (1.0 * 0.004445) = 0.02024 + 0.004445 = 0.024685
			// Total hourly: 0.024685 * 2 = 0.04937
			// Monthly: 0.04937 * 730 = 36.04
			expected: 36.04,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateECSPrice(tt.config, rates)
			if !floatEquals(result, tt.expected, 0.01) {
				t.Errorf("CalculateECSPrice() = %.2f, expected %.2f", result, tt.expected)
			}
		})
	}
}

// TestCalculateS3Price tests S3 pricing calculations
func TestCalculateS3Price(t *testing.T) {
	rates := getTestRates()

	tests := []struct {
		name     string
		config   S3Config
		expected float64
	}{
		{
			name: "10GB storage, 1000 requests/day",
			config: S3Config{
				StorageGB:      10,
				RequestsPerDay: 1000,
			},
			// Storage: 10 * 0.023 = 0.23
			// Requests: (1000 * 30 / 1000) * 0.0004 = 30 * 0.0004 = 0.012
			// Total: 0.23 + 0.012 = 0.242
			expected: 0.242,
		},
		{
			name: "100GB storage, 10000 requests/day",
			config: S3Config{
				StorageGB:      100,
				RequestsPerDay: 10000,
			},
			// Storage: 100 * 0.023 = 2.30
			// Requests: (10000 * 30 / 1000) * 0.0004 = 300 * 0.0004 = 0.12
			// Total: 2.30 + 0.12 = 2.42
			expected: 2.42,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateS3Price(tt.config, rates)
			if !floatEquals(result, tt.expected, 0.01) {
				t.Errorf("CalculateS3Price() = %.2f, expected %.2f", result, tt.expected)
			}
		})
	}
}

// TestCalculateAverageACU tests the ACU calculation logic
// This is critical to match between backend and frontend
func TestCalculateAverageACU(t *testing.T) {
	tests := []struct {
		name     string
		config   AuroraConfig
		expected float64
	}{
		{
			name: "Startup: 0-1 ACU (can pause)",
			config: AuroraConfig{
				MinCapacity: 0,
				MaxCapacity: 1,
				Level:       "startup",
			},
			// 0 + (1-0)*0.20 * 0.75 = 0.15
			expected: 0.15,
		},
		{
			name: "Scaleup: 1-4 ACU (always on)",
			config: AuroraConfig{
				MinCapacity: 1,
				MaxCapacity: 4,
				Level:       "scaleup",
			},
			// 1 + (4-1)*0.50 = 2.5
			expected: 2.5,
		},
		{
			name: "Highload: 2-16 ACU (always on)",
			config: AuroraConfig{
				MinCapacity: 2,
				MaxCapacity: 16,
				Level:       "highload",
			},
			// 2 + (16-2)*0.80 = 13.2
			expected: 13.2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculateAverageACU(tt.config)
			if !floatEquals(result, tt.expected, 0.001) {
				t.Errorf("calculateAverageACU() = %.3f, expected %.3f", result, tt.expected)
			}
		})
	}
}

// floatEquals checks if two floats are equal within a tolerance
func floatEquals(a, b, tolerance float64) bool {
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff < tolerance
}

// getTestEC2Rates returns rates carrying an EC2 table with round numbers, so a
// wrong formula shows up as a wrong digit rather than a rounding argument.
//
// It is separate from getTestRates rather than an addition to it: every
// existing calculator test asserts against that fixture's exact values, and the
// EC2 shape has no business appearing in their failure output.
func getTestEC2Rates() *PriceRates {
	rates := getTestRates()
	rates.EC2 = EC2Pricing{
		OnDemandHourly: map[string]float64{
			"m6i.large":  0.10,
			"m6i.xlarge": 0.20,
		},
		SpotRatio: 0.40,
	}
	return rates
}

// TestCalculateEC2PoolPrice covers the claim the whole EC2 pricing shape rests
// on: a pool costs instances x instance-hourly x 730, and that figure does not
// move when tasks are placed on it (AC-53).
func TestCalculateEC2PoolPrice(t *testing.T) {
	rates := getTestEC2Rates()

	tests := []struct {
		name        string
		config      EC2PoolConfig
		want        float64
		wantUnknown bool // the pool has no price at all, which is not $0
	}{
		{
			name: "on-demand pool: instances x hourly x 730",
			config: EC2PoolConfig{
				InstanceTypes: []string{"m6i.large"},
				InstanceCount: 2,
				CapacityType:  "on_demand",
			},
			want: 2 * 0.10 * HoursPerMonth,
		},
		{
			name: "capacity type absent reads as on-demand",
			config: EC2PoolConfig{
				InstanceTypes: []string{"m6i.large"},
				InstanceCount: 1,
			},
			want: 0.10 * HoursPerMonth,
		},
		{
			name: "an unrecognised capacity type is priced as on-demand, never cheaper",
			config: EC2PoolConfig{
				InstanceTypes: []string{"m6i.large"},
				InstanceCount: 3,
				CapacityType:  "sport",
			},
			want: 3 * 0.10 * HoursPerMonth,
		},
		{
			name: "all-spot pool pays SpotRatio of on-demand",
			config: EC2PoolConfig{
				InstanceTypes: []string{"m6i.large"},
				InstanceCount: 4,
				CapacityType:  "spot",
			},
			want: 4 * 0.10 * 0.40 * HoursPerMonth,
		},
		{
			name: "spot_with_base bills the base on demand and the rest spot",
			config: EC2PoolConfig{
				InstanceTypes: []string{"m6i.large"},
				InstanceCount: 5,
				CapacityType:  "spot_with_base",
				OnDemandBase:  2,
			},
			want: (2*0.10 + 3*0.10*0.40) * HoursPerMonth,
		},
		{
			name: "a base above the instance count never bills more than the fleet",
			config: EC2PoolConfig{
				InstanceTypes: []string{"m6i.large"},
				InstanceCount: 2,
				CapacityType:  "spot_with_base",
				OnDemandBase:  9,
			},
			want: 2 * 0.10 * HoursPerMonth,
		},
		{
			name: "the basis is the first PRICED type, not the first type",
			config: EC2PoolConfig{
				InstanceTypes: []string{"m9z.nonexistent", "m6i.xlarge", "m6i.large"},
				InstanceCount: 1,
				CapacityType:  "on_demand",
			},
			want: 0.20 * HoursPerMonth,
		},
		{
			name: "a pool scaled to zero costs nothing",
			config: EC2PoolConfig{
				InstanceTypes: []string{"m6i.large"},
				InstanceCount: 0,
				CapacityType:  "on_demand",
			},
			want: 0,
		},
		{
			// The case the single float return could not express, and the
			// reason for the second one: this pool costs $0 in exactly the
			// same way the row above does, and it must not. Nobody has a
			// price for the instance, so the answer is "unknown" -- the C-4
			// rule, and what the TypeScript twin says with `null`.
			name: "a pool of unpriced instances is unknown, not free",
			config: EC2PoolConfig{
				InstanceTypes: []string{"m9z.nonexistent"},
				InstanceCount: 3,
				CapacityType:  "on_demand",
			},
			want:        0,
			wantUnknown: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, priced := CalculateEC2PoolPrice(tt.config, rates)
			if priced == tt.wantUnknown {
				t.Fatalf("CalculateEC2PoolPrice() priced = %v, want %v", priced, !tt.wantUnknown)
			}
			if !floatEquals(got, tt.want, 0.01) {
				t.Errorf("CalculateEC2PoolPrice() = %.4f, want %.4f", got, tt.want)
			}
		})
	}
}

// TestEC2PoolHourly_UnpricedIsUnknownNotFree is the C-4 rule at the cost view's
// boundary: an instance type nobody has a price for is reported as unknown.
// Returning 0 as a price would render as "free", and a pool of instances is
// never free.
func TestEC2PoolHourly_UnpricedIsUnknownNotFree(t *testing.T) {
	rates := getTestEC2Rates()

	hourly, priced := EC2PoolHourly(EC2PoolConfig{
		InstanceTypes: []string{"m9z.nonexistent"},
		InstanceCount: 3,
		CapacityType:  "on_demand",
	}, rates)
	if priced {
		t.Fatalf("EC2PoolHourly() priced an instance type with no price (got %v)", hourly)
	}
	if hourly != 0 {
		t.Errorf("unpriced pool returned hourly = %v, want 0 alongside priced=false", hourly)
	}

	// The same pool scaled to zero is a different answer: priced, at zero,
	// because it is off rather than unknown.
	hourly, priced = EC2PoolHourly(EC2PoolConfig{
		InstanceTypes: []string{"m6i.large"},
		InstanceCount: 0,
		CapacityType:  "on_demand",
	}, rates)
	if !priced || hourly != 0 {
		t.Errorf("pool scaled to zero: got hourly=%v priced=%v, want 0/true", hourly, priced)
	}
}

// TestEC2PoolHourly_UnusableSpotRatioNeverDiscounts guards the direction of the
// failure: a rate table with a missing or nonsensical SpotRatio prices spot as
// on-demand, which over-reports, rather than as free.
func TestEC2PoolHourly_UnusableSpotRatioNeverDiscounts(t *testing.T) {
	for _, ratio := range []float64{0, -0.5, 1.5} {
		rates := getTestEC2Rates()
		rates.EC2.SpotRatio = ratio

		hourly, priced := EC2PoolHourly(EC2PoolConfig{
			InstanceTypes: []string{"m6i.large"},
			InstanceCount: 2,
			CapacityType:  "spot",
		}, rates)
		if !priced {
			t.Fatalf("SpotRatio=%v: pool went unpriced", ratio)
		}
		if want := 2 * 0.10; !floatEquals(hourly, want, 1e-9) {
			t.Errorf("SpotRatio=%v: hourly = %v, want %v (on-demand)", ratio, hourly, want)
		}
	}
}

// TestEC2PoolPrice_DoesNotVaryWithTasks is AC-53 stated as an invariant:
// EC2PoolConfig has no task field at all, so no number of tasks can change the
// bill. The Fargate calculator appears alongside it to show the contrast --
// there, tasks are exactly what moves the number, which is why an EC2 service
// must not be priced through it.
func TestEC2PoolPrice_DoesNotVaryWithTasks(t *testing.T) {
	rates := getTestEC2Rates()
	pool := EC2PoolConfig{
		InstanceTypes: []string{"m6i.large"},
		InstanceCount: 1,
		CapacityType:  "on_demand",
	}

	poolCost, priced := CalculateEC2PoolPrice(pool, rates)
	if !priced {
		t.Fatalf("pool of m6i.large priced as unknown against rates that carry it")
	}
	if want := 0.10 * HoursPerMonth; !floatEquals(poolCost, want, 0.01) {
		t.Fatalf("pool cost = %v, want %v", poolCost, want)
	}

	for _, tasks := range []int{1, 5, 50} {
		if got, _ := CalculateEC2PoolPrice(pool, rates); got != poolCost {
			t.Errorf("pool cost changed to %v at %d tasks; instances are billed, not tasks", got, tasks)
		}

		// Priced as Fargate instead, the same tasks cost more with every one
		// added -- which is the number an EC2 service must NOT report.
		fargateEquivalent := CalculateECSPrice(ECSConfig{CPU: 256, Memory: 512, DesiredCount: tasks}, rates)
		if fargateEquivalent <= 0 {
			t.Fatalf("Fargate control case priced %d tasks at %v", tasks, fargateEquivalent)
		}
	}
}

// TestFallbackRatesCarryEC2Prices pins the wiring between the fallback rate
// table and the compute fallback data: a degraded response prices a pool, and
// prices it from the same table the instance picker shows, not a second copy.
func TestFallbackRatesCarryEC2Prices(t *testing.T) {
	rates := getHardcodedFallbackRates()

	if rates.EC2.SpotRatio <= 0 || rates.EC2.SpotRatio > 1 {
		t.Errorf("fallback SpotRatio = %v, want a usable fraction in (0,1]", rates.EC2.SpotRatio)
	}

	for _, instanceType := range FallbackDefaultPoolInstanceTypes() {
		hourly, exists := rates.EC2.OnDemandHourly[instanceType]
		if !exists {
			t.Errorf("fallback rates have no price for %q, a default-pool type", instanceType)
			continue
		}
		if hourly <= 0 {
			t.Errorf("fallback price for %q is %v; absent means unknown, 0 would mean free", instanceType, hourly)
		}
	}

	// The synthesized default pool (FR-58) must produce a real number under
	// fallback rates, since that is exactly the no-credentials path.
	cost, priced := CalculateEC2PoolPrice(EC2PoolConfig{
		InstanceTypes: FallbackDefaultPoolInstanceTypes(),
		InstanceCount: 1,
		CapacityType:  "on_demand",
	}, rates)
	if !priced {
		t.Fatal("the synthesized default pool priced as unknown under fallback rates")
	}
	if cost <= 0 {
		t.Errorf("synthesized default pool priced at %v under fallback rates", cost)
	}
}
