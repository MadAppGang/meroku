import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React from "react";
import { PropertyReadonlyField } from "../../PropertyPrimitives";
import "../../property-panel.css";
import { fullscreenParameters, primitiveDecorator } from "../storyConfig";

const meta = {
	title: "Components/Readonly Field",
	component: PropertyReadonlyField,
	tags: ["!autodocs"],
	args: {
		label: "Repository",
		value: "123456789.dkr.ecr.ap-southeast-2.amazonaws.com/circl",
		mono: true,
	},
	parameters: fullscreenParameters,
	decorators: [primitiveDecorator],
	render: (args) => (
		<div
			className="mp-primitive-demo mp-compact-panel"
			data-surface="light"
			data-theme="light"
		>
			<PropertyReadonlyField {...args} />
		</div>
	),
} satisfies Meta<typeof PropertyReadonlyField>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
