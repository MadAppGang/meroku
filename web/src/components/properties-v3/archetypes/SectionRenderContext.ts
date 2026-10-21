/**
 * properties-v3 — shared archetype rendering context.
 *
 * Archetypes are pure composition: they receive schema, values, and
 * controller callbacks, and never own hidden local state. All state lives in
 * usePanelDraftState; all styled DOM lives in primitives/ or the shared
 * properties/ library.
 */

import type { ReactNode } from "react";
import type { FieldActionState } from "../primitives";
import type {
	PropertyDiffError,
	PropertyFieldDefinition,
	PropertyPanelContext,
	PropertyPanelDefinition,
	PropertyPanelInstance,
	PropertySectionDefinition,
	TelemetryReading,
} from "../types";

export interface SectionRenderContext {
	definition: PropertyPanelDefinition;
	instance: PropertyPanelInstance;
	context: PropertyPanelContext;
	theme: "dark" | "light";
	draft: Record<string, unknown>;
	baseline: Record<string, unknown>;
	lockedFieldIds: ReadonlySet<string>;
	validationErrorsByField: Map<string, PropertyDiffError[]>;
	/** True while an immutable payload is submitting — controls disable. */
	submitting: boolean;
	telemetryByField: ReadonlyMap<string, TelemetryReading>;
	/** Fixture/demo marker for telemetry rows (Storybook adapters). */
	demoTelemetry: boolean;
	fieldsById: ReadonlyMap<string, PropertyFieldDefinition>;
	setValue: (field: PropertyFieldDefinition, value: unknown) => void;
	runAction: (fieldId: string) => void;
	actionState: Record<string, FieldActionState>;
	actionError: Record<string, string | undefined>;
}

export interface ArchetypeSectionProps {
	/** Declared section group (key, title, archetype, fieldIds). */
	section: PropertySectionDefinition;
	/** Derived-visible field ids belonging to this group, in declared order. */
	visibleFieldIds: readonly string[];
	ctx: SectionRenderContext;
}

export type ArchetypeRenderer = (
	props: ArchetypeSectionProps,
) => ReactNode | Iterable<ReactNode> | null | undefined;
