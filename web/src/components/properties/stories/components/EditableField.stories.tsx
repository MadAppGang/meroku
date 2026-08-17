import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import {
	EditField as DefaultFixture,
	NumberField as NumberFixture,
} from "../../PrimitiveStoryFixtures.stories";
import { PropertyEditableField } from "../../PropertyPrimitives";
import { fullscreenParameters, primitiveDecorator } from "../storyConfig";

const meta = {
	title: "Components/Editable Field",
	component: PropertyEditableField,
	tags: ["!autodocs"],
	args: {
		id: "editable-field",
		label: "Service name",
		value: "worker-service",
		labelPlacement: "inline",
		onChange: fn(),
	},
	parameters: fullscreenParameters,
	decorators: [primitiveDecorator],
} satisfies Meta<typeof PropertyEditableField>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = { ...DefaultFixture, name: "Default" };
export const NumberWithUnit: Story = {
	...NumberFixture,
	name: "Number + unit",
};
