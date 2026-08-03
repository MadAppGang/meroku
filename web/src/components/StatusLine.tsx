import { Code, Eye, Info } from "lucide-react";
import { usePricingRates } from "../contexts/PricingContext";
import { AWS_REGIONS } from "../types/config";
import type { YamlInfrastructureConfig } from "../types/yamlConfig";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "./ui/select";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "./ui/tooltip";

interface StatusLineProps {
	selectedEnvironment: string | null;
	config: YamlInfrastructureConfig | null;
	activeEnvironmentProfile: string | null;
	activeEnvironmentAccountId: string | null;
	viewMode: "visual" | "code";
	onViewModeChange: (mode: "visual" | "code") => void;
	onConfigChange: (updates: Partial<YamlInfrastructureConfig>) => void;
}

export function StatusLine({
	selectedEnvironment,
	config,
	activeEnvironmentProfile,
	activeEnvironmentAccountId,
	viewMode,
	onViewModeChange,
	onConfigChange,
}: StatusLineProps) {
	const rates = usePricingRates();

	if (!selectedEnvironment) return null;

	return (
		<div className="h-8 bg-gray-900 border-t border-gray-800 flex items-center px-4 text-xs text-gray-400 select-none z-50 w-full justify-between">
			{/* Left Section: Environment & Project */}
			<div className="flex items-center gap-4">
				{/* Environment */}
				<div className="flex items-center gap-2 hover:text-gray-200 transition-colors cursor-default">
					<div className="w-2.5 h-2.5 bg-green-500 rounded-full" />
					<span className="font-semibold text-gray-300">
						{selectedEnvironment}
					</span>
				</div>

				{config && (
					<div className="flex items-center gap-2 hover:text-gray-200 transition-colors cursor-default">
						<span>{config.project}</span>
					</div>
				)}
			</div>

			{/* Center Section: View Mode & Region */}
			{config && (
				<div className="flex items-center gap-6 absolute left-1/2 -translate-x-1/2">
					{/* View Mode Toggle (Miniature) */}
					<div className="flex items-center bg-gray-800 rounded overflow-hidden">
						<button
							type="button"
							onClick={() => onViewModeChange("visual")}
							className={`flex items-center gap-1.5 px-3 py-1 hover:text-white transition-colors ${
								viewMode === "visual"
									? "bg-blue-600 text-white"
									: "hover:bg-gray-700"
							}`}
							title="Visual Mode"
						>
							<Eye className="w-3.5 h-3.5" />
							<span className="hidden sm:inline">Visual</span>
						</button>
						<button
							type="button"
							onClick={() => onViewModeChange("code")}
							className={`flex items-center gap-1.5 px-3 py-1 hover:text-white transition-colors ${
								viewMode === "code"
									? "bg-blue-600 text-white"
									: "hover:bg-gray-700"
							}`}
							title="Code Mode"
						>
							<Code className="w-3.5 h-3.5" />
							<span className="hidden sm:inline">Code</span>
						</button>
					</div>

					{/* Region Selector */}
					<div className="flex items-center hover:bg-gray-800 hover:text-gray-200 rounded px-2 py-0.5 transition-colors">
						<Select
							value={config.region}
							onValueChange={(value: string) => {
								onConfigChange({ region: value });
							}}
						>
							<SelectTrigger className="h-5 p-0 border-0 bg-transparent text-xs text-inherit focus:ring-0 w-auto gap-2">
								<SelectValue />
							</SelectTrigger>
							<SelectContent className="bg-gray-900 border-gray-800">
								{AWS_REGIONS.map((region) => (
									<SelectItem
										key={region.value}
										value={region.value}
										className="text-xs"
									>
										{region.label}
									</SelectItem>
								))}
							</SelectContent>
						</Select>
					</div>
				</div>
			)}

			{/* Right Section: AWS Info & Pricing */}
			<div className="flex items-center gap-6">
				{activeEnvironmentProfile && (
					<div className="flex items-center gap-2 hover:text-gray-200 transition-colors">
						<span className="hidden sm:inline text-gray-500">Profile:</span>
						<span className="text-gray-300" title={activeEnvironmentProfile}>
							{activeEnvironmentProfile}
						</span>
					</div>
				)}

				{activeEnvironmentAccountId && (
					<div className="flex items-center gap-2 hover:text-gray-200 transition-colors font-mono">
						<span className="hidden sm:inline text-gray-500">Acct:</span>
						<span>{activeEnvironmentAccountId}</span>
					</div>
				)}

				{rates && (
					<TooltipProvider>
						<Tooltip>
							<TooltipTrigger asChild>
								<div className="flex items-center gap-1.5 hover:text-gray-200 transition-colors cursor-help">
									<Info className="w-3.5 h-3.5" />
									<span className="hidden sm:inline">Pricing</span>
								</div>
							</TooltipTrigger>
							<TooltipContent
								side="top"
								className="bg-gray-800 border-gray-700 text-gray-300 text-xs text-center"
							>
								Pricing as of{" "}
								{new Date(
									rates.pricingDate || "2025-01-15",
								).toLocaleDateString()}
								<br />
								Source:{" "}
								{rates.source === "aws_api" ? "AWS API" : "curated pricing"}
							</TooltipContent>
						</Tooltip>
					</TooltipProvider>
				)}
			</div>
		</div>
	);
}
