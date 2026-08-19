import {
	AlertCircle,
	Check,
	Cpu,
	Loader2,
	RefreshCw,
	Search,
	TrendingDown,
	X,
} from "lucide-react";
import { useCallback, useEffect, useId, useMemo, useState } from "react";
import {
	type ComputeInstanceTypeInfo,
	type ComputeSpotQuote,
	infrastructureApi,
} from "../api/infrastructure";
import { useInstanceTypes } from "../hooks/use-instance-types";
import { Alert, AlertDescription } from "./ui/alert";
import { Button } from "./ui/button";
import { Input } from "./ui/input";
import { Label } from "./ui/label";

/**
 * The spot endpoint refuses more than twenty types in one request (AC-8), so
 * the spot view quotes the top of the visible list rather than everything.
 */
const MAX_SPOT_TYPES = 20;

/** How many catalog rows are rendered before the list asks to be narrowed. */
const MAX_VISIBLE_ROWS = 40;

interface InstanceTypePickerProps {
	/**
	 * The environment, not the region: `/api/compute/instance-types` takes `env`
	 * and derives the region from `{env}.yaml`. The region comes back on the
	 * response and is what every message below is labelled with.
	 */
	env: string;
	selected: string[];
	onChange: (instanceTypes: string[]) => void;
	/**
	 * Whether this pool can buy spot at all. False hides the spot view entirely
	 * rather than showing prices that cannot apply to the pool as configured.
	 */
	spotOpen: boolean;
}

function formatMemory(memoryMiB: number): string {
	const gib = memoryMiB / 1024;
	return `${Number.isInteger(gib) ? gib : gib.toFixed(1)} GiB`;
}

function formatHourly(hourly: number): string {
	return `$${hourly.toFixed(hourly < 1 ? 4 : 3)}/hr`;
}

/**
 * FR-7: the saving is measured, never nominal. It is `1 − (median / onDemand)`
 * computed from the two live figures, and it does not exist when either is
 * missing — no hardcoded percentage appears anywhere in this file.
 */
function spotSaving(
	onDemand: number | null,
	median: number | null,
): number | null {
	if (onDemand === null || median === null || onDemand <= 0) return null;
	return 1 - median / onDemand;
}

export function InstanceTypePicker({
	env,
	selected,
	onChange,
	spotOpen,
}: InstanceTypePickerProps) {
	const uid = useId();
	const { data, instanceTypes, loading, error, refresh, find } =
		useInstanceTypes(env);

	const [search, setSearch] = useState("");
	const [showSpot, setShowSpot] = useState(false);
	const [spotQuotes, setSpotQuotes] = useState<Map<string, ComputeSpotQuote>>(
		new Map(),
	);
	const [spotLoading, setSpotLoading] = useState(false);
	const [spotError, setSpotError] = useState<string | null>(null);
	const [spotNotice, setSpotNotice] = useState<string | null>(null);

	const region = data?.region ?? "";

	const toggle = useCallback(
		(instanceType: string) => {
			if (selected.includes(instanceType)) {
				onChange(selected.filter((t) => t !== instanceType));
			} else {
				onChange([...selected, instanceType]);
			}
		},
		[selected, onChange],
	);

	const matches = useMemo(() => {
		const needle = search.trim().toLowerCase();
		const pool = needle
			? instanceTypes.filter((t) => t.instanceType.includes(needle))
			: instanceTypes;
		// Selected types sort first so a chosen type never scrolls out of reach
		// behind a filter that no longer matches it.
		return [...pool].sort((a, b) => {
			const aSel = selected.includes(a.instanceType) ? 0 : 1;
			const bSel = selected.includes(b.instanceType) ? 0 : 1;
			if (aSel !== bSel) return aSel - bSel;
			return a.instanceType.localeCompare(b.instanceType);
		});
	}, [instanceTypes, search, selected]);

	const visible = matches.slice(0, MAX_VISIBLE_ROWS);

	// The set of types the spot view quotes: whatever is on screen, capped at the
	// endpoint's limit. Joined into a string so the effect below re-runs on a
	// real change rather than on every re-render's new array identity.
	const spotTypesKey = useMemo(
		() =>
			visible
				.slice(0, MAX_SPOT_TYPES)
				.map((t) => t.instanceType)
				.join(","),
		[visible],
	);

	useEffect(() => {
		if (!showSpot || !spotOpen || !env || !spotTypesKey) {
			return;
		}

		let active = true;
		setSpotLoading(true);
		infrastructureApi
			.getComputeSpotPrices(env, spotTypesKey.split(","))
			.then((response) => {
				if (!active) return;
				setSpotQuotes(new Map(response.prices.map((p) => [p.instanceType, p])));
				// A degraded spot read arrives as a 200 with every quote marked
				// unavailable, so without this the rows would read "no spot market"
				// — a claim about the market — when the truth is that we could not
				// look. The notice says which.
				setSpotNotice(response.notice);
				setSpotError(null);
			})
			.catch((err: unknown) => {
				if (!active) return;
				setSpotError(
					err instanceof Error ? err.message : "Failed to load spot prices",
				);
			})
			.finally(() => {
				if (active) setSpotLoading(false);
			});

		return () => {
			active = false;
		};
	}, [showSpot, spotOpen, env, spotTypesKey]);

	// EC-14: a configured type the region's catalog does not list. Warn, never
	// refuse — the ASG's mixed_instances_policy tolerates it as long as one type
	// is available — and stay silent when availability was never verified.
	const unavailable = useMemo(() => {
		if (
			!data ||
			!data.availabilityVerified ||
			data.instanceTypes.length === 0
		) {
			return [];
		}
		return selected.filter((t) => !find(t));
	}, [data, selected, find]);

	const renderRow = (info: ComputeInstanceTypeInfo) => {
		const isSelected = selected.includes(info.instanceType);
		const quote = spotQuotes.get(info.instanceType);
		const saving = spotSaving(info.onDemandHourly, quote?.median ?? null);

		return (
			<button
				type="button"
				key={info.instanceType}
				onClick={() => toggle(info.instanceType)}
				aria-pressed={isSelected}
				className={`w-full text-left px-3 py-2 rounded-md border transition-colors ${
					isSelected
						? "bg-blue-900/30 border-blue-600"
						: "bg-gray-900 border-gray-700 hover:border-gray-500"
				}`}
			>
				<div className="flex items-start justify-between gap-3">
					<div className="min-w-0">
						<div className="flex items-center gap-2">
							{isSelected ? (
								<Check className="w-3.5 h-3.5 text-blue-400 shrink-0" />
							) : (
								<span className="w-3.5 h-3.5 shrink-0" />
							)}
							<span className="font-mono text-sm text-gray-200">
								{info.instanceType}
							</span>
							{!info.currentGeneration && (
								<span className="text-xs text-gray-500">previous gen</span>
							)}
							{info.burstable && (
								<span className="text-xs text-yellow-500">burstable</span>
							)}
						</div>
						<div className="mt-1 ml-6 text-xs text-gray-400">
							{info.vcpu} vCPU · {formatMemory(info.memoryMiB)} ·{" "}
							{info.architectures.join(", ") || "unknown arch"} ·{" "}
							{info.networkPerformance}
							{info.gpuCount > 0 && (
								<>
									{" · "}
									<span className="text-purple-400">
										{info.gpuCount}× {info.gpuName ?? "GPU"}
									</span>
								</>
							)}
						</div>
					</div>

					<div className="text-right shrink-0">
						{info.onDemandHourly === null ? (
							<span className="text-xs text-gray-500">price unknown</span>
						) : (
							<span className="text-sm text-gray-200">
								{formatHourly(info.onDemandHourly)}
							</span>
						)}
						{showSpot && spotOpen && (
							<div className="text-xs mt-0.5">
								{quote?.spotAvailable && quote.median !== null ? (
									<span className="text-green-400">
										spot {formatHourly(quote.median)}
										{saving !== null && (
											<>
												{" "}
												<span className="text-green-500">
													−{(saving * 100).toFixed(0)}%
												</span>
											</>
										)}
									</span>
								) : quote ? (
									<span className="text-gray-500">
										{spotNotice ? "spot unknown" : "no spot market"}
									</span>
								) : (
									<span className="text-gray-600">—</span>
								)}
							</div>
						)}
					</div>
				</div>
			</button>
		);
	};

	return (
		<div className="space-y-3">
			<div className="flex items-center justify-between">
				<Label className="text-sm font-medium flex items-center gap-2">
					<Cpu className="w-4 h-4 text-blue-400" />
					Instance Types
				</Label>
				<div className="flex items-center gap-2">
					{spotOpen && (
						<Button
							type="button"
							variant="outline"
							size="sm"
							onClick={() => setShowSpot((v) => !v)}
							className="h-7 text-xs"
						>
							<TrendingDown className="w-3 h-3 mr-1" />
							{showSpot ? "Hide spot prices" : "Show spot prices"}
						</Button>
					)}
					<Button
						type="button"
						variant="outline"
						size="sm"
						onClick={refresh}
						disabled={loading}
						className="h-7 text-xs"
					>
						{loading ? (
							<Loader2 className="w-3 h-3 animate-spin" />
						) : (
							<RefreshCw className="w-3 h-3" />
						)}
					</Button>
				</div>
			</div>

			{/* Selected types — the pool's actual value, editable without scrolling
			    the catalog. The ASG tries them in order. */}
			<div className="flex flex-wrap gap-2">
				{selected.length === 0 && (
					<span className="text-xs text-gray-500">
						No instance types selected — the pool will not launch anything.
					</span>
				)}
				{selected.map((instanceType) => {
					const info = find(instanceType);
					return (
						<span
							key={instanceType}
							className="inline-flex items-center gap-1.5 px-2 py-1 rounded-md bg-blue-900/30 border border-blue-700 text-xs font-mono text-blue-200"
						>
							{instanceType}
							{info && (
								<span className="text-blue-400/70 font-sans">
									{info.vcpu}c/{formatMemory(info.memoryMiB)}
								</span>
							)}
							<button
								type="button"
								aria-label={`Remove ${instanceType}`}
								onClick={() => toggle(instanceType)}
								className="text-blue-300 hover:text-white"
							>
								<X className="w-3 h-3" />
							</button>
						</span>
					);
				})}
			</div>

			{unavailable.length > 0 && (
				<Alert className="border-yellow-600 bg-yellow-900/20">
					<AlertCircle className="h-4 w-4 text-yellow-400" />
					<AlertDescription className="text-yellow-200 text-xs">
						{unavailable.join(", ")} {unavailable.length === 1 ? "is" : "are"}{" "}
						not available in {region}. The Auto Scaling group tolerates this as
						long as one listed type is available, but it will never launch{" "}
						{unavailable.length === 1 ? "this one" : "these"}.
					</AlertDescription>
				</Alert>
			)}

			{/* Provenance. Rendered before the prices it qualifies, because a price
			    the reader has already believed is not corrected by a footnote. */}
			{data?.pricingRegion && (
				<Alert className="border-yellow-600 bg-yellow-900/20">
					<AlertCircle className="h-4 w-4 text-yellow-400" />
					<AlertDescription className="text-yellow-200 text-xs">
						Indicative {data.pricingRegion} pricing — the AWS Pricing API could
						not be reached, so these are list prices for {data.pricingRegion}
						{data.pricingDate ? ` as of ${data.pricingDate}` : ""}, not {region}{" "}
						prices. EC2 on-demand in other regions typically runs 15–25% higher,
						and the premium is not uniform across families.
					</AlertDescription>
				</Alert>
			)}

			{data && !data.availabilityVerified && (
				<Alert className="border-yellow-600 bg-yellow-900/20">
					<AlertCircle className="h-4 w-4 text-yellow-400" />
					<AlertDescription className="text-yellow-200 text-xs">
						Availability in {region} was not verified — this list comes from
						built-in data
						{data.instanceDataDate ? ` dated ${data.instanceDataDate}` : ""}. A
						type shown here may not exist in this region.
					</AlertDescription>
				</Alert>
			)}

			{error && (
				<Alert className="border-red-600 bg-red-900/20">
					<AlertCircle className="h-4 w-4 text-red-400" />
					<AlertDescription className="text-red-200 text-xs">
						{error} — instance types can still be typed into the YAML directly.
					</AlertDescription>
				</Alert>
			)}

			{showSpot && (spotError || spotNotice) && (
				<Alert className="border-yellow-600 bg-yellow-900/20">
					<AlertCircle className="h-4 w-4 text-yellow-400" />
					<AlertDescription className="text-yellow-200 text-xs">
						{spotError ?? spotNotice}
					</AlertDescription>
				</Alert>
			)}

			<div className="relative">
				<Search className="absolute left-2.5 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-gray-500" />
				<Input
					id={`${uid}-search`}
					value={search}
					onChange={(e) => setSearch(e.target.value)}
					placeholder={
						data
							? `Filter ${data.instanceTypes.length} types in ${region}…`
							: "Filter instance types…"
					}
					className="pl-8 bg-gray-900 border-gray-600 h-8 text-sm"
				/>
			</div>

			{loading && instanceTypes.length === 0 ? (
				<div className="space-y-2">
					{["a", "b", "c", "d"].map((key) => (
						<div
							key={key}
							className="h-12 rounded-md bg-gray-900 border border-gray-800 animate-pulse"
						/>
					))}
				</div>
			) : matches.length === 0 ? (
				<div className="text-center py-6 px-3 rounded-md border border-dashed border-gray-700">
					<Cpu className="w-6 h-6 mx-auto text-gray-600" />
					<p className="text-sm text-gray-400 mt-2">
						{instanceTypes.length === 0
							? "No instance catalog available."
							: `No type matches "${search}".`}
					</p>
					<p className="text-xs text-gray-500 mt-1">
						{instanceTypes.length === 0
							? "Instance types remain editable in the YAML; this list is a convenience, not the source of truth."
							: "Clear the filter to see the full catalog."}
					</p>
				</div>
			) : (
				<div className="space-y-1.5 max-h-96 overflow-y-auto pr-1">
					{visible.map(renderRow)}
				</div>
			)}

			<div className="flex items-center justify-between text-xs text-gray-500">
				<span>
					{matches.length > visible.length
						? `Showing ${visible.length} of ${matches.length} matching types — narrow the filter to see the rest.`
						: data?.filtered
							? `${instanceTypes.length} recommended types of ${data.totalAvailable} available in ${region}.`
							: null}
				</span>
				{showSpot && spotLoading && (
					<span className="flex items-center gap-1">
						<Loader2 className="w-3 h-3 animate-spin" />
						spot prices…
					</span>
				)}
			</div>

			{showSpot && spotOpen && matches.length > MAX_SPOT_TYPES && (
				<p className="text-xs text-gray-500">
					Spot prices are quoted for the first {MAX_SPOT_TYPES} rows — AWS
					accepts at most {MAX_SPOT_TYPES} types per request.
				</p>
			)}
		</div>
	);
}
