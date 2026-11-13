// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React, { type ReactNode, useState } from "react";
import type { NeonLookupResponse, NeonSchemasResponse } from "../types/neon";
import type {
	YamlInfrastructureConfig,
	YamlNeonConfig,
} from "../types/yamlConfig";
import { mergeConfigUpdates } from "../utils/configMerge";
import { pruneUndefined } from "../utils/pruneUndefined";
import type { NeonLookupFn } from "./NeonNodeProperties";
import type { NeonSchemasFn } from "./ServiceDatabaseTab";

/**
 * Synthetic data for the Neon panel stories. Every id is a placeholder.
 */

export function neonStoryConfig(
	over: Partial<YamlInfrastructureConfig> = {},
): YamlInfrastructureConfig {
	return {
		project: "myapp",
		env: "dev",
		is_prod: false,
		region: "us-east-1",
		state_bucket: "myapp-terraform-state-placeholder",
		state_file: "state.tfstate",
		workload: {},
		services: [{ name: "reports", container_port: 8080 }],
		...over,
	};
}

export const createNeon: YamlNeonConfig = {
	mode: "create",
	project: {
		name: "myapp-dev",
		org_id: "org-fake-000000",
		region_id: "aws-us-east-1",
		pg_version: 17,
	},
	databases: [
		{ name: "neondb", create: false },
		{ name: "app", create: true },
	],
};

export const reuseNeon: YamlNeonConfig = {
	mode: "reuse",
	project_id: "fake-project-000000",
	pg_version: 17,
	databases: [
		{ name: "app", create: false },
		{ name: "analytics", create: true },
	],
};

export const loggedIn: NeonLookupResponse = {
	auth: { state: "ok", source: "neonctl", expires_in_seconds: 3600 },
	organizations: [
		{ id: "org-fake-000000", name: "My team" },
		{ id: "org-fake-000001", name: "Side projects" },
	],
	projects: [
		{
			id: "fake-project-000000",
			name: "shared-dev",
			region_id: "aws-us-east-1",
			pg_version: 17,
			org_id: "org-fake-000000",
		},
		{
			id: "fake-project-000001",
			name: "legacy",
			region_id: "aws-eu-central-1",
			pg_version: 14,
			org_id: "org-fake-000000",
		},
	],
	databases: [
		{ name: "neondb", owner: "neondb_owner" },
		{ name: "app", owner: "neondb_owner" },
		{ name: "billing", owner: "neondb_owner" },
	],
	roles: [{ name: "neondb_owner" }],
};

/** The lookup when the project already exists in Neon. */
export const loggedInDeployed: NeonLookupResponse = {
	...loggedIn,
	project: {
		id: "fake-project-000000",
		name: "myapp-dev",
		region_id: "aws-us-east-1",
		pg_version: 17,
	},
};

export const notLoggedIn: NeonLookupResponse = {
	auth: { state: "no_credentials" },
};

export function answer(response: NeonLookupResponse): NeonLookupFn {
	return () => Promise.resolve(response);
}

/** A lookup that never answers. */
export const neverAnswers: NeonLookupFn = () => new Promise(() => {});

export const lookupFails: NeonLookupFn = () =>
	Promise.reject(new Error("meroku is not running"));

export const schemasLoaded: NeonSchemasResponse = {
	schemas: ["analytics", "app", "public"],
	note: "",
};

export const schemasNotDeployed: NeonSchemasResponse = {
	schemas: [],
	note: "The database app is not in Neon yet; meroku creates it at the next deploy.",
};

export const schemasNoAuth: NeonSchemasResponse = {
	schemas: [],
	note: "Log in to Neon to see the schemas (neon login).",
};

export function answerSchemas(response: NeonSchemasResponse): NeonSchemasFn {
	return () => Promise.resolve(response);
}

/** A schema lookup that never answers. */
export const schemasNeverAnswer: NeonSchemasFn = () => new Promise(() => {});

/**
 * Holds the env config like App does: an update is merged in and undefined
 * keys are pruned, as the YAML save would.
 */
export function NeonConfigHarness({
	initial,
	children,
}: {
	initial: YamlInfrastructureConfig;
	children: (
		config: YamlInfrastructureConfig,
		onConfigChange: (updates: Partial<YamlInfrastructureConfig>) => void,
	) => ReactNode;
}) {
	const [config, setConfig] = useState(initial);
	return (
		<div className="max-w-xl">
			{children(config, (updates) =>
				setConfig(
					(prev) =>
						pruneUndefined(
							mergeConfigUpdates(prev, updates),
						) as YamlInfrastructureConfig,
				),
			)}
		</div>
	);
}
