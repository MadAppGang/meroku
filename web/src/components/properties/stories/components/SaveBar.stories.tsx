import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React from "react";
import { fn } from "storybook/test";
import { PropertySaveBar } from "../../PropertyPrimitives";
import { fullscreenParameters, primitiveDecorator } from "../storyConfig";

const meta = {
	title: "Components/Save Bar",
	component: PropertySaveBar,
	tags: ["!autodocs"],
	args: {
		state: "dirty",
		onDiscard: fn(),
		onApply: fn(),
	},
	parameters: fullscreenParameters,
	decorators: [primitiveDecorator],
	render: (args) => (
		<div
			className="mp-primitive-demo mp-compact-panel"
			data-surface="light"
			data-theme="light"
		>
			<PropertySaveBar {...args} />
		</div>
	),
} satisfies Meta<typeof PropertySaveBar>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Clean: Story = { args: { state: "clean" } };
export const Unsaved: Story = { args: { state: "dirty" } };
export const Saving: Story = { args: { state: "saving" } };
export const Saved: Story = { args: { state: "saved" } };
export const SaveError: Story = { args: { state: "error" }, name: "Error" };
