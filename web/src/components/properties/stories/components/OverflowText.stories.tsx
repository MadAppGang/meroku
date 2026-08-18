import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React from "react";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { PropertyOverflowText } from "../../PropertyPrimitives";
import { fullscreenParameters, primitiveDecorator } from "../storyConfig";

const longValue =
	"STRIPE_WEBHOOK_SIGNING_SECRET_FOR_PRODUCTION_AP_SOUTHEAST_2";

const meta = {
	title: "Components/Overflow Text",
	component: PropertyOverflowText,
	tags: ["!autodocs"],
	args: {
		value: longValue,
		ariaLabel: `Environment variable ${longValue}`,
	},
	parameters: fullscreenParameters,
	decorators: [primitiveDecorator],
	render: (args) => (
		<div
			className="mp-primitive-demo mp-primitive-demo--field mp-compact-panel"
			data-surface="light"
			data-theme="light"
		>
			<PropertyOverflowText {...args} />
		</div>
	),
} satisfies Meta<typeof PropertyOverflowText>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const text = canvas.getByLabelText(`Environment variable ${longValue}`);
		await waitFor(() => expect(text).toHaveAttribute("data-overflow", "true"));
		await userEvent.hover(text);
		const page = within(canvasElement.ownerDocument.body);
		await expect(await page.findByRole("tooltip")).toHaveTextContent(longValue);
	},
};
