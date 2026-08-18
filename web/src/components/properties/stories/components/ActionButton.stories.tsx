import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React from "react";
import { fn } from "storybook/test";
import { PropertyGroup, PropertyReadonlyRows } from "../../PropertyLayouts";
import { PropertyActionButton } from "../../PropertyPrimitives";
import { fullscreenParameters, primitiveDecorator } from "../storyConfig";

const meta = {
	title: "Components/Action Button",
	component: PropertyActionButton,
	tags: ["!autodocs"],
	args: {
		children: "+ Add variable",
		onClick: fn(),
	},
	parameters: fullscreenParameters,
	decorators: [primitiveDecorator],
	render: (args) => (
		<PropertyGroup
			title="Environment"
			action={<PropertyActionButton {...args} />}
		>
			<PropertyReadonlyRows rows={[["Variables", "5 configured"]]} />
		</PropertyGroup>
	),
} satisfies Meta<typeof PropertyActionButton>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
