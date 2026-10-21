/**
 * Archetype: environment-variables — collection editor composite.
 * Per-field help copy is DERIVED from the declared collection diff rules
 * (metadata only): plaintext collections with forceImpact (always a rolling
 * deployment) vs SSM-style value-vs-membership asymmetry (value edits apply
 * after restart; key membership changes require a rolling deployment). No
 * hardcoded blanket statements — fields without diff-rule metadata render
 * without help copy.
 */

import { getImpactMetadata } from "../helpers";
import { Stack, Text } from "../primitives";
import { renderField } from "./renderFields";
import type { ArchetypeSectionProps } from "./SectionRenderContext";

function helpLines({ visibleFieldIds, ctx }: ArchetypeSectionProps): string[] {
	const lines: string[] = [];
	for (const fieldId of visibleFieldIds) {
		const field = ctx.fieldsById.get(fieldId);
		const rules = field?.collection?.diffRules;
		if (!field || !rules) continue;
		if (rules.forceImpact) {
			lines.push(
				`${field.label}: any change — ${getImpactMetadata(rules.forceImpact).summary}.`,
			);
			continue;
		}
		if (rules.valueEditImpact && rules.membershipImpact) {
			lines.push(
				`${field.label}: value edit — ${getImpactMetadata(rules.valueEditImpact).summary}. Key add/remove — ${getImpactMetadata(rules.membershipImpact).summary}.`,
			);
		}
	}
	return lines;
}

export function EnvironmentVariablesSection(props: ArchetypeSectionProps) {
	const lines = helpLines(props);
	return (
		<Stack gap={10}>
			{lines.map((line) => (
				<Text key={line} variant="muted">
					{line}
				</Text>
			))}
			{props.visibleFieldIds.map((fieldId) => renderField(fieldId, props.ctx))}
		</Stack>
	);
}
