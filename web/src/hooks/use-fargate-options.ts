import { useEffect, useState } from "react";
import {
	type FargateCPUOption,
	infrastructureApi,
} from "../api/infrastructure";

// Module-level cache so the fetch only happens once across all components
let cachedOptions: FargateCPUOption[] | null = null;
let fetchPromise: Promise<FargateCPUOption[]> | null = null;

export function useFargateOptions() {
	const [options, setOptions] = useState<FargateCPUOption[]>(
		cachedOptions || [],
	);

	useEffect(() => {
		if (cachedOptions) {
			setOptions(cachedOptions);
			return;
		}

		if (!fetchPromise) {
			fetchPromise = infrastructureApi
				.getFargateOptions()
				.then((res) => {
					cachedOptions = res.options;
					return res.options;
				})
				.catch((err) => {
					console.error("Failed to fetch Fargate options:", err);
					fetchPromise = null;
					return [];
				});
		}

		fetchPromise.then(setOptions);
	}, []);

	const getMemoryOptions = (cpu: number): number[] => {
		const opt = options.find((o) => o.cpu === cpu);
		return opt?.memoryOptions || [];
	};

	const formatMemory = (mb: number): string => {
		return mb >= 1024
			? `${(mb / 1024).toFixed(mb % 1024 === 0 ? 0 : 1)} GB`
			: `${mb} MB`;
	};

	return { options, getMemoryOptions, formatMemory };
}
