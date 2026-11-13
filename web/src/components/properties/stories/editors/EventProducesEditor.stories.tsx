import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React, { useState } from "react";
import { fn } from "storybook/test";
import type { EventProduce } from "../../../../types/yamlConfig";
import { EventProducesEditor } from "../../../EventProducesEditor";
import { editorDecorator, editorParameters } from "./editorStoryKit";

const meta = {
	title: "Editors/Event Produces",
	tags: ["!autodocs"],
	parameters: editorParameters,
	decorators: [editorDecorator],
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

function Editor({
	initial,
	busEnabled = true,
}: {
	initial: EventProduce[];
	busEnabled?: boolean;
}) {
	const [produces, setProduces] = useState(initial);
	return (
		<EventProducesEditor
			produces={produces}
			busEnabled={busEnabled}
			projectBusName="demo-dev"
			eventSource="demo.orders"
			onEntryChange={(index, entry) =>
				setProduces((prev) => prev.map((e, i) => (i === index ? entry : e)))
			}
			onEntryAdd={(entry) => setProduces((prev) => [...prev, entry])}
			onEntryRemove={(index) =>
				setProduces((prev) => prev.filter((_, i) => i !== index))
			}
		/>
	);
}

export const Empty: Story = {
	render: () => <Editor initial={[]} />,
};

export const Entries: Story = {
	render: () => (
		<Editor
			initial={[
				{ detail_types: ["OrderCreated", "OrderPaid"] },
				{ detail_types: ["Audit"], own_source_only: false, bus: "default" },
			]}
		/>
	),
};

export const AnyDetailType: Story = {
	name: "Any detail-type",
	render: () => <Editor initial={[{ any: true }]} />,
};

export const BusOff: Story = {
	name: "Project bus off",
	render: () => (
		<Editor initial={[{ detail_types: ["OrderCreated"] }]} busEnabled={false} />
	),
};

export const ReadOnlyCallbacks: Story = {
	name: "Callbacks only",
	render: () => (
		<EventProducesEditor
			produces={[{ detail_types: ["OrderCreated"] }]}
			busEnabled
			projectBusName="demo-dev"
			eventSource="demo.orders"
			onEntryChange={fn()}
			onEntryAdd={fn()}
			onEntryRemove={fn()}
		/>
	),
};
