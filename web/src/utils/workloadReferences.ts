import type {
	VaultConnection,
	YamlInfrastructureConfig,
} from "../types/yamlConfig";

/**
 * Workload references across the config.
 *
 * Several YAML fields name a workload — `backend`, or a service, scheduled task
 * or event task — by its name. `meroku generate` refuses a reference that names
 * nothing, so deleting or renaming a workload has to rewrite every one of them in
 * the same save. Each field that can hold a reference has one rewriter below;
 * the delete and rename paths run all of them, so a new field is covered once its
 * rewriter is in the list.
 *
 * A vault is referenced the same way, by the workloads' `vaults[].name`, so
 * deleting a vault runs the same rewriters. `mapName` takes the kind of name it
 * is asked about; a rewriter that holds workload names never passes a kind, and
 * the vault rewriter passes "vault", so deleting a vault named like a workload
 * touches only vault references.
 */

/** The name the backend has as a workload. */
export const BACKEND_WORKLOAD_NAME = "backend";

/** What a reference names. */
export type ReferenceKind = "workload" | "vault";

/**
 * Maps a referenced name to its new name, or `null` to remove the reference.
 * `kind` defaults to "workload".
 */
type NameMapper = (name: string, kind?: ReferenceKind) => string | null;

/**
 * Given the current config and a mapping function, returns the updates that
 * rewrite one field's references. The mapping returns the new name, or `null` to
 * remove the reference. A rewriter returns `{}` when nothing changes.
 */
type ReferenceRewriter = (
	config: YamlInfrastructureConfig,
	mapName: NameMapper,
) => Partial<YamlInfrastructureConfig>;

function mapNames(
	names: string[] | undefined,
	mapName: (name: string) => string | null,
): string[] | undefined {
	if (!names) return names;
	const out: string[] = [];
	for (const name of names) {
		const mapped = mapName(name);
		if (mapped !== null && !out.includes(mapped)) out.push(mapped);
	}
	return out;
}

/** `data_bucket.shared[].read_write` and `data_bucket.shared[].read`. */
const dataBucketSharedReferences: ReferenceRewriter = (config, mapName) => {
	const shared = config.data_bucket?.shared;
	if (!shared || shared.length === 0) return {};
	const rewritten = shared.map((folder) => ({
		...folder,
		read_write: mapNames(folder.read_write, mapName),
		read: mapNames(folder.read, mapName),
	}));
	if (JSON.stringify(rewritten) === JSON.stringify(shared)) return {};
	return { data_bucket: { ...config.data_bucket, shared: rewritten } };
};

function mapVaultConnections(
	connections: VaultConnection[] | undefined,
	mapName: NameMapper,
): VaultConnection[] | undefined {
	if (!connections) return connections;
	const out: VaultConnection[] = [];
	for (const connection of connections) {
		const mapped = mapName(connection.name, "vault");
		if (mapped !== null) out.push({ ...connection, name: mapped });
	}
	// An emptied list is dropped, so the YAML keeps no `vaults: []`.
	return out.length > 0 ? out : undefined;
}

function withMappedVaults<T extends { vaults?: VaultConnection[] }>(
	item: T,
	mapName: NameMapper,
): T {
	if (!item.vaults) return item;
	return { ...item, vaults: mapVaultConnections(item.vaults, mapName) };
}

/**
 * `workload.vaults[].name`, `services[].vaults[].name`,
 * `scheduled_tasks[].vaults[].name` and `event_processor_tasks[].vaults[].name`.
 */
const vaultConnectionReferences: ReferenceRewriter = (config, mapName) => {
	const updates: Partial<YamlInfrastructureConfig> = {};
	const same = (a: unknown, b: unknown) =>
		JSON.stringify(a) === JSON.stringify(b);

	if (config.workload?.vaults) {
		const vaults = mapVaultConnections(config.workload.vaults, mapName);
		if (!same(vaults, config.workload.vaults)) {
			updates.workload = { ...config.workload, vaults };
		}
	}
	if (config.services) {
		const services = config.services.map((s) => withMappedVaults(s, mapName));
		if (!same(services, config.services)) updates.services = services;
	}
	if (config.scheduled_tasks) {
		const tasks = config.scheduled_tasks.map((t) =>
			withMappedVaults(t, mapName),
		);
		if (!same(tasks, config.scheduled_tasks)) updates.scheduled_tasks = tasks;
	}
	if (config.event_processor_tasks) {
		const tasks = config.event_processor_tasks.map((t) =>
			withMappedVaults(t, mapName),
		);
		if (!same(tasks, config.event_processor_tasks)) {
			updates.event_processor_tasks = tasks;
		}
	}
	return updates;
};

/**
 * Event sources that name a workload: `<project>.<service>`,
 * `<project>.task.<task>` and `<project>.event.<task>` (ai_docs/EVENTS.md). A
 * rename points them at the new name. A delete leaves them: a source that names
 * nothing is valid and matches no event, and dropping the filter would widen the
 * subscription instead.
 */
function mapEventSource(
	project: string | undefined,
	source: string | undefined,
	mapName: (name: string) => string | null,
): string | undefined {
	if (!project || !source) return source;
	for (const prefix of [
		`${project}.task.`,
		`${project}.event.`,
		`${project}.`,
	]) {
		if (!source.startsWith(prefix)) continue;
		const name = source.slice(prefix.length);
		if (!name || name.includes(".")) continue;
		const mapped = mapName(name);
		return mapped === null ? source : `${prefix}${mapped}`;
	}
	return source;
}

/** `events.subscribes[].source` on the backend and services, and event-task rule sources. */
const eventSourceReferences: ReferenceRewriter = (config, mapName) => {
	const project = config.project;
	const updates: Partial<YamlInfrastructureConfig> = {};

	const backendSubs = config.workload?.events?.subscribes;
	if (backendSubs && backendSubs.length > 0) {
		const rewritten = backendSubs.map((s) => ({
			...s,
			source: mapEventSource(project, s.source, mapName),
		}));
		if (JSON.stringify(rewritten) !== JSON.stringify(backendSubs)) {
			updates.workload = {
				...config.workload,
				events: { ...config.workload?.events, subscribes: rewritten },
			} as YamlInfrastructureConfig["workload"];
		}
	}

	const services = config.services;
	if (services && services.length > 0) {
		const rewritten = services.map((svc) =>
			svc.events?.subscribes
				? {
						...svc,
						events: {
							...svc.events,
							subscribes: svc.events.subscribes.map((s) => ({
								...s,
								source: mapEventSource(project, s.source, mapName),
							})),
						},
					}
				: svc,
		);
		if (JSON.stringify(rewritten) !== JSON.stringify(services)) {
			updates.services = rewritten;
		}
	}

	const tasks = config.event_processor_tasks;
	if (tasks && tasks.length > 0) {
		const mapSources = (sources: string[] | undefined) =>
			sources?.map((s) => mapEventSource(project, s, mapName) ?? s);
		const rewritten = tasks.map((task) => ({
			...task,
			sources: mapSources(task.sources),
			rules: task.rules?.map((rule) => ({
				...rule,
				sources: mapSources(rule.sources) ?? [],
			})),
		}));
		if (JSON.stringify(rewritten) !== JSON.stringify(tasks)) {
			updates.event_processor_tasks = rewritten;
		}
	}
	return updates;
};

const REFERENCE_REWRITERS: ReferenceRewriter[] = [
	dataBucketSharedReferences,
	vaultConnectionReferences,
	eventSourceReferences,
];

function rewriteReferences(
	config: YamlInfrastructureConfig,
	mapName: NameMapper,
): Partial<YamlInfrastructureConfig> {
	let updates: Partial<YamlInfrastructureConfig> = {};
	for (const rewrite of REFERENCE_REWRITERS) {
		updates = { ...updates, ...rewrite({ ...config, ...updates }, mapName) };
	}
	return updates;
}

/** The updates that remove every reference to a deleted workload. */
export function removeWorkloadReferences(
	config: YamlInfrastructureConfig,
	name: string,
): Partial<YamlInfrastructureConfig> {
	return rewriteReferences(config, (n, kind = "workload") =>
		kind === "workload" && n === name ? null : n,
	);
}

/** The updates that point every reference to `from` at `to` instead. */
export function renameWorkloadReferences(
	config: YamlInfrastructureConfig,
	from: string,
	to: string,
): Partial<YamlInfrastructureConfig> {
	return rewriteReferences(config, (n, kind = "workload") =>
		kind === "workload" && n === from ? to : n,
	);
}

/** The updates that remove every workload's connection to a deleted vault. */
export function removeVaultReferences(
	config: YamlInfrastructureConfig,
	name: string,
): Partial<YamlInfrastructureConfig> {
	return rewriteReferences(config, (n, kind = "workload") =>
		kind === "vault" && n === name ? null : n,
	);
}

/**
 * Every workload name in the config: the backend first, then services,
 * scheduled tasks and event tasks, switched-off ones included (a reference to a
 * switched-off workload is kept; generate drops it with a warning).
 */
export function workloadNames(config: YamlInfrastructureConfig): string[] {
	return [
		BACKEND_WORKLOAD_NAME,
		...(config.services || []).map((s) => s.name),
		...(config.scheduled_tasks || []).map((t) => t.name),
		...(config.event_processor_tasks || []).map((t) => t.name),
	];
}

/** The names in `names` that are not a workload of `config`. */
export function unknownWorkloadNames(
	config: YamlInfrastructureConfig,
	names: string[] | undefined,
): string[] {
	const known = new Set(workloadNames(config));
	return (names || []).filter((n) => !known.has(n));
}
