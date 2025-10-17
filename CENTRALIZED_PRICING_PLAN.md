# Centralized AWS Pricing Architecture - Implementation Plan

## 🎯 Goal
Create a single source of truth for AWS pricing calculations across the entire application, eliminating inconsistencies between backend and frontend.

## 🏗️ Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│                    SINGLE BINARY                            │
│  ┌────────────────────────────────────────────────────┐    │
│  │  Backend: Pricing Service (Singleton)              │    │
│  │  Location: app/pricing/                            │    │
│  │  - PriceCache (24h TTL, thread-safe)              │    │
│  │  - AWSPricingClient (retry + fallback)            │    │
│  │  - Unified Calculators (Go)                       │    │
│  │  - Automatic background refresh                    │    │
│  └────────────────────────────────────────────────────┘    │
│                      ↓                                       │
│  ┌────────────────────────────────────────────────────┐    │
│  │  API Endpoint: /api/pricing/rates                  │    │
│  │  - Returns: PriceRates JSON                        │    │
│  │  - Query params: ?region=us-east-1 (optional)     │    │
│  │  - Response cached by service (fast!)             │    │
│  └────────────────────────────────────────────────────┘    │
│                      ↓                                       │
│  ┌────────────────────────────────────────────────────┐    │
│  │  Frontend: React App (TypeScript)                  │    │
│  │  Location: web/src/                                │    │
│  │  - pricingService.ts: Fetch & cache rates          │    │
│  │  - awsPricing.ts: Unified calculators (mirrors Go)│    │
│  │  - PricingContext: React Context for app-wide     │    │
│  │  - All components use same calculators            │    │
│  └────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────┘
```

## 📊 Current vs. New State

### Current State (Problems):
- ❌ Backend: 900+ lines of pricing code with hardcoded values
- ❌ Frontend: Hardcoded `0.12` ACU price in PricingBadge
- ❌ Frontend: Separate rdsPricing.ts with different values
- ❌ Inconsistent calculations: $15 vs $49.75 for same config
- ❌ No single source of truth

### New State (Solution):
- ✅ Backend: 15-line handler + reusable pricing package
- ✅ Frontend: Fetches rates once, uses unified calculators
- ✅ Same calculation logic everywhere
- ✅ Consistent prices: Backend tests === Frontend results
- ✅ Single source of truth: pricing package

## 🚀 Implementation Steps

### Phase 1: Backend Simplification

#### 1.1 Add getPricingRates Handler (NEW)
**File:** `app/api_pricing.go`

```go
// GET /api/pricing/rates?region=us-east-1
// Returns raw pricing rates for frontend caching
// Uses global pricing service initialized at startup
func getPricingRates(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }

    // Get region from query params (default to us-east-1)
    region := r.URL.Query().Get("region")
    if region == "" {
        region = "us-east-1"
    }

    // Get rates from pricing service (cached, fast!)
    rates, err := globalPricingService.GetRates(region)
    if err != nil {
        w.WriteHeader(http.StatusInternalServerError)
        json.NewEncoder(w).Encode(ErrorResponse{Error: err.Error()})
        return
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(rates)
}
```

#### 1.2 Initialize Pricing Service (main.go)
**File:** `app/main.go`

```go
import (
    pricingpkg "madappgang.com/meroku/pricing"
)

// Global pricing service (initialized once at startup)
var globalPricingService *pricingpkg.Service

func main() {
    // ... existing code ...

    // Initialize pricing service for common regions
    ctx := context.Background()
    regions := []string{"us-east-1", "us-west-2", "eu-west-1"}

    var err error
    globalPricingService, err = pricingpkg.NewService(ctx, regions)
    if err != nil {
        log.Printf("[ERROR] Failed to initialize pricing service: %v", err)
        log.Printf("[WARN] Pricing service will use fallback prices only")
    } else {
        log.Printf("[INFO] Pricing service initialized successfully for regions: %v", regions)
    }

    // ... rest of main ...
}
```

#### 1.3 Update Routing (spa_server.go)
**File:** `app/spa_server.go`

```diff
  // Pricing
- mux.HandleFunc("/api/pricing", corsMiddleware(getPricing))
+ mux.HandleFunc("/api/pricing/rates", corsMiddleware(getPricingRates))
```

#### 1.4 Delete Old Code (api_pricing.go)
**Delete these functions (700+ lines):**
- `getPricing()` - Old endpoint
- `calculateBackendPricing()`
- `calculateServicePricing()`
- `calculateRDSPricing()`
- `calculateALBPricing()`
- `calculateAPIGatewayPricing()`
- `calculateRoute53Pricing()`
- `calculateSESPricing()`
- `calculateEventBridgePricing()`
- `calculateScheduledTaskPricing()`
- `calculateCloudWatchPricing()`
- `calculateECRPricing()`
- `calculateVPCPricing()`
- `calculateCognitoPricing()`
- `calculateS3Pricing()`
- All helper functions: `getFargatePrice()`, `getRDSInstancePrice()`, etc.
- `WorkloadSpecs` map
- `estimateRunsPerMonth()`
- `parsePriceFromProduct()`

**Keep only:**
- Type definitions (for now, can be removed later if not used elsewhere)
- Import statements

### Phase 2: Frontend Implementation

#### 2.1 Create Pricing Service
**File:** `web/src/services/pricingService.ts`

```typescript
export interface AWSPriceRates {
  region: string;
  lastUpdate: string;
  source: 'aws_api' | 'fallback';
  rds: Record<string, number>;
  aurora: {
    acuHourly: number;
    storageGbMonth: number;
    ioRequestsPerM: number;
  };
  fargate: {
    vcpuHourly: number;
    memoryGbHourly: number;
  };
  storage: {
    gp3PerGbMonth: number;
    gp2PerGbMonth: number;
  };
  s3: {
    standardPerGbMonth: number;
    requestsPer1000: number;
  };
  alb: {
    hourlyPrice: number;
    lcuPrice: number;
  };
  apiGateway: {
    requestsPerMillion: number;
  };
  cloudWatch: {
    logsIngestionPerGb: number;
    metricsPerMetric: number;
  };
  route53: {
    hostedZonePerMonth: number;
    queriesPerMillion: number;
  };
  cognito: {
    mauPrice: number;
    freeMAUs: number;
  };
}

const STORAGE_KEY = 'aws_pricing_rates';
const CACHE_DURATION = 60 * 60 * 1000; // 1 hour

export async function fetchPricingRates(region = 'us-east-1'): Promise<AWSPriceRates> {
  const response = await fetch(`/api/pricing/rates?region=${region}`);
  if (!response.ok) throw new Error('Failed to fetch pricing rates');

  const rates: AWSPriceRates = await response.json();

  // Cache in sessionStorage
  sessionStorage.setItem(STORAGE_KEY, JSON.stringify({
    rates,
    timestamp: Date.now(),
  }));

  return rates;
}

export function getCachedRates(): AWSPriceRates | null {
  const cached = sessionStorage.getItem(STORAGE_KEY);
  if (!cached) return null;

  const { rates, timestamp } = JSON.parse(cached);
  if (Date.now() - timestamp > CACHE_DURATION) return null;

  return rates;
}
```

#### 2.2 Create Unified Calculators
**File:** `web/src/utils/awsPricing.ts`

```typescript
import type { AWSPriceRates } from '../services/pricingService';

const HOURS_PER_MONTH = 730;

export interface RDSConfig {
  instanceClass: string;
  allocatedStorage: number;
  multiAz: boolean;
}

export interface AuroraConfig {
  minCapacity: number;
  maxCapacity: number;
  level: 'startup' | 'scaleup' | 'highload';
}

// CRITICAL: This calculation MUST match backend exactly (app/pricing/calculators.go)
export function calculateRDSPrice(config: RDSConfig, rates: AWSPriceRates): number {
  let instanceHourly = rates.rds[config.instanceClass] || rates.rds['db.t4g.micro'];

  if (config.multiAz) {
    instanceHourly *= 2;
  }

  const instanceCostMonthly = instanceHourly * HOURS_PER_MONTH;
  const storageCostMonthly = config.allocatedStorage * rates.storage.gp3PerGbMonth;

  return instanceCostMonthly + storageCostMonthly;
}

// CRITICAL: This calculation MUST match backend exactly (app/pricing/calculators.go)
export function calculateAuroraPrice(config: AuroraConfig, rates: AWSPriceRates): number {
  const avgACU = calculateAverageACU(config);
  const hourlyACUCost = avgACU * rates.aurora.acuHourly;
  return hourlyACUCost * HOURS_PER_MONTH;
}

// CRITICAL: This logic MUST match backend exactly (app/pricing/calculators.go)
function calculateAverageACU(config: AuroraConfig): number {
  const { minCapacity, maxCapacity, level } = config;

  const utilization = {
    startup: 0.20,
    scaleup: 0.50,
    highload: 0.80,
  }[level];

  let avgACU = minCapacity + (maxCapacity - minCapacity) * utilization;

  if (minCapacity === 0) {
    const activeTime = {
      startup: 0.75,
      scaleup: 0.90,
      highload: 1.00,
    }[level];
    avgACU *= activeTime;
  }

  return avgACU;
}

export function formatPrice(price: number): string {
  return price < 1 ? price.toFixed(2) : price.toFixed(0);
}
```

#### 2.3 Create Pricing Context
**File:** `web/src/contexts/PricingContext.tsx`

```typescript
import { createContext, useContext, useEffect, useState, ReactNode } from 'react';
import { fetchPricingRates, getCachedRates, AWSPriceRates } from '../services/pricingService';

interface PricingContextValue {
  rates: AWSPriceRates | null;
  loading: boolean;
  error: Error | null;
  refresh: () => Promise<void>;
}

const PricingContext = createContext<PricingContextValue | undefined>(undefined);

export function PricingProvider({ children }: { children: ReactNode }) {
  const [rates, setRates] = useState<AWSPriceRates | null>(getCachedRates());
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<Error | null>(null);

  const fetchRates = async () => {
    setLoading(true);
    setError(null);
    try {
      const newRates = await fetchPricingRates();
      setRates(newRates);
    } catch (err) {
      setError(err as Error);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (!rates) fetchRates();
  }, []);

  return (
    <PricingContext.Provider value={{ rates, loading, error, refresh: fetchRates }}>
      {children}
    </PricingContext.Provider>
  );
}

export function usePricing() {
  const context = useContext(PricingContext);
  if (!context) throw new Error('usePricing must be used within PricingProvider');
  return context;
}
```

#### 2.4 Update PricingBadge Component
**File:** `web/src/components/PricingBadge.tsx`

```diff
+ import { usePricing } from '../contexts/PricingContext';
+ import { calculateAuroraPrice } from '../utils/awsPricing';

  export function PricingBadge({ nodeType, pricing, level, serviceName, configProperties }: PricingBadgeProps) {
+   const { rates } = usePricing();

    // Aurora pricing
    if (nodeType === "postgres" && configProperties?.aurora) {
-     const ACU_HOURLY_PRICE = 0.12;
-     const minCapacity = configProperties.minCapacity ?? 0;
-     const maxCapacity = configProperties.maxCapacity || 1;
-     // ... complex calculation ...
-     const monthlyPrice = avgACUs * ACU_HOURLY_PRICE * 24 * 30;

+     if (!rates) return <LoadingBadge />;
+
+     const config = {
+       minCapacity: configProperties.minCapacity ?? 0,
+       maxCapacity: configProperties.maxCapacity || 1,
+       level: level,
+     };
+     const monthlyPrice = calculateAuroraPrice(config, rates);

      return <Badge>...</Badge>;
    }
  }
```

#### 2.5 Delete Old Code
**Delete:** `web/src/utils/rdsPricing.ts` (entire file, 94 lines)

### Phase 3: Integration Testing 🧪

#### 3.1 Backend Integration Test
**File:** `app/pricing/integration_test.go`

```go
// +build integration

package pricing

import (
    "context"
    "testing"
    "time"
)

// TestServiceIntegration tests the full pricing service lifecycle
func TestServiceIntegration(t *testing.T) {
    ctx := context.Background()

    // Create service with real AWS config
    service, err := NewService(ctx, []string{"us-east-1"})
    if err != nil {
        t.Fatalf("Failed to create service: %v", err)
    }

    // Test 1: Get rates (should use fallback since AWS API not implemented)
    rates, err := service.GetRates("us-east-1")
    if err != nil {
        t.Fatalf("Failed to get rates: %v", err)
    }

    if rates.Source != "fallback" {
        t.Logf("Got rates from: %s (expected fallback for now)", rates.Source)
    }

    // Test 2: Verify rate structure
    if rates.Aurora.ACUHourly == 0 {
        t.Error("Aurora ACU price is zero")
    }
    if len(rates.RDS) == 0 {
        t.Error("RDS prices are empty")
    }

    // Test 3: Calculate Aurora price (min=0, max=1, startup)
    config := AuroraConfig{MinCapacity: 0, MaxCapacity: 1, Level: "startup"}
    price := CalculateAuroraPrice(config, rates)

    expected := 13.14 // 0.15 ACU * 0.12 * 730 = 13.14
    if !floatEquals(price, expected, 0.01) {
        t.Errorf("Aurora price = %.2f, expected %.2f", price, expected)
    }

    // Test 4: Cache metrics
    metrics := service.GetCacheMetrics()
    if metrics.Hits+metrics.Misses == 0 {
        t.Error("No cache activity recorded")
    }

    t.Logf("Cache metrics: Hits=%d, Misses=%d, LastRefresh=%v",
        metrics.Hits, metrics.Misses, metrics.LastRefresh)
}

// TestCalculatorConsistency ensures all calculators produce consistent results
func TestCalculatorConsistency(t *testing.T) {
    rates := getTestRates() // From calculators_test.go

    // Test multiple times to ensure consistency
    for i := 0; i < 100; i++ {
        config := AuroraConfig{MinCapacity: 0, MaxCapacity: 1, Level: "startup"}
        price := CalculateAuroraPrice(config, rates)

        if !floatEquals(price, 13.14, 0.01) {
            t.Errorf("Iteration %d: Inconsistent price %.2f", i, price)
        }
    }
}

func floatEquals(a, b, tolerance float64) bool {
    diff := a - b
    if diff < 0 {
        diff = -diff
    }
    return diff < tolerance
}
```

**Run with:** `cd app && go test ./pricing/... -tags=integration -v`

#### 3.2 Backend API Integration Test
**File:** `app/api_pricing_integration_test.go`

```go
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

func TestPricingRatesEndpoint(t *testing.T) {
    // Initialize global pricing service
    ctx := context.Background()
    var err error
    globalPricingService, err = pricingpkg.NewService(ctx, []string{"us-east-1"})
    if err != nil {
        t.Fatalf("Failed to initialize pricing service: %v", err)
    }

    // Test 1: Basic request
    req := httptest.NewRequest("GET", "/api/pricing/rates", nil)
    w := httptest.NewRecorder()

    getPricingRates(w, req)

    if w.Code != http.StatusOK {
        t.Errorf("Status = %d, expected %d", w.Code, http.StatusOK)
    }

    // Test 2: Parse response
    var rates pricingpkg.PriceRates
    if err := json.NewDecoder(w.Body).Decode(&rates); err != nil {
        t.Fatalf("Failed to decode response: %v", err)
    }

    // Test 3: Verify data
    if rates.Region == "" {
        t.Error("Region is empty")
    }
    if rates.Aurora.ACUHourly == 0 {
        t.Error("Aurora ACU price is zero")
    }

    t.Logf("Response: Region=%s, Source=%s, AuroraACU=%.4f",
        rates.Region, rates.Source, rates.Aurora.ACUHourly)
}

func TestPricingRatesWithRegion(t *testing.T) {
    ctx := context.Background()
    globalPricingService, _ = pricingpkg.NewService(ctx, []string{"us-east-1", "us-west-2"})

    // Test with explicit region
    req := httptest.NewRequest("GET", "/api/pricing/rates?region=us-west-2", nil)
    w := httptest.NewRecorder()

    getPricingRates(w, req)

    var rates pricingpkg.PriceRates
    json.NewDecoder(w.Body).Decode(&rates)

    if rates.Region != "us-west-2" {
        t.Errorf("Region = %s, expected us-west-2", rates.Region)
    }
}
```

#### 3.3 End-to-End Consistency Test
**File:** `app/pricing/e2e_consistency_test.go`

```go
// +build integration

package pricing

import (
    "encoding/json"
    "os"
    "testing"
)

// TestBackendFrontendConsistency ensures backend and frontend produce same results
// This test generates a JSON file that can be used by frontend tests
func TestBackendFrontendConsistency(t *testing.T) {
    rates := getTestRates()

    testCases := []struct {
        Name    string
        Config  AuroraConfig
        Expected float64
    }{
        {"Startup 0-1 ACU", AuroraConfig{0, 1, "startup"}, 13.14},
        {"Scaleup 1-4 ACU", AuroraConfig{1, 4, "scaleup"}, 219.00},
        {"Highload 2-16 ACU", AuroraConfig{2, 16, "highload"}, 1156.32},
    }

    results := make(map[string]interface{})

    for _, tc := range testCases {
        price := CalculateAuroraPrice(tc.Config, rates)

        if !floatEquals(price, tc.Expected, 0.01) {
            t.Errorf("%s: %.2f != %.2f", tc.Name, price, tc.Expected)
        }

        results[tc.Name] = map[string]interface{}{
            "config": tc.Config,
            "price":  price,
        }
    }

    // Export for frontend testing
    data, _ := json.MarshalIndent(results, "", "  ")
    os.WriteFile("test_results.json", data, 0644)

    t.Logf("Exported test results to test_results.json")
}
```

#### 3.4 Frontend Integration Test
**File:** `web/src/utils/__tests__/awsPricing.test.ts`

```typescript
import { calculateAuroraPrice, calculateRDSPrice } from '../awsPricing';
import type { AWSPriceRates } from '../../services/pricingService';

// Test rates (must match backend test rates exactly)
const testRates: AWSPriceRates = {
  region: 'us-east-1',
  lastUpdate: new Date().toISOString(),
  source: 'test',
  rds: {
    'db.t4g.micro': 0.016,
    'db.t4g.small': 0.032,
  },
  aurora: {
    acuHourly: 0.12,
    storageGbMonth: 0.10,
    ioRequestsPerM: 0.20,
  },
  fargate: { vcpuHourly: 0.04048, memoryGbHourly: 0.004445 },
  storage: { gp3PerGbMonth: 0.115, gp2PerGbMonth: 0.10 },
  // ... other fields
};

describe('Aurora Pricing Calculator', () => {
  test('Startup: 0-1 ACU (can pause)', () => {
    const config = { minCapacity: 0, maxCapacity: 1, level: 'startup' as const };
    const price = calculateAuroraPrice(config, testRates);

    // MUST match backend: 0.15 * 0.12 * 730 = 13.14
    expect(price).toBeCloseTo(13.14, 2);
  });

  test('Scaleup: 1-4 ACU', () => {
    const config = { minCapacity: 1, maxCapacity: 4, level: 'scaleup' as const };
    const price = calculateAuroraPrice(config, testRates);

    // MUST match backend: 2.5 * 0.12 * 730 = 219.00
    expect(price).toBeCloseTo(219.00, 2);
  });

  test('Highload: 2-16 ACU', () => {
    const config = { minCapacity: 2, maxCapacity: 16, level: 'highload' as const };
    const price = calculateAuroraPrice(config, testRates);

    // MUST match backend: 13.2 * 0.12 * 730 = 1156.32
    expect(price).toBeCloseTo(1156.32, 2);
  });
});

describe('RDS Pricing Calculator', () => {
  test('Basic t4g.micro single-AZ with 20GB', () => {
    const config = { instanceClass: 'db.t4g.micro', allocatedStorage: 20, multiAz: false };
    const price = calculateRDSPrice(config, testRates);

    // MUST match backend: (0.016 * 730) + (20 * 0.115) = 13.98
    expect(price).toBeCloseTo(13.98, 2);
  });

  test('t4g.small multi-AZ with 100GB', () => {
    const config = { instanceClass: 'db.t4g.small', allocatedStorage: 100, multiAz: true };
    const price = calculateRDSPrice(config, testRates);

    // MUST match backend: (0.032 * 2 * 730) + (100 * 0.115) = 58.22
    expect(price).toBeCloseTo(58.22, 2);
  });
});
```

**Run with:** `cd web && npm test`

## 🎯 Success Criteria

### Mathematical Consistency ✅
```
Backend Aurora (min=0, max=1, startup) = $13.14/mo
Frontend Aurora (min=0, max=1, startup) = $13.14/mo
✅ MATCH!
```

### Code Quality ✅
- Backend: Delete 700+ lines of duplicated code
- Frontend: Single unified calculator
- Tests: >80% coverage with integration tests
- Documentation: Clear architecture docs

### Performance ✅
- Backend: Cached rates, no AWS API calls per request
- Frontend: Fetch once, reuse everywhere
- Response time: <50ms for pricing calculations

## 📝 Testing Commands

```bash
# Backend unit tests
cd app && go test ./pricing/... -v

# Backend integration tests
cd app && go test ./pricing/... -tags=integration -v

# Frontend tests
cd web && npm test

# Full test suite
make test-all

# Build and verify
make build && ./meroku --version
```

## 🚀 Deployment

Since frontend and backend are bundled:
1. Make all changes in one commit
2. Run full test suite
3. Build single binary
4. Deploy together

No gradual migration or compatibility layers needed!

## 📚 Documentation Updates

Update these files:
- `CLAUDE.md` - Add centralized pricing architecture section
- `app/pricing/README.md` - Backend pricing service docs
- `web/src/services/PRICING.md` - Frontend integration guide

---

**Author:** Claude Code
**Date:** 2025-10-17
**Status:** Ready to Implement ✅
