import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React, { useState } from "react";
import { expect, userEvent, within } from "storybook/test";
import type { IAMStatement } from "../../../../types/yamlConfig";
import { WorkloadPolicyEditor } from "../../../WorkloadPolicyEditor";
import { editorDecorator, editorParameters } from "./editorStoryKit";

const meta = {
	title: "Editors/Workload Policy",
	tags: ["!autodocs"],
	parameters: editorParameters,
	decorators: [editorDecorator],
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

function Editor({ initial }: { initial: IAMStatement[] }) {
	const [statements, setStatements] = useState(initial);
	return (
		<WorkloadPolicyEditor
			statements={statements}
			onChange={setStatements}
			roleName="demo_service_orders_task_dev"
		/>
	);
}

export const NoStatements: Story = {
	name: "No statements",
	render: () => <Editor initial={[]} />,
};

export const Statements: Story = {
	render: () => (
		<Editor
			initial={[
				{
					actions: ["dynamodb:GetItem", "dynamodb:PutItem"],
					resources: ["arn:aws:dynamodb:us-east-1:000000000000:table/orders"],
				},
				{
					effect: "Deny",
					actions: ["s3:DeleteObject"],
					resources: ["*"],
					conditions: [
						{
							test: "StringNotEquals",
							variable: "aws:PrincipalTag/team",
							values: ["ops"],
						},
					],
				},
			]}
		/>
	),
};

export const AddingStatement: Story = {
	name: "Adding a statement",
	render: () => <Editor initial={[]} />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button", { name: /Add/ }));
		await expect(
			await canvas.findByTestId("policy-statement-form"),
		).toBeInTheDocument();
	},
};
