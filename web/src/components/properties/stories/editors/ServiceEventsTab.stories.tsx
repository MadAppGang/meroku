import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React from "react";
import { expect, userEvent, within } from "storybook/test";
import type {
	WorkloadEvents,
	YamlInfrastructureConfig,
} from "../../../../types/yamlConfig";
import { ServiceEventsTab } from "../../../ServiceEventsTab";
import {
	baseConfig,
	ConfigHarness,
	configWith,
	editorDecorator,
	editorParameters,
} from "./editorStoryKit";

const meta = {
	title: "Editors/Service Events",
	tags: ["!autodocs"],
	parameters: editorParameters,
	decorators: [editorDecorator],
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

function ServiceEditor({ initial }: { initial: YamlInfrastructureConfig }) {
	return (
		<ConfigHarness initial={initial}>
			{(config, onChange) => (
				<ServiceEventsTab
					config={config}
					workloadName="orders"
					kind="service"
					events={config.services?.find((s) => s.name === "orders")?.events}
					onEventsChange={(events) =>
						onChange({
							services: (config.services || []).map((s) =>
								s.name === "orders" ? { ...s, events } : s,
							),
						})
					}
				/>
			)}
		</ConfigHarness>
	);
}

function withOrdersEvents(
	events: WorkloadEvents,
	overrides: Partial<YamlInfrastructureConfig> = {},
): YamlInfrastructureConfig {
	return configWith({
		...overrides,
		services: (baseConfig.services || []).map((s) =>
			s.name === "orders" ? { ...s, events } : s,
		),
	});
}

export const Empty: Story = {
	name: "Empty (project bus off)",
	render: () => <ServiceEditor initial={baseConfig} />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByText("Subscribes to nothing, so it has no queue."),
		).toBeVisible();
		await expect(canvas.queryByTestId("subscribes-section-project")).toBeNull();
		await expect(
			canvas.getByTestId("subscribes-section-default"),
		).toBeVisible();
		await expect(
			canvas.getByText(/Use \* as a wildcard: job\.\* matches every job event/),
		).toBeVisible();
	},
};

export const EnvironmentBusSubscriptions: Story = {
	name: "Environment-bus subscriptions",
	render: () => (
		<ServiceEditor
			initial={withOrdersEvents(
				{
					subscribes: [
						{ detail_type: "job.completed" },
						{ detail_type: "payment.received", source: "demo.backend" },
					],
					produces: [{ detail_types: ["order.created"] }],
				},
				{ event_bus: { enabled: true } },
			)}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const envSection = within(canvas.getByTestId("subscribes-section-project"));
		await expect(
			envSection.getByText("Environment bus (demo-dev)"),
		).toBeVisible();
		await expect(
			envSection.getByLabelText("Event type for job-completed"),
		).toHaveValue("job.completed");
		await expect(
			envSection.getByLabelText("Event type for payment-received"),
		).toHaveValue("payment.received");
		await expect(envSection.getByText("source = demo.backend")).toBeVisible();
	},
};

export const DefaultBusSubscriptions: Story = {
	name: "Default-bus subscriptions",
	render: () => (
		<ServiceEditor
			initial={withOrdersEvents(
				{
					subscribes: [
						{
							detail_type: "ECS Task State Change",
							source: "aws.ecs",
							bus: "default",
						},
						{ detail_type_prefix: "user.", bus: "default" },
					],
				},
				{ event_bus: { enabled: true } },
			)}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const defSection = within(canvas.getByTestId("subscribes-section-default"));
		await expect(
			defSection.getByText("Default bus (AWS services, CI)"),
		).toBeVisible();
		await expect(
			defSection.getByText(/publish only to the default bus/),
		).toBeVisible();
		await expect(
			defSection.getByLabelText("Event type for user-prefix-default"),
		).toHaveValue("user.*");
	},
};

export const WildcardSubscriptions: Story = {
	name: "Wildcard subscriptions",
	render: () => (
		<ServiceEditor
			initial={withOrdersEvents(
				{
					subscribes: [{ detail_type: "job.*" }],
				},
				{ event_bus: { enabled: true } },
			)}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const row = within(canvas.getByTestId("subscription-job"));
		await expect(row.getByLabelText("Event type for job")).toHaveValue("job.*");
		await expect(row.getByText("wildcard")).toBeVisible();
	},
};

export const WithFilters: Story = {
	name: "Subscriptions with filters",
	render: () => (
		<ServiceEditor
			initial={withOrdersEvents(
				{
					subscribes: [
						{
							detail_type: "job.*",
							source: "demo.worker",
							filters: [
								{ field: "detail.status", values: ["done", "failed"] },
								{ field: "detail.region", values: ["eu"] },
							],
						},
					],
				},
				{ event_bus: { enabled: true } },
			)}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const row = within(canvas.getByTestId("subscription-job"));
		const filterBtn = row.getByRole("button", { name: "Filter for job" });
		await expect(filterBtn).toHaveTextContent("3 filters");
		await expect(row.getByText("detail.status = done, failed")).toBeVisible();
		await userEvent.click(filterBtn);
		const filterBox = within(row.getByTestId("subscription-filter-job"));
		await expect(filterBox.getByLabelText("Source for job")).toHaveValue(
			"demo.worker",
		);
		await expect(
			filterBox.getByTestId("filter-row-detail.status"),
		).toBeVisible();
		await expect(
			filterBox.getByTestId("filter-row-detail.region"),
		).toBeVisible();
	},
};

export const ValidationErrors: Story = {
	name: "Validation errors",
	render: () => (
		<ServiceEditor
			initial={withOrdersEvents(
				{
					subscribes: [
						{
							detail_type: "job.*",
							filters: [
								{ field: "status", values: ["done"] },
								{ field: "detail.region", values: ["eu"] },
								{ field: "detail.region", values: ["us"] },
							],
						},
						{
							detail_type: "job.completed",
						},
					],
				},
				{ event_bus: { enabled: true } },
			)}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByText(/Field path must start with detail\./),
		).toBeVisible();
		await expect(
			canvas.getByText(/Field "detail\.region" is already filtered above/),
		).toBeVisible();

		// Duplicate key error when typing an already-used event type in the add form
		const envSection = within(canvas.getByTestId("subscribes-section-project"));
		const typeInput = envSection.getByLabelText(
			"Event type for environment bus",
		);
		await userEvent.type(typeInput, "job.completed");
		await expect(
			envSection.getByText(
				/Another subscription already has the key "job-completed"/,
			),
		).toBeVisible();
		await expect(
			envSection.getByRole("button", {
				name: "Add subscription to environment bus",
			}),
		).toBeDisabled();
	},
};

export const QueueSettings: Story = {
	name: "Queue settings",
	render: () => (
		<ServiceEditor
			initial={withOrdersEvents({
				subscribes: [{ detail_type: "UserSignedUp" }],
				queue: { visibility_timeout: 120, max_receive_count: 5 },
			})}
		/>
	),
};

export const Backend: Story = {
	render: () => (
		<ConfigHarness initial={baseConfig}>
			{(config, onChange) => (
				<ServiceEventsTab
					config={config}
					workloadName="backend"
					kind="backend"
					events={config.workload?.events}
					onEventsChange={(events) =>
						onChange({
							workload: { events } as YamlInfrastructureConfig["workload"],
						})
					}
				/>
			)}
		</ConfigHarness>
	),
};
