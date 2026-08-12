import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React from "react";
import { expect, userEvent, within } from "storybook/test";
import { ServiceProperties } from "./ServiceProperties";

const meta = {
	title: "Properties/Service",
	component: ServiceProperties,
	tags: ["!autodocs"],
	parameters: {
		controls: { disable: true },
		layout: "fullscreen",
	},
} satisfies Meta<typeof ServiceProperties>;

export default meta;
type Story = StoryObj<typeof meta>;

const centerY = (element: Element) => {
	const bounds = element.getBoundingClientRect();
	return bounds.top + bounds.height / 2;
};

const verifyDefaultLayout = async (
	canvasElement: HTMLElement,
	theme: "dark" | "light",
) => {
	const canvas = within(canvasElement);
	const panel = canvas.getByRole("complementary", {
		name: "terminator service properties",
	});
	const cpu = canvas.getByLabelText("CPU").getBoundingClientRect();
	const memory = canvas.getByLabelText("Memory").getBoundingClientRect();
	const autoscaling = canvas.getByLabelText("Autoscaling grouped");
	const minimum = canvas.getByLabelText("Minimum grouped");
	const maximum = canvas.getByLabelText("Maximum grouped");

	await expect(panel).toHaveAttribute("data-theme", theme);
	await expect(panel.getBoundingClientRect().width).toBe(540);
	await expect(cpu.height).toBe(34);
	await expect(Math.abs(cpu.top - memory.top)).toBeLessThan(2);
	await expect(autoscaling).toBeChecked();
	await expect(minimum).toHaveValue(1);
	await expect(maximum).toHaveValue(5);
	await expect(canvas.queryByLabelText("Desired grouped")).toBeNull();
	await expect(Math.abs(centerY(autoscaling) - centerY(minimum))).toBeLessThan(
		2,
	);
	await expect(Math.abs(centerY(minimum) - centerY(maximum))).toBeLessThan(2);
	await expect(canvas.getByText("Environment")).toBeVisible();
	await expect(canvas.queryByText("Container & Process")).toBeNull();

	await userEvent.click(autoscaling);
	await expect(autoscaling).not.toBeChecked();
	await expect(canvas.getByLabelText("Desired grouped")).toHaveValue(2);
	await expect(canvas.queryByLabelText("Minimum grouped")).toBeNull();
	await expect(canvas.queryByLabelText("Maximum grouped")).toBeNull();
	await userEvent.click(autoscaling);
	await expect(autoscaling).toBeChecked();
	await userEvent.click(canvas.getByRole("button", { name: "Details" }));
	await expect(
		canvas.getByRole("heading", { name: "Container & Process" }),
	).toBeVisible();
	await expect(canvas.getByLabelText("Command override")).toBeVisible();
	await userEvent.click(canvas.getByRole("button", { name: "Setup" }));
	await userEvent.click(canvas.getByRole("button", { name: "Discard" }));
};

export const DefaultDark: Story = {
	name: "Default · Dark",
	render: () => (
		<div className="mp-story-stage" data-theme="dark">
			<ServiceProperties theme="dark" />
		</div>
	),
	play: async ({ canvasElement }) => {
		await verifyDefaultLayout(canvasElement, "dark");
	},
};

export const DefaultLight: Story = {
	name: "Default · Light",
	parameters: {
		backgrounds: { disable: true },
	},
	render: () => (
		<div className="mp-story-stage" data-theme="light">
			<ServiceProperties theme="light" />
		</div>
	),
	play: async ({ canvasElement }) => {
		await verifyDefaultLayout(canvasElement, "light");
		const canvas = within(canvasElement);
		const panel = canvas.getByRole("complementary", {
			name: "terminator service properties",
		});
		await expect(getComputedStyle(panel).backgroundColor).toBe(
			"rgb(255, 255, 255)",
		);
	},
};
