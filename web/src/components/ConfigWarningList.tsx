import { AlertTriangle, ExternalLink } from "lucide-react";
import type { ConfigWarning } from "../types/configWarnings";
import {
	groupWarningsByItem,
	safeDocsUrl,
	warningKey,
} from "../utils/configWarnings";
import { StatusNote } from "./ui/status-note";

/**
 * A warning's detail and docs link, without its title. The detail is plain
 * text from the server; its line breaks are kept and it is never parsed as
 * HTML. A docs value that is not an absolute https URL is not linked.
 */
export function ConfigWarningBody({ warning }: { warning: ConfigWarning }) {
	const docs = safeDocsUrl(warning.docs);
	return (
		<div className="space-y-1">
			<p className="whitespace-pre-line break-words text-xs text-muted-foreground">
				{warning.detail}
			</p>
			{docs && (
				<a
					href={docs}
					target="_blank"
					rel="noreferrer"
					className="inline-flex items-center gap-1 rounded-sm text-xs font-medium text-warning underline-offset-4 hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
				>
					How to fix this
					<ExternalLink className="size-3" aria-hidden="true" />
					<span className="sr-only">(opens in a new tab)</span>
				</a>
			)}
		</div>
	);
}

function WarningEntries({ warnings }: { warnings: readonly ConfigWarning[] }) {
	return (
		<ul className="space-y-3">
			{warnings.map((warning) => (
				<li key={warningKey(warning)} data-warning-code={warning.code}>
					<StatusNote tone="warning">
						<p className="break-words font-medium">{warning.title}</p>
						<ConfigWarningBody warning={warning} />
					</StatusNote>
				</li>
			))}
		</ul>
	);
}

export interface ConfigWarningListProps {
	warnings: readonly ConfigWarning[];
	/**
	 * `flat` lists the warnings as given (one item's warnings, in the sidebar);
	 * `byItem` puts each item's warnings under its name (the whole report).
	 */
	layout?: "flat" | "byItem";
	/** Shown when there are no warnings. */
	emptyMessage?: string;
}

/**
 * Configuration warnings: for each, the title, the detail with what to do,
 * and the docs link.
 */
export function ConfigWarningList({
	warnings,
	layout = "flat",
	emptyMessage = "No configuration warnings.",
}: ConfigWarningListProps) {
	if (warnings.length === 0) {
		return (
			<StatusNote tone="success">
				<p>{emptyMessage}</p>
			</StatusNote>
		);
	}

	if (layout === "flat") {
		return <WarningEntries warnings={warnings} />;
	}

	return (
		<div className="space-y-4">
			{groupWarningsByItem(warnings).map((group) => (
				<section key={group.key} aria-label={group.label} className="space-y-2">
					<h3 className="text-xs font-medium text-muted-foreground">
						{group.label}
					</h3>
					<WarningEntries warnings={group.warnings} />
				</section>
			))}
		</div>
	);
}

/**
 * One node's warnings under a "Configuration warnings" heading, for the top
 * of the sidebar. Renders nothing when there are none.
 */
export function ConfigWarningSection({
	warnings,
}: {
	warnings: readonly ConfigWarning[];
}) {
	if (warnings.length === 0) return null;
	return (
		<section
			aria-label="Configuration warnings"
			data-testid="node-warnings"
			className="space-y-3 rounded-lg border border-warning/40 bg-card p-3 text-card-foreground"
		>
			<h3 className="flex items-center gap-2 text-sm font-medium">
				<AlertTriangle className="size-4 text-warning" aria-hidden="true" />
				Configuration warnings
			</h3>
			<ConfigWarningList warnings={warnings} />
		</section>
	);
}
