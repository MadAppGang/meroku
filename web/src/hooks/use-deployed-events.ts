import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { infrastructureApi } from "../api/infrastructure";
import {
	deployedEventsReads,
	type LoadDeployedEvents,
} from "../utils/deployedEventsReads";
import type { DeployedEvents } from "../utils/workloadEvents";
import { CONFIG_WARNINGS_DEBOUNCE_MS } from "./use-config-warnings";

export type { LoadDeployedEvents } from "../utils/deployedEventsReads";

/**
 * Reads the deployed events state again, now, and resolves with that read's
 * own answer, even when a newer read started meanwhile. Resolves undefined
 * when the read fails, or when it was cancelled because the env changed or
 * the owner unmounted: unknown, so the caller keeps ordered queues.
 */
export type RereadDeployedEvents = () => Promise<DeployedEvents | undefined>;

export interface DeployedEventsState {
	/**
	 * The deployed events state of `env`, or undefined while it is being read,
	 * when the read failed, or for another env's answer. Undefined means
	 * unknown: callers keep ordered queues and show no revocation notice.
	 */
	deployed: DeployedEvents | undefined;
	/**
	 * A fresh read, for a decision that must not rest on an earlier answer:
	 * a deploy from the CLI changes what is deployed without telling the UI.
	 */
	reread: RereadDeployedEvents;
}

const defaultLoad: LoadDeployedEvents = (env, signal) =>
	infrastructureApi.getDeployedEvents(env, { signal });

/**
 * The deployed events state of `env`.
 *
 * - An `env` change reads at once.
 * - A `refreshKey` change (bumped after each successful save) reads again
 *   `CONFIG_WARNINGS_DEBOUNCE_MS` later; the answer kept meanwhile is the
 *   previous one for the same env.
 * - `reread` reads at once. Only the latest read started publishes the
 *   answer, but every read answers its own caller (deployedEventsReads).
 *   An env change or unmount cancels the rereads still running.
 */
export function useDeployedEvents(
	env: string | null,
	refreshKey: number,
	load: LoadDeployedEvents = defaultLoad,
): DeployedEventsState {
	const [state, setState] = useState<{
		env: string;
		value: DeployedEvents;
	} | null>(null);
	const answeredEnv = state?.env ?? null;
	const currentEnv = useRef(env);
	currentEnv.current = env;
	const rereads = useRef(new Set<AbortController>());

	const read = useMemo(
		() =>
			deployedEventsReads(
				load,
				() => currentEnv.current,
				(forEnv, value) => setState(value ? { env: forEnv, value } : null),
			),
		[load],
	);

	// biome-ignore lint/correctness/useExhaustiveDependencies: refreshKey only triggers a new read; answeredEnv picks the delay.
	useEffect(() => {
		if (!env) return;
		const controller = new AbortController();
		const timer = setTimeout(
			() => {
				void read(env, controller.signal);
			},
			answeredEnv === env ? CONFIG_WARNINGS_DEBOUNCE_MS : 0,
		);
		return () => {
			clearTimeout(timer);
			controller.abort();
		};
	}, [env, refreshKey, read]);

	// biome-ignore lint/correctness/useExhaustiveDependencies: env only marks when the rereads of the previous env are cancelled.
	useEffect(() => {
		const running = rereads.current;
		return () => {
			for (const controller of running) controller.abort();
			running.clear();
		};
	}, [env]);

	const reread = useCallback<RereadDeployedEvents>(() => {
		if (!env) return Promise.resolve(undefined);
		const controller = new AbortController();
		rereads.current.add(controller);
		return read(env, controller.signal).finally(() => {
			rereads.current.delete(controller);
		});
	}, [env, read]);

	return {
		deployed: state && state.env === env ? state.value : undefined,
		reread,
	};
}
