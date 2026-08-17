import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React from "react";
import { PropertyEmptyState } from "../../PropertyPrimitives";
import "../../property-panel.css";
import { fullscreenParameters, primitiveDecorator } from "../storyConfig";

const meta = {
	title: "Components/Empty State",
	component: PropertyEmptyState,
	tags: ["!autodocs"],
	args: {
		title: "Nothing to configure",
		description: "This node is generated from the environment.",
	},
	parameters: fullscreenParameters,
	decorators: [primitiveDecorator],
	render: (args) => (
		<div
			className="mp-primitive-demo mp-compact-panel"
			data-surface="light"
			data-theme="light"
		>
			<PropertyEmptyState {...args} />
		</div>
	),
} satisfies Meta<typeof PropertyEmptyState>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
