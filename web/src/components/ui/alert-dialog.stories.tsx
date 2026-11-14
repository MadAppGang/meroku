import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React from "react";
import { expect, screen } from "storybook/test";
import {
	AlertDialog,
	AlertDialogAction,
	AlertDialogCancel,
	AlertDialogContent,
	AlertDialogDescription,
	AlertDialogFooter,
	AlertDialogHeader,
	AlertDialogTitle,
} from "./alert-dialog";

/**
 * A confirmation that blocks the page until it is answered. The action is the
 * default button; `variant="destructive"` when it deletes something for good.
 */
function Confirm({
	destructive,
	title,
	description,
	action,
}: {
	destructive?: boolean;
	title: string;
	description: string;
	action: string;
}) {
	return (
		<AlertDialog defaultOpen>
			<AlertDialogContent>
				<AlertDialogHeader>
					<AlertDialogTitle>{title}</AlertDialogTitle>
					<AlertDialogDescription>{description}</AlertDialogDescription>
				</AlertDialogHeader>
				<AlertDialogFooter>
					<AlertDialogCancel>Cancel</AlertDialogCancel>
					<AlertDialogAction variant={destructive ? "destructive" : undefined}>
						{action}
					</AlertDialogAction>
				</AlertDialogFooter>
			</AlertDialogContent>
		</AlertDialog>
	);
}

const meta = {
	title: "Components/AlertDialog",
	component: Confirm,
	parameters: { layout: "centered" },
} satisfies Meta<typeof Confirm>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
	args: {
		title: "Disconnect Neon from this environment?",
		description: "The project and its databases stay in Neon with their data.",
		action: "Disconnect",
	},
	play: async () => {
		await expect(
			await screen.findByRole("button", { name: "Disconnect" }),
		).toBeInTheDocument();
	},
};

export const Destructive: Story = {
	args: {
		destructive: true,
		title: "Delete old_token from AWS?",
		description:
			"The value is deleted for good. A workload that still lists it cannot start.",
		action: "Delete from AWS",
	},
	play: async () => {
		await expect(
			await screen.findByRole("button", { name: "Delete from AWS" }),
		).toBeInTheDocument();
	},
};
