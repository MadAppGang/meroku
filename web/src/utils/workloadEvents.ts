import type {
	EventBusChoice,
	EventSubscription,
	WorkloadEvents,
	YamlInfrastructureConfig,
} from "../types/yamlConfig";

/** The workload kinds, as their event sources tell them apart. */
export type EventWorkloadKind =
	| "backend"
	| "service"
	| "scheduled_task"
	| "event_task";

/**
 * EVENT_SOURCE of a workload. Same four formulas as app/events.go, pinned by
 * app/testdata/event_source_vectors.json.
 */
export function eventSourceOf(
	project: string,
	kind: EventWorkloadKind,
	name: string,
): string {
	switch (kind) {
		case "backend":
			return `${project}.backend`;
		case "service":
			return `${project}.${name}`;
		case "scheduled_task":
			return `${project}.task.${name}`;
		case "event_task":
			return `${project}.event.${name}`;
	}
}

/** The project bus modules/events creates. */
export function projectBusName(config: YamlInfrastructureConfig): string {
	return `${config.project}-${config.env}`;
}

export function isEventBusEnabled(config: YamlInfrastructureConfig): boolean {
	return config.event_bus?.enabled === true;
}

/** Same rule as app/events.go for an explicit subscription name. */
export const SUBSCRIPTION_NAME_PATTERN = /^[a-z0-9][a-z0-9-]{0,31}$/;

/** The resolved bus of an entry: "project" or "default". */
export function resolveBus(
	bus: EventBusChoice | undefined,
	busEnabled: boolean,
): EventBusChoice {
	if (bus) return bus;
	return busEnabled ? "project" : "default";
}

/**
 * A subscription's key, as app/events.go derives it: the name when set,
 * otherwise the detail-type (or the prefix followed by "-prefix"), lower-cased,
 * every run outside [a-z0-9] turned into "-", trimmed, cut to 32, and
 * "-default" appended on the default bus. "" when nothing usable is left.
 */
export function subscriptionKey(
	sub: EventSubscription,
	busEnabled: boolean,
): string {
	if (sub.name) return sub.name;
	const base = sub.detail_type
		? sub.detail_type
		: `${sub.detail_type_prefix ?? ""}-prefix`;
	let key = base
		.toLowerCase()
		.replace(/[^a-z0-9]+/g, "-")
		.replace(/^-+|-+$/g, "");
	if (key.length > 32) key = key.slice(0, 32).replace(/-+$/g, "");
	if (key === "") return "";
	if (resolveBus(sub.bus, busEnabled) === "default") key += "-default";
	return key;
}

/** Drops empty lists and blocks, so an emptied editor saves no `events` key. */
export function compactEvents(
	events: WorkloadEvents | undefined,
): WorkloadEvents | undefined {
	if (!events) return undefined;
	const out: WorkloadEvents = {};
	if (events.subscribes && events.subscribes.length > 0)
		out.subscribes = events.subscribes;
	if (events.queue && Object.values(events.queue).some((v) => v !== undefined))
		out.queue = events.queue;
	if (events.produces && events.produces.length > 0)
		out.produces = events.produces;
	return Object.keys(out).length > 0 ? out : undefined;
}
