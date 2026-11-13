import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React, { useState } from "react";
import type { EventBusChoice } from "../../../../types/yamlConfig";
import { EventBusSelect } from "../../../EventProducesEditor";
import { editorDecorator, editorParameters } from "./editorStoryKit";

const meta = {
	title: "Editors/Event Bus Select",
	tags: ["!autodocs"],
	parameters: editorParameters,
	decorators: [editorDecorator],
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

function Select({
	initial,
	busEnabled,
}: {
	initial: EventBusChoice | undefined;
	busEnabled: boolean;
}) {
	const [bus, setBus] = useState(initial);
	return (
		<EventBusSelect
			value={bus}
			busEnabled={busEnabled}
			projectBusName="demo-dev"
			onChange={setBus}
			ariaLabel="Bus"
		/>
	);
}

export const FollowsTheSwitch: Story = {
	name: "Follows the switch",
	render: () => <Select initial={undefined} busEnabled />,
};

export const DefaultBus: Story = {
	name: "Default bus",
	render: () => <Select initial="default" busEnabled />,
};

export const ProjectBusOff: Story = {
	name: "Project bus off",
	render: () => <Select initial={undefined} busEnabled={false} />,
};
