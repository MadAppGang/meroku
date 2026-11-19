import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React from "react";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import type { YamlInfrastructureConfig } from "../../../../types/yamlConfig";
import type {
	DeployedEvents,
	ShareTargetInfo,
} from "../../../../utils/workloadEvents";
import {
	EventBusSettings,
	type LoadShareTargets,
} from "../../../EventBusSettings";
import {
	baseConfig,
	ConfigHarness,
	configWith,
	deployedEvents,
	deployedUnknown,
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

/** Synthetic sibling environments, as GET /api/event-bus/share-targets lists them. */
const siblingTargets: ShareTargetInfo[] = [
	{
		env: "staging",
		status: "ok",
		accountId: "111111111111",
		region: "us-east-1",
		files: ["staging.yaml"],
	},
	{ env: "qa", status: "ambiguous", files: ["qa.yaml", "qa-old.yaml"] },
	{ env: "perf", status: "no_account", files: ["perf.yaml"] },
	{ env: "sandbox", status: "own_account", files: ["sandbox.yaml"] },
];

const loadSiblings: LoadShareTargets = async () => siblingTargets;
const loadNone: LoadShareTargets = async () => [];
/** The share-targets request never answers while the story runs. */
const loadPending: LoadShareTargets = (_env, signal) =>
	new Promise((_, reject) => {
		signal.addEventListener("abort", () =>
			reject(new DOMException("Aborted", "AbortError")),
		);
	});
/** Fails until a story's play turns `failing` off, then lists the siblings. */
const failingTargets = { failing: true };
const loadFailing: LoadShareTargets = async () => {
	if (failingTargets.failing) throw new Error("the server did not answer");
	return siblingTargets;
};

function Editor({
	initial,
	load = loadSiblings,
	deployed,
	onWrite,
}: {
	initial: YamlInfrastructureConfig;
	load?: LoadShareTargets;
	/** GET /api/events/deployed's answer; absent means not known yet. */
	deployed?: DeployedEvents;
	/** Sees every write the panel makes. */
	onWrite?: (updates: Partial<YamlInfrastructureConfig>) => void;
}) {
	return (
		<ConfigHarness initial={initial}>
			{(config, onChange) => (
				<EventBusSettings
					config={config}
					onConfigChange={(updates) => {
						onWrite?.(updates);
						onChange(updates);
					}}
					loadShareTargets={load}
					deployed={deployed}
				/>
			)}
		</ConfigHarness>
	);
}

/** The event task's rule on the default bus, so an Enhanced bus can be used. */
const tasksOnDefaultBus: YamlInfrastructureConfig["event_processor_tasks"] = [
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
];

function enhancedConfig(
	overrides: Partial<YamlInfrastructureConfig> = {},
): YamlInfrastructureConfig {
	return configWith({
		event_bus: { enabled: true, type: "enhanced" },
		event_processor_tasks: tasksOnDefaultBus,
		...overrides,
	});
}

export const Off: Story = {
	render: () => <Editor initial={baseConfig} />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.queryByText("Kind of bus")).toBeNull();
	},
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

export const Standard: Story = {
	name: "Standard bus",
	render: () => (
		<Editor initial={configWith({ event_bus: { enabled: true } })} />
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByLabelText("Standard")).toBeChecked();
		await expect(canvas.getByLabelText("Enhanced")).toBeEnabled();
		await expect(canvas.queryByTestId("share-card")).toBeNull();
	},
};

export const Enhanced: Story = {
	name: "Enhanced bus",
	render: () => <Editor initial={enhancedConfig()} />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByLabelText("Enhanced")).toBeChecked();
		await expect(canvas.getByTestId("share-card")).toBeVisible();
		await expect(
			canvas.getByText("Not shared with any other account."),
		).toBeVisible();
	},
};

export const EnhancedWithShares: Story = {
	name: "Enhanced, shared with all four kinds and one invalid",
	render: () => (
		<Editor
			initial={enhancedConfig({
				event_bus: {
					enabled: true,
					type: "enhanced",
					share: [
						{ organization: true, access: "publish" },
						{ ou: "ou-ab12-cd34ef56", access: "subscribe" },
						{ account: "222222222222", access: "full" },
						{ env: "staging", access: "publish" },
						{ account: "12345", access: "publish" },
					],
				},
			})}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await waitFor(() =>
			expect(
				within(canvas.getByTestId("share-row-4")).getByText(
					"An AWS account id is 12 digits.",
				),
			).toBeVisible(),
		);
		await expect(
			within(canvas.getByTestId("share-row-1")).getByText(
				"They can read every event on this bus.",
			),
		).toBeVisible();
	},
};

export const RegionUnavailable: Story = {
	name: "Enhanced not offered in the region",
	render: () => (
		<Editor
			initial={configWith({
				region: "sa-east-1",
				event_bus: { enabled: true },
			})}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByLabelText("Enhanced")).toBeDisabled();
		await expect(
			canvas.getByText("Not available in sa-east-1 yet."),
		).toBeVisible();
	},
};

export const EnhancedInUnavailableRegion: Story = {
	name: "Enhanced, region does not offer it",
	render: () => <Editor initial={enhancedConfig({ region: "sa-east-1" })} />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByText(/The Enhanced bus is not available in sa-east-1/),
		).toBeVisible();
	},
};

export const EnhancedWithProjectBusEventTask: Story = {
	name: "Enhanced, an event task still on the project bus",
	render: () => (
		<Editor
			initial={configWith({ event_bus: { enabled: true, type: "enhanced" } })}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByText(/An Enhanced bus cannot start event tasks/),
		).toBeVisible();
	},
};

const orderedBackend = {
	events: {
		subscribes: [
			{ name: "ledger", detail_type_prefix: "ledger.", ordered: true },
		],
	},
} as YamlInfrastructureConfig["workload"];

export const LeavingEnhancedAlert: Story = {
	name: "Leaving Enhanced: the confirmation",
	render: () => (
		<Editor initial={enhancedConfig({ workload: orderedBackend })} />
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByLabelText("Standard"));
		const dialog = within(canvasElement.ownerDocument.body);
		// The dialog fades in; wait until it is shown.
		await waitFor(() =>
			expect(
				dialog.getByText(/Changing the kind replaces the bus/),
			).toBeVisible(),
		);
		await expect(
			dialog.getByText(/These services lose ordered delivery/),
		).toBeVisible();
		await expect(
			dialog.getByText(/Their ordered queues are kept until you delete them/),
		).toBeVisible();
		await expect(
			dialog.queryByText(/lose their subscribers for good/),
		).toBeNull();
	},
};

const neverDeployedWrites = fn();

export const LeavingEnhancedNeverDeployedOrdered: Story = {
	name: "Leaving Enhanced: an ordered queue that was never deployed",
	render: () => (
		<Editor
			initial={enhancedConfig({ workload: orderedBackend })}
			deployed={deployedEvents()}
			onWrite={neverDeployedWrites}
		/>
	),
	play: async ({ canvasElement }) => {
		neverDeployedWrites.mockClear();
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByLabelText("Standard"));
		const dialog = within(canvasElement.ownerDocument.body);
		await waitFor(() =>
			expect(
				dialog.getByText(/These services lose ordered delivery: backend./),
			).toBeVisible(),
		);
		await expect(dialog.queryByText(/ordered queues are kept/)).toBeNull();
		await userEvent.click(
			dialog.getByRole("button", { name: "Use a Standard bus" }),
		);
		await waitFor(() => expect(neverDeployedWrites).toHaveBeenCalled());
		const written = neverDeployedWrites.mock.calls[0][0];
		await expect(written.workload?.events?.queue?.fifo_retired).toBeUndefined();
	},
};

const deployedOrderedWrites = fn();

export const LeavingEnhancedDeployedOrdered: Story = {
	name: "Leaving Enhanced: a deployed ordered queue is kept",
	render: () => (
		<Editor
			initial={enhancedConfig({ workload: orderedBackend })}
			deployed={deployedEvents({ fifoConsumers: ["backend"] })}
			onWrite={deployedOrderedWrites}
		/>
	),
	play: async ({ canvasElement }) => {
		deployedOrderedWrites.mockClear();
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByLabelText("Standard"));
		const dialog = within(canvasElement.ownerDocument.body);
		await waitFor(() =>
			expect(
				dialog.getByText(/Their ordered queues are kept until you delete them/),
			).toBeVisible(),
		);
		await userEvent.click(
			dialog.getByRole("button", { name: "Use a Standard bus" }),
		);
		await waitFor(() => expect(deployedOrderedWrites).toHaveBeenCalled());
		const written = deployedOrderedWrites.mock.calls[0][0];
		await expect(written.workload?.events?.queue?.fifo_retired).toBe(true);
	},
};

const unknownStateWrites = fn();

export const LeavingEnhancedStateUnknown: Story = {
	name: "Leaving Enhanced: deployed state unreadable, the queue is kept",
	render: () => (
		<Editor
			initial={enhancedConfig({ workload: orderedBackend })}
			deployed={deployedUnknown}
			onWrite={unknownStateWrites}
		/>
	),
	play: async ({ canvasElement }) => {
		unknownStateWrites.mockClear();
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByLabelText("Standard"));
		const dialog = within(canvasElement.ownerDocument.body);
		await waitFor(() =>
			expect(
				dialog.getByText(/Their ordered queues are kept until you delete them/),
			).toBeVisible(),
		);
		await userEvent.click(
			dialog.getByRole("button", { name: "Use a Standard bus" }),
		);
		await waitFor(() => expect(unknownStateWrites).toHaveBeenCalled());
		const written = unknownStateWrites.mock.calls[0][0];
		await expect(written.workload?.events?.queue?.fifo_retired).toBe(true);
	},
};

export const LeavingEnhancedAlertWithShare: Story = {
	name: "Leaving Enhanced: the confirmation, with a subscribe share",
	render: () => (
		<Editor
			initial={enhancedConfig({
				event_bus: {
					enabled: true,
					type: "enhanced",
					share: [{ account: "222222222222", access: "subscribe" }],
				},
			})}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByLabelText("Use the project bus"));
		const dialog = within(canvasElement.ownerDocument.body);
		await waitFor(() =>
			expect(dialog.getByText("Turn the Enhanced bus off?")).toBeVisible(),
		);
		await expect(
			dialog.getByText(
				"Accounts you shared the bus with lose their subscribers for good.",
			),
		).toBeVisible();
	},
};

export const ShareRemovalNotice: Story = {
	name: "Removing a deployed share: the revocation notice",
	render: () => (
		<Editor
			initial={enhancedConfig({
				event_bus: {
					enabled: true,
					type: "enhanced",
					share: [
						{ account: "222222222222", access: "full" },
						{ organization: true, access: "publish" },
					],
				},
			})}
			deployed={deployedEvents({ sharedPrincipals: ["account:222222222222"] })}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.queryByText(/No longer shared/)).toBeNull();
		await userEvent.click(
			canvas.getByRole("button", { name: "Remove share 1" }),
		);
		await expect(
			canvas.getByText("No longer shared: account 222222222222."),
		).toBeVisible();
		await expect(
			canvas.getByText(/every subscriber it created on the bus is revoked/),
		).toBeVisible();
	},
};

export const ShareRemovedBeforeOpening: Story = {
	name: "Deployed shares already gone from the YAML",
	render: () => (
		<Editor
			initial={enhancedConfig({
				event_bus: {
					enabled: true,
					type: "enhanced",
					share: [{ env: "staging", access: "publish" }],
				},
			})}
			deployed={deployedEvents({
				sharedPrincipals: [
					"account:111111111111",
					"ou:ou-ab12-cd34ef56",
					"organization",
				],
			})}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// staging is account 111111111111, still named, so it is not listed.
		await expect(
			await canvas.findByText(
				"No longer shared: the whole organization, organizational unit ou-ab12-cd34ef56.",
			),
		).toBeVisible();
	},
};

export const ShareStateUnknown: Story = {
	name: "Deployed state unreadable: no revocation notice",
	render: () => (
		<Editor
			initial={enhancedConfig({
				event_bus: {
					enabled: true,
					type: "enhanced",
					share: [{ account: "222222222222", access: "publish" }],
				},
			})}
			deployed={deployedUnknown}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.queryByText(/No longer shared/)).toBeNull();
		await expect(canvas.queryByText(/is revoked/)).toBeNull();
	},
};

const pendingTargetsWrites = fn();

export const ShareEditWhileTargetsPending: Story = {
	name: "Editing an account while the environments are still listed",
	render: () => (
		<Editor
			initial={enhancedConfig({
				event_bus: {
					enabled: true,
					type: "enhanced",
					share: [
						{ env: "staging", access: "publish" },
						{ account: "222222222222", access: "publish" },
					],
				},
			})}
			load={loadPending}
			onWrite={pendingTargetsWrites}
		/>
	),
	play: async ({ canvasElement }) => {
		pendingTargetsWrites.mockClear();
		const canvas = within(canvasElement);
		const account = canvas.getByLabelText("Account id for share 2");
		await userEvent.clear(account);
		await userEvent.type(account, "111111111111");
		await userEvent.tab();
		await expect(
			canvas.getByText(
				/Not saved yet: checking whether this is the same account as environment staging/,
			),
		).toBeVisible();
		await expect(pendingTargetsWrites).not.toHaveBeenCalled();
		// Removing an entry needs no comparison and still works.
		await userEvent.click(
			canvas.getByRole("button", { name: "Remove share 1" }),
		);
		await waitFor(() => expect(pendingTargetsWrites).toHaveBeenCalled());
	},
};

const failedTargetsWrites = fn();

export const ShareEditWhileTargetsFailed: Story = {
	name: "Editing an account when the environments could not be listed",
	render: () => {
		failingTargets.failing = true;
		return (
			<Editor
				initial={enhancedConfig({
					event_bus: {
						enabled: true,
						type: "enhanced",
						share: [
							{ env: "staging", access: "publish" },
							{ account: "222222222222", access: "publish" },
						],
					},
				})}
				load={loadFailing}
				onWrite={failedTargetsWrites}
			/>
		);
	},
	play: async ({ canvasElement }) => {
		failedTargetsWrites.mockClear();
		const canvas = within(canvasElement);
		const account = canvas.getByLabelText("Account id for share 2");
		await userEvent.clear(account);
		await userEvent.type(account, "111111111111");
		await userEvent.tab();
		await expect(
			await canvas.findByText(
				/Not saved: the other environments could not be listed, so this cannot be compared with environment staging/,
			),
		).toBeVisible();
		await expect(failedTargetsWrites).not.toHaveBeenCalled();
		// Once listed, the held-back draft is checked: it is staging's account.
		failingTargets.failing = false;
		await userEvent.click(canvas.getByRole("button", { name: "Try again" }));
		await expect(
			await canvas.findByText(
				/Shared again: entry 1 already shares with environment staging/,
			),
		).toBeVisible();
		await expect(failedTargetsWrites).not.toHaveBeenCalled();
	},
};

const duplicateEditWrites = fn();

export const EditingShareIntoDuplicate: Story = {
	name: "Editing a share to name another entry's account",
	render: () => (
		<Editor
			initial={enhancedConfig({
				event_bus: {
					enabled: true,
					type: "enhanced",
					share: [
						{ account: "222222222222", access: "publish" },
						{ account: "333333333333", access: "full" },
					],
				},
			})}
			onWrite={duplicateEditWrites}
		/>
	),
	play: async ({ canvasElement }) => {
		duplicateEditWrites.mockClear();
		const canvas = within(canvasElement);
		const first = canvas.getByLabelText("Account id for share 1");
		await userEvent.clear(first);
		await userEvent.type(first, "333333333333");
		await userEvent.tab();
		await expect(
			canvas.getByText(
				/Shared again: entry 2 already shares with account 333333333333/,
			),
		).toBeVisible();
		await expect(duplicateEditWrites).not.toHaveBeenCalled();
	},
};

export const AmbiguousEnvTarget: Story = {
	name: "Share with an environment two files declare",
	render: () => (
		<Editor
			initial={enhancedConfig({
				event_bus: {
					enabled: true,
					type: "enhanced",
					share: [{ env: "qa", access: "publish" }],
				},
			})}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			await canvas.findByText(
				"qa.yaml and qa-old.yaml both declare environment qa of project demo; keep one.",
			),
		).toBeVisible();
	},
};

export const NoAccountEnvTarget: Story = {
	name: "Share with an environment that has no account_id",
	render: () => (
		<Editor
			initial={enhancedConfig({
				event_bus: {
					enabled: true,
					type: "enhanced",
					share: [{ env: "perf", access: "publish" }],
				},
			})}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			await canvas.findByText(
				"perf.yaml has no account_id, so its account is unknown.",
			),
		).toBeVisible();
	},
};

export const NoSiblingEnvironments: Story = {
	name: "Share card with no sibling environments",
	render: () => <Editor initial={enhancedConfig()} load={loadNone} />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			await canvas.findByText(/No other environment of project demo was found/),
		).toBeVisible();
	},
};

export const BusOffBlockedByExplicitRefs: Story = {
	name: "Turning the bus off, blocked by explicit project-bus entries",
	render: () => (
		<Editor
			initial={configWith({
				event_bus: { enabled: true },
				workload: {
					events: {
						subscribes: [{ detail_type: "job.completed", bus: "project" }],
						produces: [{ detail_types: ["job.started"], bus: "project" }],
					},
				} as YamlInfrastructureConfig["workload"],
				event_processor_tasks: [
					{
						name: "ingest",
						rules: [
							{
								name: "order-created",
								sources: ["demo.orders"],
								detail_types: ["OrderCreated"],
								bus: "project",
							},
						],
					},
				],
			})}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const toggle = canvas.getByLabelText("Use the project bus");
		await expect(toggle).toBeDisabled();
		await expect(
			canvas.getByText(
				/backend subscribes job\.completed, backend produces job\.started, ingest rule order-created/,
			),
		).toBeVisible();
		await userEvent.click(
			canvas.getByRole("button", { name: "Let them follow the switch" }),
		);
		await expect(canvas.queryByTestId("bus-off-blocked")).toBeNull();
		await expect(toggle).toBeEnabled();
	},
};
