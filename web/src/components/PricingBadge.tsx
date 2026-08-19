import { Server } from "lucide-react";
import { usePricingRates } from "../contexts/PricingContext";
import {
	COMPUTE_POOL_NODE_PREFIX,
	computePoolNameFromKey,
	DEFAULT_COMPUTE_POOL,
	type NodePricing,
	type PricingResponse,
} from "../hooks/use-pricing";
import type { NodeConfigValues } from "../types";
import {
	type AuroraConfig,
	calculateAuroraMaxPrice,
	calculateAuroraMinPrice,
	calculateECSPrice,
	calculateRDSPrice,
	calculateScheduledTaskPrice,
	type ECSConfig,
	formatPrice,
	type RDSConfig,
	type ScheduledTaskConfig,
} from "../utils/awsPricing";
import { Badge } from "./ui/badge";

type PriceLevel = "startup" | "scaleup" | "highload";

/**
 * The badge's three appearances, named once so a caller never restyles one.
 *
 * Position lives here because the badge is a corner ornament of the canvas node
 * that renders it and nothing else ever places it.
 */
const BADGE_BASE = "absolute -top-2 -right-2 text-xs px-1 py-0.5";
/** A figure we stand behind. */
const BADGE_PRICE = `${BADGE_BASE} bg-green-600/90 text-white border-green-700`;
/** No figure available. Never rendered as $0 — unknown is not free. */
const BADGE_UNKNOWN = `${BADGE_BASE} bg-gray-600/90 text-gray-300 border-gray-700`;
/** Costs money, but the money is billed on another node. */
const BADGE_ATTRIBUTED = `${BADGE_BASE} bg-sky-600/90 text-white border-sky-700`;

/**
 * The capacity pool an EC2-placed workload is billed through, or null when the
 * workload is not on EC2 at all.
 *
 * The live config outranks the pricing response deliberately: /api/pricing is
 * refetched on environment change, so straight after a save its `runtime` can
 * still read "fargate" for a workload the YAML has already moved to EC2 — and a
 * per-task Fargate figure on an instance-hour workload is the one answer worse
 * than no figure.
 */
function ec2PoolFor(
	configProperties: NodeConfigValues | undefined,
	node: NodePricing | undefined,
): string | null {
	const placedOnEC2 =
		configProperties?.runtime === "ec2" ||
		node?.runtime === "ec2" ||
		Boolean(node?.costAttributedTo);
	if (!placedOnEC2) return null;

	if (configProperties?.runtime === "ec2" && configProperties.computePool) {
		return configProperties.computePool;
	}
	if (node?.costAttributedTo) {
		return computePoolNameFromKey(node.costAttributedTo);
	}
	// An EC2 workload naming no pool runs on the one generation synthesizes.
	return configProperties?.computePool || DEFAULT_COMPUTE_POOL;
}

interface ComputePoolTotal {
	monthly: number;
	/** Pools with a real figure in the sum. */
	priced: number;
	/** Pools the server could not price, and which are therefore NOT in it. */
	unpriced: number;
}

/**
 * What this environment's EC2 capacity pools cost at the given level.
 *
 * Reads the `compute_pool_*` nodes rather than summing the tasks placed on
 * them, because EC2 is billed per instance-hour: a pool at min_size 1 costs a
 * full instance-hour every hour with nothing deployed on it, and adding up
 * per-task prices would bill one instance once per task (FR-56).
 */
function totalComputePoolCost(
	nodes: Record<string, NodePricing>,
	level: PriceLevel,
): ComputePoolTotal | null {
	let monthly = 0;
	let priced = 0;
	let unpriced = 0;

	for (const [key, node] of Object.entries(nodes)) {
		if (!key.startsWith(COMPUTE_POOL_NODE_PREFIX)) continue;
		const price = node.levels?.[level];
		// The server sends $0 with details.price === "unknown" when it holds no
		// rate for the pool's instance types. That is not free, so it is counted
		// apart from the sum rather than folded into it as a zero.
		if (!price || price.details?.price === "unknown") {
			unpriced += 1;
			continue;
		}
		monthly += price.monthlyPrice;
		priced += 1;
	}

	if (priced === 0 && unpriced === 0) return null;
	return { monthly, priced, unpriced };
}

function poolCountLabel(count: number): string {
	return count === 1 ? "1 pool" : `${count} pools`;
}

interface PricingBadgeProps {
	nodeType: string;
	pricing: PricingResponse | null;
	level?: PriceLevel;
	serviceName?: string;
	configProperties?: NodeConfigValues;
}

export function PricingBadge({
	nodeType,
	pricing,
	level = "startup",
	serviceName,
	configProperties,
}: PricingBadgeProps) {
	// Get pricing rates from context (unified source of truth)
	const rates = usePricingRates();

	if (!pricing) return null;

	// Handle both pricing.nodes and direct pricing object structures
	const pricingData: Record<string, NodePricing> =
		pricing.nodes ?? (pricing as unknown as Record<string, NodePricing>);

	// Special handling for PostgreSQL/Aurora database pricing
	if (nodeType === "postgres" && configProperties) {
		if (configProperties.aurora) {
			// Aurora Serverless v2 pricing - show min-max range based on capacity
			if (!rates) {
				// Show loading state while fetching rates
				return (
					<Badge variant="secondary" className={BADGE_UNKNOWN}>
						...
					</Badge>
				);
			}

			const config: AuroraConfig = {
				minCapacity: configProperties.minCapacity ?? 0,
				maxCapacity: configProperties.maxCapacity || 1,
				level: level,
			};

			// Calculate price range based on min and max capacity
			const minPrice = calculateAuroraMinPrice(config, rates);
			const maxPrice = calculateAuroraMaxPrice(config, rates);

			// If min and max are the same, show single price
			if (config.minCapacity === config.maxCapacity) {
				return (
					<Badge variant="secondary" className={BADGE_PRICE}>
						{formatPrice(minPrice)}/mo
					</Badge>
				);
			}

			// Show price range (min-max)
			return (
				<Badge variant="secondary" className={BADGE_PRICE}>
					{formatPrice(minPrice)}-{formatPrice(maxPrice)}/mo
				</Badge>
			);
		}

		// For standard RDS, also use unified calculator for consistency
		if (configProperties.instanceClass) {
			if (!rates) {
				return (
					<Badge variant="secondary" className={BADGE_UNKNOWN}>
						...
					</Badge>
				);
			}

			const rdsConfig: RDSConfig = {
				instanceClass: configProperties.instanceClass || "db.t4g.micro",
				allocatedStorage: configProperties.allocatedStorage || 20,
				multiAz: configProperties.multiAz || false,
			};

			// Use unified calculator (matches backend exactly)
			const monthlyPrice = calculateRDSPrice(rdsConfig, rates);

			return (
				<Badge variant="secondary" className={BADGE_PRICE}>
					{formatPrice(monthlyPrice)}/mo
				</Badge>
			);
		}
	}

	// Map node types to pricing keys (matching API response keys)
	const pricingMap: Record<string, string> = {
		ecs: "vpc", // Show VPC pricing on ECS node
		backend: "backend", // Backend service pricing
		service: "ecs",
		postgres: "rds",
		aurora: "rds",
		s3: "s3",
		cloudwatch: "cloudwatch",
		cognito: "cognito",
		alb: "alb",
		nat_gateway: "nat_gateway",
		"api-gateway": "api_gateway", // Fixed: api-gateway -> api_gateway
		eventbridge: "eventbridge",
		lambda: "lambda",
		ses: "ses",
		sqs: "sqs",
		ssm: "ssm",
		secrets: "secrets",
		route53: "route53", // Added route53
		ecr: "ecr", // Added ecr
		"scheduled-task": "scheduled", // Added scheduled-task (handled specially above)
		"event-task": "event", // Added event-task
		xray: "xray",
		efs: "efs",
		sns: "sns",
		waf: "waf",
		"secrets-manager": "secrets",
	};

	// ---------------------------------------------------------------------
	// EC2 placement, decided BEFORE any Fargate arithmetic runs.
	//
	// An EC2-placed workload is billed nothing per task: its instances are paid
	// for by the hour on its capacity pool, which is why the server reports it
	// at $0 marginal with costAttributedTo pointing at that pool. Every
	// calculator below is Fargate-only, so letting one of them run here would
	// print a confident per-task price for exactly the workloads that are not
	// billed per task — and adding that price to the pool's own would bill one
	// instance once per task placed on it.
	// ---------------------------------------------------------------------
	if (nodeType === "backend" || nodeType === "service") {
		const nodeKey = nodeType === "backend" ? "backend" : (serviceName ?? "");
		const pool = ec2PoolFor(configProperties, pricingData[nodeKey]);
		if (pool) {
			return (
				<Badge
					variant="secondary"
					className={BADGE_ATTRIBUTED}
					title={`Billed as instance-hours on compute pool "${pool}", not per task. This workload adds no cost of its own — the pool is charged whether or not tasks are placed on it.`}
				>
					<Server aria-hidden="true" />
					via {pool}
				</Badge>
			);
		}
	}

	// The ECS cluster node carries the EC2 capacity behind it: the pools are
	// where the instance-hours of every EC2 workload above are actually billed,
	// and without this they would appear nowhere on the canvas at all.
	if (nodeType === "ecs") {
		const pools = totalComputePoolCost(pricingData, level);
		if (pools && pools.priced === 0) {
			return (
				<Badge
					variant="secondary"
					className={BADGE_UNKNOWN}
					title={`EC2 capacity: ${poolCountLabel(pools.unpriced)} with no price for the configured instance types in this region.`}
				>
					--.--/mo
				</Badge>
			);
		}
		if (pools) {
			const unpricedNote =
				pools.unpriced > 0
					? ` ${poolCountLabel(pools.unpriced)} could not be priced and is not in this total, so the real figure is higher.`
					: "";
			return (
				<Badge
					variant="secondary"
					className={BADGE_PRICE}
					title={`EC2 capacity: ${poolCountLabel(pools.priced)} billed by instance-hour at the ${level} level, charged whether or not tasks are placed on them. Tasks add nothing further.${unpricedNote}`}
				>
					${pools.monthly.toFixed(0)}
					{pools.unpriced > 0 ? "+" : ""}/mo
				</Badge>
			);
		}
		// No pools at all: fall through to the VPC mapping, which is $0 and true.
	}

	// Special handling for backend service - calculate dynamically
	if (
		nodeType === "backend" &&
		serviceName === "Backend service" &&
		configProperties
	) {
		if (!rates) {
			return (
				<Badge variant="secondary" className={BADGE_UNKNOWN}>
					...
				</Badge>
			);
		}

		// Extract configuration from configProperties
		const cpu =
			typeof configProperties.cpu === "string"
				? parseInt(configProperties.cpu, 10)
				: configProperties.cpu || 256;
		const memory =
			typeof configProperties.memory === "string"
				? parseInt(configProperties.memory, 10)
				: configProperties.memory || 512;

		// If autoscaling is enabled, show price range (min to max)
		if (configProperties.autoscalingEnabled) {
			const minCount = configProperties.autoscalingMinCapacity || 1;
			const maxCount = configProperties.autoscalingMaxCapacity || 1;

			const minConfig: ECSConfig = { cpu, memory, desiredCount: minCount };
			const maxConfig: ECSConfig = { cpu, memory, desiredCount: maxCount };

			const minPrice = calculateECSPrice(minConfig, rates);
			const maxPrice = calculateECSPrice(maxConfig, rates);

			return (
				<Badge variant="secondary" className={BADGE_PRICE}>
					{formatPrice(minPrice)}-{formatPrice(maxPrice)}/mo
				</Badge>
			);
		}

		// Fixed capacity - show single price
		const desiredCount = configProperties.desiredCount ?? 1;
		const ecsConfig: ECSConfig = { cpu, memory, desiredCount };
		const monthlyPrice = calculateECSPrice(ecsConfig, rates);

		return (
			<Badge variant="secondary" className={BADGE_PRICE}>
				{formatPrice(monthlyPrice)}/mo
			</Badge>
		);
	}

	// Special handling for scheduled tasks - calculate dynamically from configProperties
	if (nodeType === "scheduled-task" && configProperties) {
		if (!rates) {
			return (
				<Badge variant="secondary" className={BADGE_UNKNOWN}>
					...
				</Badge>
			);
		}

		const cpu =
			typeof configProperties.cpu === "string"
				? parseInt(configProperties.cpu, 10)
				: configProperties.cpu || 256;
		const memory =
			typeof configProperties.memory === "string"
				? parseInt(configProperties.memory, 10)
				: configProperties.memory || 512;
		const schedule = configProperties.schedule || "rate(1 day)";

		const taskConfig: ScheduledTaskConfig = { cpu, memory, schedule };
		const monthlyPrice = calculateScheduledTaskPrice(taskConfig, rates);
		const displayPrice =
			monthlyPrice < 1
				? `$${monthlyPrice.toFixed(2)}/mo`
				: `${formatPrice(monthlyPrice)}/mo`;

		return (
			<Badge variant="secondary" className={BADGE_PRICE}>
				{displayPrice}
			</Badge>
		);
	}

	// Special handling for event processor tasks
	if (nodeType === "event-task" && serviceName) {
		const eventKey = `event_${serviceName.toLowerCase()}`;
		if (pricingData[eventKey]) {
			const price = pricingData[eventKey].levels[level];
			if (price) {
				// For event tasks, show more precision since costs are typically small
				const monthlyPrice = price.monthlyPrice;
				const displayPrice =
					monthlyPrice < 1
						? `$${monthlyPrice.toFixed(2)}/mo`
						: `$${monthlyPrice.toFixed(0)}/mo`;

				return (
					<Badge variant="secondary" className={BADGE_PRICE}>
						{displayPrice}
					</Badge>
				);
			}
		}
	}

	// For other services, calculate dynamically from config properties
	if (nodeType === "service" && serviceName && configProperties) {
		if (!rates) {
			return (
				<Badge variant="secondary" className={BADGE_UNKNOWN}>
					...
				</Badge>
			);
		}

		// Extract configuration from configProperties
		const cpu =
			typeof configProperties.cpu === "string"
				? parseInt(configProperties.cpu, 10)
				: configProperties.cpu || 256;
		const memory =
			typeof configProperties.memory === "string"
				? parseInt(configProperties.memory, 10)
				: configProperties.memory || 512;
		const desiredCount = configProperties.desiredCount ?? 1;

		const ecsConfig: ECSConfig = {
			cpu,
			memory,
			desiredCount,
		};

		// Calculate price using ECS calculator
		const monthlyPrice = calculateECSPrice(ecsConfig, rates);

		return (
			<Badge variant="secondary" className={BADGE_PRICE}>
				{formatPrice(monthlyPrice)}/mo
			</Badge>
		);
	}

	// Use the type mapping
	const pricingKey = pricingMap[nodeType];

	// If we have a mapping but no pricing data, show placeholder
	if (pricingKey && !pricingData[pricingKey]) {
		// Show placeholder for mapped services without pricing
		return (
			<Badge variant="secondary" className={BADGE_UNKNOWN}>
				--.--/mo
			</Badge>
		);
	}

	if (!pricingKey) return null;

	const price = pricingData[pricingKey]?.levels[level];
	if (!price) {
		// Show placeholder if pricing key exists but no price for this level
		return (
			<Badge variant="secondary" className={BADGE_UNKNOWN}>
				--.--/mo
			</Badge>
		);
	}

	return (
		<Badge variant="secondary" className={BADGE_PRICE}>
			${price.monthlyPrice.toFixed(0)}/mo
		</Badge>
	);
}
