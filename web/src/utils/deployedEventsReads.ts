import type { DeployedEvents } from "./workloadEvents";

export type LoadDeployedEvents = (
	env: string,
	signal: AbortSignal,
) => Promise<DeployedEvents>;

/**
 * Reads of the deployed events state, with two separate rules:
 *
 * - Publication: only the latest read started may publish its answer (or
 *   its failure, as null) for the env shown, so an older answer never
 *   replaces a newer one.
 * - Validity: a read returns its own answer to its caller whenever it was
 *   not aborted and the env shown is still the one it read. A newer read,
 *   such as a routine refresh after a save, does not make a decision read's
 *   answer stale: both are fresh.
 *
 * A failed, aborted or other-env read returns undefined: unknown.
 */
export function deployedEventsReads(
	load: LoadDeployedEvents,
	shownEnv: () => string | null,
	publish: (env: string, value: DeployedEvents | null) => void,
): (env: string, signal: AbortSignal) => Promise<DeployedEvents | undefined> {
	let started = 0;
	return async (env, signal) => {
		started += 1;
		const id = started;
		const valid = () => !signal.aborted && shownEnv() === env;
		const publishable = () => valid() && id === started;
		try {
			const value = await load(env, signal);
			if (publishable()) publish(env, value);
			return valid() ? value : undefined;
		} catch (err: unknown) {
			if (publishable()) {
				console.error("Failed to read the deployed events state:", err);
				publish(env, null);
			}
			return undefined;
		}
	};
}
