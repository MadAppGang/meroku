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

/** Saved keys: their type is shown read-only, with the workloads using them. */
export const WithKeys: Story = {
	name: "With keys",
	render: () => <Editor initial={withKeys} />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const auth = within(canvas.getByTestId("kms-key-auth"));
		await expect(auth.getByText("backend · Sign")).toBeVisible();
		await expect(auth.getByText("orders · Verify")).toBeVisible();
		await expect(auth.getByText(/AWS cannot change the type/)).toBeVisible();
		await expect(
			auth.queryByRole("combobox", { name: "Key type of auth" }),
		).toBeNull();
		await expect(
			within(canvas.getByTestId("kms-key-legacy")).getByText(
				"No workload uses this key yet.",
			),
		).toBeVisible();
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
		await expect(card.getByText("ECC P-256 (ES256)")).toBeVisible();
		await userEvent.click(
			card.getByRole("combobox", { name: "Key type of key1" }),
		);
		await userEvent.click(
			await screen.findByRole("option", {
				name: "RSA 2048 (RS256) — RS256 compatibility",
			}),
		);
		await expect(await card.findByText("RSA 2048 (RS256)")).toBeVisible();
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
		await expect(card.queryByText("backend · Sign")).toBeNull();
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
