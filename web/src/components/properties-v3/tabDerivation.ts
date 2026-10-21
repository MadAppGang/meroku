/**
 * properties-v3 — pure tab derivation engine.
 *
 * Tabs and section placement are DERIVED from field declarations and panel
 * metadata. No definition declares a tab list or tab membership. Placement
 * follows the finite priority rules from the approved architecture:
 *
 * 1. Canvas-only panel → Layout only (no AWS actions or deploy controls).
 * 2. Derived field → Overview / Relationships (overrides mutability).
 * 3. Explicit logRole fields → Logs/<role> ONLY when a real CloudWatch
 *    destination exists. Ordinary logging configuration without that
 *    destination stays in its declared canonical section.
 * 4. AWS-assigned → Overview / Identity or Endpoints; telemetry →
 *    Operations / Health & metrics; rollout status (declared `deployments`
 *    section) → Operations / Deployments; actions → Operations / Actions.
 * 5. LIVE / LIVE-RESTART stored edits → Operations / Actions.
 * 6. Desired-state classes → Configuration in the declared canonical
 *    semantic section. Set-once fields become readonly rows after creation
 *    (still placed in Configuration; the diff engine excludes them).
 *
 * Standard panels always show Overview; every other tab requires visible
 * content. Sections render in canonical order (SECTION_ORDER).
 */

import { isFieldLocked } from "./helpers";
import type {
	ClosedSectionKey,
	DerivedSection,
	DerivedTab,
	DeriveTabsResult,
	PropertyFieldDefinition,
	PropertyPanelContext,
	PropertyPanelDefinition,
	PropertyTabId,
	SectionArchetype,
} from "./types";
import { SECTION_ORDER, TAB_ORDER } from "./types";
import { getAllFields, getVisibleFields } from "./visibilityEngine";

const OVERVIEW_SECTION_KEYS: ReadonlySet<string> = new Set(
	SECTION_ORDER.overview,
);

/** Placement actually used for a field; exported for tests/debugging. */
export interface FieldPlacement {
	tab: PropertyTabId;
	section: ClosedSectionKey;
}

/** Compute the ordered visible tabs and canonical sections for a panel. */
export function deriveTabs(
	definition: PropertyPanelDefinition,
	values: Record<string, unknown>,
	context: PropertyPanelContext,
): DeriveTabsResult {
	const visible = getVisibleFields(definition, values);
	const placement: Record<string, FieldPlacement> = {};
	const bucket = new Map<
		PropertyTabId,
		Map<ClosedSectionKey, PropertyFieldDefinition[]>
	>();

	const push = (
		field: PropertyFieldDefinition,
		tab: PropertyTabId,
		section: ClosedSectionKey,
	) => {
		placement[field.id] = { tab, section };
		let sections = bucket.get(tab);
		if (!sections) {
			sections = new Map();
			bucket.set(tab, sections);
		}
		const list = sections.get(section);
		if (list) {
			list.push(field);
		} else {
			sections.set(section, [field]);
		}
	};

	for (const field of visible) {
		push(field, ...placeField(field, definition, context));
	}

	// Rule 1: canvas-only panels get Layout only; standard panels always have Overview.
	let tabIds: PropertyTabId[];
	if (definition.canvasOnly) {
		tabIds = ["layout"];
	} else {
		tabIds = TAB_ORDER.filter(
			(tab) => tab === "overview" || (bucket.get(tab)?.size ?? 0) > 0,
		);
		// A canvas-only field on a standard panel still belongs to Layout.
		if (!tabIds.includes("layout") && (bucket.get("layout")?.size ?? 0) > 0) {
			tabIds = [...tabIds, "layout"];
		}
	}

	const tabs: DerivedTab[] = tabIds.map((tab) => {
		const sectionsMap =
			bucket.get(tab) ?? new Map<ClosedSectionKey, PropertyFieldDefinition[]>();
		const sections: DerivedSection[] = [];
		for (const key of SECTION_ORDER[tab]) {
			const fieldsInSection = sectionsMap.get(key);
			if (!fieldsInSection || fieldsInSection.length === 0) continue;
			sections.push({
				key,
				archetype: archetypeFor(tab, key, definition, fieldsInSection),
				fieldIds: fieldsInSection.map((f) => f.id),
			});
		}
		return { id: tab, sections };
	});

	return { tabs, placement };
}

/** Finite precedence chain — first matching rule wins. */
function placeField(
	field: PropertyFieldDefinition,
	definition: PropertyPanelDefinition,
	context: PropertyPanelContext,
): [PropertyTabId, ClosedSectionKey] {
	// Rule 1 companion: a canvas-only PANEL gets Layout only, before anything
	// else — no Overview/Relationships routing on canvas-only panels.
	if (definition.canvasOnly) {
		return ["layout", "layout"];
	}
	// Rule 2: derived outranks ANY mutability class, including UI — never
	// editable relationship info lands in Overview/Relationships.
	if (field.valueKind === "derived") {
		return ["overview", "relationships"];
	}
	// Ordinary canvas-only values AND UI-mutability fields always land in
	// Layout, even when the field's valueKind is not canvas-only.
	if (field.valueKind === "canvas-only" || field.mutability === "UI") {
		return ["layout", "layout"];
	}
	// Rule 3: explicit log roles gate on a REAL CloudWatch destination.
	if (field.logRole && hasCloudWatchDestination(context)) {
		switch (field.logRole) {
			case "viewer":
				return ["logs", "log-viewer"];
			case "destination":
				return ["logs", "destination"];
			case "retention":
				return ["logs", "retention"];
		}
	}
	// Rule 4: AWS-assigned / telemetry / actions.
	if (field.valueKind === "aws-assigned") {
		return [
			"overview",
			OVERVIEW_SECTION_KEYS.has(field.section) ? field.section : "identity",
		];
	}
	// Rollout status (declared `deployments` section) OUTRANKS the telemetry
	// valueKind check: a deployments-section telemetry field (e.g. the deployed
	// image tag) routes to Operations/Deployments, never Health & metrics.
	if (field.section === "deployments") {
		return ["operations", "deployments"];
	}
	if (field.valueKind === "telemetry") {
		return ["operations", "health-metrics"];
	}
	if (field.valueKind === "action") {
		return ["operations", "actions"];
	}
	// Rule 5: LIVE / LIVE-RESTART stored edits render as editors in Actions.
	if (
		field.mutability === "LIVE" ||
		field.mutability === "LIVE (action)" ||
		field.mutability === "LIVE-RESTART"
	) {
		return ["operations", "actions"];
	}
	// Rule 6: desired state in its declared canonical section (Overview keys
	// stay Overview, e.g. CREATE identity fields like region/env).
	if (OVERVIEW_SECTION_KEYS.has(field.section)) {
		return ["overview", field.section];
	}
	return ["configuration", field.section];
}

function hasCloudWatchDestination(context: PropertyPanelContext): boolean {
	return (
		typeof context.cloudWatchDestination === "string" &&
		context.cloudWatchDestination !== ""
	);
}

/**
 * Canonical archetype for a DERIVED destination section — used before
 * inheriting a contributing field's declared section style. A log viewer
 * derived from an `observability` field must render as telemetry metrics,
 * not as the source section's standard grid. Explicitly declared sections
 * (matched by key) still win outright.
 */
function canonicalArchetype(
	tab: PropertyTabId,
	key: ClosedSectionKey,
): SectionArchetype | null {
	if (tab === "logs" && key === "log-viewer") return "telemetry-metrics";
	if (tab === "operations" && key === "actions") return "action-list";
	if (tab === "operations" && key === "health-metrics")
		return "telemetry-metrics";
	return null;
}

/**
 * Archetype for a derived section: the declared section archetype wins;
 * then the canonical derived-destination archetype (log viewer, actions,
 * health metrics); otherwise the declared archetype of the contributing
 * fields' sections, with deterministic defaults per section key.
 * No label heuristics — placement comes from explicit logRole/section
 * metadata only.
 */
function archetypeFor(
	tab: PropertyTabId,
	key: ClosedSectionKey,
	definition: PropertyPanelDefinition,
	fields: readonly PropertyFieldDefinition[],
): SectionArchetype {
	const declared = definition.sections.find((section) => section.key === key);
	if (declared) return declared.archetype;
	const canonical = canonicalArchetype(tab, key);
	if (canonical) return canonical;
	const fieldDeclared = fields
		.map((field) =>
			definition.sections.find((section) =>
				section.fieldIds.includes(field.id),
			),
		)
		.find((section) => section);
	if (fieldDeclared) return fieldDeclared.archetype;
	return defaultArchetype(tab, key);
}

function defaultArchetype(
	tab: PropertyTabId,
	key: ClosedSectionKey,
): SectionArchetype {
	if (tab === "overview") return "readonly-identity";
	if (tab === "logs" && key === "log-viewer") return "telemetry-metrics";
	if (tab === "operations" && key === "actions") return "action-list";
	if (tab === "operations" && key === "health-metrics")
		return "telemetry-metrics";
	if (key === "environment-secrets") return "environment-variables";
	if (key === "compute-scaling") return "autoscaling-group";
	if (key === "source") return "container-process";
	if (key === "network-routing") return "container-process";
	return "standard";
}

export { getAllFields, isFieldLocked };
