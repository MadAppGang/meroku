import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React from "react";
import { expect, within } from "storybook/test";
import { Textarea } from "../../../ui/textarea";
import { editorDecorator, editorParameters } from "../editors/editorStoryKit";

const meta = {
	title: "UI/Textarea",
	component: Textarea,
	tags: ["!autodocs"],
	parameters: editorParameters,
	decorators: [editorDecorator],
	args: { "aria-label": "Field" },
} satisfies Meta<typeof Textarea>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
	args: { defaultValue: "Describe the change." },
};

export const Code: Story = {
	args: {
		variant: "code",
		defaultValue: "dynamodb:GetItem\ndynamodb:PutItem",
	},
	play: async ({ canvasElement }) => {
		const field = within(canvasElement).getByRole("textbox", { name: "Field" });
		await expect(field.className).toContain("font-mono");
	},
};

export const CodeInvalid: Story = {
	args: { variant: "code", defaultValue: "", "aria-invalid": true },
};

export const Disabled: Story = {
	args: { variant: "code", defaultValue: '{"orderId": "123"}', disabled: true },
};
