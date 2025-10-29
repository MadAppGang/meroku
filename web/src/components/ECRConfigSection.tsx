import type React from "react";
import { memo, useRef, useEffect, useCallback } from "react";
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
	accountId?: string;
	region?: string;
}

export const ECRConfigSection = memo(function ECRConfigSection({
	config,
	onChange,
	availableSources,
	currentServiceName,
	errors = {},
	accountId,
	region,
}: ECRConfigSectionProps) {
	// DEBUG: Render counter
	const renderCountRef = useRef(0);
	renderCountRef.current++;

	// Track previous props to detect what changed
	const prevPropsRef = useRef({ config, onChange, availableSources, errors, accountId, region });
	useEffect(() => {
		const prev = prevPropsRef.current;
		const changes: string[] = [];
		if (prev.config !== config) changes.push('config');
		if (prev.onChange !== onChange) changes.push('onChange');
		if (prev.availableSources !== availableSources) changes.push('availableSources');
		if (prev.errors !== errors) changes.push('errors');
		if (prev.accountId !== accountId) changes.push('accountId');
		if (prev.region !== region) changes.push('region');

		if (changes.length > 0) {
			console.log(`🔧 [ECRConfigSection] Props changed: ${changes.join(', ')}`, {
				configRef: prev.config === config ? 'same' : 'CHANGED',
				onChangeRef: prev.onChange === onChange ? 'same' : 'CHANGED',
				availableSourcesRef: prev.availableSources === availableSources ? 'same' : 'CHANGED',
				availableSourcesLength: availableSources.length,
			});
		}
		prevPropsRef.current = { config, onChange, availableSources, errors, accountId, region };
	}, [config, onChange, availableSources, errors, accountId, region]);

	console.log(`🔄 [ECRConfigSection] Render #${renderCountRef.current} for ${currentServiceName}`);

	if (renderCountRef.current > 50) {
		console.error('⚠️ [ECRConfigSection] INFINITE LOOP DETECTED - More than 50 renders!');
		console.trace('Stack trace at 50th render');
	}

	console.log(`🐳 [ECRConfigSection] Props:`, {
		config,
		availableSourcesCount: availableSources.length,
		availableSources,
		currentServiceName,
	});

	const mode = config.mode || "create_ecr";

	// Generate preconfigured ECR repository URI
	const preConfiguredECRUri = currentServiceName && accountId && region
		? `${accountId}.dkr.ecr.${region}.amazonaws.com/${currentServiceName}`
		: null;

	const handleModeChange = useCallback((newMode: string) => {
		console.log(`🔧 [ECRConfigSection] handleModeChange called:`, newMode);
		onChange({
			mode: newMode as ECRConfig["mode"],
			repository_uri: "",
			source_service_name: "",
			source_service_type: undefined,
		});
	}, [onChange]);

	const handleRepositoryURIChange = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
		console.log(`🔧 [ECRConfigSection] handleRepositoryURIChange called:`, e.target.value);
		onChange({
			...config,
			repository_uri: e.target.value,
		});
	}, [config, onChange]);

	const handleSourceChange = useCallback((value: string) => {
		console.log(`🔧 [ECRConfigSection] handleSourceChange called:`, value);
		const [sourceType, sourceName] = value.split("-", 2);
		onChange({
			...config,
			source_service_name: sourceName,
			source_service_type: sourceType as ECRConfig["source_service_type"],
		});
	}, [config, onChange]);

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
				<Label className="text-sm font-medium">Docker Registry Configuration</Label>
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
				{mode === "create_ecr" && preConfiguredECRUri && (
					<div className="mt-2 ml-6 p-3 bg-slate-800/30 border border-slate-700/50 rounded-lg">
						<p className="text-xs text-slate-500 mb-1">ECR Repository URI</p>
						<code className="text-xs text-slate-300 font-mono break-all">
							{preConfiguredECRUri}
						</code>
					</div>
				)}

				<div className="flex items-center space-x-2 mt-3">
					<RadioGroupItem value="manual_repo" id="manual_repo" />
					<Label htmlFor="manual_repo" className="font-normal cursor-pointer">
						Use existing Docker image (Docker Hub, ECR, etc.)
					</Label>
				</div>
				<p className="text-xs text-muted-foreground ml-6">
					Provide the Docker image URI from any registry
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
				<div className="mt-6 ml-6 grid gap-2">
					<Label htmlFor="repository_uri">Docker Image URI</Label>
					<Input
						id="repository_uri"
						placeholder="nginx:latest or 123456789012.dkr.ecr.us-east-1.amazonaws.com/my-repo"
						value={config.repository_uri || ""}
						onChange={handleRepositoryURIChange}
						className={errors.repository_uri ? "border-red-500" : ""}
					/>
					{errors.repository_uri && (
						<p className="text-sm text-red-500 -mt-1">{errors.repository_uri}</p>
					)}
					<p className="text-xs text-muted-foreground -mt-1">
						Examples: nginx:latest, ubuntu:22.04, or ECR URI
					</p>
				</div>
			)}

			{mode === "use_existing" && (
				<div className="mt-6 ml-6 grid gap-2">
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
						<p className="text-sm text-red-500 -mt-1">
							{errors.source_service_name}
						</p>
					)}
					<p className="text-xs text-muted-foreground -mt-1">
						Only services with dedicated ECR repositories can be selected
					</p>
				</div>
			)}
		</div>
	);
});
