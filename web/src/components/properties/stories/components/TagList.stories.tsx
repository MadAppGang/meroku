import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { TagListField as DefaultFixture } from "../../PrimitiveStoryFixtures.stories";
import { PropertyTagList } from "../../PropertyPrimitives";
import { fullscreenParameters, primitiveDecorator } from "../storyConfig";

const meta = {
	title: "Components/Tag List",
	component: PropertyTagList,
	tags: ["!autodocs"],
	args: {
		label: "Allowed subjects",
		values: ["repo:madappgang/circl:*"],
		onChange: fn(),
	},
	parameters: fullscreenParameters,
	decorators: [primitiveDecorator],
} satisfies Meta<typeof PropertyTagList>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = { ...DefaultFixture, name: "Default" };
