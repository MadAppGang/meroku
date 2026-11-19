import { useCallback, useEffect, useRef, useState } from "react";
import { infrastructureApi } from "../api/infrastructure";
import type { DeployedEvents } from "../utils/workloadEvents";
import { CONFIG_WARNINGS_DEBOUNCE_MS } from "./use-config-warnings";

export type LoadDeployedEvents = (
	env: string,
	signal: AbortSignal,
) => Promise<DeployedEvents>;

/**
 * Reads the deployed events state again, now. Resolves undefined when the
 * read fails or the env changed meanwhile: unknown, so the caller keeps
 * ordered queues.
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
 * - `reread` reads at once. Only the latest read started writes the answer.
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
	const latestRead = useRef(0);
	const currentEnv = useRef(env);
	currentEnv.current = env;

	// read starts a read of env and settles the answer if it is still the
	// latest read of the env shown. undefined is a failed or superseded read.
	const read = useCallback(
		async (
			forEnv: string,
			signal: AbortSignal,
		): Promise<DeployedEvents | undefined> => {
			latestRead.current += 1;
			const id = latestRead.current;
			const current = () =>
				!signal.aborted &&
				id === latestRead.current &&
				currentEnv.current === forEnv;
			try {
				const value = await load(forEnv, signal);
				if (!current()) return undefined;
				setState({ env: forEnv, value });
				return value;
			} catch (err: unknown) {
				if (!current()) return undefined;
				console.error("Failed to read the deployed events state:", err);
				setState(null);
				return undefined;
			}
		},
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

	const reread = useCallback<RereadDeployedEvents>(() => {
		if (!env) return Promise.resolve(undefined);
		return read(env, new AbortController().signal);
	}, [env, read]);

	return {
		deployed: state && state.env === env ? state.value : undefined,
		reread,
	};
}
