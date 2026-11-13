import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React from "react";
import { Badge } from "../../../ui/badge";
import { editorDecorator, editorParameters } from "../editors/editorStoryKit";

const meta = {
	title: "UI/Badge",
	component: Badge,
	tags: ["!autodocs"],
	parameters: editorParameters,
	decorators: [editorDecorator],
} satisfies Meta<typeof Badge>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = { args: { children: "Active" } };
export const Secondary: Story = {
	args: { variant: "secondary", children: "Private" },
};
export const Outline: Story = {
	args: { variant: "outline", children: "No CORS" },
};
export const Destructive: Story = {
	args: { variant: "destructive", children: "any detail-type" },
};
export const Code: Story = {
	args: { variant: "code", children: "EVENT_QUEUE_URL" },
};
