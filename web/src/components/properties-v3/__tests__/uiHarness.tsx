import type React from "react";
import { useState } from "react";
import { flushSync } from "react-dom";
import { createRoot, type Root } from "react-dom/client";
import { applyChanges } from "../diffEngine";
import { PropertyPanel } from "../index";
import type {
	PropertyFieldDefinition,
	PropertyPanelDefinition,
	PropertyPanelInstance,
} from "../types";

// Black-box harness: only public props, public engine, and adapter payloads.
// This is NOT a Storybook story and intentionally uses test-only native DOM.
const copy = <T,>(value: T): T => structuredClone(value);
type SaveRequest = Parameters<
	NonNullable<
		NonNullable<
			React.ComponentProps<typeof PropertyPanel>["adapters"]
		>["onSaveChannel"]
	>
>[0];
type ActionRequest = Parameters<
	NonNullable<
		NonNullable<
			React.ComponentProps<typeof PropertyPanel>["adapters"]
		>["onRunAction"]
	>
>[0];
type SaveResult = { ok: true } | { ok: false; message: string };
type Deferred<T> = {
	resolve: (value: T) => void;
	reject: (error: Error) => void;
};

function field(
	id: string,
	extra: Partial<PropertyFieldDefinition> = {},
): PropertyFieldDefinition {
	return {
		id,
		property: id,
		label: id,
		mutability: "TF",
		valueKind: "editable",
		controlType: "text",
		section: "source",
		...extra,
	};
}
const definition: PropertyPanelDefinition = {
	id: "black-box-ui",
	title: "Synthetic regression panel",
	fields: [
		field("image", { label: "Synthetic image" }),
		field("command", { label: "Synthetic command" }),
		field("string-list", {
			label: "Synthetic string list",
			controlType: "textarea",
			valueType: "string-array",
		}),
		field("dbname", {
			label: "Synthetic database name",
			property: "postgres.dbname",
			mutability: "TF-REPLACE",
			section: "storage-data",
			affectedResources: [{ id: "fake-db-1", name: "Synthetic PostgreSQL" }],
		}),
		field("environment", {
			label: "Synthetic environment",
			mutability: "TF-DEPLOY",
			section: "environment-secrets",
			controlType: "key-value-table",
			collection: {
				recordKeys: true,
				diffRules: { forceImpact: "rolling-restart" },
			},
		}),
		field("cpu", {
			label: "Synthetic CPU",
			mutability: "RUNTIME",
			valueKind: "telemetry",
			controlType: "telemetry-row",
			section: "health-metrics",
			resolution: "runtime",
		}),
		field("action-one", {
			label: "Synthetic action one",
			mutability: "LIVE (action)",
			valueKind: "action",
			controlType: "action-button",
			section: "actions",
		}),
		field("action-two", {
			label: "Synthetic action two",
			mutability: "LIVE (action)",
			valueKind: "action",
			controlType: "action-button",
			section: "actions",
		}),
	],
	sections: [
		{
			key: "source",
			archetype: "standard",
			fieldIds: ["image", "command", "string-list"],
		},
		{ key: "storage-data", archetype: "standard", fieldIds: ["dbname"] },
		{
			key: "environment-secrets",
			archetype: "environment-variables",
			fieldIds: ["environment"],
		},
		{
			key: "health-metrics",
			archetype: "telemetry-metrics",
			fieldIds: ["cpu"],
		},
		{
			key: "actions",
			archetype: "action-list",
			fieldIds: ["action-one", "action-two"],
		},
	],
};
let emptyEnvironment = false;
let publicDefinition: PropertyPanelDefinition | undefined;
let publicInstance: PropertyPanelInstance | undefined;
function instance(nodeInstanceId = "synthetic-node-A"): PropertyPanelInstance {
	return {
		nodeInstanceId,
		lifecycleState: "created",
		cloudWatchDestination: null,
		baseline: {
			image: "synthetic:old",
			command: "synthetic-command",
			"string-list": ["synthetic-first", "synthetic-second"],
			dbname: "synthetic_db",
			environment: emptyEnvironment ? {} : { SYNTHETIC_KEY: "fake-value" },
			cpu: 12,
		},
		telemetry: [
			{
				fieldId: "cpu",
				timestamp: "2026-09-09T01:02:03Z",
				resolution: "runtime",
			},
		],
		resource: { id: "fake-db-1", name: "Synthetic PostgreSQL" },
	};
}

let root: Root | undefined;
let host: HTMLDivElement | undefined;
let update:
	| React.Dispatch<React.SetStateAction<PropertyPanelInstance>>
	| undefined;
let saves: SaveRequest[] = [];
let actions: ActionRequest[] = [];
let pendingSaves: Deferred<SaveResult>[] = [];
let pendingActions: Deferred<void>[] = [];
let submittedBaselines: Record<string, unknown>[] = [];
let sourceInstances = new Map<string, PropertyPanelInstance>();

function Harness() {
	const [current, setCurrent] = useState(() =>
		publicInstance ? copy(publicInstance) : instance(),
	);
	update = setCurrent;
	sourceInstances.set(current.nodeInstanceId, copy(current));
	return (
		<div data-testid="v3-black-box-panel">
			<PropertyPanel
				definition={publicDefinition ?? definition}
				instance={current}
				initialTab="configuration"
				deployed={false}
				demoTelemetry
				onShowDeployInfo={() => undefined}
				adapters={{
					onSaveChannel: (request) => {
						saves.push(copy(request));
						const original = sourceInstances.get(request.nodeInstanceId);
						if (!original) throw new Error("unknown synthetic node");
						submittedBaselines.push(
							applyChanges(
								copy(original.baseline),
								request.changes,
								request.channel,
							),
						);
						return new Promise<SaveResult>((resolve, reject) => {
							pendingSaves.push({ resolve, reject });
						});
					},
					onRunAction: (request) => {
						actions.push(copy(request));
						return new Promise<void>((resolve, reject) => {
							pendingActions.push({ resolve, reject });
						});
					},
				}}
			/>
		</div>
	);
}

export function mount(
	options: {
		emptyEnvironment?: boolean;
		definition?: PropertyPanelDefinition;
		instance?: PropertyPanelInstance;
	} = {},
) {
	unmount();
	emptyEnvironment = options.emptyEnvironment ?? false;
	publicDefinition = options.definition;
	publicInstance = options.instance;
	saves = [];
	actions = [];
	pendingSaves = [];
	pendingActions = [];
	submittedBaselines = [];
	sourceInstances = new Map();
	host = document.createElement("div");
	host.id = "v3-regression-harness";
	document.body.appendChild(host);
	root = createRoot(host);
	flushSync(() => root?.render(<Harness />));
}
export function mountPublicPanel(
	definition: PropertyPanelDefinition,
	instance: PropertyPanelInstance,
) {
	mount({ definition, instance });
}
export function unmount() {
	root?.unmount();
	root = undefined;
	host?.remove();
	host = undefined;
	update = undefined;
}
export function records() {
	return copy({ saves, actions, submittedBaselines });
}
export function settleSave(index: number, result: SaveResult | "throw") {
	const pending = pendingSaves[index];
	if (!pending) throw new Error(`No pending save ${index}`);
	if (result === "throw") pending.reject(new Error("Synthetic save rejection"));
	else pending.resolve(result);
}
export function settleAction(index: number, fail = false) {
	const pending = pendingActions[index];
	if (!pending) throw new Error(`No pending action ${index}`);
	if (fail) pending.reject(new Error("Synthetic action rejection"));
	else pending.resolve();
}
export function updateTelemetry() {
	flushSync(() =>
		update?.((previous) => ({
			...copy(previous),
			baseline: { ...copy(previous.baseline), cpu: 73 },
			telemetry: [
				{
					fieldId: "cpu",
					timestamp: "2026-09-09T04:05:06Z",
					resolution: "runtime",
				},
			],
		})),
	);
}
export function switchNode() {
	flushSync(() =>
		update?.({
			...instance("synthetic-node-B"),
			baseline: {
				...instance().baseline,
				image: "synthetic:node-B",
				command: "node-B-command",
				dbname: "node_B_db",
			},
		}),
	);
}

// Mount the independent publicHookProbe bridge. Its hook invocation is based
// only on the compiler-emitted public declaration, never implementation code.
export function mountPublicHookProbe(Probe: React.ComponentType) {
	unmount();
	host = document.createElement("div");
	host.id = "v3-regression-harness";
	document.body.appendChild(host);
	root = createRoot(host);
	flushSync(() => root?.render(<Probe />));
}
