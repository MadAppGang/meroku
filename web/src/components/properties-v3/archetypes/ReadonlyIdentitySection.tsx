/**
 * Archetype: readonly-identity — Overview identity/endpoints/relationships.
 * Value rows with resolution placeholders and copy feedback; locked set-once
 * CREATE values render as immutable readonly rows. The heading comes from the
 * canonical section container (never hardcoded here).
 */

import { Stack } from "../primitives";
import { renderField } from "./renderFields";
import type { ArchetypeSectionProps } from "./SectionRenderContext";

export function ReadonlyIdentitySection({
	visibleFieldIds,
	ctx,
}: ArchetypeSectionProps) {
	return (
		<Stack gap={4}>
			{visibleFieldIds.map((fieldId) => renderField(fieldId, ctx))}
		</Stack>
	);
}
