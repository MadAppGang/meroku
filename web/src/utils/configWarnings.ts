import type {
	ConfigWarning,
	WarningItem,
	WarningKind,
} from "../types/configWarnings";

const KNOWN_KINDS: ReadonlySet<string> = new Set<WarningKind>([
	"env",
	"backend",
	"service",
	"scheduled_task",
	"event_task",
	"amplify_app",
	"compute_pool",
	"valkey",
	"data_bucket",
	"event_bus",
]);

/** False for a kind a newer server added that this UI does not know. */
export function isKnownWarningKind(kind: string): kind is WarningKind {
	return KNOWN_KINDS.has(kind);
}

function assertNever(value: never): never {
	throw new Error(`Unhandled warning kind: ${String(value)}`);
}

/**
 * The canvas node a warning belongs on, or null for an env-wide warning.
 * The ids are the ones the canvas builds in `DeploymentCanvas.tsx`,
 * `additionalServicesNodes.ts` and `hiddenComponentsNodes.ts`.
 */
export function warningNodeId(item: WarningItem): string | null {
	if (!isKnownWarningKind(item.kind)) return null;
	switch (item.kind) {
		case "env":
			return null;
		case "backend":
			return "backend-service";
		case "service":
			return `service-${item.name}`;
		case "scheduled_task":
			return `scheduled-${item.name}`;
		case "event_task":
			return `event-${item.name}`;
		case "amplify_app":
			return `amplify-${item.name}`;
		case "compute_pool":
			return "ecs-cluster";
		case "valkey":
			return "valkey";
		case "data_bucket":
			return "s3";
		case "event_bus":
			return "eventbridge";
		default:
			return assertNever(item.kind);
	}
}

/**
 * Splits warnings between the canvas nodes in `nodeIds` and the rest.
 * `unplaced` holds env-wide warnings, warnings whose node is not on the
 * canvas (hidden, or an unnamed item) and warnings of an unknown kind, in
 * the order the server sent them.
 */
export function warningsByNode(
	warnings: readonly ConfigWarning[],
	nodeIds: ReadonlySet<string>,
): { byNode: Map<string, ConfigWarning[]>; unplaced: ConfigWarning[] } {
	const byNode = new Map<string, ConfigWarning[]>();
	const unplaced: ConfigWarning[] = [];
	for (const warning of warnings) {
		const nodeId = warningNodeId(warning.item);
		if (nodeId === null || !nodeIds.has(nodeId)) {
			unplaced.push(warning);
			continue;
		}
		const list = byNode.get(nodeId);
		if (list) {
			list.push(warning);
		} else {
			byNode.set(nodeId, [warning]);
		}
	}
	return { byNode, unplaced };
}

/** The warnings that belong on one canvas node. */
export function warningsForNode(
	warnings: readonly ConfigWarning[],
	nodeId: string,
): ConfigWarning[] {
	return warnings.filter((w) => warningNodeId(w.item) === nodeId);
}

const KIND_LABELS: Record<WarningKind, string> = {
	env: "Environment",
	backend: "Backend",
	service: "Service",
	scheduled_task: "Scheduled task",
	event_task: "Event task",
	amplify_app: "Amplify app",
	compute_pool: "Compute pool",
	valkey: "Valkey",
	data_bucket: "Shared folder",
	event_bus: "Event bus",
};

/**
 * "Service magento-bridge", "Environment", "Valkey". An unknown kind is shown
 * as the server spelled it.
 */
export function warningItemLabel(item: WarningItem): string {
	const kind = isKnownWarningKind(item.kind)
		? KIND_LABELS[item.kind]
		: String(item.kind).replaceAll("_", " ");
	return item.name ? `${kind} ${item.name}` : kind;
}

export interface WarningGroup {
	/** `kind/name`, unique within one report. */
	key: string;
	label: string;
	warnings: ConfigWarning[];
}

/**
 * Groups warnings by item, keeping the server's order (env-wide first, then
 * by kind and name), so each item appears once.
 */
export function groupWarningsByItem(
	warnings: readonly ConfigWarning[],
): WarningGroup[] {
	const groups = new Map<string, WarningGroup>();
	for (const warning of warnings) {
		const key = `${warning.item.kind}/${warning.item.name}`;
		const group = groups.get(key);
		if (group) {
			group.warnings.push(warning);
		} else {
			groups.set(key, {
				key,
				label: warningItemLabel(warning.item),
				warnings: [warning],
			});
		}
	}
	return [...groups.values()];
}

/** A stable React key for one warning; a report never repeats it. */
export function warningKey(warning: ConfigWarning): string {
	return `${warning.code}|${warning.item.kind}|${warning.item.name}|${warning.detail}`;
}

/** Only an absolute https URL is rendered as a link. */
export function safeDocsUrl(docs: string | undefined): string | null {
	if (!docs) return null;
	try {
		const url = new URL(docs);
		return url.protocol === "https:" ? url.toString() : null;
	} catch {
		return null;
	}
}

/** "1 warning", "3 warnings". */
export function warningCountLabel(count: number, noun = "warning"): string {
	return `${count} ${noun}${count === 1 ? "" : "s"}`;
}
