import type {
	EventBusChoice,
	EventProduce,
	EventSubscription,
	EventSubscriptionFilter,
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

/**
 * What one `events.produces` entry lets the workload publish, as
 * modules/events/iam.tf builds its condition: `any: true` lifts the
 * detail-type restriction, and `own_source_only: false` lifts the source one.
 */
export function describeProduceEntry(
	entry: EventProduce,
	busName: string,
	ownSource: string,
): string {
	const types = entry.any
		? "any detail-type"
		: (entry.detail_types ?? []).length > 0
			? `detail-type ${(entry.detail_types ?? []).join(", ")}`
			: "no detail-type";
	const source =
		entry.own_source_only === false ? "any source" : `source ${ownSource}`;
	return `${busName}: ${types}, ${source}`;
}

/** Same rule as app/events.go for an explicit subscription name. */
export const SUBSCRIPTION_NAME_PATTERN = /^[a-z0-9][a-z0-9-]{0,31}$/;

/** Same rule as app/events.go for a subscription filter field path. */
export const SUBSCRIPTION_FILTER_FIELD_PATTERN = /^detail(\.[A-Za-z0-9_-]+)+$/;

export const SUBSCRIPTION_MAX_FILTERS = 5;
export const SUBSCRIPTION_MAX_FILTER_VALUES = 10;

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

/** How a subscription's event type is shown in the UI. */
export function subscriptionEventTypeDisplay(sub: EventSubscription): string {
	if (sub.detail_type !== undefined) return sub.detail_type;
	if (sub.detail_type_prefix !== undefined) return `${sub.detail_type_prefix}*`;
	return "";
}

/** True when the event type is only `*` (or `**`), matching everything on the bus. */
export function isCatchAllWildcard(eventType: string): boolean {
	const trimmed = eventType.trim();
	return trimmed.length > 0 && /^\*+$/.test(trimmed);
}

/** Parses a comma-separated list of filter values into trimmed non-empty strings. */
export function parseFilterValues(text: string): string[] {
	return text
		.split(",")
		.map((s) => s.trim())
		.filter((s) => s !== "");
}

/** Summary label for a subscription's optional filter trigger. */
export function subscriptionFilterSummary(sub: EventSubscription): string {
	const hasSource = Boolean(sub.source && sub.source.trim() !== "");
	const filterCount = (sub.filters ?? []).length;
	const total = (hasSource ? 1 : 0) + filterCount;
	if (total === 0) return "Filter (optional)";
	return total === 1 ? "1 filter" : `${total} filters`;
}

/**
 * True when one field path lies inside the other (detail.order and
 * detail.order.type). EventBridge cannot match a key as both a value list and
 * an object, so app/events.go refuses the pair.
 */
export function filterFieldsNest(a: string, b: string): boolean {
	return a !== b && (a.startsWith(`${b}.`) || b.startsWith(`${a}.`));
}

export function nestedFilterFieldError(field: string, other: string): string {
	return `"${field}" and "${other}" overlap: one field lies inside the other. Keep one of them.`;
}

/** Validates a single filter condition (field path + values). */
export function subscriptionFilterError(
	filter: EventSubscriptionFilter,
	index: number,
	allFilters: EventSubscriptionFilter[],
): string | null {
	const field = filter.field.trim();
	if (field === "") {
		return "Enter a field path starting with detail., such as detail.status.";
	}
	if (!SUBSCRIPTION_FILTER_FIELD_PATTERN.test(field)) {
		return "Field path must start with detail. followed by letters, digits, underscores or hyphens (e.g. detail.status or detail.order.type).";
	}
	const firstIndex = allFilters.findIndex((f) => f.field.trim() === field);
	if (firstIndex !== -1 && firstIndex < index) {
		return `Field "${field}" is already filtered above; combine the values into one filter.`;
	}
	const nested = allFilters.find(
		(f, i) => i < index && filterFieldsNest(f.field.trim(), field),
	);
	if (nested) {
		return nestedFilterFieldError(field, nested.field.trim());
	}
	const values = (filter.values ?? []).map((v) => v.trim());
	if (values.length === 0 || values.some((v) => v === "")) {
		return `Enter at least one value for ${field}.`;
	}
	if (values.length > SUBSCRIPTION_MAX_FILTER_VALUES) {
		return `A filter can match at most ${SUBSCRIPTION_MAX_FILTER_VALUES} values (${field} has ${values.length}).`;
	}
	return null;
}

/** All validation errors on a saved or draft subscription entry. */
export function subscriptionErrors(
	sub: EventSubscription,
	index: number,
	allSubscribes: EventSubscription[],
	busEnabled: boolean,
): string[] {
	const errors: string[] = [];
	const hasDetail = Boolean(sub.detail_type && sub.detail_type.trim() !== "");
	const hasPrefix = Boolean(
		sub.detail_type_prefix && sub.detail_type_prefix.trim() !== "",
	);
	if (hasDetail === hasPrefix) {
		errors.push("Enter an event type to subscribe to.");
	} else if (hasPrefix && sub.detail_type_prefix?.includes("*")) {
		errors.push(
			"detail_type_prefix cannot contain *; edit the event type to save it as a wildcard.",
		);
	}
	if (sub.name !== undefined && sub.name !== "") {
		if (!SUBSCRIPTION_NAME_PATTERN.test(sub.name)) {
			errors.push(
				"A name is 1 to 32 lower-case letters, digits and hyphens, starting with a letter or a digit.",
			);
		}
	}
	const key = subscriptionKey(sub, busEnabled);
	if ((hasDetail || hasPrefix) && key === "") {
		errors.push(
			"No key can be derived from this event type; open Advanced and give it a name.",
		);
	} else if (key !== "") {
		const firstIdx = allSubscribes.findIndex(
			(other) => subscriptionKey(other, busEnabled) === key,
		);
		if (firstIdx !== -1 && firstIdx !== index) {
			errors.push(
				`Another subscription already has the key "${key}"; give one of them a distinct name.`,
			);
		}
	}
	if (sub.bus === "project" && !busEnabled) {
		errors.push(
			"The project bus is off; turn it on in the EventBridge node or move this subscription to the default bus.",
		);
	}
	const filters = sub.filters ?? [];
	if (filters.length > SUBSCRIPTION_MAX_FILTERS) {
		errors.push(
			`A subscription can have at most ${SUBSCRIPTION_MAX_FILTERS} field filters.`,
		);
	}
	filters.forEach((f, fi) => {
		const err = subscriptionFilterError(f, fi, filters);
		if (err) errors.push(err);
	});
	return errors;
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
