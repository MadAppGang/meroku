import type { Meta, StoryObj } from "@storybook/react-vite";
import { fixtures } from "../../fixtures";
import { NODE_CATALOG_BY_ID_V3 } from "../../nodeCatalog";
import { PropertyPanel } from "../../PropertyPanel";
import { panelDecorator } from "../storyConfig";

const definition = NODE_CATALOG_BY_ID_V3.route53;

const meta = {
	title: "Panels-v3/Route 53",
	component: PropertyPanel,
	decorators: [panelDecorator],
} satisfies Meta<typeof PropertyPanel>;

export default meta;

/**
 * Created zone: `is_dns_root` is a CREATE field locked after creation;
 * domain name / zone creation remain destructive (TF-REPLACE).
 */
export const Default: StoryObj<typeof meta> = {
	args: {
		definition,
		instance: fixtures.route53Default,
		theme: "light",
		demoTelemetry: true,
	},
};
