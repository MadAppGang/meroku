import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React from "react";
import { expect, userEvent, waitFor, within } from "storybook/test";
import type {
	WorkloadEvents,
	YamlInfrastructureConfig,
} from "../../../../types/yamlConfig";
import {
	type DeployedEvents,
	orderedQueueMayBeDeployed,
	orderedQueueRereader,
	updateWorkloadEvents,
} from "../../../../utils/workloadEvents";
import { ServiceEventsTab } from "../../../ServiceEventsTab";
import { Button } from "../../../ui/button";
import {
	baseConfig,
	ConfigHarness,
	configWith,
	controlledReads,
	deployedEvents,
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

function ServiceEditor({
	initial,
	deployed,
	reread,
	onSave,
	serviceAdder = false,
}: {
	initial: YamlInfrastructureConfig;
	/** GET /api/events/deployed's answer; absent means not known yet. */
	deployed?: DeployedEvents;
	/** A fresh read, answered by the story. Absent: no re-read. */
	reread?: () => Promise<DeployedEvents>;
	/** Sees every config the app would save. */
	onSave?: (config: YamlInfrastructureConfig) => void;
	/** Shows an "Add service" button and the service names, as another panel would edit them. */
	serviceAdder?: boolean;
}) {
	return (
		<ConfigHarness initial={initial} onSave={onSave}>
			{(config, onChange, onUpdate) => (
				<>
					{serviceAdder && (
						<>
							<Button
								onClick={() =>
									onChange({
										services: [...(config.services || []), { name: "billing" }],
									})
								}
							>
								Add service
							</Button>
							<p hidden data-testid="service-names">
								{(config.services || []).map((s) => s.name).join(",")}
							</p>
						</>
					)}
					<ServiceEventsTab
						orderedQueueMayExist={orderedQueueMayBeDeployed(deployed, "orders")}
						rereadOrderedQueueMayExist={orderedQueueRereader(reread, "orders")}
						onEventsUpdate={(update) =>
							onUpdate(config.env, (current) =>
								updateWorkloadEvents(current, "orders", update),
							)
						}
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
				</>
			)}
		</ConfigHarness>
	);
}

/** The deployed-state reads of the FIFO decision stories. */
const ledgerReads = controlledReads<DeployedEvents>();
/** The configs the FIFO decision stories saved. */
const ledgerSaves: YamlInfrastructureConfig[] = [];
const recordLedgerSave = (config: YamlInfrastructureConfig) => {
	ledgerSaves.push(config);
};
const resetLedger = () => {
	ledgerReads.reset();
	ledgerSaves.length = 0;
};
const ordersEvents = (config: YamlInfrastructureConfig | undefined) =>
	config?.services?.find((s) => s.name === "orders")?.events;

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
	name: "Project-bus subscriptions",
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
		await expect(envSection.getByText("Project bus (demo-dev)")).toBeVisible();
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
		const typeInput = envSection.getByLabelText("Event type for project bus");
		await userEvent.type(typeInput, "job.completed");
		await expect(
			envSection.getByText(
				/Another subscription already has the key "job-completed"/,
			),
		).toBeVisible();
		await expect(
			envSection.getByRole("button", {
				name: "Add subscription to project bus",
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

/** An Enhanced project bus; the event task's rule sits on the default bus. */
const enhancedBus: Partial<YamlInfrastructureConfig> = {
	event_bus: { enabled: true, type: "enhanced" },
	event_processor_tasks: [
		{
			name: "ingest",
			rules: [
				{
					name: "order-created",
					sources: ["demo.orders"],
					detail_types: ["OrderCreated"],
					bus: "default",
				},
			],
		},
	],
};

export const EnhancedOrderedSubscription: Story = {
	name: "Enhanced: an ordered subscription",
	render: () => (
		<ServiceEditor
			initial={withOrdersEvents(
				{
					subscribes: [
						{ detail_type: "payment.completed" },
						{ name: "ledger", detail_type_prefix: "ledger.", ordered: true },
					],
				},
				enhancedBus,
			)}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByLabelText("Keep order for ledger")).toBeChecked();
		await expect(
			canvas.getByLabelText("Keep order for payment-completed"),
		).not.toBeChecked();
		await expect(canvas.getByText("EVENT_FIFO_QUEUE_URL")).toBeVisible();
		await expect(canvas.getByText("EVENT_BUS_ARN")).toBeVisible();
		await expect(canvas.getByTestId("event-flow-fifo")).toBeVisible();
		await expect(
			canvas.getByText(/ordered entries share one ordered subscriber/),
		).toBeVisible();
	},
};

export const EnhancedLastOrderedTurnedOff: Story = {
	name: "Enhanced: turning the last ordered subscription off retires its queue",
	render: () => (
		<ServiceEditor
			initial={withOrdersEvents(
				{
					subscribes: [
						{ name: "ledger", detail_type_prefix: "ledger.", ordered: true },
					],
				},
				enhancedBus,
			)}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByLabelText("Keep order for ledger"));
		await expect(
			canvas.getByText(
				/The ordered queue is kept, with any events still in it/,
			),
		).toBeVisible();
		await expect(canvas.getByTestId("fifo-retired")).toBeVisible();
		await userEvent.click(canvas.getByLabelText("Keep order for ledger"));
		await expect(canvas.queryByTestId("fifo-retired")).toBeNull();
	},
};

export const EnhancedOrderedNeverDeployedTurnedOff: Story = {
	name: "Enhanced: turning off an ordered subscription that was never deployed",
	render: () => (
		<ServiceEditor
			deployed={deployedEvents()}
			initial={withOrdersEvents(
				{
					subscribes: [
						{ name: "ledger", detail_type_prefix: "ledger.", ordered: true },
					],
				},
				enhancedBus,
			)}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByLabelText("Keep order for ledger"));
		await expect(canvas.queryByTestId("fifo-retired")).toBeNull();
		await expect(canvas.queryByText(/The ordered queue is kept/)).toBeNull();
		await expect(canvas.queryByTestId("event-flow-fifo")).toBeNull();
	},
};

const orderedLedger = (): YamlInfrastructureConfig =>
	withOrdersEvents(
		{
			subscribes: [
				{ name: "ledger", detail_type_prefix: "ledger.", ordered: true },
			],
		},
		enhancedBus,
	);

/**
 * Clicks the ledger's order switch off and checks that nothing is saved
 * until the fresh read the edit started is answered.
 */
async function turnLedgerOrderOff(canvasElement: HTMLElement) {
	const canvas = within(canvasElement);
	await userEvent.click(canvas.getByLabelText("Keep order for ledger"));
	await waitFor(() => expect(ledgerReads.waiting()).toBe(1));
	await expect(
		canvas.getByText(/Checking whether this service's ordered queue/),
	).toBeVisible();
	await expect(canvas.getByLabelText("Keep order for ledger")).toBeChecked();
	await expect(ledgerSaves).toHaveLength(0);
	return canvas;
}

/**
 * The last read said the ordered queue was never deployed, and a deploy from
 * the CLI has created it since. The edit waits for a fresh read, which says
 * the queue is deployed, and is saved once, with fifo_retired.
 */
export const EnhancedOrderedDeployedSinceLastRead: Story = {
	name: "Enhanced: an ordered queue deployed since the last read is kept",
	beforeEach: resetLedger,
	render: () => (
		<ServiceEditor
			deployed={deployedEvents()}
			reread={ledgerReads.read}
			onSave={recordLedgerSave}
			initial={orderedLedger()}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = await turnLedgerOrderOff(canvasElement);
		ledgerReads.resolve(deployedEvents({ fifoConsumers: ["orders"] }));
		await waitFor(() =>
			expect(canvas.getByTestId("fifo-retired")).toBeVisible(),
		);
		await expect(ledgerSaves).toHaveLength(1);
		await expect(ordersEvents(ledgerSaves[0])?.queue?.fifo_retired).toBe(true);
		await expect(
			canvas.queryByText(/Checking whether this service's ordered queue/),
		).toBeNull();
	},
};

/**
 * The fresh read confirms the queue was never deployed: one save, without
 * fifo_retired.
 */
export const EnhancedOrderedNeverDeployedConfirmed: Story = {
	name: "Enhanced: a fresh read confirms an ordered queue was never deployed",
	beforeEach: resetLedger,
	render: () => (
		<ServiceEditor
			deployed={deployedEvents()}
			reread={ledgerReads.read}
			onSave={recordLedgerSave}
			initial={orderedLedger()}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = await turnLedgerOrderOff(canvasElement);
		ledgerReads.resolve(deployedEvents());
		await waitFor(() =>
			expect(canvas.getByLabelText("Keep order for ledger")).not.toBeChecked(),
		);
		await expect(ledgerSaves).toHaveLength(1);
		await expect(ordersEvents(ledgerSaves[0])?.queue).toBeUndefined();
		await expect(canvas.queryByTestId("fifo-retired")).toBeNull();
		await expect(canvas.queryByText(/The ordered queue is kept/)).toBeNull();
	},
};

/** A fresh read that fails is unknown: one save, keeping the queue. */
export const EnhancedOrderedRereadFailed: Story = {
	name: "Enhanced: a failed fresh read keeps the ordered queue",
	beforeEach: resetLedger,
	render: () => (
		<ServiceEditor
			deployed={deployedEvents()}
			reread={ledgerReads.read}
			onSave={recordLedgerSave}
			initial={orderedLedger()}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = await turnLedgerOrderOff(canvasElement);
		ledgerReads.reject(new Error("the state could not be read"));
		await waitFor(() =>
			expect(canvas.getByTestId("fifo-retired")).toBeVisible(),
		);
		await expect(ledgerSaves).toHaveLength(1);
		await expect(ordersEvents(ledgerSaves[0])?.queue?.fifo_retired).toBe(true);
	},
};

/**
 * A service is added while the read is pending. The decision lands on the
 * config as it is then: the new service stays, in the editor and in the
 * saved config, and the ledger's order is off.
 */
export const EnhancedOrderedServiceAddedWhileReading: Story = {
	name: "Enhanced: a service added while the read is pending is kept",
	beforeEach: resetLedger,
	render: () => (
		<ServiceEditor
			deployed={deployedEvents({ fifoConsumers: ["orders"] })}
			reread={ledgerReads.read}
			onSave={recordLedgerSave}
			serviceAdder
			initial={orderedLedger()}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = await turnLedgerOrderOff(canvasElement);
		await userEvent.click(canvas.getByRole("button", { name: "Add service" }));
		await waitFor(() =>
			expect(canvas.getByTestId("service-names")).toHaveTextContent(
				"orders,worker,billing",
			),
		);
		await expect(ledgerSaves).toHaveLength(1);
		ledgerReads.resolve(deployedEvents());
		await waitFor(() =>
			expect(canvas.getByLabelText("Keep order for ledger")).not.toBeChecked(),
		);
		await expect(canvas.getByTestId("service-names")).toHaveTextContent(
			"orders,worker,billing",
		);
		await expect(ledgerSaves).toHaveLength(2);
		const last = ledgerSaves[1];
		await expect(last.services?.map((s) => s.name)).toEqual([
			"orders",
			"worker",
			"billing",
		]);
		await expect(ordersEvents(last)?.subscribes?.[0].ordered).toBeUndefined();
		await expect(ordersEvents(last)?.queue).toBeUndefined();
	},
};

export const EnhancedMixedBusFlow: Story = {
	name: "Enhanced: the flow draws default-bus rules apart from subscribers",
	render: () => (
		<ServiceEditor
			initial={withOrdersEvents(
				{
					subscribes: [
						{ detail_type: "payment.completed" },
						{ detail_type: "invoice.sent", bus: "default" },
					],
				},
				enhancedBus,
			)}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const flow = within(canvas.getByTestId("event-flow"));
		await expect(flow.getByText("Subscribers")).toBeVisible();
		await expect(flow.getByText("Rules")).toBeVisible();
		await expect(flow.getByText("default")).toBeVisible();
		await expect(flow.getAllByText("1 subscription")).toHaveLength(2);
	},
};

export const EnhancedWildcardMapped: Story = {
	name: "Enhanced: wildcards mapped to a prefix and a suffix",
	render: () => (
		<ServiceEditor
			initial={withOrdersEvents(
				{
					subscribes: [{ detail_type: "job.*" }, { detail_type: "*.created" }],
				},
				enhancedBus,
			)}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByText("Matches detail types starting with job."),
		).toBeVisible();
		await expect(
			canvas.getByText("Matches detail types ending with .created"),
		).toBeVisible();
	},
};

export const EnhancedWildcardError: Story = {
	name: "Enhanced: a wildcard the bus cannot match",
	render: () => (
		<ServiceEditor
			initial={withOrdersEvents(
				{ subscribes: [{ detail_type: "job.*.done" }] },
				enhancedBus,
			)}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByText(/uses \* in a way the enhanced bus cannot match/),
		).toBeVisible();
		const envSection = within(canvas.getByTestId("subscribes-section-project"));
		await userEvent.type(
			envSection.getByLabelText("Event type for project bus"),
			"a*b",
		);
		await expect(
			envSection.getByRole("button", {
				name: "Add subscription to project bus",
			}),
		).toBeDisabled();
	},
};

export const OrderedOnClassic: Story = {
	name: "Ordered on a Standard bus: the switch stays to turn it off",
	render: () => (
		<ServiceEditor
			initial={withOrdersEvents(
				{
					subscribes: [
						{ detail_type: "payment.completed" },
						{ name: "ledger", detail_type_prefix: "ledger.", ordered: true },
					],
				},
				{ event_bus: { enabled: true } },
			)}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.queryByLabelText("Keep order for payment-completed"),
		).toBeNull();
		await expect(canvas.getByLabelText("Keep order for ledger")).toBeChecked();
		await expect(
			canvas.getByText(/Keep order needs the Enhanced project bus/),
		).toBeVisible();
	},
};

export const FifoRetired: Story = {
	name: "Retired ordered queue",
	render: () => (
		<ServiceEditor
			initial={withOrdersEvents(
				{
					subscribes: [{ detail_type: "payment.completed" }],
					queue: { fifo_retired: true, visibility_timeout: 60 },
				},
				enhancedBus,
			)}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const retired = within(canvas.getByTestId("fifo-retired"));
		await expect(retired.getByText("Ordered queue (retired)")).toBeVisible();
		await expect(
			retired.getByText("demo-events-orders-dev.fifo"),
		).toBeVisible();
		await userEvent.click(
			retired.getByRole("button", { name: "Delete the ordered queue" }),
		);
		const dialog = within(canvasElement.ownerDocument.body);
		// The dialog fades in; wait until it is shown.
		await waitFor(() =>
			expect(
				dialog.getByText(/Deleting it also deletes any events still in it/),
			).toBeVisible(),
		);
		await expect(
			dialog.queryByText(/its standard queue is deleted as well/),
		).toBeNull();
	},
};

export const FifoRetiredNoSubscriptions: Story = {
	name: "Retired ordered queue, no subscriptions left",
	render: () => (
		<ServiceEditor
			initial={withOrdersEvents({ queue: { fifo_retired: true } }, enhancedBus)}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByText(/Subscribes to nothing\. Its queues stay/),
		).toBeVisible();
		await userEvent.click(
			canvas.getByRole("button", { name: "Delete the ordered queue" }),
		);
		const dialog = within(canvasElement.ownerDocument.body);
		await waitFor(() =>
			expect(
				dialog.getByText(/its standard queue is deleted as well/),
			).toBeVisible(),
		);
		await userEvent.click(
			dialog.getByRole("button", { name: "Delete the ordered queue" }),
		);
		await expect(
			await canvas.findByText("Subscribes to nothing, so it has no queue."),
		).toBeVisible();
	},
};

/**
 * fifo_retired on a consumer whose ordered queue was never deployed: written
 * by hand, or by an edit made while the deployed state could not be read (the
 * editor then keeps the queue to be safe). Generate warns
 * event_fifo_retired_unused; the editor shows the same retired card.
 */
export const FifoRetiredUnused: Story = {
	name: "Retired ordered queue on a never-deployed environment",
	render: () => (
		<ServiceEditor
			initial={withOrdersEvents(
				{
					subscribes: [{ name: "ledger", detail_type_prefix: "ledger." }],
					queue: { fifo_retired: true },
				},
				enhancedBus,
			)}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByTestId("fifo-retired")).toBeVisible();
		await expect(
			canvas.getByLabelText("Keep order for ledger"),
		).not.toBeChecked();
	},
};

export const EnhancedSummaryRetiredQueue: Story = {
	name: "Enhanced summary with a retired ordered queue",
	render: () => (
		<ServiceEditor
			deployed={deployedEvents({ fifoConsumers: ["orders"] })}
			initial={withOrdersEvents(
				{
					subscribes: [{ detail_type: "payment.completed" }],
					queue: { fifo_retired: true },
				},
				enhancedBus,
			)}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("Bus")).toBeVisible();
		await expect(canvas.getByText("Enhanced")).toBeVisible();
		await expect(canvas.getByText("EVENT_BUS_ARN")).toBeVisible();
		await expect(canvas.getByText("EVENT_FIFO_QUEUE_URL")).toBeVisible();
		await expect(canvas.getByText("EVENT_FIFO_DLQ_URL")).toBeVisible();
		await expect(
			canvas.getByText(
				/The ordered queue is kept, with any events still in it/,
			),
		).toBeVisible();
		await expect(canvas.getByTestId("fifo-retired")).toBeVisible();
	},
};

export const EnhancedDeployedOrderedTurnedOff: Story = {
	name: "Enhanced: turning off an ordered subscription that is deployed",
	render: () => (
		<ServiceEditor
			deployed={deployedEvents({ fifoConsumers: ["orders"] })}
			initial={withOrdersEvents(
				{
					subscribes: [
						{ name: "ledger", detail_type_prefix: "ledger.", ordered: true },
					],
				},
				enhancedBus,
			)}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByLabelText("Keep order for ledger"));
		await expect(canvas.getByTestId("fifo-retired")).toBeVisible();
	},
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
