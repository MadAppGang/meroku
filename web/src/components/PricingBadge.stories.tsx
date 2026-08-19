import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React from "react";
import { expect, within } from "storybook/test";
import { PricingProvider } from "../contexts/PricingContext";
import type {
	LevelPrice,
	NodePricing,
	PricingResponse,
} from "../hooks/use-pricing";
import type { AWSPriceRates } from "../services/pricingService";
import { PricingBadge } from "./PricingBadge";

/**
 * Rates as they arrive from /api/pricing/rates, seeded into sessionStorage so
 * `PricingProvider` reads them synchronously instead of firing a request
 * Storybook has no server for.
 *
 * The key mirrors the private `STORAGE_KEY` in `services/pricingService.ts`,
 * which is bumped whenever `AWSPriceRates` grows a field. If it moves ahead of
 * this file the seed simply misses and the Fargate story renders the "..."
 * pending badge — a real state of the component, never a crash.
 */
const RATES_STORAGE_KEY = "aws_pricing_rates_v2";

const RATES: AWSPriceRates = {
	region: "us-east-1",
	lastUpdate: "2026-08-19T00:00:00Z",
	source: "aws_api",
	pricingDate: "2026-08-19",
	rds: { "db.t4g.micro": 0.016, "db.t3.micro": 0.018 },
	aurora: { acuHourly: 0.12, storageGbMonth: 0.1, ioRequestsPerM: 0.2 },
	fargate: { vcpuHourly: 0.04048, memoryGbHourly: 0.004445 },
	ec2: {
		onDemandHourly: { "m7i-flex.large": 0.0958, "m6i.large": 0.096 },
		spotRatio: 0.35,
	},
	storage: { gp3PerGbMonth: 0.08, gp2PerGbMonth: 0.1 },
	s3: { standardPerGbMonth: 0.023, requestsPer1000: 0.0004 },
	alb: { hourlyPrice: 0.0225, lcuPrice: 0.008 },
	apiGateway: { requestsPerMillion: 3.5 },
	natGateway: { hourlyPrice: 0.045, dataPerGbMonth: 0.045 },
	cloudWatch: { logsIngestionPerGb: 0.5, metricsPerMetric: 0.3 },
	route53: { hostedZonePerMonth: 0.5, queriesPerMillion: 0.4 },
	cognito: { mauPrice: 0.0055, freeMAUs: 50000 },
	ses: { per1000Emails: 0.1 },
	eventBridge: { eventsPerMillion: 1 },
	ecr: { storagePerGbMonth: 0.1 },
};

/** The same figure at all three levels, which is all these stories need. */
function levels(price: LevelPrice) {
	return { startup: price, scaleup: price, highload: price };
}

function fargateService(name: string, monthlyPrice: number): NodePricing {
	return {
		serviceName: `Service: ${name}`,
		serviceType: "compute",
		runtime: "fargate",
		levels: levels({
			monthlyPrice,
			hourlyPrice: monthlyPrice / 730,
			details: { runtime: "Fargate", vCPU: "0.25 vCPU", memory: "512 MB" },
		}),
	};
}

/** $0 marginal with the money pointed at a pool — the server's EC2 shape. */
function ec2Service(name: string, pool: string): NodePricing {
	return {
		serviceName: `Service: ${name}`,
		serviceType: "compute",
		runtime: "ec2",
		costAttributedTo: `compute_pool_${pool}`,
		levels: levels({
			monthlyPrice: 0,
			hourlyPrice: 0,
			details: {
				runtime: "EC2",
				compute_pool: pool,
				billing: "instance-hours, billed on the capacity pool",
				note: "$0 marginal: tasks share instances the pool already pays for",
			},
		}),
	};
}

function pool(
	name: string,
	monthlyPrice: number,
	attributedFrom: string[],
): NodePricing {
	return {
		serviceName: `Compute Pool: ${name}`,
		serviceType: "compute",
		runtime: "ec2",
		attributedFrom,
		levels: levels({
			monthlyPrice,
			hourlyPrice: monthlyPrice / 730,
			details: {
				runtime: "EC2",
				pool: name,
				instances: "1",
				basis: "min_size",
				billing: "instance-hours (billed whether or not tasks are placed)",
			},
		}),
	};
}

/** A pool whose instance types have no rate: unknown, and never $0. */
function unpricedPool(name: string): NodePricing {
	return {
		serviceName: `Compute Pool: ${name}`,
		serviceType: "compute",
		runtime: "ec2",
		levels: levels({
			monthlyPrice: 0,
			hourlyPrice: 0,
			details: { runtime: "EC2", pool: name, price: "unknown" },
		}),
	};
}

const vpcFree: NodePricing = {
	serviceName: "VPC",
	serviceType: "networking",
	levels: levels({
		monthlyPrice: 0,
		hourlyPrice: 0,
		details: { cost: "Free" },
	}),
};

function response(nodes: Record<string, NodePricing>): PricingResponse {
	return { region: "us-east-1", nodes: { vpc: vpcFree, ...nodes } };
}

const meta = {
	title: "Components/PricingBadge",
	component: PricingBadge,
	parameters: {
		a11y: {
			config: {
				// Same exemption as ServiceNode: these badges are corner ornaments
				// on a coloured canvas node, and their contrast is inherited from
				// that surface rather than decided here.
				rules: [{ id: "color-contrast", enabled: false }],
			},
		},
		layout: "centered",
	},
	decorators: [
		(Story) => {
			sessionStorage.setItem(
				RATES_STORAGE_KEY,
				JSON.stringify({ rates: RATES, timestamp: Date.now() }),
			);
			return (
				<PricingProvider>
					{/* Stands in for the canvas node the badge hangs off. */}
					<div className="relative w-56 rounded-lg border border-gray-700 bg-gray-800 p-4 text-sm text-gray-200">
						Canvas node
						<Story />
					</div>
				</PricingProvider>
			);
		},
	],
	tags: ["autodocs"],
} satisfies Meta<typeof PricingBadge>;

export default meta;
type Story = StoryObj<typeof meta>;

/** Fargate is billed per task, so a per-task figure is the honest answer. */
export const FargateService: Story = {
	args: {
		nodeType: "service",
		serviceName: "api",
		pricing: response({ api: fargateService("api", 12) }),
		configProperties: { cpu: 256, memory: 512, desiredCount: 1 },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// A dollar figure, whatever the seeded rates make it.
		await expect(canvas.getByText(/^\$[\d.,]+\/mo$/)).toBeVisible();
	},
};

/**
 * The defect this component was fixed for: an EC2-placed service used to show a
 * Fargate price. Its instances are billed by the hour on its pool, so the badge
 * names the pool instead of inventing a per-task number.
 */
export const Ec2ServiceBilledViaPool: Story = {
	args: {
		nodeType: "service",
		serviceName: "worker",
		pricing: response({
			worker: ec2Service("worker", "batch"),
			compute_pool_batch: pool("batch", 70, ["worker"]),
		}),
		configProperties: {
			cpu: 512,
			memory: 1024,
			desiredCount: 2,
			runtime: "ec2",
			computePool: "batch",
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("via batch")).toBeVisible();
		// The whole point: no per-task Fargate figure anywhere on this node.
		await expect(canvas.queryByText(/\$/)).toBeNull();
	},
};

/**
 * Attribution taken from the pricing response alone — the case where the YAML
 * names no pool and generation synthesizes `default` (FR-58).
 */
export const Ec2BackendOnSynthesizedDefaultPool: Story = {
	args: {
		nodeType: "backend",
		serviceName: "Backend service",
		pricing: response({
			backend: ec2Service("backend", "default"),
			compute_pool_default: pool("default", 70, ["backend"]),
		}),
		configProperties: { cpu: 256, memory: 512, desiredCount: 1 },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("via default")).toBeVisible();
		await expect(canvas.queryByText(/\$/)).toBeNull();
	},
};

/**
 * The cluster carries the instance-hours. This is where the money of every EC2
 * workload above actually lands, and it is charged whether or not tasks run.
 */
export const EcsClusterWithEc2Pools: Story = {
	args: {
		nodeType: "ecs",
		serviceName: "Amazon ECS Cluster",
		pricing: response({
			compute_pool_batch: pool("batch", 70, ["worker"]),
			compute_pool_web: pool("web", 140, ["backend"]),
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("$210/mo")).toBeVisible();
	},
};

/** A pool with no rate for its instance types is excluded and marked, not zeroed. */
export const EcsClusterWithUnpricedPool: Story = {
	args: {
		nodeType: "ecs",
		serviceName: "Amazon ECS Cluster",
		pricing: response({
			compute_pool_batch: pool("batch", 70, ["worker"]),
			compute_pool_gpu: unpricedPool("gpu"),
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// "+" because the unpriced pool is missing from the sum, not free.
		await expect(canvas.getByText("$70+/mo")).toBeVisible();
	},
};

/** Every pool unpriced: unknown, shown as unknown. */
export const EcsClusterAllPoolsUnpriced: Story = {
	args: {
		nodeType: "ecs",
		serviceName: "Amazon ECS Cluster",
		pricing: response({ compute_pool_gpu: unpricedPool("gpu") }),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("--.--/mo")).toBeVisible();
	},
};

/** No pools at all: the cluster itself costs nothing, and says so. */
export const EcsClusterFargateOnly: Story = {
	args: {
		nodeType: "ecs",
		serviceName: "Amazon ECS Cluster",
		pricing: response({ backend: fargateService("backend", 12) }),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("$0/mo")).toBeVisible();
	},
};
