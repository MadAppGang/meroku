import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React from "react";
import { expect, screen, userEvent, waitFor, within } from "storybook/test";
import type { VaultSecretStatus } from "../../../../api/infrastructure";
import type { YamlInfrastructureConfig } from "../../../../types/yamlConfig";
import { VaultsProperties } from "../../../VaultsProperties";
import {
	baseConfig,
	ConfigHarness,
	configWith,
	editorDecorator,
	editorParameters,
} from "./editorStoryKit";

/**
 * The Vaults node. `window.fetch` is replaced for this file only and answers
 * /api/vaults/secrets from an in-memory AWS keyed by vault name. Values are
 * never stored or returned, as in the real API. All names are synthetic.
 */

type FakeParameter = Omit<VaultSecretStatus, "declared" | "set">;

/** What AWS holds per vault. `"denied"` answers 403, `"pending"` never answers. */
type FakeVault = FakeParameter[] | "denied" | "pending";

const AWS_FIXTURE: Record<string, FakeVault> = {
	stripe: [
		{
			name: "api_key",
			type: "SecureString",
			version: 3,
			lastModified: "2026-11-10T09:30:00Z",
		},
		{
			name: "old_token",
			type: "SecureString",
			version: 1,
			lastModified: "2026-03-02T14:05:00Z",
		},
	],
	legacy: [
		{ name: "database_url", type: "SecureString", version: 2 },
		{ name: "api_key", type: "SecureString", version: 1 },
	],
	empty: [],
	locked: "denied",
	slow: "pending",
};

let aws: Record<string, FakeVault> = {};
let realFetch: typeof window.fetch | null = null;

function json(body: unknown, status = 200): Response {
	return new Response(JSON.stringify(body), {
		status,
		headers: { "Content-Type": "application/json" },
	});
}

function installFakeAws() {
	aws = structuredClone(AWS_FIXTURE);
	if (!realFetch) realFetch = window.fetch.bind(window);
	const passthrough = realFetch;
	window.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
		const url = new URL(
			typeof input === "string" ? input : input.toString(),
			window.location.origin,
		);
		if (url.pathname !== "/api/vaults/secrets") return passthrough(input, init);
		const vault = url.searchParams.get("vault") ?? "";
		const params = aws[vault] ?? [];
		if (params === "pending") return new Promise<Response>(() => {});
		if (params === "denied") {
			return json(
				{ error: "AccessDeniedException: not allowed to read parameters" },
				403,
			);
		}
		const method = init?.method ?? "GET";
		if (method === "PUT") {
			const { name } = JSON.parse(String(init?.body)) as { name: string };
			const existing = params.find((p) => p.name === name);
			const version = (existing?.version ?? 0) + 1;
			if (existing) existing.version = version;
			else params.push({ name, type: "SecureString", version });
			return json({ name, version });
		}
		if (method === "DELETE") {
			const name = url.searchParams.get("name") ?? "";
			aws[vault] = params.filter((p) => p.name !== name);
			return json({ name, deleted: true });
		}
		const secrets = params
			.map((p) => ({
				...p,
				envName: p.name.toUpperCase(),
				declared: false,
				set: true,
			}))
			.sort((a, b) => a.name.localeCompare(b.name));
		return json({ vault, path: `/dev/demo/vault/${vault}/`, secrets });
	}) as typeof window.fetch;
	return () => {
		if (realFetch) window.fetch = realFetch;
	};
}

const meta = {
	title: "Editors/Vaults",
	tags: ["!autodocs"],
	parameters: editorParameters,
	decorators: [editorDecorator],
	beforeEach: installFakeAws,
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

/** Every password field on the page is empty: values are never shown. */
async function expectNoValueShown(canvasElement: HTMLElement) {
	for (const input of canvasElement.querySelectorAll<HTMLInputElement>(
		'input[type="password"]',
	)) {
		await expect(input).toHaveValue("");
	}
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
				vaults: [{ name: "empty", secrets: [] }],
				workload: {
					vaults: [{ name: "empty" }],
				} as YamlInfrastructureConfig["workload"],
			})}
		/>
	),
	play: async ({ canvasElement }) => {
		await expect(
			await within(canvasElement).findByText("This vault has no secrets yet."),
		).toBeVisible();
	},
};

export const InvalidName: Story = {
	name: "Invalid new vault name",
	render: () => (
		<Editor
			initial={configWith({ vaults: [{ name: "empty", secrets: [] }] })}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const input = canvas.getByLabelText("New vault");
		await userEvent.type(input, "empty");
		await expect(input).toHaveAttribute("aria-invalid", "true");
	},
};

/** The older shape: no `secrets` key. Import the names AWS has, or start empty. */
export const NoSecretsList: Story = {
	name: "No secrets list: import banner",
	render: () => (
		<Editor initial={configWith({ vaults: [{ name: "legacy" }] })} />
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByText("meroku needs this vault's secret names"),
		).toBeVisible();
		await expect(
			await canvas.findByText("Found in AWS: api_key, database_url."),
		).toBeVisible();
		await userEvent.click(
			canvas.getByRole("button", { name: "Import names from AWS" }),
		);
		await expect(await canvas.findByText("DATABASE_URL")).toBeVisible();
		await expect(
			canvas.queryByText("Not in the YAML", { exact: false }),
		).toBeNull();
	},
};

/** Declared with a value, declared with none, and one in AWS the YAML does not list. */
export const DeclaredAndUndeclared: Story = {
	name: "Declared set, declared unset, undeclared in AWS",
	render: () => (
		<Editor
			initial={configWith({
				vaults: [{ name: "stripe", secrets: ["api_key", "webhook_secret"] }],
			})}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(await canvas.findByText(/version 3/)).toBeVisible();
		await expect(canvas.getByText(/No value yet/)).toBeVisible();
		await expect(canvas.getByText(/Not in the YAML/)).toBeVisible();
		await expect(
			canvas.getByRole("button", { name: "Replace value of api_key" }),
		).toBeVisible();
		await expect(
			canvas.getByRole("button", { name: "Set value of webhook_secret" }),
		).toBeVisible();
		await expect(
			canvas.getByRole("button", { name: "Delete old_token from AWS" }),
		).toBeVisible();
		await expectNoValueShown(canvasElement);
	},
};

/** Adding a secret: the name joins the list, the value goes to AWS, the fields clear. */
export const AddSecret: Story = {
	name: "Add a secret",
	render: () => (
		<Editor
			initial={configWith({ vaults: [{ name: "empty", secrets: [] }] })}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await canvas.findByText("This vault has no secrets yet.");
		await userEvent.type(canvas.getByLabelText("New secret name"), "api_key");
		await userEvent.type(
			canvas.getByLabelText("Value"),
			"fake-value-do-not-use",
		);
		await userEvent.click(
			canvas.getByRole("button", { name: "Add secret to empty" }),
		);
		await expect(await canvas.findByText(/version 1/)).toBeVisible();
		await expect(
			canvas.getByText("Apply to give the connected workloads this change."),
		).toBeVisible();
		await expect(canvas.getByLabelText("New secret name")).toHaveValue("");
		await expectNoValueShown(canvasElement);
	},
};

/** Replacing a value: an empty password field, then the redeploy hint. */
export const ReplaceValue: Story = {
	name: "Replace a value",
	render: () => (
		<Editor
			initial={configWith({
				vaults: [{ name: "stripe", secrets: ["api_key"] }],
			})}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			await canvas.findByRole("button", { name: "Replace value of api_key" }),
		);
		const field = canvas.getByLabelText("New value for api_key");
		await expect(field).toHaveValue("");
		await userEvent.type(field, "fake-rotated-do-not-use");
		await userEvent.click(canvas.getByRole("button", { name: "Save value" }));
		await expect(await canvas.findByText(/version 4/)).toBeVisible();
		await expect(
			canvas.getByText(
				"Redeploy the connected workloads to pick up the new value.",
			),
		).toBeVisible();
	},
};

/** Deleting a value the YAML does not list asks first. */
export const DeleteUndeclared: Story = {
	name: "Delete an undeclared value",
	render: () => (
		<Editor
			initial={configWith({
				vaults: [{ name: "stripe", secrets: ["api_key"] }],
			})}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			await canvas.findByRole("button", { name: "Delete old_token from AWS" }),
		);
		await expect(
			await screen.findByRole("alertdialog", {
				name: "Delete old_token from AWS?",
			}),
		).toBeInTheDocument();
		await userEvent.click(
			screen.getByRole("button", { name: "Delete from AWS" }),
		);
		await waitFor(() =>
			expect(canvas.queryByTestId("vault-secret-old_token")).toBeNull(),
		);
	},
};

/** AWS refused the listing: the reason inline, with a retry. */
export const AwsError: Story = {
	name: "AWS error",
	render: () => (
		<Editor
			initial={configWith({
				vaults: [{ name: "locked", secrets: ["api_key"] }],
			})}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			await canvas.findByText(/Couldn't read this vault from AWS/),
		).toBeVisible();
		await expect(
			canvas.getByRole("button", { name: "Try again" }),
		).toBeVisible();
	},
};

/** The listing is still in flight. */
export const Loading: Story = {
	render: () => (
		<Editor
			initial={configWith({ vaults: [{ name: "slow", secrets: ["api_key"] }] })}
		/>
	),
	play: async ({ canvasElement }) => {
		await expect(
			within(canvasElement).getByText("Checking AWS…"),
		).toBeVisible();
	},
};
