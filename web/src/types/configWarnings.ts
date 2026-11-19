/**
 * Configuration warnings: things in the env YAML worth fixing that do not
 * stop `meroku generate`. Mirrors `ConfigWarning` and `ConfigWarningReport`
 * in `app/config_warnings.go`, served by `GET /api/config-warnings?env=`.
 */

/**
 * The kind of item a warning is about. Singleton kinds (`env`, `backend`,
 * `valkey`, `event_bus`) always carry `name: ""`; the others carry the YAML
 * name verbatim, or `"#<1-based index>"` when the entry has no name.
 *
 * One literal per line: `app/config_warnings_kinds_test.go` reads this
 * declaration and compares it with the Go list.
 */
export type WarningKind =
	| "env"
	| "backend"
	| "service"
	| "scheduled_task"
	| "event_task"
	| "amplify_app"
	| "compute_pool"
	| "valkey"
	| "data_bucket"
	| "event_bus";

/**
 * The codes of the events warnings (`app/config_warnings.go`). `code` stays a
 * plain string because other rules add codes; these are the ones the events
 * UI keys on.
 */
export type EventWarningCode =
	| "event_subscription_all_types"
	| "event_produces_any"
	| "event_queue_unused"
	| "event_bus_toggle"
	| "event_bus_type_change"
	| "event_subscription_order_change"
	| "event_fifo_retired_unused"
	| "event_queue_deleted"
	| "event_share_revoke";

export interface WarningItem {
	kind: WarningKind;
	name: string;
}

export interface ConfigWarning {
	/** Stable rule id, snake_case. */
	code: string;
	item: WarningItem;
	/** One line, no newline. */
	title: string;
	/** Full text including what to do; may contain "\n". */
	detail: string;
	/** Absolute https URL; absent when there is no link. */
	docs?: string;
}

export interface ConfigWarningReport {
	/** The env name requested. */
	env: string;
	/** Always an array; empty when the YAML is clean. Env-wide items first. */
	warnings: ConfigWarning[];
	/** Present only when `meroku generate` would refuse this YAML. */
	configError?: string;
}
