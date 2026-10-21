/**
 * properties-v3 — draft state machine.
 *
 * Owns the full draft/baseline/save lifecycle for one panel instance:
 *
 * - Keystrokes mutate the DRAFT only; nothing persists per keystroke.
 * - Saving applies the submitted channel's OPERATIONS to the baseline only
 *   (applyChanges is channel-scoped), so unsaved edits in other channels —
 *   including other rows of the same collection — remain dirty.
 * - Destructive saves require an immutable reviewed payload; edits are
 *   blocked while submitting and the confirmation is invalidated whenever
 *   the pending operations change OR the baseline changes (another
 *   channel's save, observation sync). Cancel keeps the draft.
 * - Failed saves retain draft and prior baseline; dismissing the error
 *   notice NEVER touches drafts.
 * - Changing nodeInstanceId resets everything AND invalidates in-flight
 *   async results (node-generation guard). Same-node object recreation
 *   (new definition/instance objects, same nodeInstanceId) never resets
 *   EDITABLE drafts, but DOES refresh observation values (AWS-assigned,
 *   telemetry, derived) from the incoming instance baseline.
 * - Saves are serialized per channel (an overlapping requestSave while a
 *   channel is submitting is ignored synchronously — it can never
 *   invalidate a completed in-flight persistence). One-shot actions use
 *   independent per-field tokens, so concurrent actions never invalidate
 *   each other or any save.
 * - A visibility-controller change that hides a DIRTY field resets that
 *   field to baseline and raises an explicit notice naming the discarded
 *   inactive changes (Clarification 4).
 */

import { useCallback, useEffect, useMemo, useReducer, useRef } from "react";
import { applyChanges, computeDiff } from "./diffEngine";
import {
	buildFieldIndex,
	deepEqual,
	isFieldLocked,
	resolveFieldValue,
} from "./helpers";
import type { FieldActionState } from "./primitives";
import type {
	AffectedResource,
	PersistenceChannel,
	PropertyChange,
	PropertyDiffError,
	PropertyDiffResult,
	PropertyFieldDefinition,
	PropertyPanelContext,
	PropertyPanelDefinition,
	PropertyPanelInstance,
	ValueKind,
} from "./types";
import { isFieldVisible } from "./visibilityEngine";

/* ---------------------------------------------------------------------------
 * Public adapter contracts (demo adapters in Storybook; no AWS calls)
 * ------------------------------------------------------------------------- */

export interface PanelSaveRequest {
	nodeInstanceId: string;
	channel: PersistenceChannel;
	/** Immutable reviewed operations for exactly this channel. */
	changes: readonly PropertyChange[];
}

export type PanelSaveResult = { ok: true } | { ok: false; message: string };

export interface PanelActionRequest {
	nodeInstanceId: string;
	fieldId: string;
}

export interface PanelDraftAdapters {
	/** Persist one channel. Throw or return { ok: false } to simulate failure. */
	onSaveChannel?: (
		request: PanelSaveRequest,
	) => Promise<PanelSaveResult> | PanelSaveResult;
	/** One-shot LIVE (action) command. Throw to surface a failure state. */
	onRunAction?: (request: PanelActionRequest) => Promise<void> | void;
}

/* ---------------------------------------------------------------------------
 * State
 * ------------------------------------------------------------------------- */

export interface PendingConfirmation {
	channel: PersistenceChannel;
	changes: readonly PropertyChange[];
	affectedResources: readonly AffectedResource[];
}

export interface HiddenChangesNotice {
	title: string;
	fieldLabels: readonly string[];
}

type ChannelStatus = "idle" | "submitting" | "error";

interface DraftState {
	baseline: Record<string, unknown>;
	/** Explicit draft overrides, flat by field.id. */
	overrides: Record<string, unknown>;
	channelStatus: Record<PersistenceChannel, ChannelStatus>;
	channelError: Partial<Record<PersistenceChannel, string>>;
	confirmation: PendingConfirmation | null;
	notice: HiddenChangesNotice | null;
	actionState: Record<string, FieldActionState>;
	actionError: Record<string, string | undefined>;
	savedChannels: PersistenceChannel[];
}

type DraftAction =
	| { type: "reset"; baseline: Record<string, unknown> }
	| {
			type: "syncObservations";
			baseline: Record<string, unknown>;
			fields: readonly PropertyFieldDefinition[];
	  }
	| {
			type: "setValue";
			field: PropertyFieldDefinition;
			value: unknown;
			fields: readonly PropertyFieldDefinition[];
	  }
	| { type: "requestConfirmation"; confirmation: PendingConfirmation }
	| { type: "cancelConfirmation" }
	| { type: "submitting"; channel: PersistenceChannel }
	| {
			type: "saveSuccess";
			channel: PersistenceChannel;
			/** Reviewed operations to merge INTO the current reducer baseline. */
			changes: readonly PropertyChange[];
	  }
	| { type: "saveFailure"; channel: PersistenceChannel; message: string }
	| { type: "dismissErrors" }
	| { type: "discardAll" }
	| { type: "dismissNotice" }
	| { type: "actionPending"; fieldId: string }
	| { type: "actionDone"; fieldId: string }
	| { type: "actionError"; fieldId: string; message: string };

const idleChannels = (): Record<PersistenceChannel, ChannelStatus> => ({
	configuration: "idle",
	live: "idle",
	layout: "idle",
});

function initState(baseline: Record<string, unknown>): DraftState {
	return {
		baseline,
		overrides: {},
		channelStatus: idleChannels(),
		channelError: {},
		confirmation: null,
		notice: null,
		actionState: {},
		actionError: {},
		savedChannels: [],
	};
}

/** Value kinds whose values are OBSERVED, never drafted by the user. */
const OBSERVATION_KINDS: ReadonlySet<ValueKind> = new Set([
	"aws-assigned",
	"telemetry",
	"derived",
]);

function reducer(state: DraftState, action: DraftAction): DraftState {
	switch (action.type) {
		case "reset":
			return initState(action.baseline);
		case "syncObservations": {
			// Refresh observed values from the incoming instance baseline WITHOUT
			// touching editable drafts/overrides (stored flat by field.id, which
			// wins alias resolution).
			const merged: Record<string, unknown> = { ...state.baseline };
			for (const field of action.fields) {
				if (!OBSERVATION_KINDS.has(field.valueKind)) continue;
				const incoming = resolveFieldValue(action.baseline, field);
				if (incoming !== undefined) merged[field.id] = incoming;
			}
			// A baseline change can alter what an armed confirmation's reviewed
			// `before` values mean — invalidate it (re-arm requires re-review).
			return { ...state, baseline: merged, confirmation: null };
		}
		case "setValue": {
			// Computed from CURRENT reducer state so batched dispatches never
			// lose earlier edits (no closure snapshots).
			const { field, value, fields } = action;
			const baseline = state.baseline;
			const overrides = state.overrides;
			const next: Record<string, unknown> = { ...overrides, [field.id]: value };

			// Hydrated records (baseline resolved by alias + overrides) so
			// visibility is evaluated against full values, never partial
			// override-only records — second-order controllers count too.
			const hydrate = (source: Record<string, unknown>) => {
				const hydrated: Record<string, unknown> = {};
				for (const entry of fields) {
					const resolved = resolveFieldValue(baseline, entry);
					if (resolved !== undefined) hydrated[entry.id] = resolved;
				}
				return { ...hydrated, ...source };
			};

			// Clarification 4: a visibility change that hides a DIRTY field
			// resets it to baseline and records an explicit notice. Resetting
			// can itself hide further dependents (chained controllers), so
			// iterate until the hidden-dirty set is stable.
			const discarded: string[] = [];
			for (let pass = 0; pass <= fields.length; pass += 1) {
				const beforeRecord = hydrate(overrides);
				const afterRecord = hydrate(next);
				let changed = false;
				for (const candidate of fields) {
					// Only fields that WERE visible and are NOW hidden.
					if (!isFieldVisible(candidate, beforeRecord, fields)) continue;
					if (isFieldVisible(candidate, afterRecord, fields)) continue;
					const current = next[candidate.id];
					if (current === undefined) continue;
					const before = resolveFieldValue(baseline, candidate);
					if (deepEqual(current, before)) continue;
					// Newly hidden AND dirty → reset to baseline value.
					if (before === undefined) delete next[candidate.id];
					else next[candidate.id] = before;
					discarded.push(candidate.label);
					changed = true;
				}
				if (!changed) break;
			}

			return {
				...state,
				overrides: next,
				// Any edit invalidates an armed confirmation payload (reconfirm).
				confirmation: null,
				notice:
					discarded.length > 0
						? {
								title:
									"Mode switch hid fields with pending edits — those inactive changes were reset to their saved values",
								fieldLabels: discarded,
							}
						: state.notice,
			};
		}
		case "requestConfirmation":
			return { ...state, confirmation: action.confirmation };
		case "cancelConfirmation":
			return { ...state, confirmation: null };
		case "submitting":
			return {
				...state,
				channelStatus: {
					...state.channelStatus,
					[action.channel]: "submitting",
				},
				channelError: { ...state.channelError, [action.channel]: undefined },
				confirmation: null,
			};
		case "saveSuccess":
			// Merge INSIDE the reducer against the CURRENT baseline so two
			// same-tick channel completions compose instead of the second
			// overwriting the first's merge (serialized source of truth).
			return {
				...state,
				baseline: applyChanges(state.baseline, action.changes, action.channel),
				channelStatus: { ...state.channelStatus, [action.channel]: "idle" },
				// Saved baseline changed → any armed confirmation is stale.
				confirmation: null,
				savedChannels: state.savedChannels.includes(action.channel)
					? state.savedChannels
					: [...state.savedChannels, action.channel],
			};
		case "saveFailure":
			return {
				...state,
				channelStatus: { ...state.channelStatus, [action.channel]: "error" },
				channelError: {
					...state.channelError,
					[action.channel]: action.message,
				},
			};
		case "dismissErrors":
			// Clears error surfacing only — drafts and baseline are untouched.
			return {
				...state,
				channelStatus: idleChannels(),
				channelError: {},
			};
		case "discardAll":
			return {
				...state,
				overrides: {},
				confirmation: null,
				notice: null,
				channelError: {},
				channelStatus: idleChannels(),
			};
		case "dismissNotice":
			return { ...state, notice: null };
		case "actionPending":
			return {
				...state,
				actionState: { ...state.actionState, [action.fieldId]: "pending" },
			};
		case "actionDone":
			return {
				...state,
				actionState: { ...state.actionState, [action.fieldId]: "idle" },
				actionError: { ...state.actionError, [action.fieldId]: undefined },
			};
		case "actionError":
			return {
				...state,
				actionState: { ...state.actionState, [action.fieldId]: "error" },
				actionError: { ...state.actionError, [action.fieldId]: action.message },
			};
	}
}

/* ---------------------------------------------------------------------------
 * Hook
 * ------------------------------------------------------------------------- */

export interface PanelDraftController {
	/** Draft values (baseline hydrated by alias resolution + overrides). */
	draft: Record<string, unknown>;
	baseline: Record<string, unknown>;
	diff: PropertyDiffResult;
	validationErrorsByField: Map<string, PropertyDiffError[]>;
	channelStatus: Record<PersistenceChannel, ChannelStatus>;
	channelError: Partial<Record<PersistenceChannel, string>>;
	/** Channels saved this session — saved ≠ deployed. */
	savedChannels: readonly PersistenceChannel[];
	/** Armed destructive confirmation (immutable payload), if any. */
	confirmation: PendingConfirmation | null;
	notice: HiddenChangesNotice | null;
	submitting: boolean;
	lockedFieldIds: ReadonlySet<string>;
	setValue: (field: PropertyFieldDefinition, value: unknown) => void;
	requestSave: (channel: PersistenceChannel) => void;
	confirmSave: () => void;
	cancelConfirmation: () => void;
	discardAll: () => void;
	dismissNotice: () => void;
	/** Dismiss save-error surfacing without touching any draft. */
	dismissErrors: () => void;
	actionState: Record<string, FieldActionState>;
	actionError: Record<string, string | undefined>;
	runAction: (fieldId: string) => void;
}

export interface UsePanelDraftStateArgs {
	definition: PropertyPanelDefinition;
	instance: PropertyPanelInstance;
	adapters?: PanelDraftAdapters;
}

export function usePanelDraftState({
	definition,
	instance,
	adapters,
}: UsePanelDraftStateArgs): PanelDraftController {
	const fields = useMemo(() => definition.fields ?? [], [definition]);
	const fieldIndex = useMemo(() => buildFieldIndex(fields), [fields]);

	const [state, dispatch] = useReducer(reducer, instance, (init) =>
		initState(init.baseline),
	);

	// Latest state for synchronous guards (overlapping requestSave/runAction).
	const stateRef = useRef(state);
	stateRef.current = state;

	// Node generation: bumped ONLY when nodeInstanceId changes; invalidates
	// every in-flight save and action belonging to the previous node.
	const nodeIdRef = useRef(instance.nodeInstanceId);
	const nodeGenerationRef = useRef(0);

	// Per-channel save tokens: saves on different channels and actions never
	// invalidate each other; a second submit on the SAME channel is rejected
	// synchronously, so an in-flight completed persistence is never dropped.
	const saveTokensRef = useRef<Record<PersistenceChannel, number>>({
		configuration: 0,
		live: 0,
		layout: 0,
	});
	// Independent per-field action tokens: concurrent actions coexist.
	const actionTokensRef = useRef<Record<string, number>>({});

	useEffect(() => {
		if (nodeIdRef.current !== instance.nodeInstanceId) {
			nodeIdRef.current = instance.nodeInstanceId;
			nodeGenerationRef.current += 1;
			dispatch({ type: "reset", baseline: instance.baseline });
			return;
		}
		// Same node, incoming object: refresh observed (AWS/telemetry/derived)
		// values only — editable drafts are preserved.
		if (instance.baseline !== stateRef.current.baseline) {
			dispatch({
				type: "syncObservations",
				baseline: instance.baseline,
				fields,
			});
		}
	}, [instance, fields]);

	const context = useMemo<PropertyPanelContext>(
		() => ({
			nodeInstanceId: instance.nodeInstanceId,
			lifecycleState: instance.lifecycleState,
			cloudWatchDestination: instance.cloudWatchDestination,
		}),
		[
			instance.nodeInstanceId,
			instance.lifecycleState,
			instance.cloudWatchDestination,
		],
	);

	const draft = useMemo(() => {
		const hydrated: Record<string, unknown> = {};
		for (const field of fields) {
			const value = resolveFieldValue(state.baseline, field);
			if (value !== undefined) hydrated[field.id] = value;
		}
		return { ...hydrated, ...state.overrides };
	}, [fields, state.baseline, state.overrides]);

	const diff = useMemo(
		() =>
			computeDiff(definition, state.baseline, draft, context, {
				resource: instance.resource,
			}),
		[definition, state.baseline, draft, context, instance.resource],
	);

	const validationErrorsByField = useMemo(() => {
		const map = new Map<string, PropertyDiffError[]>();
		for (const error of diff.validationErrors) {
			const list = map.get(error.fieldId) ?? [];
			list.push(error);
			map.set(error.fieldId, list);
		}
		return map;
	}, [diff]);

	const lockedFieldIds = useMemo(() => {
		const locked = new Set<string>();
		for (const field of fields) {
			if (isFieldLocked(field, instance.lifecycleState)) locked.add(field.id);
		}
		return locked;
	}, [fields, instance.lifecycleState]);

	const submitting =
		state.channelStatus.configuration === "submitting" ||
		state.channelStatus.live === "submitting" ||
		state.channelStatus.layout === "submitting";

	const setValue = useCallback(
		(field: PropertyFieldDefinition, value: unknown) => {
			if (stateRef.current.channelStatus.configuration === "submitting") return;
			if (stateRef.current.channelStatus.live === "submitting") return;
			if (stateRef.current.channelStatus.layout === "submitting") return;
			dispatch({ type: "setValue", field, value, fields });
		},
		[fields],
	);

	const submitChannel = useCallback(
		async (
			channel: PersistenceChannel,
			changes: readonly PropertyChange[],
		): Promise<void> => {
			// Serialize per channel SYNCHRONOUSLY: an overlapping submit on a
			// busy channel is ignored and never invalidates the in-flight one.
			if (stateRef.current.channelStatus[channel] === "submitting") return;
			const token = ++saveTokensRef.current[channel];
			const generation = nodeGenerationRef.current;
			const nodeId = instance.nodeInstanceId;
			dispatch({ type: "submitting", channel });
			try {
				const result = adapters?.onSaveChannel
					? await adapters.onSaveChannel({
							nodeInstanceId: nodeId,
							channel,
							changes,
						})
					: { ok: true as const };
				if (
					token !== saveTokensRef.current[channel] ||
					generation !== nodeGenerationRef.current
				) {
					return; // superseded by a newer same-channel save or node change
				}
				if (result && result.ok === false) {
					dispatch({ type: "saveFailure", channel, message: result.message });
					return;
				}
				// Baseline merge happens in the REDUCER against its current state —
				// two channels resolving in the same tick both compose.
				dispatch({ type: "saveSuccess", channel, changes });
			} catch (error) {
				if (
					token !== saveTokensRef.current[channel] ||
					generation !== nodeGenerationRef.current
				) {
					return;
				}
				dispatch({
					type: "saveFailure",
					channel,
					message: error instanceof Error ? error.message : "Save failed",
				});
			}
		},
		[adapters, instance.nodeInstanceId],
	);

	const requestSave = useCallback(
		(channel: PersistenceChannel) => {
			// Overlapping requestSave while this channel is submitting: ignore.
			if (stateRef.current.channelStatus[channel] === "submitting") return;
			const currentDiff = diff;
			const channelErrors = currentDiff.validationErrors.filter(
				(error) => error.persistenceChannel === channel,
			);
			if (channelErrors.length > 0) {
				dispatch({
					type: "saveFailure",
					channel,
					message: channelErrors.map((error) => error.message).join("; "),
				});
				return;
			}
			const changes = currentDiff.changes.filter(
				(change) => change.persistenceChannel === channel,
			);
			if (changes.length === 0) return;
			if (changes.some((change) => change.impact === "recreates-data-loss")) {
				dispatch({
					type: "requestConfirmation",
					confirmation: {
						channel,
						changes: changes.map((change) => ({ ...change })),
						affectedResources: currentDiff.affectedResources.map(
							(resource) => ({
								...resource,
							}),
						),
					},
				});
				return;
			}
			void submitChannel(channel, changes);
		},
		[diff, submitChannel],
	);

	const confirmSave = useCallback(() => {
		const pending = stateRef.current.confirmation;
		if (!pending) return;
		void submitChannel(pending.channel, pending.changes);
	}, [submitChannel]);

	const cancelConfirmation = useCallback(() => {
		dispatch({ type: "cancelConfirmation" });
	}, []);

	const discardAll = useCallback(() => {
		dispatch({ type: "discardAll" });
	}, []);

	const dismissNotice = useCallback(() => {
		dispatch({ type: "dismissNotice" });
	}, []);

	const dismissErrors = useCallback(() => {
		dispatch({ type: "dismissErrors" });
	}, []);

	const runAction = useCallback(
		(fieldId: string) => {
			const field = fieldIndex.get(fieldId);
			if (!field || !adapters?.onRunAction) return;
			const run = adapters.onRunAction;
			const token = (actionTokensRef.current[fieldId] ?? 0) + 1;
			actionTokensRef.current[fieldId] = token;
			const generation = nodeGenerationRef.current;
			const nodeId = instance.nodeInstanceId;
			dispatch({ type: "actionPending", fieldId });
			void (async () => {
				try {
					await run({ nodeInstanceId: nodeId, fieldId });
					if (
						token !== actionTokensRef.current[fieldId] ||
						generation !== nodeGenerationRef.current
					) {
						return;
					}
					dispatch({ type: "actionDone", fieldId });
				} catch (error: unknown) {
					if (
						token !== actionTokensRef.current[fieldId] ||
						generation !== nodeGenerationRef.current
					) {
						return;
					}
					dispatch({
						type: "actionError",
						fieldId,
						message: error instanceof Error ? error.message : "Action failed",
					});
				}
			})();
		},
		[adapters, fieldIndex, instance.nodeInstanceId],
	);

	return {
		draft,
		baseline: state.baseline,
		diff,
		validationErrorsByField,
		channelStatus: state.channelStatus,
		channelError: state.channelError,
		savedChannels: state.savedChannels,
		confirmation: state.confirmation,
		notice: state.notice,
		submitting,
		lockedFieldIds,
		setValue,
		requestSave,
		confirmSave,
		cancelConfirmation,
		discardAll,
		dismissNotice,
		dismissErrors,
		actionState: state.actionState,
		actionError: state.actionError,
		runAction,
	};
}
