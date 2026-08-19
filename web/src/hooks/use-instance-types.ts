import { useCallback, useEffect, useState } from "react";
import {
	type ComputeInstanceTypeInfo,
	type ComputeInstanceTypesResponse,
	infrastructureApi,
} from "../api/infrastructure";

/**
 * The region's EC2 instance catalog, cached across every mounted picker.
 *
 * Built on the shape of `use-fargate-options.ts:7-37` — a module-level cache
 * plus a shared in-flight promise so N mounted components produce one request —
 * with one difference that matters: the Fargate CPU/memory table is
 * region-invariant, so a single global is correct there. This catalog is not.
 * Instance availability and price both vary by region, so the cache is keyed
 * rather than global (FR-38).
 *
 * The key is the ENVIRONMENT name, not the region. The endpoint takes `env` and
 * derives the region from `{env}.yaml` server-side, so the region is not known
 * until the first response has already arrived. Env is a strictly finer key
 * than region — two envs in one region each get their own entry, which costs
 * one extra request and makes it impossible for one environment's catalog to be
 * served to another.
 */
const catalogCache = new Map<string, ComputeInstanceTypesResponse>();
const inFlight = new Map<string, Promise<ComputeInstanceTypesResponse>>();

function load(
	env: string,
	force: boolean,
): Promise<ComputeInstanceTypesResponse> {
	if (!force) {
		const cached = catalogCache.get(env);
		if (cached) return Promise.resolve(cached);

		const pending = inFlight.get(env);
		if (pending) return pending;
	}

	const request = infrastructureApi
		.getComputeInstanceTypes(env, force ? { refresh: true } : undefined)
		.then((response) => {
			catalogCache.set(env, response);
			return response;
		})
		.finally(() => {
			inFlight.delete(env);
		});

	inFlight.set(env, request);
	return request;
}

export interface UseInstanceTypesResult {
	data: ComputeInstanceTypesResponse | null;
	instanceTypes: ComputeInstanceTypeInfo[];
	loading: boolean;
	error: string | null;
	/**
	 * Re-reads past the cache, sending `refresh=true` (FR-10) — for this click
	 * and this environment only. Later loads, including every environment
	 * switch, go back to the cached path.
	 */
	refresh: () => void;
	/** The catalog entry for a type, or undefined when the region has none (EC-14). */
	find: (instanceType: string) => ComputeInstanceTypeInfo | undefined;
}

export function useInstanceTypes(env: string): UseInstanceTypesResult {
	const [data, setData] = useState<ComputeInstanceTypesResponse | null>(
		() => catalogCache.get(env) ?? null,
	);
	const [loading, setLoading] = useState(!catalogCache.has(env));
	const [error, setError] = useState<string | null>(null);

	/**
	 * The environment a Refresh click is outstanding for, or null.
	 *
	 * Forcing belongs to the CLICK, not to the session and not to the hook. This
	 * holds the env name rather than a boolean or a counter because both of the
	 * simpler shapes leak: a counter that only ever increments makes every later
	 * env switch skip the client cache AND send `refresh=true`, which on the
	 * server bypasses both 24h caches and re-pays a full DescribeInstanceTypes +
	 * GetProducts (~7s cold) on a switch that should have cost a cache read. An
	 * env-tagged flag un-forces itself the moment `env` changes, with no reset
	 * effect and therefore no render in which the new env is still forced.
	 */
	const [forcedEnv, setForcedEnv] = useState<string | null>(null);

	useEffect(() => {
		if (!env) {
			setData(null);
			setLoading(false);
			return;
		}

		let active = true;
		const force = forcedEnv === env;
		const cached = force ? undefined : catalogCache.get(env);
		if (cached) {
			setData(cached);
			setLoading(false);
			setError(null);
			return;
		}

		setLoading(true);
		load(env, force)
			.then((response) => {
				if (!active) return;
				setData(response);
				setError(null);
			})
			.catch((err: unknown) => {
				if (!active) return;
				setError(
					err instanceof Error ? err.message : "Failed to load instance types",
				);
			})
			.finally(() => {
				if (active) setLoading(false);
				// Deliberately NOT guarded by `active`: a click whose request
				// outlives its env must still be spent, or switching back to that
				// env would re-force a cold refetch nobody asked for.
				//
				// Clearing re-runs this effect once with force=false. After a
				// success that is a cache read; after a failure it is one
				// unforced retry, which is the price of never stranding the
				// flag in the forced state.
				setForcedEnv(null);
			});

		return () => {
			active = false;
		};
	}, [env, forcedEnv]);

	const refresh = useCallback(() => {
		setForcedEnv(env);
	}, [env]);

	const instanceTypes = data?.instanceTypes ?? [];

	const find = useCallback(
		(instanceType: string) =>
			data?.instanceTypes.find((t) => t.instanceType === instanceType),
		[data],
	);

	return { data, instanceTypes, loading, error, refresh, find };
}
