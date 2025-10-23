import type React from "react";
import { memo } from "react";
import { Label } from "./ui/label";
import { RadioGroup, RadioGroupItem } from "./ui/radio-group";
import { Input } from "./ui/input";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "./ui/select";
import type { ECRConfig } from "../types/yamlConfig";

interface ECRSource {
	name: string;
	type: "services" | "event_processor_tasks" | "scheduled_tasks";
	displayType: string;
}

interface ECRConfigSectionProps {
	config: ECRConfig;
	onChange: (config: ECRConfig) => void;
	availableSources: ECRSource[];
	currentServiceName?: string;
	errors?: Record<string, string>;
}

export const ECRConfigSection = memo(function ECRConfigSection({
	config,
	onChange,
	availableSources,
	currentServiceName,
	errors = {},
}: ECRConfigSectionProps) {
	const mode = config.mode || "create_ecr";

	const handleModeChange = (newMode: string) => {
		onChange({
			mode: newMode as ECRConfig["mode"],
			repository_uri: "",
			source_service_name: "",
			source_service_type: undefined,
		});
	};

	const handleRepositoryURIChange = (e: React.ChangeEvent<HTMLInputElement>) => {
		onChange({
			...config,
			repository_uri: e.target.value,
		});
	};

	const handleSourceChange = (value: string) => {
		const [sourceType, sourceName] = value.split("-", 2);
		onChange({
			...config,
			source_service_name: sourceName,
			source_service_type: sourceType as ECRConfig["source_service_type"],
		});
	};

	const filteredSources = availableSources.filter(
		(source) => source.name !== currentServiceName,
	);

	const currentSourceValue =
		config.source_service_type && config.source_service_name
			? `${config.source_service_type}-${config.source_service_name}`
			: "";

	return (
		<div className="space-y-4">
			<div>
				<Label className="text-sm font-medium">ECR Repository Configuration</Label>
				<p className="text-sm text-muted-foreground mt-1">
					Choose how to manage the container registry for this service
				</p>
			</div>

			<RadioGroup value={mode} onValueChange={handleModeChange}>
				<div className="flex items-center space-x-2">
					<RadioGroupItem value="create_ecr" id="create_ecr" />
					<Label htmlFor="create_ecr" className="font-normal cursor-pointer">
						Create new ECR repository (recommended)
					</Label>
				</div>
				<p className="text-xs text-muted-foreground ml-6">
					A dedicated ECR repository will be created for this service
				</p>

				<div className="flex items-center space-x-2 mt-3">
					<RadioGroupItem value="manual_repo" id="manual_repo" />
					<Label htmlFor="manual_repo" className="font-normal cursor-pointer">
						Use existing ECR repository (manual URI)
					</Label>
				</div>
				<p className="text-xs text-muted-foreground ml-6">
					Provide the full ECR repository URI
				</p>

				<div className="flex items-center space-x-2 mt-3">
					<RadioGroupItem value="use_existing" id="use_existing" />
					<Label htmlFor="use_existing" className="font-normal cursor-pointer">
						Use ECR from another service
					</Label>
				</div>
				<p className="text-xs text-muted-foreground ml-6">
					Share an ECR repository from another service, event processor, or scheduled task
				</p>
			</RadioGroup>

			{mode === "manual_repo" && (
				<div className="mt-4 ml-6">
					<Label htmlFor="repository_uri">Repository URI</Label>
					<Input
						id="repository_uri"
						placeholder="123456789012.dkr.ecr.us-east-1.amazonaws.com/my-repo"
						value={config.repository_uri || ""}
						onChange={handleRepositoryURIChange}
						className={errors.repository_uri ? "border-red-500" : ""}
					/>
					{errors.repository_uri && (
						<p className="text-sm text-red-500 mt-1">{errors.repository_uri}</p>
					)}
					<p className="text-xs text-muted-foreground mt-1">
						Format: account-id.dkr.ecr.region.amazonaws.com/repository-name
					</p>
				</div>
			)}

			{mode === "use_existing" && (
				<div className="mt-4 ml-6">
					<Label htmlFor="source_service">Source Service</Label>
					<Select value={currentSourceValue} onValueChange={handleSourceChange}>
						<SelectTrigger
							id="source_service"
							className={errors.source_service_name ? "border-red-500" : ""}
						>
							<SelectValue placeholder="Select a service to share ECR from" />
						</SelectTrigger>
						<SelectContent>
							{filteredSources.length === 0 ? (
								<div className="px-2 py-1.5 text-sm text-muted-foreground">
									No services available
								</div>
							) : (
								filteredSources.map((source) => (
									<SelectItem
										key={`${source.type}-${source.name}`}
										value={`${source.type}-${source.name}`}
									>
										{source.name} ({source.displayType})
									</SelectItem>
								))
							)}
						</SelectContent>
					</Select>
					{errors.source_service_name && (
						<p className="text-sm text-red-500 mt-1">
							{errors.source_service_name}
						</p>
					)}
					<p className="text-xs text-muted-foreground mt-1">
						Only services with dedicated ECR repositories can be selected
					</p>
				</div>
			)}
		</div>
	);
});
