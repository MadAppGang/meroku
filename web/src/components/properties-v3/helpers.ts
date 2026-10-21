/**
 * properties-v3 — pure helpers shared by the engines.
 *
 * Depends only on types.ts. No React, no DOM, no imports from v1/v2 panels.
 */

import type {
	AffectedResource,
	LifecycleState,
	MutabilityClass,
	PersistenceChannel,
	PropertyChange,
	PropertyDiffResult,
	PropertyFieldDefinition,
	PropertyImpact,
	PropertyImpactMetadata,
	ValueKind,
} from "./types";

/* ---------------------------------------------------------------------------
 * Mutability → impact / channel mapping (deterministic, from requirements §4)
 * ------------------------------------------------------------------------- */

export const MUTABILITY_IMPACT: Readonly<
	Record<MutabilityClass, PropertyImpact | null>
> = {
	LIVE: "applies-now",
	"LIVE (action)": "applies-now",
	"LIVE-RESTART": "restart-to-observe",
	"TF-DEPLOY": "rolling-restart",
	TF: "applies-on-deploy",
	"TF-REPLACE": "recreates-data-loss",
	"CREATE / TF-REPLACE": "recreates-data-loss",
	"TF-REPLACE / TF-DEPLOY": "recreates-data-loss",
	CREATE: "set-once",
	"LIVE, create-only": "set-once",
	RO: null,
	RUNTIME: null,
	// Canvas-only UI preference: internal applies-now, never destructive.
	UI: "applies-now",
};

export function getImpactForMutability(
	mutability: MutabilityClass,
): PropertyImpact | null {
	return MUTABILITY_IMPACT[mutability] ?? null;
}

export function getPersistenceChannel(
	field: PropertyFieldDefinition,
): PersistenceChannel {
	if (field.persistenceChannel) return field.persistenceChannel;
	if (field.valueKind === "canvas-only" || field.mutability === "UI")
		return "layout";
	if (
		field.mutability === "LIVE" ||
		field.mutability === "LIVE (action)" ||
		field.mutability === "LIVE-RESTART" ||
		field.mutability === "LIVE, create-only"
	) {
		return "live";
	}
	return "configuration";
}

/* ---------------------------------------------------------------------------
 * Impact aggregation (worst consequence; set-once ≠ destruction)
 * ------------------------------------------------------------------------- */

const IMPACT_SEVERITY: Readonly<
	Record<Exclude<PropertyImpact, "set-once">, number>
> = {
	"recreates-data-loss": 5,
	"rolling-restart": 4,
	"applies-on-deploy": 3,
	"restart-to-observe": 2,
	"applies-now": 1,
};

/**
 * Aggregate maximum consequence: replacement > rolling deployment >
 * deploy-in-place > restart-to-observe > immediate. `set-once` is treated as
 * initial configuration, never as destruction, and never outranks real
 * consequences of dirty changes.
 */
export function aggregateImpacts(
	impacts: readonly PropertyImpact[],
): PropertyImpact | null {
	let worst: PropertyImpact | null = null;
	let worstSeverity = 0;
	for (const impact of impacts) {
		if (impact === "set-once") {
			if (worst === null) worst = "set-once";
			continue;
		}
		const severity = IMPACT_SEVERITY[impact];
		if (severity > worstSeverity) {
			worst = impact;
			worstSeverity = severity;
		}
	}
	return worst;
}

/* ---------------------------------------------------------------------------
 * Impact metadata (pure data for later UI badges)
 * ------------------------------------------------------------------------- */

const IMPACT_METADATA: Readonly<
	Record<PropertyImpact, PropertyImpactMetadata>
> = {
	"applies-now": {
		impact: "applies-now",
		label: "Applies now",
		tone: "success",
		summary: "Applies immediately to the running resource",
		isDestructive: false,
		requiresConfirmation: false,
	},
	"restart-to-observe": {
		impact: "restart-to-observe",
		label: "Restart to observe",
		tone: "info",
		summary: "Value is live in AWS; restart the container to observe it",
		isDestructive: false,
		requiresConfirmation: false,
	},
	"rolling-restart": {
		impact: "rolling-restart",
		label: "Rolling restart",
		tone: "warning",
		summary: "Rolling deployment on next apply — new task definition revision",
		isDestructive: false,
		requiresConfirmation: false,
	},
	"applies-on-deploy": {
		impact: "applies-on-deploy",
		label: "Applies on deploy",
		tone: "neutral",
		summary: "Applied on the next infrastructure deployment",
		isDestructive: false,
		requiresConfirmation: false,
	},
	"recreates-data-loss": {
		impact: "recreates-data-loss",
		label: "Recreates · data loss",
		tone: "danger",
		summary: "The next deploy may recreate resources and lose data",
		isDestructive: true,
		requiresConfirmation: true,
	},
	"set-once": {
		impact: "set-once",
		label: "Set once",
		tone: "locked",
		summary: "Initial configuration — locked after creation",
		isDestructive: false,
		requiresConfirmation: false,
	},
};

export function getImpactMetadata(
	impact: PropertyImpact,
): PropertyImpactMetadata {
	return IMPACT_METADATA[impact];
}

/* ---------------------------------------------------------------------------
 * Field lookup by id and property-key aliases
 * ------------------------------------------------------------------------- */

export function getFieldPropertyAliases(
	field: PropertyFieldDefinition,
): string[] {
	const raw = field.property;
	return typeof raw === "string" ? [raw] : [...raw];
}

function ownValue(source: Record<string, unknown>, key: string): unknown {
	return Object.hasOwn(source, key) ? source[key] : undefined;
}

/** Walk a dot-separated path using own properties only (no prototype access). */
function nestedLookup(source: Record<string, unknown>, path: string): unknown {
	let current: unknown = source;
	for (const segment of path.split(".")) {
		if (current === null || typeof current !== "object") return undefined;
		const record = current as Record<string, unknown>;
		if (!Object.hasOwn(record, segment)) return undefined;
		current = record[segment];
	}
	return current;
}

/**
 * Deterministic alias resolution against a value record:
 * 1. own property `source[field.id]`
 * 2. each declared property alias, in declared order:
 *    a. own property `source[alias]` (flat)
 *    b. nested own-property walk of the alias split on "."
 * Returns `undefined` when the key is absent everywhere. Own-property checks
 * preserve `false`, `0`, `0.0`, and `""` as present, valid values.
 */
export function resolveFieldValue(
	source: Record<string, unknown>,
	field: PropertyFieldDefinition,
): unknown {
	const byId = ownValue(source, field.id);
	if (byId !== undefined) return byId;
	for (const alias of getFieldPropertyAliases(field)) {
		const flat = ownValue(source, alias);
		if (flat !== undefined) return flat;
		const nested = nestedLookup(source, alias);
		if (nested !== undefined) return nested;
	}
	return undefined;
}

/**
 * Where a field's value actually lives in a record: a flat own key (field.id
 * or alias) or a dotted nested alias path. `key` is directly usable by
 * setPath-style merge. Null when absent everywhere. Same precedence as
 * resolveFieldValue.
 */
export function resolveFieldLocation(
	source: Record<string, unknown>,
	field: PropertyFieldDefinition,
): { key: string } | null {
	if (ownValue(source, field.id) !== undefined) return { key: field.id };
	for (const alias of getFieldPropertyAliases(field)) {
		if (ownValue(source, alias) !== undefined) return { key: alias };
		if (nestedLookup(source, alias) !== undefined) return { key: alias };
	}
	return null;
}

/**
 * Resolve a lookup key (a field id OR a property key) using the field catalog.
 * The key matches a field by id, or by any of its declared property aliases;
 * the matched field's full resolution chain (id → aliases in declared order,
 * flat-before-nested) is then applied. Unknown keys fall back to flat/nested
 * lookup on the raw values.
 */
export function resolveLookupKey(
	source: Record<string, unknown>,
	key: string,
	fields: readonly PropertyFieldDefinition[],
): unknown {
	const field =
		fields.find((candidate) => candidate.id === key) ??
		fields.find((candidate) =>
			getFieldPropertyAliases(candidate).includes(key),
		);
	if (field) return resolveFieldValue(source, field);
	const flat = ownValue(source, key);
	if (flat !== undefined) return flat;
	return nestedLookup(source, key);
}

/** Own-property existence check (never resolves inherited properties). */
export function hasOwnKey(
	record: Record<string, unknown>,
	key: string,
): boolean {
	return Object.hasOwn(record, key);
}

export function buildFieldIndex(
	fields: readonly PropertyFieldDefinition[],
): Map<string, PropertyFieldDefinition> {
	return new Map(fields.map((field) => [field.id, field]));
}

/* ---------------------------------------------------------------------------
 * Deep equality (object-key-order insensitive)
 * ------------------------------------------------------------------------- */

export function deepEqual(a: unknown, b: unknown): boolean {
	if (a === b) return true;
	if (typeof a !== typeof b) return false;
	if (
		a === null ||
		b === null ||
		typeof a !== "object" ||
		typeof b !== "object"
	) {
		// NaN handling
		return (
			typeof a === "number" &&
			typeof b === "number" &&
			Number.isNaN(a) &&
			Number.isNaN(b)
		);
	}
	if (Array.isArray(a) || Array.isArray(b)) {
		if (!Array.isArray(a) || !Array.isArray(b) || a.length !== b.length)
			return false;
		return a.every((item, index) => deepEqual(item, b[index]));
	}
	const recordA = a as Record<string, unknown>;
	const recordB = b as Record<string, unknown>;
	const keysA = Object.keys(recordA);
	const keysB = Object.keys(recordB);
	if (keysA.length !== keysB.length) return false;
	return keysA.every(
		(key) =>
			Object.hasOwn(recordB, key) && deepEqual(recordA[key], recordB[key]),
	);
}

/** Structural clone that preserves plain objects/arrays but passes other values through. */
export function cloneValue<T>(value: T): T {
	if (value === null || typeof value !== "object") return value;
	if (Array.isArray(value)) return value.map(cloneValue) as unknown as T;
	const out: Record<string, unknown> = {};
	for (const [key, nested] of Object.entries(
		value as Record<string, unknown>,
	)) {
		out[key] = cloneValue(nested);
	}
	return out as unknown as T;
}

/* ---------------------------------------------------------------------------
 * Dirty-state summary
 * ------------------------------------------------------------------------- */

export function getDirtyState(result: PropertyDiffResult): {
	dirty: boolean;
	channels: Record<PersistenceChannel, boolean>;
	changeCount: number;
	worstImpact: PropertyImpact | null;
} {
	const channels: Record<PersistenceChannel, boolean> = {
		configuration: result.dirtyChannels.includes("configuration"),
		live: result.dirtyChannels.includes("live"),
		layout: result.dirtyChannels.includes("layout"),
	};
	return {
		dirty: result.changes.length > 0,
		channels,
		changeCount: result.changes.length,
		worstImpact: result.worstImpact,
	};
}

/** Resources affected by destructive changes, deduplicated by id, order-stable. */
export function collectAffectedResources(
	changes: readonly PropertyChange[],
): AffectedResource[] {
	const seen = new Set<string>();
	const out: AffectedResource[] = [];
	for (const change of changes) {
		if (change.impact !== "recreates-data-loss") continue;
		for (const resource of change.affectedResources) {
			if (seen.has(resource.id)) continue;
			seen.add(resource.id);
			out.push(resource);
		}
	}
	return out;
}

/* ---------------------------------------------------------------------------
 * Locking
 * ------------------------------------------------------------------------- */

/** A field is diffable: excluded kinds are RO/RUNTIME-derived read paths. */
export function isDiffableValueKind(valueKind: ValueKind): boolean {
	return valueKind === "editable" || valueKind === "canvas-only";
}

/** Set-once fields lock permanently once the resource is created. */
export function isFieldLocked(
	field: PropertyFieldDefinition,
	lifecycleState: LifecycleState,
): boolean {
	if (
		field.mutability === "CREATE" ||
		field.mutability === "LIVE, create-only"
	) {
		return lifecycleState === "created";
	}
	return false;
}
