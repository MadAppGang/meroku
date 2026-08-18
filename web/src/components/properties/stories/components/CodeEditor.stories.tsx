import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React, { useId, useState } from "react";
import { expect, waitFor, within } from "storybook/test";
import { PropertyCodeEditorField } from "../../PropertyPrimitives";
import { fullscreenParameters, primitiveDecorator } from "../storyConfig";

const terraformExample = `resource "aws_cloudwatch_log_group" "custom" {
  name              = "/custom/circl"
  retention_in_days = 14
}`;

function CodeEditorExample() {
	const id = useId();
	const [value, setValue] = useState(terraformExample);

	return (
		<div
			className="mp-primitive-demo mp-primitive-demo--editor mp-compact-panel"
			data-surface="light"
			data-theme="light"
		>
			<PropertyCodeEditorField
				id={id}
				label="HCL"
				value={value}
				theme="light"
				rows={10}
				onChange={setValue}
			/>
		</div>
	);
}

const meta = {
	title: "Components/Code Editor",
	component: PropertyCodeEditorField,
	tags: ["!autodocs"],
	parameters: fullscreenParameters,
	decorators: [primitiveDecorator],
} satisfies Meta<typeof PropertyCodeEditorField>;

export default meta;
type Story = StoryObj<typeof meta>;

export const HCL: Story = {
	render: () => <CodeEditorExample />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			await canvas.findByRole(
				"textbox",
				{ name: "HCL code editor" },
				{ timeout: 10_000 },
			),
		).toBeVisible();
		await waitFor(
			() => {
				expect(
					canvasElement.querySelectorAll(".line-numbers").length,
				).toBeGreaterThan(2);
			},
			{ timeout: 10_000 },
		);
		await expect(
			canvas.getByText("Ctrl/⌘ Space for suggestions"),
		).toBeVisible();
		const editor = canvasElement.querySelector<HTMLElement>(".mp-code-editor");
		const status = canvasElement.querySelector<HTMLElement>(
			".mp-code-editor__status",
		);
		await expect(editor).not.toBeNull();
		await expect(status).not.toBeNull();
		await expect(
			(status?.getBoundingClientRect().bottom ?? Number.POSITIVE_INFINITY) <=
				(editor?.getBoundingClientRect().bottom ?? 0) + 1,
		).toBe(true);
		await expect(editor?.scrollHeight ?? 1).toBeLessThanOrEqual(
			editor?.clientHeight ?? 0,
		);
	},
};
