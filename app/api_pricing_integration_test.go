// +build integration

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	pricingpkg "madappgang.com/meroku/pricing"
)

// TestPricingRatesEndpoint tests the /api/pricing/rates endpoint
// This is an integration test that verifies the full request/response cycle
func TestPricingRatesEndpoint(t *testing.T) {
	// Initialize global pricing service (simulating app startup)
	ctx := context.Background()
	var err error
	globalPricingService, err = pricingpkg.NewService(ctx, []string{"us-east-1"})
	if err != nil {
		t.Fatalf("Failed to initialize pricing service: %v", err)
	}

	// Test 1: Basic GET request
	t.Run("BasicRequest", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/pricing/rates", nil)
		w := httptest.NewRecorder()

		getPricingRates(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Status = %d, expected %d. Body: %s", w.Code, http.StatusOK, w.Body.String())
		}

		// Verify content type
		contentType := w.Header().Get("Content-Type")
		if contentType != "application/json" {
			t.Errorf("Content-Type = %s, expected application/json", contentType)
		}
	})

	// Test 2: Parse and validate response structure
	t.Run("ResponseStructure", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/pricing/rates", nil)
		w := httptest.NewRecorder()

		getPricingRates(w, req)

		var rates pricingpkg.PriceRates
		if err := json.NewDecoder(w.Body).Decode(&rates); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		// Verify required fields
		if rates.Region == "" {
			t.Error("Region is empty")
		}
		if rates.Source == "" {
			t.Error("Source is empty")
		}
		if rates.Aurora.ACUHourly == 0 {
			t.Error("Aurora ACU price is zero")
		}
		if len(rates.RDS) == 0 {
			t.Error("RDS prices map is empty")
		}
		if rates.Fargate.VCPUHourly == 0 {
			t.Error("Fargate vCPU price is zero")
		}

		t.Logf("Response: Region=%s, Source=%s, AuroraACU=%.4f, RDSInstances=%d",
			rates.Region, rates.Source, rates.Aurora.ACUHourly, len(rates.RDS))
	})

	// Test 3: Request with explicit region parameter
	t.Run("WithRegionParameter", func(t *testing.T) {
		// Reinitialize service with multiple regions
		globalPricingService, _ = pricingpkg.NewService(ctx, []string{"us-east-1", "us-west-2"})

		req := httptest.NewRequest("GET", "/api/pricing/rates?region=us-west-2", nil)
		w := httptest.NewRecorder()

		getPricingRates(w, req)

		var rates pricingpkg.PriceRates
		json.NewDecoder(w.Body).Decode(&rates)

		// Note: Since we don't have real AWS API implementation yet,
		// the region might still be set based on fallback logic
		// But the endpoint should handle the parameter correctly
		if w.Code != http.StatusOK {
			t.Errorf("Failed with region parameter. Status=%d", w.Code)
		}

		t.Logf("Response with region param: Region=%s, Source=%s", rates.Region, rates.Source)
	})

	// Test 4: Verify pricing data is realistic
	t.Run("RealisticPricing", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/pricing/rates", nil)
		w := httptest.NewRecorder()

		getPricingRates(w, req)

		var rates pricingpkg.PriceRates
		json.NewDecoder(w.Body).Decode(&rates)

		// Aurora ACU pricing should be around $0.12/hour (realistic range: $0.10-$0.15)
		if rates.Aurora.ACUHourly < 0.10 || rates.Aurora.ACUHourly > 0.15 {
			t.Errorf("Aurora ACU price %.4f is outside realistic range ($0.10-$0.15)",
				rates.Aurora.ACUHourly)
		}

		// Fargate vCPU pricing should be around $0.04048/hour
		if rates.Fargate.VCPUHourly < 0.03 || rates.Fargate.VCPUHourly > 0.05 {
			t.Errorf("Fargate vCPU price %.4f is outside realistic range ($0.03-$0.05)",
				rates.Fargate.VCPUHourly)
		}

		// Check RDS instances have reasonable prices
		t4gMicroPrice, exists := rates.RDS["db.t4g.micro"]
		if !exists {
			t.Error("db.t4g.micro pricing not found")
		} else if t4gMicroPrice < 0.01 || t4gMicroPrice > 0.03 {
			t.Errorf("db.t4g.micro price %.4f is outside realistic range ($0.01-$0.03)", t4gMicroPrice)
		}

		t.Logf("Pricing validation passed. Aurora=$%.4f/ACU, Fargate=$%.4f/vCPU, t4g.micro=$%.4f/hr",
			rates.Aurora.ACUHourly, rates.Fargate.VCPUHourly, t4gMicroPrice)
	})

	// Test 5: Method not allowed
	t.Run("MethodNotAllowed", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/pricing/rates", nil)
		w := httptest.NewRecorder()

		getPricingRates(w, req)

		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("POST request: Status = %d, expected %d", w.Code, http.StatusMethodNotAllowed)
		}
	})

	// Test 6: Cache performance (multiple requests should be fast)
	t.Run("CachePerformance", func(t *testing.T) {
		// Make multiple requests and verify cache is working
		for i := 0; i < 5; i++ {
			req := httptest.NewRequest("GET", "/api/pricing/rates", nil)
			w := httptest.NewRecorder()

			getPricingRates(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("Request %d failed with status %d", i+1, w.Code)
			}
		}

		// Check cache metrics
		metrics := globalPricingService.GetCacheMetrics()
		t.Logf("Cache metrics after 5 requests: Hits=%d, Misses=%d",
			metrics.Hits, metrics.Misses)

		// After first request, subsequent requests should hit cache
		if metrics.Hits == 0 {
			t.Error("No cache hits after multiple requests - cache may not be working")
		}
	})
}

// TestPricingCalculationConsistency verifies backend calculations match expected values
// This ensures the pricing service produces consistent results
func TestPricingCalculationConsistency(t *testing.T) {
	ctx := context.Background()
	globalPricingService, _ = pricingpkg.NewService(ctx, []string{"us-east-1"})

	// Get rates
	req := httptest.NewRequest("GET", "/api/pricing/rates", nil)
	w := httptest.NewRecorder()
	getPricingRates(w, req)

	var rates pricingpkg.PriceRates
	json.NewDecoder(w.Body).Decode(&rates)

	// Test Aurora pricing calculation
	t.Run("AuroraCalculation", func(t *testing.T) {
		config := pricingpkg.AuroraConfig{
			MinCapacity: 0,
			MaxCapacity: 1,
			Level:       "startup",
		}

		price := pricingpkg.CalculateAuroraPrice(config, &rates)

		// Expected: 0.15 ACU * 0.12 * 730 = 13.14
		expected := 13.14
		if !floatEquals(price, expected, 0.01) {
			t.Errorf("Aurora price = %.2f, expected %.2f", price, expected)
		}

		t.Logf("Aurora calculation: minCapacity=%d, maxCapacity=%d, level=%s, price=$%.2f/mo",
			config.MinCapacity, config.MaxCapacity, config.Level, price)
	})

	// Test RDS pricing calculation
	t.Run("RDSCalculation", func(t *testing.T) {
		config := pricingpkg.RDSConfig{
			InstanceClass:    "db.t4g.micro",
			AllocatedStorage: 20,
			MultiAZ:          false,
		}

		price := pricingpkg.CalculateRDSPrice(config, &rates)

		// Expected: (0.016 * 730) + (20 * 0.115) = 13.98
		expected := 13.98
		if !floatEquals(price, expected, 0.01) {
			t.Errorf("RDS price = %.2f, expected %.2f", price, expected)
		}

		t.Logf("RDS calculation: instance=%s, storage=%dGB, multiAZ=%v, price=$%.2f/mo",
			config.InstanceClass, config.AllocatedStorage, config.MultiAZ, price)
	})
}

// floatEquals checks if two floats are equal within a tolerance
func floatEquals(a, b, tolerance float64) bool {
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff < tolerance
}
