import {
	AlertTriangle,
	ChevronDown,
	ChevronRight,
	ChevronUp,
} from "lucide-react";
import { useId, useState } from "react";
import type { ConfigWarning } from "../types/configWarnings";
import {
	warningCountLabel,
	warningItemLabel,
	warningKey,
} from "../utils/configWarnings";
import { ConfigWarningBody } from "./ConfigWarningList";
import {
	Collapsible,
	CollapsibleContent,
	CollapsibleTrigger,
} from "./ui/collapsible";
import { StatusNote } from "./ui/status-note";

function EnvWarningRow({ warning }: { warning: ConfigWarning }) {
	const [open, setOpen] = useState(false);
	const isEnvWide = warning.item.kind === "env";
	return (
		<li data-warning-code={warning.code}>
			<Collapsible open={open} onOpenChange={setOpen}>
				<CollapsibleTrigger asChild>
					<button
						type="button"
						className="flex w-full items-start gap-2 rounded-sm text-left text-sm hover:bg-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
					>
						{open ? (
							<ChevronDown
								className="mt-0.5 size-4 shrink-0 text-muted-foreground"
								aria-hidden="true"
							/>
						) : (
							<ChevronRight
								className="mt-0.5 size-4 shrink-0 text-muted-foreground"
								aria-hidden="true"
							/>
						)}
						<span className="min-w-0 flex-1 break-words">
							{!isEnvWide && (
								<span className="text-muted-foreground">
									{warningItemLabel(warning.item)}:{" "}
								</span>
							)}
							{warning.title}
						</span>
					</button>
				</CollapsibleTrigger>
				<CollapsibleContent>
					<div className="pl-6 pt-1">
						<ConfigWarningBody warning={warning} />
					</div>
				</CollapsibleContent>
			</Collapsible>
		</li>
	);
}

export interface EnvWarningsPanelProps {
	/**
	 * Warnings with no node on the canvas: env-wide ones, items the canvas
	 * does not show, kinds this UI does not know.
	 */
	warnings: readonly ConfigWarning[];
	/** Set when `meroku generate` would refuse the YAML. */
	configError?: string;
	/** Starts collapsed to its one-line header. */
	defaultCollapsed?: boolean;
}

/**
 * Environment-wide configuration warnings, visible without selecting a node.
 * Every title is listed; a row opens to the detail and docs link. The header
 * collapses the card to one line that stays on screen. Renders nothing when
 * there is nothing to say.
 */
export function EnvWarningsPanel({
	warnings,
	configError,
	defaultCollapsed = false,
}: EnvWarningsPanelProps) {
	const [collapsed, setCollapsed] = useState(defaultCollapsed);
	const bodyId = useId();

	if (warnings.length === 0 && !configError) return null;

	const heading =
		warnings.length > 0
			? warningCountLabel(warnings.length, "environment warning")
			: "Configuration error";

	return (
		<section
			aria-label="Environment warnings"
			data-testid="env-warnings"
			className="w-full rounded-lg border border-warning/40 bg-card text-card-foreground shadow-lg"
		>
			<div className="flex items-center gap-2 px-3 py-2">
				<AlertTriangle
					className="size-4 shrink-0 text-warning"
					aria-hidden="true"
				/>
				<h2 className="min-w-0 flex-1 text-sm font-medium">{heading}</h2>
				<button
					type="button"
					aria-expanded={!collapsed}
					aria-controls={bodyId}
					onClick={() => setCollapsed((value) => !value)}
					className="inline-flex size-6 shrink-0 items-center justify-center rounded-sm text-muted-foreground hover:bg-accent hover:text-accent-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
				>
					{collapsed ? (
						<ChevronDown className="size-4" aria-hidden="true" />
					) : (
						<ChevronUp className="size-4" aria-hidden="true" />
					)}
					<span className="sr-only">
						{collapsed
							? "Show environment warnings"
							: "Hide environment warnings"}
					</span>
				</button>
			</div>
			{!collapsed && (
				<div
					id={bodyId}
					className="max-h-80 space-y-3 overflow-y-auto border-t border-border px-3 py-2"
				>
					{configError && (
						<StatusNote tone="error">
							<p className="font-medium">
								meroku generate will refuse this configuration
							</p>
							<p className="whitespace-pre-line break-words text-xs">
								{configError}
							</p>
						</StatusNote>
					)}
					{warnings.length > 0 && (
						<ul className="space-y-1">
							{warnings.map((warning) => (
								<EnvWarningRow key={warningKey(warning)} warning={warning} />
							))}
						</ul>
					)}
				</div>
			)}
		</section>
	);
}
