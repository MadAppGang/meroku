import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React from "react";
import { expect, userEvent, within } from "storybook/test";
import type { YamlInfrastructureConfig } from "../../../../types/yamlConfig";
import { ValkeyNodeProperties } from "../../../ValkeyNodeProperties";
import {
	baseConfig,
	ConfigHarness,
	configWith,
	editorDecorator,
	editorParameters,
} from "./editorStoryKit";

const meta = {
	title: "Editors/Valkey",
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
				<ValkeyNodeProperties config={config} onConfigChange={onChange} />
			)}
		</ConfigHarness>
	);
}

const enabled = configWith({
	valkey: {
		enabled: true,
		engine_version: "8",
		max_storage_gb: 2,
		access: ["backend", "orders"],
	},
});

export const Off: Story = {
	render: () => <Editor initial={baseConfig} />,
};

export const On: Story = {
	render: () => <Editor initial={enabled} />,
};

export const UnknownAccess: Story = {
	name: "Access names a missing workload",
	render: () => (
		<Editor
			initial={configWith({
				valkey: { enabled: true, access: ["backend", "billing"] },
			})}
		/>
	),
};

export const InvalidStorage: Story = {
	name: "Invalid storage cap",
	render: () => <Editor initial={enabled} />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const input = canvas.getByLabelText("Storage cap (GB)");
		await userEvent.clear(input);
		await userEvent.type(input, "0");
		await expect(input).toHaveAttribute("aria-invalid", "true");
	},
};
