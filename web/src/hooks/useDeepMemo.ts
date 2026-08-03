import { useRef } from "react";

/**
 * Deep comparison function for objects and arrays
 */
function deepEqual(a: unknown, b: unknown): boolean {
	if (a === b) return true;

	if (a == null || b == null) return false;

	if (typeof a !== "object" || typeof b !== "object") return false;

	// Both are non-null objects here, so indexing by string key is sound.
	const recordA = a as Record<string, unknown>;
	const recordB = b as Record<string, unknown>;

	const keysA = Object.keys(recordA);
	const keysB = Object.keys(recordB);

	if (keysA.length !== keysB.length) return false;

	for (const key of keysA) {
		if (!keysB.includes(key)) return false;
		if (!deepEqual(recordA[key], recordB[key])) return false;
	}

	return true;
}

/**
 * useMemo with deep equality comparison instead of reference equality
 */
export function useDeepMemo<T>(factory: () => T, deps: unknown[]): T {
	const ref = useRef<{ deps: unknown[]; value: T } | undefined>(undefined);

	if (!ref.current || !deepEqual(ref.current.deps, deps)) {
		ref.current = {
			deps,
			value: factory(),
		};
	}

	return ref.current.value;
}
