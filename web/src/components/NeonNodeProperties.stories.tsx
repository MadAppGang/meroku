import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React from "react";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import type { YamlInfrastructureConfig } from "../types/yamlConfig";
import {
	type NeonLookupFn,
	NeonNodeProperties,
	type NeonPanelSection,
} from "./NeonNodeProperties";
import {
	answer,
	createNeon,
	loggedIn,
	loggedInDeployed,
	lookupFails,
	NeonConfigHarness,
	neonStoryConfig,
	neverAnswers,
	notLoggedIn,
	reuseNeon,
} from "./NeonStoryFixtures.stories";

/**
 * The env's Neon panel, one story per state. Both tabs of the Sidebar render
 * this component; `section` picks the tab. Valid changes write immediately
 * through `onConfigChange`.
 */

interface HarnessProps {
	config: YamlInfrastructureConfig;
	section: NeonPanelSection;
	lookup: NeonLookupFn;
	onConfigChange?: (updates: Partial<YamlInfrastructureConfig>) => void;
}

function Harness({ config, section, lookup, onConfigChange }: HarnessProps) {
	return (
		<NeonConfigHarness initial={config} onChange={onConfigChange}>
			{(current, handleChange) => (
				<NeonNodeProperties
					config={current}
					onConfigChange={handleChange}
					section={section}
					lookupNeon={lookup}
				/>
			)}
		</NeonConfigHarness>
	);
}

const meta = {
	title: "Panels/Neon Postgres",
	component: Harness,
	parameters: { layout: "padded" },
	args: {
		onConfigChange: fn(),
	},
	argTypes: {
		config: { control: false },
		lookup: { control: false },
		onConfigChange: { control: false },
		section: { control: "radio", options: ["settings", "databases"] },
	},
} satisfies Meta<typeof Harness>;

export default meta;
type Story = StoryObj<typeof meta>;

/** No `neon:` block yet: one button starts setup. */
export const NotSetUp: Story = {
	args: {
		config: neonStoryConfig(),
		section: "settings",
		lookup: answer(loggedIn),
	},
};

/** Setting up Neon: choosing an organization writes the complete `neon` block immediately. */
export const SettingUp: Story = {
	args: {
		config: neonStoryConfig(),
		section: "settings",
		lookup: answer(loggedIn),
	},
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);

		await userEvent.click(
			await canvas.findByRole("button", {
				name: /set up neon for this environment/i,
			}),
		);
		await expect(args.onConfigChange).not.toHaveBeenCalled();

		await userEvent.click(
			await canvas.findByRole("combobox", { name: "Organization" }),
		);
		await userEvent.click(await body.findByRole("option", { name: "My team" }));

		await expect(args.onConfigChange).toHaveBeenCalledWith({
			neon: {
				mode: "create",
				project: {
					name: "myapp-dev",
					org_id: "org-fake-000000",
					region_id: "aws-us-east-1",
					pg_version: 17,
				},
				databases: [],
			},
		});
	},
};

/** Create mode, logged in: editing the project name writes immediately without a Save button. */
export const CreateMode: Story = {
	args: {
		config: neonStoryConfig({ neon: createNeon }),
		section: "settings",
		lookup: answer(loggedIn),
	},
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.queryByRole("button", { name: /save neon settings/i }),
		).toBeNull();
		const nameField = await canvas.findByLabelText("Project name");
		await userEvent.clear(nameField);
		await expect(args.onConfigChange).not.toHaveBeenCalled();
		await userEvent.type(nameField, "myapp-staging");
		await expect(args.onConfigChange).toHaveBeenLastCalledWith({
			neon: {
				...createNeon,
				project: {
					...createNeon.project,
					name: "myapp-staging",
					org_id: "org-fake-000000",
					region_id: "aws-us-east-1",
					pg_version: 17,
				},
			},
		});
	},
};

/** Reuse mode, logged in: the project is picked from the user's projects. */
export const ReuseMode: Story = {
	args: {
		config: neonStoryConfig({ neon: reuseNeon }),
		section: "settings",
		lookup: answer(loggedIn),
	},
};

/** Not logged in to Neon: the login hint, and ids under Advanced. */
export const NoAuth: Story = {
	args: {
		config: neonStoryConfig({ neon: reuseNeon }),
		section: "settings",
		lookup: answer(notLoggedIn),
	},
};

/** The lookup has not answered yet. */
export const LookingUp: Story = {
	args: {
		config: neonStoryConfig({ neon: createNeon }),
		section: "settings",
		lookup: neverAnswers,
	},
};

/** The lookup request failed. */
export const LookupFailed: Story = {
	args: {
		config: neonStoryConfig({ neon: createNeon }),
		section: "settings",
		lookup: lookupFails,
	},
};

/** An invalid project id typed under Advanced shows its error and is never written. */
export const AdvancedValidationError: Story = {
	args: {
		config: neonStoryConfig({ neon: reuseNeon }),
		section: "settings",
		lookup: answer(notLoggedIn),
	},
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			await canvas.findByRole("button", { name: /enter ids yourself/i }),
		);
		const field = await canvas.findByLabelText("Project id");
		await userEvent.clear(field);
		await userEvent.type(field, "Not_A_Project");
		await expect(
			canvas.getByText(/3 to 63 lower-case letters/i),
		).toBeInTheDocument();
		await expect(args.onConfigChange).not.toHaveBeenCalled();
	},
};

/**
 * A deployed create-mode project switched to "Use an existing project": picking
 * a project opens confirmation immediately; cancel reverts to create mode, and
 * confirm writes the reuse block.
 */
export const CreateToReuseSwitch: Story = {
	args: {
		config: neonStoryConfig({ neon: createNeon }),
		section: "settings",
		lookup: answer(loggedInDeployed),
	},
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);

		await userEvent.click(
			await canvas.findByRole("radio", { name: "Use an existing project" }),
		);
		await expect(args.onConfigChange).not.toHaveBeenCalled();

		await userEvent.click(
			await canvas.findByRole("combobox", { name: "Project" }),
		);
		await userEvent.click(
			await body.findByRole("option", { name: /shared-dev/i }),
		);

		await expect(await body.findByRole("alertdialog")).toBeInTheDocument();
		await userEvent.click(body.getByRole("button", { name: "Cancel" }));
		await waitFor(() => expect(body.queryByRole("alertdialog")).toBeNull());
		await expect(args.onConfigChange).not.toHaveBeenCalled();
		await expect(
			canvas.getByRole("radio", { name: "Create a new project" }),
		).toBeChecked();

		await userEvent.click(
			canvas.getByRole("radio", { name: "Use an existing project" }),
		);
		await userEvent.click(
			await canvas.findByRole("combobox", { name: "Project" }),
		);
		await userEvent.click(
			await body.findByRole("option", { name: /shared-dev/i }),
		);
		await userEvent.click(await body.findByRole("button", { name: "Confirm" }));
		await waitFor(() => expect(body.queryByRole("alertdialog")).toBeNull());

		await expect(args.onConfigChange).toHaveBeenCalledWith({
			neon: {
				mode: "reuse",
				project_id: "fake-project-000000",
				pg_version: 17,
				databases: createNeon.databases,
			},
		});
	},
};

/** A reused project on Postgres 14: services cannot get access. */
export const ReusedPostgres14: Story = {
	args: {
		config: neonStoryConfig({
			neon: { ...reuseNeon, project_id: "fake-project-000001", pg_version: 14 },
		}),
		section: "settings",
		lookup: answer(loggedIn),
	},
};

/** Neon is in use by services: it cannot be removed until they let go. */
export const RemoveBlockedByServices: Story = {
	args: {
		config: neonStoryConfig({
			neon: createNeon,
			workload: {
				backend_neon: { database: "app", access: "readwrite" },
			},
		}),
		section: "settings",
		lookup: answer(loggedIn),
	},
};

/** Disconnecting Neon keeps its confirmation dialog and writes removal immediately on confirm. */
export const DisconnectNeon: Story = {
	args: {
		config: neonStoryConfig({ neon: createNeon }),
		section: "settings",
		lookup: answer(loggedIn),
	},
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);

		await userEvent.click(
			await canvas.findByRole("button", {
				name: /disconnect neon from this environment/i,
			}),
		);
		await userEvent.click(await body.findByRole("button", { name: "Cancel" }));
		await waitFor(() => expect(body.queryByRole("alertdialog")).toBeNull());
		await expect(args.onConfigChange).not.toHaveBeenCalled();

		await userEvent.click(
			canvas.getByRole("button", {
				name: /disconnect neon from this environment/i,
			}),
		);
		await userEvent.click(
			await body.findByRole("button", { name: "Disconnect" }),
		);
		await waitFor(() => expect(body.queryByRole("alertdialog")).toBeNull());
		await expect(args.onConfigChange).toHaveBeenCalledWith({ neon: undefined });
	},
};

/** Databases tab, create mode: adding a database writes immediately. */
export const DatabasesCreateMode: Story = {
	args: {
		config: neonStoryConfig({ neon: createNeon }),
		section: "databases",
		lookup: answer(loggedIn),
	},
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);

		await userEvent.click(
			await canvas.findByRole("combobox", { name: "Add a database" }),
		);
		await userEvent.click(
			await body.findByRole("option", { name: /new database/i }),
		);

		await expect(args.onConfigChange).toHaveBeenCalledWith({
			neon: {
				...createNeon,
				databases: [
					...(createNeon.databases ?? []),
					{ name: "app_2", create: true },
				],
			},
		});
	},
};

/** Databases tab, reuse mode, logged in: live databases are choices. */
export const DatabasesReuseMode: Story = {
	args: {
		config: neonStoryConfig({ neon: reuseNeon }),
		section: "databases",
		lookup: answer(loggedIn),
	},
};

/** Databases tab, no databases yet. */
export const DatabasesEmpty: Story = {
	args: {
		config: neonStoryConfig({ neon: { ...reuseNeon, databases: [] } }),
		section: "databases",
		lookup: answer(notLoggedIn),
	},
};

/** A database a service uses cannot be removed; the reason is shown. */
export const DatabaseUsedByService: Story = {
	args: {
		config: neonStoryConfig({
			neon: createNeon,
			services: [
				{
					name: "reports",
					container_port: 8080,
					neon: { database: "app", access: "readonly" },
				},
			],
		}),
		section: "databases",
		lookup: answer(loggedIn),
	},
};

/** A new database's invalid name shows an inline error and is never written. */
export const DatabaseNameInvalid: Story = {
	args: {
		config: neonStoryConfig({ neon: createNeon }),
		section: "databases",
		lookup: answer(loggedIn),
	},
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		const field = await canvas.findByLabelText("Name of new database 2");
		await userEvent.clear(field);
		await userEvent.type(field, "My-App");
		await expect(
			canvas.getByText(/lower-case letters, digits and underscores/i),
		).toBeInTheDocument();
		await expect(args.onConfigChange).not.toHaveBeenCalled();
	},
};

/**
 * Renaming a created database in a deployed project asks for confirmation on
 * Enter/blur: cancel reverts the input, and confirm writes the rename.
 */
export const DatabaseRenameWarning: Story = {
	args: {
		config: neonStoryConfig({ neon: createNeon }),
		section: "databases",
		lookup: answer(loggedInDeployed),
	},
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);

		const field = await canvas.findByLabelText("Name of new database 2");
		await userEvent.clear(field);
		await userEvent.type(field, "main");
		await expect(
			canvas.getByText(/the database app is kept in neon with its data/i),
		).toBeInTheDocument();
		await expect(args.onConfigChange).not.toHaveBeenCalled();

		await userEvent.keyboard("{Enter}");
		await expect(await body.findByRole("alertdialog")).toBeInTheDocument();
		await userEvent.click(body.getByRole("button", { name: "Cancel" }));
		await waitFor(() => expect(body.queryByRole("alertdialog")).toBeNull());
		await expect(args.onConfigChange).not.toHaveBeenCalled();
		await expect(field).toHaveValue("app");

		await userEvent.clear(field);
		await userEvent.type(field, "main{Enter}");
		await userEvent.click(await body.findByRole("button", { name: "Confirm" }));
		await waitFor(() => expect(body.queryByRole("alertdialog")).toBeNull());
		await expect(args.onConfigChange).toHaveBeenCalledWith({
			neon: {
				...createNeon,
				databases: [
					{ name: "neondb", create: false },
					{ name: "main", create: true },
				],
			},
		});
	},
};
