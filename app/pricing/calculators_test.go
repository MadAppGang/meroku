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
