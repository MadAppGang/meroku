import type { Meta, StoryObj } from "@storybook/react-vite";
import { CheckCircle, Info, XCircle } from "lucide-react";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React from "react";
import { Alert, AlertDescription } from "../../../ui/alert";
import { editorDecorator, editorParameters } from "../editors/editorStoryKit";

const meta = {
	title: "UI/Alert",
	component: Alert,
	tags: ["!autodocs"],
	parameters: editorParameters,
	decorators: [editorDecorator],
} satisfies Meta<typeof Alert>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
	render: () => (
		<Alert>
			<Info className="h-4 w-4" />
			<AlertDescription>
				Events from AWS stay on the default bus.
			</AlertDescription>
		</Alert>
	),
};

export const Success: Story = {
	render: () => (
		<Alert variant="success">
			<CheckCircle className="h-4 w-4" />
			<AlertDescription>Event sent.</AlertDescription>
		</Alert>
	),
};

export const Danger: Story = {
	render: () => (
		<Alert variant="danger">
			<XCircle className="h-4 w-4" />
			<AlertDescription>Source and Detail Type are required.</AlertDescription>
		</Alert>
	),
};
