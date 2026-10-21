/**
 * properties-v3 — encapsulated on-demand CloudWatch log viewer field.
 *
 * Renders for `logRole: "viewer"` fields when a real CloudWatch destination
 * gates the Logs tab. On-demand only: NO AWS/network calls, no fabricated
 * log groups or telemetry — the output stays empty ("Logs are not loaded.")
 * until the user explicitly presses "Stream logs".
 *
 * - `awsHref` is derived from the REAL destination metadata (ARN → regional
 *   console URL; free-form name → generic log-groups console page). Never a
 *   fabricated group link.
 * - Demo mode (fixture adapters) streams clearly-labeled synthetic lines and
 *   marks the field label with "(demo)".
 */

import { PropertyLogViewer } from "./RuntimePrimitives";

/** Explicit synthetic fixture lines — only ever shown in demo mode. */
const DEMO_LOG_LINES = [
	"DEMO SYNTHETIC log line — not real infrastructure output",
	"DEMO SYNTHETIC service event sequence=1 status=ok",
	"DEMO SYNTHETIC service event sequence=2 status=ok",
];

/** Build a CloudWatch console URL from real destination metadata. */
function cloudWatchHref(destination: string | null | undefined): string {
	if (typeof destination !== "string" || destination === "") {
		return "https://console.aws.amazon.com/cloudwatch/home#logsV2:log-groups";
	}
	// arn:aws:logs:REGION:ACCOUNT:log-group:NAME → regional deep link.
	const match = destination.match(
		/^arn:aws:logs:([a-z0-9-]+):\d+:log-group:(.+)$/,
	);
	if (match) {
		const [, region, group] = match;
		const groupPath = group.replace(/\/$/, "").replace(/\//g, "$252F");
		return `https://${region}.console.aws.amazon.com/cloudwatch/home?region=${region}#logsV2:log-groups/log-group/${groupPath}`;
	}
	return "https://console.aws.amazon.com/cloudwatch/home#logsV2:log-groups";
}

export interface PropertyLogViewerFieldProps {
	label: string;
	/** Real CloudWatch destination (log group ARN or name); null disables. */
	destination: string | null | undefined;
	/** Fixture/demo marker — synthetic stream, clearly labeled. */
	demo?: boolean;
}

export function PropertyLogViewerField({
	label,
	destination,
	demo = false,
}: PropertyLogViewerFieldProps) {
	return (
		<PropertyLogViewer
			label={demo ? `${label} (demo)` : label}
			awsHref={cloudWatchHref(destination)}
			// Real mode: no fixture data — empty until the user streams; demo
			// mode uses the explicit synthetic lines above (never defaults).
			sampleMessages={demo ? DEMO_LOG_LINES : []}
		/>
	);
}
