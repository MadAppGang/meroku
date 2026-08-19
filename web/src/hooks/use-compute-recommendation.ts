import { useCallback, useEffect, useRef, useState } from "react";
import {
	type ComputePosture,
	type ComputeRecommendationResponse,
	infrastructureApi,
} from "../api/infrastructure";

export interface UseComputeRecommendationArgs {
	env: string;
	poolName?: string;
	posture: ComputePosture;
	networkMode?: string;
	amiFamily?: string;
	/** False keeps the hook idle — nothing is fetched and nothing is shown. */
	enabled?: boolean;
}

export interface UseComputeRecommendationResult {
	data: ComputeRecommendationResponse | null;
	/** True while a request is in flight, INCLUDING while `data` still holds the previous answer. */
	loading: boolean;
	error: string | null;
	refresh: () => void;
}

/**
 * The sizing recommendation for one pool, re-requested whenever any input that
 * changes the answer changes: the posture (FR-41), and the two fields a
 * not-yet-created pool has to be able to explore, `network_mode` and
 * `ami_family` (DEV-23, DEV-24).
 *
 * The previous answer stays on screen while the next one loads. Blanking the
 * panel on every posture click would make the control feel like it had failed,
 * and the recommendation costs an AWS round trip that can take seconds.
 */
export function useComputeRecommendation({
	env,
	poolName,
	posture,
	networkMode,
	amiFamily,
	enabled = true,
}: UseComputeRecommendationArgs): UseComputeRecommendationResult {
	const [data, setData] = useState<ComputeRecommendationResponse | null>(null);
	const [loading, setLoading] = useState(false);
	const [error, setError] = useState<string | null>(null);

	// Guards against an out-of-order response overwriting a newer one: a slow
	// performance-first request must not land on top of a fast cost-first one.
	const requestSeq = useRef(0);

	// One callback, called both by the effect below and by `refresh`. A reload
	// counter in the dependency array would say the same thing and would read to
	// a linter — correctly — as a dependency the effect never uses.
	const run = useCallback(() => {
		if (!enabled || !env) {
			setLoading(false);
			return;
		}

		requestSeq.current += 1;
		const seq = requestSeq.current;
		setLoading(true);

		infrastructureApi
			.getComputeRecommendation({
				env,
				posture,
				pool: poolName,
				networkMode,
				amiFamily,
			})
			.then((response) => {
				if (seq !== requestSeq.current) return;
				setData(response);
				setError(null);
			})
			.catch((err: unknown) => {
				if (seq !== requestSeq.current) return;
				setError(
					err instanceof Error
						? err.message
						: "Failed to load compute recommendation",
				);
			})
			.finally(() => {
				if (seq === requestSeq.current) setLoading(false);
			});
	}, [env, poolName, posture, networkMode, amiFamily, enabled]);

	useEffect(() => {
		run();
	}, [run]);

	return { data, loading, error, refresh: run };
}
