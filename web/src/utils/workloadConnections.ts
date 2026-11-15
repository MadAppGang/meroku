import type { ComponentNode } from "../types";
import type {
	KmsKeyConnection,
	VaultConnection,
	YamlInfrastructureConfig,
} from "../types/yamlConfig";

/** The kinds of workload a connection can belong to. */
export type WorkloadKind =
	| "backend"
	| "service"
	| "scheduled-task"
	| "event-task";

/** A workload, as a canvas node names it. */
export interface WorkloadRef {
	kind: WorkloadKind;
	name: string;
}

/** The workload a canvas node stands for, or `null` for any other node. */
export function workloadOf(node: ComponentNode): WorkloadRef | null {
	switch (node.type) {
		case "backend":
			return { kind: "backend", name: "backend" };
		case "service":
			return { kind: "service", name: node.id.replace(/^service-/, "") };
		case "scheduled-task":
			return {
				kind: "scheduled-task",
				name: node.id.replace(/^scheduled-/, ""),
			};
		case "event-task":
			return { kind: "event-task", name: node.id.replace(/^event-/, "") };
		default:
			return null;
	}
}

/** The per-workload lists a workload panel owns. */
interface ConnectionLists {
	vaults: VaultConnection;
	kms_keys: KmsKeyConnection;
}

export type ConnectionListField = keyof ConnectionLists;

/** The workload's `field` list from the current config, `[]` when absent. */
export function workloadConnections<F extends ConnectionListField>(
	config: YamlInfrastructureConfig,
	workload: WorkloadRef,
	field: F,
): ConnectionLists[F][] {
	const pick = (item?: Partial<Record<ConnectionListField, unknown>>) =>
		(item?.[field] as ConnectionLists[F][] | undefined) || [];
	switch (workload.kind) {
		case "backend":
			return pick(config.workload);
		case "service":
			return pick(config.services?.find((s) => s.name === workload.name));
		case "scheduled-task":
			return pick(
				config.scheduled_tasks?.find((t) => t.name === workload.name),
			);
		case "event-task":
			return pick(
				config.event_processor_tasks?.find((t) => t.name === workload.name),
			);
	}
}

/**
 * The update that writes `next` as the workload's whole `field` list, into the
 * one entry of the one list the workload lives in. An empty list is dropped,
 * so the YAML keeps no `field: []`. `null` when the workload's list is absent.
 */
export function workloadConnectionsUpdate<F extends ConnectionListField>(
	config: YamlInfrastructureConfig,
	workload: WorkloadRef,
	field: F,
	next: ConnectionLists[F][],
): Partial<YamlInfrastructureConfig> | null {
	const value = next.length > 0 ? next : undefined;
	const withList = <T extends { name: string }>(item: T): T =>
		item.name === workload.name ? { ...item, [field]: value } : item;
	switch (workload.kind) {
		case "backend":
			return {
				workload: { [field]: value } as YamlInfrastructureConfig["workload"],
			};
		case "service":
			if (!config.services) return null;
			return { services: config.services.map(withList) };
		case "scheduled-task":
			if (!config.scheduled_tasks) return null;
			return { scheduled_tasks: config.scheduled_tasks.map(withList) };
		case "event-task":
			if (!config.event_processor_tasks) return null;
			return {
				event_processor_tasks: config.event_processor_tasks.map(withList),
			};
	}
}
