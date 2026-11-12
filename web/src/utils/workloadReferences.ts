import type { YamlInfrastructureConfig } from "../types/yamlConfig";

/**
 * Workload references across the config.
 *
 * Several YAML fields name a workload — `backend`, or a service, scheduled task
 * or event task — by its name. `meroku generate` refuses a reference that names
 * nothing, so deleting or renaming a workload has to rewrite every one of them in
 * the same save. Each field that can hold a reference has one rewriter below;
 * the delete and rename paths run all of them, so a new field is covered once its
 * rewriter is in the list.
 */

/** The name the backend has as a workload. */
export const BACKEND_WORKLOAD_NAME = "backend";

/**
 * Given the current config and a mapping function, returns the updates that
 * rewrite one field's references. The mapping returns the new name, or `null` to
 * remove the reference. A rewriter returns `{}` when nothing changes.
 */
type ReferenceRewriter = (
	config: YamlInfrastructureConfig,
	mapName: (name: string) => string | null,
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

const REFERENCE_REWRITERS: ReferenceRewriter[] = [dataBucketSharedReferences];

function rewriteReferences(
	config: YamlInfrastructureConfig,
	mapName: (name: string) => string | null,
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
	return rewriteReferences(config, (n) => (n === name ? null : n));
}

/** The updates that point every reference to `from` at `to` instead. */
export function renameWorkloadReferences(
	config: YamlInfrastructureConfig,
	from: string,
	to: string,
): Partial<YamlInfrastructureConfig> {
	return rewriteReferences(config, (n) => (n === from ? to : n));
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
