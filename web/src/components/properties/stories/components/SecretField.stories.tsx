import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React, { useId, useState } from "react";
import { fn } from "storybook/test";
import { PropertySecretField } from "../../PropertyPrimitives";
import { fullscreenParameters, primitiveDecorator } from "../storyConfig";

function SecretFieldExample() {
	const id = useId();
	const [value, setValue] = useState("secret-value");
	return (
		<div
			className="mp-primitive-demo mp-primitive-demo--field mp-compact-panel"
			data-surface="light"
			data-theme="light"
		>
			<PropertySecretField
				id={id}
				label="API key"
				value={value}
				onChange={setValue}
			/>
		</div>
	);
}

const meta = {
	title: "Components/Secret Field",
	component: PropertySecretField,
	tags: ["!autodocs"],
	args: {
		id: "secret-field",
		label: "API key",
		value: "secret-value",
		onChange: fn(),
	},
	parameters: fullscreenParameters,
	decorators: [primitiveDecorator],
} satisfies Meta<typeof PropertySecretField>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = { render: () => <SecretFieldExample /> };
