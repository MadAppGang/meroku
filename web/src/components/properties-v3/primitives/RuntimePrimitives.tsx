/**
 * properties-v3 — runtime value row and on-demand log viewer.
 *
 * Ported from the panel-specifications branch of `properties/` (never merged
 * to main) so V3 owns them instead of depending on unmerged library changes.
 * Both compose the shared `PropertyFieldFrame` / `PropertyOverflowText`.
 */

import { Copy, ExternalLink } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import {
	PropertyFieldFrame,
	PropertyOverflowText,
} from "../../properties/PropertyPanelPrimitives";
import type { ResolutionState } from "../types";

const runtimePlaceholder: Record<ResolutionState, string> = {
	known: "Not available",
	provisioned: "Resolved when created",
	runtime: "Available at runtime",
};

export function PropertyRuntimeParameter({
	label,
	value,
	mono = false,
	span = "half",
	resolution = "known",
	href,
	copyable = true,
}: {
	label: string;
	value: string;
	mono?: boolean;
	span?: "full" | "half";
	resolution?: ResolutionState;
	href?: string;
	copyable?: boolean;
}) {
	const resolved = Boolean(value.trim());
	const displayValue = resolved ? value : runtimePlaceholder[resolution];

	return (
		<PropertyFieldFrame label={label} span={span} kind="runtime">
			<div
				className={`mp-runtime-parameter ${mono ? "mp-mono" : ""}`}
				data-resolution={resolution}
				data-resolved={resolved}
			>
				<PropertyOverflowText
					className="mp-runtime-parameter__value"
					value={displayValue}
					ariaLabel={label}
				/>
				{resolved && href && (
					<a
						className="mp-runtime-parameter__action"
						href={href}
						target="_blank"
						rel="noreferrer"
						aria-label={`Open ${label} in AWS`}
						title="Open in AWS"
					>
						<ExternalLink />
					</a>
				)}
				{resolved && copyable && (
					<button
						type="button"
						className="mp-runtime-parameter__action"
						aria-label={`Copy ${label}`}
						title="Copy"
						onClick={() => navigator.clipboard?.writeText(value)}
					>
						<Copy />
					</button>
				)}
			</div>
		</PropertyFieldFrame>
	);
}

export type PropertyLogSubscription = (
	append: (line: string) => void,
) => undefined | (() => void);

export function PropertyLogViewer({
	label = "Live logs",
	awsHref,
	span = "full",
	sampleMessages = [],
	subscribe,
}: {
	label?: string;
	awsHref: string;
	span?: "full" | "half";
	sampleMessages?: string[];
	subscribe?: PropertyLogSubscription;
}) {
	const [lines, setLines] = useState<string[]>([]);
	const [live, setLive] = useState(false);
	const nextLine = useRef(0);
	const outputRef = useRef<HTMLTextAreaElement>(null);

	useEffect(() => {
		if (!live) return;

		const append = (line: string) =>
			setLines((current) => [...current.slice(-49), line]);

		if (subscribe) {
			const unsubscribe = subscribe(append);
			return typeof unsubscribe === "function" ? unsubscribe : undefined;
		}

		if (sampleMessages.length === 0) return;

		const appendLine = () => {
			const timestamp = new Date().toISOString().slice(11, 23);
			const message = sampleMessages[nextLine.current % sampleMessages.length];
			nextLine.current += 1;
			append(`${timestamp}  ${message}`);
		};

		appendLine();
		const interval = window.setInterval(appendLine, 1200);
		return () => window.clearInterval(interval);
	}, [live, sampleMessages, subscribe]);

	useEffect(() => {
		if (outputRef.current) {
			outputRef.current.scrollTop = outputRef.current.scrollHeight;
		}
	});

	const status = live ? "Live" : lines.length > 0 ? "Paused" : "Not loaded";

	return (
		<PropertyFieldFrame
			label={label}
			span={span}
			kind="log-viewer"
			labelPlacement="stacked"
		>
			<div className="mp-log-viewer" data-live={live}>
				<div className="mp-log-viewer__toolbar">
					<span className="mp-log-viewer__status">
						<span aria-hidden="true" />
						{status}
					</span>
					<div className="mp-log-viewer__actions">
						<button
							type="button"
							className="mp-log-viewer__button"
							onClick={() => setLive((current) => !current)}
						>
							{live ? "Stop stream" : "Stream logs"}
						</button>
						<a
							className="mp-log-viewer__button mp-log-viewer__button--secondary"
							href={awsHref}
							target="_blank"
							rel="noreferrer"
						>
							Open in AWS
							<ExternalLink aria-hidden="true" />
						</a>
					</div>
				</div>
				<textarea
					ref={outputRef}
					className="mp-log-viewer__output mp-mono"
					aria-label={`${label} output`}
					aria-live={live ? "polite" : "off"}
					data-empty={lines.length === 0}
					readOnly
					value={lines.length > 0 ? lines.join("\n") : "Logs are not loaded."}
				/>
			</div>
		</PropertyFieldFrame>
	);
}
