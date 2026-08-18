import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React from "react";
import { PropertyPanelHeader, PropertyPanelMeta } from "../../PropertyLayouts";
import { fullscreenParameters, primitiveDecorator } from "../storyConfig";

const meta = {
	title: "Layouts/Panel Meta",
	component: PropertyPanelMeta,
	tags: ["!autodocs"],
	args: {
		status: "Running",
		context: "ap-southeast-2",
		tone: "success",
	},
	parameters: fullscreenParameters,
	decorators: [primitiveDecorator],
	render: (args) => (
		<PropertyPanelHeader
			name="terminator"
			meta={<PropertyPanelMeta {...args} />}
		/>
	),
} satisfies Meta<typeof PropertyPanelMeta>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Running: Story = {};

export const Creating: Story = {
	args: {
		status: "New ECS service",
		context: "ap-southeast-2",
		tone: "neutral",
		showMarker: false,
	},
};
