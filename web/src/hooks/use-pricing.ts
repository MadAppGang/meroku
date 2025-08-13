import { useCallback, useEffect, useState } from "react";
import { infrastructureApi } from "../api/infrastructure";

export interface LevelPrice {
	monthlyPrice: number;
	hourlyPrice: number;
	details: Record<string, string>;
}

export interface NodePricing {
	serviceName: string;
	serviceType: string;
	levels: {
		startup: LevelPrice;
		scaleup: LevelPrice;
		highload: LevelPrice;
	};
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
