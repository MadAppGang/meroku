/**
 * Centralized RDS pricing calculations
 * Ensures consistent pricing across all UI components
 */

// RDS Instance pricing (us-east-1, Single-AZ, hourly rates)
// Prices as of January 2025
export const RDS_INSTANCE_PRICES: Record<string, number> = {
	"db.t4g.micro": 0.016,
	"db.t4g.small": 0.032,
	"db.t4g.medium": 0.065,
	"db.t4g.large": 0.129,
	"db.t3.micro": 0.018,
	"db.t3.small": 0.036,
	"db.t3.medium": 0.073,
	"db.m6i.large": 0.178,
	"db.m6i.xlarge": 0.356,
	"db.m6i.2xlarge": 0.712,
	"db.m5.large": 0.192,
	"db.m5.xlarge": 0.384,
	"db.r6i.large": 0.24,
	"db.r6i.xlarge": 0.48,
	"db.r5.large": 0.26,
	"db.r5.xlarge": 0.52,
};

// Storage pricing (per GB per month)
export const STORAGE_PRICE_GP3 = 0.115;

// Hours per month for cost calculations
const HOURS_PER_MONTH = 730;

export interface RDSPricingConfig {
	instanceClass: string;
	allocatedStorage: number;
	multiAz: boolean;
}

export interface RDSPricingResult {
	instanceCostMonthly: number;
	storageCostMonthly: number;
	totalCostMonthly: number;
	hourlyRate: number;
}

/**
 * Calculate RDS pricing based on configuration
 * @param config RDS configuration
 * @returns Pricing breakdown
 */
export function calculateRDSPricing(
	config: RDSPricingConfig,
): RDSPricingResult {
	const instanceClass = config.instanceClass || "db.t4g.micro";
	const storage = config.allocatedStorage || 20;
	const multiAz = config.multiAz || false;

	// Get hourly instance price
	let instanceHourlyPrice = RDS_INSTANCE_PRICES[instanceClass] || 0.016;

	// Multi-AZ doubles instance cost (storage is already replicated)
	if (multiAz) {
		instanceHourlyPrice *= 2;
	}

	// Calculate monthly costs
	const instanceCostMonthly = instanceHourlyPrice * HOURS_PER_MONTH;
	const storageCostMonthly = storage * STORAGE_PRICE_GP3;
	const totalCostMonthly = instanceCostMonthly + storageCostMonthly;
	const hourlyRate = totalCostMonthly / HOURS_PER_MONTH;

	return {
		instanceCostMonthly,
		storageCostMonthly,
		totalCostMonthly,
		hourlyRate,
	};
}

/**
 * Get monthly price for display (formatted)
 */
export function getMonthlyPrice(config: RDSPricingConfig): number {
	const pricing = calculateRDSPricing(config);
	return pricing.totalCostMonthly;
}

/**
 * Format price for display
 */
export function formatPrice(price: number): string {
	return price < 1 ? price.toFixed(2) : price.toFixed(0);
}
