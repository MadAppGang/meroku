/**
 * properties-v3 — public UI barrel.
 *
 * Core types/engines are re-exported for one-stop consumption by catalog,
 * fixtures, and story authors. The UI layer (panel, save bar, draft state,
 * primitives, archetypes) is owned by this module set; core files are
 * consumed read-only.
 */

export type {
	ArchetypeRenderer,
	ArchetypeSectionProps,
	SectionRenderContext,
} from "./archetypes";
export { SECTION_ARCHETYPES } from "./archetypes";
export { PropertyPanel, type PropertyPanelProps } from "./PropertyPanel";
export {
	PropertySaveBar,
	type PropertySaveBarProps,
} from "./PropertySaveBar";
export * from "./primitives";
export {
	type HiddenChangesNotice,
	type PanelActionRequest,
	type PanelDraftAdapters,
	type PanelDraftController,
	type PanelSaveRequest,
	type PanelSaveResult,
	type PendingConfirmation,
	type UsePanelDraftStateArgs,
	usePanelDraftState,
} from "./usePanelDraftState";
