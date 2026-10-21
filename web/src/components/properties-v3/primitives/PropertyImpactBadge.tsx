/**
 * properties-v3 — impact badge.
 *
 * Renders the six user-facing impact categories from pure metadata
 * (helpers.getImpactMetadata). Every editable field carries one.
 */

import { getImpactMetadata } from "../helpers";
import type { PropertyImpact } from "../types";

export interface PropertyImpactBadgeProps {
	impact: PropertyImpact;
	compact?: boolean;
}

export function PropertyImpactBadge({
	impact,
	compact = false,
}: PropertyImpactBadgeProps) {
	const metadata = getImpactMetadata(impact);
	return (
		<span
			className={`mp-v3-impact-badge mp-v3-impact-badge--${metadata.tone}${
				compact ? " mp-impact-badge--compact" : ""
			}`}
			title={metadata.summary}
			data-impact={impact}
			data-tone={metadata.tone}
		>
			<span className="mp-v3-impact-badge__dot" aria-hidden="true" />
			<span className="mp-impact-badge__label">{metadata.label}</span>
		</span>
	);
}
