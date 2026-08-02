import { Cloud, Globe, Plus, Trash2 } from "lucide-react";
import { useState } from "react";
import type {
	CloudFrontAdditionalZone,
	CloudFrontConfig,
	YamlInfrastructureConfig,
} from "../types/yamlConfig";
import { Button } from "./ui/button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "./ui/dialog";
import { Input } from "./ui/input";
import { Label } from "./ui/label";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "./ui/select";
import { Switch } from "./ui/switch";

interface AddCloudFrontDialogProps {
	open: boolean;
	onClose: () => void;
	onAdd: (distribution: CloudFrontConfig) => Promise<void>;
	existingDistributions?: string[];
	config?: YamlInfrastructureConfig;
}

export const AddCloudFrontDialog: React.FC<AddCloudFrontDialogProps> = ({
	open,
	onClose,
	onAdd,
	existingDistributions = [],
	config,
}) => {
	const [formData, setFormData] = useState<{
		name: string;
		enabled: boolean;
		spa_mode: boolean;
		price_class: "PriceClass_100" | "PriceClass_200" | "PriceClass_All";
		domain_aliases: string[];
		additional_zones: CloudFrontAdditionalZone[];
		origin_type: "amplify" | "s3" | "alb" | "custom";
		origin_name: string;
		amplify_app_name: string;
		bucket_name: string;
		create_bucket: boolean;
		custom_domain: string;
	}>({
		name: "",
		enabled: true,
		spa_mode: true,
		price_class: "PriceClass_100",
		domain_aliases: [],
		additional_zones: [],
		origin_type: "amplify",
		origin_name: "frontend",
		amplify_app_name: "",
		bucket_name: "",
		create_bucket: true,
		custom_domain: "",
	});

	const [newDomainAlias, setNewDomainAlias] = useState("");
	const [newZone, setNewZone] = useState<{
		domain: string;
		create_zone: boolean;
		zone_id: string;
	}>({
		domain: "",
		create_zone: true,
		zone_id: "",
	});
	const [pendingZonePrompt, setPendingZonePrompt] = useState<string | null>(
		null,
	);
	const [errors, setErrors] = useState<Record<string, string>>({});

	// Extract root domain from an alias (e.g., "*.app.example.com" -> "example.com")
	const extractRootDomain = (alias: string): string => {
		// Remove wildcard prefix
		const cleaned = alias.replace(/^\*\./, "");
		const parts = cleaned.split(".");
		// Return last two parts (or more for co.uk style domains)
		if (parts.length >= 2) {
			return parts.slice(-2).join(".");
		}
		return cleaned;
	};

	// Check if a domain alias needs an additional zone
	const needsAdditionalZone = (alias: string): string | null => {
		const rootDomain = extractRootDomain(alias);
		const mainDomain = config?.domain?.domain_name;

		// If no main domain configured, all domains need zones
		if (!mainDomain) return rootDomain;

		const mainRootDomain = extractRootDomain(mainDomain);

		// If alias root domain is different from main domain
		if (rootDomain !== mainRootDomain) {
			// Check if we already have a zone for this domain
			const hasZone = formData.additional_zones.some(
				(z) => z.domain === rootDomain || rootDomain.endsWith(`.${z.domain}`),
			);
			if (!hasZone) {
				return rootDomain;
			}
		}
		return null;
	};

	const addDomainAlias = () => {
		if (!newDomainAlias.trim()) return;
		if (formData.domain_aliases.includes(newDomainAlias.trim())) {
			setErrors({ ...errors, domain_alias: "Domain alias already added" });
			return;
		}

		const alias = newDomainAlias.trim();
		const zoneNeeded = needsAdditionalZone(alias);

		setFormData({
			...formData,
			domain_aliases: [...formData.domain_aliases, alias],
		});
		setNewDomainAlias("");
		setErrors({ ...errors, domain_alias: "" });

		// If this alias needs an additional zone, prompt user
		if (zoneNeeded) {
			setPendingZonePrompt(zoneNeeded);
			setNewZone({ domain: zoneNeeded, create_zone: true, zone_id: "" });
		}
	};

	const removeDomainAlias = (alias: string) => {
		setFormData({
			...formData,
			domain_aliases: formData.domain_aliases.filter((a) => a !== alias),
		});
	};

	const addAdditionalZone = () => {
		if (!newZone.domain.trim()) return;
		if (!newZone.create_zone && !newZone.zone_id.trim()) {
			setErrors({
				...errors,
				zone_id: "Zone ID is required when using an existing zone",
			});
			return;
		}
		if (
			formData.additional_zones.some((z) => z.domain === newZone.domain.trim())
		) {
			setErrors({
				...errors,
				zone_domain: "Zone for this domain already added",
			});
			return;
		}
		setFormData({
			...formData,
			additional_zones: [
				...formData.additional_zones,
				{
					domain: newZone.domain.trim(),
					create_zone: newZone.create_zone,
					...(newZone.create_zone ? {} : { zone_id: newZone.zone_id.trim() }),
				},
			],
		});
		setNewZone({ domain: "", create_zone: true, zone_id: "" });
		setPendingZonePrompt(null);
		setErrors({ ...errors, zone_domain: "", zone_id: "" });
	};

	const dismissZonePrompt = () => {
		setPendingZonePrompt(null);
		setNewZone({ domain: "", create_zone: true, zone_id: "" });
	};

	const removeAdditionalZone = (domain: string) => {
		setFormData({
			...formData,
			additional_zones: formData.additional_zones.filter(
				(z) => z.domain !== domain,
			),
		});
	};

	const handleSubmit = async () => {
		const newErrors: Record<string, string> = {};

		// Validate name
		if (!formData.name) {
			newErrors.name = "Distribution name is required";
		} else if (!/^[a-z0-9-]+$/.test(formData.name)) {
			newErrors.name =
				"Name must contain only lowercase letters, numbers, and hyphens";
		} else if (existingDistributions.includes(formData.name)) {
			newErrors.name = "A distribution with this name already exists";
		}

		// Validate origin
		if (!formData.origin_name) {
			newErrors.origin_name = "Origin name is required";
		}

		if (formData.origin_type === "amplify" && !formData.amplify_app_name) {
			newErrors.amplify_app_name = "Please select an Amplify app";
		}

		if (
			formData.origin_type === "s3" &&
			!formData.create_bucket &&
			!formData.bucket_name
		) {
			newErrors.bucket_name =
				"Bucket name is required when not creating a new bucket";
		}

		if (formData.origin_type === "custom" && !formData.custom_domain) {
			newErrors.custom_domain = "Custom domain is required";
		}

		if (Object.keys(newErrors).length > 0) {
			setErrors(newErrors);
			return;
		}

		// Build the CloudFront distribution config
		const distribution: CloudFrontConfig = {
			name: formData.name,
			enabled: formData.enabled,
			spa_mode: formData.spa_mode,
			price_class: formData.price_class,
			domain_aliases:
				formData.domain_aliases.length > 0
					? formData.domain_aliases
					: undefined,
			additional_zones:
				formData.additional_zones.length > 0
					? formData.additional_zones
					: undefined,
			default_root_object: "index.html",
			origins: [
				{
					name: formData.origin_name,
					type: formData.origin_type,
					...(formData.origin_type === "amplify" && {
						amplify_app_name: formData.amplify_app_name,
					}),
					...(formData.origin_type === "s3" && {
						...(formData.create_bucket
							? { create_bucket: true }
							: { bucket_name: formData.bucket_name }),
						use_oac: true,
					}),
					...(formData.origin_type === "custom" && {
						domain_name: formData.custom_domain,
					}),
				},
			],
		};

		await onAdd(distribution);
		handleClose();
	};

	const handleClose = () => {
		setFormData({
			name: "",
			enabled: true,
			spa_mode: true,
			price_class: "PriceClass_100",
			domain_aliases: [],
			additional_zones: [],
			origin_type: "amplify",
			origin_name: "frontend",
			amplify_app_name: "",
			bucket_name: "",
			create_bucket: true,
			custom_domain: "",
		});
		setNewZone({ domain: "", create_zone: true, zone_id: "" });
		setNewDomainAlias("");
		setErrors({});
		onClose();
	};

	return (
		<Dialog open={open} onOpenChange={handleClose}>
			<DialogContent className="max-w-lg max-h-[90vh] overflow-y-auto">
				<DialogHeader>
					<DialogTitle className="flex items-center gap-2">
						<Cloud className="h-5 w-5 text-orange-500" />
						Add CloudFront Distribution
					</DialogTitle>
					<DialogDescription>
						Create a new CloudFront CDN distribution for content delivery with
						support for wildcard domains and path-based routing.
					</DialogDescription>
				</DialogHeader>

				<div className="grid gap-4 py-4">
					{/* Distribution Name */}
					<div className="space-y-2">
						<Label htmlFor="name">Distribution Name *</Label>
						<Input
							id="name"
							value={formData.name}
							onChange={(e) =>
								setFormData({ ...formData, name: e.target.value })
							}
							placeholder="frontend-cdn"
							className={errors.name ? "border-red-500" : ""}
						/>
						{errors.name && (
							<p className="text-xs text-red-500">{errors.name}</p>
						)}
						<p className="text-xs text-muted-foreground">
							Unique identifier for this distribution (lowercase, hyphens
							allowed)
						</p>
					</div>

					{/* Enable Toggle */}
					<div className="flex items-center justify-between">
						<div>
							<Label>Enable Distribution</Label>
							<p className="text-xs text-muted-foreground">
								Create and deploy this CloudFront distribution
							</p>
						</div>
						<Switch
							checked={formData.enabled}
							onCheckedChange={(enabled) =>
								setFormData({ ...formData, enabled })
							}
						/>
					</div>

					{/* Origin Configuration */}
					<div className="space-y-3 border rounded-lg p-4">
						<h3 className="text-sm font-semibold">Origin Configuration</h3>

						<div className="grid grid-cols-2 gap-3">
							<div className="space-y-2">
								<Label htmlFor="origin_name">Origin Name *</Label>
								<Input
									id="origin_name"
									value={formData.origin_name}
									onChange={(e) =>
										setFormData({ ...formData, origin_name: e.target.value })
									}
									placeholder="frontend"
									className={errors.origin_name ? "border-red-500" : ""}
								/>
								{errors.origin_name && (
									<p className="text-xs text-red-500">{errors.origin_name}</p>
								)}
							</div>

							<div className="space-y-2">
								<Label htmlFor="origin_type">Origin Type *</Label>
								<Select
									value={formData.origin_type}
									onValueChange={(value: "amplify" | "s3" | "alb" | "custom") =>
										setFormData({ ...formData, origin_type: value })
									}
								>
									<SelectTrigger id="origin_type">
										<SelectValue />
									</SelectTrigger>
									<SelectContent>
										<SelectItem value="amplify">Amplify App</SelectItem>
										<SelectItem value="s3">S3 Bucket</SelectItem>
										<SelectItem value="alb">ALB (Load Balancer)</SelectItem>
										<SelectItem value="custom">Custom URL</SelectItem>
									</SelectContent>
								</Select>
							</div>
						</div>

						{/* Type-specific fields */}
						{formData.origin_type === "amplify" && (
							<div className="space-y-2">
								<Label htmlFor="amplify_app">Amplify App *</Label>
								<Select
									value={formData.amplify_app_name}
									onValueChange={(value) =>
										setFormData({ ...formData, amplify_app_name: value })
									}
								>
									<SelectTrigger
										id="amplify_app"
										className={errors.amplify_app_name ? "border-red-500" : ""}
									>
										<SelectValue placeholder="Select Amplify app" />
									</SelectTrigger>
									<SelectContent>
										{config?.amplify_apps?.map((app) => (
											<SelectItem key={app.name} value={app.name}>
												{app.name}
											</SelectItem>
										))}
									</SelectContent>
								</Select>
								{errors.amplify_app_name && (
									<p className="text-xs text-red-500">
										{errors.amplify_app_name}
									</p>
								)}
								{(!config?.amplify_apps ||
									config.amplify_apps.length === 0) && (
									<p className="text-xs text-amber-400">
										No Amplify apps configured. Add an Amplify app first.
									</p>
								)}
							</div>
						)}

						{formData.origin_type === "s3" && (
							<div className="space-y-3">
								<div className="flex items-center justify-between">
									<div>
										<Label>Create New Bucket</Label>
										<p className="text-xs text-muted-foreground">
											Auto-create an S3 bucket for this origin
										</p>
									</div>
									<Switch
										checked={formData.create_bucket}
										onCheckedChange={(create_bucket) =>
											setFormData({ ...formData, create_bucket })
										}
									/>
								</div>
								{formData.create_bucket ? (
									<div className="p-3 bg-muted/50 rounded-md">
										<p className="text-xs text-muted-foreground">
											Bucket will be created with name:{" "}
											<code className="text-xs bg-background px-1 py-0.5 rounded">
												{config?.project || "project"}-{config?.env || "env"}
												-cf-{formData.name || "dist"}-
												{formData.origin_name || "origin"}
											</code>
										</p>
									</div>
								) : (
									<div className="space-y-2">
										<Label htmlFor="bucket_name">
											Existing S3 Bucket Name *
										</Label>
										<Input
											id="bucket_name"
											value={formData.bucket_name}
											onChange={(e) =>
												setFormData({
													...formData,
													bucket_name: e.target.value,
												})
											}
											placeholder="my-static-assets"
											className={errors.bucket_name ? "border-red-500" : ""}
										/>
										{errors.bucket_name && (
											<p className="text-xs text-red-500">
												{errors.bucket_name}
											</p>
										)}
									</div>
								)}
								<p className="text-xs text-muted-foreground">
									Origin Access Control (OAC) will be enabled for secure S3
									access
								</p>
							</div>
						)}

						{formData.origin_type === "alb" && (
							<p className="text-xs text-muted-foreground">
								The ALB domain will be automatically resolved from your
								workloads configuration
							</p>
						)}

						{formData.origin_type === "custom" && (
							<div className="space-y-2">
								<Label htmlFor="custom_domain">Custom Domain *</Label>
								<Input
									id="custom_domain"
									value={formData.custom_domain}
									onChange={(e) =>
										setFormData({ ...formData, custom_domain: e.target.value })
									}
									placeholder="api.example.com"
									className={errors.custom_domain ? "border-red-500" : ""}
								/>
								{errors.custom_domain && (
									<p className="text-xs text-red-500">{errors.custom_domain}</p>
								)}
							</div>
						)}
					</div>

					{/* Domain Aliases */}
					<div className="space-y-3">
						<Label>Domain Aliases (optional)</Label>
						<p className="text-xs text-muted-foreground">
							Custom domains including wildcards (e.g., *.app.example.com)
						</p>
						<div className="flex gap-2">
							<Input
								placeholder="*.app.example.com"
								value={newDomainAlias}
								onChange={(e) => setNewDomainAlias(e.target.value)}
								onKeyDown={(e) => {
									if (e.key === "Enter") {
										e.preventDefault();
										addDomainAlias();
									}
								}}
							/>
							<Button type="button" onClick={addDomainAlias} size="sm">
								<Plus className="h-4 w-4" />
							</Button>
						</div>
						{errors.domain_alias && (
							<p className="text-xs text-red-500">{errors.domain_alias}</p>
						)}
						{formData.domain_aliases.map((alias) => (
							<div
								key={alias}
								className="flex items-center justify-between p-2 bg-muted rounded-md"
							>
								<div className="flex items-center gap-2">
									<Globe className="h-4 w-4 text-muted-foreground" />
									<code className="text-sm">{alias}</code>
								</div>
								<Button
									variant="ghost"
									size="sm"
									onClick={() => removeDomainAlias(alias)}
								>
									<Trash2 className="h-4 w-4 text-destructive" />
								</Button>
							</div>
						))}
					</div>

					{/* Zone Configuration Prompt (auto-triggered when adding non-main domain alias) */}
					{pendingZonePrompt && (
						<div className="space-y-3 border-2 border-amber-500 rounded-lg p-4 bg-amber-500/10">
							<div>
								<Label className="text-amber-200">
									DNS Zone Required for "{pendingZonePrompt}"
								</Label>
								<p className="text-xs text-muted-foreground">
									This domain is different from your main domain (
									{config?.domain?.domain_name || "not configured"}). Configure
									how to handle DNS for certificate validation.
								</p>
							</div>
							<div className="space-y-3">
								<Select
									value={newZone.create_zone ? "create" : "existing"}
									onValueChange={(value) =>
										setNewZone({ ...newZone, create_zone: value === "create" })
									}
								>
									<SelectTrigger>
										<SelectValue />
									</SelectTrigger>
									<SelectContent>
										<SelectItem value="create">
											Create new Route 53 zone for {pendingZonePrompt}
										</SelectItem>
										<SelectItem value="existing">
											Use existing Route 53 zone
										</SelectItem>
									</SelectContent>
								</Select>
								{!newZone.create_zone && (
									<Input
										placeholder="Zone ID (e.g., Z1234567890ABC)"
										value={newZone.zone_id}
										onChange={(e) =>
											setNewZone({ ...newZone, zone_id: e.target.value })
										}
										className={errors.zone_id ? "border-red-500" : ""}
									/>
								)}
								{errors.zone_id && (
									<p className="text-xs text-red-500">{errors.zone_id}</p>
								)}
								<div className="flex gap-2">
									<Button type="button" onClick={addAdditionalZone} size="sm">
										Configure Zone
									</Button>
									<Button
										type="button"
										onClick={dismissZonePrompt}
										size="sm"
										variant="ghost"
									>
										Skip (use main zone)
									</Button>
								</div>
							</div>
						</div>
					)}

					{/* Additional Zones (for non-main domain aliases) */}
					{formData.domain_aliases.length > 0 && !pendingZonePrompt && (
						<div className="space-y-3 border rounded-lg p-4 bg-muted/30">
							<div>
								<Label>Additional DNS Zones (optional)</Label>
								<p className="text-xs text-muted-foreground">
									Add Route 53 zones for domains outside your main domain (
									{config?.domain?.domain_name || "not configured"})
								</p>
							</div>
							<div className="space-y-3">
								<div className="grid grid-cols-2 gap-2">
									<Input
										placeholder="otherdomain.com"
										value={newZone.domain}
										onChange={(e) =>
											setNewZone({ ...newZone, domain: e.target.value })
										}
									/>
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
											<SelectItem value="create">Create new zone</SelectItem>
											<SelectItem value="existing">
												Use existing zone
											</SelectItem>
										</SelectContent>
									</Select>
								</div>
								{!newZone.create_zone && (
									<Input
										placeholder="Zone ID (e.g., Z1234567890ABC)"
										value={newZone.zone_id}
										onChange={(e) =>
											setNewZone({ ...newZone, zone_id: e.target.value })
										}
										className={errors.zone_id ? "border-red-500" : ""}
									/>
								)}
								{errors.zone_id && (
									<p className="text-xs text-red-500">{errors.zone_id}</p>
								)}
								{errors.zone_domain && (
									<p className="text-xs text-red-500">{errors.zone_domain}</p>
								)}
								<Button
									type="button"
									onClick={addAdditionalZone}
									size="sm"
									variant="outline"
								>
									<Plus className="h-4 w-4 mr-1" />
									Add Zone
								</Button>
							</div>
							{formData.additional_zones.map((zone) => (
								<div
									key={zone.domain}
									className="flex items-center justify-between p-2 bg-background rounded-md border"
								>
									<div>
										<code className="text-sm font-medium">{zone.domain}</code>
										<p className="text-xs text-muted-foreground">
											{zone.create_zone
												? "Will create new Route 53 zone"
												: `Using zone: ${zone.zone_id}`}
										</p>
									</div>
									<Button
										variant="ghost"
										size="sm"
										onClick={() => removeAdditionalZone(zone.domain)}
									>
										<Trash2 className="h-4 w-4 text-destructive" />
									</Button>
								</div>
							))}
						</div>
					)}

					{/* SPA Mode */}
					<div className="flex items-center justify-between">
						<div>
							<Label>SPA Mode</Label>
							<p className="text-xs text-muted-foreground">
								Handle 404 errors by returning index.html (for
								React/Vue/Angular)
							</p>
						</div>
						<Switch
							checked={formData.spa_mode}
							onCheckedChange={(spa_mode) =>
								setFormData({ ...formData, spa_mode })
							}
						/>
					</div>

					{/* Price Class */}
					<div className="space-y-2">
						<Label>Price Class</Label>
						<Select
							value={formData.price_class}
							onValueChange={(
								value: "PriceClass_100" | "PriceClass_200" | "PriceClass_All",
							) => setFormData({ ...formData, price_class: value })}
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
				</div>

				<DialogFooter>
					<Button variant="outline" onClick={handleClose}>
						Cancel
					</Button>
					<Button onClick={handleSubmit}>Add Distribution</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
};
