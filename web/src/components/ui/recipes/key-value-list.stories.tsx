import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React from "react";
import { expect, within } from "storybook/test";
import { Badge } from "../badge";
import { Card, CardTitle } from "../card";
import { KeyValue, KeyValueList } from "./key-value-list";

/**
 * Labelled facts, one per row. Composed for a workload's event summary
 * (Bus, Env vars) and for each event processor on the Test Event tab.
 */
const meta = {
	title: "Recipes/Key Value List",
	component: KeyValueList,
	tags: ["!autodocs"],
	parameters: { layout: "padded" },
	decorators: [
		(Story) => (
			<div className="dark bg-background p-6 text-foreground">
				<div className="mx-auto max-w-xl">
					<Story />
				</div>
			</div>
		),
	],
} satisfies Meta<typeof KeyValueList>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Summary: Story = {
	name: "Summary with tags",
	render: () => (
		<KeyValueList>
			<KeyValue label="Bus" variant="tags">
				<Badge variant="code">demo-dev</Badge>
				<Badge variant="outline">Enhanced</Badge>
			</KeyValue>
			<KeyValue label="Env vars" variant="tags">
				<Badge variant="code">EVENT_SOURCE=demo.orders</Badge>
				<Badge variant="code">EVENT_BUS_NAME=demo-dev</Badge>
				<Badge variant="code">EVENT_QUEUE_URL</Badge>
			</KeyValue>
		</KeyValueList>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("Bus")).toBeVisible();
		await expect(canvas.getByText("EVENT_QUEUE_URL")).toBeVisible();
	},
};

export const Values: Story = {
	name: "Plain and code values",
	render: () => (
		<KeyValueList>
			<KeyValue label="Kind">Enhanced bus</KeyValue>
			<KeyValue label="Name" variant="code">
				demo-dev
			</KeyValue>
		</KeyValueList>
	),
};

export const CompactInInsetCard: Story = {
	name: "Compact, in an inset card",
	render: () => (
		<Card size="inset">
			<CardTitle variant="item">order-processor</CardTitle>
			<KeyValueList size="compact">
				<KeyValue label="Sources">demo.orders, demo.payments</KeyValue>
				<KeyValue label="Detail types">order.paid</KeyValue>
				<KeyValue label="Rule" variant="code">
					demo-order-processor-dev
				</KeyValue>
			</KeyValueList>
		</Card>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("demo-order-processor-dev")).toBeVisible();
	},
};
