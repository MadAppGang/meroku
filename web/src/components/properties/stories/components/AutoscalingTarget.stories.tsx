import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React, { useState } from "react";
import { fn, userEvent, within } from "storybook/test";
import { PropertyAutoscalingTarget } from "../../PropertyPrimitives";
import { fullscreenParameters, primitiveDecorator } from "../storyConfig";

function AutoscalingTargetExample() {
	const [value, setValue] = useState(70);
	return (
		<div
			className="mp-primitive-demo mp-primitive-demo--field mp-compact-panel"
			data-surface="light"
			data-theme="light"
		>
			<PropertyAutoscalingTarget
				value={value}
				theme="light"
				onChange={setValue}
			/>
		</div>
	);
}

const meta = {
	title: "Components/Autoscaling Target",
	component: PropertyAutoscalingTarget,
	tags: ["!autodocs"],
	args: { value: 70, theme: "light", onChange: fn() },
	parameters: fullscreenParameters,
	decorators: [primitiveDecorator],
} satisfies Meta<typeof PropertyAutoscalingTarget>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
	name: "Closed",
	render: () => <AutoscalingTargetExample />,
};

export const Open: Story = {
	render: () => <AutoscalingTargetExample />,
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", {
				name: "Edit autoscaling target",
			}),
		);
	},
};
