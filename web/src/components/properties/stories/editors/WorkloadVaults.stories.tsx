import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React from "react";
import type { YamlInfrastructureConfig } from "../../../../types/yamlConfig";
import { WorkloadVaults } from "../../../WorkloadVaults";
import {
	baseConfig,
	ConfigHarness,
	configWith,
	editorDecorator,
	editorParameters,
	storyNode,
} from "./editorStoryKit";

const meta = {
	title: "Editors/Workload Vaults",
	tags: ["!autodocs"],
	parameters: editorParameters,
	decorators: [editorDecorator],
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

const ordersNode = storyNode("service-orders", "service", "orders");

function Editor({ initial }: { initial: YamlInfrastructureConfig }) {
	return (
		<ConfigHarness initial={initial}>
			{(config, onChange) => (
				<WorkloadVaults
					config={config}
					node={ordersNode}
					onConfigChange={onChange}
				/>
			)}
		</ConfigHarness>
	);
}

function ordersWithVaults(
	vaults: NonNullable<YamlInfrastructureConfig["vaults"]>,
	connections: {
		name: string;
		access?: "read" | "read_write";
		prefix?: string;
	}[],
): YamlInfrastructureConfig {
	return configWith({
		vaults,
		services: (baseConfig.services || []).map((s) =>
			s.name === "orders" ? { ...s, vaults: connections } : s,
		),
	});
}

export const NoVaultsDeclared: Story = {
	name: "No vaults declared",
	render: () => <Editor initial={baseConfig} />,
};

export const Connected: Story = {
	render: () => (
		<Editor
			initial={ordersWithVaults(
				[{ name: "stripe" }, { name: "shared-db" }],
				[
					{ name: "stripe" },
					{ name: "shared-db", access: "read_write", prefix: "DB_" },
				],
			)}
		/>
	),
};

export const UndeclaredVault: Story = {
	name: "Connection to a missing vault",
	render: () => (
		<Editor
			initial={ordersWithVaults([{ name: "stripe" }], [{ name: "billing" }])}
		/>
	),
};
