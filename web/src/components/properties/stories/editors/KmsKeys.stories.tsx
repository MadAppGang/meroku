import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React from "react";
import { expect, screen, userEvent, waitFor, within } from "storybook/test";
import type { YamlInfrastructureConfig } from "../../../../types/yamlConfig";
import { KmsKeysProperties } from "../../../KmsKeysProperties";
import {
	baseConfig,
	ConfigHarness,
	configWith,
	editorDecorator,
	editorParameters,
} from "./editorStoryKit";

const meta = {
	title: "Editors/KMS Keys",
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
				<KmsKeysProperties config={config} onConfigChange={onChange} />
			)}
		</ConfigHarness>
	);
}

/** The backend signs with `auth`; `orders` checks its signatures. */
const withKeys = configWith({
	kms_keys: [
		{ name: "auth", key_spec: "ECC_NIST_P256" },
		{ name: "legacy", key_spec: "RSA_2048" },
	],
	workload: {
		kms_keys: [{ name: "auth", access: "sign" }],
	} as YamlInfrastructureConfig["workload"],
	services: (baseConfig.services || []).map((s) =>
		s.name === "orders"
			? { ...s, kms_keys: [{ name: "auth", access: "verify" as const }] }
			: s,
	),
});

export const NoKeys: Story = {
	name: "No keys",
	render: () => <Editor initial={baseConfig} />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("No keys yet.")).toBeVisible();
		await expect(canvas.getByLabelText("New key name")).toHaveValue("key1");
		await expect(
			canvas.getByRole("combobox", { name: "Type of the new key" }),
		).toHaveTextContent("ECC P-256 (ES256) — recommended for JWTs");
	},
};

/** Saved keys: a fixed type badge and coloured counts of the workloads using them. */
export const WithKeys: Story = {
	name: "With keys",
	render: () => <Editor initial={withKeys} />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const auth = within(canvas.getByTestId("kms-key-auth"));
		await expect(auth.getByText("1 sign")).toHaveAttribute("title", "backend");
		await expect(auth.getByText("1 verify")).toHaveAttribute("title", "orders");
		await expect(auth.getByText("ES256", { exact: true })).toBeVisible();
		await expect(
			auth.queryByRole("combobox", { name: "Key type of auth" }),
		).toBeNull();
		await expect(
			within(canvas.getByTestId("kms-key-legacy")).getByText("Not used"),
		).toBeVisible();
	},
};

/** An HMAC key used by two services: one creates MACs, the other checks them. */
const withHmacKey = configWith({
	kms_keys: [{ name: "requests", key_spec: "HMAC_256" }],
	workload: {
		kms_keys: [{ name: "requests", access: "sign" }],
	} as YamlInfrastructureConfig["workload"],
	services: (baseConfig.services || []).map((s) =>
		s.name === "orders"
			? { ...s, kms_keys: [{ name: "requests", access: "verify" as const }] }
			: s,
	),
});

export const HmacKey: Story = {
	name: "HMAC key",
	render: () => <Editor initial={withHmacKey} />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const requests = within(canvas.getByTestId("kms-key-requests"));
		await expect(requests.getByText("HS256", { exact: true })).toHaveAttribute(
			"title",
			"HMAC-256 (HS256)",
		);
		await expect(requests.getByText("1 sign")).toHaveAttribute(
			"title",
			"backend",
		);
		await expect(
			canvas.getByText(/An HMAC key has no public key/),
		).toBeVisible();
	},
};

/** Signing and HMAC keys side by side. */
export const MixedKeys: Story = {
	name: "Signing and HMAC keys",
	render: () => (
		<Editor
			initial={configWith({
				...withKeys,
				kms_keys: [
					...(withKeys.kms_keys || []),
					{ name: "requests", key_spec: "HMAC_512" },
				],
			})}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			within(canvas.getByTestId("kms-key-auth")).getByText("ES256", {
				exact: true,
			}),
		).toBeVisible();
		await expect(
			within(canvas.getByTestId("kms-key-requests")).getByText("HS512", {
				exact: true,
			}),
		).toBeVisible();
	},
};

/** A new key can be an HMAC key; the HMAC types follow the signing ones. */
export const AddHmacKey: Story = {
	name: "Add an HMAC key",
	render: () => <Editor initial={baseConfig} />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("combobox", { name: "Type of the new key" }),
		);
		await userEvent.click(
			await screen.findByRole("option", {
				name: "HMAC-256 (HS256) — shared secret for service-to-service requests",
			}),
		);
		await userEvent.click(canvas.getByRole("button", { name: "Add key" }));
		const card = within(await canvas.findByTestId("kms-key-key1"));
		await expect(
			card.getByRole("combobox", { name: "Key type of key1" }),
		).toHaveTextContent("HMAC-256 (HS256)");
	},
};

/** The default name follows the highest key<n>, not the number of keys. */
export const NextDefaultName: Story = {
	name: "Default name after key7",
	render: () => (
		<Editor
			initial={configWith({
				kms_keys: [{ name: "key7" }, { name: "auth" }],
			})}
		/>
	),
	play: async ({ canvasElement }) => {
		await expect(
			within(canvasElement).getByLabelText("New key name"),
		).toHaveValue("key8");
	},
};

/** A new key keeps an editable type until the panel is opened again. */
export const AddKey: Story = {
	name: "Add a key and change its type",
	render: () => <Editor initial={baseConfig} />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button", { name: "Add key" }));
		const card = within(await canvas.findByTestId("kms-key-key1"));
		await expect(
			card.getByRole("combobox", { name: "Key type of key1" }),
		).toHaveTextContent("ECC P-256 (ES256)");
		await userEvent.click(
			card.getByRole("combobox", { name: "Key type of key1" }),
		);
		await userEvent.click(
			await screen.findByRole("option", {
				name: "RSA 2048 (RS256) — RS256 compatibility",
			}),
		);
		await expect(
			card.getByRole("combobox", { name: "Key type of key1" }),
		).toHaveTextContent("RSA 2048 (RS256)");
		await expect(canvas.getByLabelText("New key name")).toHaveValue("key2");
	},
};

export const InvalidName: Story = {
	name: "Invalid new key name",
	render: () => <Editor initial={withKeys} />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const input = canvas.getByLabelText("New key name");
		await userEvent.clear(input);
		await userEvent.type(input, "Auth-Key");
		await expect(input).toHaveAttribute("aria-invalid", "true");
		await expect(
			canvas.getByText(/Use 1 to 31 lower-case letters/),
		).toBeVisible();
		await expect(
			canvas.getByRole("button", { name: "Add key" }),
		).toBeDisabled();
	},
};

export const DuplicateName: Story = {
	name: "Duplicate new key name",
	render: () => <Editor initial={withKeys} />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const input = canvas.getByLabelText("New key name");
		await userEvent.clear(input);
		await expect(canvas.getByText("Give the key a name.")).toBeVisible();
		await userEvent.type(input, "auth");
		await expect(
			canvas.getByText('A key named "auth" already exists.'),
		).toBeVisible();
	},
};

/** Retiring removes every connection and offers Restore and Delete. */
export const Retire: Story = {
	name: "Retire a key",
	render: () => <Editor initial={withKeys} />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: "Retire key auth" }),
		);
		const card = within(canvas.getByTestId("kms-key-auth"));
		await expect(await card.findByText("Retired")).toBeVisible();
		await expect(card.getByText("Apply, then delete the key.")).toBeVisible();
		await expect(card.queryByText("1 sign")).toBeNull();
		await expect(
			card.getByRole("button", { name: "Restore key auth" }),
		).toBeVisible();
		await expect(
			card.getByRole("button", { name: "Delete key auth" }),
		).toBeVisible();
	},
};

const retired = configWith({
	kms_keys: [{ name: "auth" }, { name: "old", retired: true }],
});

export const Retired: Story = {
	name: "Retired key",
	render: () => <Editor initial={retired} />,
};

export const Restore: Story = {
	name: "Restore a retired key",
	render: () => <Editor initial={retired} />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: "Restore key old" }),
		);
		const card = within(canvas.getByTestId("kms-key-old"));
		await expect(card.queryByText("Retired")).toBeNull();
		await expect(
			card.getByRole("button", { name: "Retire key old" }),
		).toBeVisible();
	},
};

/** Deleting asks first, with the 7-day note. */
export const Delete: Story = {
	name: "Delete a retired key",
	render: () => <Editor initial={retired} />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.queryByRole("button", { name: "Delete key auth" }),
		).toBeNull();
		await userEvent.click(
			canvas.getByRole("button", { name: "Delete key old" }),
		);
		const dialog = within(
			await screen.findByRole("alertdialog", { name: "Delete key old?" }),
		);
		await expect(dialog.getByText(/after 7 days/)).toBeInTheDocument();
		await expect(dialog.getByText(/stops working at once/)).toBeInTheDocument();
		await userEvent.click(dialog.getByRole("button", { name: "Delete key" }));
		await waitFor(() => expect(canvas.queryByTestId("kms-key-old")).toBeNull());
		await expect(canvas.getByTestId("kms-key-auth")).toBeVisible();
	},
};
