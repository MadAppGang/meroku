import type { Meta, StoryObj } from "@storybook/react-vite";
import { ClientApp as DraftFixture } from "../../NodePanelStoryFixtures.stories";
import { PropertyPanel } from "../../PropertyPanel";
import {
	fullscreenParameters,
	lightPanelDecorator,
	panelArgTypes,
} from "../storyConfig";

const meta = {
	title: "Panels/Client App",
	component: PropertyPanel,
	tags: ["!autodocs"],
	parameters: fullscreenParameters,
	decorators: [lightPanelDecorator],
	argTypes: panelArgTypes,
} satisfies Meta<typeof PropertyPanel>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Draft: Story = { ...DraftFixture, name: "Draft" };
