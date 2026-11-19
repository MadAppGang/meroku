import type {
	EventBridgeRule,
	EventBusChoice,
	EventBusShare,
	EventBusShareAccess,
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

/**
 * The name of a consumer's event queue, as modules/events names it:
 * `<project>-events-<workload>-<env>`, with `.fifo` for the ordered set.
 * Null past the 80-character SQS cap (the suffix counts), where the naming
 * cascade falls back to a hashed name.
 */
export function eventQueueName(
	config: YamlInfrastructureConfig,
	workload: string,
	kind: "queue" | "dlq" | "delivery-dlq" = "queue",
	fifo = false,
): string | null {
	const tail = kind === "queue" ? "" : `-${kind}`;
	const suffix = fifo ? ".fifo" : "";
	const name = `${config.project}-events-${workload}${tail}-${config.env}${suffix}`;
	return name.length <= 80 ? name : null;
}

/** The queue's ARN, or null when the account is unknown. */
export function eventQueueArn(
	config: YamlInfrastructureConfig,
	queueName: string,
): string | null {
	if (!config.account_id || !config.region) return null;
	return `arn:aws:sqs:${config.region}:${config.account_id}:${queueName}`;
}

export function isEventBusEnabled(config: YamlInfrastructureConfig): boolean {
	return config.event_bus?.enabled === true;
}

/**
 * The Regions that offer the enhanced custom event bus. Mirrors
 * enhancedEventBusRegions in app/events_enhanced.go; change both together.
 */
export const ENHANCED_EVENT_BUS_REGIONS: readonly string[] = [
	"ap-east-1",
	"ap-northeast-1",
	"ap-south-1",
	"ap-southeast-1",
	"ap-southeast-2",
	"ap-southeast-5",
	"ap-southeast-7",
	"eu-central-1",
	"eu-north-1",
	"eu-south-2",
	"eu-west-1",
	"us-east-1",
	"us-east-2",
	"us-west-2",
];

export function enhancedBusAvailableIn(region: string | undefined): boolean {
	return region !== undefined && ENHANCED_EVENT_BUS_REGIONS.includes(region);
}

/** True when the project bus is on and is the enhanced kind. */
export function isEnhancedBus(config: YamlInfrastructureConfig): boolean {
	return (
		config.event_bus?.enabled === true && config.event_bus?.type === "enhanced"
	);
}

/** True when any subscription of the block is ordered. */
export function eventsHaveOrdered(events: WorkloadEvents | undefined): boolean {
	return (events?.subscribes ?? []).some((sub) => sub.ordered === true);
}

/** True when the backend or the named service has an ordered subscription. */
export function consumerHasOrdered(
	config: YamlInfrastructureConfig,
	workload: string,
): boolean {
	if (workload === "backend") return eventsHaveOrdered(config.workload?.events);
	const service = (config.services ?? []).find((s) => s.name === workload);
	return eventsHaveOrdered(service?.events);
}

/**
 * The one rule every subscription edit goes through, so no edit leaves YAML
 * that generate refuses. When the edit leaves the consumer with no ordered
 * subscription and it had one before, the deployed ordered queue is kept with
 * `queue.fifo_retired: true`. When the result has an ordered subscription,
 * `fifo_retired` is removed (the two together are refused).
 */
export function withOrderingCleared(
	previous: WorkloadEvents | undefined,
	next: WorkloadEvents | undefined,
): WorkloadEvents | undefined {
	const had = eventsHaveOrdered(previous);
	const has = eventsHaveOrdered(next);
	if (has && next?.queue?.fifo_retired !== undefined) {
		return { ...next, queue: { ...next.queue, fifo_retired: undefined } };
	}
	if (had && !has) {
		return { ...next, queue: { ...next?.queue, fifo_retired: true } };
	}
	return next;
}

/** The block with every `ordered` removed, through `withOrderingCleared`. */
function eventsWithoutOrdering(
	events: WorkloadEvents | undefined,
): WorkloadEvents | undefined {
	if (!eventsHaveOrdered(events)) return events;
	const next: WorkloadEvents = {
		...events,
		subscribes: (events?.subscribes ?? []).map((sub) =>
			sub.ordered === undefined ? sub : { ...sub, ordered: undefined },
		),
	};
	return withOrderingCleared(events, next);
}

/**
 * Removes every `ordered` from the backend and the services, keeping each
 * deployed ordered queue with `fifo_retired`. Used when the bus stops being
 * enhanced. `updates` holds only the sections that change; `affected` names
 * the workloads that lose ordered delivery.
 */
export function clearOrdering(config: YamlInfrastructureConfig): {
	updates: Partial<YamlInfrastructureConfig>;
	affected: string[];
} {
	const updates: Partial<YamlInfrastructureConfig> = {};
	const affected: string[] = [];
	if (eventsHaveOrdered(config.workload?.events)) {
		affected.push("backend");
		updates.workload = {
			events: eventsWithoutOrdering(config.workload?.events),
		} as YamlInfrastructureConfig["workload"];
	}
	const services = config.services ?? [];
	if (services.some((s) => eventsHaveOrdered(s.events))) {
		updates.services = services.map((s) => {
			if (!eventsHaveOrdered(s.events)) return s;
			affected.push(s.name);
			return { ...s, events: eventsWithoutOrdering(s.events) };
		});
	}
	return { updates, affected };
}

/** An entry that names the project bus with `bus: project`. */
export interface ProjectBusRef {
	/** Plain label, e.g. "orders subscribes order.created". */
	label: string;
}

/**
 * Every `subscribes`, `produces` and event-task `rules` entry whose bus is
 * `project`. Generate refuses them while the project bus is off, so the bus
 * cannot be turned off while any exists.
 */
export function explicitProjectBusRefs(
	config: YamlInfrastructureConfig,
): ProjectBusRef[] {
	const out: ProjectBusRef[] = [];
	const subs = (owner: string, list: EventSubscription[] | undefined) => {
		for (const sub of list ?? []) {
			if (sub.bus === "project")
				out.push({
					label: `${owner} subscribes ${subscriptionEventTypeDisplay(sub)}`,
				});
		}
	};
	const prods = (owner: string, list: EventProduce[] | undefined) => {
		for (const entry of list ?? []) {
			if (entry.bus !== "project") continue;
			const what = entry.any
				? "any event"
				: (entry.detail_types ?? []).join(", ") || "events";
			out.push({ label: `${owner} produces ${what}` });
		}
	};
	const rules = (owner: string, list: EventBridgeRule[] | undefined) => {
		for (const rule of list ?? []) {
			if (rule.bus === "project")
				out.push({ label: `${owner} rule ${rule.name}` });
		}
	};
	subs("backend", config.workload?.events?.subscribes);
	prods("backend", config.workload?.events?.produces);
	for (const s of config.services ?? []) {
		subs(s.name, s.events?.subscribes);
		prods(s.name, s.events?.produces);
	}
	for (const t of config.scheduled_tasks ?? [])
		prods(t.name, t.events?.produces);
	for (const t of config.event_processor_tasks ?? []) {
		prods(t.name, t.events?.produces);
		rules(t.name, t.rules);
	}
	return out;
}

/**
 * Event-task rules that run from the project bus while it is on: a rule
 * without `bus: default`, or a legacy single-rule task. An enhanced bus cannot
 * start ECS tasks, so generate refuses these on an enhanced bus.
 */
export function eventTaskRulesOnProjectBus(
	config: YamlInfrastructureConfig,
): ProjectBusRef[] {
	if (!isEventBusEnabled(config)) return [];
	const out: ProjectBusRef[] = [];
	for (const t of config.event_processor_tasks ?? []) {
		if (t.rules && t.rules.length > 0) {
			for (const rule of t.rules) {
				if (resolveBus(rule.bus, true) === "project")
					out.push({ label: `${t.name} rule ${rule.name}` });
			}
		} else if (t.rule_name) {
			out.push({ label: `${t.name} rule ${t.rule_name}` });
		}
	}
	return out;
}

function withoutProjectBus<T extends { bus?: EventBusChoice }>(
	list: T[] | undefined,
): T[] | undefined {
	if (!list) return list;
	return list.map((e) => (e.bus === "project" ? { ...e, bus: undefined } : e));
}

function namesProjectBus(events: WorkloadEvents | undefined): boolean {
	return [...(events?.subscribes ?? []), ...(events?.produces ?? [])].some(
		(e) => e.bus === "project",
	);
}

function eventsWithoutProjectBus(
	events: WorkloadEvents | undefined,
): WorkloadEvents | undefined {
	if (!namesProjectBus(events)) return events;
	return {
		...events,
		subscribes: withoutProjectBus(events?.subscribes),
		produces: withoutProjectBus(events?.produces),
	};
}

/**
 * The updates that remove `bus` from every entry `explicitProjectBusRefs`
 * lists, so they follow the switch. A key set to `undefined` is pruned on save.
 */
export function clearExplicitProjectBus(
	config: YamlInfrastructureConfig,
): Partial<YamlInfrastructureConfig> {
	const updates: Partial<YamlInfrastructureConfig> = {};
	if (namesProjectBus(config.workload?.events)) {
		updates.workload = {
			events: eventsWithoutProjectBus(config.workload?.events),
		} as YamlInfrastructureConfig["workload"];
	}
	const services = config.services ?? [];
	if (services.some((s) => namesProjectBus(s.events))) {
		updates.services = services.map((s) => ({
			...s,
			events: eventsWithoutProjectBus(s.events),
		}));
	}
	const scheduled = config.scheduled_tasks ?? [];
	if (scheduled.some((t) => namesProjectBus(t.events))) {
		updates.scheduled_tasks = scheduled.map((t) => ({
			...t,
			events: eventsWithoutProjectBus(t.events),
		}));
	}
	const tasks = config.event_processor_tasks ?? [];
	if (
		tasks.some(
			(t) =>
				namesProjectBus(t.events) ||
				(t.rules ?? []).some((r) => r.bus === "project"),
		)
	) {
		updates.event_processor_tasks = tasks.map((t) => ({
			...t,
			events: eventsWithoutProjectBus(t.events),
			rules: withoutProjectBus(t.rules),
		}));
	}
	return updates;
}

/**
 * How the enhanced bus matches a detail type (subscriber filters have no
 * wildcard): exactly one `*` at the end is a prefix, at the start a suffix;
 * any other `*` cannot be matched.
 */
export type EnhancedDetailTypeMatch =
	| { kind: "exact" }
	| { kind: "prefix"; value: string }
	| { kind: "suffix"; value: string }
	| { kind: "unmatchable" };

export function enhancedDetailTypeMatch(
	detailType: string,
): EnhancedDetailTypeMatch {
	const stars = detailType.split("*").length - 1;
	if (stars === 0) return { kind: "exact" };
	if (stars === 1 && detailType.length > 1) {
		if (detailType.endsWith("*"))
			return { kind: "prefix", value: detailType.slice(0, -1) };
		if (detailType.startsWith("*"))
			return { kind: "suffix", value: detailType.slice(1) };
	}
	return { kind: "unmatchable" };
}

/** Plain copy for a mapped wildcard, or null when nothing is mapped. */
export function enhancedDetailTypeNote(detailType: string): string | null {
	const match = enhancedDetailTypeMatch(detailType);
	if (match.kind === "prefix")
		return `Matches detail types starting with ${match.value}`;
	if (match.kind === "suffix")
		return `Matches detail types ending with ${match.value}`;
	return null;
}

/** The refusal app/events.go gives a `*` the enhanced bus cannot match. */
export function enhancedDetailTypeError(detailType: string): string | null {
	if (enhancedDetailTypeMatch(detailType).kind !== "unmatchable") return null;
	return `"${detailType}" uses * in a way the enhanced bus cannot match: it matches a fixed start (job.*) or a fixed end (*.created) only. List the exact detail types as separate entries, or use a prefix.`;
}

/**
 * Why an ordered subscription cannot be deployed where it is, or null: the
 * bus is not enhanced, or the entry is on the default bus.
 */
export function orderedSubscriptionError(
	sub: EventSubscription,
	config: YamlInfrastructureConfig,
): string | null {
	if (sub.ordered !== true) return null;
	if (resolveBus(sub.bus, isEventBusEnabled(config)) === "default")
		return "Keep order works on the project bus only; the default bus cannot keep order. Turn Keep order off.";
	if (!isEnhancedBus(config))
		return "Keep order needs the Enhanced project bus; the Standard bus cannot keep order. Turn Keep order off, or choose Enhanced in the EventBridge node.";
	return null;
}

/** The longest event group the ordered queue accepts (SQS adds the account). */
export const EVENT_GROUP_ID_MAX = 115;

/** An event group id: letters, digits and punctuation, at most 115 characters. */
export function eventGroupIdError(value: string): string | null {
	if (value === "") return null;
	if (value.length > EVENT_GROUP_ID_MAX)
		return `An event group is at most ${EVENT_GROUP_ID_MAX} characters (this one has ${value.length}).`;
	if (!/^[\x21-\x7E]+$/.test(value))
		return "An event group uses letters, digits and punctuation only, without spaces.";
	return null;
}

/** Who a share entry names. */
export type EventBusShareKind = "organization" | "ou" | "account" | "env";

export function shareEntryKind(entry: EventBusShare): EventBusShareKind {
	if (entry.organization !== undefined) return "organization";
	if (entry.ou !== undefined) return "ou";
	if (entry.env !== undefined) return "env";
	return "account";
}

/** True for an access level that lets the other account subscribe. */
export function isSubscribeCapable(access: EventBusShareAccess): boolean {
	return access === "subscribe" || access === "full";
}

/** Same rules as app/events_enhanced.go. */
export const SHARE_OU_PATTERN = /^ou-[a-z0-9]{4,32}-[a-z0-9]{8,32}$/;
export const SHARE_ACCOUNT_PATTERN = /^[0-9]{12}$/;

/** A sibling environment the bus can be shared with (share-targets API). */
export interface ShareTargetInfo {
	env: string;
	status: "ok" | "ambiguous" | "no_account" | "own_account";
	accountId?: string;
	region?: string;
	files: string[];
}

/** Why a sibling environment cannot be chosen, or null when it can. */
export function shareTargetProblem(
	target: ShareTargetInfo,
	project: string,
): string | null {
	switch (target.status) {
		case "ok":
			return null;
		case "ambiguous":
			return `${target.files.join(" and ")} both declare environment ${target.env} of project ${project}; keep one.`;
		case "no_account":
			return `${target.files[0] ?? target.env} has no account_id, so its account is unknown.`;
		case "own_account":
			return "It is in this environment's own account; the bus needs no share there.";
	}
}

/**
 * The principal a share entry resolves to (`organization`, the OU id, or the
 * account id, with `env` resolved through `targets`), or null when it cannot
 * be resolved yet.
 */
export function sharePrincipal(
	entry: EventBusShare,
	targets: ShareTargetInfo[] | undefined,
): string | null {
	switch (shareEntryKind(entry)) {
		case "organization":
			return "organization";
		case "ou":
			return entry.ou || null;
		case "account":
			return entry.account || null;
		case "env": {
			const target = targets?.find(
				(t) => t.env === entry.env && t.status === "ok",
			);
			return target?.accountId ?? null;
		}
	}
}

/** A readable name for the principal of a share entry. */
export function shareEntryLabel(entry: EventBusShare): string {
	switch (shareEntryKind(entry)) {
		case "organization":
			return "the whole organization";
		case "ou":
			return `organizational unit ${entry.ou}`;
		case "account":
			return `account ${entry.account}`;
		case "env":
			return `environment ${entry.env}`;
	}
}

/**
 * The value error of one share entry, as app/events_enhanced.go refuses it:
 * a malformed OU or account id, this environment's own account, or an env
 * that cannot be shared with. `targets` undefined means not loaded yet.
 */
export function shareValueError(
	entry: EventBusShare,
	config: YamlInfrastructureConfig,
	targets: ShareTargetInfo[] | undefined,
): string | null {
	switch (shareEntryKind(entry)) {
		case "organization":
			return entry.organization === true ? null : "organization must be true.";
		case "ou":
			return SHARE_OU_PATTERN.test(entry.ou ?? "")
				? null
				: "An organizational unit id looks like ou-ab12-cd34ef56.";
		case "account":
			if (!SHARE_ACCOUNT_PATTERN.test(entry.account ?? ""))
				return "An AWS account id is 12 digits.";
			if (entry.account === config.account_id)
				return "This is this environment's own account; the bus needs no share there.";
			return null;
		case "env": {
			if (!entry.env) return "Choose an environment.";
			if (targets === undefined) return null;
			const target = targets.find((t) => t.env === entry.env);
			if (!target)
				return `No other environment of project ${config.project} has the name ${entry.env} and an account_id.`;
			return shareTargetProblem(target, config.project);
		}
	}
}

/**
 * The error for share entry `index` naming a principal an earlier entry
 * already names, or null.
 */
export function shareDuplicateError(
	share: EventBusShare[],
	index: number,
	targets: ShareTargetInfo[] | undefined,
): string | null {
	const principal = sharePrincipal(share[index], targets);
	if (principal === null) return null;
	const first = share.findIndex(
		(e, i) => i < index && sharePrincipal(e, targets) === principal,
	);
	if (first === -1) return null;
	return `Shared again: entry ${first + 1} already shares with ${shareEntryLabel(share[first])}. Give it one entry; Send and subscribe covers both.`;
}

/**
 * Entries of `before` that could subscribe and, in `after`, are gone or may
 * only send events. Their subscribers are revoked at the next deploy.
 */
export function shareSubscribersLost(
	before: EventBusShare[],
	after: EventBusShare[],
	targets: ShareTargetInfo[] | undefined,
): EventBusShare[] {
	const key = (e: EventBusShare) => sharePrincipalKey(e, targets);
	return before.filter(
		(old) =>
			isSubscribeCapable(old.access) &&
			!after.some((e) => isSubscribeCapable(e.access) && key(e) === key(old)),
	);
}

/**
 * What tells two share entries apart: the resolved principal, or the entry's
 * label while an `env` entry cannot be resolved yet.
 */
export function sharePrincipalKey(
	entry: EventBusShare,
	targets: ShareTargetInfo[] | undefined,
): string {
	return sharePrincipal(entry, targets) ?? shareEntryLabel(entry);
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
