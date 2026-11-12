import { Layers } from "lucide-react";
import { useId, useState } from "react";
import type {
	ValkeyConfig,
	ValkeyEngineVersion,
	YamlInfrastructureConfig,
} from "../types/yamlConfig";
import {
	unknownWorkloadNames,
	workloadNames,
} from "../utils/workloadReferences";
import { Alert, AlertDescription } from "./ui/alert";
import { Badge } from "./ui/badge";
import { Button } from "./ui/button";
import {
	Card,
	CardContent,
	CardDescription,
	CardHeader,
	CardTitle,
} from "./ui/card";
import { Checkbox } from "./ui/checkbox";
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

interface ValkeyNodePropertiesProps {
	config: YamlInfrastructureConfig;
	onConfigChange: (config: Partial<YamlInfrastructureConfig>) => void;
}

// Same values as app/valkey.go and modules/valkey.
const ENGINE_VERSIONS: ValkeyEngineVersion[] = ["7", "8", "9"];
const DEFAULT_ENGINE_VERSION: ValkeyEngineVersion = "8";
const DEFAULT_MAX_STORAGE_GB = 1;

function storageError(text: string): string | null {
	if (text === "") return "Enter a whole number of GB, at least 1.";
	const value = Number(text);
	if (!Number.isInteger(value) || value < 1) {
		return "Enter a whole number of GB, at least 1.";
	}
	return null;
}

/**
 * The Valkey node: serverless Valkey on or off, its major version, the cap on
 * stored data, and the workloads that may connect.
 *
 * Access is by security group only: a listed workload can reach the cache and
 * gets VALKEY_HOST, VALKEY_PORT and VALKEY_URL; nothing else can reach it.
 */
export function ValkeyNodeProperties({
	config,
	onConfigChange,
}: ValkeyNodePropertiesProps) {
	const uid = useId();
	const valkey: ValkeyConfig = config.valkey || { enabled: false };
	const access = valkey.access || [];
	const workloads = workloadNames(config);
	const unknown = unknownWorkloadNames(config, access);

	const savedStorage = valkey.max_storage_gb ?? DEFAULT_MAX_STORAGE_GB;
	const [storageText, setStorageText] = useState(String(savedStorage));
	const currentStorageError = storageError(storageText);

	const save = (updates: Partial<ValkeyConfig>) => {
		onConfigChange({ valkey: { ...valkey, ...updates } });
	};

	const handleToggle = (enabled: boolean) => {
		save(
			enabled
				? {
						enabled,
						engine_version: valkey.engine_version || DEFAULT_ENGINE_VERSION,
						max_storage_gb: valkey.max_storage_gb ?? DEFAULT_MAX_STORAGE_GB,
						access,
					}
				: { enabled },
		);
	};

	const handleStorageChange = (text: string) => {
		setStorageText(text);
		if (storageError(text) === null) save({ max_storage_gb: Number(text) });
	};

	const handleAccessChange = (name: string, allowed: boolean) => {
		const next = access.filter((n) => n !== name);
		if (allowed) next.push(name);
		save({ access: next });
	};

	const handleDropUnknown = () => {
		const drop = new Set(unknown);
		save({ access: access.filter((n) => !drop.has(n)) });
	};

	return (
		<div className="space-y-4">
			<Card>
				<CardHeader>
					<CardTitle className="flex items-center gap-2">
						<Layers className="w-5 h-5" />
						Valkey
					</CardTitle>
					<CardDescription>
						A serverless Valkey cache in the environment's VPC. It bills by the
						hour from about $6 a month plus stored data, so leave it off until a
						workload needs it.
					</CardDescription>
				</CardHeader>
				<CardContent>
					<div className="flex items-center justify-between">
						<div className="space-y-1">
							<Label htmlFor={`${uid}-enabled`} className="text-base">
								Enable Valkey
							</Label>
							<p className="text-sm text-muted-foreground">
								No password: only the workloads you choose below can reach it.
							</p>
						</div>
						<Switch
							id={`${uid}-enabled`}
							checked={valkey.enabled}
							onCheckedChange={handleToggle}
						/>
					</div>
				</CardContent>
			</Card>

			{valkey.enabled && (
				<>
					<Card>
						<CardHeader>
							<CardTitle>Settings</CardTitle>
						</CardHeader>
						<CardContent className="space-y-4">
							<div className="space-y-2">
								<Label htmlFor={`${uid}-version`}>Valkey version</Label>
								<Select
									value={valkey.engine_version || DEFAULT_ENGINE_VERSION}
									onValueChange={(value) =>
										save({ engine_version: value as ValkeyEngineVersion })
									}
								>
									<SelectTrigger id={`${uid}-version`}>
										<SelectValue />
									</SelectTrigger>
									<SelectContent>
										{ENGINE_VERSIONS.map((v) => (
											<SelectItem key={v} value={v}>
												Valkey {v}
											</SelectItem>
										))}
									</SelectContent>
								</Select>
							</div>

							<div className="space-y-2">
								<Label htmlFor={`${uid}-storage`}>Storage cap (GB)</Label>
								<Input
									id={`${uid}-storage`}
									type="number"
									min={1}
									step={1}
									value={storageText}
									aria-invalid={currentStorageError !== null}
									onChange={(e) => handleStorageChange(e.target.value.trim())}
								/>
								{currentStorageError ? (
									<p className="text-sm text-destructive">
										{currentStorageError}
									</p>
								) : (
									<p className="text-xs text-muted-foreground">
										The most data the cache may hold. Writes past it fail;
										storage is billed for what is used, up to this cap.
									</p>
								)}
							</div>
						</CardContent>
					</Card>

					<Card>
						<CardHeader>
							<CardTitle>Access</CardTitle>
							<CardDescription>
								Each chosen workload can connect over TLS and gets VALKEY_HOST,
								VALKEY_PORT and VALKEY_URL.
							</CardDescription>
						</CardHeader>
						<CardContent className="space-y-3">
							{unknown.length > 0 && (
								<Alert variant="destructive">
									<AlertDescription>
										<span>
											{unknown.join(", ")} {unknown.length === 1 ? "is" : "are"}{" "}
											not a workload of this environment; generate refuses the
											config until {unknown.length === 1 ? "it is" : "they are"}{" "}
											removed.
										</span>
										<Button
											variant="outline"
											size="sm"
											onClick={handleDropUnknown}
										>
											Remove
										</Button>
									</AlertDescription>
								</Alert>
							)}
							{access.length === 0 && (
								<p className="text-sm text-muted-foreground">
									No workload can connect yet.
								</p>
							)}
							{workloads.map((name) => (
								<div key={name} className="flex items-center gap-2">
									<Checkbox
										id={`${uid}-access-${name}`}
										checked={access.includes(name)}
										onCheckedChange={(checked) =>
											handleAccessChange(name, checked === true)
										}
									/>
									<Label htmlFor={`${uid}-access-${name}`}>{name}</Label>
									{access.includes(name) && (
										<Badge variant="secondary">connects</Badge>
									)}
								</div>
							))}
						</CardContent>
					</Card>
				</>
			)}
		</div>
	);
}
