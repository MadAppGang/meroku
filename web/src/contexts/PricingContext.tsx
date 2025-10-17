/**
 * Pricing Context
 *
 * Provides AWS pricing rates to all components in the application.
 * Fetches rates once on app initialization and caches them.
 *
 * Usage:
 *   const { rates, loading, error, refresh } = usePricing();
 *   if (rates) {
 *     const price = calculateAuroraPrice(config, rates);
 *   }
 *
 * @module PricingContext
 */

import {
	createContext,
	useContext,
	useEffect,
	useState,
	type ReactNode,
} from 'react';
import {
	fetchPricingRates,
	getCachedRates,
	type AWSPriceRates,
} from '../services/pricingService';

interface PricingContextValue {
	rates: AWSPriceRates | null;
	loading: boolean;
	error: Error | null;
	lastFetched: Date | null;
	refresh: () => Promise<void>;
}

const PricingContext = createContext<PricingContextValue | undefined>(
	undefined,
);

interface PricingProviderProps {
	children: ReactNode;
	region?: string; // Optional region override (defaults to us-east-1)
}

/**
 * Pricing Provider Component
 *
 * Wraps the application and provides pricing data to all child components.
 * Should be placed near the root of the component tree.
 *
 * @example
 * ```tsx
 * <PricingProvider>
 *   <App />
 * </PricingProvider>
 * ```
 */
export function PricingProvider({
	children,
	region = 'us-east-1',
}: PricingProviderProps) {
	// Try to get cached rates immediately
	const [rates, setRates] = useState<AWSPriceRates | null>(getCachedRates());
	const [loading, setLoading] = useState(false);
	const [error, setError] = useState<Error | null>(null);
	const [lastFetched, setLastFetched] = useState<Date | null>(null);

	/**
	 * Fetch pricing rates from backend
	 * Automatically called on mount and can be manually triggered
	 */
	const fetchRates = async () => {
		// Don't fetch if already loading
		if (loading) return;

		setLoading(true);
		setError(null);

		try {
			const newRates = await fetchPricingRates(region);
			setRates(newRates);
			setLastFetched(new Date());

			console.log(
				`[PricingContext] Fetched pricing rates for ${region} (source: ${newRates.source})`,
			);
		} catch (err) {
			const error = err as Error;
			setError(error);
			console.error('[PricingContext] Failed to fetch pricing rates:', error);

			// If we have cached rates, keep using them despite the error
			if (!rates) {
				console.error('[PricingContext] No cached rates available');
			}
		} finally {
			setLoading(false);
		}
	};

	// Fetch rates on mount if not cached
	useEffect(() => {
		if (!rates) {
			console.log('[PricingContext] No cached rates, fetching...');
			fetchRates();
		} else {
			console.log(
				'[PricingContext] Using cached rates:',
				rates.region,
				rates.source,
			);
		}
	}, []); // Only run once on mount

	// Auto-refresh every hour to keep rates current
	useEffect(() => {
		const interval = setInterval(
			() => {
				console.log('[PricingContext] Auto-refresh triggered');
				fetchRates();
			},
			60 * 60 * 1000,
		); // 1 hour

		return () => clearInterval(interval);
	}, [region]); // Re-create interval if region changes

	return (
		<PricingContext.Provider
			value={{
				rates,
				loading,
				error,
				lastFetched,
				refresh: fetchRates,
			}}
		>
			{children}
		</PricingContext.Provider>
	);
}

/**
 * Hook to access pricing context
 *
 * @throws Error if used outside PricingProvider
 * @returns Pricing context value
 *
 * @example
 * ```tsx
 * const { rates, loading, error } = usePricing();
 *
 * if (loading) return <LoadingSpinner />;
 * if (error) return <ErrorMessage error={error} />;
 * if (!rates) return null;
 *
 * const price = calculateAuroraPrice(config, rates);
 * ```
 */
export function usePricing(): PricingContextValue {
	const context = useContext(PricingContext);

	if (!context) {
		throw new Error('usePricing must be used within PricingProvider');
	}

	return context;
}

/**
 * Hook to safely access pricing rates with loading state
 *
 * Convenience hook that handles the common case of waiting for rates.
 * Returns undefined while loading, null if error, or the rates if available.
 *
 * @returns Rates if available, undefined if loading, null if error
 *
 * @example
 * ```tsx
 * const rates = usePricingRates();
 * if (!rates) return <LoadingOrError />;
 *
 * const price = calculateAuroraPrice(config, rates);
 * ```
 */
export function usePricingRates(): AWSPriceRates | null | undefined {
	const { rates, loading, error } = usePricing();

	if (loading) return undefined; // Still loading
	if (error && !rates) return null; // Error and no fallback
	return rates; // Available (may be from cache)
}
