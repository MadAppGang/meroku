/**
 * Archetype: telemetry-metrics — Operations health & metrics, Logs viewer.
 * Every reading is timestamped and demo/fixture-labeled or explicitly empty;
 * no fabricated live health.
 */

import { PropertyEmptyState } from "../../properties/PropertyPanelPrimitives";
import { Grid, Stack } from "../primitives";
import { renderField } from "./renderFields";
import type { ArchetypeSectionProps } from "./SectionRenderContext";

export function TelemetryMetricsSection({
	visibleFieldIds,
	ctx,
}: ArchetypeSectionProps) {
	const hasReadings = visibleFieldIds.some((id) =>
		ctx.telemetryByField.has(id),
	);
	return (
		<Stack gap={8}>
			{hasReadings ? (
				<Grid gap={8}>
					{visibleFieldIds.map((fieldId) => renderField(fieldId, ctx))}
				</Grid>
			) : (
				<PropertyEmptyState
					title="No telemetry available"
					description="This node has no connected telemetry source. Demo adapters can supply fixture readings."
				/>
			)}
		</Stack>
	);
}
