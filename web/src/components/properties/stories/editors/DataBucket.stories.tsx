import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React from "react";
import { expect, userEvent, within } from "storybook/test";
import type { YamlInfrastructureConfig } from "../../../../types/yamlConfig";
import { DataBucketProperties } from "../../../DataBucketProperties";
import {
	baseConfig,
	ConfigHarness,
	configWith,
	editorDecorator,
	editorParameters,
} from "./editorStoryKit";

const meta = {
	title: "Editors/Data Bucket",
	tags: ["!autodocs"],
	parameters: editorParameters,
	decorators: [editorDecorator],
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

function Editor({ initial }: { initial: YamlInfrastructureConfig }) {
	return (
		<ConfigHarness initial={initial}>
			{(config, onChange) => (
				<DataBucketProperties config={config} onConfigChange={onChange} />
			)}
		</ConfigHarness>
	);
}

export const NoSharedFolders: Story = {
	name: "No shared folders",
	render: () => <Editor initial={baseConfig} />,
};

export const SharedFolders: Story = {
	name: "Shared folders",
	render: () => (
		<Editor
			initial={configWith({
				data_bucket: {
					shared: [
						{ name: "uploads", read_write: ["backend"], read: ["orders"] },
						{ name: "reports", read_write: ["nightly"], read: [] },
					],
				},
			})}
		/>
	),
};

export const UnknownWorkload: Story = {
	name: "Folder names a missing workload",
	render: () => (
		<Editor
			initial={configWith({
				data_bucket: {
					shared: [{ name: "uploads", read_write: ["billing"], read: [] }],
				},
			})}
		/>
	),
};

export const InvalidFolderName: Story = {
	name: "Invalid new folder name",
	render: () => <Editor initial={baseConfig} />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.type(
			canvas.getByLabelText("New shared folder"),
			"Bad_Name",
		);
		await expect(
			canvas.getByText(/lower-case letters, digits and hyphens/),
		).toBeInTheDocument();
		await expect(canvas.getByRole("button", { name: /Add/ })).toBeDisabled();
	},
};
