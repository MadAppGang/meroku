import type { Meta, StoryObj } from "@storybook/react-vite";
import {
	DefaultDark as DarkFixture,
	DefaultLight as LightFixture,
} from "../../ServicePanelStoryFixtures.stories";
import { ServiceProperties } from "../../ServiceProperties";

const meta = {
	title: "Panels/Service",
	component: ServiceProperties,
	tags: ["!autodocs"],
	parameters: {
		controls: { disable: true },
		layout: "fullscreen",
	},
} satisfies Meta<typeof ServiceProperties>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
	...LightFixture,
	name: "Default",
};

export const Dark: Story = {
	...DarkFixture,
	name: "Dark",
};
