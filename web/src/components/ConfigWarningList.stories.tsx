import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React from "react";
import { expect, within } from "storybook/test";
import { ConfigWarningList, ConfigWarningSection } from "./ConfigWarningList";
import {
	fullReport,
	serviceNoHealthCheck,
	servicePolicyWildcard,
	unsafeDocs,
} from "./configWarningStoryFixtures";

/**
 * Configuration warnings as a list: title, detail with what to do, and the
 * docs link. `flat` is one item's warnings (the sidebar); `byItem` is a whole
 * report under each item's name (the status-line popover).
 */
const meta = {
	title: "Components/ConfigWarningList",
	component: ConfigWarningList,
	parameters: { layout: "padded" },
	args: { warnings: [serviceNoHealthCheck] },
	argTypes: {
		layout: { control: "inline-radio", options: ["flat", "byItem"] },
	},
	decorators: [
		(Story) => (
			<div className="max-w-xl rounded-lg border bg-card p-4">
				<Story />
			</div>
		),
	],
} satisfies Meta<typeof ConfigWarningList>;

export default meta;
type Story = StoryObj<typeof meta>;

/** One warning with a multi-line detail and a docs link. */
export const Single: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByText(serviceNoHealthCheck.title),
		).toBeInTheDocument();
		const link = canvas.getByRole("link", { name: /how to fix this/i });
		await expect(link).toHaveAttribute("href", serviceNoHealthCheck.docs);
		await expect(link).toHaveAttribute("target", "_blank");
		await expect(link).toHaveAttribute("rel", "noreferrer");
	},
};

/** Two warnings about the same service; the second has no docs link. */
export const SeveralForOneItem: Story = {
	args: { warnings: [servicePolicyWildcard, serviceNoHealthCheck] },
};

/** A warning without a docs link shows no link. */
export const WithoutDocs: Story = {
	args: { warnings: [servicePolicyWildcard] },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.queryByRole("link")).not.toBeInTheDocument();
	},
};

/** A docs value that is not an absolute https URL is never linked. */
export const NonHttpsDocsNotLinked: Story = {
	args: { warnings: [unsafeDocs] },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.queryByRole("link")).not.toBeInTheDocument();
	},
};

/** A whole report, each item's warnings under its name, env-wide first. */
export const ByItem: Story = {
	args: { warnings: fullReport, layout: "byItem" },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("region", { name: "Environment" }),
		).toBeInTheDocument();
		await expect(
			canvas.getByRole("region", { name: "Service magento-bridge" }),
		).toBeInTheDocument();
	},
};

/** No warnings. */
export const Empty: Story = {
	args: { warnings: [] },
};

/** The sidebar section above every tab of the selected node. */
export const SidebarSection: Story = {
	args: { warnings: [servicePolicyWildcard, serviceNoHealthCheck] },
	render: (args) => <ConfigWarningSection warnings={args.warnings} />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("region", { name: "Configuration warnings" }),
		).toBeInTheDocument();
	},
};

/** The sidebar section renders nothing for a node without warnings. */
export const SidebarSectionEmpty: Story = {
	args: { warnings: [] },
	render: (args) => <ConfigWarningSection warnings={args.warnings} />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.queryByTestId("node-warnings")).not.toBeInTheDocument();
	},
};
