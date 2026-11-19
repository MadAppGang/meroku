import { useEffect, useState } from "react";
import { infrastructureApi } from "../api/infrastructure";
import type { DeployedEvents } from "../utils/workloadEvents";
import { CONFIG_WARNINGS_DEBOUNCE_MS } from "./use-config-warnings";

export type LoadDeployedEvents = (
	env: string,
	signal: AbortSignal,
) => Promise<DeployedEvents>;

const defaultLoad: LoadDeployedEvents = (env, signal) =>
	infrastructureApi.getDeployedEvents(env, { signal });

/**
 * The deployed events state of `env`, or undefined while it is being read,
 * when the read failed, or for another env's answer. Undefined means unknown:
 * callers keep ordered queues and show no revocation notice.
 *
 * - An `env` change reads at once.
 * - A `refreshKey` change (bumped after each successful save) reads again
 *   `CONFIG_WARNINGS_DEBOUNCE_MS` later; the answer kept meanwhile is the
 *   previous one for the same env.
 */
export function useDeployedEvents(
	env: string | null,
	refreshKey: number,
	load: LoadDeployedEvents = defaultLoad,
): DeployedEvents | undefined {
	const [state, setState] = useState<{
		env: string;
		value: DeployedEvents;
	} | null>(null);
	const answeredEnv = state?.env ?? null;

	// biome-ignore lint/correctness/useExhaustiveDependencies: refreshKey only triggers a new read; answeredEnv picks the delay.
	useEffect(() => {
		if (!env) return;
		const controller = new AbortController();
		const timer = setTimeout(
			() => {
				load(env, controller.signal)
					.then((value) => {
						if (!controller.signal.aborted) setState({ env, value });
					})
					.catch((err: unknown) => {
						if (controller.signal.aborted) return;
						console.error("Failed to read the deployed events state:", err);
						setState(null);
					});
			},
			answeredEnv === env ? CONFIG_WARNINGS_DEBOUNCE_MS : 0,
		);
		return () => {
			clearTimeout(timer);
			controller.abort();
		};
	}, [env, refreshKey, load]);

	return state && state.env === env ? state.value : undefined;
}
