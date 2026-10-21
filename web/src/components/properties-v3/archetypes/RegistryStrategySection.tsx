/**
 * Archetype: registry-strategy — ECR source strategy composite.
 * The strategy select is rendered first; the visibility engine reveals the
 * local or cross-account repository controls beneath it. Heading owned by the
 * canonical section container.
 */

import { Grid, Stack, Text } from "../primitives";
import { renderField } from "./renderFields";
import type { ArchetypeSectionProps } from "./SectionRenderContext";

export function RegistryStrategySection({
	visibleFieldIds,
	ctx,
}: ArchetypeSectionProps) {
	const [strategyId, ...restIds] = visibleFieldIds;
	return (
		<Stack gap={8}>
			<Text variant="muted">Where container images are pulled from.</Text>
			{strategyId ? renderField(strategyId, ctx) : null}
			{restIds.length > 0 ? (
				<Grid gap={8}>
					{restIds.map((fieldId) => renderField(fieldId, ctx))}
				</Grid>
			) : null}
		</Stack>
	);
}
