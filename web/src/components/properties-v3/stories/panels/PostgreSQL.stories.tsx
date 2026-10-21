import type { Meta, StoryObj } from "@storybook/react-vite";
import { fixtures } from "../../fixtures";
import { NODE_CATALOG_BY_ID_V3 } from "../../nodeCatalog";
import { PropertyPanel } from "../../PropertyPanel";
import { panelDecorator } from "../storyConfig";

const definition = NODE_CATALOG_BY_ID_V3.postgres;

const meta = {
	title: "Panels-v3/PostgreSQL",
	component: PropertyPanel,
	decorators: [panelDecorator],
} satisfies Meta<typeof PropertyPanel>;

export default meta;

/**
 * Created RDS instance (aurora = false): RDS-only instance class, storage,
 * multi-AZ and encryption fields are visible; Aurora capacity fields are not.
 * dbname/username/encryption/aurora remain editable AND destructive.
 */
export const Default: StoryObj<typeof meta> = {
	args: {
		definition,
		instance: fixtures.postgresRdsDefault,
		theme: "light",
		demoTelemetry: true,
	},
};

/**
 * Aurora Serverless v2 variant (aurora = true): min/max ACU capacity shown —
 * min_capacity 0 is a VALID value (pause when idle). RDS-only fields are
 * hidden.
 */
export const Aurora: StoryObj<typeof meta> = {
	args: {
		definition,
		instance: fixtures.postgresAurora,
		theme: "light",
		demoTelemetry: true,
	},
};

/**
 * Not created: destructive fields are framed as INITIAL CREATION (no
 * existing data — this configures the new resource); the confirmation gate
 * still applies, but the wording makes no destruction claim.
 */
export const NotCreated: StoryObj<typeof meta> = {
	args: {
		definition,
		instance: fixtures.postgresNotCreated,
		theme: "light",
	},
};

/**
 * Destructive pending scenario: with the Default fixture, edit
 * `postgres.dbname` (or toggle `postgres.aurora`) on the Configuration tab —
 * the save bar flags `recreates-data-loss` and saving opens a confirmation
 * naming the exact resource (myapp_postgres_dev) and property paths. Saving
 * itself destroys nothing; the next deploy may recreate and lose data.
 * Cancel preserves the draft; Discard resets to baseline.
 */
export const DestructivePendingChanges: StoryObj<typeof meta> = {
	args: {
		definition,
		instance: fixtures.postgresRdsDefault,
		theme: "light",
		initialTab: "configuration",
		demoTelemetry: true,
	},
};
