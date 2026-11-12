import type { IAMStatement } from "../types/yamlConfig";

/**
 * Returns a copy of a service or task with its `policy` replaced. An empty list
 * removes the key, so a workload whose last statement was deleted saves without
 * an empty `policy: []` in its YAML.
 */
export function withPolicy<T extends { policy?: IAMStatement[] }>(
	item: T,
	policy: IAMStatement[],
): T {
	const { policy: _previous, ...rest } = item;
	return policy.length > 0 ? ({ ...rest, policy } as T) : (rest as T);
}
