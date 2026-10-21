/**
 * Archetype: container-process — image source strategy composite.
 * The first field (schema-declared source strategy select) drives which
 * repository/tag/command controls are visible; the visibility engine (schema)
 * does the swapping, this composite only groups them into a source cluster.
 * Heading owned by the canonical section container.
 */

import { Grid, Stack, Text } from "../primitives";
import { renderField } from "./renderFields";
import type { ArchetypeSectionProps } from "./SectionRenderContext";

export function ContainerProcessSection({
	section,
	visibleFieldIds,
	ctx,
}: ArchetypeSectionProps) {
	const [strategyId, ...restIds] = visibleFieldIds;
	// Container copy only for the canonical container-source section;
	// non-container reuse (metadata choice) gets no container wording.
	const isSourceSection = section.key === "source";
	return (
		<Stack gap={8}>
			{isSourceSection ? (
				<Text variant="muted">
					Image source and entrypoint — the source strategy controls which
					fields apply.
				</Text>
			) : null}
			{strategyId ? renderField(strategyId, ctx) : null}
			{restIds.length > 0 ? (
				<Grid gap={8}>
					{restIds.map((fieldId) => renderField(fieldId, ctx))}
				</Grid>
			) : null}
		</Stack>
	);
}
