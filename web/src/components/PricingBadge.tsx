import type { PricingResponse } from "../hooks/use-pricing";
import { usePricingRates } from "../contexts/PricingContext";
import {
	calculateAuroraPrice,
	calculateAuroraMinPrice,
	calculateAuroraMaxPrice,
	calculateECSPrice,
	calculateRDSPrice,
	formatPrice,
	type AuroraConfig,
	type ECSConfig,
	type RDSConfig,
} from "../utils/awsPricing";
import { Badge } from "./ui/badge";

interface PricingBadgeProps {
	nodeType: string;
	pricing: PricingResponse | null;
	level?: "startup" | "scaleup" | "highload";
	serviceName?: string;
	configProperties?: any;
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
	const pricingData = pricing.nodes || pricing;

	// Special handling for PostgreSQL/Aurora database pricing
	if (nodeType === "postgres" && configProperties) {
		if (configProperties.aurora) {
			// Aurora Serverless v2 pricing - show min-max range based on capacity
			if (!rates) {
				// Show loading state while fetching rates
				return (
					<Badge
						variant="secondary"
						className="absolute -top-2 -right-2 bg-gray-600/90 text-gray-300 border-gray-700 text-xs px-1 py-0.5"
					>
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
					<Badge
						variant="secondary"
						className="absolute -top-2 -right-2 bg-green-600/90 text-white border-green-700 text-xs px-1 py-0.5"
					>
						{formatPrice(minPrice)}/mo
					</Badge>
				);
			}

			// Show price range (min-max)
			return (
				<Badge
					variant="secondary"
					className="absolute -top-2 -right-2 bg-green-600/90 text-white border-green-700 text-xs px-1 py-0.5"
				>
					{formatPrice(minPrice)}-{formatPrice(maxPrice)}/mo
				</Badge>
			);
		}

		// For standard RDS, also use unified calculator for consistency
		if (configProperties.instanceClass) {
			if (!rates) {
				return (
					<Badge
						variant="secondary"
						className="absolute -top-2 -right-2 bg-gray-600/90 text-gray-300 border-gray-700 text-xs px-1 py-0.5"
					>
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
				<Badge
					variant="secondary"
					className="absolute -top-2 -right-2 bg-green-600/90 text-white border-green-700 text-xs px-1 py-0.5"
				>
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

	// Special handling for backend service - calculate dynamically
	if (nodeType === "backend" && serviceName === "Backend service" && configProperties) {
		if (!rates) {
			return (
				<Badge
					variant="secondary"
					className="absolute -top-2 -right-2 bg-gray-600/90 text-gray-300 border-gray-700 text-xs px-1 py-0.5"
				>
					...
				</Badge>
			);
		}

		// Extract configuration from configProperties
		const cpu = typeof configProperties.cpu === 'string'
			? parseInt(configProperties.cpu)
			: (configProperties.cpu || 256);
		const memory = typeof configProperties.memory === 'string'
			? parseInt(configProperties.memory)
			: (configProperties.memory || 512);

		// If autoscaling is enabled, show price range (min to max)
		if (configProperties.autoscalingEnabled) {
			const minCount = configProperties.autoscalingMinCapacity || 1;
			const maxCount = configProperties.autoscalingMaxCapacity || 1;

			const minConfig: ECSConfig = { cpu, memory, desiredCount: minCount };
			const maxConfig: ECSConfig = { cpu, memory, desiredCount: maxCount };

			const minPrice = calculateECSPrice(minConfig, rates);
			const maxPrice = calculateECSPrice(maxConfig, rates);

			return (
				<Badge
					variant="secondary"
					className="absolute -top-2 -right-2 bg-green-600/90 text-white border-green-700 text-xs px-1 py-0.5"
				>
					{formatPrice(minPrice)}-{formatPrice(maxPrice)}/mo
				</Badge>
			);
		}

		// Fixed capacity - show single price
		const desiredCount = configProperties.desiredCount || 1;
		const ecsConfig: ECSConfig = { cpu, memory, desiredCount };
		const monthlyPrice = calculateECSPrice(ecsConfig, rates);

		return (
			<Badge
				variant="secondary"
				className="absolute -top-2 -right-2 bg-green-600/90 text-white border-green-700 text-xs px-1 py-0.5"
			>
				{formatPrice(monthlyPrice)}/mo
			</Badge>
		);
	}

	// Special handling for scheduled tasks
	if (nodeType === "scheduled-task" && serviceName) {
		const scheduledKey = `scheduled_${serviceName.toLowerCase()}`;
		if (pricingData[scheduledKey]) {
			const price = pricingData[scheduledKey].levels[level];
			if (price) {
				// For scheduled tasks, show more precision since costs are typically small
				const monthlyPrice = price.monthlyPrice;
				const displayPrice =
					monthlyPrice < 1
						? `$${monthlyPrice.toFixed(2)}/mo`
						: `$${monthlyPrice.toFixed(0)}/mo`;

				return (
					<Badge
						variant="secondary"
						className="absolute -top-2 -right-2 bg-green-600/90 text-white border-green-700 text-xs px-1 py-0.5"
					>
							{displayPrice}
					</Badge>
				);
			}
		}
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
					<Badge
						variant="secondary"
						className="absolute -top-2 -right-2 bg-green-600/90 text-white border-green-700 text-xs px-1 py-0.5"
					>
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
				<Badge
					variant="secondary"
					className="absolute -top-2 -right-2 bg-gray-600/90 text-gray-300 border-gray-700 text-xs px-1 py-0.5"
				>
					...
				</Badge>
			);
		}

		// Extract configuration from configProperties
		const cpu = typeof configProperties.cpu === 'string'
			? parseInt(configProperties.cpu)
			: (configProperties.cpu || 256);
		const memory = typeof configProperties.memory === 'string'
			? parseInt(configProperties.memory)
			: (configProperties.memory || 512);
		const desiredCount = configProperties.desiredCount || 1;

		const ecsConfig: ECSConfig = {
			cpu,
			memory,
			desiredCount,
		};

		// Calculate price using ECS calculator
		const monthlyPrice = calculateECSPrice(ecsConfig, rates);

		return (
			<Badge
				variant="secondary"
				className="absolute -top-2 -right-2 bg-green-600/90 text-white border-green-700 text-xs px-1 py-0.5"
			>
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
			<Badge
				variant="secondary"
				className="absolute -top-2 -right-2 bg-gray-600/90 text-gray-300 border-gray-700 text-xs px-1 py-0.5"
			>
				--.--/mo
			</Badge>
		);
	}

	if (!pricingKey) return null;

	const price = pricingData[pricingKey]?.levels[level];
	if (!price) {
		// Show placeholder if pricing key exists but no price for this level
		return (
			<Badge
				variant="secondary"
				className="absolute -top-2 -right-2 bg-gray-600/90 text-gray-300 border-gray-700 text-xs px-1 py-0.5"
			>
				--.--/mo
			</Badge>
		);
	}

	return (
		<Badge
			variant="secondary"
			className="absolute -top-2 -right-2 bg-green-600/90 text-white border-green-700 text-xs px-1 py-0.5"
		>
			${price.monthlyPrice.toFixed(0)}/mo
		</Badge>
	);
}
