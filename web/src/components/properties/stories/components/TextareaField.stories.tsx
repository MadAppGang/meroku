import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React, { useId, useState } from "react";
import { fn } from "storybook/test";
import { PropertyTextareaField } from "../../PropertyPrimitives";
import { fullscreenParameters, primitiveDecorator } from "../storyConfig";

function TextareaFieldExample() {
	const id = useId();
	const [value, setValue] = useState("Compact multi-line content.");
	return (
		<div
			className="mp-primitive-demo mp-compact-panel"
			data-surface="light"
			data-theme="light"
		>
			<PropertyTextareaField
				id={id}
				label="Notes"
				value={value}
				onChange={setValue}
			/>
		</div>
	);
}

const meta = {
	title: "Components/Textarea Field",
	component: PropertyTextareaField,
	tags: ["!autodocs"],
	args: {
		id: "textarea-field",
		label: "Notes",
		value: "Compact multi-line content.",
		onChange: fn(),
	},
	parameters: fullscreenParameters,
	decorators: [primitiveDecorator],
} satisfies Meta<typeof PropertyTextareaField>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = { render: () => <TextareaFieldExample /> };
