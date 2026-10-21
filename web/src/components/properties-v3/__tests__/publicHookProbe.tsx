import { useRef } from "react";
import { resolveFieldValue } from "../helpers";
import { usePanelDraftState } from "../index";
import type {
	PropertyFieldDefinition,
	PropertyPanelDefinition,
	PropertyPanelInstance,
} from "../types";

// Built only from the compiler-emitted public usePanelDraftState declaration.
// Two calls occur in one React event; separate events would miss lost updates.
const first: PropertyFieldDefinition = {
	id: "first",
	property: "first",
	label: "Batched first",
	mutability: "TF",
	valueKind: "editable",
	controlType: "text",
	section: "source",
};
const second: PropertyFieldDefinition = {
	...first,
	id: "second",
	property: "second",
	label: "Batched second",
};
const definition: PropertyPanelDefinition = {
	id: "synthetic-hook-probe",
	title: "Synthetic public hook probe",
	fields: [first, second],
	sections: [
		{ key: "source", archetype: "standard", fieldIds: ["first", "second"] },
	],
};
const instance: PropertyPanelInstance = {
	nodeInstanceId: "synthetic-batched-node",
	lifecycleState: "created",
	cloudWatchDestination: null,
	baseline: { first: "first-original", second: "second-original" },
};

const liveField: PropertyFieldDefinition = { ...second, mutability: "LIVE" };
const concurrentDefinition: PropertyPanelDefinition = {
	...definition,
	fields: [first, liveField],
};

export function ConcurrentSaveProbe() {
	const pending = useRef<Partial<Record<"configuration" | "live", () => void>>>(
		{},
	);
	const controller = usePanelDraftState({
		definition: concurrentDefinition,
		instance,
		adapters: {
			onSaveChannel: (request) =>
				new Promise<{ ok: true }>((resolve) => {
					if (request.channel === "layout")
						throw new Error("Unexpected layout request");
					pending.current[request.channel] = () => resolve({ ok: true });
				}),
		},
	});
	return (
		<div>
			<button
				type="button"
				onClick={() => {
					controller.setValue(first, "first-updated");
					controller.setValue(liveField, "second-updated");
				}}
			>
				Stage both channels
			</button>
			<button
				type="button"
				onClick={() => {
					controller.requestSave("configuration");
					controller.requestSave("live");
				}}
			>
				Submit both channels
			</button>
			<button
				type="button"
				onClick={() => {
					if (!pending.current.configuration || !pending.current.live)
						throw new Error("Both channel requests must be pending");
					pending.current.configuration();
					pending.current.live();
				}}
			>
				Resolve both saves together
			</button>
			<output data-testid="concurrent-save-state">
				{JSON.stringify({
					baseline: controller.baseline,
					draft: controller.draft,
					dirtyChannels: controller.diff.dirtyChannels,
					status: controller.channelStatus,
				})}
			</output>
		</div>
	);
}

const gate: PropertyFieldDefinition = {
	...first,
	id: "gate",
	property: "feature.gate",
	label: "Visibility gate",
	controlType: "boolean",
};
const controllerField: PropertyFieldDefinition = {
	...gate,
	id: "controller",
	property: "feature.controller",
	label: "Visibility controller",
	visibleWhen: { all: [{ kind: "equals", key: "feature.gate", value: true }] },
};
const dependent: PropertyFieldDefinition = {
	...first,
	id: "dependent",
	property: "feature.dependent",
	label: "Visibility dependent",
	visibleWhen: {
		all: [{ kind: "equals", key: "feature.controller", value: true }],
	},
};
const visibilityDefinition: PropertyPanelDefinition = {
	...definition,
	fields: [gate, controllerField, dependent],
	sections: [
		{
			key: "source",
			archetype: "standard",
			fieldIds: ["gate", "controller", "dependent"],
		},
	],
};
const visibilityInstance: PropertyPanelInstance = {
	...instance,
	nodeInstanceId: "synthetic-chained-visibility",
	baseline: {
		feature: { gate: true, controller: false, dependent: "baseline-dependent" },
	},
};

export function ChainedVisibilityProbe() {
	const controller = usePanelDraftState({
		definition: visibilityDefinition,
		instance: visibilityInstance,
	});
	return (
		<div>
			<button
				type="button"
				onClick={() => controller.setValue(controllerField, true)}
			>
				Enable dependent
			</button>
			<button
				type="button"
				onClick={() => controller.setValue(dependent, "dirty-dependent")}
			>
				Edit dependent
			</button>
			<button type="button" onClick={() => controller.setValue(gate, false)}>
				Hide controller chain
			</button>
			<output data-testid="chained-visibility-state">
				{JSON.stringify({
					values: {
						gate: resolveFieldValue(controller.draft, gate),
						controller: resolveFieldValue(controller.draft, controllerField),
						dependent: resolveFieldValue(controller.draft, dependent),
					},
					changes: controller.diff.changes,
					notice: controller.notice,
				})}
			</output>
		</div>
	);
}

export default function PublicHookProbe() {
	const controller = usePanelDraftState({ definition, instance });
	return (
		<div data-testid="v3-public-hook-probe">
			<label>
				Batched first
				<input
					value={String(controller.draft.first ?? "")}
					onChange={(event) => controller.setValue(first, event.target.value)}
				/>
			</label>
			<label>
				Batched second
				<input
					value={String(controller.draft.second ?? "")}
					onChange={(event) => controller.setValue(second, event.target.value)}
				/>
			</label>
			<button
				type="button"
				onClick={() => {
					controller.setValue(first, "first-updated");
					controller.setValue(second, "second-updated");
				}}
			>
				Batch public setValue
			</button>
		</div>
	);
}
