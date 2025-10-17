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
	source: 'aws_api' | 'fallback';
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

const STORAGE_KEY = 'aws_pricing_rates';
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
	region = 'us-east-1',
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
		console.error('[Pricing] Failed to fetch rates:', error);

		// Try to use cached data as fallback
		const cached = getCachedRates();
		if (cached) {
			console.warn('[Pricing] Using cached rates due to fetch error');
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
			console.log('[Pricing] Cache is stale, will refresh');
			return null;
		}

		return rates;
	} catch (error) {
		console.error('[Pricing] Error reading cache:', error);
		return null;
	}
}

/**
 * Clears the pricing cache
 * Useful for testing or forcing a refresh
 */
export function clearPricingCache(): void {
	sessionStorage.removeItem(STORAGE_KEY);
	console.log('[Pricing] Cache cleared');
}

/**
 * Checks if pricing rates are currently cached
 *
 * @returns true if rates are cached and not stale
 */
export function hasCachedRates(): boolean {
	return getCachedRates() !== null;
}
