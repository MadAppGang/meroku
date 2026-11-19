import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React from "react";
import {
	Card,
	CardContent,
	CardDescription,
	CardHeader,
	CardTitle,
} from "../../../ui/card";
import { editorDecorator, editorParameters } from "../editors/editorStoryKit";

const meta = {
	title: "UI/Card",
	component: Card,
	tags: ["!autodocs"],
	parameters: editorParameters,
	decorators: [editorDecorator],
} satisfies Meta<typeof Card>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
	render: () => (
		<Card>
			<CardHeader>
				<CardTitle>Shared folders</CardTitle>
				<CardDescription>Opened to the workloads you choose.</CardDescription>
			</CardHeader>
			<CardContent>Content below a header.</CardContent>
		</Card>
	),
};

export const CodeTitle: Story = {
	name: "Code title and description",
	render: () => (
		<Card>
			<CardHeader>
				<CardTitle variant="code">shared/uploads/</CardTitle>
				<CardDescription variant="code">
					DATA_BUCKET_SHARED_UPLOADS
				</CardDescription>
			</CardHeader>
			<CardContent>Content below a header.</CardContent>
		</Card>
	),
};

export const StandaloneContent: Story = {
	name: "Content without a header",
	render: () => (
		<Card>
			<CardContent inset="standalone">
				A card whose only section is its content.
			</CardContent>
		</Card>
	),
};

export const Compact: Story = {
	name: "Compact: one entry of a list",
	render: () => (
		<div className="space-y-2">
			<Card size="compact">
				<p className="text-sm">Shared with environment staging</p>
				<p className="text-sm text-muted-foreground">Send events only</p>
			</Card>
			<Card size="compact">
				<p className="text-sm">Shared with account 000000000000</p>
				<p className="text-sm text-muted-foreground">Send and subscribe</p>
			</Card>
		</div>
	),
};

export const Flush: Story = {
	name: "Flush: sections bring their own padding",
	render: () => (
		<Card size="flush">
			<div className="border-b p-4 text-sm">A heading section</div>
			<div className="p-4 text-sm text-muted-foreground">
				A body section below the divider.
			</div>
		</Card>
	),
};

export const Inset: Story = {
	name: "Inset: a recessed panel inside a section",
	render: () => (
		<Card>
			<CardHeader>
				<CardTitle>Configured event processors</CardTitle>
			</CardHeader>
			<CardContent className="space-y-3">
				<Card size="inset">
					<CardTitle variant="item">order-processor</CardTitle>
					<CardDescription>Runs when an order is paid.</CardDescription>
				</Card>
				<Card size="inset">
					<CardTitle variant="item">refund-processor</CardTitle>
					<CardDescription>Runs when a refund is requested.</CardDescription>
				</Card>
			</CardContent>
		</Card>
	),
};

export const ItemTitle: Story = {
	name: "Item title in a compact card",
	render: () => (
		<Card size="compact">
			<CardTitle variant="item">Shared with environment staging</CardTitle>
			<CardDescription>Send events only</CardDescription>
		</Card>
	),
};
