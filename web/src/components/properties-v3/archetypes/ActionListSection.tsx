/**
 * Archetype: action-list — Operations / Actions.
 * Hosts BOTH one-shot action buttons (pending/failure states) AND the
 * standard editors for LIVE / LIVE-RESTART stored edits (Clarification 3):
 * field kind decides rendering, never the section archetype alone.
 */

import { Stack, Text } from "../primitives";
import { renderField } from "./renderFields";
import type { ArchetypeSectionProps } from "./SectionRenderContext";

export function ActionListSection({
	visibleFieldIds,
	ctx,
}: ArchetypeSectionProps) {
	const actionIds = visibleFieldIds.filter(
		(id) => ctx.fieldsById.get(id)?.valueKind === "action",
	);
	const editorIds = visibleFieldIds.filter(
		(id) => ctx.fieldsById.get(id)?.valueKind !== "action",
	);
	return (
		<Stack gap={10}>
			{editorIds.length > 0 ? (
				<Stack gap={8}>
					<Text variant="muted">
						Live edits below are applied directly to AWS and persisted in the
						live channel.
					</Text>
					{editorIds.map((fieldId) => renderField(fieldId, ctx))}
				</Stack>
			) : null}
			{actionIds.length > 0 ? (
				<Stack gap={6}>
					{actionIds.map((fieldId) => renderField(fieldId, ctx))}
				</Stack>
			) : null}
		</Stack>
	);
}
