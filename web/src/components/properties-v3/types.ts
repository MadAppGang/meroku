/**
 * properties-v3 — public core types.
 *
 * Single source of truth for the v3 property panel contract.
 * Pure engines (tabDerivation, visibilityEngine, diffEngine) depend only on
 * this file and `helpers.ts`. No imports from properties/ or properties-v2/.
 *
 * Schema (definitions) and instance data are strictly separated:
 * - `PropertyPanelDefinition` declares metadata only (fields, sections, archetypes).
 * - `PropertyPanelInstance` supplies per-node data (baseline values, lifecycle
 *   state, CloudWatch destination, telemetry timestamps).
 */

/* ---------------------------------------------------------------------------
 * Impacts, kinds, states
 * ------------------------------------------------------------------------- */

/** The six user-facing impact categories. */
export type PropertyImpact =
	| "applies-now"
	| "restart-to-observe"
	| "rolling-restart"
	| "applies-on-deploy"
	| "recreates-data-loss"
	| "set-once";

/** What a field's value is, independent of its mutability. */
export type ValueKind =
	| "editable"
	| "aws-assigned"
	| "telemetry"
	| "derived"
	| "canvas-only"
	| "action";

/** Compatibility alias — exact requested name from the plan. */
export type PropertyValueKind = ValueKind;

/** How an AWS-assigned value can be resolved at runtime. */
export type ResolutionState = "known" | "provisioned" | "runtime";

/** Compatibility alias — exact requested name from the plan. */
export type PropertyRuntimeResolution = ResolutionState;

/** The eight declared section archetypes. */
export type SectionArchetype =
	| "container-process"
	| "autoscaling-group"
	| "registry-strategy"
	| "environment-variables"
	| "readonly-identity"
	| "telemetry-metrics"
	| "action-list"
	| "standard";

/** Derived tab ids. Definitions never declare tabs or tab membership. */
export type PropertyTabId =
	| "overview"
	| "configuration"
	| "operations"
	| "logs"
	| "layout";

/** Closed section vocabulary, fixed order per tab (see SECTION_ORDER). */
export type ClosedSectionKey =
	// Overview
	| "identity"
	| "endpoints"
	| "relationships"
	| "current-state"
	// Configuration
	| "naming-lifecycle"
	| "source"
	| "compute-scaling"
	| "network-routing"
	| "storage-data"
	| "security-access"
	| "environment-secrets"
	| "observability"
	| "advanced"
	// Operations
	| "deployments"
	| "health-metrics"
	| "actions"
	// Logs
	| "log-viewer"
	| "destination"
	| "retention"
	// Layout
	| "layout";

/** Canonical order of sections inside each tab. */
export const SECTION_ORDER: Record<PropertyTabId, readonly ClosedSectionKey[]> =
	{
		overview: ["identity", "endpoints", "relationships", "current-state"],
		configuration: [
			"naming-lifecycle",
			"source",
			"compute-scaling",
			"network-routing",
			"storage-data",
			"security-access",
			"environment-secrets",
			"observability",
			"advanced",
		],
		operations: ["deployments", "health-metrics", "actions"],
		logs: ["log-viewer", "destination", "retention"],
		layout: ["layout"],
	};

/** Canonical tab order used by deriveTabs. */
export const TAB_ORDER: readonly PropertyTabId[] = [
	"overview",
	"configuration",
	"operations",
	"logs",
	"layout",
];

/* ---------------------------------------------------------------------------
 * Mutability
 * ------------------------------------------------------------------------- */

/**
 * Documented mutability classes. Deterministically mapped to impacts via
 * MUTABILITY_IMPACT (see helpers.ts getImpactForMutability).
 */
export type MutabilityClass =
	| "CREATE"
	| "CREATE / TF-REPLACE"
	| "TF"
	| "TF-DEPLOY"
	| "TF-REPLACE"
	| "TF-REPLACE / TF-DEPLOY"
	| "LIVE"
	| "LIVE (action)"
	| "LIVE, create-only"
	| "LIVE-RESTART"
	| "RO"
	| "RUNTIME"
	/** Canvas-only UI preference (positions, labels, handles): applies immediately, never destructive. */
	| "UI";

/**
 * Where a change is persisted. The SaveBar renders one button per dirty
 * channel; saving one channel never touches the baselines of the others.
 */
export type PersistenceChannel = "configuration" | "live" | "layout";

/** Resource lifecycle relevant to CREATE locking. */
export type LifecycleState = "uncreated" | "created";

/* ---------------------------------------------------------------------------
 * Field definitions (schema)
 * ------------------------------------------------------------------------- */

/** Explicit identity of a resource affected by a field change. */
export interface AffectedResource {
	/** Stable resource identifier (synthetic in fixtures). */
	id: string;
	/** Human-readable resource name for destructive summaries. */
	name: string;
}

/** Conditional visibility predicate. Lookups accept field IDs and property keys. */
export type VisibilityCondition =
	| { kind: "equals"; key: string; value: unknown }
	| { kind: "notEquals"; key: string; value: unknown }
	| { kind: "exists"; key: string };

/** `all` (AND) and `any` (OR) groups; when both are present both must hold. */
export interface VisibilitySpec {
	all?: readonly VisibilityCondition[];
	any?: readonly VisibilityCondition[];
}

/** Per-collection diff semantics (value-vs-membership asymmetry). */
export interface CollectionDiffRules {
	/**
	 * Row properties whose VALUE edits use `valueEditImpact` instead of the
	 * field-level impact (e.g. SSM parameter `value` → restart-to-observe while
	 * membership add/remove → rolling-restart).
	 */
	valueProperties?: readonly string[];
	valueEditImpact?: PropertyImpact;
	membershipImpact?: PropertyImpact;
	/**
	 * Channel for membership (row add/remove) operations. Defaults to
	 * "configuration": row membership changes the task definition even when
	 * value edits on the same collection are live-channel.
	 */
	membershipChannel?: PersistenceChannel;
	/** Force ALL operations to this impact (plaintext task env vars → rolling-restart). */
	forceImpact?: PropertyImpact;
	valueEditChannel?: PersistenceChannel;
}

/**
 * Declared value type for typed editors (UI/catalog metadata). Engines use it
 * to validate pending drafts: a value that does not satisfy the declared type
 * produces a per-channel validation error that blocks that channel's save.
 * The UI may hold a raw draft string temporarily; only the error blocks.
 */
export type FieldValueType =
	| "string"
	| "number"
	| "boolean"
	| "string-array"
	| "number-array"
	| "json";

/** Per-column typed-editor metadata for tabular collections. */
export interface CollectionColumnSpec {
	/** Column key — the row-record property name. */
	key: string;
	/** Optional display label (UI). */
	label?: string;
	/** Declared cell value type; validated on pending drafts. */
	valueType: FieldValueType;
	/** Required cells must be present and non-empty; optional cells may be missing. */
	required?: boolean;
}

/** Declared collection identity. Key renames are always remove+add. */
export interface CollectionSpec {
	/** Array collections: record properties forming the row identity. */
	identityKeys?: readonly string[];
	/** Map-style collections: the record key itself is the identity. */
	recordKeys?: boolean;
	/** Typed column metadata for tabular editors (UI + draft validation). */
	columns?: readonly CollectionColumnSpec[];
	diffRules?: CollectionDiffRules;
}

export type FieldControlType =
	| "text"
	| "number"
	| "boolean"
	| "select"
	| "textarea"
	| "key-value-table"
	| "readonly-value"
	| "telemetry-row"
	| "action-button"
	| "layout-dimensions";

export interface FieldValidation {
	required?: boolean;
	pattern?: string;
	min?: number;
	max?: number;
	minLength?: number;
	maxLength?: number;
	/** Pure predicate returning an error message or null. */
	validate?: (value: unknown) => string | null;
}

/** Roles that route fields into the Logs tab when a CW destination exists. */
export type LogRole = "viewer" | "destination" | "retention";

export interface PropertyFieldDefinition {
	/** Stable field identifier (never renamed; used for lookups and drafts). */
	id: string;
	/**
	 * Documented property key, or ordered alias list (flat or dot-nested).
	 * Alias precedence: field.id → aliases in declared order (flat before
	 * nested within each alias). See resolveFieldValue in helpers.ts.
	 */
	property: string | readonly string[];
	label: string;
	mutability: MutabilityClass;
	valueKind: ValueKind;
	controlType: FieldControlType;
	/** Canonical semantic section (closed vocabulary). */
	section: ClosedSectionKey;
	/** Optional visibility predicate; absent means always visible. */
	visibleWhen?: VisibilitySpec;
	validation?: FieldValidation;
	/**
	 * Declared primitive/structured value type for typed editors. When present,
	 * pending draft values are validated against it (invalid values block the
	 * field's persistence channel). Absent: number/boolean controls infer the
	 * primitive type themselves; the engine does no type check.
	 */
	valueType?: FieldValueType;
	/** Declared collection identity for deep diffs. */
	collection?: CollectionSpec;
	/** Routes the field into Logs/<role> when a real CW destination exists. */
	logRole?: LogRole;
	/** Explicit affected-resource identities for destructive summaries. */
	affectedResources?: readonly AffectedResource[];
	/** Channel override; derived from mutability/valueKind when absent. */
	persistenceChannel?: PersistenceChannel;
	/** Optional option list for select controls. */
	options?: readonly { label: string; value: string }[];
	/** Per-field resolution state for readonly/aws-assigned rows (defaults to `known`). */
	resolution?: ResolutionState;
	/** Placeholder text (documentation for UI; engines ignore). */
	placeholder?: string;
	/** Free-form documentation string surfaced by UI tooltips. */
	description?: string;
}

/* ---------------------------------------------------------------------------
 * Section / panel definitions (schema)
 * ------------------------------------------------------------------------- */

export interface PropertySectionDefinition {
	key: ClosedSectionKey;
	title?: string;
	/** Every section explicitly declares one of the eight archetypes. */
	archetype: SectionArchetype;
	/** Field IDs belonging to this section, in declared order. */
	fieldIds: readonly string[];
}

export interface PropertyPanelDefinition {
	id: string;
	title: string;
	/** Canvas-only panels (group, dynamicGroup) get Layout only. */
	canvasOnly?: boolean;
	sections: readonly PropertySectionDefinition[];
	/**
	 * Full field catalog for this panel. Sections reference field IDs; this
	 * list carries the definitions engines evaluate. Pure engines accept
	 * definitions WITHOUT concrete runtime values — instances hydrate values.
	 */
	fields?: readonly PropertyFieldDefinition[];
}

/* ---------------------------------------------------------------------------
 * Panel instance data
 * ------------------------------------------------------------------------- */

/** Telemetry timestamps live here — NEVER inside baseline/draft values. */
export interface TelemetryReading {
	fieldId: string;
	timestamp: string;
	resolution: ResolutionState;
}

export interface PropertyPanelInstance {
	/** Stable node-instance ID. Changing it resets drafts. */
	nodeInstanceId: string;
	lifecycleState: LifecycleState;
	/** Baseline (saved) values, keyed by field.id / property aliases. */
	baseline: Record<string, unknown>;
	/**
	 * Nullable CloudWatch destination (log group ARN/name). Only a truthy
	 * destination enables the Logs tab. S3 access logging never qualifies.
	 */
	cloudWatchDestination: string | null;
	/** Telemetry readings, separate from values. */
	telemetry?: readonly TelemetryReading[];
	/** Default affected resource for this node when a field declares none. */
	resource?: AffectedResource;
}

/** Context threaded through engine calls. */
export interface PropertyPanelContext {
	nodeInstanceId: string;
	lifecycleState: LifecycleState;
	cloudWatchDestination: string | null;
}

/* ---------------------------------------------------------------------------
 * Derived tab structure (tabDerivation output)
 * ------------------------------------------------------------------------- */

export interface DerivedSection {
	key: ClosedSectionKey;
	archetype: SectionArchetype;
	fieldIds: string[];
}

export interface DerivedTab {
	id: PropertyTabId;
	sections: DerivedSection[];
}

export interface DeriveTabsResult {
	tabs: DerivedTab[];
	/** Placement actually used per field id (debugging + tests). */
	placement: Record<string, { tab: PropertyTabId; section: ClosedSectionKey }>;
}

/* ---------------------------------------------------------------------------
 * Diff engine
 * ------------------------------------------------------------------------- */

export type DiffOperation =
	| "set"
	| "add"
	| "remove"
	| "row-add"
	| "row-remove"
	| "row-update";

/** One typed identity pair of an array-collection row. */
export interface ArrayRowIdentityPair {
	key: string;
	value: string | number | boolean;
}

/**
 * Structured, literal change address for `applyChanges`. Segments and keys are
 * used VERBATIM — never interpolated into dotted paths, never re-parsed — so
 * collection keys containing `.`, `|`, `=`, `[`, `]` round-trip safely and
 * no prototype path can ever be traversed or written.
 */
export interface ChangeAddress {
	/**
	 * Storage path segments from the record root to the value container
	 * (field.id / alias, then alias path segments for nested storage).
	 */
	path: readonly string[];
	/** Map-style collection: the record key addressed literally. */
	recordKey?: string;
	/** Array collection: typed row identity (alternative to recordKey). */
	rowIdentity?: readonly ArrayRowIdentityPair[];
	/** Remaining literal cell path segments inside the addressed row. */
	cellPath?: readonly string[];
}

export interface PropertyChange {
	fieldId: string;
	/** Property path, e.g. `postgres.dbname` or `parameters.DB_HOST.value`. */
	propertyPath: string;
	/** Row identity value for collection row operations. */
	rowKey?: string;
	/**
	 * Structured literal address for `applyChanges`. When present it is used
	 * exclusively; `propertyPath`/`rowKey` remain as readable display forms.
	 * Changes without an address fall back to guarded legacy path addressing.
	 */
	address?: ChangeAddress;
	operation: DiffOperation;
	before: unknown;
	after: unknown;
	impact: PropertyImpact;
	persistenceChannel: PersistenceChannel;
	affectedResources: AffectedResource[];
}

export interface ChannelSummary {
	changeCount: number;
	worstImpact: PropertyImpact | null;
}

/** Per-channel validation error — recoverable, never blocks other channels. */
export interface PropertyDiffError {
	fieldId: string;
	propertyPath: string;
	persistenceChannel: PersistenceChannel;
	message: string;
}

export interface PropertyDiffResult {
	/** All visible, diffable changes across channels. */
	changes: PropertyChange[];
	dirtyChannels: PersistenceChannel[];
	channelSummaries: Partial<Record<PersistenceChannel, ChannelSummary>>;
	/** Worst impact across all dirty channels (aggregation order in helpers). */
	worstImpact: PropertyImpact | null;
	/** True when any dirty change is `recreates-data-loss`. */
	destructive: boolean;
	/** Resources explicitly named in destructive changes. */
	affectedResources: AffectedResource[];
	validationErrors: PropertyDiffError[];
}

/** Exact requested alias for test consumption. */
export type PendingDiff = PropertyDiffResult;

/** Channel-level dirty summary for the SaveBar. */
export interface DirtyState {
	dirty: boolean;
	channels: Record<PersistenceChannel, boolean>;
	changeCount: number;
	worstImpact: PropertyImpact | null;
}

/* ---------------------------------------------------------------------------
 * Impact metadata (consumed later by UI badges; pure data now)
 * ------------------------------------------------------------------------- */

export type ImpactTone =
	| "neutral"
	| "info"
	| "success"
	| "warning"
	| "danger"
	| "locked";

export interface PropertyImpactMetadata {
	impact: PropertyImpact;
	label: string;
	tone: ImpactTone;
	/** Worst-consequence wording for save-bar summaries. */
	summary: string;
	isDestructive: boolean;
	requiresConfirmation: boolean;
}
