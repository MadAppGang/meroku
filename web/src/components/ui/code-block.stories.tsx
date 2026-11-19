import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React from "react";
import { expect, within } from "storybook/test";
import { Card } from "./card";
import { CodeBlock } from "./code-block";

const EXAMPLE = `{
  "eventType": "order.processed",
  "orderId": "ORD-123456",
  "amount": 99.99,
  "currency": "USD"
}`;

/**
 * Code shown as written, such as an example event. One story per surface;
 * an editable block is a Textarea with `variant="code"`.
 */
const meta = {
	title: "Components/CodeBlock",
	component: CodeBlock,
	parameters: { layout: "padded" },
	args: { children: EXAMPLE },
	decorators: [
		(Story) => (
			<div className="dark bg-background p-6 text-foreground">
				<div className="mx-auto max-w-xl">
					<Story />
				</div>
			</div>
		),
	],
} satisfies Meta<typeof CodeBlock>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Muted: Story = {
	name: "Muted (on a card)",
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText(/"orderId": "ORD-123456"/)).toBeVisible();
	},
};

export const WithTitle: Story = {
	name: "With a title",
	args: { title: "Business event" },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("Business event")).toBeVisible();
	},
};

export const Plain: Story = {
	name: "Plain (inside an inset card)",
	render: (args) => (
		<Card size="inset">
			<CodeBlock {...args} surface="plain" />
		</Card>
	),
};

export const LongLine: Story = {
	name: "A long line scrolls",
	args: {
		title: "One long line",
		children: `{"message": "${"a very long value ".repeat(12).trim()}"}`,
	},
};
