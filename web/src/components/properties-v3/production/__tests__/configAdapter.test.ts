import { describe, expect, it } from "vitest";
import type { YamlInfrastructureConfig } from "../../../../types/yamlConfig";
import { computeDiff } from "../../diffEngine";
import { NODE_CATALOG_BY_ID_V3 } from "../../nodeCatalog";
import {
	bindDefinitionResources,
	buildConfigInstance,
	CONFIG_PANEL_BINDINGS,
	configKeyFor,
	configUpdatesFromChanges,
} from "../configAdapter";

// Synthetic environment config — placeholder account, no real values.
const config = (
	postgres: YamlInfrastructureConfig["postgres"],
): YamlInfrastructureConfig =>
	({
		project: "myapp",
		env: "dev",
		is_prod: false,
		region: "us-east-1",
		account_id: "000000000000",
		state_bucket: "myapp-terraform-state-example",
		state_file: "state.tfstate",
		postgres,
	}) as YamlInfrastructureConfig;

const binding = CONFIG_PANEL_BINDINGS.postgres;
const definition = NODE_CATALOG_BY_ID_V3.postgres;
const fieldsById = new Map(definition.fields?.map((f) => [f.id, f]));

function diffFor(
	base: YamlInfrastructureConfig,
	draft: Record<string, unknown>,
) {
	const instance = buildConfigInstance(definition, base, binding);
	return computeDiff(
		definition,
		instance.baseline,
		{ ...instance.baseline, ...draft },
		{
			nodeInstanceId: instance.nodeInstanceId,
			lifecycleState: instance.lifecycleState,
			cloudWatchDestination: null,
		},
		{ resource: instance.resource },
	);
}

describe("configKeyFor", () => {
	it("binds every editable postgres field to a direct postgres.* key", () => {
		const editable = (definition.fields ?? []).filter(
			(f) => f.valueKind === "editable",
		);
		expect(editable.length).toBeGreaterThan(0);
		for (const field of editable) {
			expect(configKeyFor(field, binding)).not.toBeNull();
		}
	});

	it("leaves AWS-assigned and runtime fields unbound", () => {
		for (const id of [
			"postgres-endpoint",
			"postgres-port",
			"postgres-status",
		]) {
			const field = fieldsById.get(id);
			if (!field) throw new Error(`missing ${id}`);
			expect(configKeyFor(field, binding)).toBeNull();
		}
	});
});

describe("buildConfigInstance", () => {
	it("keys the baseline by field id from the YAML block", () => {
		const instance = buildConfigInstance(
			definition,
			config({
				enabled: true,
				dbname: "myapp",
				username: "postgres",
				aurora: true,
				min_capacity: 0,
			}),
			binding,
		);
		expect(instance.nodeInstanceId).toBe("myapp/dev/postgres");
		expect(instance.lifecycleState).toBe("created");
		expect(instance.baseline["postgres-dbname"]).toBe("myapp");
		// 0 is a real value (Aurora pause), not a missing one.
		expect(instance.baseline["postgres-min-capacity"]).toBe(0);
		expect(instance.baseline["postgres-max-capacity"]).toBeUndefined();
		expect(instance.baseline["postgres-endpoint"]).toBeUndefined();
	});

	it("names the real RDS / Aurora identifier as the affected resource", () => {
		expect(
			buildConfigInstance(definition, config({ enabled: true }), binding)
				.resource?.name,
		).toBe("myapp-postgres-dev (RDS instance)");
		expect(
			buildConfigInstance(
				definition,
				config({ enabled: true, aurora: true }),
				binding,
			).resource?.name,
		).toBe("myapp-aurora-dev (Aurora cluster)");
	});

	it("replaces fixture affected resources with the bound resource", () => {
		const resource = {
			id: "postgres",
			name: "myapp-postgres-dev (RDS instance)",
		};
		const bound = bindDefinitionResources(definition, resource);
		const named = (bound.fields ?? []).flatMap(
			(f) => f.affectedResources ?? [],
		);
		expect(named.length).toBeGreaterThan(0);
		expect(named.every((r) => r === resource)).toBe(true);
	});

	it("shows the template default when the YAML omits a defaulted key", () => {
		const instance = buildConfigInstance(
			definition,
			config({ enabled: true }),
			binding,
		);
		expect(instance.baseline["postgres-instance-class"]).toBe("db.t4g.micro");
		expect(instance.baseline["postgres-allocated-storage"]).toBe(20);
	});

	it("tolerates a config without a postgres block", () => {
		const instance = buildConfigInstance(
			definition,
			config(undefined),
			binding,
		);
		expect(instance.baseline["postgres-dbname"]).toBeUndefined();
	});
});

describe("configUpdatesFromChanges", () => {
	const base = config({
		enabled: true,
		dbname: "myapp",
		username: "postgres",
		instance_class: "db.t4g.micro",
	});

	it("writes reviewed changes into the postgres block and keeps the rest", () => {
		const diff = diffFor(base, {
			"postgres-dbname": "orders",
			"postgres-allocated-storage": 50,
		});
		const result = configUpdatesFromChanges(
			definition,
			base,
			binding,
			diff.changes,
		);
		expect(result).toEqual({
			ok: true,
			updates: {
				postgres: {
					enabled: true,
					dbname: "orders",
					username: "postgres",
					instance_class: "db.t4g.micro",
					allocated_storage: 50,
				},
			},
		});
	});

	it("marks a dbname change as destructive before anything is written", () => {
		const diff = diffFor(base, { "postgres-dbname": "orders" });
		expect(diff.destructive).toBe(true);
		expect(diff.changes[0]?.impact).toBe("recreates-data-loss");
	});

	it("rejects clearing a value instead of silently keeping the old one", () => {
		const diff = diffFor(base, { "postgres-dbname": undefined });
		const result = configUpdatesFromChanges(
			definition,
			base,
			binding,
			diff.changes,
		);
		expect(result.ok).toBe(false);
	});

	it("rejects changes to fields with no YAML source", () => {
		const result = configUpdatesFromChanges(definition, base, binding, [
			{
				fieldId: "postgres-endpoint",
				propertyPath: "endpoint",
				operation: "set",
				before: undefined,
				after: "db.example.invalid",
				impact: "applies-on-deploy",
				persistenceChannel: "configuration",
				affectedResources: [],
			},
		]);
		expect(result).toEqual({
			ok: false,
			message: "endpoint is not stored in the environment YAML",
		});
	});
});
