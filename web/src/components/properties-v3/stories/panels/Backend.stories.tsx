import type { Meta, StoryObj } from "@storybook/react-vite";
import { fixtures } from "../../fixtures";
import { NODE_CATALOG_BY_ID_V3 } from "../../nodeCatalog";
import { PropertyPanel } from "../../PropertyPanel";
import { panelArgTypes, panelDecorator } from "../storyConfig";

const definition = NODE_CATALOG_BY_ID_V3.backend;

/**
 * Mocked save adapter. `saveBehavior: "fail"` simulates a failed save so the
 * failure path (draft retained, baseline untouched) can be reviewed. No AWS
 * or Terraform calls are ever made from stories.
 */
const makeAdapters = (behavior: "succeed" | "fail") => ({
	onSaveChannel: async () =>
		behavior === "fail"
			? ({ ok: false, message: "Demo adapter: save failed (fixture)" } as const)
			: ({ ok: true } as const),
});

const meta = {
	title: "Panels-v3/Backend",
	component: PropertyPanel,
	decorators: [panelDecorator],
	parameters: {},
	argTypes: panelArgTypes,
	args: { saveBehavior: "succeed" },
} satisfies Meta<typeof PropertyPanel>;

export default meta;

/** Created backend with an explicitly configured CloudWatch destination → Logs tab. */
export const Default: StoryObj<typeof meta> = {
	args: {
		definition,
		instance: fixtures.backendDefault,
		theme: "light",
		demoTelemetry: true,
		deployed: false,
		adapters: makeAdapters("succeed"),
	},
};

/**
 * Unresolved telemetry: readings are explicitly absent (never fabricated) and
 * no CloudWatch destination exists, so the Logs tab does not render.
 */
export const UnresolvedTelemetry: StoryObj<typeof meta> = {
	args: {
		definition,
		instance: fixtures.backendUnresolvedTelemetry,
		theme: "light",
		demoTelemetry: true,
		adapters: makeAdapters("succeed"),
	},
};

/**
 * Save failure: edit any field (e.g. CPU or an env var) and save — the demo
 * adapter fails; the draft and prior baseline are retained.
 */
export const SaveFailure: StoryObj<typeof meta> = {
	args: {
		definition,
		instance: fixtures.backendDefault,
		theme: "light",
		demoTelemetry: true,
		adapters: makeAdapters("fail"),
	},
};
