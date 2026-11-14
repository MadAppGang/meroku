import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React from "react";
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

export const NoEvents: Story = {
	name: "No events",
	render: () => <ServiceEditor initial={baseConfig} />,
};

export const SubscribesAndProduces: Story = {
	name: "Subscribes and produces",
	render: () => (
		<ServiceEditor
			initial={withOrdersEvents(
				{
					subscribes: [
						{ detail_type: "UserSignedUp", source: "demo.backend" },
						{ detail_type_prefix: "Payment", bus: "default" },
					],
					produces: [{ detail_types: ["OrderCreated"] }],
				},
				{ event_bus: { enabled: true } },
			)}
		/>
	),
};

export const QueueSettings: Story = {
	name: "Queue settings",
	render: () => (
		<ServiceEditor
			initial={withOrdersEvents({
				subscribes: [{ detail_type: "UserSignedUp" }],
				queue: { visibility_timeout_seconds: 120, max_receive_count: 5 },
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
