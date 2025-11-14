/**
 * Unified AWS Pricing Calculators
 *
 * CRITICAL: These calculations MUST match the backend calculators exactly
 * Location: app/pricing/calculators.go
 *
 * Any changes to calculation logic must be synchronized between:
 * - Backend: app/pricing/calculators.go
 * - Frontend: web/src/utils/awsPricing.ts
 * - Tests: Both backend and frontend tests
 *
 * @module awsPricing
 */

import type { AWSPriceRates } from '../services/pricingService';

const HOURS_PER_MONTH = 730; // Standard hours per month for cost calculations

// Configuration types (mirror backend types)

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

export interface ECSConfig {
	cpu: number; // CPU units (e.g., 256, 512, 1024)
	memory: number; // Memory in MB (e.g., 512, 1024, 2048)
	desiredCount: number; // Number of tasks
}

export interface S3Config {
	storageGb: number;
	requestsPerDay: number;
}

/**
 * Calculate monthly RDS cost
 *
 * MUST match backend: app/pricing/calculators.go:CalculateRDSPrice()
 *
 * @param config - RDS configuration
 * @param rates - Current pricing rates from service
 * @returns Monthly cost in USD
 */
export function calculateRDSPrice(
	config: RDSConfig,
	rates: AWSPriceRates,
): number {
	// Get hourly instance price
	let instanceHourly =
		rates.rds[config.instanceClass] || rates.rds['db.t4g.micro'];

	// Multi-AZ doubles instance cost (storage is already replicated)
	if (config.multiAz) {
		instanceHourly *= 2;
	}

	// Calculate monthly costs
	const instanceCostMonthly = instanceHourly * HOURS_PER_MONTH;
	const storageCostMonthly =
		config.allocatedStorage * rates.storage.gp3PerGbMonth;

	return instanceCostMonthly + storageCostMonthly;
}

/**
 * Calculate monthly Aurora Serverless v2 cost
 *
 * MUST match backend: app/pricing/calculators.go:CalculateAuroraPrice()
 *
 * Uses workload-level based ACU estimation with realistic utilization assumptions:
 * - startup:  20% utilization, 75% active time if min=0
 * - scaleup:  50% utilization, 90% active time if min=0
 * - highload: 80% utilization, 100% active time
 *
 * @param config - Aurora configuration
 * @param rates - Current pricing rates from service
 * @returns Monthly cost in USD
 */
export function calculateAuroraPrice(
	config: AuroraConfig,
	rates: AWSPriceRates,
): number {
	const avgACU = calculateAverageACU(config);
	const hourlyACUCost = avgACU * rates.aurora.acuHourly;
	return hourlyACUCost * HOURS_PER_MONTH;
}

/**
 * Calculate minimum Aurora Serverless price (at min capacity)
 * This represents the lowest possible cost when scaled to minimum ACUs
 */
export function calculateAuroraMinPrice(
	config: AuroraConfig,
	rates: AWSPriceRates,
): number {
	const minACU = config.minCapacity;
	const hourlyACUCost = minACU * rates.aurora.acuHourly;
	return hourlyACUCost * HOURS_PER_MONTH;
}

/**
 * Calculate maximum Aurora Serverless price (at max capacity)
 * This represents the highest possible cost when scaled to maximum ACUs
 */
export function calculateAuroraMaxPrice(
	config: AuroraConfig,
	rates: AWSPriceRates,
): number {
	const maxACU = config.maxCapacity;
	const hourlyACUCost = maxACU * rates.aurora.acuHourly;
	return hourlyACUCost * HOURS_PER_MONTH;
}

/**
 * Calculate average ACU based on workload level
 *
 * CRITICAL: This logic MUST match backend exactly
 * Backend: app/pricing/calculators.go:calculateAverageACU()
 *
 * Utilization assumptions:
 * - startup:  20% of (max - min) capacity, 75% active time if min=0
 * - scaleup:  50% of (max - min) capacity, 90% active time if min=0
 * - highload: 80% of (max - min) capacity, 100% active time (always on)
 *
 * @param config - Aurora configuration
 * @returns Average ACU usage
 */
function calculateAverageACU(config: AuroraConfig): number {
	const { minCapacity, maxCapacity, level } = config;

	// Determine utilization percentage based on workload level
	const utilization = {
		startup: 0.2, // 20% average utilization for startup workloads
		scaleup: 0.5, // 50% average utilization for scaleup workloads
		highload: 0.8, // 80% average utilization for highload workloads
	}[level];

	// Calculate average ACU: min + (range * utilization)
	let avgACU = minCapacity + (maxCapacity - minCapacity) * utilization;

	// If min capacity is 0, database can pause (scale to zero)
	// Apply active time percentage to account for pause periods
	if (minCapacity === 0) {
		const activeTime = {
			startup: 0.75, // Active 75% of the time
			scaleup: 0.9, // Active 90% of the time
			highload: 1.0, // Always active (100%)
		}[level];

		avgACU *= activeTime;
	}

	return avgACU;
}

/**
 * Calculate monthly ECS Fargate cost
 *
 * MUST match backend: app/pricing/calculators.go:CalculateECSPrice()
 *
 * @param config - ECS configuration
 * @param rates - Current pricing rates from service
 * @returns Monthly cost in USD
 */
export function calculateECSPrice(
	config: ECSConfig,
	rates: AWSPriceRates,
): number {
	// Convert CPU units to vCPU (256 units = 0.25 vCPU)
	const vCPU = config.cpu / 1024.0;
	const memoryGB = config.memory / 1024.0;

	// Calculate hourly cost per task
	const vCPUCostPerTask = vCPU * rates.fargate.vcpuHourly;
	const memoryCostPerTask = memoryGB * rates.fargate.memoryGbHourly;
	const hourlyCostPerTask = vCPUCostPerTask + memoryCostPerTask;

	// Multiply by desired count for total hourly cost
	const totalHourlyCost = hourlyCostPerTask * config.desiredCount;

	// Calculate monthly cost
	return totalHourlyCost * HOURS_PER_MONTH;
}

/**
 * Calculate monthly S3 cost
 *
 * MUST match backend: app/pricing/calculators.go:CalculateS3Price()
 *
 * @param config - S3 configuration
 * @param rates - Current pricing rates from service
 * @returns Monthly cost in USD
 */
export function calculateS3Price(
	config: S3Config,
	rates: AWSPriceRates,
): number {
	// Storage cost
	const storageCost = config.storageGb * rates.s3.standardPerGbMonth;

	// Request cost (convert daily to monthly)
	const monthlyRequests = config.requestsPerDay * 30;
	const requestCost = (monthlyRequests / 1000.0) * rates.s3.requestsPer1000;

	return storageCost + requestCost;
}

/**
 * Format price for display
 *
 * @param price - Price in USD
 * @returns Formatted price string
 */
export function formatPrice(price: number): string {
	if (price < 0.01) {
		return `$${price.toFixed(4)}`;
	}
	if (price < 1) {
		return `$${price.toFixed(2)}`;
	}
	return `$${price.toFixed(0)}`;
}

/**
 * Format price with /mo suffix for monthly costs
 *
 * @param price - Price in USD
 * @returns Formatted price string with /mo
 */
export function formatMonthlyPrice(price: number): string {
	return `${formatPrice(price)}/mo`;
}
