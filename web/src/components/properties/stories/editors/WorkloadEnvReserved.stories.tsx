import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React from "react";
import { expect, userEvent, within } from "storybook/test";
import type { YamlInfrastructureConfig } from "../../../../types/yamlConfig";
import { EventTaskEnvVars } from "../../../EventTaskEnvVars";
import { ScheduledTaskEnvironmentVariables } from "../../../ScheduledTaskEnvironmentVariables";
import { ServiceEnvironmentVariables } from "../../../ServiceEnvironmentVariables";
import {
	baseConfig,
	ConfigHarness,
	configWith,
	editorDecorator,
	editorParameters,
	storyNode,
} from "./editorStoryKit";

/**
 * A workload's YAML variable named like one meroku sets for a KMS key the
 * workload connects to is refused by generate; each workload's variable editor
 * says so beside the variable.
 */
const meta = {
	title: "Editors/Workload Env Reserved Names",
	tags: ["!autodocs"],
	parameters: {
		...editorParameters,
		a11y: {
			config: {
				// These editors still use the older gray-500-on-gray-800 text for
				// their descriptions, below 4.5:1; the warnings tested here use the
				// danger token, which passes.
				rules: [{ id: "color-contrast", enabled: false }],
			},
		},
	},
	decorators: [editorDecorator],
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

const withAuthKey = (
	overrides: Partial<YamlInfrastructureConfig>,
): YamlInfrastructureConfig =>
	configWith({ kms_keys: [{ name: "auth" }], ...overrides });

export const Service: Story = {
	name: "Service variables",
	render: () => (
		<ConfigHarness
			initial={withAuthKey({
				services: (baseConfig.services || []).map((s) =>
					s.name === "orders"
						? {
								...s,
								env_vars: { KMS_AUTH_PUBLIC_KEY: "fake-pem-do-not-use" },
								kms_keys: [{ name: "auth", access: "verify" }],
							}
						: s,
				),
			})}
		>
			{(config, onChange) => (
				<ServiceEnvironmentVariables
					config={config}
					node={storyNode("service-orders", "service", "orders")}
					onConfigChange={onChange}
				/>
			)}
		</ConfigHarness>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByText(/KMS_AUTH_PUBLIC_KEY is reserved: meroku sets it/),
		).toBeVisible();

		const name = canvas.getByLabelText("Variable Name");
		await userEvent.type(name, "KMS_AUTH_KEY_ARN");
		await userEvent.type(canvas.getByLabelText("Value"), "x");
		await expect(name).toHaveAttribute("aria-invalid", "true");
		await expect(
			canvas.getByText(/KMS_AUTH_KEY_ARN is reserved: meroku sets it/),
		).toBeVisible();
		await expect(
			canvas.getByRole("button", { name: "Add Variable" }),
		).toBeDisabled();

		await userEvent.clear(name);
		await userEvent.type(name, "LOG_LEVEL");
		await expect(name).toHaveAttribute("aria-invalid", "false");
		await expect(
			canvas.getByRole("button", { name: "Add Variable" }),
		).toBeEnabled();
	},
};

export const ScheduledTask: Story = {
	name: "Scheduled task variables",
	render: () => (
		<ConfigHarness
			initial={withAuthKey({
				scheduled_tasks: (baseConfig.scheduled_tasks || []).map((t) =>
					t.name === "nightly"
						? {
								...t,
								environment_variables: {
									KMS_AUTH_KEY_ARN: "fake-arn-do-not-use",
								},
								kms_keys: [{ name: "auth", access: "sign" }],
							}
						: t,
				),
			})}
		>
			{(config, onChange) => (
				<ScheduledTaskEnvironmentVariables
					config={config}
					node={storyNode("scheduled-nightly", "scheduled-task", "nightly")}
					onConfigChange={onChange}
				/>
			)}
		</ConfigHarness>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByText(/KMS_AUTH_KEY_ARN is reserved: meroku sets it/),
		).toBeVisible();
		const name = canvas.getByLabelText("Variable Name");
		await userEvent.type(name, "KMS_AUTH_PUBLIC_KEY");
		await expect(name).toHaveAttribute("aria-invalid", "true");
	},
};

export const EventTask: Story = {
	name: "Event task variables",
	render: () => (
		<EventTaskEnvVars
			config={withAuthKey({
				event_processor_tasks: (baseConfig.event_processor_tasks || []).map(
					(t) =>
						t.name === "ingest"
							? {
									...t,
									environment_variables: {
										KMS_AUTH_KEY_ARN: "fake-arn-do-not-use",
										LOG_LEVEL: "info",
									},
									kms_keys: [{ name: "auth", access: "both" }],
								}
							: t,
				),
			})}
			node={storyNode("event-ingest", "event-task", "ingest")}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("LOG_LEVEL")).toBeVisible();
		await expect(
			canvas.getAllByText(/KMS_AUTH_KEY_ARN is reserved: meroku sets it/),
		).toHaveLength(1);
	},
};
