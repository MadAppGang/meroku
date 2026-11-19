import type { Decorator } from "@storybook/react-vite";
import { type ReactNode, useRef, useState } from "react";
import type { ComponentNode } from "../../../../types";
import type { YamlInfrastructureConfig } from "../../../../types/yamlConfig";
import { mergeConfigUpdates } from "../../../../utils/configMerge";
import {
	applyConfigUpdate,
	type ConfigUpdate,
	type ConfigUpdateResult,
	type DeployedEvents,
} from "../../../../utils/workloadEvents";

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

/**
 * An answer of GET /api/events/deployed, as the app passes it to the event
 * editors. Synthetic: placeholder account ids only.
 */
export function deployedEvents(
	answer: Partial<DeployedEvents> = {},
): DeployedEvents {
	return {
		known: true,
		fifoConsumers: [],
		sharedPrincipals: [],
		subscribePrincipals: [],
		...answer,
	};
}

/** The answer when the deployed state could not be read. */
export const deployedUnknown: DeployedEvents = {
	known: false,
	fifoConsumers: [],
	sharedPrincipals: [],
	subscribePrincipals: [],
};

/** A canvas node, as the Sidebar hands one to a workload panel. */
export function storyNode(
	id: string,
	type: ComponentNode["type"],
	name: string,
): ComponentNode {
	return { id, type, name, status: "running" };
}

type OnConfigChange = (updates: Partial<YamlInfrastructureConfig>) => void;

type OnConfigUpdate = (
	file: string,
	update: ConfigUpdate,
) => ConfigUpdateResult;

/**
 * Holds a config, loaded from the environment file `initial.env`, and merges
 * every update the way App does, so a story is interactive: adding, editing
 * and removing work as in the app. `onUpdate` applies an updater to the
 * config held when it runs, for that file only, as App's handleConfigUpdate
 * does. `onSave` sees every config that changed, once, as App would write it.
 */
export function ConfigHarness({
	initial,
	children,
	onSave,
}: {
	initial: YamlInfrastructureConfig;
	children: (
		config: YamlInfrastructureConfig,
		onChange: OnConfigChange,
		onUpdate: OnConfigUpdate,
	) => ReactNode;
	onSave?: (config: YamlInfrastructureConfig) => void;
}) {
	const [config, setConfig] = useState(initial);
	const held = useRef(initial);
	const commit = (next: YamlInfrastructureConfig) => {
		if (next === held.current) return;
		held.current = next;
		setConfig(next);
		onSave?.(next);
	};
	return (
		<>
			{children(
				config,
				(updates) => commit(mergeConfigUpdates(held.current, updates)),
				(file, update) => {
					const { result, config: next } = applyConfigUpdate(
						{ file: initial.env, config: held.current },
						file,
						update,
					);
					if (result === "applied" && next) commit(next);
					return result;
				},
			)}
		</>
	);
}

/**
 * Reads whose answers a story hands out, so a play function decides when a
 * read finishes instead of waiting a fixed time. Call `reset` in the story's
 * beforeEach.
 */
export function controlledReads<T>() {
	let pending: { resolve: (value: T) => void; reject: (err: Error) => void }[] =
		[];
	return {
		read: () =>
			new Promise<T>((resolve, reject) => {
				pending.push({ resolve, reject });
			}),
		/** How many reads are still waiting for an answer. */
		waiting: () => pending.length,
		/** Answers the oldest waiting read. */
		resolve: (value: T) => pending.shift()?.resolve(value),
		/** Fails the oldest waiting read. */
		reject: (err: Error) => pending.shift()?.reject(err),
		reset: () => {
			pending = [];
		},
	};
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
