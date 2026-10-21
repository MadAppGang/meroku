/**
 * properties-v3 — archetype registry.
 *
 * Eight explicit composites; a section's archetype is declared in the schema
 * (PropertySectionDefinition.archetype), never inferred from labels or ids.
 */

import type { SectionArchetype } from "../types";
import { ActionListSection } from "./ActionListSection";
import { AutoscalingGroupSection } from "./AutoscalingGroupSection";
import { ContainerProcessSection } from "./ContainerProcessSection";
import { EnvironmentVariablesSection } from "./EnvironmentVariablesSection";
import { ReadonlyIdentitySection } from "./ReadonlyIdentitySection";
import { RegistryStrategySection } from "./RegistryStrategySection";
import type {
	ArchetypeRenderer,
	ArchetypeSectionProps,
	SectionRenderContext,
} from "./SectionRenderContext";
import { StandardFormSection } from "./StandardFormSection";
import { TelemetryMetricsSection } from "./TelemetryMetricsSection";

export type { ArchetypeRenderer, ArchetypeSectionProps, SectionRenderContext };

export const SECTION_ARCHETYPES: Readonly<
	Record<SectionArchetype, ArchetypeRenderer>
> = {
	standard: StandardFormSection,
	"container-process": ContainerProcessSection,
	"autoscaling-group": AutoscalingGroupSection,
	"registry-strategy": RegistryStrategySection,
	"environment-variables": EnvironmentVariablesSection,
	"readonly-identity": ReadonlyIdentitySection,
	"telemetry-metrics": TelemetryMetricsSection,
	"action-list": ActionListSection,
};

export { SECTION_ORDER, TAB_ORDER } from "../types";

export { SECTION_TITLES } from "./sectionTitles";
