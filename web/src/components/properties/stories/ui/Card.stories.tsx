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
