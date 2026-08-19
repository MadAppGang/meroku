import type { YamlInfrastructureConfig } from "../types/yamlConfig";

/**
 * Config sections that are merged field-by-field instead of being replaced
 * wholesale when a component emits a partial update.
 *
 * A section MUST be listed here if any component can emit a partial update for
 * it, otherwise every field the emitting component does not know about is
 * dropped on save. `pubsub_appsync` is the sharp example: the AppSync authorizer
 * fields (`jwks_uri`, `jwt_issuer`, `jwt_audience`, schema v21) are not surfaced
 * by any editor panel, so a wholesale replace would silently erase the JWKS
 * trust anchor on an unrelated toggle.
 *
 * Listing a section here is necessary but NOT sufficient. The merge below is
 * one level deep, so with `compute` listed a `{ compute: { pools } }` update
 * merges `pools` over the previous `compute` and `pools` itself is still
 * replaced wholesale. An editor emitting a single pool object would therefore
 * still delete every sibling pool. The guard against that is a type signature,
 * not this list: `ComputePoolEditor` takes `onChange(index, patch)` and never
 * sees the array, and `ComputePoolProperties` is the single owner that maps
 * over the full current array. See architecture.md §8.4 (C-3).
 *
 * `web/` has no test runner, so there is no unit test asserting any of this.
 */
export const NESTED_CONFIG_KEYS = [
	"workload",
	"domain",
	"postgres",
	"cognito",
	"ses",
	"sqs",
	"alb",
	"pubsub_appsync",
	"compute",
] as const;

export type NestedConfigKey = (typeof NESTED_CONFIG_KEYS)[number];

/**
 * Apply a partial update to a config, deep merging the known nested sections.
 *
 * Components may capture e.g. `config.workload` at render time, which can be
 * stale if another component just updated a different field of the same
 * section. Merging into `prevConfig` (always current when called from a
 * `setState` updater) keeps fields the emitting component never saw.
 *
 * Returns `prevConfig` by reference when the update is a no-op, so callers can
 * skip a re-render and a redundant save with a `===` check.
 */
export function mergeConfigUpdates(
	prevConfig: YamlInfrastructureConfig,
	updates: Partial<YamlInfrastructureConfig>,
): YamlInfrastructureConfig {
	const updatedConfig: YamlInfrastructureConfig = { ...prevConfig };
	const nestedKeys: readonly string[] = NESTED_CONFIG_KEYS;

	for (const [key, value] of Object.entries(updates)) {
		if (
			value != null &&
			typeof value === "object" &&
			!Array.isArray(value) &&
			nestedKeys.includes(key)
		) {
			// Deep merge: prevConfig fields as base, updates override only specified fields
			(updatedConfig as unknown as Record<string, unknown>)[key] = {
				...(((prevConfig as unknown as Record<string, unknown>)[key] as
					| object
					| undefined) || {}),
				...value,
			};
		} else {
			(updatedConfig as unknown as Record<string, unknown>)[key] = value;
		}
	}

	// Only return a new object if something actually changed (deep equality
	// check). This prevents unnecessary re-renders when the data is unchanged.
	if (JSON.stringify(prevConfig) === JSON.stringify(updatedConfig)) {
		return prevConfig;
	}

	return updatedConfig;
}
