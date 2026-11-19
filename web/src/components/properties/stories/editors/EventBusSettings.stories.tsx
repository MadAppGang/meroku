import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React from "react";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import type { YamlInfrastructureConfig } from "../../../../types/yamlConfig";
import type { ShareTargetInfo } from "../../../../utils/workloadEvents";
import {
	EventBusSettings,
	type LoadShareTargets,
} from "../../../EventBusSettings";
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

function Editor({
	initial,
	load = loadSiblings,
	loaded,
	onWrite,
}: {
	initial: YamlInfrastructureConfig;
	load?: LoadShareTargets;
	/** The YAML as loaded from disk; absent means unknown. */
	loaded?: YamlInfrastructureConfig;
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
					loadedConfig={loaded}
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
	name: "Leaving Enhanced: an ordered queue added since the YAML was loaded",
	render: () => (
		<Editor
			initial={enhancedConfig({ workload: orderedBackend })}
			loaded={enhancedConfig()}
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
	name: "Removing a subscribe share: the revocation notice",
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
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.queryByText(/No longer shared for subscribing/),
		).toBeNull();
		await userEvent.click(
			canvas.getByRole("button", { name: "Remove share 1" }),
		);
		await expect(
			canvas.getByText(
				"No longer shared for subscribing: account 222222222222.",
			),
		).toBeVisible();
		await expect(
			canvas.getByText(
				/every subscriber this account created on the bus is revoked/,
			),
		).toBeVisible();
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
