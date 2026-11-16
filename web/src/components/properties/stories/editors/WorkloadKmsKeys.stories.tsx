import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React from "react";
import { expect, screen, userEvent, within } from "storybook/test";
import type {
	KmsKeyConnection,
	KmsKeyDefinition,
	YamlInfrastructureConfig,
} from "../../../../types/yamlConfig";
import { WorkloadKmsKeys } from "../../../WorkloadKmsKeys";
import {
	baseConfig,
	ConfigHarness,
	configWith,
	editorDecorator,
	editorParameters,
	storyNode,
} from "./editorStoryKit";

const meta = {
	title: "Editors/Workload KMS Keys",
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
				<WorkloadKmsKeys
					config={config}
					node={ordersNode}
					onConfigChange={onChange}
				/>
			)}
		</ConfigHarness>
	);
}

function ordersWithKeys(
	keys: KmsKeyDefinition[],
	connections: KmsKeyConnection[],
): YamlInfrastructureConfig {
	return configWith({
		kms_keys: keys,
		services: (baseConfig.services || []).map((s) =>
			s.name === "orders" ? { ...s, kms_keys: connections } : s,
		),
	});
}

export const NoKeysDeclared: Story = {
	name: "No keys declared",
	render: () => <Editor initial={baseConfig} />,
	play: async ({ canvasElement }) => {
		await expect(
			within(canvasElement).getByText(/no KMS keys to use/),
		).toBeVisible();
	},
};

export const NotConnected: Story = {
	name: "Keys declared, none connected",
	render: () => <Editor initial={ordersWithKeys([{ name: "auth" }], [])} />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: "Connect a key" }),
		);
		await expect(await canvas.findByText("KMS_AUTH_KEY_ARN")).toBeVisible();
		await expect(canvas.getByText("KMS_AUTH_PUBLIC_KEY")).toBeVisible();
		await expect(
			canvas.getByRole("combobox", { name: "Access to key auth" }),
		).toHaveTextContent("Verify");
	},
};

export const Sign: Story = {
	name: "Connected to sign",
	render: () => (
		<Editor
			initial={ordersWithKeys(
				[{ name: "auth" }],
				[{ name: "auth", access: "sign" }],
			)}
		/>
	),
	play: async ({ canvasElement }) => {
		await expect(
			within(canvasElement).getByText(/can sign with this key/),
		).toBeVisible();
	},
};

export const Verify: Story = {
	name: "Connected to verify",
	render: () => (
		<Editor
			initial={ordersWithKeys(
				[{ name: "auth" }],
				[{ name: "auth", access: "verify" }],
			)}
		/>
	),
	play: async ({ canvasElement }) => {
		await expect(
			within(canvasElement).getByText(/It cannot sign/),
		).toBeVisible();
	},
};

export const Both: Story = {
	name: "Connected to sign and verify",
	render: () => (
		<Editor
			initial={ordersWithKeys(
				[{ name: "auth" }, { name: "billing_v2" }],
				[
					{ name: "auth", access: "both" },
					{ name: "billing_v2", access: "verify" },
				],
			)}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("KMS_BILLING_V2_KEY_ARN")).toBeVisible();
		await expect(
			canvas.queryByRole("button", { name: "Connect a key" }),
		).toBeNull();
	},
};

export const ChangeAccess: Story = {
	name: "Change access",
	render: () => (
		<Editor
			initial={ordersWithKeys(
				[{ name: "auth" }],
				[{ name: "auth", access: "verify" }],
			)}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("combobox", { name: "Access to key auth" }),
		);
		await userEvent.click(
			await screen.findByRole("option", { name: "Sign and verify" }),
		);
		await expect(
			await canvas.findByText(/can sign, check signatures/),
		).toBeVisible();
	},
};

/** An HMAC key gives only its ARN, and access reads as MAC wording. */
export const HmacKey: Story = {
	name: "Connected to an HMAC key",
	render: () => (
		<Editor
			initial={ordersWithKeys(
				[{ name: "requests", key_spec: "HMAC_256" }],
				[{ name: "requests", access: "sign" }],
			)}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("KMS_REQUESTS_KEY_ARN")).toBeVisible();
		await expect(canvas.queryByText("KMS_REQUESTS_PUBLIC_KEY")).toBeNull();
		await expect(
			canvas.getByRole("combobox", { name: "Access to key requests" }),
		).toHaveTextContent("Create MAC");
		await expect(canvas.getByText(/create a MAC for a request/)).toBeVisible();
		await userEvent.click(
			canvas.getByRole("combobox", { name: "Access to key requests" }),
		);
		await userEvent.click(
			await screen.findByRole("option", { name: "Create and check MAC" }),
		);
		await expect(
			await canvas.findByText(/create and check MACs/),
		).toBeVisible();
	},
};

/** A signing key and an HMAC key on one workload. */
export const MixedKeys: Story = {
	name: "Connected to a signing and an HMAC key",
	render: () => (
		<Editor
			initial={ordersWithKeys(
				[{ name: "auth" }, { name: "requests", key_spec: "HMAC_384" }],
				[
					{ name: "auth", access: "verify" },
					{ name: "requests", access: "both" },
				],
			)}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const auth = within(canvas.getByTestId("kms-key-connection-auth"));
		const requests = within(canvas.getByTestId("kms-key-connection-requests"));
		await expect(auth.getByText("KMS_AUTH_PUBLIC_KEY")).toBeVisible();
		await expect(requests.getByText("KMS_REQUESTS_KEY_ARN")).toBeVisible();
		await expect(requests.queryByText("KMS_REQUESTS_PUBLIC_KEY")).toBeNull();
		await expect(
			requests.getByRole("combobox", { name: "Access to key requests" }),
		).toHaveTextContent("Create and check MAC");
	},
};

/** A workload variable named like an HMAC key's would-be public key is free. */
export const HmacKeyPublicKeyNameFree: Story = {
	name: "HMAC key leaves the PUBLIC_KEY name free",
	render: () => (
		<Editor
			initial={configWith({
				kms_keys: [{ name: "requests", key_spec: "HMAC_256" }],
				services: (baseConfig.services || []).map((s) =>
					s.name === "orders"
						? {
								...s,
								env_vars: { KMS_REQUESTS_PUBLIC_KEY: "mine" },
								kms_keys: [{ name: "requests", access: "verify" }],
							}
						: s,
				),
			})}
		/>
	),
	play: async ({ canvasElement }) => {
		await expect(within(canvasElement).queryByText(/is reserved/)).toBeNull();
	},
};

/** Retired keys are never offered for a new connection. */
export const OnlyRetiredKeys: Story = {
	name: "Only retired keys",
	render: () => (
		<Editor initial={ordersWithKeys([{ name: "old", retired: true }], [])} />
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText(/no KMS keys to use/)).toBeVisible();
		await expect(
			canvas.queryByRole("button", { name: "Connect a key" }),
		).toBeNull();
	},
};

export const RetiredKeyConnection: Story = {
	name: "Connection to a retired key",
	render: () => (
		<Editor
			initial={ordersWithKeys(
				[{ name: "old", retired: true }],
				[{ name: "old", access: "sign" }],
			)}
		/>
	),
	play: async ({ canvasElement }) => {
		await expect(
			within(canvasElement).getByText(/is retired; generate refuses/),
		).toBeVisible();
	},
};

export const UndeclaredKey: Story = {
	name: "Connection to a missing key",
	render: () => (
		<Editor
			initial={ordersWithKeys(
				[{ name: "auth" }],
				[{ name: "billing", access: "verify" }],
			)}
		/>
	),
	play: async ({ canvasElement }) => {
		await expect(
			within(canvasElement).getByText(/is not in this environment's KMS keys/),
		).toBeVisible();
	},
};

/** The workload's own variable already holds a name the key connection sets. */
export const ReservedEnvCollision: Story = {
	name: "Workload variable named like the key's",
	render: () => (
		<Editor
			initial={configWith({
				kms_keys: [{ name: "auth" }],
				services: (baseConfig.services || []).map((s) =>
					s.name === "orders"
						? {
								...s,
								env_vars: { KMS_AUTH_KEY_ARN: "fake-arn-do-not-use" },
								kms_keys: [{ name: "auth", access: "sign" }],
							}
						: s,
				),
			})}
		/>
	),
	play: async ({ canvasElement }) => {
		const card = within(
			within(canvasElement).getByTestId("kms-key-connection-auth"),
		);
		await expect(
			card.getByText(/KMS_AUTH_KEY_ARN is reserved: meroku sets it/),
		).toBeVisible();
		await expect(
			card.queryByText(/KMS_AUTH_PUBLIC_KEY is reserved/),
		).toBeNull();
	},
};

export const MissingAccess: Story = {
	name: "Connection without access",
	render: () => (
		<Editor
			initial={ordersWithKeys(
				[{ name: "auth" }],
				[{ name: "auth" } as KmsKeyConnection],
			)}
		/>
	),
	play: async ({ canvasElement }) => {
		await expect(
			within(canvasElement).getByText(
				"Choose what this workload may do with the key.",
			),
		).toBeVisible();
	},
};
