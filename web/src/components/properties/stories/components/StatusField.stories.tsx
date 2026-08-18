import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React from "react";
import { PropertyStatusField } from "../../PropertyPrimitives";
import { fullscreenParameters, primitiveDecorator } from "../storyConfig";

const meta = {
	title: "Components/Status Field",
	component: PropertyStatusField,
	tags: ["!autodocs"],
	args: {
		label: "Runtime state",
		value: "Healthy",
		tone: "success",
	},
	parameters: fullscreenParameters,
	decorators: [primitiveDecorator],
	render: (args) => (
		<div
			className="mp-primitive-demo mp-primitive-demo--field mp-compact-panel"
			data-surface="light"
			data-theme="light"
		>
			<PropertyStatusField {...args} />
		</div>
	),
} satisfies Meta<typeof PropertyStatusField>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
