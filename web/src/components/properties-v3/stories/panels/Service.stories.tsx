import type { Meta, StoryObj } from "@storybook/react-vite";
import { fixtures } from "../../fixtures";
import { NODE_CATALOG_BY_ID_V3 } from "../../nodeCatalog";
import { PropertyPanel } from "../../PropertyPanel";
import { panelDecorator } from "../storyConfig";

const definition = NODE_CATALOG_BY_ID_V3.service;

const meta = {
	title: "Panels-v3/Service",
	component: PropertyPanel,
	decorators: [panelDecorator],
} satisfies Meta<typeof PropertyPanel>;

export default meta;

/**
 * NOT created: the CREATE / TF-REPLACE `name` field is still editable — the
 * resource does not exist yet, so no set-once lock applies.
 */
export const NotCreated: StoryObj<typeof meta> = {
	args: {
		definition,
		instance: fixtures.serviceNotCreated,
		theme: "light",
		demoTelemetry: true,
	},
};

/**
 * Created: `name` is locked as a set-once read-only row; AWS-assigned
 * identity (ECS service name, task definition ARN) is resolved.
 */
export const Created: StoryObj<typeof meta> = {
	args: {
		definition,
		instance: fixtures.serviceCreated,
		theme: "light",
		demoTelemetry: true,
	},
};
