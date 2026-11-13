import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React from "react";
import { userEvent, within } from "storybook/test";
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
 * this component; `section` picks the tab. The lookup is replaced by a fixed
 * answer, and Save updates the story's own config.
 */

interface HarnessProps {
	config: YamlInfrastructureConfig;
	section: NeonPanelSection;
	lookup: NeonLookupFn;
}

function Harness({ config, section, lookup }: HarnessProps) {
	return (
		<NeonConfigHarness initial={config}>
			{(current, onConfigChange) => (
				<NeonNodeProperties
					config={current}
					onConfigChange={onConfigChange}
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
	argTypes: {
		config: { control: false },
		lookup: { control: false },
		section: { control: "radio", options: ["settings", "databases"] },
	},
} satisfies Meta<typeof Harness>;

export default meta;
type Story = StoryObj<typeof meta>;

/** No `neon:` block yet: one button sets it up. */
export const NotSetUp: Story = {
	args: {
		config: neonStoryConfig(),
		section: "settings",
		lookup: answer(loggedIn),
	},
};

/** Create mode, logged in: organization, region and name are choices. */
export const CreateMode: Story = {
	args: {
		config: neonStoryConfig({ neon: createNeon }),
		section: "settings",
		lookup: answer(loggedIn),
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

/** A project id typed under Advanced shows its error on every keystroke. */
export const AdvancedValidationError: Story = {
	args: {
		config: neonStoryConfig({ neon: reuseNeon }),
		section: "settings",
		lookup: answer(notLoggedIn),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			await canvas.findByRole("button", { name: /enter ids yourself/i }),
		);
		const field = await canvas.findByLabelText("Project id");
		await userEvent.clear(field);
		await userEvent.type(field, "Not_A_Project");
	},
};

/**
 * A deployed create-mode project switched to "Use an existing project": the
 * warning, and Save asks for confirmation.
 */
export const CreateToReuseSwitch: Story = {
	args: {
		config: neonStoryConfig({ neon: createNeon }),
		section: "settings",
		lookup: answer(loggedInDeployed),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			await canvas.findByRole("radio", { name: "Use an existing project" }),
		);
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

/** Databases tab, create mode: neondb comes with the project; app is new. */
export const DatabasesCreateMode: Story = {
	args: {
		config: neonStoryConfig({ neon: createNeon }),
		section: "databases",
		lookup: answer(loggedIn),
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

/** A new database's name with an error, checked on every keystroke. */
export const DatabaseNameInvalid: Story = {
	args: {
		config: neonStoryConfig({ neon: createNeon }),
		section: "databases",
		lookup: answer(loggedIn),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const field = await canvas.findByLabelText("Name of new database 2");
		await userEvent.clear(field);
		await userEvent.type(field, "My-App");
	},
};

/** Renaming a created database in a deployed project: kept, and a new one made. */
export const DatabaseRenameWarning: Story = {
	args: {
		config: neonStoryConfig({ neon: createNeon }),
		section: "databases",
		lookup: answer(loggedInDeployed),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const field = await canvas.findByLabelText("Name of new database 2");
		await userEvent.clear(field);
		await userEvent.type(field, "main");
	},
};
