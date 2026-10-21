/**
 * properties-v3 — telemetry row primitive (Clarification 10).
 *
 * Timestamped value row with explicit resolution state, refresh state, and
 * fixture/demo labeling. Never fabricates live health: unresolved readings
 * render an empty placeholder, and demo data is labeled as such.
 */

import { useEffect, useState } from "react";
import { PropertyFieldFrame } from "../../properties/PropertyPanelPrimitives";
import type { ResolutionState } from "../types";

export interface PropertyTelemetryProps {
	label: string;
	value: string;
	timestamp?: string;
	resolution?: ResolutionState;
	/** Marks fixture/demo data explicitly (Storybook adapters). */
	demo?: boolean;
	span?: "full" | "half";
}

const RESOLUTION_PLACEHOLDER: Record<ResolutionState, string> = {
	known: "No data",
	provisioned: "Collected after provisioning",
	runtime: "Available at runtime",
};

export function PropertyTelemetry({
	label,
	value,
	timestamp,
	resolution = "known",
	demo = false,
	// Full-width rows: ISO timestamps (20 chars) and metric values must not
	// compete for a half-width column at 540px panel width.
	span = "full",
}: PropertyTelemetryProps) {
	const [copyState, setCopyState] = useState<"idle" | "copied">("idle");

	useEffect(() => {
		if (copyState !== "copied") return;
		const timer = window.setTimeout(() => setCopyState("idle"), 1500);
		return () => window.clearTimeout(timer);
	}, [copyState]);

	const resolved = value.trim().length > 0;

	return (
		<PropertyFieldFrame label={label} span={span} kind="telemetry">
			<div className="mp-v3-telemetry__row" data-resolved={resolved}>
				<code className="mp-runtime-parameter__value mp-overflow-text">
					{resolved ? value : RESOLUTION_PLACEHOLDER[resolution]}
				</code>
				{demo && resolved ? (
					<span className="mp-v3-telemetry__demo">demo</span>
				) : null}
				{timestamp && resolved ? (
					<time className="mp-v3-telemetry__timestamp">{timestamp}</time>
				) : null}
			</div>
		</PropertyFieldFrame>
	);
}
