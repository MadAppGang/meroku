import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React from "react";
import type { YamlInfrastructureConfig } from "../../../../types/yamlConfig";
import { EventBusSettings } from "../../../EventBusSettings";
import {
	baseConfig,
	ConfigHarness,
	configWith,
	editorDecorator,
	editorParameters,
} from "./editorStoryKit";

const meta = {
	title: "Editors/Event Bus Settings",
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
				<EventBusSettings config={config} onConfigChange={onChange} />
			)}
		</ConfigHarness>
	);
}

export const Off: Story = {
	render: () => <Editor initial={baseConfig} />,
};

export const OnWithFollowingEntries: Story = {
	name: "On, with entries following it",
	render: () => (
		<Editor
			initial={configWith({
				event_bus: { enabled: true },
				workload: {
					events: {
						subscribes: [{ detail_type: "OrderCreated" }],
						produces: [{ detail_types: ["UserSignedUp"] }],
					},
				} as YamlInfrastructureConfig["workload"],
			})}
		/>
	),
};
