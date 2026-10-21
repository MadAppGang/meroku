import { describe, expect, it } from "vitest";
import { applyChanges, computeDiff } from "../diffEngine";
import type {
	MutabilityClass,
	PersistenceChannel,
	PropertyImpact,
	ValueKind,
} from "../types";
import {
	context,
	databaseFields,
	field,
	freeze,
	panel,
	parameterField,
} from "./contractFixtures";

const mappings: [MutabilityClass, PropertyImpact, PersistenceChannel][] = [
	["LIVE", "applies-now", "live"],
	["LIVE-RESTART", "restart-to-observe", "live"],
	["TF", "applies-on-deploy", "configuration"],
	["TF-DEPLOY", "rolling-restart", "configuration"],
	["TF-REPLACE", "recreates-data-loss", "configuration"],
	["CREATE / TF-REPLACE", "recreates-data-loss", "configuration"],
	["TF-REPLACE / TF-DEPLOY", "recreates-data-loss", "configuration"],
	["CREATE", "set-once", "configuration"],
	["LIVE, create-only", "set-once", "live"],
];

describe("field impact, lifecycle, and eligibility — DIF-01/02/03/04", () => {
	it.each(mappings)(
		"%s keeps impact %s on an uncreated resource",
		(mutability, impact, persistenceChannel) => {
			const result = computeDiff(
				panel([field("value", { mutability })]),
				{ value: "old" },
				{ value: "new" },
				{ ...context, lifecycleState: "uncreated" },
			);
			expect(result.changes).toHaveLength(1);
			expect(result.changes[0]).toMatchObject({
				fieldId: "value",
				propertyPath: "value",
				before: "old",
				after: "new",
				impact,
				persistenceChannel,
			});
			expect(result.worstImpact).toBe(impact);
			expect(result.destructive).toBe(impact === "recreates-data-loss");
			expect(result.dirtyChannels).toEqual([persistenceChannel]);
		},
	);

	it.each<MutabilityClass>(["CREATE", "LIVE, create-only"])(
		"locks genuine %s after creation",
		(mutability) => {
			const result = computeDiff(
				panel([field("name", { mutability })]),
				{ name: "old" },
				{ name: "new" },
				context,
			);
			expect(result.changes).toEqual([]);
			expect(result.dirtyChannels).toEqual([]);
			expect(result.worstImpact).toBeNull();
			expect(result.destructive).toBe(false);
		},
	);

	it.each(["mode", "dbname", "username"])(
		"existing database %s remains destructive rather than locked",
		(id) => {
			const selected = databaseFields.find((item) => item.id === id);
			if (!selected) throw new Error(`unknown database field ${id}`);
			const result = computeDiff(
				panel([selected]),
				{ [id]: id === "mode" ? false : "old" },
				{ [id]: id === "mode" ? true : "new" },
				context,
			);
			expect(result.changes).toHaveLength(1);
			expect(result.changes[0].impact).toBe("recreates-data-loss");
			expect(result.destructive).toBe(true);
		},
	);

	it.each<ValueKind>(["aws-assigned", "telemetry", "derived", "action"])(
		"excludes non-editable kind %s even if mutability suggests replacement",
		(valueKind) => {
			const result = computeDiff(
				panel([field("excluded", { valueKind, mutability: "TF-REPLACE" })]),
				{ excluded: "old" },
				{ excluded: "new" },
				context,
			);
			expect(result.changes).toEqual([]);
			expect(result.affectedResources).toEqual([]);
			expect(result.destructive).toBe(false);
		},
	);

	it.each<MutabilityClass>(["RO", "RUNTIME"])(
		"excludes %s regardless of changed supplied values",
		(mutability) => {
			const result = computeDiff(
				panel([field("excluded", { mutability })]),
				{ excluded: 1 },
				{ excluded: 2 },
				context,
			);
			expect(result.changes).toEqual([]);
			expect(result.dirtyChannels).toEqual([]);
		},
	);

	it("excludes hidden destructive fields from changes, resources, and worst impact", () => {
		const definition = panel([
			field("mode", { mutability: "RO", section: "identity" }),
			field("hidden", {
				mutability: "TF-REPLACE",
				affectedResources: [{ id: "hidden-resource", name: "Hidden" }],
				visibleWhen: { all: [{ kind: "equals", key: "mode", value: true }] },
			}),
			field("visible", { mutability: "LIVE" }),
		]);
		const result = computeDiff(
			definition,
			{ mode: true, hidden: 1, visible: "old" },
			{ mode: false, hidden: 2, visible: "new" },
			context,
		);
		expect(result.changes.map((change) => change.fieldId)).toEqual(["visible"]);
		expect(result.worstImpact).toBe("applies-now");
		expect(result.destructive).toBe(false);
		expect(result.affectedResources).toEqual([]);
	});

	it("pure diff re-includes a changed field when made visible again; controller owns hidden-draft reset", () => {
		const definition = panel(databaseFields);
		const baseline = {
			mode: false,
			instance: "db.synthetic.small",
			capacity: 1,
		};
		const hidden = computeDiff(
			definition,
			baseline,
			{ ...baseline, mode: true, instance: "db.synthetic.large" },
			context,
		);
		expect(hidden.changes.some((change) => change.fieldId === "instance")).toBe(
			false,
		);
		const restoredBaseline = computeDiff(
			definition,
			baseline,
			{ ...baseline },
			context,
		);
		expect(restoredBaseline.changes).toEqual([]);
		const freshEdit = computeDiff(
			definition,
			baseline,
			{ ...baseline, instance: "db.synthetic.large" },
			context,
		);
		expect(freshEdit.changes.map((change) => change.fieldId)).toEqual([
			"instance",
		]);
	});

	it("Aurora capacity zero is a real deploy-in-place edit", () => {
		const result = computeDiff(
			panel(databaseFields),
			{ mode: true, capacity: 1 },
			{ mode: true, capacity: 0 },
			context,
		);
		expect(result.changes).toHaveLength(1);
		expect(result.changes[0]).toMatchObject({
			fieldId: "capacity",
			before: 1,
			after: 0,
			impact: "applies-on-deploy",
		});
	});

	it("UI canvas changes are non-destructive layout operations", () => {
		const definition = panel(
			[
				field("width", {
					mutability: "UI",
					valueKind: "canvas-only",
					controlType: "number",
					section: "layout",
				}),
			],
			{ canvasOnly: true },
		);
		const result = computeDiff(
			definition,
			{ width: 100 },
			{ width: 200 },
			context,
		);
		expect(result.changes).toHaveLength(1);
		expect(result.changes[0]).toMatchObject({
			fieldId: "width",
			before: 100,
			after: 200,
			persistenceChannel: "layout",
		});
		expect(result.dirtyChannels).toEqual(["layout"]);
		expect(result.destructive).toBe(false);
		expect(result.affectedResources).toEqual([]);
		expect(applyChanges({ width: 100 }, result.changes, "layout")).toEqual({
			width: 200,
		});
	});
});

describe("deep equality, storage paths, and purity — DIF-05", () => {
	it("key-order differences are clean and real nested changes have correct before/after", () => {
		const definition = panel([field("value")]);
		const baseline = { value: { a: 0, b: { enabled: false } } };
		expect(
			computeDiff(
				definition,
				baseline,
				{ value: { b: { enabled: false }, a: 0 } },
				context,
			).changes,
		).toEqual([]);
		const changed = computeDiff(
			definition,
			baseline,
			{ value: { a: 0, b: { enabled: true } } },
			context,
		);
		expect(changed.changes).toHaveLength(1);
		expect(changed.changes[0]).toMatchObject({
			fieldId: "value",
			propertyPath: "value",
			before: baseline.value,
			after: { a: 0, b: { enabled: true } },
		});
	});

	it.each([
		[
			"ID",
			{ value: "old", untouched: 0 },
			{ value: "new", untouched: 0 },
			"value",
		],
		[
			"flat alias",
			{ "synthetic.value": "old", untouched: false },
			{ "synthetic.value": "new", untouched: false },
			"synthetic.value",
		],
		[
			"nested alias",
			{ synthetic: { value: "old", sibling: 0 } },
			{ synthetic: { value: "new", sibling: 0 } },
			"synthetic.value",
		],
	])(
		"merges back into actual %s storage without mutation",
		(_name, baseline, draft, propertyPath) => {
			const definition = freeze(panel([field("value")]));
			freeze(baseline);
			freeze(draft);
			const result = computeDiff(
				definition,
				baseline,
				draft,
				freeze({ ...context }),
			);
			expect(result.changes).toHaveLength(1);
			expect(result.changes[0].propertyPath).toBe(propertyPath);
			const merged = applyChanges(
				baseline,
				freeze(result.changes),
				"configuration",
			);
			expect(merged).toEqual(draft);
			expect(merged).not.toBe(baseline);
			expect(computeDiff(definition, merged, draft, context).changes).toEqual(
				[],
			);
		},
	);

	it("reverting an edit clears dirty state and destructive resources", () => {
		const definition = panel([field("name", { mutability: "TF-REPLACE" })]);
		const baseline = { name: "original" };
		expect(
			computeDiff(definition, baseline, { name: "changed" }, context)
				.destructive,
		).toBe(true);
		const reverted = computeDiff(
			definition,
			baseline,
			{ ...baseline },
			context,
		);
		expect(reverted.changes).toEqual([]);
		expect(reverted.dirtyChannels).toEqual([]);
		expect(reverted.worstImpact).toBeNull();
		expect(reverted.affectedResources).toEqual([]);
	});

	it("does not guess row identities for unkeyed collections", () => {
		const definition = panel([
			field("items", { collection: {}, controlType: "key-value-table" }),
		]);
		const baseline = {
			items: [
				{ name: "a", value: "one" },
				{ name: "b", value: "two" },
			],
		};
		const draft = {
			items: [
				{ name: "b", value: "changed" },
				{ name: "a", value: "one" },
			],
		};
		const result = computeDiff(definition, baseline, draft, context);
		expect(result.changes.length).toBeGreaterThan(0);
		expect(result.changes.every((change) => change.rowKey === undefined)).toBe(
			true,
		);
		expect(applyChanges(baseline, result.changes, "configuration")).toEqual(
			draft,
		);
	});
});

describe("parameter collections and plaintext domain impacts — DIF-06/07/08", () => {
	it.each([
		["value", "restart-to-observe"],
		["description", "applies-now"],
	] as const)(
		"parameter %s cell edit has %s impact on live channel",
		(key, impact) => {
			const definition = panel([parameterField()]);
			const baseline = {
				parameters: {
					API_KEY: { value: "fake-old", description: "synthetic old" },
				},
			};
			const draft = {
				parameters: {
					API_KEY: { ...baseline.parameters.API_KEY, [key]: "synthetic new" },
				},
			};
			const result = computeDiff(definition, baseline, draft, context);
			expect(result.changes).toHaveLength(1);
			expect(result.changes[0]).toMatchObject({
				fieldId: "parameters",
				propertyPath: `parameters.API_KEY.${key}`,
				operation: "row-update",
				impact,
				persistenceChannel: "live",
				before: baseline.parameters.API_KEY[key],
				after: "synthetic new",
			});
		},
	);

	it.each(["add", "remove", "rename"] as const)(
		"parameter record membership %s is configuration rolling restart",
		(operation) => {
			const row = { value: "fake-value", description: "synthetic" };
			const baseline = { parameters: { OLD: row } };
			const draft = {
				parameters:
					operation === "add"
						? { OLD: row, NEW: row }
						: operation === "remove"
							? {}
							: { NEW: row },
			};
			// membershipImpact does not document an automatic channel override.
			const result = computeDiff(
				panel([parameterField({ persistenceChannel: "configuration" })]),
				baseline,
				draft,
				context,
			);
			expect(result.changes.map((change) => change.operation).sort()).toEqual(
				operation === "rename"
					? ["row-add", "row-remove"]
					: [operation === "add" ? "row-add" : "row-remove"],
			);
			expect(
				result.changes.every(
					(change) =>
						change.impact === "rolling-restart" &&
						change.persistenceChannel === "configuration",
				),
			).toBe(true);
			expect(applyChanges(baseline, result.changes, "configuration")).toEqual(
				draft,
			);
		},
	);

	it("declared array identity survives reordering and updates the correct row", () => {
		const item = parameterField({
			collection: {
				identityKeys: ["name"],
				diffRules: {
					valueProperties: ["value"],
					valueEditImpact: "restart-to-observe",
					valueEditChannel: "live",
					membershipImpact: "rolling-restart",
				},
			},
		});
		const definition = panel([item]);
		const baseline = {
			parameters: [
				{ name: "A", value: "a" },
				{ name: "B", value: "b" },
			],
		};
		const reordered = {
			parameters: [baseline.parameters[1], baseline.parameters[0]],
		};
		expect(
			computeDiff(definition, baseline, reordered, context).changes,
		).toEqual([]);
		const draft = {
			parameters: [{ name: "B", value: "changed" }, baseline.parameters[0]],
		};
		const result = computeDiff(definition, baseline, draft, context);
		expect(result.changes).toHaveLength(1);
		expect(result.changes[0]).toMatchObject({
			rowKey: "name=B",
			operation: "row-update",
			before: "b",
			after: "changed",
			impact: "restart-to-observe",
			persistenceChannel: "live",
		});
		const merged = applyChanges(baseline, result.changes, "live");
		expect(computeDiff(definition, merged, draft, context).changes).toEqual([]);
	});

	it("compound identity-key rename produces remove+add, not positional update", () => {
		const definition = panel([
			parameterField({
				persistenceChannel: "configuration",
				collection: {
					identityKeys: ["scope", "name"],
					diffRules: {
						membershipImpact: "rolling-restart",
						valueProperties: ["value"],
						valueEditImpact: "restart-to-observe",
						valueEditChannel: "live",
					},
				},
			}),
		]);
		const baseline = {
			parameters: [{ scope: "task", name: "OLD", value: "fake" }],
		};
		const draft = {
			parameters: [{ scope: "task", name: "NEW", value: "fake" }],
		};
		const result = computeDiff(definition, baseline, draft, context);
		expect(result.changes.map((change) => change.operation).sort()).toEqual([
			"row-add",
			"row-remove",
		]);
		expect(result.changes.map((change) => change.rowKey).sort()).toEqual([
			"scope=task|name=NEW",
			"scope=task|name=OLD",
		]);
		expect(
			result.changes.every(
				(change) =>
					change.impact === "rolling-restart" &&
					change.persistenceChannel === "configuration",
			),
		).toBe(true);
		expect(applyChanges(baseline, result.changes, "configuration")).toEqual(
			draft,
		);
	});

	for (const property of [
		"workload.backend_env_variables",
		"services[].environment_variables",
	]) {
		it.each(["value", "empty", "add", "remove", "rename"] as const)(
			`${property} %s is always rolling-restart/configuration`,
			(operation) => {
				const definition = panel([
					field("env", {
						property,
						mutability: "TF-DEPLOY",
						section: "environment-secrets",
						controlType: "key-value-table",
						collection: {
							recordKeys: true,
							diffRules: { forceImpact: "rolling-restart" },
						},
					}),
				]);
				const baseline = { env: { OLD: { value: "plain-old" } } };
				const variants = {
					value: { OLD: { value: "plain-new" } },
					empty: { OLD: { value: "" } },
					add: { ...baseline.env, NEW: { value: "plain-new" } },
					remove: {},
					rename: { NEW: { value: "plain-old" } },
				};
				const draft = { env: variants[operation] };
				const result = computeDiff(definition, baseline, draft, context);
				expect(result.changes.length).toBeGreaterThan(0);
				expect(
					result.changes.every(
						(change) =>
							change.impact === "rolling-restart" &&
							change.persistenceChannel === "configuration",
					),
				).toBe(true);
				expect(applyChanges(baseline, result.changes, "configuration")).toEqual(
					draft,
				);
			},
		);
	}
});

describe("opaque array identity values preserve roundtrips", () => {
	const keyed = () =>
		panel([
			parameterField({
				persistenceChannel: "configuration",
				collection: {
					identityKeys: ["scope", "name"],
					diffRules: {
						valueProperties: ["value"],
						valueEditImpact: "restart-to-observe",
						valueEditChannel: "live",
						membershipImpact: "rolling-restart",
					},
				},
			}),
		]);

	it.each(["with|pipe", "with=equals", "with|name=embedded", "a=b|c=d"])(
		"updates the exact row when an identity contains %s",
		(name) => {
			const definition = keyed();
			const baseline = freeze({
				parameters: [
					{ scope: "synthetic", name, value: "old" },
					{ scope: "synthetic", name: "untouched", value: "same" },
				],
			});
			const draft = freeze({
				parameters: [
					{ scope: "synthetic", name, value: "new" },
					{ scope: "synthetic", name: "untouched", value: "same" },
				],
			});
			const result = computeDiff(definition, baseline, draft, context);
			expect(result.changes).toHaveLength(1);
			expect(result.changes[0]).toMatchObject({
				operation: "row-update",
				before: "old",
				after: "new",
				persistenceChannel: "live",
			});
			const merged = applyChanges(baseline, result.changes, "live");
			expect(merged).toEqual(draft);
			expect(computeDiff(definition, merged, draft, context).changes).toEqual(
				[],
			);
		},
	);

	it("does not collide distinct composite identities containing delimiter-like text", () => {
		const definition = keyed();
		// These tuples collide if identity values are concatenated without escaping.
		const baseline = freeze({
			parameters: [
				{ scope: "a|name=b", name: "c", value: "first-old" },
				{ scope: "a", name: "b|name=c", value: "second-old" },
			],
		});
		const draft = freeze({
			parameters: [
				{ scope: "a|name=b", name: "c", value: "first-new" },
				{ scope: "a", name: "b|name=c", value: "second-new" },
			],
		});
		const result = computeDiff(definition, baseline, draft, context);
		expect(result.changes).toHaveLength(2);
		const merged = applyChanges(baseline, result.changes, "live");
		expect(merged).toEqual(draft);
		expect(computeDiff(definition, merged, draft, context).changes).toEqual([]);
	});

	it("roundtrips a delimiter-containing key rename as remove plus add", () => {
		const definition = keyed();
		const baseline = freeze({
			parameters: [{ scope: "a=b", name: "old|name=x", value: "fake" }],
		});
		const draft = freeze({
			parameters: [{ scope: "a=b", name: "new|name=y", value: "fake" }],
		});
		const result = computeDiff(definition, baseline, draft, context);
		expect(result.changes.map((change) => change.operation).sort()).toEqual([
			"row-add",
			"row-remove",
		]);
		const merged = applyChanges(baseline, result.changes, "configuration");
		expect(merged).toEqual(draft);
		expect(computeDiff(definition, merged, draft, context).changes).toEqual([]);
	});
});

describe("affected resource summary and channel-scoped consequence — DIF-09/SAV-05", () => {
	it("deduplicates resources by identity and includes only dirty destructive resources", () => {
		const first = { id: "rds-1", name: "PostgreSQL" };
		const second = { id: "rds-2", name: "PostgreSQL" };
		const definition = panel([
			field("dbname", {
				property: "postgres.dbname",
				mutability: "TF-REPLACE",
				affectedResources: [first],
			}),
			field("username", {
				property: "postgres.username",
				mutability: "TF-REPLACE",
				affectedResources: [first, second],
			}),
			field("clean", {
				mutability: "TF-REPLACE",
				affectedResources: [{ id: "clean", name: "Untouched" }],
			}),
			field("ordinary", {
				mutability: "TF",
				affectedResources: [{ id: "ordinary", name: "Not replaced" }],
			}),
		]);
		const baseline = {
			postgres: { dbname: "old", username: "old-user" },
			clean: "same",
			ordinary: 1,
		};
		const draft = {
			postgres: { dbname: "new", username: "new-user" },
			clean: "same",
			ordinary: 2,
		};
		const result = computeDiff(definition, baseline, draft, context);
		expect(result.affectedResources).toHaveLength(2);
		expect(result.affectedResources).toEqual(
			expect.arrayContaining([first, second]),
		);
		expect(
			result.changes
				.filter((change) => change.impact === "recreates-data-loss")
				.map((change) => change.propertyPath)
				.sort(),
		).toEqual(["postgres.dbname", "postgres.username"]);
	});

	it("uses explicit field resources before the node resource fallback", () => {
		const explicit = { id: "explicit", name: "Explicit synthetic DB" };
		const fallback = { id: "node", name: "Node synthetic DB" };
		const definition = panel([
			field("a", { mutability: "TF-REPLACE", affectedResources: [explicit] }),
			field("b", { mutability: "TF-REPLACE" }),
		]);
		const result = computeDiff(
			definition,
			{ a: 1, b: 1 },
			{ a: 2, b: 2 },
			context,
			{ resource: fallback },
		);
		expect(
			result.changes.find((change) => change.fieldId === "a")
				?.affectedResources,
		).toEqual([explicit]);
		expect(
			result.changes.find((change) => change.fieldId === "b")
				?.affectedResources,
		).toEqual([fallback]);
	});

	it("recalculates global and channel maximum as severe edits are reverted", () => {
		const fields = [
			field("replace", { mutability: "TF-REPLACE" }),
			field("rolling", { mutability: "TF-DEPLOY" }),
			field("deploy", { mutability: "TF" }),
			field("restart", { mutability: "LIVE-RESTART" }),
			field("now", { mutability: "LIVE" }),
		];
		const definition = panel(fields);
		const baseline = Object.fromEntries(fields.map((item) => [item.id, 0]));
		const expected: PropertyImpact[] = [
			"recreates-data-loss",
			"rolling-restart",
			"applies-on-deploy",
			"restart-to-observe",
			"applies-now",
		];
		for (let index = 0; index < fields.length; index += 1) {
			const draft = {
				...baseline,
				...Object.fromEntries(fields.slice(index).map((item) => [item.id, 1])),
			};
			const result = computeDiff(definition, baseline, draft, context);
			expect(result.worstImpact).toBe(expected[index]);
			expect(result.destructive).toBe(index === 0);
			expect(result.channelSummaries.live?.worstImpact).toBe(
				index < 4 ? "restart-to-observe" : "applies-now",
			);
			if (index < 3)
				expect(result.channelSummaries.configuration?.worstImpact).toBe(
					expected[index],
				);
		}
	});

	it("set-once never outranks a real consequence or becomes destructive", () => {
		const result = computeDiff(
			panel([
				field("name", { mutability: "CREATE" }),
				field("live", { mutability: "LIVE" }),
			]),
			{ name: "old", live: 0 },
			{ name: "new", live: 1 },
			{ ...context, lifecycleState: "uncreated" },
		);
		expect(result.worstImpact).toBe("applies-now");
		expect(result.channelSummaries.configuration?.worstImpact).toBe("set-once");
		expect(result.destructive).toBe(false);
	});
});
