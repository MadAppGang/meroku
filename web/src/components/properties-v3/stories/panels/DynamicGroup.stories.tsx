import type { Meta, StoryObj } from "@storybook/react-vite";
import { fixtures } from "../../fixtures";
import { NODE_CATALOG_BY_ID_V3 } from "../../nodeCatalog";
import { PropertyPanel } from "../../PropertyPanel";
import { panelDecorator } from "../storyConfig";

const definition = NODE_CATALOG_BY_ID_V3.dynamicGroup;

const meta = {
	title: "Panels-v3/Dynamic Group",
	component: PropertyPanel,
	decorators: [panelDecorator],
} satisfies Meta<typeof PropertyPanel>;

export default meta;

/**
 * Canvas-only dynamic group: Layout tab only, proving the canvas-only
 * precedence rule — no Overview, Configuration, Operations or Logs tabs.
 */
export const Default: StoryObj<typeof meta> = {
	args: {
		definition,
		instance: fixtures.dynamicGroupDefault,
		theme: "light",
	},
};
