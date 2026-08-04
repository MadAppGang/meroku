import { describe, expect, test } from "bun:test";
import * as yaml from "js-yaml";
import type { YamlInfrastructureConfig } from "../types/yamlConfig";
import { mergeConfigUpdates, NESTED_CONFIG_KEYS } from "./configMerge";

/**
 * Mirrors the editor's real round trip:
 *   App.loadConfiguration   -> yaml.load(content)
 *   App.handleConfigChange  -> mergeConfigUpdates(prev, updates)
 *   App.saveConfigToBackend -> yaml.dump(updatedConfig, ...)
 */
function load(content: string): YamlInfrastructureConfig {
	return yaml.load(content) as YamlInfrastructureConfig;
}

function save(config: YamlInfrastructureConfig): string {
	return yaml.dump(config, {
		indent: 2,
		lineWidth: -1,
		noRefs: true,
		sortKeys: false,
	});
}

const YAML_WITH_APPSYNC_AUTHORIZER = `schema_version: 21
project: demo
env: dev
is_prod: false
region: us-east-1
state_bucket: demo-tf-state
state_file: dev/terraform.tfstate
pubsub_appsync:
  enabled: true
  schema: true
  auth_lambda: true
  resolvers: true
  jwks_uri: https://auth.example.com/.well-known/jwks.json
  jwt_issuer: https://auth.example.com/
  jwt_audience: demo-api
workload:
  bucket_postfix: abc12
  xray_enabled: false
`;

describe("AppSync authorizer round trip (schema v21)", () => {
	test("preserves jwks_uri/jwt_issuer/jwt_audience across an unrelated edit", () => {
		const loaded = load(YAML_WITH_APPSYNC_AUTHORIZER);

		// before
		expect(loaded.pubsub_appsync?.jwks_uri).toBe(
			"https://auth.example.com/.well-known/jwks.json",
		);
		expect(loaded.pubsub_appsync?.jwt_issuer).toBe("https://auth.example.com/");
		expect(loaded.pubsub_appsync?.jwt_audience).toBe("demo-api");

		// an unrelated edit somewhere else in the config
		const afterUnrelatedEdit = mergeConfigUpdates(loaded, {
			workload: { ...loaded.workload, xray_enabled: true },
		});

		// after: save -> reload
		const reloaded = load(save(afterUnrelatedEdit));

		expect(reloaded.workload?.xray_enabled).toBe(true);
		expect(reloaded.pubsub_appsync?.jwks_uri).toBe(
			"https://auth.example.com/.well-known/jwks.json",
		);
		expect(reloaded.pubsub_appsync?.jwt_issuer).toBe(
			"https://auth.example.com/",
		);
		expect(reloaded.pubsub_appsync?.jwt_audience).toBe("demo-api");
	});

	test("preserves the authorizer fields when pubsub_appsync itself is partially edited", () => {
		const loaded = load(YAML_WITH_APPSYNC_AUTHORIZER);

		// A panel that only knows about the legacy AppSync toggles emits a
		// partial update. The authorizer fields must survive the merge.
		const afterToggle = mergeConfigUpdates(loaded, {
			pubsub_appsync: { enabled: true, resolvers: false },
		});

		const reloaded = load(save(afterToggle));

		expect(reloaded.pubsub_appsync?.resolvers).toBe(false);
		expect(reloaded.pubsub_appsync?.jwks_uri).toBe(
			"https://auth.example.com/.well-known/jwks.json",
		);
		expect(reloaded.pubsub_appsync?.jwt_issuer).toBe(
			"https://auth.example.com/",
		);
		expect(reloaded.pubsub_appsync?.jwt_audience).toBe("demo-api");
	});

	test("the whole saved document survives a no-op-shaped edit", () => {
		const loaded = load(YAML_WITH_APPSYNC_AUTHORIZER);
		const merged = mergeConfigUpdates(loaded, {
			pubsub_appsync: { enabled: true },
		});

		// mergeConfigUpdates returns the same reference when nothing changed
		expect(merged === loaded).toBe(true);
		expect(save(merged)).toBe(save(loaded));
	});

	test("pubsub_appsync must stay a deep-merged section", () => {
		// Regression guard: dropping pubsub_appsync from NESTED_CONFIG_KEYS turns
		// every partial update into a wholesale replace, which erases jwks_uri.
		expect(NESTED_CONFIG_KEYS).toContain("pubsub_appsync");

		const loaded = load(YAML_WITH_APPSYNC_AUTHORIZER);
		const wholesaleReplace: YamlInfrastructureConfig = {
			...loaded,
			pubsub_appsync: { enabled: true, resolvers: false },
		};

		// This is what the bug would look like - documented so the guard above
		// has visible teeth.
		expect(
			load(save(wholesaleReplace)).pubsub_appsync?.jwks_uri,
		).toBeUndefined();
		expect(
			load(
				save(
					mergeConfigUpdates(loaded, {
						pubsub_appsync: { enabled: true, resolvers: false },
					}),
				),
			).pubsub_appsync?.jwks_uri,
		).toBe("https://auth.example.com/.well-known/jwks.json");
	});
});

describe("mergeConfigUpdates", () => {
	test("deep merges known nested sections and replaces everything else", () => {
		const base = load(YAML_WITH_APPSYNC_AUTHORIZER);
		const merged = mergeConfigUpdates(base, {
			postgres: { enabled: true, dbname: "demo" },
			services: [{ name: "api" }],
		});

		// nested section merged onto an absent base
		expect(merged.postgres?.dbname).toBe("demo");
		// arrays are replaced wholesale, not merged
		expect(merged.services?.length).toBe(1);
		// untouched sections are carried through
		expect(merged.pubsub_appsync?.jwks_uri).toBe(
			"https://auth.example.com/.well-known/jwks.json",
		);
	});
});
