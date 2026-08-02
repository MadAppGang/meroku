import { Cloud, Globe, Plus, Server, Trash2 } from "lucide-react";
import { useState } from "react";
import type {
	CloudFrontAdditionalZone,
	CloudFrontCacheBehavior,
	CloudFrontConfig,
	CloudFrontOrigin,
	YamlInfrastructureConfig,
} from "../types/yamlConfig";
import { Badge } from "./ui/badge";
import { Button } from "./ui/button";
import { Input } from "./ui/input";
import { Label } from "./ui/label";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "./ui/select";
import { Separator } from "./ui/separator";
import { Switch } from "./ui/switch";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "./ui/tooltip";

interface CloudFrontNodePropertiesProps {
	config: YamlInfrastructureConfig;
	distributionName: string;
	onConfigChange?: (updates: Partial<YamlInfrastructureConfig>) => void;
}

export function CloudFrontNodeProperties({
	config,
	distributionName,
	onConfigChange,
}: CloudFrontNodePropertiesProps) {
	// Find the specific distribution by name
	const distributions = config.cloudfront_distributions || [];
	const distribution = distributions.find((d) => d.name === distributionName);

	const [newDomainAlias, setNewDomainAlias] = useState("");
	const [newOrigin, setNewOrigin] = useState<Partial<CloudFrontOrigin>>({
		type: "amplify",
	});
	const [showAddOrigin, setShowAddOrigin] = useState(false);
	const [newZone, setNewZone] = useState<{
		domain: string;
		create_zone: boolean;
		zone_id: string;
	}>({
		domain: "",
		create_zone: true,
		zone_id: "",
	});
	const [showAddZone, setShowAddZone] = useState(false);

	if (!distribution) {
		return (
			<div className="p-4 text-muted-foreground">
				Distribution "{distributionName}" not found.
			</div>
		);
	}

	const updateDistribution = (updates: Partial<CloudFrontConfig>) => {
		const updatedDistributions = distributions.map((d) =>
			d.name === distributionName ? { ...d, ...updates } : d,
		);
		onConfigChange?.({
			cloudfront_distributions: updatedDistributions,
		});
	};

	const addDomainAlias = () => {
		if (!newDomainAlias.trim()) return;
		const aliases = [
			...(distribution.domain_aliases || []),
			newDomainAlias.trim(),
		];
		updateDistribution({ domain_aliases: aliases });
		setNewDomainAlias("");
	};

	const removeDomainAlias = (index: number) => {
		const aliases = (distribution.domain_aliases || []).filter(
			(_: string, i: number) => i !== index,
		);
		updateDistribution({ domain_aliases: aliases });
	};

	const addOrigin = () => {
		if (!newOrigin.name || !newOrigin.type) return;
		const origins = [
			...(distribution.origins || []),
			newOrigin as CloudFrontOrigin,
		];
		updateDistribution({ origins });
		setNewOrigin({ type: "amplify" });
		setShowAddOrigin(false);
	};

	const removeOrigin = (index: number) => {
		const origins = (distribution.origins || []).filter(
			(_: CloudFrontOrigin, i: number) => i !== index,
		);
		updateDistribution({ origins });
	};

	const addAdditionalZone = () => {
		if (!newZone.domain.trim()) return;
		if (!newZone.create_zone && !newZone.zone_id.trim()) return;
		const zones: CloudFrontAdditionalZone[] = [
			...(distribution.additional_zones || []),
			{
				domain: newZone.domain.trim(),
				create_zone: newZone.create_zone,
				...(newZone.create_zone ? {} : { zone_id: newZone.zone_id.trim() }),
			},
		];
		updateDistribution({ additional_zones: zones });
		setNewZone({ domain: "", create_zone: true, zone_id: "" });
		setShowAddZone(false);
	};

	const removeAdditionalZone = (index: number) => {
		const zones = (distribution.additional_zones || []).filter(
			(_: CloudFrontAdditionalZone, i: number) => i !== index,
		);
		updateDistribution({ additional_zones: zones });
	};

	// Get the URL pattern for an origin (from cache_behaviors or default)
	const getOriginUrlPattern = (
		originName: string,
		originIndex: number,
	): string => {
		// First origin is always the default (handles everything not matched)
		if (originIndex === 0) return "/*";
		// Find the cache behavior for this origin
		const behavior = distribution.cache_behaviors?.find(
			(b) => b.origin_name === originName,
		);
		return behavior?.path_pattern || "";
	};

	// Update URL pattern for an origin
	const updateOriginUrlPattern = (originName: string, pattern: string) => {
		const existingBehaviors = distribution.cache_behaviors || [];
		const existingIndex = existingBehaviors.findIndex(
			(b) => b.origin_name === originName,
		);

		let newBehaviors: CloudFrontCacheBehavior[];

		if (pattern.trim() === "") {
			// Remove the behavior if pattern is empty
			newBehaviors = existingBehaviors.filter(
				(b) => b.origin_name !== originName,
			);
		} else if (existingIndex >= 0) {
			// Update existing behavior
			newBehaviors = existingBehaviors.map((b, i) =>
				i === existingIndex ? { ...b, path_pattern: pattern.trim() } : b,
			);
		} else {
			// Add new behavior
			newBehaviors = [
				...existingBehaviors,
				{
					path_pattern: pattern.trim(),
					origin_name: originName,
					viewer_protocol_policy: "redirect-to-https",
					compress: true,
				},
			];
		}

		updateDistribution({ cache_behaviors: newBehaviors });
	};

	const getOriginTypeIcon = (type: string) => {
		switch (type) {
			case "amplify":
				return <Cloud className="h-4 w-4 text-orange-500" />;
			case "s3":
				return <Server className="h-4 w-4 text-green-500" />;
			case "alb":
				return <Globe className="h-4 w-4 text-blue-500" />;
			default:
				return <Globe className="h-4 w-4 text-gray-500" />;
		}
	};

	return (
		<div className="space-y-6">
			{/* Header */}
			<div className="flex items-center gap-3">
				<div className="p-2 bg-orange-100 dark:bg-orange-900/30 rounded-lg">
					<Cloud className="h-5 w-5 text-orange-600" />
				</div>
				<div>
					<h3 className="font-semibold">{distribution.name}</h3>
					<p className="text-sm text-muted-foreground">
						CloudFront CDN distribution
					</p>
				</div>
			</div>

			<Separator />

			{/* Enable/Disable */}
			<div className="flex items-center justify-between">
				<div>
					<Label>Enable Distribution</Label>
					<p className="text-sm text-muted-foreground">
						Create and deploy this CloudFront distribution
					</p>
				</div>
				<Switch
					checked={distribution.enabled}
					onCheckedChange={(enabled) => updateDistribution({ enabled })}
				/>
			</div>

			{distribution.enabled && (
				<>
					{/* Domain Aliases */}
					<div className="space-y-3">
						<Label>Domain Aliases</Label>
						<p className="text-sm text-muted-foreground">
							Custom domains including wildcards (e.g., *.app.example.com)
						</p>
						<div className="flex gap-2">
							<Input
								placeholder="*.app.example.com"
								value={newDomainAlias}
								onChange={(e) => setNewDomainAlias(e.target.value)}
								onKeyDown={(e) => e.key === "Enter" && addDomainAlias()}
							/>
							<Button onClick={addDomainAlias} size="sm">
								<Plus className="h-4 w-4" />
							</Button>
						</div>
						{distribution.domain_aliases?.map(
							(alias: string, index: number) => (
								<div
									key={`alias-${alias}`}
									className="flex items-center justify-between p-2 bg-muted rounded-md"
								>
									<code className="text-sm">{alias}</code>
									<Button
										variant="ghost"
										size="sm"
										onClick={() => removeDomainAlias(index)}
									>
										<Trash2 className="h-4 w-4 text-destructive" />
									</Button>
								</div>
							),
						)}
					</div>

					{/* Additional Zones */}
					{(distribution.domain_aliases?.length ?? 0) > 0 && (
						<div className="space-y-3">
							<div className="flex items-center justify-between">
								<div>
									<Label>Additional DNS Zones</Label>
									<p className="text-sm text-muted-foreground">
										Route 53 zones for non-main domain aliases
									</p>
								</div>
								<Button
									variant="outline"
									size="sm"
									onClick={() => setShowAddZone(!showAddZone)}
								>
									<Plus className="h-4 w-4 mr-1" />
									Add Zone
								</Button>
							</div>

							{showAddZone && (
								<div className="p-4 border rounded-lg space-y-4 bg-muted/30">
									<div className="grid grid-cols-2 gap-4">
										<div className="space-y-2">
											<Label>Domain</Label>
											<Input
												placeholder="otherdomain.com"
												value={newZone.domain}
												onChange={(e) =>
													setNewZone({ ...newZone, domain: e.target.value })
												}
											/>
										</div>
										<div className="space-y-2">
											<Label>Zone Type</Label>
											<Select
												value={newZone.create_zone ? "create" : "existing"}
												onValueChange={(value) =>
													setNewZone({
														...newZone,
														create_zone: value === "create",
													})
												}
											>
												<SelectTrigger>
													<SelectValue />
												</SelectTrigger>
												<SelectContent>
													<SelectItem value="create">
														Create new zone
													</SelectItem>
													<SelectItem value="existing">
														Use existing zone
													</SelectItem>
												</SelectContent>
											</Select>
										</div>
									</div>

									{!newZone.create_zone && (
										<div className="space-y-2">
											<Label>Zone ID</Label>
											<Input
												placeholder="Z1234567890ABC"
												value={newZone.zone_id}
												onChange={(e) =>
													setNewZone({ ...newZone, zone_id: e.target.value })
												}
											/>
										</div>
									)}

									<div className="flex justify-end gap-2">
										<Button
											variant="ghost"
											size="sm"
											onClick={() => setShowAddZone(false)}
										>
											Cancel
										</Button>
										<Button size="sm" onClick={addAdditionalZone}>
											Add Zone
										</Button>
									</div>
								</div>
							)}

							{distribution.additional_zones?.map(
								(zone: CloudFrontAdditionalZone, index: number) => (
									<div
										key={`zone-${zone.domain}`}
										className="flex items-center justify-between p-3 border rounded-lg"
									>
										<div>
											<div className="font-medium">{zone.domain}</div>
											<div className="text-sm text-muted-foreground">
												{zone.create_zone
													? "Will create new zone"
													: `Zone ID: ${zone.zone_id}`}
											</div>
										</div>
										<div className="flex items-center gap-2">
											<Badge variant="secondary">
												{zone.create_zone ? "Create" : "Existing"}
											</Badge>
											<Button
												variant="ghost"
												size="sm"
												onClick={() => removeAdditionalZone(index)}
											>
												<Trash2 className="h-4 w-4 text-destructive" />
											</Button>
										</div>
									</div>
								),
							)}
						</div>
					)}

					<Separator />

					{/* Origins */}
					<div className="space-y-3">
						<div className="flex items-center justify-between">
							<div>
								<Label>Origins</Label>
								<p className="text-sm text-muted-foreground">
									Content sources (S3, Amplify, ALB, or custom)
								</p>
							</div>
							<Button
								variant="outline"
								size="sm"
								onClick={() => setShowAddOrigin(!showAddOrigin)}
							>
								<Plus className="h-4 w-4 mr-1" />
								Add Origin
							</Button>
						</div>

						{showAddOrigin && (
							<div className="p-4 border rounded-lg space-y-4 bg-muted/30">
								<div className="grid grid-cols-2 gap-4">
									<div className="space-y-2">
										<Label>Name</Label>
										<Input
											placeholder="frontend"
											value={newOrigin.name || ""}
											onChange={(e) =>
												setNewOrigin({ ...newOrigin, name: e.target.value })
											}
										/>
									</div>
									<div className="space-y-2">
										<Label>Type</Label>
										<Select
											value={newOrigin.type}
											onValueChange={(value) =>
												setNewOrigin({
													...newOrigin,
													type: value as CloudFrontOrigin["type"],
												})
											}
										>
											<SelectTrigger>
												<SelectValue placeholder="Select type" />
											</SelectTrigger>
											<SelectContent>
												<SelectItem value="amplify">Amplify App</SelectItem>
												<SelectItem value="s3">S3 Bucket</SelectItem>
												<SelectItem value="alb">ALB</SelectItem>
												<SelectItem value="custom">Custom URL</SelectItem>
											</SelectContent>
										</Select>
									</div>
								</div>

								{newOrigin.type === "amplify" && (
									<div className="space-y-2">
										<Label>Amplify App Name</Label>
										<Select
											value={newOrigin.amplify_app_name}
											onValueChange={(value) =>
												setNewOrigin({ ...newOrigin, amplify_app_name: value })
											}
										>
											<SelectTrigger>
												<SelectValue placeholder="Select Amplify app" />
											</SelectTrigger>
											<SelectContent>
												{config.amplify_apps?.map((app) => (
													<SelectItem key={app.name} value={app.name}>
														{app.name}
													</SelectItem>
												))}
											</SelectContent>
										</Select>
									</div>
								)}

								{newOrigin.type === "s3" && (
									<div className="space-y-3">
										<div className="flex items-center justify-between">
											<div>
												<Label>Create New Bucket</Label>
												<p className="text-xs text-muted-foreground">
													Auto-create an S3 bucket for this origin
												</p>
											</div>
											<Switch
												checked={newOrigin.create_bucket || false}
												onCheckedChange={(create_bucket) =>
													setNewOrigin({ ...newOrigin, create_bucket })
												}
											/>
										</div>
										{newOrigin.create_bucket ? (
											<div className="p-2 bg-muted rounded-md">
												<p className="text-xs text-muted-foreground">
													Bucket name:{" "}
													<code className="text-xs">
														{config?.project}-{config?.env}-cf-
														{distribution.name}-{newOrigin.name || "origin"}
													</code>
												</p>
											</div>
										) : (
											<div className="space-y-2">
												<Label>Existing Bucket Name</Label>
												<Input
													placeholder="my-bucket"
													value={newOrigin.bucket_name || ""}
													onChange={(e) =>
														setNewOrigin({
															...newOrigin,
															bucket_name: e.target.value,
														})
													}
												/>
											</div>
										)}
									</div>
								)}

								{newOrigin.type === "custom" && (
									<div className="space-y-2">
										<Label>Domain Name</Label>
										<Input
											placeholder="api.example.com"
											value={newOrigin.domain_name || ""}
											onChange={(e) =>
												setNewOrigin({
													...newOrigin,
													domain_name: e.target.value,
												})
											}
										/>
									</div>
								)}

								<div className="flex justify-end gap-2">
									<Button
										variant="ghost"
										size="sm"
										onClick={() => setShowAddOrigin(false)}
									>
										Cancel
									</Button>
									<Button size="sm" onClick={addOrigin}>
										Add Origin
									</Button>
								</div>
							</div>
						)}

						{distribution.origins?.map(
							(origin: CloudFrontOrigin, index: number) => (
								<div
									key={`origin-${origin.name}`}
									className="p-3 border rounded-lg space-y-3"
								>
									<div className="flex items-center justify-between">
										<div className="flex items-center gap-3">
											{getOriginTypeIcon(origin.type)}
											<div>
												<div className="font-medium">{origin.name}</div>
												<div className="text-sm text-muted-foreground">
													{origin.type === "amplify" &&
														`Amplify: ${origin.amplify_app_name}`}
													{origin.type === "s3" &&
														(origin.create_bucket
															? `S3: ${config.project}-${config.env}-cf-${distribution.name}-${origin.name} (auto-created)`
															: `S3: ${origin.bucket_name}`)}
													{origin.type === "alb" && "Application Load Balancer"}
													{origin.type === "custom" && origin.domain_name}
												</div>
											</div>
										</div>
										<div className="flex items-center gap-2">
											<Badge variant="secondary">{origin.type}</Badge>
											<Button
												variant="ghost"
												size="sm"
												onClick={() => removeOrigin(index)}
											>
												<Trash2 className="h-4 w-4 text-destructive" />
											</Button>
										</div>
									</div>

									{/* URL Pattern - shown for all origins when there are multiple */}
									{(distribution.origins?.length ?? 0) > 1 && (
										<div className="flex items-center gap-2 pl-7">
											<span className="text-sm text-muted-foreground whitespace-nowrap">
												Handles:
											</span>
											{index === 0 ? (
												<TooltipProvider>
													<Tooltip>
														<TooltipTrigger asChild>
															<div className="flex items-center gap-2">
																<code className="bg-muted px-2 py-1 rounded text-sm font-mono">
																	/*
																</code>
																<Badge variant="outline" className="text-xs">
																	Default
																</Badge>
															</div>
														</TooltipTrigger>
														<TooltipContent>
															<p>
																Handles all URLs not matched by other origins
															</p>
														</TooltipContent>
													</Tooltip>
												</TooltipProvider>
											) : (
												<div className="flex items-center gap-2 flex-1">
													<Input
														placeholder="/api/*, /images/*, etc."
														className="font-mono text-sm h-8 max-w-xs"
														value={getOriginUrlPattern(origin.name, index)}
														onChange={(e) =>
															updateOriginUrlPattern(
																origin.name,
																e.target.value,
															)
														}
													/>
													{!getOriginUrlPattern(origin.name, index) && (
														<span className="text-xs text-amber-500">
															⚠️ No URL pattern set
														</span>
													)}
												</div>
											)}
										</div>
									)}
								</div>
							),
						)}
					</div>

					<Separator />

					{/* SPA Mode */}
					<div className="flex items-center justify-between">
						<div>
							<Label>SPA Mode</Label>
							<p className="text-sm text-muted-foreground">
								Handle 404 errors by returning index.html (for
								React/Vue/Angular)
							</p>
						</div>
						<Switch
							checked={distribution.spa_mode || false}
							onCheckedChange={(spa_mode) => updateDistribution({ spa_mode })}
						/>
					</div>

					{/* Price Class */}
					<div className="space-y-2">
						<Label>Price Class</Label>
						<Select
							value={distribution.price_class || "PriceClass_100"}
							onValueChange={(value) =>
								updateDistribution({
									price_class: value as
										| "PriceClass_100"
										| "PriceClass_200"
										| "PriceClass_All",
								})
							}
						>
							<SelectTrigger>
								<SelectValue />
							</SelectTrigger>
							<SelectContent>
								<SelectItem value="PriceClass_100">
									US & Europe Only (cheapest)
								</SelectItem>
								<SelectItem value="PriceClass_200">
									US, Europe, Asia, Middle East, Africa
								</SelectItem>
								<SelectItem value="PriceClass_All">
									All Edge Locations (global)
								</SelectItem>
							</SelectContent>
						</Select>
					</div>

					{/* Default Root Object */}
					<div className="space-y-2">
						<Label>Default Root Object</Label>
						<Input
							placeholder="index.html"
							value={distribution.default_root_object || "index.html"}
							onChange={(e) =>
								updateDistribution({ default_root_object: e.target.value })
							}
						/>
					</div>
				</>
			)}
		</div>
	);
}
