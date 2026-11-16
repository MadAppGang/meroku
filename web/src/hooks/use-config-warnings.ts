import { useCallback, useEffect, useRef, useState } from "react";
import { infrastructureApi } from "../api/infrastructure";
import type { ConfigWarningReport } from "../types/configWarnings";

/** Saves arrive per edit; wait this long after the last one before asking. */
export const CONFIG_WARNINGS_DEBOUNCE_MS = 300;

interface WarningsState {
	/** The env `report` and `error` describe. */
	env: string | null;
	report: ConfigWarningReport | null;
	loading: boolean;
	error: Error | null;
}

const EMPTY: WarningsState = {
	env: null,
	report: null,
	loading: false,
	error: null,
};

function isAbortError(err: unknown): boolean {
	return err instanceof DOMException && err.name === "AbortError";
}

export interface UseConfigWarningsResult {
	/** The latest good report for `env`; null before the first answer. */
	report: ConfigWarningReport | null;
	loading: boolean;
	/** The last request's failure. `report` keeps the previous good answer. */
	error: Error | null;
	/** Asks again now, without the debounce. */
	refresh: () => void;
}

/**
 * The configuration warnings for one environment.
 *
 * - An `env` change fetches at once and drops the previous env's report, so
 *   one env's warnings never paint another env's canvas.
 * - A `refreshKey` change (bumped after each successful save) fetches
 *   `CONFIG_WARNINGS_DEBOUNCE_MS` later; another change restarts the wait.
 * - Only the latest request's answer is applied: starting a request aborts
 *   the previous one, and an answer whose request id is no longer the latest
 *   is dropped even if the abort lost the race.
 * - A failed request keeps the last good report for the same env and is
 *   logged; it never blocks the canvas.
 */
export function useConfigWarnings(
	env: string | null,
	refreshKey: number,
): UseConfigWarningsResult {
	const [state, setState] = useState<WarningsState>(EMPTY);
	const requestIdRef = useRef(0);
	const controllerRef = useRef<AbortController | null>(null);
	const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
	const envRef = useRef(env);
	const lastRefreshKeyRef = useRef(refreshKey);

	const cancelPending = useCallback(() => {
		if (timerRef.current !== null) {
			clearTimeout(timerRef.current);
			timerRef.current = null;
		}
		controllerRef.current?.abort();
		controllerRef.current = null;
		requestIdRef.current += 1;
	}, []);

	const fetchFor = useCallback(
		(target: string) => {
			cancelPending();
			const controller = new AbortController();
			controllerRef.current = controller;
			const requestId = requestIdRef.current;
			setState((prev) =>
				prev.env === target
					? { ...prev, loading: true }
					: { env: target, report: null, loading: true, error: null },
			);
			infrastructureApi
				.getConfigWarnings(target, controller.signal)
				.then((report) => {
					if (requestId !== requestIdRef.current) return;
					controllerRef.current = null;
					setState({ env: target, report, loading: false, error: null });
				})
				.catch((err: unknown) => {
					if (requestId !== requestIdRef.current || isAbortError(err)) return;
					controllerRef.current = null;
					const error = err instanceof Error ? err : new Error(String(err));
					console.error("Failed to load configuration warnings:", error);
					setState((prev) =>
						prev.env === target
							? { ...prev, loading: false, error }
							: { env: target, report: null, loading: false, error },
					);
				});
		},
		[cancelPending],
	);

	// Environment change: ask at once and forget the old env's report.
	useEffect(() => {
		envRef.current = env;
		if (!env) {
			cancelPending();
			setState(EMPTY);
			return;
		}
		fetchFor(env);
		return cancelPending;
	}, [env, fetchFor, cancelPending]);

	// Saved edit: ask once the edits stop.
	useEffect(() => {
		if (lastRefreshKeyRef.current === refreshKey) return;
		lastRefreshKeyRef.current = refreshKey;
		const target = envRef.current;
		if (!target) return;
		if (timerRef.current !== null) clearTimeout(timerRef.current);
		timerRef.current = setTimeout(() => {
			timerRef.current = null;
			fetchFor(target);
		}, CONFIG_WARNINGS_DEBOUNCE_MS);
	}, [refreshKey, fetchFor]);

	const refresh = useCallback(() => {
		const target = envRef.current;
		if (target) fetchFor(target);
	}, [fetchFor]);

	const current = state.env === env ? state : EMPTY;
	return {
		report: current.report,
		loading: current.loading,
		error: current.error,
		refresh,
	};
}
