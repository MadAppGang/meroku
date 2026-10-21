/**
 * properties-v3 — pure visibility engine.
 *
 * Evaluates `visibleWhen` declarations against current values without
 * mutation. Lookup keys may be field IDs or property keys (flat or
 * dot-nested); alias precedence is documented in helpers.ts.
 */

import { deepEqual, resolveFieldValue, resolveLookupKey } from "./helpers";
import type {
	PropertyFieldDefinition,
	PropertyPanelDefinition,
	VisibilityCondition,
	VisibilitySpec,
} from "./types";

/**
 * Evaluate a single condition.
 * - equals / notEquals use deep, key-order-insensitive equality.
 * - exists checks OWN-PROPERTY presence: `false`, `0`, `0.0`, and `""` are
 *   present values; only an absent key (resolved to `undefined`) is missing.
 */
export function evaluateCondition(
	condition: VisibilityCondition,
	values: Record<string, unknown>,
	fields: readonly PropertyFieldDefinition[],
): boolean {
	const actual = resolveLookupKey(values, condition.key, fields);
	switch (condition.kind) {
		case "equals":
			return deepEqual(actual, condition.value);
		case "notEquals":
			return !deepEqual(actual, condition.value);
		case "exists":
			return actual !== undefined;
	}
}

/**
 * Evaluate a visibility spec. A field without a spec is always visible.
 * When both `all` and `any` are declared, both groups must hold (AND).
 */
export function evaluateVisibility(
	spec: VisibilitySpec | undefined,
	values: Record<string, unknown>,
	fields: readonly PropertyFieldDefinition[],
): boolean {
	if (!spec) return true;
	const allOk = spec.all
		? spec.all.every((c) => evaluateCondition(c, values, fields))
		: true;
	const anyOk = spec.any
		? spec.any.some((c) => evaluateCondition(c, values, fields))
		: true;
	return allOk && anyOk;
}

/** Is a single field visible under the current values? Pure, no mutation. */
export function isFieldVisible(
	field: PropertyFieldDefinition,
	values: Record<string, unknown>,
	fields: readonly PropertyFieldDefinition[],
): boolean {
	return evaluateVisibility(field.visibleWhen, values, fields);
}

/** All fields declared on the definition, in declaration order. */
export function getAllFields(
	definition: PropertyPanelDefinition,
): PropertyFieldDefinition[] {
	return [...(definition.fields ?? [])];
}

/** Visible fields of a definition, in declaration order. */
export function getVisibleFields(
	definition: PropertyPanelDefinition,
	values: Record<string, unknown>,
): PropertyFieldDefinition[] {
	const fields = getAllFields(definition);
	return fields.filter((field) => isFieldVisible(field, values, fields));
}

export { resolveFieldValue };
