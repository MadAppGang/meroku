import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React from "react";
import { expect, within } from "storybook/test";
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
				[
					{ name: "stripe", secrets: ["api_key", "webhook_secret"] },
					{ name: "shared-db", secrets: ["database_url"] },
				],
				[
					{ name: "stripe" },
					{ name: "shared-db", access: "read_write", prefix: "DB" },
				],
			)}
		/>
	),
};

export const UndeclaredVault: Story = {
	name: "Connection to a missing vault",
	render: () => (
		<Editor
			initial={ordersWithVaults(
				[{ name: "stripe", secrets: ["api_key"] }],
				[{ name: "billing" }],
			)}
		/>
	),
};

export const VaultWithoutSecretsList: Story = {
	name: "Vault without a secrets list",
	render: () => (
		<Editor
			initial={ordersWithVaults([{ name: "stripe" }], [{ name: "stripe" }])}
		/>
	),
	play: async ({ canvasElement }) => {
		await expect(
			within(canvasElement).getByText(/does not list its secret names yet/),
		).toBeVisible();
	},
};

export const VaultWithNoSecrets: Story = {
	name: "Vault with no secrets yet",
	render: () => (
		<Editor
			initial={ordersWithVaults(
				[{ name: "stripe", secrets: [] }],
				[{ name: "stripe" }],
			)}
		/>
	),
};

export const SecretsWithPrefix: Story = {
	name: "Secrets the workload gets, with a prefix",
	render: () => (
		<Editor
			initial={ordersWithVaults(
				[{ name: "stripe", secrets: ["api_key", "webhook_secret"] }],
				[{ name: "stripe", prefix: "PAY" }],
			)}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("PAY_API_KEY")).toBeVisible();
		await expect(canvas.getByText("PAY_WEBHOOK_SECRET")).toBeVisible();
	},
};

export const ReservedEnvName: Story = {
	name: "A secret named env without a prefix",
	render: () => (
		<Editor
			initial={ordersWithVaults(
				[{ name: "stripe", secrets: ["env", "api_key"] }],
				[{ name: "stripe" }],
			)}
		/>
	),
	play: async ({ canvasElement }) => {
		await expect(
			within(canvasElement).getByText(/which meroku reserves/),
		).toBeVisible();
	},
};
