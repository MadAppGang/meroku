import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React from "react";
import { expect, userEvent, waitFor, within } from "storybook/test";
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
	const port = canvas.getByLabelText("Port grouped").getBoundingClientRect();
	const health = canvas.getByLabelText("Health path").getBoundingClientRect();
	const cpu = canvas.getByLabelText("CPU").getBoundingClientRect();
	const memory = canvas.getByLabelText("Memory").getBoundingClientRect();
	const autoscaling = canvas.getByLabelText("Autoscaling grouped");
	const minimum = canvas.getByLabelText("Minimum grouped");
	const maximum = canvas.getByLabelText("Maximum grouped");

	await expect(panel).toHaveAttribute("data-theme", theme);
	await expect(panel.getBoundingClientRect().width).toBe(540);
	await expect(cpu.height).toBe(34);
	await expect(Math.abs(cpu.top - memory.top)).toBeLessThan(2);
	await expect(Math.abs(port.left - cpu.left)).toBeLessThan(2);
	await expect(Math.abs(health.left - memory.left)).toBeLessThan(2);
	await expect(autoscaling).toBeChecked();
	await expect(minimum).toHaveValue(1);
	await expect(maximum).toHaveValue(5);
	await expect(canvas.queryByLabelText("Desired grouped")).toBeNull();
	await expect(Math.abs(centerY(autoscaling) - centerY(minimum))).toBeLessThan(
		2,
	);
	await expect(Math.abs(centerY(minimum) - centerY(maximum))).toBeLessThan(2);
	await expect(canvas.getByText("Environment")).toBeVisible();
	await expect(canvas.getAllByText("System")).toHaveLength(2);
	await expect(canvas.getByText("Manual")).toBeVisible();
	await expect(canvas.getByText("postgres")).toBeVisible();
	await expect(canvas.getByText("shared/prod")).toBeVisible();
	await expect(canvas.queryByText("Container & Process")).toBeNull();

	const targetTrigger = canvas.getByRole("button", {
		name: "Edit autoscaling target",
	});
	await userEvent.click(targetTrigger);
	const page = within(canvasElement.ownerDocument.body);
	const targetDialog = page.getByRole("dialog", {
		name: "Autoscaling target",
	});
	await waitFor(() => expect(targetDialog).toBeVisible());
	const targetSlider = page.getByRole("slider", {
		name: "Autoscaling CPU target",
	});
	await expect(targetSlider).toHaveAttribute("aria-valuenow", "70");
	targetSlider.focus();
	await userEvent.keyboard("{ArrowRight}");
	await expect(targetSlider).toHaveAttribute("aria-valuenow", "75");
	await expect(targetTrigger).toHaveTextContent("Target 75%");
	await userEvent.keyboard("{ArrowLeft}");
	await expect(targetSlider).toHaveAttribute("aria-valuenow", "70");
	await userEvent.keyboard("{Escape}");
	await waitFor(() => expect(targetDialog).not.toBeVisible());

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
		<div className="mp-story-stage" data-surface="light" data-theme="light">
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
		await expect(
			getComputedStyle(panel).getPropertyValue("--mp-accent").trim(),
		).toBe("#b7f500");
	},
};
