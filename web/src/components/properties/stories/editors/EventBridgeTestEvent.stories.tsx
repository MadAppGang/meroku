import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React from "react";
import { expect, userEvent, within } from "storybook/test";
import type {
	TestEventRequest,
	TestEventResponse,
} from "../../../../api/infrastructure";
import type { YamlInfrastructureConfig } from "../../../../types/yamlConfig";
import { EventBridgeTestEvent } from "../../../EventBridgeTestEvent";
import {
	configWith,
	editorDecorator,
	editorParameters,
} from "./editorStoryKit";

const meta = {
	title: "Editors/EventBridge Test Event",
	tags: ["!autodocs"],
	parameters: editorParameters,
	decorators: [editorDecorator],
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** Answers like the server, echoing the event group it was given. */
const sendSynthetic = async (
	event: TestEventRequest,
): Promise<TestEventResponse> => ({
	success: true,
	eventId: "00000000-0000-0000-0000-000000000000",
	message: `Event sent to ${event.eventBusName}${
		event.eventGroupId ? ` in group ${event.eventGroupId}` : ""
	}`,
});

const sendFailing = async (): Promise<TestEventResponse> => ({
	success: false,
	message: "Event failed: InternalFailure: synthetic failure",
});

function Editor({
	config,
	send = sendSynthetic,
}: {
	config: YamlInfrastructureConfig;
	send?: (event: TestEventRequest) => Promise<TestEventResponse>;
}) {
	return <EventBridgeTestEvent config={config} sendTestEvent={send} />;
}

export const StandardBus: Story = {
	name: "Standard project bus",
	render: () => (
		<Editor config={configWith({ event_bus: { enabled: true } })} />
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.queryByLabelText("Event group (optional)")).toBeNull();
		await expect(canvas.getByText("demo-dev")).toBeVisible();
	},
};

export const EnhancedEventGroup: Story = {
	name: "Enhanced project bus: the event group",
	render: () => (
		<Editor
			config={configWith({ event_bus: { enabled: true, type: "enhanced" } })}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const group = canvas.getByLabelText("Event group (optional)");
		await userEvent.type(group, "order-1042");
		await userEvent.click(
			canvas.getByRole("button", { name: /Send Test Event/ }),
		);
		await expect(
			await canvas.findByText("Event sent to demo-dev in group order-1042"),
		).toBeVisible();
	},
};

export const EventGroupTooLong: Story = {
	name: "Enhanced project bus: an event group that is too long",
	render: () => (
		<Editor
			config={configWith({ event_bus: { enabled: true, type: "enhanced" } })}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.type(
			canvas.getByLabelText("Event group (optional)"),
			"g".repeat(116),
		);
		await expect(
			canvas.getByText(/An event group is at most 115 characters/),
		).toBeVisible();
		await expect(
			canvas.getByRole("button", { name: /Send Test Event/ }),
		).toBeDisabled();
	},
};

export const SendFailed: Story = {
	name: "Send failed",
	render: () => (
		<Editor
			config={configWith({ event_bus: { enabled: true } })}
			send={sendFailing}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: /Send Test Event/ }),
		);
		await expect(await canvas.findByText(/synthetic failure/)).toBeVisible();
	},
};
