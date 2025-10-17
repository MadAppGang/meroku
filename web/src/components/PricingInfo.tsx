import { Info } from "lucide-react";
import { usePricingRates } from "../contexts/PricingContext";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "./ui/tooltip";

/**
 * PricingInfo Component
 *
 * Displays pricing source information in a subtle, non-intrusive way.
 * Shows when pricing data was last updated and from what source.
 *
 * Part of Option 3: Hybrid Approach for centralized pricing.
 */
export function PricingInfo() {
	const rates = usePricingRates();

	if (!rates) return null;

	const pricingDate = rates.pricingDate || "2025-01-15";
	const source = rates.source === "aws_api" ? "AWS API" : "curated pricing";
	const formattedDate = new Date(pricingDate).toLocaleDateString("en-US", {
		year: "numeric",
		month: "long",
		day: "numeric",
	});

	return (
		<TooltipProvider>
			<Tooltip>
				<TooltipTrigger asChild>
					<div className="flex items-center gap-1.5 text-xs text-gray-500 dark:text-gray-400 cursor-help">
						<Info className="w-3.5 h-3.5" />
						<span>Pricing as of {formattedDate}</span>
					</div>
				</TooltipTrigger>
				<TooltipContent side="top" className="max-w-xs">
					<p className="text-sm">
						Pricing data sourced from {source} and updated quarterly to ensure
						accuracy.
					</p>
					{rates.source === "fallback" && (
						<p className="text-xs mt-1 text-gray-400">
							Next review: April 2025
						</p>
					)}
				</TooltipContent>
			</Tooltip>
		</TooltipProvider>
	);
}
