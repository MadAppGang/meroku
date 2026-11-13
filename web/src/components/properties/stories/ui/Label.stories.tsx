import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React, { useId } from "react";
import { Input } from "../../../ui/input";
import { Label } from "../../../ui/label";
import { editorDecorator, editorParameters } from "../editors/editorStoryKit";

const meta = {
	title: "UI/Label",
	component: Label,
	tags: ["!autodocs"],
	parameters: editorParameters,
	decorators: [editorDecorator],
} satisfies Meta<typeof Label>;

export default meta;
type Story = StoryObj<typeof meta>;

function LabelledInput({
	size,
	text,
}: {
	size: "default" | "lg";
	text: string;
}) {
	const id = useId();
	return (
		<div className="space-y-2">
			<Label htmlFor={id} size={size}>
				{text}
			</Label>
			<Input id={id} defaultValue="1" />
		</div>
	);
}

export const Default: Story = {
	render: () => <LabelledInput size="default" text="Storage cap (GB)" />,
};

export const Large: Story = {
	render: () => <LabelledInput size="lg" text="Enable Valkey" />,
};
