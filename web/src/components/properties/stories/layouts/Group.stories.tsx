import type { Meta, StoryObj } from "@storybook/react-vite";
import { Activity } from "lucide-react";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React from "react";
import { PropertyGroup, PropertyReadonlyRows } from "../../PropertyLayouts";
import "../../property-panel.css";
import { fullscreenParameters, primitiveDecorator } from "../storyConfig";

const meta = {
	title: "Layouts/Group",
	component: PropertyGroup,
	tags: ["!autodocs"],
	args: {
		title: "Runtime Status",
		description: "Current deployment details",
		icon: <Activity />,
	},
	parameters: fullscreenParameters,
	decorators: [primitiveDecorator],
	render: (args) => (
		<div
			className="mp-layout-demo mp-compact-panel"
			data-surface="light"
			data-theme="light"
		>
			<PropertyGroup {...args}>
				<PropertyReadonlyRows
					rows={[
						["Health", "Healthy · 2/2 tasks"],
						["Region", "ap-southeast-2"],
					]}
				/>
			</PropertyGroup>
		</div>
	),
} satisfies Meta<typeof PropertyGroup>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const ReadonlyRows: Story = {
	name: "Readonly rows",
};
