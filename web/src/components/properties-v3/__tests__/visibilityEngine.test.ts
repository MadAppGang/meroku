import { describe, expect, it } from "vitest";
import { resolveFieldValue } from "../helpers";
import {
	evaluateCondition,
	evaluateVisibility,
	getVisibleFields,
	isFieldVisible,
} from "../visibilityEngine";
import { databaseFields, field, freeze, panel } from "./contractFixtures";

describe("visibility public contract — VIS-01/02", () => {
	const control = field("mode", {
		property: ["postgres.aurora", "legacy.aurora"],
	});
	const fields = [control];

	it.each([
		["field ID", { mode: false }],
		["flat property alias", { "postgres.aurora": false }],
		["nested property alias", { postgres: { aurora: false } }],
		["secondary alias", { legacy: { aurora: false } }],
	])("resolves %s through either lookup key", (_label, values) => {
		for (const key of ["mode", "postgres.aurora", "legacy.aurora"]) {
			expect(
				evaluateCondition(
					{ kind: "equals", key, value: false },
					values,
					fields,
				),
			).toBe(true);
			expect(
				evaluateCondition(
					{ kind: "notEquals", key, value: false },
					values,
					fields,
				),
			).toBe(false);
		}
	});

	it.each([false, 0, ""])(
		"preserves own value %j instead of falling through to alias",
		(value) => {
			const values = { mode: value, "postgres.aurora": "incorrect fallback" };
			expect(resolveFieldValue(values, control)).toBe(value);
			expect(
				evaluateCondition(
					{ kind: "exists", key: "postgres.aurora" },
					values,
					fields,
				),
			).toBe(true);
			expect(
				evaluateCondition(
					{ kind: "equals", key: "mode", value },
					values,
					fields,
				),
			).toBe(true);
		},
	);

	it("uses ID, then aliases in declaration order, then flat-before-nested precedence", () => {
		expect(
			resolveFieldValue({ mode: false, "postgres.aurora": true }, control),
		).toBe(false);
		expect(
			resolveFieldValue(
				{ "postgres.aurora": 0, postgres: { aurora: 1 }, "legacy.aurora": 2 },
				control,
			),
		).toBe(0);
		expect(
			resolveFieldValue(
				{ postgres: { aurora: false }, "legacy.aurora": true },
				control,
			),
		).toBe(false);
		expect(
			resolveFieldValue(
				{ "legacy.aurora": "", legacy: { aurora: "fallback" } },
				control,
			),
		).toBe("");
	});

	it("does not resolve inherited ID or inherited nested aliases", () => {
		const inheritedId = Object.create({ mode: true }) as Record<
			string,
			unknown
		>;
		const inheritedAlias = { postgres: Object.create({ aurora: true }) };
		for (const values of [{}, inheritedId, inheritedAlias]) {
			expect(
				evaluateCondition({ kind: "exists", key: "mode" }, values, fields),
			).toBe(false);
		}
	});

	it("compares structured equals values without object-key-order sensitivity", () => {
		expect(
			evaluateCondition(
				{ kind: "equals", key: "mode", value: { enabled: false, count: 0 } },
				{ mode: { count: 0, enabled: false } },
				fields,
			),
		).toBe(true);
	});

	it("requires both all and any groups when both are declared", () => {
		const spec = {
			all: [{ kind: "equals" as const, key: "mode", value: false }],
			any: [
				{ kind: "equals" as const, key: "count", value: 0 },
				{ kind: "equals" as const, key: "count", value: 2 },
			],
		};
		const controls = [...fields, field("count")];
		expect(evaluateVisibility(spec, { mode: false, count: 0 }, controls)).toBe(
			true,
		);
		expect(evaluateVisibility(spec, { mode: true, count: 0 }, controls)).toBe(
			false,
		);
		expect(evaluateVisibility(spec, { mode: false, count: 1 }, controls)).toBe(
			false,
		);
		expect(evaluateVisibility(undefined, {}, controls)).toBe(true);
	});

	it("returns visible fields in declaration order without mutating inputs", () => {
		const definition = freeze(
			panel([
				field("first"),
				field("hidden", {
					visibleWhen: { all: [{ kind: "equals", key: "mode", value: true }] },
				}),
				control,
				field("last"),
			]),
		);
		const values = freeze({ mode: false });
		const fields = definition.fields ?? [];
		expect(getVisibleFields(definition, values).map((item) => item.id)).toEqual(
			["first", "mode", "last"],
		);
		expect(isFieldVisible(fields[1], values, fields)).toBe(false);
		expect(values).toEqual({ mode: false });
	});
});

describe("unified database visibility — VIS-03", () => {
	const definition = panel(databaseFields);

	it.each([
		["ID", { mode: false }],
		["property", { "postgres.aurora": false }],
		["nested", { postgres: { aurora: false } }],
	])(
		"standalone mode via %s shows RDS instance and hides Aurora capacity",
		(_label, values) => {
			const visible = getVisibleFields(definition, values).map(
				(item) => item.id,
			);
			expect(visible).toContain("instance");
			expect(visible).not.toContain("capacity");
		},
	);

	it("Aurora capacity zero remains visible and resolvable", () => {
		const values = { postgres: { aurora: true, min_capacity: 0 } };
		const visible = getVisibleFields(definition, values).map((item) => item.id);
		expect(visible).toContain("capacity");
		expect(visible).not.toContain("instance");
		expect(resolveFieldValue(values, databaseFields[2])).toBe(0);
	});
});
