/**
 * Archetype: standard — generic schema-driven form grid.
 * Every field renders through FieldControl (badge + library control).
 */

import { Grid, Stack } from "../primitives";
import { renderField } from "./renderFields";
import type { ArchetypeSectionProps } from "./SectionRenderContext";

export function StandardFormSection({
	visibleFieldIds,
	ctx,
}: ArchetypeSectionProps) {
	return (
		<Stack gap={10}>
			<Grid gap={8}>
				{visibleFieldIds.map((fieldId) => renderField(fieldId, ctx))}
			</Grid>
		</Stack>
	);
}
