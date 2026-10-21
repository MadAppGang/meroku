import type { Meta, StoryObj } from "@storybook/react-vite";
import { fixtures } from "../../fixtures";
import { NODE_CATALOG_BY_ID_V3 } from "../../nodeCatalog";
import { PropertyPanel } from "../../PropertyPanel";
import { panelDecorator } from "../storyConfig";

const definition = NODE_CATALOG_BY_ID_V3.ecr;

const meta = {
	title: "Panels-v3/ECR",
	component: PropertyPanel,
	decorators: [panelDecorator],
} satisfies Meta<typeof PropertyPanel>;

export default meta;

/** Local strategy: trusted-account controls visible, cross-account fields hidden. */
export const LocalStrategy: StoryObj<typeof meta> = {
	args: {
		definition,
		instance: fixtures.ecrDefault,
		theme: "light",
		demoTelemetry: true,
	},
};

/** Cross-account strategy: account/region selectors revealed, trusted accounts hidden. */
export const CrossAccountStrategy: StoryObj<typeof meta> = {
	args: {
		definition,
		instance: fixtures.ecrCrossAccount,
		theme: "light",
		demoTelemetry: true,
	},
};
