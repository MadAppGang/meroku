import { useCallback, useEffect, useState } from "react";
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

export interface PricingResponse {
	region: string;
	nodes: Record<string, NodePricing>;
}

export function usePricing(
	environment: string | null,
	_refreshTrigger?: number,
) {
	const [pricing, setPricing] = useState<PricingResponse | null>(null);
	const [loading, setLoading] = useState(false);
	const [error, setError] = useState<Error | null>(null);

	const fetchPricing = useCallback(async () => {
		if (!environment) {
			setPricing(null);
			return;
		}

		setLoading(true);
		setError(null);
		try {
			const data = await infrastructureApi.getPricing(environment);
			setPricing(data);
		} catch (err) {
			setError(err as Error);
			setPricing(null);
		} finally {
			setLoading(false);
		}
	}, [environment]);

	useEffect(() => {
		fetchPricing();
	}, [fetchPricing]);

	return { pricing, loading, error, refreshPricing: fetchPricing };
}
