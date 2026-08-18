import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React, { useId, useState } from "react";
import { fn } from "storybook/test";
import { PropertySelectField } from "../../PropertyPrimitives";
import { fullscreenParameters, primitiveDecorator } from "../storyConfig";

const options = [
	{ value: "256", label: "256 (0.25 vCPU)" },
	{ value: "512", label: "512 (0.5 vCPU)" },
	{ value: "1024", label: "1024 (1 vCPU)" },
];

function SelectFieldExample() {
	const id = useId();
	const [value, setValue] = useState("512");
	return (
		<div
			className="mp-primitive-demo mp-primitive-demo--field mp-compact-panel"
			data-surface="light"
			data-theme="light"
		>
			<PropertySelectField
				id={id}
				label="CPU"
				value={value}
				options={options}
				theme="light"
				labelPlacement="inline"
				onChange={setValue}
			/>
		</div>
	);
}

const meta = {
	title: "Components/Select Field",
	component: PropertySelectField,
	tags: ["!autodocs"],
	args: {
		id: "select-field",
		label: "CPU",
		value: "512",
		options,
		theme: "light",
		onChange: fn(),
	},
	parameters: fullscreenParameters,
	decorators: [primitiveDecorator],
} satisfies Meta<typeof PropertySelectField>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = { render: () => <SelectFieldExample /> };
