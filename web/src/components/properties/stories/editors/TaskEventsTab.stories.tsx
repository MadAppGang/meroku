import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React from "react";
import { expect, within } from "storybook/test";
import type { YamlInfrastructureConfig } from "../../../../types/yamlConfig";
import { TaskEventsTab } from "../../../TaskEventsTab";
import {
	baseConfig,
	ConfigHarness,
	configWith,
	editorDecorator,
	editorParameters,
} from "./editorStoryKit";

const meta = {
	title: "Editors/Task Events",
	tags: ["!autodocs"],
	parameters: editorParameters,
	decorators: [editorDecorator],
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

function Editor({ initial }: { initial: YamlInfrastructureConfig }) {
	return (
		<ConfigHarness initial={initial}>
			{(config, onChange) => (
				<TaskEventsTab
					config={config}
					taskName="nightly"
					kind="scheduled_task"
					events={
						config.scheduled_tasks?.find((t) => t.name === "nightly")?.events
					}
					onEventsChange={(events) =>
						onChange({
							scheduled_tasks: (config.scheduled_tasks || []).map((t) =>
								t.name === "nightly" ? { ...t, events } : t,
							),
						})
					}
				/>
			)}
		</ConfigHarness>
	);
}

export const NoProduces: Story = {
	name: "Produces nothing",
	render: () => <Editor initial={baseConfig} />,
};

export const Produces: Story = {
	render: () => (
		<Editor
			initial={configWith({
				scheduled_tasks: [
					{
						name: "nightly",
						schedule: "rate(1 day)",
						events: { produces: [{ detail_types: ["ReportReady"] }] },
					},
				],
			})}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.queryByTestId("task-events-bus-arn")).toBeNull();
	},
};

export const ProducesOnEnhancedBus: Story = {
	name: "Produces, on an Enhanced bus",
	render: () => (
		<Editor
			initial={configWith({
				event_bus: { enabled: true, type: "enhanced" },
				scheduled_tasks: [
					{
						name: "nightly",
						schedule: "rate(1 day)",
						events: { produces: [{ detail_types: ["ReportReady"] }] },
					},
				],
			})}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const vars = within(canvas.getByTestId("task-events-bus-arn"));
		await expect(vars.getByText("EVENT_BUS_ARN")).toBeVisible();
		await expect(vars.getByText("EVENT_BUS_NAME=demo-dev")).toBeVisible();
	},
};
