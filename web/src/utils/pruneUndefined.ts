/**
 * A copy of a config value with every `undefined` object property and array
 * element removed, at any depth.
 *
 * Editors remove a YAML key by setting it to `undefined` (e.g. a service's
 * `neon` access). The YAML writer cannot represent `undefined`, so the save
 * path prunes it first and the key is simply absent from the file. `null` is
 * kept: it is a YAML value.
 */
export function pruneUndefined<T>(value: T): T {
	if (Array.isArray(value)) {
		return value
			.filter((item) => item !== undefined)
			.map((item) => pruneUndefined(item)) as T;
	}
	if (value !== null && typeof value === "object") {
		const pruned: Record<string, unknown> = {};
		for (const [key, item] of Object.entries(value)) {
			if (item !== undefined) pruned[key] = pruneUndefined(item);
		}
		return pruned as T;
	}
	return value;
}
