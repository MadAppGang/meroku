import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React from "react";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "../../../ui/table";
import { editorDecorator, editorParameters } from "../editors/editorStoryKit";

const meta = {
	title: "UI/Table Cell",
	component: TableCell,
	tags: ["!autodocs"],
	parameters: editorParameters,
	decorators: [editorDecorator],
} satisfies Meta<typeof TableCell>;

export default meta;
type Story = StoryObj<typeof meta>;

function Sample({ variant }: { variant: "default" | "code" | "compact" }) {
	return (
		<Table>
			<TableHeader>
				<TableRow>
					<TableHead>Workload</TableHead>
					<TableHead>Value</TableHead>
				</TableRow>
			</TableHeader>
			<TableBody>
				<TableRow>
					<TableCell>orders</TableCell>
					<TableCell variant={variant}>orders/</TableCell>
				</TableRow>
			</TableBody>
		</Table>
	);
}

export const Default: Story = { render: () => <Sample variant="default" /> };
export const Code: Story = { render: () => <Sample variant="code" /> };
export const Compact: Story = { render: () => <Sample variant="compact" /> };
