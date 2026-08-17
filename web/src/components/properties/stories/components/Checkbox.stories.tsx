import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React, { useState } from "react";
import { expect, fn, userEvent, within } from "storybook/test";
import { PropertyCheckboxField } from "../../PropertyPrimitives";
import "../../property-panel.css";
import { fullscreenParameters, primitiveDecorator } from "../storyConfig";

function CheckboxExample({
	initialChecked,
	disabled = false,
}: {
	initialChecked: boolean;
	disabled?: boolean;
}) {
	const [checked, setChecked] = useState(initialChecked);

	return (
		<div
			className="mp-primitive-demo mp-primitive-demo--field mp-compact-panel"
			data-surface="light"
			data-theme="light"
		>
			<PropertyCheckboxField
				item={{
					id: `storybook-checkbox-${initialChecked ? "checked" : "unchecked"}`,
					label: "X-Ray",
					checked,
					disabled,
					onCheckedChange: setChecked,
				}}
			/>
		</div>
	);
}

const meta = {
	title: "Components/Checkbox",
	component: PropertyCheckboxField,
	tags: ["!autodocs"],
	args: {
		item: {
			id: "xray",
			label: "X-Ray",
			checked: true,
			onCheckedChange: fn(),
		},
	},
	parameters: fullscreenParameters,
	decorators: [primitiveDecorator],
} satisfies Meta<typeof PropertyCheckboxField>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
	name: "Checked",
	render: () => <CheckboxExample initialChecked />,
	play: async ({ canvasElement }) => {
		const checkbox = within(canvasElement).getByRole("checkbox", {
			name: "X-Ray",
		});
		await expect(checkbox).toBeChecked();
		await userEvent.click(checkbox);
		await expect(checkbox).not.toBeChecked();
		await userEvent.click(checkbox);
		await expect(checkbox).toBeChecked();
	},
};

export const Unchecked: Story = {
	render: () => <CheckboxExample initialChecked={false} />,
};

export const Disabled: Story = {
	render: () => <CheckboxExample initialChecked={false} disabled />,
};
