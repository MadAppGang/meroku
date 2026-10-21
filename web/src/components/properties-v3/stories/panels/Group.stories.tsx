import type { Meta, StoryObj } from "@storybook/react-vite";
import { fixtures } from "../../fixtures";
import { NODE_CATALOG_BY_ID_V3 } from "../../nodeCatalog";
import { PropertyPanel } from "../../PropertyPanel";
import { panelDecorator } from "../storyConfig";

const definition = NODE_CATALOG_BY_ID_V3.group;

const meta = {
	title: "Panels-v3/Group",
	component: PropertyPanel,
	decorators: [panelDecorator],
} satisfies Meta<typeof PropertyPanel>;

export default meta;

/**
 * Canvas-only node: a single Layout tab, no AWS actions or deploy controls.
 * Edits persist to the Layout channel only ("Save layout") and never reach
 * AWS.
 */
export const Default: StoryObj<typeof meta> = {
	args: {
		definition,
		instance: fixtures.groupDefault,
		theme: "light",
	},
};
