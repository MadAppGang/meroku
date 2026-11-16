import { useCallback, useEffect, useRef, useState } from "react";
import { infrastructureApi } from "../api/infrastructure";

export interface LevelPrice {
	monthlyPrice: number;
	hourlyPrice: number;
	details: Record<string, string>;
}

/**
 * A compute node's launch type, mirroring `NodePricing.Runtime` in
 * `app/api_pricing.go`. Absent on every non-compute node, and absent on any
 * response from a server older than schema v26.
 */
export type PricingRuntime = "fargate" | "ec2";

/**
 * The prefix `PricingResponse.nodes` keys capacity pools with, mirroring
 * `computePoolNodeKey` in `app/api_pricing.go`.
 */
export const COMPUTE_POOL_NODE_PREFIX = "compute_pool_";

/**
 * The pool that `synthesizeDefaultComputePool` injects when a workload asks for
 * EC2 and the environment declares no pools (FR-58). Mirrors
 * `defaultComputePoolName` in `app/api_pricing.go`.
 */
export const DEFAULT_COMPUTE_POOL = "default";

/** The pool name behind a `compute_pool_<name>` node key. */
export function computePoolNameFromKey(nodeKey: string): string {
	return nodeKey.startsWith(COMPUTE_POOL_NODE_PREFIX)
		? nodeKey.slice(COMPUTE_POOL_NODE_PREFIX.length)
		: nodeKey;
}

export interface NodePricing {
	serviceName: string;
	serviceType: string;
	levels: {
		startup: LevelPrice;
		scaleup: LevelPrice;
		highload: LevelPrice;
	};

	/**
	 * `"fargate"` or `"ec2"` on compute nodes, absent everywhere else. The two
	 * are billed in different units — Fargate per task, EC2 per instance-hour —
	 * so a caller that cannot tell them apart cannot render either honestly.
	 */
	runtime?: PricingRuntime;

	/**
	 * The node key that carries this node's cost. Non-empty exactly when every
	 * level here is $0 because the money is billed on that other node: today,
	 * an EC2-runtime service whose instances its capacity pool pays for.
	 *
	 * A node with this set is "billed via <that node>", NEVER free, and must
	 * never be added into a total — the pool it points at already holds the
	 * whole cost, so adding both bills one instance once per task placed on it.
	 */
	costAttributedTo?: string;

	/**
	 * The reverse edge, present on a capacity-pool node: the node keys whose
	 * cost this one carries.
	 */
	attributedFrom?: string[];
}

/** Mirrors EgressStrategy in app/egress_advisor.go. */
export type EgressStrategy = "public_ip" | "nat_gateway" | "nat_gateway_ha";

export interface EgressFootprint {
	Services: number;
	Tasks: number;
	AZs: number;
	TrafficGB: number;
}

/**
 * A non-blocking recommendation about how ECS tasks should reach the internet.
 *
 * Public IPv4 costs per task and nothing per GB; a NAT Gateway costs a flat
 * hourly rate and nothing per task. They cross at roughly 5 services (10 in
 * production, which runs one NAT per AZ). Mirrors EgressAdvice in
 * app/egress_advisor.go — see ai_docs/EGRESS_COST_MODEL.md.
 */
export interface EgressAdvice {
	footprint: EgressFootprint;
	current: EgressStrategy;
	recommended: EgressStrategy;
	shouldSwitch: boolean;
	threshold: number;
	currentMonthlyCost: number;
	switchedMonthlyCost: number;
	monthlySaving: number;
	servicesUntilSwitch: number;
	summary: string;
}

export interface PricingResponse {
	region: string;
	nodes: Record<string, NodePricing>;
	egress?: EgressAdvice;
	/**
	 * True when the server priced this response while loading the region's
	 * live EC2 prices in the background, or is loading them now. False while
	 * nothing loads, including the few minutes after a failed load. Pool
	 * figures come from the cache or the dated table until a load ends, and
	 * each pool level's `details.price_source` says which:
	 * `"aws_api"`, `"cached"` or `"fallback"`. Absent on older servers.
	 */
	pricesRefreshing?: boolean;
}

/**
 * Longest a background price load may run on the server (`liveEC2LoadTimeout`
 * in app/pricing_live_ec2.go). Change the two together.
 */
const REFRESH_LOAD_TIMEOUT_MS = 120_000;
/**
 * While prices are refreshing the hook asks again after 3s, doubling the wait
 * each time up to 15s (3, 6, 12, 15, 15, ...). It stops once it has waited
 * longer than the server's load timeout plus one last wait, so a load that
 * ends at its timeout is still picked up and the polling is always bounded.
 */
const REFRESH_POLL_FIRST_MS = 3000;
const REFRESH_POLL_MAX_MS = 15_000;
const REFRESH_POLL_BUDGET_MS = REFRESH_LOAD_TIMEOUT_MS + REFRESH_POLL_MAX_MS;

export function usePricing(
	environment: string | null,
	_refreshTrigger?: number,
) {
	const [pricing, setPricing] = useState<PricingResponse | null>(null);
	const [loading, setLoading] = useState(false);
	const [error, setError] = useState<Error | null>(null);

	// requestId changes on every fetch, environment change and unmount, so a
	// response or timer from an earlier one is dropped.
	const requestId = useRef(0);
	const pollTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

	const stopPolling = useCallback(() => {
		if (pollTimer.current !== null) {
			clearTimeout(pollTimer.current);
			pollTimer.current = null;
		}
	}, []);

	const fetchPricing = useCallback(async () => {
		stopPolling();
		const id = ++requestId.current;
		if (!environment) {
			setPricing(null);
			return;
		}

		// While the server reports a refresh, ask again quietly: the figures on
		// screen stay, no loading state, and errors leave them as they are.
		const pollWhileRefreshing = (
			data: PricingResponse,
			delayMs: number,
			waitedMs: number,
		) => {
			if (!data.pricesRefreshing || waitedMs >= REFRESH_POLL_BUDGET_MS) {
				return;
			}
			const wait = Math.min(delayMs, REFRESH_POLL_BUDGET_MS - waitedMs);
			pollTimer.current = setTimeout(async () => {
				pollTimer.current = null;
				try {
					const next = await infrastructureApi.getPricing(environment);
					if (id !== requestId.current) {
						return;
					}
					setPricing(next);
					pollWhileRefreshing(
						next,
						Math.min(delayMs * 2, REFRESH_POLL_MAX_MS),
						waitedMs + wait,
					);
				} catch {
					// Keep the figures already shown and stop asking.
				}
			}, wait);
		};

		setLoading(true);
		setError(null);
		try {
			const data = await infrastructureApi.getPricing(environment);
			if (id !== requestId.current) {
				return;
			}
			setPricing(data);
			pollWhileRefreshing(data, REFRESH_POLL_FIRST_MS, 0);
		} catch (err) {
			if (id !== requestId.current) {
				return;
			}
			setError(err as Error);
			setPricing(null);
		} finally {
			if (id === requestId.current) {
				setLoading(false);
			}
		}
	}, [environment, stopPolling]);

	useEffect(() => {
		fetchPricing();
		return () => {
			requestId.current++;
			stopPolling();
		};
	}, [fetchPricing, stopPolling]);

	return { pricing, loading, error, refreshPricing: fetchPricing };
}
