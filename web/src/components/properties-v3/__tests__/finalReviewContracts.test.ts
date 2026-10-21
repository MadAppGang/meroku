import { describe, expect, it } from "vitest";
import { applyChanges, computeDiff } from "../diffEngine";
import type { PropertyChange, PropertyFieldDefinition } from "../types";
import { context, field, freeze, panel } from "./contractFixtures";

const recordDefinition = () =>
	panel([
		field("records", {
			controlType: "key-value-table",
			collection: { recordKeys: true },
		}),
	]);

// Cleanup protects subsequent tests if an older implementation pollutes a
// prototype. No concurrent tests: mutation checks and restoration are synchronous.
function withPrototypeCleanup(run: () => void) {
	const before = Object.getOwnPropertyDescriptors(Object.prototype);
	try {
		run();
	} finally {
		for (const key of Reflect.ownKeys(Object.prototype)) {
			if (!Object.hasOwn(before, key))
				Reflect.deleteProperty(Object.prototype, key);
		}
		Object.defineProperties(Object.prototype, before);
	}
}

describe("literal record keys and prototype safety — final review", () => {
	it.each(["synthetic.key", "synthetic|key", "synthetic=key", "a.b|c=d"])(
		"record key %s remains a literal key through add/update/remove",
		(key) => {
			const definition = recordDefinition();
			const initial = freeze({ records: { untouched: "same" } });
			const added = freeze({ records: { untouched: "same", [key]: "first" } });
			const updated = freeze({
				records: { untouched: "same", [key]: "second" },
			});
			let baseline: Record<string, unknown> = initial;
			for (const draft of [added, updated, initial]) {
				const result = computeDiff(definition, baseline, draft, context);
				expect(result.changes.length).toBeGreaterThan(0);
				const merged = applyChanges(baseline, result.changes, "configuration");
				expect(merged).toEqual(draft);
				expect(computeDiff(definition, merged, draft, context).changes).toEqual(
					[],
				);
				baseline = freeze(merged);
			}
		},
	);

	it.each(["__proto__", "constructor.prototype"])(
		"synthetic prototype-sensitive record key %s cannot mutate Object.prototype",
		(key) => {
			withPrototypeCleanup(() => {
				const definition = recordDefinition();
				const before = Object.getOwnPropertyDescriptors(Object.prototype);
				const baseline = { records: {} };
				// Computed property syntax creates a real own key, unlike __proto__ in
				// a non-computed object literal. All payloads are synthetic data.
				const draft = {
					records: { [key]: { __v3SyntheticPrototypeProbe: "fake-value" } },
				};
				const result = computeDiff(definition, baseline, draft, context);
				expect(Object.getOwnPropertyDescriptors(Object.prototype)).toEqual(
					before,
				);
				const merged = applyChanges(baseline, result.changes, "configuration");
				expect(Object.getOwnPropertyDescriptors(Object.prototype)).toEqual(
					before,
				);
				expect(Object.hasOwn(merged.records as object, key)).toBe(true);
				expect(merged).toEqual(draft);
				expect(baseline).toEqual({ records: {} });
				expect(computeDiff(definition, merged, draft, context).changes).toEqual(
					[],
				);
			});
		},
	);

	it("merge creates an own record entry without mutating an inherited object", () => {
		const inheritedEntry = { value: "original-inherited-value" };
		const prototype = { SYNTHETIC_INHERITED: inheritedEntry };
		const records = Object.create(prototype) as Record<string, unknown>;
		const baseline = { records };
		const draft = {
			records: { SYNTHETIC_INHERITED: { value: "new-own-value" } },
		};
		try {
			const result = computeDiff(recordDefinition(), baseline, draft, context);
			const merged = applyChanges(baseline, result.changes, "configuration");
			expect(inheritedEntry).toEqual({ value: "original-inherited-value" });
			expect(Object.hasOwn(records, "SYNTHETIC_INHERITED")).toBe(false);
			expect(
				Object.hasOwn(merged.records as object, "SYNTHETIC_INHERITED"),
			).toBe(true);
			expect(merged).toEqual(draft);
		} finally {
			inheritedEntry.value = "original-inherited-value";
		}
	});
});

describe("manually authored structured FIELD addresses enforce prototype guards", () => {
	// Frozen ChangeAddress contract: FIELD path segments are guarded; a literal
	// RECORD key '__proto__' remains a supported own key (covered separately).
	for (const segment of ["__proto__", "prototype", "constructor"]) {
		it.each([
			{ path: [segment, "__v3SyntheticFieldProbe"] },
			{ path: ["synthetic", segment, "__v3SyntheticFieldProbe"] },
		])(
			"rejects dangerous FIELD address $path without mutating input or prototypes",
			({ path }) => {
				withPrototypeCleanup(() => {
					const before = Object.getOwnPropertyDescriptors(Object.prototype);
					const baseline = freeze({ synthetic: {} });
					const change: PropertyChange = {
						fieldId: "synthetic-field",
						propertyPath: path.join("."),
						address: { path },
						operation: "set",
						before: undefined,
						after: "fake-value",
						impact: "applies-on-deploy",
						persistenceChannel: "configuration",
						affectedResources: [],
					};
					expect(() =>
						applyChanges(baseline, [change], "configuration"),
					).toThrow();
					expect(baseline).toEqual({ synthetic: {} });
					expect(Object.getOwnPropertyDescriptors(Object.prototype)).toEqual(
						before,
					);
				});
			},
		);
	}
});

describe("newly visible required fields validate even when missing values are unchanged", () => {
	it("cross-account ECR strategy reports the missing source account only on configuration channel", () => {
		const definition = panel([
			field("strategy", { property: "ecr_strategy", controlType: "select" }),
			field("source-account", {
				property: "ecr_account_id",
				valueType: "string",
				validation: { required: true },
				visibleWhen: {
					all: [
						{ kind: "equals", key: "ecr_strategy", value: "cross_account" },
					],
				},
			}),
			field("live", { mutability: "LIVE" }),
		]);
		const baseline = freeze({ ecr_strategy: "local", live: "old" });
		const unchanged = computeDiff(definition, baseline, baseline, context);
		expect(
			unchanged.validationErrors.some(
				(error) => error.fieldId === "source-account",
			),
		).toBe(false);
		const draft = freeze({ ecr_strategy: "cross_account", live: "new" });
		const result = computeDiff(definition, baseline, draft, context);
		expect(result.validationErrors).toEqual(
			expect.arrayContaining([
				expect.objectContaining({
					fieldId: "source-account",
					persistenceChannel: "configuration",
					message: expect.any(String),
				}),
			]),
		);
		expect(
			result.validationErrors.some(
				(error) => error.persistenceChannel === "live",
			),
		).toBe(false);
		expect(
			result.changes.find((change) => change.fieldId === "live"),
		).toMatchObject({
			persistenceChannel: "live",
			before: "old",
			after: "new",
		});
		const merged = applyChanges(baseline, result.changes, "live");
		expect(merged).toEqual({ ecr_strategy: "local", live: "new" });
	});
});

describe("optional keyed-array cells do not become row membership operations", () => {
	it.each(["add", "remove"] as const)(
		"%s optional cell preserves row identity and siblings",
		(operation) => {
			const definition = panel([
				field("rows", {
					controlType: "key-value-table",
					collection: { identityKeys: ["name"] },
				}),
			]);
			const absent = {
				rows: [
					{ name: "target", value: "same" },
					{ name: "other", value: "untouched" },
				],
			};
			const present = {
				rows: [
					{ name: "target", value: "same", description: "synthetic optional" },
					{ name: "other", value: "untouched" },
				],
			};
			const baseline = freeze(operation === "add" ? absent : present);
			const draft = freeze(operation === "add" ? present : absent);
			const result = computeDiff(definition, baseline, draft, context);
			expect(result.changes).toHaveLength(1);
			expect(result.changes[0]).toMatchObject({
				rowKey: "name=target",
				propertyPath: "rows[name=target].description",
			});
			const merged = applyChanges(baseline, result.changes, "configuration");
			expect(merged).toEqual(draft);
			expect((merged.rows as unknown[]).length).toBe(2);
			if (operation === "remove") {
				expect(Object.hasOwn((merged.rows as object[])[0], "description")).toBe(
					false,
				);
			}
			expect(computeDiff(definition, merged, draft, context).changes).toEqual(
				[],
			);
		},
	);
});

describe("explicit value types validate without silent coercion", () => {
	const cases: [
		NonNullable<PropertyFieldDefinition["valueType"]>,
		unknown,
		unknown,
	][] = [
		["string", "synthetic", 42],
		["number", 0, "not-a-number"],
		["boolean", false, "false"],
		["string-array", ["synthetic-a", "synthetic-b"], ["synthetic", 42]],
		["number-array", [0, 2], [0, "two"]],
		["json", { synthetic: [0, false] }, undefined],
	];
	it.each(cases.filter(([kind]) => kind !== "json"))(
		"%s rejects a mismatched dirty value on the correct channel",
		(valueType, valid, invalid) => {
			const definition = panel([
				field("typed", { valueType }),
				field("live", { mutability: "LIVE" }),
			]);
			const result = computeDiff(
				definition,
				{ typed: valid, live: "old" },
				{ typed: invalid, live: "new" },
				context,
			);
			expect(result.validationErrors).toEqual(
				expect.arrayContaining([
					expect.objectContaining({
						fieldId: "typed",
						persistenceChannel: "configuration",
					}),
				]),
			);
			expect(result.changes.some((change) => change.fieldId === "live")).toBe(
				true,
			);
		},
	);

	it.each(cases)(
		"%s valid value roundtrips without losing zero/false or structure",
		(valueType, valid) => {
			const definition = panel([field("typed", { valueType })]);
			const result = computeDiff(definition, {}, { typed: valid }, context);
			expect(result.validationErrors).toEqual([]);
			expect(applyChanges({}, result.changes, "configuration")).toEqual({
				typed: valid,
			});
		},
	);

	it("typed collection columns validate a required missing cell without dropping live changes", () => {
		const definition = panel([
			field("rows", {
				controlType: "key-value-table",
				collection: {
					identityKeys: ["name"],
					columns: [
						{ key: "name", valueType: "string", required: true },
						{
							key: "replicas",
							label: "Replicas",
							valueType: "number",
							required: true,
						},
					],
				},
			}),
			field("live", { mutability: "LIVE" }),
		]);
		for (const row of [
			{ name: "synthetic" },
			{ name: "synthetic", replicas: "invalid" },
		]) {
			const result = computeDiff(
				definition,
				{ rows: [], live: "old" },
				{ rows: [row], live: "new" },
				context,
			);
			expect(result.validationErrors).toEqual(
				expect.arrayContaining([
					expect.objectContaining({
						fieldId: "rows",
						persistenceChannel: "configuration",
					}),
				]),
			);
			expect(
				result.changes.find((change) => change.fieldId === "live"),
			).toMatchObject({ persistenceChannel: "live" });
		}
		const valid = computeDiff(
			definition,
			{ rows: [] },
			{ rows: [{ name: "synthetic", replicas: 0 }] },
			context,
		);
		expect(valid.validationErrors).toEqual([]);
	});
});
