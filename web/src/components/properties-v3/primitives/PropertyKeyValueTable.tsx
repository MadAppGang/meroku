/**
 * properties-v3 — editable key-value table primitive.
 *
 * Schema-controlled editable collections (task env vars, SSM parameters,
 * trusted ECR accounts). Rows are plain records; membership edits produce
 * add/remove rows in the diff (impact semantics come from the diff engine's
 * CollectionDiffRules, not from this component). Includes a per-row impact
 * badge so every editable cell stays badged.
 *
 * Row identity for React keys is a stable internal ID independent of editable
 * cell contents, so typing never remounts a row or steals input focus.
 */

import { Plus, X } from "lucide-react";
import { useRef } from "react";
import { PropertyActionButton } from "../../properties/PropertyPanelPrimitives";
import type { PropertyImpact } from "../types";
import { PropertyImpactBadge } from "./PropertyImpactBadge";
import { Row } from "./Structural";

export interface KeyValueColumn {
	key: string;
	label: string;
	type?: "text" | "number" | "password" | "boolean";
	/**
	 * Declared structured cell type from the schema. When set, cell edits are
	 * JSON (e.g. `["GET","POST"]`) and valid input is parsed back to a native
	 * array/object so core validation sees the declared type. Invalid raw text
	 * is preserved as a string so validation blocks the save without losing
	 * the draft or input focus.
	 */
	valueType?: "string-array" | "number-array" | "json";
	placeholder?: string;
	/** Wider column rendered with monospace text. */
	mono?: boolean;
}

const isPlainObjectCell = (entry: unknown): entry is Record<string, unknown> =>
	entry !== null && typeof entry === "object" && !Array.isArray(entry);

const isStructuredColumn = (
	column: KeyValueColumn,
): column is KeyValueColumn & {
	valueType: "string-array" | "number-array" | "json";
} => Boolean(column.valueType);

/** Typed default + change conversion so cells save as their DECLARED types. */
function defaultCell(column: KeyValueColumn): unknown {
	if (column.type === "boolean") return false;
	return "";
}

function cellFromEvent(column: KeyValueColumn, raw: string | boolean): unknown {
	if (column.type === "boolean") return Boolean(raw);
	if (column.type === "number") {
		if (typeof raw !== "string" || raw.trim() === "") return undefined;
		const parsed = Number(raw);
		return Number.isFinite(parsed) ? parsed : raw;
	}
	if (isStructuredColumn(column) && typeof raw === "string") {
		if (raw.trim() === "") return undefined;
		try {
			const parsed: unknown = JSON.parse(raw);
			if (column.valueType === "string-array") {
				return Array.isArray(parsed) &&
					parsed.every((item) => typeof item === "string")
					? parsed
					: raw;
			}
			if (column.valueType === "number-array") {
				return Array.isArray(parsed) &&
					parsed.every(
						(item) => typeof item === "number" && Number.isFinite(item),
					)
					? parsed
					: raw;
			}
			return isPlainObjectCell(parsed) || Array.isArray(parsed) ? parsed : raw;
		} catch {
			return raw;
		}
	}
	return raw;
}

/** Display a cell value: structured values serialize to JSON, raw invalid drafts stay as typed. */
function cellDisplay(value: unknown): string {
	if (value === undefined || value === null) return "";
	if (typeof value === "object") return JSON.stringify(value);
	return String(value);
}

export interface PropertyKeyValueTableProps {
	id: string;
	label: string;
	columns: readonly KeyValueColumn[];
	rows: readonly Record<string, unknown>[];
	onChange: (rows: Record<string, unknown>[]) => void;
	disabled?: boolean;
	/** Badge shown on each editable row (value/membership impact). */
	rowImpact?: PropertyImpact;
	/** Badge shown for membership (add/remove) hints; defaults to rowImpact. */
	membershipImpact?: PropertyImpact;
}

export function PropertyKeyValueTable({
	id,
	label,
	columns,
	rows,
	onChange,
	disabled = false,
	rowImpact,
	membershipImpact,
}: PropertyKeyValueTableProps) {
	// Stable per-row ids: allocated once per row slot, never derived from the
	// edited cell values (prevents remount/focus loss on every keystroke).
	const rowIdsRef = useRef<string[]>([]);
	const idCounterRef = useRef(0);
	if (rowIdsRef.current.length !== rows.length) {
		const existing = rowIdsRef.current;
		rowIdsRef.current = rows.map((_, index) => {
			if (existing[index]) return existing[index];
			idCounterRef.current += 1;
			return `${id}-row-${idCounterRef.current}`;
		});
	}
	const rowIds = rowIdsRef.current;

	const updateCell = (rowIndex: number, key: string, value: unknown) => {
		const next = rows.map((row, index) =>
			index === rowIndex ? { ...row, [key]: value } : row,
		);
		onChange(next);
	};

	const removeRow = (rowIndex: number) => {
		onChange(rows.filter((_, index) => index !== rowIndex));
	};

	const addRow = () => {
		const empty: Record<string, unknown> = {};
		for (const column of columns) empty[column.key] = defaultCell(column);
		onChange([...rows, empty]);
	};

	return (
		<table className="mp-v3-kv-table" aria-label={label}>
			<thead>
				<tr>
					{columns.map((column) => (
						<th key={column.key} scope="col">
							{column.label}
						</th>
					))}
					<th scope="col">
						<span>{membershipImpact ? "Add / remove" : "Actions"}</span>
					</th>
				</tr>
			</thead>
			<tbody>
				{rows.map((row, rowIndex) => (
					<tr key={rowIds[rowIndex]}>
						{columns.map((column) => (
							<td key={column.key}>
								{column.type === "boolean" ? (
									<input
										id={`${rowIds[rowIndex]}-${column.key}`}
										aria-label={`${label} row ${rowIndex + 1} ${column.label}`}
										className="mp-v3-kv-table__checkbox"
										type="checkbox"
										checked={Boolean(row[column.key])}
										disabled={disabled}
										onChange={(event) =>
											updateCell(
												rowIndex,
												column.key,
												cellFromEvent(column, event.target.checked),
											)
										}
									/>
								) : (
									<input
										id={`${rowIds[rowIndex]}-${column.key}`}
										aria-label={`${label} row ${rowIndex + 1} ${column.label}`}
										className="mp-control"
										type={column.type === "number" ? "number" : "text"}
										value={cellDisplay(row[column.key])}
										placeholder={column.placeholder}
										disabled={disabled}
										onChange={(event) =>
											updateCell(
												rowIndex,
												column.key,
												cellFromEvent(column, event.target.value),
											)
										}
									/>
								)}
							</td>
						))}
						<td>
							<Row gap={6}>
								{/* This cell only hosts the remove action, so it carries the
								    membership badge (same as the Add footer), not the
								    value-edit badge. */}
								{(() => {
									const actionImpact = membershipImpact ?? rowImpact;
									return actionImpact ? (
										<PropertyImpactBadge impact={actionImpact} compact />
									) : null;
								})()}
								<button
									type="button"
									className="mp-icon-button mp-v3-kv-table__row-remove"
									aria-label={`Remove ${label} row ${rowIndex + 1}`}
									disabled={disabled}
									onClick={() => removeRow(rowIndex)}
								>
									<X />
								</button>
							</Row>
						</td>
					</tr>
				))}
			</tbody>
			<tfoot>
				<tr>
					<td colSpan={columns.length + 1}>
						<PropertyActionButton disabled={disabled} onClick={addRow}>
							<Plus aria-hidden="true" /> Add {label.toLowerCase()} row
						</PropertyActionButton>
						{membershipImpact ? (
							<PropertyImpactBadge impact={membershipImpact} compact />
						) : null}
					</td>
				</tr>
			</tfoot>
		</table>
	);
}
