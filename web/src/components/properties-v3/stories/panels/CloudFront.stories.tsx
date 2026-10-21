import type { Meta, StoryObj } from "@storybook/react-vite";
import { fixtures } from "../../fixtures";
import { NODE_CATALOG_BY_ID_V3 } from "../../nodeCatalog";
import { PropertyPanel } from "../../PropertyPanel";
import { panelDecorator } from "../storyConfig";

const definition = NODE_CATALOG_BY_ID_V3.cloudfront;

const meta = {
	title: "Panels-v3/CloudFront",
	component: PropertyPanel,
	decorators: [panelDecorator],
} satisfies Meta<typeof PropertyPanel>;

export default meta;

/**
 * Created distribution. Access logging here is S3-logging configuration only
 * — there is deliberately NO CloudWatch destination, hence no fabricated
 * CloudFront/S3 logging viewer and no Logs tab.
 */
export const Default: StoryObj<typeof meta> = {
	args: {
		definition,
		instance: fixtures.cloudFrontDefault,
		theme: "light",
		demoTelemetry: true,
	},
};
