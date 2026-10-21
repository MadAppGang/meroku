/**
 * properties-v3 — shared per-field renderer used by archetype composites.
 * Resolves a field id to a FieldControl with validation error and lock state.
 */

import { resolveFieldValue } from "../helpers";
import { FieldControl } from "../primitives";
import type { SectionRenderContext } from "./SectionRenderContext";

export function renderField(fieldId: string, ctx: SectionRenderContext) {
	const field = ctx.fieldsById.get(fieldId);
	if (!field) return null;
	const errors = ctx.validationErrorsByField.get(fieldId);
	return (
		<FieldControl
			key={fieldId}
			field={field}
			value={ctx.draft[field.id]}
			baselineValue={resolveFieldValue(ctx.baseline, field)}
			theme={ctx.theme}
			locked={ctx.lockedFieldIds.has(fieldId)}
			disabled={ctx.submitting}
			error={errors?.map((error) => error.message).join("; ")}
			telemetry={ctx.telemetryByField.get(fieldId)}
			demoTelemetry={ctx.demoTelemetry}
			logDestination={ctx.instance.cloudWatchDestination}
			onChange={(value) => ctx.setValue(field, value)}
			onAction={ctx.runAction}
			actionState={ctx.actionState[fieldId] ?? "idle"}
			actionError={ctx.actionError[fieldId]}
		/>
	);
}
