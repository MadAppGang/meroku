import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React from "react";
import { expect, within } from "storybook/test";
import { Input } from "../../../ui/input";
import { editorDecorator, editorParameters } from "../editors/editorStoryKit";

const meta = {
	title: "UI/Input",
	component: Input,
	tags: ["!autodocs"],
	parameters: editorParameters,
	decorators: [editorDecorator],
	args: { "aria-label": "Field" },
} satisfies Meta<typeof Input>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
	args: { defaultValue: "My application" },
};

export const Placeholder: Story = {
	args: { placeholder: "Same as source" },
};

export const Code: Story = {
	args: { variant: "code", defaultValue: "/api/<*>" },
	play: async ({ canvasElement }) => {
		const field = within(canvasElement).getByRole("textbox", { name: "Field" });
		await expect(field.className).toContain("font-mono");
	},
};

export const CodeInvalid: Story = {
	args: {
		variant: "code",
		defaultValue: "/api/<*>/<*>",
		"aria-invalid": true,
	},
};

export const Disabled: Story = {
	args: { variant: "code", defaultValue: "/pricing", disabled: true },
};
