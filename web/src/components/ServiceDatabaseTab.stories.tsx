import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React from "react";
import { userEvent, within } from "storybook/test";
import type { NeonAccessTarget } from "../types/neon";
import type { YamlInfrastructureConfig } from "../types/yamlConfig";
import {
	createNeon,
	NeonConfigHarness,
	neonStoryConfig,
	reuseNeon,
} from "./NeonStoryFixtures.stories";
import { ServiceDatabaseTab } from "./ServiceDatabaseTab";

/**
 * The Database tab of the backend and named-service panels, one story per
 * state. Save updates the story's own config.
 */

interface HarnessProps {
	config: YamlInfrastructureConfig;
	target: NeonAccessTarget;
}

function Harness({ config, target }: HarnessProps) {
	return (
		<NeonConfigHarness initial={config}>
			{(current, onConfigChange) => (
				<ServiceDatabaseTab
					config={current}
					onConfigChange={onConfigChange}
					target={target}
				/>
			)}
		</NeonConfigHarness>
	);
}

const meta = {
	title: "Panels/Service Database",
	component: Harness,
	parameters: { layout: "padded" },
	argTypes: {
		config: { control: false },
		target: { control: false },
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
};

/** Switching on: the first database, public, read only, then Save. */
export const Connecting: Story = {
	args: { config: neonStoryConfig({ neon: createNeon }), target: reports },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			await canvas.findByRole("switch", { name: /give reports a database/i }),
		);
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
};

/** A named service with read-only access. */
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
};

/**
 * Two schemas on one service. `app` comes from the backend, so it is a
 * choice; `audit` is new and meroku creates it.
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
	},
};

/** A schema name typed under Advanced shows its error on every keystroke. */
export const AdvancedSchemaError: Story = {
	args: {
		config: neonStoryConfig({
			neon: createNeon,
			workload: { backend_neon: { database: "app", access: "readwrite" } },
		}),
		target: backend,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			await canvas.findByRole("button", { name: "Advanced" }),
		);
		await userEvent.type(
			await canvas.findByLabelText("Another schema"),
			"pg_reports",
		);
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

/** Switching a saved service off: Save removes its login at the next deploy. */
export const Disconnecting: Story = {
	args: {
		config: neonStoryConfig({
			neon: createNeon,
			workload: { backend_neon: { database: "app", access: "readwrite" } },
		}),
		target: backend,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			await canvas.findByRole("switch", { name: /give backend a database/i }),
		);
	},
};
