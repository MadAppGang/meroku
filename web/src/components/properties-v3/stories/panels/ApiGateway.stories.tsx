import type { Meta, StoryObj } from "@storybook/react-vite";
import { fixtures } from "../../fixtures";
import { NODE_CATALOG_BY_ID_V3 } from "../../nodeCatalog";
import { PropertyPanel } from "../../PropertyPanel";
import { panelDecorator } from "../storyConfig";

const definition = NODE_CATALOG_BY_ID_V3.apiGateway;

const meta = {
	title: "Panels-v3/API Gateway",
	component: PropertyPanel,
	decorators: [panelDecorator],
} satisfies Meta<typeof PropertyPanel>;

export default meta;

/**
 * Created HTTP API with an explicitly configured CloudWatch access-log
 * destination: the Logs tab (viewer / destination / retention) renders.
 * Routes are derived → read-only Overview/Relationships.
 */
export const Default: StoryObj<typeof meta> = {
	args: {
		definition,
		instance: fixtures.apiGatewayDefault,
		theme: "light",
		demoTelemetry: true,
	},
};
