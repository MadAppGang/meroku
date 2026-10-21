/**
 * properties-v3 — field control core.
 *
 * Maps one PropertyFieldDefinition (schema) to a library control with its
 * impact badge. This is the only place control types are interpreted;
 * archetypes compose these rows without touching control details.
 *
 * Rules encoded here:
 * - Every editable / canvas-only field carries an impact badge.
 * - Locked set-once fields (CREATE after creation) render readonly rows.
 * - Readonly values are value rows with resolution placeholders (copy
 *   feedback via the library runtime parameter), never disabled inputs.
 * - Telemetry rows are timestamped and demo/empty labeled.
 * - Actions are one-shot buttons with pending/failure states.
 */

import {
	PropertyActionButton,
	PropertyCheckboxField,
	PropertyEditableField,
	PropertyFieldFrame,
	PropertySelectField,
	PropertyTextareaField,
} from "../../properties/PropertyPanelPrimitives";
import { Input } from "../../ui/input";
import { getImpactForMutability } from "../helpers";
import type {
	CollectionSpec,
	PropertyFieldDefinition,
	ResolutionState,
	TelemetryReading,
} from "../types";
import { PropertyImpactBadge } from "./PropertyImpactBadge";
import {
	type KeyValueColumn,
	PropertyKeyValueTable,
} from "./PropertyKeyValueTable";
import { PropertyLogViewerField } from "./PropertyLogViewerField";
import { PropertyTelemetry } from "./PropertyTelemetry";
import { PropertyRuntimeParameter } from "./RuntimePrimitives";
import { Grid, Row } from "./Structural";

export type FieldActionState = "idle" | "pending" | "error";

/* ---------------------------------------------------------------------------
 * Typed value metadata (core `FieldValueType`, applied additively here via
 * structural types so the UI compiles independently of the core rollout).
 * ------------------------------------------------------------------------- */

type FieldValueTypeMetadata =
	| "string"
	| "number"
	| "boolean"
	| "string-array"
	| "number-array"
	| "json";

interface TypedColumnMetadata {
	key: string;
	label?: string;
	valueType?: FieldValueTypeMetadata;
	required?: boolean;
}

type TypedField = PropertyFieldDefinition & {
	valueType?: FieldValueTypeMetadata;
};
type TypedCollection = CollectionSpec & {
	columns?: readonly TypedColumnMetadata[];
};

const isPlainObjectCheck = (entry: unknown): entry is Record<string, unknown> =>
	entry !== null && typeof entry === "object" && !Array.isArray(entry);

/**
 * Parse raw textarea/text input against a DECLARED value type. Invalid raw
 * text is preserved (returned as-is with ok:false) so validation can surface
 * it and the save stays blocked — nothing is silently cleared per keystroke.
 */
function parseTypedText(
	valueType: FieldValueTypeMetadata | undefined,
	raw: string,
): { value: unknown; ok: boolean } {
	if (raw.trim() === "") return { value: undefined, ok: true };
	switch (valueType) {
		case "number": {
			const parsed = Number(raw);
			return Number.isFinite(parsed)
				? { value: parsed, ok: true }
				: { value: raw, ok: false };
		}
		case "string-array":
		case "number-array":
		case "json": {
			try {
				const parsed: unknown = JSON.parse(raw);
				if (valueType === "string-array") {
					const ok =
						Array.isArray(parsed) &&
						parsed.every((item) => typeof item === "string");
					return ok ? { value: parsed, ok: true } : { value: raw, ok: false };
				}
				if (valueType === "number-array") {
					const ok =
						Array.isArray(parsed) &&
						parsed.every(
							(item) => typeof item === "number" && Number.isFinite(item),
						);
					return ok ? { value: parsed, ok: true } : { value: raw, ok: false };
				}
				// json: declared JSON object or array.
				return isPlainObjectCheck(parsed) || Array.isArray(parsed)
					? { value: parsed, ok: true }
					: { value: raw, ok: false };
			} catch {
				return { value: raw, ok: false };
			}
		}
		default:
			return { value: raw, ok: true };
	}
}

/** Table input type for a declared column value type. */
function cellInputType(
	valueType: FieldValueTypeMetadata | undefined,
): KeyValueColumn["type"] {
	if (valueType === "number") return "number";
	if (valueType === "boolean") return "boolean";
	return "text";
}

export interface FieldControlProps {
	field: PropertyFieldDefinition;
	value: unknown;
	baselineValue: unknown;
	theme: "dark" | "light";
	locked: boolean;
	disabled?: boolean;
	error?: string;
	onChange: (value: unknown) => void;
	telemetry?: TelemetryReading;
	/** Fixture/demo markers come from the adapter (Storybook) layer. */
	demoTelemetry?: boolean;
	/** Real CloudWatch destination (log viewer fields only; on-demand, no AWS calls). */
	logDestination?: string | null;
	onAction?: (fieldId: string) => void;
	actionState?: FieldActionState;
	actionError?: string;
}

function badgeFor(field: PropertyFieldDefinition, locked: boolean) {
	if (locked) return <PropertyImpactBadge impact="set-once" compact />;
	const impact = getImpactForMutability(field.mutability);
	if (!impact) return null;
	return <PropertyImpactBadge impact={impact} compact />;
}

function toDisplay(value: unknown): string {
	if (value === undefined || value === null) return "";
	if (typeof value === "object") return JSON.stringify(value);
	return String(value);
}

/**
 * Columns for key-value tables. DECLARED collection.columns metadata wins
 * (key, label, typed cells); otherwise columns are derived from the declared
 * collection schema (identityKeys, recordKeys, diffRules.valueProperties)
 * plus the row shapes present in baseline/draft. Identity columns are created
 * even for EMPTY collections (e.g. identityKeys: ["name"] yields a "Name"
 * column). No label regex — labels are metadata or schema keys with a
 * capitalized first letter. A scalar recordKeys map always gets BOTH a Key
 * and a Value column, never key-only. When a recordKeys map DECLARES columns,
 * the metadata scopes the editable value surface: the record-key column still
 * auto-renders, but other undeclared row keys do NOT become columns (they stay
 * preserved data).
 */
function tableColumns(
	rows: readonly Record<string, unknown>[],
	field: PropertyFieldDefinition,
): KeyValueColumn[] {
	const typedSpec = field.collection as TypedCollection | undefined;
	const declared = typedSpec?.columns;
	const spec = field.collection;
	const keys: string[] = [];
	const add = (key: string) => {
		if (!keys.includes(key)) keys.push(key);
	};
	if (declared && declared.length > 0) {
		const columns: KeyValueColumn[] = declared.map((column) => ({
			key: column.key,
			label:
				column.label ??
				column.key.charAt(0).toUpperCase() + column.key.slice(1),
			type: cellInputType(column.valueType),
			// Preserve declared structured types so table cells parse JSON edits
			// back into native arrays/objects (e.g. S3 CORS allowed_methods).
			valueType:
				column.valueType === "string-array" ||
				column.valueType === "number-array" ||
				column.valueType === "json"
					? column.valueType
					: undefined,
			mono: column.key === "value",
		}));
		// Preserve any row keys the metadata does not cover (append-only). For
		// recordKeys maps the declared columns scope the EDITABLE value surface:
		// only the record-key identity column auto-renders — undeclared value
		// keys (e.g. create-only SSM `type` metadata) stay as preserved data in
		// the row round-trip, never an editable column.
		const recordKeyOnly = Boolean(spec?.recordKeys);
		const appended = new Set<string>();
		for (const row of rows) {
			for (const key of Object.keys(row)) {
				if (declared.some((column) => column.key === key)) continue;
				if (recordKeyOnly && key !== "key") continue;
				if (appended.has(key)) continue;
				appended.add(key);
				columns.push({
					key,
					label: key.charAt(0).toUpperCase() + key.slice(1),
					mono: key === "value",
				});
			}
		}
		// The auto-added record-key identity column is an implicit column, not a
		// declared one, so it always leads the table (Key | Value), preserving the
		// declared order of any explicitly declared columns.
		const identity = columns.findIndex(
			(column) =>
				column.key === "key" && !declared.some((d) => d.key === "key"),
		);
		if (identity > 0) columns.unshift(columns.splice(identity, 1)[0]);
		return columns;
	}
	// Declared identity first so the schema owns column order for empty tables.
	if (spec?.recordKeys) add("key");
	for (const identityKey of spec?.identityKeys ?? []) add(identityKey);
	for (const property of spec?.diffRules?.valueProperties ?? []) add(property);
	for (const row of rows) {
		for (const key of Object.keys(row)) add(key);
	}
	if (keys.length === 0) {
		add("key");
		add("value");
	}
	// Scalar record maps need BOTH columns — a key-only empty row can never
	// be completed or diffed.
	if (spec?.recordKeys && keys.length === 1 && keys[0] === "key") add("value");
	return keys.map((key) => ({
		key,
		label: key.charAt(0).toUpperCase() + key.slice(1),
		mono: key === "value",
	}));
}

/** Map-style (recordKeys) collections ↔ editable row records round-trip. */
function toEditableRows(value: unknown): Record<string, unknown>[] {
	if (Array.isArray(value)) {
		return value.map((row) =>
			row !== null && typeof row === "object" && !Array.isArray(row)
				? (row as Record<string, unknown>)
				: { value: row },
		);
	}
	if (value !== null && typeof value === "object") {
		return Object.entries(value as Record<string, unknown>).map(
			([entryKey, entry]) => ({
				key: entryKey,
				...(entry !== null && typeof entry === "object" && !Array.isArray(entry)
					? (entry as Record<string, unknown>)
					: { value: entry }),
			}),
		);
	}
	return [];
}

const isPlainObject = (entry: unknown): entry is Record<string, unknown> =>
	entry !== null && typeof entry === "object" && !Array.isArray(entry);

/**
 * Rebuild the record map from edited rows while preserving each entry's
 * scalar vs object shape:
 * 1. Same key as before → reuse that entry's shape (value edits, re-adds).
 * 2. Unmatched key at a position that held an entry → that entry's shape
 *    (renamed keys keep their original shape even though the NEW key has
 *    no baseline lookup hit).
 * 3. Brand-new row (no key match, no positional match) → object shape when
 *    the schema declares valueProperties, scalar otherwise.
 * Without this, scalar entries like {KEY: "value"} would round-trip into
 * {KEY: {value: "value"}} after any rename/edit.
 */
function fromEditableRows(
	rows: readonly Record<string, unknown>[],
	recordKeys: boolean,
	previousValue: unknown,
	objectShapeDefault: boolean,
): Record<string, unknown>[] | Record<string, unknown> {
	if (!recordKeys) return [...rows];
	const previousEntries: [string, unknown][] = isPlainObject(previousValue)
		? Object.entries(previousValue)
		: [];
	const map: Record<string, unknown> = {};
	rows.forEach((row, index) => {
		const { key, ...rest } = row;
		const keyText = String(key ?? "");
		const matchedIndex = previousEntries.findIndex(([k]) => k === keyText);
		const source =
			matchedIndex >= 0
				? previousEntries[matchedIndex][1]
				: index < previousEntries.length
					? previousEntries[index][1]
					: undefined;
		const objectShape =
			source === undefined ? objectShapeDefault : isPlainObject(source);
		map[keyText] = objectShape ? rest : rest.value;
	});
	return map;
}

export function FieldControl({
	field,
	value,
	baselineValue,
	theme,
	locked,
	disabled = false,
	error,
	onChange,
	telemetry,
	demoTelemetry = false,
	logDestination,
	onAction,
	actionState = "idle",
	actionError,
}: FieldControlProps) {
	const badge = badgeFor(field, locked);
	const header = badge ? (
		<div className="mp-v3-field-item__header">{badge}</div>
	) : null;
	const note = field.description ? (
		<div className="mp-v3-field-item__note">{field.description}</div>
	) : null;

	// Encapsulated on-demand log viewer (logRole: viewer): never a generic
	// telemetry row; empty until an explicit stream action, no AWS calls.
	if (field.logRole === "viewer") {
		return (
			<PropertyLogViewerField
				label={field.label}
				destination={logDestination ?? null}
				demo={demoTelemetry}
			/>
		);
	}

	// Telemetry rows: timestamped, demo-labeled, never fabricated.
	if (
		field.valueKind === "telemetry" ||
		field.controlType === "telemetry-row"
	) {
		return (
			<PropertyTelemetry
				label={field.label}
				value={toDisplay(value)}
				timestamp={telemetry?.timestamp}
				resolution={telemetry?.resolution ?? "known"}
				demo={demoTelemetry}
			/>
		);
	}

	// One-shot actions with pending/failure states.
	if (field.controlType === "action-button" || field.valueKind === "action") {
		const label =
			actionState === "pending"
				? `${field.label}…`
				: actionState === "error"
					? `${field.label} — failed, retry`
					: field.label;
		return (
			<div className="mp-v3-field-item">
				{header}
				<Row gap={6}>
					<PropertyActionButton
						disabled={disabled || actionState === "pending"}
						onClick={() => onAction?.(field.id)}
					>
						{label}
					</PropertyActionButton>
				</Row>
				{actionState === "error" && actionError ? (
					<div className="mp-v3-field-item__note">{actionError}</div>
				) : null}
				{note}
			</div>
		);
	}

	// Readonly / locked / AWS-assigned / derived: value row with resolution.
	const isReadonlyRow =
		locked ||
		field.valueKind === "aws-assigned" ||
		field.valueKind === "derived" ||
		field.controlType === "readonly-value";
	if (isReadonlyRow) {
		const resolution: ResolutionState =
			field.resolution ??
			(field.valueKind === "aws-assigned" && value === undefined
				? "provisioned"
				: "known");
		return (
			<PropertyRuntimeParameter
				label={field.label}
				value={toDisplay(locked && value === undefined ? baselineValue : value)}
				mono
				resolution={resolution}
			/>
		);
	}

	// Editable collections (arrays AND recordKeys maps round-trip correctly).
	if (field.controlType === "key-value-table") {
		const recordKeys = Boolean(field.collection?.recordKeys);
		const rows = toEditableRows(value);
		const baselineRows = toEditableRows(baselineValue);
		const columns = tableColumns([...baselineRows, ...rows], field);
		const impact =
			field.collection?.diffRules?.forceImpact ??
			field.collection?.diffRules?.membershipImpact ??
			getImpactForMutability(field.mutability) ??
			undefined;
		return (
			<div className="mp-v3-field-item">
				{header}
				<PropertyKeyValueTable
					id={field.id}
					label={field.label}
					columns={columns}
					rows={rows}
					disabled={disabled}
					/* Per-row badge wiring removed: the remove-action cell now shows
					   the membership badge (same as Add), and the field-level
					   value-edit badge stays on the field header (badgeFor). */
					membershipImpact={
						field.collection?.diffRules?.membershipImpact ?? impact ?? undefined
					}
					onChange={(nextRows) =>
						onChange(
							fromEditableRows(
								nextRows,
								recordKeys,
								value,
								Boolean(field.collection?.diffRules?.valueProperties?.length),
							),
						)
					}
				/>
				{note}
			</div>
		);
	}

	// Boolean toggle.
	if (field.controlType === "boolean") {
		return (
			<div className="mp-v3-field-item">
				{header}
				<PropertyCheckboxField
					item={{
						id: field.id,
						label: field.label,
						checked: Boolean(value),
						description: field.description,
						disabled: disabled,
						onCheckedChange: (checked) => onChange(checked),
					}}
					span="half"
				/>
			</div>
		);
	}

	// Select.
	if (field.controlType === "select") {
		return (
			<div className="mp-v3-field-item">
				{header}
				<PropertySelectField
					id={field.id}
					label={field.label}
					value={toDisplay(value)}
					options={(field.options ?? []).map((option) => ({
						label: option.label,
						value: option.value,
					}))}
					theme={theme}
					placeholder={field.placeholder}
					disabled={disabled}
					onChange={(next) => onChange(next)}
				/>
				{error ? <div className="mp-v3-field-item__note">{error}</div> : null}
				{note}
			</div>
		);
	}

	// Textarea.
	if (field.controlType === "textarea") {
		const typed = (field as TypedField).valueType;
		return (
			<div className="mp-v3-field-item">
				{header}
				<PropertyTextareaField
					id={field.id}
					label={field.label}
					value={toDisplay(value)}
					placeholder={field.placeholder}
					disabled={disabled}
					onChange={(next) => {
						if (
							typed === "json" ||
							typed === "string-array" ||
							typed === "number-array"
						) {
							// Declared structured value: parse the edit; keep INVALID
							// raw text so validation surfaces it and save stays blocked
							// (nothing cleared per keystroke).
							const { value: parsed } = parseTypedText(typed, next);
							onChange(parsed);
							return;
						}
						onChange(next);
					}}
				/>
				{error ? <div className="mp-v3-field-item__note">{error}</div> : null}
				{note}
			</div>
		);
	}

	// Layout dimensions: schema supplies two number fields; single-value
	// numeric fields fall through to the shared editable control.
	if (field.controlType === "layout-dimensions") {
		const dimensions =
			value !== null && typeof value === "object" && !Array.isArray(value)
				? (value as Record<string, number>)
				: {};
		return (
			<Grid gap={4}>
				{(["width", "height"] as const).map((axis) => (
					<PropertyEditableField
						key={axis}
						id={`${field.id}-${axis}`}
						label={`${field.label} ${axis}`}
						type="number"
						value={dimensions[axis] ?? 0}
						disabled={disabled}
						onChange={(next) => onChange({ ...dimensions, [axis]: next })}
					/>
				))}
			</Grid>
		);
	}

	// Number fields: absent value renders as an empty CONTROLLED input —
	// never a fabricated 0. Explicit 0 (valid for e.g. Aurora min_capacity)
	// is preserved. Clearing emits undefined (explicit absence); non-numeric
	// input is clamped to undefined rather than NaN.
	if (field.controlType === "number") {
		const display = value === undefined || value === null ? "" : String(value);
		return (
			<div className="mp-v3-field-item">
				{header}
				<PropertyFieldFrame label={field.label} span="half" kind="number">
					<Input
						id={field.id}
						aria-label={field.label}
						aria-invalid={Boolean(error)}
						type="number"
						value={display}
						placeholder={field.placeholder}
						disabled={disabled}
						className={`mp-control ${field.collection ? "mp-mono" : ""}`}
						onChange={(event) => {
							const raw = event.target.value;
							if (raw.trim() === "") {
								onChange(undefined);
								return;
							}
							const parsed = Number(raw);
							onChange(Number.isFinite(parsed) ? parsed : undefined);
						}}
					/>
					{error && <span className="mp-compact-field__error">{error}</span>}
				</PropertyFieldFrame>
				{note}
			</div>
		);
	}

	// Default: text editable field.
	const textValueType = (field as TypedField).valueType;
	return (
		<div className="mp-v3-field-item">
			{header}
			<PropertyEditableField
				id={field.id}
				label={field.label}
				type="text"
				value={toDisplay(value)}
				placeholder={field.placeholder}
				mono={Boolean(field.collection)}
				disabled={disabled}
				error={error}
				onChange={(next) => {
					if (textValueType === "number") {
						// Declared numeric value in a text control: save as a number;
						// invalid raw text is preserved for validation.
						if (typeof next === "number") {
							onChange(next);
							return;
						}
						const { value: parsed } = parseTypedText("number", next);
						onChange(parsed);
						return;
					}
					onChange(next);
				}}
			/>
			{note}
		</div>
	);
}
