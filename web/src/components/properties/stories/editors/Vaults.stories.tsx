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

type FakeParameter = Omit<VaultSecretStatus, "declared" | "set" | "envName">;

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
	// Written before secret names were declared: one name is upper case.
	older: [
		{ name: "DATABASE_URL", type: "SecureString", version: 5 },
		{ name: "api_key", type: "SecureString", version: 1 },
	],
	empty: [],
	// Lists, but AWS refuses every write.
	readonly: [],
	// Its saved YAML still lists every name, so DELETE answers with a warning.
	racing: [{ name: "old_token", type: "SecureString", version: 2 }],
	locked: "denied",
	slow: "pending",
};

/** Vaults whose PUT AWS refuses. */
const PUT_DENIED = new Set(["readonly"]);
/** Vaults whose DELETE answers as for a name the saved YAML still lists. */
const DELETE_WARNS = new Set(["racing"]);
const STILL_LISTED =
	"still listed in the YAML; connected tasks will fail to start until it is set again or removed and applied";

// The server's name rules: PUT takes a declarable name, DELETE a wider one so
// an older upper-case parameter can still be deleted.
const SECRET_NAME = /^[a-z][a-z0-9_]{0,62}$/;
const DELETABLE_NAME = /^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$/;

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
		if (method !== "GET" && method !== "PUT" && method !== "DELETE") {
			return json({ error: "method not allowed: use GET, PUT or DELETE" }, 405);
		}
		if (method === "PUT" && PUT_DENIED.has(vault)) {
			return json(
				{ error: "AccessDeniedException: not allowed to write parameters" },
				403,
			);
		}
		if (method === "PUT") {
			const { name, value } = JSON.parse(String(init?.body)) as {
				name: string;
				value: string;
			};
			if (!SECRET_NAME.test(name)) {
				return json({ error: "invalid secret name" }, 400);
			}
			if (!value) return json({ error: "value is required" }, 400);
			const existing = params.find((p) => p.name === name);
			const version = (existing?.version ?? 0) + 1;
			if (existing) existing.version = version;
			else params.push({ name, type: "SecureString", version });
			return json({ name, version });
		}
		if (method === "DELETE") {
			const name = url.searchParams.get("name") ?? "";
			if (!DELETABLE_NAME.test(name)) {
				return json({ error: "invalid secret name" }, 400);
			}
			if (!params.some((p) => p.name === name)) {
				return json({ error: `${name} has no value in vault "${vault}"` }, 404);
			}
			aws[vault] = params.filter((p) => p.name !== name);
			return json({
				name,
				deleted: true,
				...(DELETE_WARNS.has(vault) ? { warning: STILL_LISTED } : {}),
			});
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

/**
 * A parameter written before names were declared, under an upper-case name:
 * it cannot join the YAML, so the row explains why and offers only deletion.
 */
export const UpperCaseName: Story = {
	name: "Upper-case name in AWS",
	render: () => (
		<Editor
			initial={configWith({ vaults: [{ name: "older", secrets: [] }] })}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const row = await canvas.findByTestId("vault-secret-DATABASE_URL");
		const inRow = within(row);
		await expect(
			inRow.getByText(/a secret name must be lower case/),
		).toBeVisible();
		await expect(
			inRow.getByText(/set it again under a lower-case name/),
		).toBeVisible();
		await expect(
			inRow.queryByRole("button", { name: "Add DATABASE_URL to the YAML" }),
		).toBeNull();
		await expect(
			inRow.getByRole("button", { name: "Delete DATABASE_URL from AWS" }),
		).toBeVisible();
		await expect(
			canvas.getByRole("button", { name: "Add api_key to the YAML" }),
		).toBeVisible();
	},
};

/** Importing names skips one that is not lower case, and says so. */
export const UpperCaseNameImport: Story = {
	name: "No secrets list: upper-case name not imported",
	render: () => (
		<Editor initial={configWith({ vaults: [{ name: "older" }] })} />
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			await canvas.findByText("Found in AWS: api_key."),
		).toBeVisible();
		await expect(
			canvas.getByText(
				/Not imported, because a secret name must be lower case/,
			),
		).toBeVisible();
		await userEvent.click(
			canvas.getByRole("button", { name: "Import names from AWS" }),
		);
		await expect(
			await canvas.findByRole("button", { name: "Replace value of api_key" }),
		).toBeVisible();
		await expect(
			canvas.getByRole("button", { name: "Delete DATABASE_URL from AWS" }),
		).toBeVisible();
		await expect(
			canvas.queryByRole("button", { name: "Add DATABASE_URL to the YAML" }),
		).toBeNull();
	},
};

/**
 * A name removed in this session may still be in the deployed task
 * definitions: the row and the delete dialog both say to apply first, and the
 * delete stays available.
 */
export const RemoveThenDelete: Story = {
	name: "Removed, not applied: delete warns",
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
			await canvas.findByRole("button", {
				name: "Remove api_key from the YAML",
			}),
		);
		const row = within(await canvas.findByTestId("vault-secret-api_key"));
		await expect(
			row.getByText("Not in the YAML any more, and not applied yet."),
		).toBeVisible();
		await expect(
			row.getByText(/still use this secret until you apply/),
		).toBeVisible();
		await userEvent.click(
			row.getByRole("button", { name: "Delete api_key from AWS" }),
		);
		const dialog = await screen.findByRole("alertdialog", {
			name: "Delete api_key from AWS?",
		});
		const inDialog = within(dialog);
		await expect(
			inDialog.getByText(
				"The deployed workloads still use this secret until you apply.",
			),
		).toBeVisible();
		await expect(
			inDialog.getByText(/Deleting it now makes their new tasks fail to start/),
		).toBeVisible();
		await expect(dialog).not.toHaveTextContent(/no workload gets it/);
		await userEvent.click(
			inDialog.getByRole("button", { name: "Delete from AWS" }),
		);
		await waitFor(() =>
			expect(canvas.queryByTestId("vault-secret-api_key")).toBeNull(),
		);
	},
};

/** The server's DELETE warning is shown when it sends one. */
export const DeleteWarning: Story = {
	name: "Delete answered with a warning",
	render: () => (
		<Editor
			initial={configWith({ vaults: [{ name: "racing", secrets: [] }] })}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			await canvas.findByRole("button", { name: "Delete old_token from AWS" }),
		);
		await userEvent.click(
			within(
				await screen.findByRole("alertdialog", {
					name: "Delete old_token from AWS?",
				}),
			).getByRole("button", { name: "Delete from AWS" }),
		);
		await expect(
			await canvas.findByText(`old_token: ${STILL_LISTED}`),
		).toBeVisible();
	},
};

/** A refused write keeps what was typed, so it can be sent again. */
export const AddFailureKeepsValue: Story = {
	name: "Add refused by AWS keeps the value",
	render: () => (
		<Editor
			initial={configWith({ vaults: [{ name: "readonly", secrets: [] }] })}
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
			canvas.getByRole("button", { name: "Add secret to readonly" }),
		);
		await expect(
			await canvas.findByText(/AWS refused the request/),
		).toBeVisible();
		await expect(canvas.getByLabelText("New secret name")).toHaveValue(
			"api_key",
		);
		await expect(canvas.getByLabelText("Value")).toHaveValue(
			"fake-value-do-not-use",
		);
		await expect(
			canvas.getByRole("button", { name: "Add secret to readonly" }),
		).toBeEnabled();
	},
};

/** A name AWS already has, but the YAML does not list, is refused in the form. */
export const AddNameAlreadyInAws: Story = {
	name: "Add a name already in AWS",
	render: () => (
		<Editor
			initial={configWith({ vaults: [{ name: "stripe", secrets: [] }] })}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await canvas.findByTestId("vault-secret-old_token");
		await userEvent.type(canvas.getByLabelText("New secret name"), "old_token");
		await userEvent.type(
			canvas.getByLabelText("Value"),
			"fake-value-do-not-use",
		);
		await expect(
			canvas.getByText(
				"old_token is already in AWS — use Add to YAML on its row.",
			),
		).toBeVisible();
		await expect(
			canvas.getByRole("button", { name: "Add secret to stripe" }),
		).toBeDisabled();
	},
};

/** The limit is 4096 bytes: 2049 two-byte characters are over it. */
export const ValueTooLarge: Story = {
	name: "Value over 4096 bytes",
	render: () => (
		<Editor
			initial={configWith({ vaults: [{ name: "empty", secrets: [] }] })}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await canvas.findByText("This vault has no secrets yet.");
		await userEvent.click(canvas.getByLabelText("Value"));
		await userEvent.paste("é".repeat(2049));
		await expect(
			canvas.getByText("The value is 4098 bytes; the limit is 4096 bytes."),
		).toBeVisible();
	},
};
