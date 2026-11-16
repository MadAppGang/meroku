import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React from "react";
import { expect, userEvent, within } from "storybook/test";
import { ConfigWarningsStatus } from "./ConfigWarningsStatus";
import { fullReport, serviceNoHealthCheck } from "./configWarningStoryFixtures";

/**
 * The status-line control: "N warnings", opening every warning grouped by
 * item. Shown only when the report has warnings.
 */
const meta = {
	title: "Components/ConfigWarningsStatus",
	component: ConfigWarningsStatus,
	parameters: { layout: "centered" },
	args: { warnings: fullReport },
	decorators: [
		(Story) => (
			<div className="flex h-8 items-center bg-card px-4">
				<Story />
			</div>
		),
	],
} satisfies Meta<typeof ConfigWarningsStatus>;

export default meta;
type Story = StoryObj<typeof meta>;

/** Closed: the count. */
export const Closed: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("button", { name: "6 warnings" }),
		).toBeInTheDocument();
	},
};

/** One warning reads in the singular. */
export const Single: Story = {
	args: { warnings: [serviceNoHealthCheck] },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("button", { name: "1 warning" }),
		).toBeInTheDocument();
	},
};

/** Opened: every warning, grouped by item. */
export const Open: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button", { name: "6 warnings" }));
		const body = within(canvasElement.ownerDocument.body);
		await expect(
			await body.findByText(serviceNoHealthCheck.title),
		).toBeInTheDocument();
	},
};

/** No warnings: the control is absent. */
export const Empty: Story = {
	args: { warnings: [] },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.queryByTestId("config-warnings-status"),
		).not.toBeInTheDocument();
	},
};
