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

import type { AWSPriceRates } from "../services/pricingService";

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
	level: "startup" | "scaleup" | "highload";
}

export interface ScheduledTaskConfig {
	cpu: number;
	memory: number;
	schedule: string; // e.g. "rate(1 minute)", "rate(1 hours)", "cron(...)"
	runtimeMinutes?: number; // average runtime per execution, defaults to 5
}

export interface ECSConfig {
	cpu: number; // CPU units (e.g., 256, 512, 1024)
	memory: number; // Memory in MB (e.g., 512, 1024, 2048)
	desiredCount: number; // Number of tasks
}

/**
 * Cost-relevant shape of one EC2 capacity pool.
 *
 * Mirrors app/pricing/types.go:EC2PoolConfig. Note what is NOT here: any task
 * count, CPU or memory. An EC2 pool's bill does not depend on what runs on it.
 */
export interface EC2PoolConfig {
	/**
	 * The pool's instance types in priority order. The first entry with a known
	 * price is the costing basis -- the ASG may launch any of them, so one
	 * figure is an estimate by construction.
	 */
	instanceTypes: string[];
	/** Number of INSTANCES the pool runs. 0 is valid and costs nothing. */
	instanceCount: number;
	/** on_demand | spot | spot_with_base. Anything else reads as on_demand. */
	capacityType?: string;
	/** On-demand base in INSTANCES, read only for spot_with_base. */
	onDemandBase?: number;
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
		rates.rds[config.instanceClass] || rates.rds["db.t4g.micro"];

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
 * FARGATE ONLY. The per-task shape below is correct for Fargate, where every
 * task is billed for its own vCPU and memory reservation, and wrong for EC2,
 * where the money is spent on instances billed whether or not tasks run on
 * them. Calling this for an EC2-runtime service bills the same instance once
 * per task. Use calculateEC2PoolPrice on the pool instead, and show the service
 * itself as $0 marginal.
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
 * Pick the instance type a pool's cost is quoted against: the first entry of
 * instanceTypes with a known on-demand price.
 *
 * Taking the first PRICED entry -- rather than the first entry, or the cheapest
 * -- keeps the estimate deterministic and tied to the priority order the
 * operator wrote down, while still producing an answer when a newly-added type
 * is missing from the rate table.
 *
 * Returns null when no listed type has a price. That means "price unknown", and
 * callers must render it as unknown. It never means free.
 *
 * MUST match backend: app/pricing/calculators.go:EC2PoolBasis()
 */
export function ec2PoolBasis(
	config: EC2PoolConfig,
	rates: AWSPriceRates,
): { instanceType: string; onDemandHourly: number } | null {
	// Optional chaining, despite the type: these rates may have been restored
	// from a cache written by an older build, and a missing table has to read
	// as "no prices" rather than throw inside a cost badge.
	const table = rates.ec2?.onDemandHourly ?? {};

	for (const instanceType of config.instanceTypes ?? []) {
		const hourly = table[instanceType];
		if (typeof hourly === "number" && hourly > 0) {
			return { instanceType, onDemandHourly: hourly };
		}
	}
	return null;
}

/**
 * Calculate the hourly cost of a whole EC2 capacity pool, in $/hour, blending
 * on-demand and spot the way the ASG's instances_distribution bills them.
 *
 *   N     = instanceCount                       instances the pool runs
 *   p_od  = on-demand $/instance-hour           (ec2PoolBasis)
 *   p_sp  = p_od * spotRatio                    spot $/instance-hour
 *   b     = on-demand base, in INSTANCES:       on_demand      -> N
 *                                               spot           -> 0
 *                                               spot_with_base -> onDemandBase
 *   n_od  = min(N, b)                           instances billed on demand
 *   n_sp  = N - n_od                            instances billed spot
 *
 *   hourly = n_od * p_od + n_sp * p_sp
 *
 * The weights are instances over instances, so the per-instance blend always
 * lands inside [p_sp, p_od] -- a price that can exist.
 *
 * Returns null when the pool's price is unknown. A pool with instanceCount 0 is
 * a different answer: 0, because it is scaled to nothing rather than unknown.
 *
 * MUST match backend: app/pricing/calculators.go:EC2PoolHourly()
 */
export function ec2PoolHourly(
	config: EC2PoolConfig,
	rates: AWSPriceRates,
): number | null {
	const instances = Math.max(0, config.instanceCount);

	const basis = ec2PoolBasis(config, rates);
	if (!basis) return null;

	// A ratio outside (0,1] is not a discount and is refused. Treating it as 1
	// prices spot as on-demand, which over-reports rather than inventing free
	// capacity.
	const rawRatio = rates.ec2?.spotRatio;
	const spotRatio =
		typeof rawRatio === "number" && rawRatio > 0 && rawRatio <= 1
			? rawRatio
			: 1.0;
	const spotHourly = basis.onDemandHourly * spotRatio;

	// On-demand base, in instances, by capacity type. An unrecognised value
	// falls through to on_demand -- the most expensive reading, so a typo in the
	// YAML never under-reports the bill.
	let base: number;
	switch (config.capacityType) {
		case "spot":
			base = 0;
			break;
		case "spot_with_base":
			base = Math.max(0, config.onDemandBase ?? 0);
			break;
		default:
			base = instances;
	}

	const onDemandInstances = Math.min(instances, base);
	const spotInstances = instances - onDemandInstances;

	return onDemandInstances * basis.onDemandHourly + spotInstances * spotHourly;
}

/**
 * Calculate monthly cost for one EC2 capacity pool:
 * instances x instance-hourly x 730, blended for spot.
 *
 * This is where an EC2-runtime service's money lives. The instances are billed
 * for existing, not for running tasks, so the cost belongs to the pool and the
 * services placed on it show $0 marginal. Summing a per-task figure across
 * those services instead would bill one instance once per task.
 *
 * Returns null when the pool's price is unknown -- render that as "price
 * unknown", never as $0.
 *
 * MUST match backend: app/pricing/calculators.go:CalculateEC2PoolPrice()
 */
export function calculateEC2PoolPrice(
	config: EC2PoolConfig,
	rates: AWSPriceRates,
): number | null {
	const hourly = ec2PoolHourly(config, rates);
	if (hourly === null) return null;
	return hourly * HOURS_PER_MONTH;
}

/**
 * Estimate runs per month from a schedule expression.
 * Supports rate() and cron() formats.
 */
function estimateRunsPerMonth(schedule: string): number {
	const rateMatch = schedule.match(/rate\((\d+)\s+(minute|hour|day)s?\)/i);
	if (rateMatch) {
		const value = Number.parseInt(rateMatch[1], 10);
		const unit = rateMatch[2].toLowerCase();
		if (unit === "minute") return (60 * 24 * 30) / value;
		if (unit === "hour") return (24 * 30) / value;
		if (unit === "day") return 30 / value;
	}
	// Default: assume once per day for cron or unknown
	return 30;
}

/**
 * Calculate monthly cost for a scheduled task (Fargate spot pricing).
 * Accounts for schedule frequency and per-run duration.
 */
export function calculateScheduledTaskPrice(
	config: ScheduledTaskConfig,
	rates: AWSPriceRates,
): number {
	const vCPU = config.cpu / 1024.0;
	const memoryGB = config.memory / 1024.0;
	const runtimeHours = (config.runtimeMinutes || 5) / 60.0;
	const runsPerMonth = estimateRunsPerMonth(config.schedule);

	const vCPUCostPerRun = vCPU * rates.fargate.vcpuHourly * runtimeHours;
	const memoryCostPerRun =
		memoryGB * rates.fargate.memoryGbHourly * runtimeHours;
	const costPerRun = vCPUCostPerRun + memoryCostPerRun;

	return costPerRun * runsPerMonth;
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
