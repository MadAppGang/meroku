import { useRef } from 'react';

/**
 * Deep comparison function for objects and arrays
 */
function deepEqual(a: any, b: any): boolean {
	if (a === b) return true;

	if (a == null || b == null) return false;

	if (typeof a !== 'object' || typeof b !== 'object') return false;

	const keysA = Object.keys(a);
	const keysB = Object.keys(b);

	if (keysA.length !== keysB.length) return false;

	for (const key of keysA) {
		if (!keysB.includes(key)) return false;
		if (!deepEqual(a[key], b[key])) return false;
	}

	return true;
}

/**
 * useMemo with deep equality comparison instead of reference equality
 */
export function useDeepMemo<T>(factory: () => T, deps: any[]): T {
	const ref = useRef<{ deps: any[]; value: T } | undefined>(undefined);

	if (!ref.current || !deepEqual(ref.current.deps, deps)) {
		ref.current = {
			deps,
			value: factory(),
		};
	}

	return ref.current.value;
}
