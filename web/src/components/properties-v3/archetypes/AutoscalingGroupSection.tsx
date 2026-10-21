/**
 * Archetype: autoscaling-group — schema-controlled capacity editor.
 * The first boolean field is the enable toggle; the composite swaps the
 * disabled summary (Desired count) for the enabled fields (Min/Max/target).
 * Which fields exist is decided by schema visibility, never local state.
 * The toggle is a schema-declared editable field, so it carries its
 * mutability-derived impact badge like every other editable control.
 */

import { getImpactForMutability } from "../helpers";
import { AutoscalingGroupComposite, Grid, Stack } from "../primitives";
import { PropertyImpactBadge } from "../primitives/PropertyImpactBadge";
import { renderField } from "./renderFields";
import type { ArchetypeSectionProps } from "./SectionRenderContext";

export function AutoscalingGroupSection({
	visibleFieldIds,
	ctx,
}: ArchetypeSectionProps) {
	const toggleField =
		visibleFieldIds
			.map((id) => ctx.fieldsById.get(id))
			.find((field) => field?.controlType === "boolean") ?? null;
	const restIds = visibleFieldIds.filter((id) => id !== toggleField?.id);

	// No declared boolean toggle: render the declared fields normally.
	// Never fabricate a default-on autoscaling group.
	if (!toggleField) {
		return (
			<Stack gap={8}>
				<Grid gap={8}>{visibleFieldIds.map((id) => renderField(id, ctx))}</Grid>
			</Stack>
		);
	}

	const enabled = Boolean(ctx.draft[toggleField.id]);
	// Same derivation as FieldControl.badgeFor: locked fields are set-once,
	// otherwise the schema's declared mutability maps to the impact category.
	const locked = ctx.lockedFieldIds.has(toggleField.id);
	const impact = locked
		? "set-once"
		: getImpactForMutability(toggleField.mutability);
	const badge = impact ? <PropertyImpactBadge impact={impact} compact /> : null;

	return (
		<Stack gap={8}>
			<AutoscalingGroupComposite
				enabled={enabled}
				ariaLabel={toggleField.label ?? "Autoscaling"}
				onEnabledChange={(next) => ctx.setValue(toggleField, next)}
				badge={badge}
				enabledFields={
					<Grid gap={8}>{restIds.map((id) => renderField(id, ctx))}</Grid>
				}
				disabledFields={
					<Grid gap={8}>
						{restIds
							.filter((id) => ctx.fieldsById.get(id)?.controlType !== "boolean")
							.map((id) => renderField(id, ctx))}
					</Grid>
				}
			/>
		</Stack>
	);
}
