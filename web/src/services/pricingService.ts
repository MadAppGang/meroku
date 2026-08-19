/**
 * AWS Pricing Service
 *
 * Fetches and caches pricing rates from the backend pricing API.
 * This service provides the single source of truth for all AWS pricing data.
 *
 * Architecture:
 * - Fetches rates once from /api/pricing/rates
 * - Caches in sessionStorage for offline support and performance
 * - All components use the same cached rates for consistency
 *
 * @module pricingService
 */

export interface AWSPriceRates {
	region: string;
	lastUpdate: string;
	source: "aws_api" | "fallback";
	pricingDate?: string; // When pricing was sourced (e.g., "2025-01-15")

	// Compute pricing
	rds: Record<string, number>; // Instance type -> hourly price
	aurora: {
		acuHourly: number; // $/ACU/hour
		storageGbMonth: number; // $/GB/month
		ioRequestsPerM: number; // $/million I/O requests
	};
	fargate: {
		vcpuHourly: number; // $/vCPU/hour
		memoryGbHourly: number; // $/GB/hour
	};

	// ECS on EC2 capacity pools.
	//
	// The unit is the point, and it differs from every other compute price
	// here: Fargate is billed per TASK, EC2 per INSTANCE-hour, whether or not a
	// task is running on the instance. A pool at min_size: 1 with zero tasks
	// costs a full instance-hour every hour. Pricing an EC2-runtime service the
	// way Fargate is priced bills one instance once per task -- see
	// calculateEC2PoolPrice in utils/awsPricing.ts.
	//
	// Mirrors app/pricing/types.go:EC2Pricing.
	ec2: {
		// Instance type -> $/instance-hour, Linux, shared tenancy, on-demand.
		// An absent key means "price unknown" and must never be read as free.
		onDemandHourly: Record<string, number>;
		// Spot price as a FRACTION OF on-demand for the same type, e.g. 0.35
		// means "spot typically costs 35% of on-demand". Indicative planning
		// figure, not a quote. A value outside (0,1] is treated as 1.
		spotRatio: number;
	};

	// Storage pricing
	storage: {
		gp3PerGbMonth: number; // $/GB/month
		gp2PerGbMonth: number; // $/GB/month
	};
	s3: {
		standardPerGbMonth: number; // $/GB/month
		requestsPer1000: number; // $/1000 requests
	};

	// Networking pricing
	alb: {
		hourlyPrice: number; // $/hour
		lcuPrice: number; // $/LCU/hour
	};
	apiGateway: {
		requestsPerMillion: number; // $/million requests
	};
	natGateway: {
		hourlyPrice: number;
		dataPerGbMonth: number;
	};

	// Other services
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
	ses: {
		per1000Emails: number;
	};
	eventBridge: {
		eventsPerMillion: number;
	};
	ecr: {
		storagePerGbMonth: number;
	};
}

// The key carries a version because the cached value is a whole rate table and
// this module is the only thing that can tell a stale shape from a fresh one. A
// v1 entry written before `ec2` existed would otherwise survive in
// sessionStorage for an hour and reach the calculators as an AWSPriceRates with
// a field the type system promises is there. Bump this whenever a field is
// added to AWSPriceRates.
const STORAGE_KEY = "aws_pricing_rates_v2";
const CACHE_DURATION = 60 * 60 * 1000; // 1 hour

/**
 * Fetches current pricing rates from backend API
 * Automatically caches result in sessionStorage
 *
 * @param region - AWS region (default: us-east-1)
 * @returns Promise resolving to AWS pricing rates
 * @throws Error if fetch fails and no cache available
 */
export async function fetchPricingRates(
	region = "us-east-1",
): Promise<AWSPriceRates> {
	try {
		const response = await fetch(`/api/pricing/rates?region=${region}`);

		if (!response.ok) {
			throw new Error(`Failed to fetch pricing rates: ${response.statusText}`);
		}

		const rates: AWSPriceRates = await response.json();

		// Cache in sessionStorage for offline support and performance
		sessionStorage.setItem(
			STORAGE_KEY,
			JSON.stringify({
				rates,
				timestamp: Date.now(),
			}),
		);

		console.log(
			`[Pricing] Fetched rates for ${region} (source: ${rates.source})`,
		);

		return rates;
	} catch (error) {
		console.error("[Pricing] Failed to fetch rates:", error);

		// Try to use cached data as fallback
		const cached = getCachedRates();
		if (cached) {
			console.warn("[Pricing] Using cached rates due to fetch error");
			return cached;
		}

		throw error;
	}
}

/**
 * Returns cached pricing rates if available and not stale
 *
 * @returns Cached rates or null if not available/stale
 */
export function getCachedRates(): AWSPriceRates | null {
	try {
		const cached = sessionStorage.getItem(STORAGE_KEY);
		if (!cached) return null;

		const { rates, timestamp } = JSON.parse(cached);

		// Check if cache is stale (older than 1 hour)
		if (Date.now() - timestamp > CACHE_DURATION) {
			console.log("[Pricing] Cache is stale, will refresh");
			return null;
		}

		return rates;
	} catch (error) {
		console.error("[Pricing] Error reading cache:", error);
		return null;
	}
}

/**
 * Clears the pricing cache
 * Useful for testing or forcing a refresh
 */
export function clearPricingCache(): void {
	sessionStorage.removeItem(STORAGE_KEY);
	console.log("[Pricing] Cache cleared");
}

/**
 * Checks if pricing rates are currently cached
 *
 * @returns true if rates are cached and not stale
 */
export function hasCachedRates(): boolean {
	return getCachedRates() !== null;
}
