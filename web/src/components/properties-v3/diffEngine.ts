/**
 * properties-v3 — pure diff engine.
 *
 * Computes typed pending changes between baseline and draft values.
 *
 * Exclusions: currently-invisible fields, readonly (RO/RUNTIME), telemetry,
 * derived, action, and locked set-once fields (CREATE after creation).
 *
 * Collection identity is explicit: `identityKeys` for array collections,
 * `recordKeys: true` for map-style collections. Items without a declared
 * identity are compared whole. Key renames produce remove+add.
 *
 * Value-vs-membership asymmetry (CollectionDiffRules): e.g. an SSM parameter
 * VALUE edit is restart-to-observe while add/remove is rolling-restart;
 * plaintext task env vars force rolling-restart for ALL operations.
 *
 * `applyChanges` performs an OPERATION-LEVEL baseline merge for one
 * persistence channel — other-channel drafts stay untouched because only
 * the submitted channel's operations are applied.
 *
 * Security: every change carries a STRUCTURED literal address (`address`),
 * applied with own-property traversal only and guarded against prototype
 * PATH traversal (`__proto__`/`prototype`/`constructor` as path segments
 * throw). Literal RECORD keys (map entries) are data, not traversal — they are
 * read/written via own-property operations (Object.defineProperty /
 * Reflect.deleteProperty), so a record key of `__proto__` is stored as a
 * plain own entry and can never mutate Object.prototype. Collection keys
 * and row identities are never interpolated into dotted paths, so keys
 * containing `.`, `|`, `=`, `[`, `]` are safe. Invalid addresses produce
 * explicit errors — never a silent successful no-op.
 */

import {
	aggregateImpacts,
	cloneValue,
	collectAffectedResources,
	deepEqual,
	getImpactForMutability,
	getPersistenceChannel,
	isDiffableValueKind,
	isFieldLocked,
	resolveFieldLocation,
	resolveFieldValue,
} from "./helpers";
import type {
	AffectedResource,
	ArrayRowIdentityPair,
	ChangeAddress,
	ChannelSummary,
	CollectionColumnSpec,
	DiffOperation,
	FieldValueType,
	PersistenceChannel,
	PropertyChange,
	PropertyDiffError,
	PropertyDiffResult,
	PropertyFieldDefinition,
	PropertyPanelContext,
	PropertyPanelDefinition,
} from "./types";
import { getAllFields, getVisibleFields } from "./visibilityEngine";

export interface ComputeDiffOptions {
	/** Default affected resource for the node (from the panel instance). */
	resource?: AffectedResource;
}

/** Keys that must NEVER be traversed or written — explicit error, no pollution. */
const DANGEROUS_KEYS: ReadonlySet<string> = new Set([
	"__proto__",
	"prototype",
	"constructor",
]);

function assertSafeSegment(segment: string): void {
	if (DANGEROUS_KEYS.has(segment)) {
		throw new Error(
			`applyChanges: refusing to traverse prototype path segment "${segment}"`,
		);
	}
}

/**
 * Compute the pending diff between baseline and draft.
 * Pure: returns new objects, never mutates inputs.
 */
export function computeDiff(
	definition: PropertyPanelDefinition,
	baseline: Record<string, unknown>,
	draft: Record<string, unknown>,
	context: PropertyPanelContext,
	options: ComputeDiffOptions = {},
): PropertyDiffResult {
	const fields = getAllFields(definition);
	const visible = new Set(
		getVisibleFields(definition, draft).map((field) => field.id),
	);
	const changes: PropertyChange[] = [];
	const validationErrors: PropertyDiffError[] = [];

	for (const field of fields) {
		// Diff scope matches visibility: invisible fields never enter the payload.
		if (!visible.has(field.id)) continue;
		if (!isDiffableValueKind(field.valueKind)) continue; // AWS/telemetry/derived/action
		// RO/RUNTIME are excluded by MUTABILITY regardless of value kind.
		if (field.mutability === "RO" || field.mutability === "RUNTIME") continue;
		if (isFieldLocked(field, context.lifecycleState)) continue; // set-once after creation

		const before = resolveFieldValue(baseline, field);
		const after = resolveFieldValue(draft, field);

		// Validate ALL visible editable eligible fields BEFORE the equality
		// skip: a required field left missing (e.g. ECR local→cross_account
		// without a source account) must block its channel even when unchanged.
		collectValidationErrors(field, after, validationErrors);

		if (deepEqual(before, after)) continue;

		// Merge location: where the value actually lives (field.id key, flat
		// alias, or nested alias path) — falls back to the primary property path.
		const location =
			resolveFieldLocation(baseline, field) ??
			resolveFieldLocation(draft, field);
		const containerPath = location ? location.key : primaryPropertyPath(field);
		// Structured storage root: a FLAT own key is one literal segment (keys
		// may legitimately contain dots); a nested alias is its schema-declared
		// dotted path split into segments (schema strings, never user data).
		const flatOwn =
			!!location &&
			(Object.hasOwn(baseline, location.key) ||
				Object.hasOwn(draft, location.key));
		const rootSegments = flatOwn ? [containerPath] : containerPath.split(".");

		if (field.collection) {
			changes.push(
				...diffCollection(
					field,
					rootSegments,
					containerPath,
					before,
					after,
					options,
				),
			);
		} else {
			const operation: DiffOperation =
				before === undefined ? "add" : after === undefined ? "remove" : "set";
			changes.push(
				makeChange(
					field,
					containerPath,
					undefined,
					operation,
					before,
					after,
					options,
					{},
					{ path: rootSegments },
				),
			);
		}
	}

	return summarize(changes, validationErrors);
}

/* ---------------------------------------------------------------------------
 * Collection diffs
 * ------------------------------------------------------------------------- */

interface RowIdentity {
	key: string;
	pairs: ArrayRowIdentityPair[];
	record: Record<string, unknown>;
}

/**
 * Row identity encoding (`k=v|k2=v2`) is REVERSIBLE and collision-safe:
 * - `\`, `|`, `=`, `[`, `]` are backslash-escaped inside keys and string
 *   values, so values containing the delimiters round-trip exactly.
 * - Numbers are prefixed `\#`, booleans `\#b` — markers an escaped string can
 *   never produce (every literal `\` in a string is doubled), so `{a: 1}` and
 *   `{a: "1"}` never share an identity key and matching is strict (===).
 * - Non-primitive identity values (objects, arrays, NaN, null, undefined)
 *   yield no identity — the collection falls back to whole-value comparison
 *   instead of colliding via String coercion ("[object Object]").
 *
 * The typed `pairs` are carried on every array-row change as the structured
 * `rowIdentity` address, so applyChanges never re-parses the display string.
 */
const IDENTITY_ESCAPE = "\\";

function encodeIdentityString(raw: string): string {
	return raw.replace(/[\\|=[\]]/g, (ch) => IDENTITY_ESCAPE + ch);
}

function encodeIdentityValue(value: unknown): string | null {
	if (typeof value === "string") return encodeIdentityString(value);
	if (typeof value === "number") {
		if (!Number.isFinite(value)) return null;
		return `${IDENTITY_ESCAPE}#${value}`;
	}
	if (typeof value === "boolean") {
		return `${IDENTITY_ESCAPE}#b${value}`;
	}
	return null; // unsupported identity value → row compared whole
}

function arrayRowIdentity(
	record: unknown,
	identityKeys: readonly string[] | undefined,
): RowIdentity | null {
	if (record === null || typeof record !== "object" || Array.isArray(record))
		return null;
	const entry = record as Record<string, unknown>;
	if (!identityKeys || identityKeys.length === 0) return null; // compared whole
	const parts: string[] = [];
	const pairs: ArrayRowIdentityPair[] = [];
	for (const key of identityKeys) {
		const raw = entry[key];
		const encodedValue = encodeIdentityValue(raw);
		if (encodedValue === null) return null; // identity unusable → compared whole
		parts.push(`${encodeIdentityString(key)}=${encodedValue}`);
		pairs.push({
			key,
			value: raw as string | number | boolean,
		});
	}
	return { key: parts.join("|"), pairs, record: entry };
}

function diffCollection(
	field: PropertyFieldDefinition,
	rootSegments: readonly string[],
	pathRoot: string,
	before: unknown,
	after: unknown,
	options: ComputeDiffOptions,
): PropertyChange[] {
	const spec = field.collection;
	if (!spec) return [];
	const changes: PropertyChange[] = [];

	const isArray = (value: unknown): value is unknown[] => Array.isArray(value);

	// Map-style (record) collections.
	if (spec.recordKeys) {
		const beforeRecord = toRecord(before);
		const afterRecord = toRecord(after);
		const keys = new Set([
			...Object.keys(beforeRecord),
			...Object.keys(afterRecord),
		]);
		for (const key of keys) {
			const had = Object.hasOwn(beforeRecord, key);
			const has = Object.hasOwn(afterRecord, key);
			const path = `${pathRoot}.${key}`;
			// The record key is addressed LITERALLY (may contain dots/=/|).
			const rowAddress: ChangeAddress = { path: rootSegments, recordKey: key };
			if (had && !has) {
				changes.push(
					makeChange(
						field,
						path,
						key,
						"row-remove",
						beforeRecord[key],
						undefined,
						options,
						{ membership: true },
						rowAddress,
					),
				);
			} else if (!had && has) {
				changes.push(
					makeChange(
						field,
						path,
						key,
						"row-add",
						undefined,
						afterRecord[key],
						options,
						{ membership: true },
						rowAddress,
					),
				);
			} else if (!deepEqual(beforeRecord[key], afterRecord[key])) {
				// Per-property row diff so value-vs-membership impacts apply per cell.
				changes.push(
					...diffRowProperties(
						field,
						path,
						key,
						beforeRecord[key],
						afterRecord[key],
						options,
						{ path: rootSegments, recordKey: key },
					),
				);
			}
		}
		return changes;
	}

	// Array collections with declared identity. Without a usable identity for
	// EVERY row, the collection is compared whole (single set) — no positional
	// guessing, per the frozen contract.
	if (isArray(before) && isArray(after)) {
		const identityUsable =
			!!spec.identityKeys &&
			spec.identityKeys.length > 0 &&
			[...before, ...after].every(
				(item) => arrayRowIdentity(item, spec.identityKeys) !== null,
			);
		if (!identityUsable) {
			if (!deepEqual(before, after)) {
				changes.push(
					makeChange(
						field,
						pathRoot,
						undefined,
						"set",
						before,
						after,
						options,
						{},
						{ path: rootSegments },
					),
				);
			}
			return changes;
		}
		const beforeRows = new Map<string, Record<string, unknown>>();
		const beforeIdentities = new Map<string, ArrayRowIdentityPair[]>();
		for (const item of before) {
			const identity = arrayRowIdentity(item, spec.identityKeys);
			if (identity) {
				beforeRows.set(identity.key, identity.record);
				beforeIdentities.set(identity.key, identity.pairs);
			}
		}
		const afterRows = new Map<string, Record<string, unknown>>();
		const afterIdentities = new Map<string, ArrayRowIdentityPair[]>();
		for (const item of after) {
			const identity = arrayRowIdentity(item, spec.identityKeys);
			if (identity) {
				afterRows.set(identity.key, identity.record);
				afterIdentities.set(identity.key, identity.pairs);
			}
		}

		for (const [key, record] of beforeRows) {
			const path = `${pathRoot}[${key}]`;
			if (!afterRows.has(key)) {
				// Key rename or removal: remove+add, never a positional "update".
				changes.push(
					makeChange(
						field,
						path,
						key,
						"row-remove",
						record,
						undefined,
						options,
						{ membership: true },
						{ path: rootSegments, rowIdentity: beforeIdentities.get(key) },
					),
				);
			} else if (!deepEqual(record, afterRows.get(key))) {
				const updated = afterRows.get(key);
				if (updated !== undefined) {
					changes.push(
						...diffRowProperties(field, path, key, record, updated, options, {
							path: rootSegments,
							rowIdentity: afterIdentities.get(key),
						}),
					);
				}
			}
		}
		for (const [key, record] of afterRows) {
			if (!beforeRows.has(key)) {
				const path = `${pathRoot}[${key}]`;
				changes.push(
					makeChange(
						field,
						path,
						key,
						"row-add",
						undefined,
						record,
						options,
						{
							membership: true,
						},
						{ path: rootSegments, rowIdentity: afterIdentities.get(key) },
					),
				);
			}
		}
		return changes;
	}

	// Whole-value change (e.g. array ↔ non-array).
	const operation: DiffOperation =
		before === undefined ? "add" : after === undefined ? "remove" : "set";
	changes.push(
		makeChange(
			field,
			pathRoot,
			undefined,
			operation,
			before,
			after,
			options,
			{},
			{ path: rootSegments },
		),
	);
	return changes;
}

/** Address of the row a per-cell diff applies to. */
interface RowAddressBase {
	path: readonly string[];
	recordKey?: string;
	rowIdentity?: readonly ArrayRowIdentityPair[];
}

function diffRowProperties(
	field: PropertyFieldDefinition,
	rowPath: string,
	rowKey: string | undefined,
	before: unknown,
	after: unknown,
	options: ComputeDiffOptions,
	rowAddress: RowAddressBase,
): PropertyChange[] {
	const changes: PropertyChange[] = [];
	const beforeRecord = toRecord(before);
	const afterRecord = toRecord(after);
	const keys = new Set([
		...Object.keys(beforeRecord),
		...Object.keys(afterRecord),
	]);
	let anyPropertyDiff = false;
	for (const key of keys) {
		const had = Object.hasOwn(beforeRecord, key);
		const has = Object.hasOwn(afterRecord, key);
		if (had === has && deepEqual(beforeRecord[key], afterRecord[key])) continue;
		anyPropertyDiff = true;
		const path = `${rowPath}.${key}`;
		// Cell address: row address + the literal cell key. Cell add/remove
		// (row-add/row-remove WITH a cell address) mutate only that cell —
		// whole-row insert/splice is reserved for membership changes.
		const cellAddress: ChangeAddress = { ...rowAddress, cellPath: [key] };
		if (had && !has) {
			changes.push(
				makeChange(
					field,
					path,
					rowKey,
					"row-remove",
					beforeRecord[key],
					undefined,
					options,
					{ property: key },
					cellAddress,
				),
			);
		} else if (!had && has) {
			changes.push(
				makeChange(
					field,
					path,
					rowKey,
					"row-add",
					undefined,
					afterRecord[key],
					options,
					{ property: key },
					cellAddress,
				),
			);
		} else {
			changes.push(
				makeChange(
					field,
					path,
					rowKey,
					"row-update",
					beforeRecord[key],
					afterRecord[key],
					options,
					{ property: key },
					cellAddress,
				),
			);
		}
	}
	if (!anyPropertyDiff && !deepEqual(before, after)) {
		// Non-record row values (rare, e.g. scalar record-map rows) — one
		// whole-row update addressed structurally by record key / row identity.
		changes.push(
			makeChange(
				field,
				rowPath,
				rowKey,
				"row-update",
				before,
				after,
				options,
				{},
				{ ...rowAddress },
			),
		);
	}
	return changes;
}

/* ---------------------------------------------------------------------------
 * Change construction with impact/channel resolution
 * ------------------------------------------------------------------------- */

interface ChangeSemantics {
	/** True for add/remove of a whole row (membership). */
	membership?: boolean;
	/** Row property name for per-cell changes. */
	property?: string;
}

function makeChange(
	field: PropertyFieldDefinition,
	path: string,
	rowKey: string | undefined,
	operation: DiffOperation,
	before: unknown,
	after: unknown,
	options: ComputeDiffOptions,
	semantics: ChangeSemantics = {},
	address: ChangeAddress,
): PropertyChange {
	const rules = field.collection?.diffRules;
	const fieldImpact =
		getImpactForMutability(field.mutability) ?? "applies-on-deploy";
	let impact = fieldImpact;
	let channel = getPersistenceChannel(field);

	if (rules) {
		if (rules.forceImpact) {
			// e.g. plaintext task env vars: ALL operations are rolling-restart.
			impact = rules.forceImpact;
		} else if (semantics.membership) {
			// Whole-row add/remove: membership change (task definition scope).
			impact = rules.membershipImpact ?? fieldImpact;
			channel = rules.membershipChannel ?? "configuration";
		} else if (
			semantics.property &&
			rules.valueProperties?.includes(semantics.property)
		) {
			// e.g. SSM parameter `value` edit → restart-to-observe.
			impact = rules.valueEditImpact ?? fieldImpact;
			if (rules.valueEditChannel) channel = rules.valueEditChannel;
		}
		// Any other cell edit (e.g. parameter `description`) keeps the
		// field-level impact and channel.
	}

	return {
		fieldId: field.id,
		propertyPath: path,
		rowKey,
		address,
		operation,
		before: cloneValue(before),
		after: cloneValue(after),
		impact,
		persistenceChannel: channel,
		affectedResources: affectedFor(field, options),
	};
}

function affectedFor(
	field: PropertyFieldDefinition,
	options: ComputeDiffOptions,
): AffectedResource[] {
	if (field.affectedResources && field.affectedResources.length > 0) {
		return field.affectedResources.map((r) => ({ ...r }));
	}
	return options.resource ? [{ ...options.resource }] : [];
}

function primaryPropertyPath(field: PropertyFieldDefinition): string {
	const raw = field.property;
	return typeof raw === "string" ? raw : raw[0];
}

function toRecord(value: unknown): Record<string, unknown> {
	if (value === null || typeof value !== "object" || Array.isArray(value))
		return {};
	return value as Record<string, unknown>;
}

/* ---------------------------------------------------------------------------
 * Validation (per-channel, recoverable)
 * ------------------------------------------------------------------------- */

function collectValidationErrors(
	field: PropertyFieldDefinition,
	after: unknown,
	errors: PropertyDiffError[],
): void {
	const rules = field.validation;
	const path = primaryPropertyPath(field);
	const channel = getPersistenceChannel(field);
	const fail = (message: string) => {
		errors.push({
			fieldId: field.id,
			propertyPath: path,
			persistenceChannel: channel,
			message,
		});
	};
	if (
		rules?.required &&
		(after === undefined || after === null || after === "")
	) {
		fail(`${field.label} is required`);
		return;
	}
	if (rules) {
		if (
			typeof after === "string" &&
			rules.pattern &&
			!new RegExp(rules.pattern).test(after)
		) {
			fail(`${field.label} must match ${rules.pattern}`);
		}
		if (typeof after === "number") {
			if (rules.min !== undefined && after < rules.min) {
				fail(`${field.label} must be at least ${rules.min}`);
			}
			if (rules.max !== undefined && after > rules.max) {
				fail(`${field.label} must be at most ${rules.max}`);
			}
		}
		if (typeof after === "string") {
			if (rules.minLength !== undefined && after.length < rules.minLength) {
				fail(`${field.label} must be at least ${rules.minLength} characters`);
			}
			if (rules.maxLength !== undefined && after.length > rules.maxLength) {
				fail(`${field.label} must be at most ${rules.maxLength} characters`);
			}
		}
		if (rules.validate) {
			const message = rules.validate(after === undefined ? null : after);
			if (message) fail(message);
		}
	}
	// Typed-editor metadata: declared field/column value types are validated on
	// the pending draft so invalid raw text (e.g. unparseable JSON) blocks the
	// channel. `0`, `false`, `""` and `[]` are VALID values — never treated as
	// missing here.
	if (field.valueType) {
		const message = checkValueType(field.label, after, field.valueType);
		if (message) fail(message);
	}
	const columns = field.collection?.columns;
	if (columns && columns.length > 0) {
		collectColumnValidationErrors(field, after, columns, fail);
	}
}

/**
 * Structural check of a value against a declared FieldValueType. Returns an
 * error message or null. Lenient about string-encoding of NUMBERS only (raw
 * editor text) — booleans must be actual booleans (no "true"/"false" strings,
 * which are malformed payloads); invalid JSON text and mismatched element
 * types are errors.
 */
function checkValueType(
	label: string,
	value: unknown,
	valueType: FieldValueType,
): string | null {
	switch (valueType) {
		case "string":
			return typeof value === "string" ? null : `${label} must be text`;
		case "number":
			return typeof value === "number" && Number.isFinite(value)
				? null
				: typeof value === "string" &&
						value.trim() !== "" &&
						Number.isFinite(Number(value))
					? null
					: `${label} must be a number`;
		case "boolean":
			// STRICT: only real booleans. "true"/"false" strings are malformed
			// payloads, not editor text — UI controls submit actual booleans.
			return typeof value === "boolean" ? null : `${label} must be a boolean`;
		case "string-array":
			if (!Array.isArray(value))
				return `${label} must be a list of text values`;
			return value.every((item) => typeof item === "string")
				? null
				: `${label} must be a list of text values`;
		case "number-array":
			if (!Array.isArray(value)) return `${label} must be a list of numbers`;
			return value.every((item) =>
				typeof item === "number"
					? Number.isFinite(item)
					: typeof item === "string" &&
						item.trim() !== "" &&
						Number.isFinite(Number(item)),
			)
				? null
				: `${label} must be a list of numbers`;
		case "json": {
			if (typeof value !== "string") return null; // structured values pass
			try {
				JSON.parse(value);
				return null;
			} catch {
				return `${label} must be valid JSON`;
			}
		}
	}
}

function collectColumnValidationErrors(
	field: PropertyFieldDefinition,
	after: unknown,
	columns: readonly CollectionColumnSpec[],
	fail: (message: string) => void,
): void {
	const validateRowRecord = (
		rowLabel: string,
		record: Record<string, unknown>,
	) => {
		for (const column of columns) {
			const present =
				Object.hasOwn(record, column.key) &&
				record[column.key] !== undefined &&
				record[column.key] !== null;
			if (!present) {
				// Missing OPTIONAL array columns are allowed; required ones fail.
				if (column.required) {
					fail(`${rowLabel}: ${columnLabel(column)} is required`);
				}
				continue;
			}
			const message = checkValueType(
				`${rowLabel}: ${columnLabel(column)}`,
				record[column.key],
				column.valueType,
			);
			if (message) fail(message);
		}
	};
	const columnLabel = (column: CollectionColumnSpec): string =>
		column.label ?? column.key;

	if (Array.isArray(after)) {
		after.forEach((row, index) => {
			const rowLabel = `${field.label} row ${index + 1}`;
			if (row === null || typeof row !== "object" || Array.isArray(row)) {
				fail(`${rowLabel} must be a record`);
				return;
			}
			validateRowRecord(rowLabel, row as Record<string, unknown>);
		});
		return;
	}
	if (after !== null && typeof after === "object") {
		for (const [key, row] of Object.entries(after as Record<string, unknown>)) {
			const rowLabel = `${field.label} entry "${key}"`;
			if (row !== null && typeof row === "object" && !Array.isArray(row)) {
				validateRowRecord(rowLabel, row as Record<string, unknown>);
			} else {
				// Scalar record-map value: validated against the single declared
				// column when the schema says so (string defaults are accepted per
				// the column's declared valueType, e.g. numeric strings for numbers).
				const scalarColumn = columns.find((c) => c.required) ?? columns[0];
				if (row === undefined || row === null || row === "") {
					if (scalarColumn.required) {
						fail(`${rowLabel}: ${columnLabel(scalarColumn)} is required`);
					}
					continue;
				}
				const message = checkValueType(
					`${rowLabel}: ${columnLabel(scalarColumn)}`,
					row,
					scalarColumn.valueType,
				);
				if (message) fail(message);
			}
		}
	}
}

/* ---------------------------------------------------------------------------
 * Summary
 * ------------------------------------------------------------------------- */

function summarize(
	changes: PropertyChange[],
	validationErrors: PropertyDiffError[],
): PropertyDiffResult {
	const channelSummaries: Partial<Record<PersistenceChannel, ChannelSummary>> =
		{};
	const dirtyChannels = new Set<PersistenceChannel>();
	for (const change of changes) {
		dirtyChannels.add(change.persistenceChannel);
		let summary = channelSummaries[change.persistenceChannel];
		if (!summary) {
			summary = { changeCount: 0, worstImpact: null };
			channelSummaries[change.persistenceChannel] = summary;
		}
		summary.changeCount += 1;
		summary.worstImpact = aggregateImpacts(
			[summary.worstImpact, change.impact].filter(
				(i): i is Exclude<typeof i, null> => i !== null,
			),
		);
	}
	const worstImpact = aggregateImpacts(changes.map((c) => c.impact));
	const destructive = changes.some((c) => c.impact === "recreates-data-loss");
	return {
		changes,
		dirtyChannels: [...dirtyChannels],
		channelSummaries,
		worstImpact,
		destructive,
		affectedResources: collectAffectedResources(changes),
		validationErrors,
	};
}

/* ---------------------------------------------------------------------------
 * Operation-level baseline merge (multi-channel safe)
 * ------------------------------------------------------------------------- */

/**
 * Path addressing rules for applyChanges:
 *
 * STRUCTURED addresses (`change.address`, always produced by computeDiff):
 * - `path` segments are literal own-property keys walked verbatim — a key
 *   containing dots is ONE segment, never re-split, never re-parsed.
 * - `recordKey` addresses a map-collection key literally — including
 *   prototype-sensitive keys like `__proto__`, which are written as own data
 *   properties (defineProperty) and never touch the prototype chain.
 * - `rowIdentity` addresses an array row by typed identity pairs (strict ===
 *   matching, no string parsing).
 * - `cellPath` addresses cells INSIDE the addressed row: `row-add`/`row-remove`
 *   with a cellPath insert/remove that CELL only; whole-row insert/splice
 *   happens exclusively for membership changes (no cellPath).
 * - All traversal is own-property only; `__proto__`/`prototype`/`constructor`
 *   PATH segments throw (record KEYs are the safe exception above).
 *   Unresolvable structured addresses throw — never a silent
 *   no-op. (Exception: membership `row-remove` of an already-absent row is an
 *   idempotent no-op.)
 *
 * LEGACY paths (changes without an address) keep readable dotted addressing
 * with the same guards: flat own key first, dotted walk with optional
 * escape-aware `[k=v|k2=v2]` row selector, dangerous segments throw, and an
 * unresolvable row selector for add/update throws.
 */

function getValueAt(root: Record<string, unknown>, path: string): unknown {
	if (Object.hasOwn(root, path)) return root[path];
	let current: unknown = root;
	for (const segment of path.split(".")) {
		if (segment === "") continue;
		assertSafeSegment(segment);
		if (current === null || typeof current !== "object") return undefined;
		const record = current as Record<string, unknown>;
		if (!Object.hasOwn(record, segment)) return undefined; // own-property traversal only
		current = record[segment];
	}
	return current;
}

function setValueAt(
	root: Record<string, unknown>,
	path: string,
	value: unknown,
): void {
	if (Object.hasOwn(root, path)) {
		assertSafeSegment(path);
		if (value === undefined) delete root[path];
		else root[path] = cloneValue(value);
		return;
	}
	const segments = path.split(".").filter(Boolean);
	let current: Record<string, unknown> = root;
	for (let i = 0; i < segments.length - 1; i += 1) {
		const key = segments[i];
		assertSafeSegment(key);
		const next = current[key];
		if (next === null || typeof next !== "object") {
			current[key] = {};
		}
		current = current[key] as Record<string, unknown>;
	}
	const last = segments[segments.length - 1];
	if (last !== undefined) assertSafeSegment(last);
	if (value === undefined) {
		delete current[last];
	} else {
		current[last] = cloneValue(value);
	}
}

/* --- structured address application ------------------------------------- */

/**
 * Resolve the value at literal path segments with own-property traversal,
 * creating INTERMEDIATE plain objects as needed. The final segment is
 * returned as-is (may be an array container, an object, or undefined);
 * non-object intermediates throw.
 */
function resolveStructuredPath(
	root: Record<string, unknown>,
	segments: readonly string[],
): unknown {
	let current: unknown = root;
	for (let i = 0; i < segments.length; i += 1) {
		const segment = segments[i];
		assertSafeSegment(segment);
		const record = current as Record<string, unknown>;
		const next: unknown = Object.hasOwn(record, segment)
			? record[segment]
			: undefined;
		if (i === segments.length - 1) return next;
		if (next === null || typeof next !== "object" || Array.isArray(next)) {
			if (next !== undefined) {
				throw new Error(
					`applyChanges: structured path segment "${segment}" addresses a non-object value`,
				);
			}
			record[segment] = {};
		}
		current = record[segment];
	}
	throw new Error("applyChanges: empty structured path");
}

/** Resolve (or create) an OBJECT at the literal path. */
function objectAtPath(
	root: Record<string, unknown>,
	segments: readonly string[],
): Record<string, unknown> {
	if (segments.length === 0) return root;
	const parent = objectAtPath(root, segments.slice(0, -1));
	const leaf = segments[segments.length - 1];
	assertSafeSegment(leaf);
	const existing: unknown = Object.hasOwn(parent, leaf)
		? parent[leaf]
		: undefined;
	if (existing === undefined) {
		parent[leaf] = {};
		return parent[leaf] as Record<string, unknown>;
	}
	if (
		existing === null ||
		typeof existing !== "object" ||
		Array.isArray(existing)
	) {
		throw new Error(
			`applyChanges: structured path segment "${leaf}" addresses a non-object value`,
		);
	}
	return existing as Record<string, unknown>;
}

function setCellBySegments(
	row: Record<string, unknown>,
	cellPath: readonly string[],
	value: unknown,
): void {
	let current: Record<string, unknown> = row;
	for (let i = 0; i < cellPath.length - 1; i += 1) {
		const key = cellPath[i];
		assertSafeSegment(key);
		const next: unknown = Object.hasOwn(current, key)
			? current[key]
			: undefined;
		if (next === null || typeof next !== "object" || Array.isArray(next)) {
			if (next !== undefined) {
				throw new Error(
					`applyChanges: cell path segment "${key}" addresses a non-object value`,
				);
			}
			current[key] = {};
		}
		current = current[key] as Record<string, unknown>;
	}
	const last = cellPath[cellPath.length - 1];
	if (last === undefined) {
		throw new Error("applyChanges: empty cell path");
	}
	assertSafeSegment(last);
	if (value === undefined) delete current[last];
	else current[last] = cloneValue(value);
}

/**
 * Own-property record-key write. Assignment (`container[key] = value`) would
 * invoke setters — and for the literal record key `__proto__` would mutate the
 * prototype instead of creating an own key. defineProperty always creates a
 * plain own data property (enumerable, writable, configurable), so literal
 * record keys such as `__proto__`, `constructor` or `prototype` are stored
 * safely as map entries.
 */
function defineOwnRecordEntry(
	container: Record<string, unknown>,
	key: string,
	value: unknown,
): void {
	Object.defineProperty(container, key, {
		value,
		writable: true,
		enumerable: true,
		configurable: true,
	});
}

/**
 * Own-property record-key delete. Only removes an OWN data property; a missing
 * key (including `__proto__` when not stored as an own key) is an idempotent
 * no-op — the prototype chain is never touched.
 */
function deleteOwnRecordEntry(
	container: Record<string, unknown>,
	key: string,
): void {
	if (Object.hasOwn(container, key)) Reflect.deleteProperty(container, key);
}

function rowIdentityLabel(
	rowIdentity: readonly ArrayRowIdentityPair[],
): string {
	return rowIdentity
		.map((pair) => `${pair.key}=${String(pair.value)}`)
		.join("|");
}

function findRowByIdentity(
	rows: readonly unknown[],
	rowIdentity: readonly ArrayRowIdentityPair[],
): number {
	return rows.findIndex((row) => {
		if (row === null || typeof row !== "object" || Array.isArray(row))
			return false;
		const record = row as Record<string, unknown>;
		return rowIdentity.every((pair) => record[pair.key] === pair.value);
	});
}

/** Apply a change via its structured literal address. Throws on invalid addresses. */
function applyStructured(
	root: Record<string, unknown>,
	change: PropertyChange,
): void {
	const address = change.address;
	if (!address) throw new Error("applyChanges: missing structured address");
	if (
		address.path.some(
			(segment) => typeof segment !== "string" || segment === "",
		)
	) {
		throw new Error(`applyChanges: invalid address path for ${change.fieldId}`);
	}

	// Array-collection row addressing (typed identity, no string parsing).
	if (address.rowIdentity) {
		const container = resolveStructuredPath(root, address.path);
		if (!Array.isArray(container)) {
			throw new Error(
				`applyChanges: address for ${change.fieldId} expects an array container`,
			);
		}
		const label = rowIdentityLabel(address.rowIdentity);
		const index = findRowByIdentity(container, address.rowIdentity);
		if (address.cellPath && address.cellPath.length > 0) {
			// CELL operation inside an existing row: insert/update/remove only
			// that cell — the row itself is never inserted or spliced.
			if (index === -1) {
				throw new Error(
					`applyChanges: no row matches identity "${label}" for cell ${change.operation} of ${change.fieldId}`,
				);
			}
			const row = container[index] as Record<string, unknown>;
			setCellBySegments(
				row,
				address.cellPath,
				change.operation === "row-remove" || change.operation === "remove"
					? undefined
					: change.after,
			);
			return;
		}
		// Membership operation: whole-row insert/splice only.
		if (change.operation === "row-add" || change.operation === "add") {
			if (index === -1) container.push(cloneValue(change.after));
			return;
		}
		if (index === -1) return; // already-removed row: idempotent no-op
		if (change.operation === "row-remove" || change.operation === "remove") {
			container.splice(index, 1);
			return;
		}
		if (change.operation === "row-update" || change.operation === "set") {
			container.splice(index, 1);
			if (change.after !== undefined) container.push(cloneValue(change.after));
			return;
		}
		throw new Error(
			`applyChanges: unsupported operation "${change.operation}" for row address of ${change.fieldId}`,
		);
	}

	// Map-collection record-key addressing (literal key, may contain dots/=/
	// `__proto__` — keys are data, never traversed path segments; reads and
	// writes are own-property only via defineProperty/Reflect.deleteProperty).
	if (address.recordKey !== undefined) {
		const container = objectAtPath(root, address.path);
		if (address.cellPath && address.cellPath.length > 0) {
			const row: unknown = Object.hasOwn(container, address.recordKey)
				? container[address.recordKey]
				: undefined;
			if (row === undefined) {
				throw new Error(
					`applyChanges: record "${address.recordKey}" not found for cell ${change.operation} of ${change.fieldId}`,
				);
			}
			if (row === null || typeof row !== "object" || Array.isArray(row)) {
				throw new Error(
					`applyChanges: record "${address.recordKey}" of ${change.fieldId} is not an object`,
				);
			}
			setCellBySegments(
				row as Record<string, unknown>,
				address.cellPath,
				change.operation === "row-remove" || change.operation === "remove"
					? undefined
					: change.after,
			);
			return;
		}
		switch (change.operation) {
			case "row-remove":
			case "remove":
				deleteOwnRecordEntry(container, address.recordKey);
				return;
			default:
				if (change.after === undefined) {
					deleteOwnRecordEntry(container, address.recordKey);
				} else {
					defineOwnRecordEntry(
						container,
						address.recordKey,
						cloneValue(change.after),
					);
				}
				return;
		}
	}

	// Plain field address: `path` addresses the value itself.
	if (address.cellPath && address.cellPath.length > 0) {
		throw new Error(
			`applyChanges: ambiguous address (field path + cellPath) for ${change.fieldId}`,
		);
	}
	const leaf = address.path[address.path.length - 1];
	if (leaf === undefined) {
		throw new Error(`applyChanges: empty address path for ${change.fieldId}`);
	}
	assertSafeSegment(leaf);
	const parent = objectAtPath(root, address.path.slice(0, -1));
	if (change.operation === "remove" || change.operation === "row-remove") {
		delete parent[leaf];
	} else if (change.after === undefined) {
		delete parent[leaf];
	} else {
		parent[leaf] = cloneValue(change.after);
	}
}

/* --- legacy (string-path) addressing ------------------------------------ */

interface RowKeyPair {
	key: string;
	value: unknown;
}

/**
 * Parse a rowKey ("k=v|k2=v2") back into typed key/value pairs, honoring
 * backslash escapes. Escape pairs (`\x`) are kept verbatim so type markers
 * survive; decodeIdentityValue does the final unescape/typing. Returns null
 * when the encoding is malformed.
 */
function parseRowKey(rowKey: string): RowKeyPair[] | null {
	const pairs: RowKeyPair[] = [];
	let key = "";
	let rawValue = "";
	let haveKey = false;
	const pushPair = () => {
		pairs.push({
			key: unescapeIdentity(key),
			value: decodeIdentityValue(rawValue),
		});
		key = "";
		rawValue = "";
		haveKey = false;
	};
	for (let i = 0; i < rowKey.length; i += 1) {
		const ch = rowKey[i];
		if (ch === IDENTITY_ESCAPE) {
			const next = rowKey[i + 1];
			if (next === undefined) return null; // dangling escape
			if (haveKey) rawValue += ch + next;
			else key += ch + next;
			i += 1;
			continue;
		}
		if (ch === "=" && !haveKey) {
			haveKey = true;
			continue;
		}
		if (ch === "|") {
			if (!haveKey) return null; // delimiter in a key position
			pushPair();
			continue;
		}
		if (haveKey) rawValue += ch;
		else key += ch;
	}
	if (haveKey) pushPair();
	else if (pairs.length === 0) return null; // no pairs at all
	return pairs;
}

/** Restore number/boolean type markers, then unescape string content. */
function decodeIdentityValue(rawValue: string): unknown {
	if (rawValue.startsWith(`${IDENTITY_ESCAPE}#b`)) {
		return unescapeIdentity(rawValue.slice(3)) === "true";
	}
	if (rawValue.startsWith(`${IDENTITY_ESCAPE}#`)) {
		return Number(rawValue.slice(2));
	}
	return unescapeIdentity(rawValue);
}

/** Collapse escape pairs (`\x` → `x`, including `\\` → `\`). */
function unescapeIdentity(escaped: string): string {
	let out = "";
	for (let i = 0; i < escaped.length; i += 1) {
		if (escaped[i] === IDENTITY_ESCAPE && i + 1 < escaped.length) {
			out += escaped[i + 1];
			i += 1;
		} else {
			out += escaped[i];
		}
	}
	return out;
}

/**
 * Find the index of an array row whose own properties strictly match a
 * rowKey ("k=v|k2=v2"). Matching is type-exact (===) because the encoding
 * is injective — no String coercion, no 1-vs-"1" collisions.
 */
function findRowByRowKey(rows: readonly unknown[], rowKey: string): number {
	const pairs = parseRowKey(rowKey);
	if (!pairs) return -1;
	return rows.findIndex((row) => {
		if (row === null || typeof row !== "object" || Array.isArray(row))
			return false;
		const record = row as Record<string, unknown>;
		return pairs.every(
			(pair) => pair.value !== undefined && record[pair.key] === pair.value,
		);
	});
}

interface BracketAddress {
	containerPath: string;
	rowKey: string;
	remainder: string | undefined;
}

/**
 * Split `container[k=v|k2=v2].remainder` escape-aware: the closing `]` is
 * the first UNESCAPED one, so rowKey values containing `]` or `.` round-trip.
 */
function parseBracket(path: string): BracketAddress | null {
	const open = path.indexOf("[");
	if (open <= 0) return null;
	const inner = path.slice(open + 1);
	let close = -1;
	for (let i = 0; i < inner.length; i += 1) {
		if (inner[i] === IDENTITY_ESCAPE) {
			i += 1; // skip escaped char — never a delimiter
			continue;
		}
		if (inner[i] === "]") {
			close = i;
			break;
		}
	}
	if (close <= 0) return null;
	const containerPath = path.slice(0, open);
	const rowKey = inner.slice(0, close);
	const rest = inner.slice(close + 1);
	if (rest === "") return { containerPath, rowKey, remainder: undefined };
	if (!rest.startsWith(".")) return null;
	return { containerPath, rowKey, remainder: rest.slice(1) };
}

/**
 * Apply one legacy change inside an identity-addressed array row. Returns
 * true when handled. Unresolvable addresses for add/update THROW — never a
 * successful no-op; an already-removed membership row is an idempotent no-op.
 */
function applyBracketed(
	root: Record<string, unknown>,
	change: PropertyChange,
): boolean {
	const address = parseBracket(change.propertyPath);
	if (!address) return false;
	const container = getValueAt(root, address.containerPath);
	if (!Array.isArray(container)) {
		throw new Error(
			`applyChanges: legacy path "${change.propertyPath}" does not address an array`,
		);
	}
	const index = findRowByRowKey(container, address.rowKey);
	const hasCell = address.remainder !== undefined;
	if (change.operation === "row-add" && !hasCell) {
		if (index === -1) container.push(cloneValue(change.after));
		return true;
	}
	if (index === -1) {
		if (change.operation === "row-remove" && !hasCell) return true; // idempotent
		throw new Error(
			`applyChanges: no row matches rowKey "${address.rowKey}" for ${change.operation} of ${change.fieldId}`,
		);
	}
	if (hasCell) {
		// CELL operation: apply the remainder within the row; the row itself
		// is never inserted or spliced for cell changes.
		const row = container[index] as Record<string, unknown>;
		setValueAt(
			row,
			address.remainder ?? "",
			change.operation === "row-remove" || change.operation === "remove"
				? undefined
				: change.after,
		);
		return true;
	}
	switch (change.operation) {
		case "row-remove":
		case "remove":
			container.splice(index, 1);
			return true;
		case "set":
		case "add":
		case "row-update": {
			if (change.after !== undefined) {
				container[index] = cloneValue(change.after);
			} else {
				container.splice(index, 1);
			}
			return true;
		}
	}
	return true;
}

/**
 * Apply ONLY the changes of one persistence channel to a baseline, at the
 * operation level (each set/add/remove/row op merged individually). Returns a
 * new baseline object; the input baseline is untouched, and other channels'
 * drafts remain dirty because their operations were never applied here.
 *
 * Structured addresses (`change.address`) are applied literally with
 * own-property traversal and prototype-path guards; legacy string paths keep
 * working for externally constructed simple changes. Invalid addresses throw.
 */
export function applyChanges(
	baseline: Record<string, unknown>,
	changes: readonly PropertyChange[],
	channel: PersistenceChannel,
): Record<string, unknown> {
	const next = cloneValue(baseline);
	const scoped = changes.filter(
		(change) => change.persistenceChannel === channel,
	);
	for (const change of scoped) {
		if (change.address) {
			applyStructured(next, change);
			continue;
		}
		if (applyBracketed(next, change)) continue;
		switch (change.operation) {
			case "remove":
			case "row-remove":
				setValueAt(next, change.propertyPath, undefined);
				break;
			default:
				setValueAt(next, change.propertyPath, change.after);
				break;
		}
	}
	return next;
}

export { resolveFieldValue };
