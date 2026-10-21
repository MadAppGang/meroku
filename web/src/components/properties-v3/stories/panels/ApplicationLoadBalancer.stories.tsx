import type { Meta, StoryObj } from "@storybook/react-vite";
import { fixtures } from "../../fixtures";
import { NODE_CATALOG_BY_ID_V3 } from "../../nodeCatalog";
import { PropertyPanel } from "../../PropertyPanel";
import { panelDecorator } from "../storyConfig";

const definition = NODE_CATALOG_BY_ID_V3.alb;

const meta = {
	title: "Panels-v3/Application Load Balancer",
	component: PropertyPanel,
	decorators: [panelDecorator],
} satisfies Meta<typeof PropertyPanel>;

export default meta;

/**
 * Derived-routing precedence: routing rules are `derived`, so they render as
 * a read-only Overview/Relationships overview even though routes are
 * Terraform-managed — never editable in Configuration.
 */
export const Default: StoryObj<typeof meta> = {
	args: {
		definition,
		instance: fixtures.albDefault,
		theme: "light",
		demoTelemetry: true,
	},
};
