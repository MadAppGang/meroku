import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React from "react";
import { expect, userEvent, within } from "storybook/test";
import {
	eventBusToggle,
	unknownKind,
	unnamedService,
	vpcCidrOverlap,
} from "./configWarningStoryFixtures";
import { EnvWarningsPanel } from "./EnvWarningsPanel";

/**
 * Warnings with no node on the canvas, shown over the canvas without any
 * click: env-wide warnings, items the canvas does not show, and kinds this
 * UI does not know. One story per state.
 */
const meta = {
	title: "Components/EnvWarningsPanel",
	component: EnvWarningsPanel,
	parameters: { layout: "padded" },
	args: { warnings: [vpcCidrOverlap] },
	decorators: [
		(Story) => (
			<div className="max-w-md">
				<Story />
			</div>
		),
	],
} satisfies Meta<typeof EnvWarningsPanel>;

export default meta;
type Story = StoryObj<typeof meta>;

/** One env-wide warning: its title is visible without a click. */
export const Default: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const region = canvas.getByRole("region", {
			name: "Environment warnings",
		});
		await expect(
			within(region).getByText(vpcCidrOverlap.title),
		).toBeInTheDocument();
	},
};

/** Env-wide, an unnamed service with no node, and an unknown kind. */
export const Mixed: Story = {
	args: {
		warnings: [vpcCidrOverlap, unknownKind, eventBusToggle, unnamedService],
	},
};

/** A row opened to its detail and docs link. */
export const RowExpanded: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: new RegExp(vpcCidrOverlap.title) }),
		);
		await expect(
			canvas.getByRole("link", { name: /how to fix this/i }),
		).toHaveAttribute("href", vpcCidrOverlap.docs);
	},
};

/** Collapsed to its one-line header, which stays on screen. */
export const Collapsed: Story = {
	args: { defaultCollapsed: true },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("1 environment warning")).toBeVisible();
		await expect(
			canvas.queryByText(vpcCidrOverlap.title),
		).not.toBeInTheDocument();
		await userEvent.click(
			canvas.getByRole("button", { name: "Show environment warnings" }),
		);
		await expect(canvas.getByText(vpcCidrOverlap.title)).toBeVisible();
	},
};

/** The YAML would be refused by `meroku generate`, with a warning too. */
export const WithConfigError: Story = {
	args: {
		configError:
			"alb.enabled is true but no certificate covers api.example.com.\nAdd the domain to the certificate or set alb.enabled: false.",
	},
};

/** Refused, and nothing else to say. */
export const ConfigErrorOnly: Story = {
	args: {
		warnings: [],
		configError: "schema_version 99 is newer than this meroku. Update meroku.",
	},
};

/** No warnings and no error: the panel is absent. */
export const Empty: Story = {
	args: { warnings: [] },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.queryByTestId("env-warnings")).not.toBeInTheDocument();
	},
};
