import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React from "react";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import type { NeonAccessTarget } from "../types/neon";
import type { YamlInfrastructureConfig } from "../types/yamlConfig";
import {
	answerSchemas,
	createNeon,
	NeonConfigHarness,
	neonStoryConfig,
	reuseNeon,
	schemasLoaded,
	schemasNeverAnswer,
	schemasNoAuth,
	schemasNotDeployed,
} from "./NeonStoryFixtures.stories";
import { type NeonSchemasFn, ServiceDatabaseTab } from "./ServiceDatabaseTab";

/**
 * The Database tab of the backend and named-service panels, one story per
 * state. Every valid change writes immediately through `onConfigChange`.
 */

interface HarnessProps {
	config: YamlInfrastructureConfig;
	target: NeonAccessTarget;
	lookupSchemas?: NeonSchemasFn;
	onConfigChange?: (updates: Partial<YamlInfrastructureConfig>) => void;
}

function Harness({
	config,
	target,
	lookupSchemas = answerSchemas(schemasLoaded),
	onConfigChange,
}: HarnessProps) {
	return (
		<NeonConfigHarness initial={config} onChange={onConfigChange}>
			{(current, handleChange) => (
				<ServiceDatabaseTab
					config={current}
					onConfigChange={handleChange}
					target={target}
					lookupSchemas={lookupSchemas}
				/>
			)}
		</NeonConfigHarness>
	);
}

const meta = {
	title: "Panels/Service Database",
	component: Harness,
	parameters: { layout: "padded" },
	args: {
		onConfigChange: fn(),
	},
	argTypes: {
		config: { control: false },
		target: { control: false },
		lookupSchemas: { control: false },
		onConfigChange: { control: false },
	},
} satisfies Meta<typeof Harness>;

export default meta;
type Story = StoryObj<typeof meta>;

const backend: NeonAccessTarget = { kind: "backend" };
const reports: NeonAccessTarget = { kind: "service", name: "reports" };

/** The env has no Neon project yet. */
export const NoNeonProject: Story = {
	args: { config: neonStoryConfig(), target: backend },
};

/** The Neon project has no databases yet: the switch is off and disabled. */
export const NoDatabases: Story = {
	args: {
		config: neonStoryConfig({ neon: { ...createNeon, databases: [] } }),
		target: backend,
	},
};

/** Neon is set up and this service has no database. */
export const NotConnected: Story = {
	args: { config: neonStoryConfig({ neon: createNeon }), target: reports },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.queryByRole("button", { name: /save database access/i }),
		).toBeNull();
	},
};

/** Switching on writes the first database and read-only access immediately. */
export const Connecting: Story = {
	args: { config: neonStoryConfig({ neon: createNeon }), target: reports },
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			await canvas.findByRole("switch", { name: /give reports a database/i }),
		);
		await expect(args.onConfigChange).toHaveBeenCalledWith({
			services: [
				{
					name: "reports",
					container_port: 8080,
					neon: { database: "neondb", access: "readonly" },
				},
			],
		});
	},
};

/**
 * Login role shown under the chosen database before first deploy: matches the
 * SHA-256 role name for `(myapp, dev, reports, app)` and notes that meroku
 * creates it at the next deploy.
 */
export const RoleNameShown: Story = {
	args: { config: neonStoryConfig({ neon: reuseNeon }), target: reports },
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			await canvas.findByRole("switch", { name: /give reports a database/i }),
		);
		await expect(args.onConfigChange).toHaveBeenCalledTimes(1);
		await expect(
			await canvas.findByText("myapp_dev_reports_app_e5c4e06a"),
		).toBeInTheDocument();
		await expect(
			canvas.getByText(/meroku creates this role at the next deploy/i),
		).toBeInTheDocument();
	},
};

/** The backend reads and writes the app database. */
export const BackendReadWrite: Story = {
	args: {
		config: neonStoryConfig({
			neon: createNeon,
			workload: { backend_neon: { database: "app", access: "readwrite" } },
		}),
		target: backend,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			await canvas.findByText("myapp_dev_backend_app_f0819dbc"),
		).toBeInTheDocument();
	},
};

/** A named service with read-only access; switching access writes immediately. */
export const ServiceReadOnly: Story = {
	args: {
		config: neonStoryConfig({
			neon: reuseNeon,
			services: [
				{
					name: "reports",
					container_port: 8080,
					neon: { database: "analytics", access: "readonly" },
				},
			],
		}),
		target: reports,
	},
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			await canvas.findByText("myapp_dev_reports_analytics_010562fd"),
		).toBeInTheDocument();
		await userEvent.click(
			canvas.getByRole("radio", { name: "Read and write data" }),
		);
		await expect(args.onConfigChange).toHaveBeenCalledWith({
			services: [
				{
					name: "reports",
					container_port: 8080,
					neon: { database: "analytics", access: "readwrite" },
				},
			],
		});
	},
};

/**
 * Existing database schemas loaded from `/api/neon/schemas` as selectable
 * checkboxes; `public` is selected by default and toggling another schema
 * writes immediately.
 */
export const SchemasLoaded: Story = {
	args: {
		config: neonStoryConfig({
			neon: reuseNeon,
			workload: { backend_neon: { database: "app", access: "readwrite" } },
		}),
		target: backend,
		lookupSchemas: answerSchemas(schemasLoaded),
	},
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		const appCheckbox = await canvas.findByRole("checkbox", { name: "app" });
		await userEvent.click(appCheckbox);
		await expect(appCheckbox).toBeChecked();
		await expect(args.onConfigChange).toHaveBeenCalledWith({
			workload: {
				backend_neon: {
					database: "app",
					schemas: ["public", "app"],
					access: "readwrite",
				},
			},
		});
	},
};

/** Quiet loading state while `/api/neon/schemas` is still running. */
export const SchemasLoading: Story = {
	args: {
		config: neonStoryConfig({
			neon: reuseNeon,
			workload: { backend_neon: { database: "app", access: "readonly" } },
		}),
		target: backend,
		lookupSchemas: schemasNeverAnswer,
	},
};

/**
 * Schemas could not be read (`note` non-empty, e.g. database not deployed yet
 * or user not logged in): shows the `StatusNote` and falls back to manual
 * schema entry.
 */
export const SchemasUnavailable: Story = {
	args: {
		config: neonStoryConfig({
			neon: createNeon,
			workload: {
				backend_neon: {
					database: "app",
					schemas: ["app", "public"],
					access: "readwrite",
				},
			},
			services: [
				{
					name: "reports",
					container_port: 8080,
					neon: { database: "app", access: "readonly" },
				},
			],
		}),
		target: reports,
		lookupSchemas: answerSchemas(schemasNotDeployed),
	},
};

/** Not logged in to Neon: `note` explains how to log in, manual fallback stays usable. */
export const SchemasNoAuth: Story = {
	args: {
		config: neonStoryConfig({
			neon: reuseNeon,
			workload: { backend_neon: { database: "app", access: "readonly" } },
		}),
		target: backend,
		lookupSchemas: answerSchemas(schemasNoAuth),
	},
};

/**
 * Two schemas on one service when live schemas are loaded: `app` is in the
 * database's existing list, while `audit` is not and is labelled
 * "new — meroku creates it".
 */
export const TwoSchemas: Story = {
	args: {
		config: neonStoryConfig({
			neon: createNeon,
			workload: {
				backend_neon: {
					database: "app",
					schemas: ["app", "public"],
					access: "readwrite",
				},
			},
			services: [
				{
					name: "reports",
					container_port: 8080,
					neon: {
						database: "app",
						schemas: ["app", "audit"],
						access: "readonly",
					},
				},
			],
		}),
		target: reports,
		lookupSchemas: answerSchemas(schemasLoaded),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			await canvas.findByText("new — meroku creates it"),
		).toBeInTheDocument();
	},
};

/** An invalid schema name typed under Advanced shows its error and is never written. */
export const AdvancedSchemaError: Story = {
	args: {
		config: neonStoryConfig({
			neon: createNeon,
			workload: { backend_neon: { database: "app", access: "readwrite" } },
		}),
		target: backend,
	},
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			await canvas.findByRole("button", { name: "Advanced" }),
		);
		await userEvent.type(
			await canvas.findByLabelText("Another schema"),
			"pg_reports",
		);
		await expect(
			canvas.getByText(/postgres system schema/i),
		).toBeInTheDocument();
		await expect(args.onConfigChange).not.toHaveBeenCalled();
	},
};

/** Adding a valid schema under Advanced writes it immediately, and rotating the password bumps password_version. */
export const AdvancedAddSchemaAndRotatePassword: Story = {
	args: {
		config: neonStoryConfig({
			neon: createNeon,
			workload: { backend_neon: { database: "app", access: "readwrite" } },
		}),
		target: backend,
	},
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			await canvas.findByRole("button", { name: "Advanced" }),
		);
		await userEvent.type(
			await canvas.findByLabelText("Another schema"),
			"audit",
		);
		await userEvent.click(canvas.getByRole("button", { name: "Add" }));
		await expect(args.onConfigChange).toHaveBeenLastCalledWith({
			workload: {
				backend_neon: {
					database: "app",
					schemas: ["public", "audit"],
					access: "readwrite",
				},
			},
		});

		await userEvent.click(
			canvas.getByRole("button", { name: /issue a new password/i }),
		);
		await expect(args.onConfigChange).toHaveBeenLastCalledWith({
			workload: {
				backend_neon: {
					database: "app",
					schemas: ["public", "audit"],
					access: "readwrite",
					password_version: 2,
				},
			},
		});
		await expect(canvas.getByText(/password version 2/i)).toBeInTheDocument();
	},
};

/** Changing a service's database asks for confirmation: cancel reverts, confirm writes. */
export const DatabaseChangeConfirm: Story = {
	args: {
		config: neonStoryConfig({
			neon: createNeon,
			workload: { backend_neon: { database: "app", access: "readwrite" } },
		}),
		target: backend,
	},
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);

		const select = await canvas.findByRole("combobox", { name: "Database" });
		await userEvent.click(select);
		await userEvent.click(await body.findByRole("option", { name: "neondb" }));

		await expect(
			await body.findByRole("alertdialog", {
				name: /change backend's database to neondb/i,
			}),
		).toBeInTheDocument();
		await userEvent.click(body.getByRole("button", { name: "Cancel" }));
		await waitFor(() => expect(body.queryByRole("alertdialog")).toBeNull());
		await expect(args.onConfigChange).not.toHaveBeenCalled();
		await expect(select).toHaveTextContent("app");

		await userEvent.click(select);
		await userEvent.click(await body.findByRole("option", { name: "neondb" }));
		await userEvent.click(
			await body.findByRole("button", { name: "Change database" }),
		);
		await waitFor(() => expect(body.queryByRole("alertdialog")).toBeNull());
		await expect(args.onConfigChange).toHaveBeenCalledWith({
			workload: {
				backend_neon: { database: "neondb", access: "readwrite" },
			},
		});
	},
};

/** A reused project on Postgres 14: both access levels are disabled. */
export const ReusedPostgres14: Story = {
	args: {
		config: neonStoryConfig({
			neon: { ...reuseNeon, pg_version: 14 },
			workload: { backend_neon: { database: "app", access: "readonly" } },
		}),
		target: backend,
	},
};

/** A saved access block with problems: they show without editing. */
export const InvalidSaved: Story = {
	args: {
		config: neonStoryConfig({
			neon: createNeon,
			services: [
				{
					name: "reports",
					container_port: 8080,
					env_vars: { DATABASE_URL: "placeholder.invalid" },
					neon: { database: "missing", access: "readonly" },
				},
			],
		}),
		target: reports,
	},
};

/** Switching a saved service off removes its database access immediately. */
export const Disconnecting: Story = {
	args: {
		config: neonStoryConfig({
			neon: createNeon,
			workload: { backend_neon: { database: "app", access: "readwrite" } },
		}),
		target: backend,
	},
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			await canvas.findByRole("switch", { name: /give backend a database/i }),
		);
		await expect(args.onConfigChange).toHaveBeenCalledWith({
			workload: { backend_neon: undefined },
		});
	},
};
