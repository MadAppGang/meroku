import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React from "react";
import { NoteCode, NoteDetail, StatusNote } from "./status-note";

/**
 * A one-line status with an icon, one story per tone. `NoteDetail` is the
 * secondary line, `NoteCode` a technical value inside plain copy.
 */
const meta = {
	title: "Components/StatusNote",
	component: StatusNote,
	parameters: { layout: "padded" },
	args: { children: <p>Ready — doesn't clash with your other environments</p> },
	argTypes: {
		tone: {
			control: "select",
			options: ["success", "warning", "error", "neutral", "pending"],
		},
		children: { control: false },
	},
} satisfies Meta<typeof StatusNote>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Success: Story = { args: { tone: "success" } };

export const Warning: Story = {
	args: {
		tone: "warning",
		children: <p>Clashes with staging — you won't be able to connect them.</p>,
	},
};

export const ErrorTone: Story = {
	name: "Error",
	args: {
		tone: "error",
		children: <p>A database name has up to 63 lower-case letters.</p>,
	},
};

export const Neutral: Story = {
	args: {
		tone: "neutral",
		children: <p>We couldn't re-check this just now.</p>,
	},
};

export const Pending: Story = {
	args: { tone: "pending", children: <p>Looking up your Neon projects…</p> },
};

/** With a secondary line and a technical value. */
export const WithDetail: Story = {
	args: {
		tone: "neutral",
		title: "Technical detail shown as a tooltip",
		children: (
			<>
				<p>
					Log in to Neon with the Neon CLI: run <NoteCode>neon login</NoteCode>{" "}
					in a terminal.
				</p>
				<NoteDetail>Then press Refresh.</NoteDetail>
			</>
		),
	},
};
