import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React from "react";
import { expect, within } from "storybook/test";
import { EventFlow, type EventFlowProps } from "./event-flow";

/**
 * How events reach a workload's queue, one figure per queue set. Composed
 * on a workload's Events tab from `eventFlows` in utils/workloadEvents.ts.
 */
const meta = {
	title: "Recipes/Event Flow",
	component: EventFlow,
	tags: ["!autodocs"],
	parameters: { layout: "padded" },
	decorators: [
		(Story) => (
			<div className="dark bg-background p-6 text-foreground">
				<div className="mx-auto max-w-3xl">
					<Story />
				</div>
			</div>
		),
	],
} satisfies Meta<typeof EventFlow>;

export default meta;
type Story = StoryObj<typeof meta>;

const standardSet: Omit<EventFlowProps, "label" | "paths"> = {
	queue: {
		kind: "SQS queue",
		name: "demo-events-orders-dev",
		title: "arn:aws:sqs:us-east-1:000000000000:demo-events-orders-dev",
	},
	app: "orders",
	deliveryDlq: {
		kind: "Delivery DLQ",
		name: "demo-events-orders-delivery-dlq-dev",
	},
	dlq: { kind: "DLQ", name: "demo-events-orders-dlq-dev" },
	maxReceiveCount: 5,
	alerts: "demo-dev-alerts",
	idle: { kind: "Subscriptions", name: "none deliver here" },
};

const orderedSet: Omit<EventFlowProps, "label" | "paths"> = {
	queue: { kind: "Ordered SQS queue", name: "demo-events-orders-dev.fifo" },
	app: "orders",
	deliveryDlq: {
		kind: "Ordered delivery DLQ",
		name: "demo-events-orders-delivery-dlq-dev.fifo",
	},
	dlq: { kind: "Ordered DLQ", name: "demo-events-orders-dlq-dev.fifo" },
	maxReceiveCount: 5,
	alerts: "demo-dev-alerts",
	idle: { kind: "Ordered subscriber", name: "retired, delivers nothing" },
};

export const StandardBusRules: Story = {
	name: "Standard bus: rules",
	args: {
		...standardSet,
		label: "How events reach orders",
		paths: [{ bus: "demo-dev", delivery: "Rules", summary: "2 subscriptions" }],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("figure", { name: "How events reach orders" }),
		).toBeVisible();
		await expect(canvas.getByText("Rules")).toBeVisible();
		await expect(canvas.queryByText("Subscribers")).toBeNull();
	},
};

export const MixedBuses: Story = {
	name: "Enhanced and default bus: subscribers and rules",
	args: {
		...standardSet,
		label: "How events reach orders",
		paths: [
			{
				bus: "demo-dev",
				delivery: "Subscribers",
				summary: "2 subscriptions",
			},
			{ bus: "default", delivery: "Rules", summary: "1 subscription" },
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("Subscribers")).toBeVisible();
		await expect(canvas.getByText("Rules")).toBeVisible();
		await expect(canvas.getByText("default")).toBeVisible();
	},
};

export const OrderedActive: Story = {
	name: "Ordered queue: active",
	args: {
		...orderedSet,
		label: "How ordered events reach orders",
		paths: [
			{
				bus: "demo-dev",
				delivery: "Ordered subscriber",
				summary: "1 ordered subscription",
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("Ordered subscriber")).toBeVisible();
		await expect(canvas.queryByText("retired, delivers nothing")).toBeNull();
	},
};

export const OrderedRetired: Story = {
	name: "Ordered queue: retired, no path delivers",
	args: {
		...orderedSet,
		label: "How ordered events reach orders",
		paths: [],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("retired, delivers nothing")).toBeVisible();
		await expect(canvas.queryByText("EventBridge bus")).toBeNull();
	},
};

export const NothingDelivers: Story = {
	name: "Standard queue: no subscription delivers to it",
	args: {
		...standardSet,
		label: "How events reach orders",
		paths: [],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("none deliver here")).toBeVisible();
	},
};
