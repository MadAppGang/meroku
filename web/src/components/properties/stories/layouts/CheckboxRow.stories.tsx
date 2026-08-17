import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React, { useState } from "react";
import { PropertyCapabilities } from "../../PropertyLayouts";
import "../../property-panel.css";
import { fullscreenParameters, primitiveDecorator } from "../storyConfig";

function CheckboxRowExample() {
	const [xray, setXray] = useState(true);
	const [ssh, setSsh] = useState(false);

	return (
		<div
			className="mp-layout-demo mp-compact-panel"
			data-surface="light"
			data-theme="light"
		>
			<div className="mp-layout-demo__section">
				<PropertyCapabilities
					items={[
						{
							id: "storybook-xray",
							label: "X-Ray",
							checked: xray,
							onCheckedChange: setXray,
						},
						{
							id: "storybook-ssh",
							label: "SSH",
							checked: ssh,
							onCheckedChange: setSsh,
						},
					]}
				/>
			</div>
		</div>
	);
}

const meta = {
	title: "Layouts/Checkbox Row",
	component: PropertyCapabilities,
	tags: ["!autodocs"],
	args: { items: [] },
	parameters: fullscreenParameters,
	decorators: [primitiveDecorator],
} satisfies Meta<typeof PropertyCapabilities>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = { render: () => <CheckboxRowExample /> };
