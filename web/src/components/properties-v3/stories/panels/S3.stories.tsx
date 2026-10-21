import type { Meta, StoryObj } from "@storybook/react-vite";
import { fixtures } from "../../fixtures";
import { NODE_CATALOG_BY_ID_V3 } from "../../nodeCatalog";
import { PropertyPanel } from "../../PropertyPanel";
import { panelDecorator } from "../storyConfig";

const definition = NODE_CATALOG_BY_ID_V3.s3;

const meta = {
	title: "Panels-v3/S3",
	component: PropertyPanel,
	decorators: [panelDecorator],
} satisfies Meta<typeof PropertyPanel>;

export default meta;

/**
 * Created bucket. Immutable name (TF-REPLACE) stays editable and destructive;
 * no CloudWatch destination exists so no Logs tab renders for S3.
 */
export const Default: StoryObj<typeof meta> = {
	args: {
		definition,
		instance: fixtures.s3Default,
		theme: "light",
		demoTelemetry: true,
	},
};
