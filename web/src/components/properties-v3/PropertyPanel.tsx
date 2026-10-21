/**
 * properties-v3 — panel container (React composition layer).
 *
 * Composes ONLY library primitives (`properties/`, read-only) and v3
 * primitives/archetypes — no raw styled DOM here. Tabs and section placement
 * come from the pure derivation engine; every declared section group renders
 * through its declared archetype (a section key may host several archetyped
 * groups, e.g. Operations/Actions).
 *
 * Draft lifecycle lives in usePanelDraftState; the save bar is channel-scoped
 * and the destructive confirm dialog renders the exact reviewed payload.
 */

import { useCallback, useMemo, useState } from "react";
import { PropertyGroup } from "../properties/PropertyLayouts";
import {
	PropertyPanelMeta,
	PropertyPanelShell,
	type PropertyPanelTab,
} from "../properties/PropertyPanelPrimitives";
import type { SectionRenderContext } from "./archetypes";
import { SECTION_ARCHETYPES } from "./archetypes";
import { SECTION_TITLES } from "./archetypes/sectionTitles";
import { buildFieldIndex } from "./helpers";
import { PropertySaveBar } from "./PropertySaveBar";
import { PropertyConfirmDialog, PropertyNotice, Stack } from "./primitives";
import { deriveTabs } from "./tabDerivation";
import "./primitives/property-panel-v3.css";
import type {
	PersistenceChannel,
	PropertyPanelDefinition,
	PropertyPanelInstance,
	PropertySectionDefinition,
	PropertyTabId,
	TelemetryReading,
} from "./types";
import {
	type PanelDraftAdapters,
	usePanelDraftState,
} from "./usePanelDraftState";

export interface PropertyPanelProps {
	/** Schema: fields, section groups, archetypes. Metadata only. */
	definition: PropertyPanelDefinition;
	/** Per-node data: baseline values, lifecycle, CW destination, telemetry. */
	instance: PropertyPanelInstance;
	theme?: "dark" | "light";
	initialTab?: PropertyTabId;
	onClose?: () => void;
	/** Demo adapters (Storybook). No Terraform/AWS calls from this component. */
	adapters?: PanelDraftAdapters;
	/**
	 * Whether the saved configuration is currently deployed. Saved ≠ deployed;
	 * null means the adapter cannot tell (demo).
	 */
	deployed?: boolean | null;
	/** Demo action: show deployment information (informational only). */
	onShowDeployInfo?: () => void;
	/** Marks telemetry rows as fixture/demo data. */
	demoTelemetry?: boolean;
}

export function PropertyPanel({
	definition,
	instance,
	theme = "dark",
	initialTab,
	onClose,
	adapters,
	deployed = null,
	onShowDeployInfo,
	demoTelemetry = false,
}: PropertyPanelProps) {
	const controller = usePanelDraftState({ definition, instance, adapters });
	const { draft, context } = {
		draft: controller.draft,
		context: useMemo(
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
		),
	};

	const derived = useMemo(
		() => deriveTabs(definition, draft, context),
		[definition, draft, context],
	);

	const [activeTabId, setActiveTabId] = useState<PropertyTabId>(
		() => initialTab ?? derived.tabs[0]?.id ?? "overview",
	);
	const activeTabIdSafe = derived.tabs.some((tab) => tab.id === activeTabId)
		? activeTabId
		: (derived.tabs[0]?.id ?? "overview");

	const telemetryByField = useMemo(() => {
		const map = new Map<string, TelemetryReading>();
		for (const reading of instance.telemetry ?? []) {
			map.set(reading.fieldId, reading);
		}
		return map;
	}, [instance.telemetry]);

	const ctx = useMemo<SectionRenderContext>(
		() => ({
			definition,
			instance,
			context,
			theme,
			draft: controller.draft,
			baseline: controller.baseline,
			lockedFieldIds: controller.lockedFieldIds,
			validationErrorsByField: controller.validationErrorsByField,
			submitting: controller.submitting,
			telemetryByField,
			demoTelemetry,
			fieldsById: buildFieldIndex(definition.fields ?? []),
			setValue: controller.setValue,
			runAction: controller.runAction,
			actionState: controller.actionState,
			actionError: controller.actionError,
		}),
		[
			definition,
			instance,
			context,
			theme,
			controller.draft,
			controller.baseline,
			controller.lockedFieldIds,
			controller.validationErrorsByField,
			controller.submitting,
			controller.setValue,
			controller.runAction,
			controller.actionState,
			controller.actionError,
			telemetryByField,
			demoTelemetry,
		],
	);

	// Canonical section containers in derived order; each renders its heading
	// ONCE (from the canonical key) and hosts every declared archetyped group
	// with visible fields for that key (Clarification 3 + heading-dedup fix).
	//
	// Robustness (final review): fields the derivation engine PLACED but no
	// declared section references (e.g. derived-only telemetry groups) render
	// through a fallback group using the DERIVED section archetype — every
	// visible derived field appears exactly once, never dropped. Sections with
	// no renderable groups are omitted entirely (no empty headings).
	const sectionsOnTab = useMemo(() => {
		const tab = derived.tabs.find(
			(candidate) => candidate.id === activeTabIdSafe,
		);
		if (!tab || !definition.sections) return [];
		return tab.sections
			.map((derivedSection) => {
				const groups: {
					section: PropertySectionDefinition;
					visibleFieldIds: string[];
				}[] = [];
				const claimed = new Set<string>();
				for (const section of definition.sections) {
					const visibleFieldIds = derivedSection.fieldIds.filter((fieldId) => {
						if (!section.fieldIds.includes(fieldId)) return false;
						// Exactly-once: a field id declared in several sections of the
						// same key renders only in the first declaring group.
						if (claimed.has(fieldId)) return false;
						claimed.add(fieldId);
						return true;
					});
					if (visibleFieldIds.length > 0)
						groups.push({ section, visibleFieldIds });
				}
				const remainder = derivedSection.fieldIds.filter(
					(fieldId) => !claimed.has(fieldId),
				);
				if (remainder.length > 0) {
					groups.push({
						section: {
							key: derivedSection.key,
							archetype: derivedSection.archetype,
							fieldIds: remainder,
						},
						visibleFieldIds: remainder,
					});
				}
				return { key: derivedSection.key, groups };
			})
			.filter((entry) => entry.groups.length > 0);
	}, [definition.sections, derived.tabs, activeTabIdSafe]);

	const views: PropertyPanelTab[] = derived.tabs.map((tab) => ({
		id: tab.id,
		label: tab.id.charAt(0).toUpperCase() + tab.id.slice(1),
	}));

	const handleSaveChannel = useCallback(
		(channel: PersistenceChannel) => controller.requestSave(channel),
		[controller],
	);

	const confirmation = controller.confirmation;

	return (
		<PropertyPanelShell
			name={definition.title}
			meta={
				<PropertyPanelMeta
					status={
						definition.canvasOnly
							? "Canvas only"
							: instance.lifecycleState === "created"
								? "Created"
								: "Not created"
					}
					context={
						definition.canvasOnly
							? undefined
							: (instance.cloudWatchDestination ?? undefined)
					}
				/>
			}
			ariaLabel={`${definition.title} properties`}
			theme={theme}
			views={views}
			activeView={activeTabIdSafe}
			onViewChange={(view) => setActiveTabId(view as PropertyTabId)}
			onClose={onClose}
			footer={
				<PropertySaveBar
					controller={controller}
					canvasOnly={definition.canvasOnly}
					deployed={deployed}
					onShowDeployInfo={onShowDeployInfo}
					onSaveChannel={handleSaveChannel}
					onDiscardAll={controller.discardAll}
				/>
			}
		>
			<Stack gap={12}>
				{controller.channelError &&
				Object.values(controller.channelError).some(Boolean) ? (
					<PropertyNotice
						title="Last save attempt failed"
						tone="danger"
						items={Object.entries(controller.channelError)
							.filter((entry): entry is [string, string] => Boolean(entry[1]))
							.map(([channel, message]) => `${channel}: ${message}`)}
						onDismiss={controller.dismissErrors}
					>
						Your edits are preserved — retry the save or discard all changes.
					</PropertyNotice>
				) : null}

				{controller.notice ? (
					<PropertyNotice
						title={controller.notice.title}
						items={[...controller.notice.fieldLabels]}
						onDismiss={controller.dismissNotice}
					/>
				) : null}

				{sectionsOnTab.map(({ key, groups }) => (
					<PropertyGroup key={key} title={SECTION_TITLES[key]}>
						<Stack gap={10}>
							{groups.map(({ section, visibleFieldIds }) => {
								const Renderer = SECTION_ARCHETYPES[section.archetype];
								return Renderer ? (
									<Renderer
										key={`${section.archetype}-${visibleFieldIds.join("~")}`}
										section={section}
										visibleFieldIds={visibleFieldIds}
										ctx={ctx}
									/>
								) : null;
							})}
						</Stack>
					</PropertyGroup>
				))}
			</Stack>

			<PropertyConfirmDialog
				open={Boolean(confirmation) && !controller.submitting}
				resourceName={definition.title}
				changes={confirmation?.changes ?? []}
				resources={confirmation?.affectedResources ?? []}
				fieldLabels={Object.fromEntries(
					(definition.fields ?? []).map((field) => [field.id, field.label]),
				)}
				uncreated={instance.lifecycleState !== "created"}
				onConfirm={controller.confirmSave}
				onCancel={controller.cancelConfirmation}
			/>
		</PropertyPanelShell>
	);
}
