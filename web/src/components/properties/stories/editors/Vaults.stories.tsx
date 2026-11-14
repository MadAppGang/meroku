import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React from "react";
import { expect, userEvent, within } from "storybook/test";
import type { YamlInfrastructureConfig } from "../../../../types/yamlConfig";
import { VaultsProperties } from "../../../VaultsProperties";
import {
	baseConfig,
	ConfigHarness,
	configWith,
	editorDecorator,
	editorParameters,
} from "./editorStoryKit";

const meta = {
	title: "Editors/Vaults",
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
				<VaultsProperties config={config} onConfigChange={onChange} />
			)}
		</ConfigHarness>
	);
}

export const NoVaults: Story = {
	name: "No vaults",
	render: () => <Editor initial={baseConfig} />,
};

export const WithVaults: Story = {
	name: "With vaults",
	render: () => (
		<Editor
			initial={configWith({
				vaults: [{ name: "stripe" }, { name: "shared-db" }],
				workload: {
					vaults: [{ name: "stripe" }],
				} as YamlInfrastructureConfig["workload"],
			})}
		/>
	),
};

export const InvalidName: Story = {
	name: "Invalid new vault name",
	render: () => (
		<Editor initial={configWith({ vaults: [{ name: "stripe" }] })} />
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const input = canvas.getByLabelText("New vault");
		await userEvent.type(input, "stripe");
		await expect(input).toHaveAttribute("aria-invalid", "true");
	},
};
