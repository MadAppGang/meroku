import type { Decorator } from "@storybook/react-vite";
import { type ReactNode, useState } from "react";
import type { ComponentNode } from "../../../../types";
import type { YamlInfrastructureConfig } from "../../../../types/yamlConfig";
import { mergeConfigUpdates } from "../../../../utils/configMerge";

/**
 * Synthetic environment the capability editors render against: placeholder
 * account, no real names. Each story layers its state on top of it.
 */
export const baseConfig: YamlInfrastructureConfig = {
	project: "demo",
	env: "dev",
	is_prod: false,
	region: "us-east-1",
	account_id: "000000000000",
	state_bucket: "demo-terraform-state-000000000000",
	state_file: "state.tfstate",
	domain: {
		enabled: true,
		domain_name: "example.com",
		api_domain_prefix: "api",
	},
	services: [
		{ name: "orders" },
		{ name: "worker", network_access: "internal" },
	],
	scheduled_tasks: [{ name: "nightly", schedule: "rate(1 day)" }],
	event_processor_tasks: [
		{
			name: "ingest",
			rules: [
				{
					name: "order-created",
					sources: ["demo.orders"],
					detail_types: ["OrderCreated"],
				},
			],
		},
	],
	amplify_apps: [
		{
			name: "web",
			github_repository: "example/web",
			branches: [{ name: "main" }],
			subdomain_prefix: "app",
		},
	],
};

/** `baseConfig` with `overrides` applied on top. */
export function configWith(
	overrides: Partial<YamlInfrastructureConfig>,
): YamlInfrastructureConfig {
	return { ...baseConfig, ...overrides };
}

/** A canvas node, as the Sidebar hands one to a workload panel. */
export function storyNode(
	id: string,
	type: ComponentNode["type"],
	name: string,
): ComponentNode {
	return { id, type, name, status: "running" };
}

type OnConfigChange = (updates: Partial<YamlInfrastructureConfig>) => void;

/**
 * Holds a config in state and merges every update the way App does, so a
 * story is interactive: adding, editing and removing work as in the app.
 */
export function ConfigHarness({
	initial,
	children,
}: {
	initial: YamlInfrastructureConfig;
	children: (
		config: YamlInfrastructureConfig,
		onChange: OnConfigChange,
	) => ReactNode;
}) {
	const [config, setConfig] = useState(initial);
	return (
		<>
			{children(config, (updates) =>
				setConfig((prev) => mergeConfigUpdates(prev, updates)),
			)}
		</>
	);
}

/** The shipped sidebar's surface: dark theme tokens, panel width. */
export const editorDecorator: Decorator = (Story) => (
	<div className="dark min-h-screen bg-background p-6 text-foreground">
		<div className="mx-auto max-w-3xl">
			<Story />
		</div>
	</div>
);

export const editorParameters = {
	backgrounds: { disable: true },
	controls: { disable: true },
	layout: "fullscreen",
} as const;
