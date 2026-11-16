import { AlertTriangle } from "lucide-react";
import type { ConfigWarning } from "../types/configWarnings";
import { warningCountLabel } from "../utils/configWarnings";
import { ConfigWarningList } from "./ConfigWarningList";
import { Popover, PopoverContent, PopoverTrigger } from "./ui/popover";

export interface ConfigWarningsStatusProps {
	/** Every warning in the report, env-wide first. */
	warnings: readonly ConfigWarning[];
}

/**
 * The status-line control for configuration warnings: "N warnings", opening
 * a list of every warning grouped by item. Renders nothing when there are
 * none.
 */
export function ConfigWarningsStatus({ warnings }: ConfigWarningsStatusProps) {
	if (warnings.length === 0) return null;
	const label = warningCountLabel(warnings.length);
	return (
		<Popover>
			<PopoverTrigger asChild>
				<button
					type="button"
					data-testid="config-warnings-status"
					className="flex items-center gap-1.5 rounded-sm px-2 py-0.5 text-xs font-medium text-warning hover:bg-warning/15 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
				>
					<AlertTriangle className="size-3.5" aria-hidden="true" />
					<span>{label}</span>
				</button>
			</PopoverTrigger>
			<PopoverContent
				side="top"
				align="start"
				aria-label="Configuration warnings"
				className="max-h-96 w-96 overflow-y-auto"
			>
				<ConfigWarningList warnings={warnings} layout="byItem" />
			</PopoverContent>
		</Popover>
	);
}
