import {
	AlertCircle,
	AlertTriangle,
	ChevronDown,
	ChevronRight,
	HardDrive,
	Info,
	Network,
	Server,
	Terminal,
	Trash2,
} from "lucide-react";
import { useId, useMemo, useRef, useState } from "react";
import type { ComputePosture } from "../api/infrastructure";
import { useInstanceTypes } from "../hooks/use-instance-types";
import type {
	ComputeAMIFamily,
	ComputeCapacityType,
	ComputeNetworkMode,
	ComputePool,
	YamlInfrastructureConfig,
} from "../types/yamlConfig";
import { ComputeRecommendation } from "./ComputeRecommendation";
import { InstanceTypePicker } from "./InstanceTypePicker";
import { Alert, AlertDescription } from "./ui/alert";
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
import { Switch } from "./ui/switch";
import { Textarea } from "./ui/textarea";

/**
 * One pool's fields.
 *
 * The prop shape here IS the C-3 guard (architecture.md §8.4). `onChange` takes
 * an index and a PATCH; this component never receives the pools array and
 * therefore cannot emit one. `mergeConfigUpdates` is one level deep, so an
 * editor that emitted `{ compute: { pools: [thisPool] } }` would delete every
 * sibling pool on a save the user never knowingly triggered — and `web/` has no
 * test runner that could catch it. Making the array inexpressible here turns
 * that class of bug into a `tsc` error instead.
 *
 * `allPools` is read-only and exists for one reason: telling the user their new
 * name collides with an existing one before the write happens (FR-34).
 */
interface ComputePoolEditorProps {
	pool: ComputePool;
	index: number;
	allPools: readonly ComputePool[];
	config: YamlInfrastructureConfig;
	onChange: (index: number, patch: Partial<ComputePool>) => void;
	onDelete: (index: number) => void;
}

const POOL_NAME_PATTERN = /^[a-z][a-z0-9-]{0,31}$/;

const AMI_FAMILY_ARCH: Record<ComputeAMIFamily, "x86_64" | "arm64"> = {
	al2023: "x86_64",
	al2023_arm64: "arm64",
	al2023_gpu: "x86_64",
};

const AMI_FAMILY_LABELS: Record<ComputeAMIFamily, string> = {
	al2023: "Amazon Linux 2023 (x86_64)",
	al2023_arm64: "Amazon Linux 2023 (arm64 / Graviton)",
	al2023_gpu: "Amazon Linux 2023 GPU (x86_64)",
};

const CAPACITY_TYPE_LABELS: Record<ComputeCapacityType, string> = {
	on_demand: "On-demand only",
	spot: "Spot only",
	spot_with_base: "Spot with an on-demand base",
};

/** §8.5 — a rename is an in-place capacity-provider swap, so the copy is proportionate. */
const RENAME_WARNING =
	"Renaming this pool changes the capacity provider AWS knows it by. Terraform will create the new capacity provider, move each service onto it with a rolling deployment, and delete the old one. No service is destroyed, but every task is replaced.";

/** §8.5 — network mode changes the task definition, the target group and the DNS record at once. */
const NETWORK_MODE_WARNING =
	"Changing this pool's network mode changes how every service on it is reached: its task definition, its load-balancer target group, and its service-discovery record all change. Services already on this pool will be recreated. Plan this for a maintenance window.";

function poolNameError(
	name: string,
	index: number,
	allPools: readonly ComputePool[],
): string | null {
	if (!name)
		return "A pool needs a name — it becomes the capacity provider's name in AWS.";
	if (!POOL_NAME_PATTERN.test(name)) {
		return "Must start with a lowercase letter and contain only lowercase letters, digits and hyphens (max 32 characters).";
	}
	const clash = allPools.findIndex((p, i) => i !== index && p.name === name);
	if (clash !== -1) {
		return `Pool ${clash + 1} already uses this name. Terraform keys pools by name with for_each, so two entries would silently collapse into one.`;
	}
	return null;
}

export function ComputePoolEditor({
	pool,
	index,
	allPools,
	config,
	onChange,
	onDelete,
}: ComputePoolEditorProps) {
	const uid = useId();
	const [expanded, setExpanded] = useState(true);
	const [posture, setPosture] = useState<ComputePosture>("performance-first");
	const { find } = useInstanceTypes(config.env);

	// The values as they were when this editor mounted, i.e. as they stand in
	// {env}.yaml. The config prop is rewritten on every keystroke, so "changed
	// away from the saved value" cannot be read from it.
	const savedName = useRef(pool.name);
	const savedNetworkMode = useRef(pool.network_mode ?? "bridge");

	const capacityType: ComputeCapacityType = pool.capacity_type ?? "on_demand";
	const networkMode: ComputeNetworkMode = pool.network_mode ?? "bridge";
	const amiFamily: ComputeAMIFamily = pool.ami_family ?? "al2023";
	const instanceTypes = pool.instance_types ?? [];
	const isOnDemand = capacityType === "on_demand";
	const enabled = pool.enabled !== false;

	const nameError = poolNameError(pool.name, index, allPools);

	const assignedUnits = useMemo(() => {
		const units: string[] = [];
		if (
			config.workload?.backend_runtime === "ec2" &&
			config.workload?.backend_compute_pool === pool.name
		) {
			units.push("backend");
		}
		for (const service of config.services ?? []) {
			if (service.runtime === "ec2" && service.compute_pool === pool.name) {
				units.push(service.name);
			}
		}
		return units;
	}, [config.workload, config.services, pool.name]);

	// Rules 4, 5 and 10 mirrored client-side from the catalog's own architecture
	// data rather than from the instance name, so a family AWS ships next month
	// does not produce a false warning. Generate time is still the enforcement
	// point; this is only an earlier, softer signal.
	const archMismatches = useMemo(() => {
		const wanted = AMI_FAMILY_ARCH[amiFamily];
		return instanceTypes.filter((t) => {
			const info = find(t);
			if (!info || info.architectures.length === 0) return false;
			return !info.architectures.includes(wanted);
		});
	}, [instanceTypes, amiFamily, find]);

	const gpuMismatches = useMemo(() => {
		if (amiFamily !== "al2023_gpu") return [];
		return instanceTypes.filter((t) => {
			const info = find(t);
			return info ? info.gpuCount === 0 : false;
		});
	}, [instanceTypes, amiFamily, find]);

	const minSize = pool.min_size ?? 1;
	const maxSize = pool.max_size ?? 6;
	const sizeInverted = minSize > maxSize;

	return (
		<div className="rounded-lg border border-gray-700 bg-gray-900">
			{/* Header — always visible, so a collapsed pool still says what it is. */}
			<div className="flex items-center gap-3 p-3">
				<button
					type="button"
					onClick={() => setExpanded((v) => !v)}
					aria-expanded={expanded}
					aria-label={expanded ? "Collapse pool" : "Expand pool"}
					className="text-gray-400 hover:text-gray-200 shrink-0"
				>
					{expanded ? (
						<ChevronDown className="w-4 h-4" />
					) : (
						<ChevronRight className="w-4 h-4" />
					)}
				</button>

				<Server
					className={`w-4 h-4 shrink-0 ${enabled ? "text-blue-400" : "text-gray-600"}`}
				/>

				{/* FR-33's per-pool summary, readable without expanding: name,
				    enabled, types, capacity type and its spot mix, size range,
				    target capacity, AMI family, network mode and how many
				    workloads are on it. */}
				<div className="min-w-0 flex-1">
					<p className="text-sm font-mono text-gray-100 truncate">
						{pool.name || "unnamed"}
						{!enabled && (
							<span className="ml-2 font-sans text-xs text-gray-500">
								disabled
							</span>
						)}
					</p>
					<p className="text-xs text-gray-500 truncate">
						{instanceTypes.length > 0
							? instanceTypes.join(", ")
							: "no instance types"}{" "}
						· {CAPACITY_TYPE_LABELS[capacityType].toLowerCase()}
						{capacityType === "spot_with_base"
							? ` (${pool.on_demand_base ?? 0} on-demand)`
							: ""}{" "}
						· {minSize}–{maxSize} instances · {pool.target_capacity ?? 100}%
						target · {amiFamily} · {networkMode}
						{assignedUnits.length > 0
							? ` · ${assignedUnits.length} workload${assignedUnits.length === 1 ? "" : "s"}`
							: " · unused"}
					</p>
				</div>

				<Switch
					checked={enabled}
					onCheckedChange={(checked) => onChange(index, { enabled: checked })}
					aria-label={`Enable pool ${pool.name}`}
				/>

				<Button
					type="button"
					variant="ghost"
					size="sm"
					onClick={() => onDelete(index)}
					aria-label={`Delete pool ${pool.name}`}
					className="text-red-400 hover:text-red-300 hover:bg-red-900/20 shrink-0"
				>
					<Trash2 className="w-4 h-4" />
				</Button>
			</div>

			{expanded && (
				<div className="px-3 pb-3 space-y-4 border-t border-gray-800 pt-3">
					{/* ---- identity -------------------------------------------- */}
					<div className="space-y-2">
						<Label htmlFor={`${uid}-name`}>Pool Name</Label>
						<Input
							id={`${uid}-name`}
							value={pool.name}
							onChange={(e) =>
								onChange(index, {
									name: e.target.value.toLowerCase().replace(/[^a-z0-9-]/g, ""),
								})
							}
							className="bg-gray-800 border-gray-600 font-mono"
						/>
						{nameError ? (
							<Alert className="border-red-600 bg-red-900/20">
								<AlertCircle className="h-4 w-4 text-red-400" />
								<AlertDescription className="text-red-200 text-xs">
									{nameError}
								</AlertDescription>
							</Alert>
						) : (
							<p className="text-xs text-gray-500">
								Becomes the ECS capacity provider name. Lowercase letters,
								digits and hyphens.
							</p>
						)}

						{pool.name !== savedName.current && (
							<Alert className="border-yellow-600 bg-yellow-900/20">
								<AlertTriangle className="h-4 w-4 text-yellow-400" />
								<AlertDescription className="text-yellow-200 text-xs">
									{RENAME_WARNING}
								</AlertDescription>
							</Alert>
						)}
					</div>

					{/* ---- recommendation ---------------------------------------- */}
					<ComputeRecommendation
						env={config.env}
						pool={savedName.current}
						posture={posture}
						onPostureChange={setPosture}
						networkMode={networkMode}
						amiFamily={amiFamily}
						onAccept={(suggested) =>
							// Everything except the name: accepting a size must never
							// rename a pool that services already point at.
							onChange(index, {
								instance_types: suggested.instance_types,
								capacity_type: suggested.capacity_type as ComputeCapacityType,
								on_demand_base:
									suggested.capacity_type === "on_demand"
										? undefined
										: suggested.on_demand_base,
								min_size: suggested.min_size,
								max_size: suggested.max_size,
								target_capacity: suggested.target_capacity,
								network_mode: suggested.network_mode as ComputeNetworkMode,
								ami_family: suggested.ami_family as ComputeAMIFamily,
								root_volume_gb: suggested.root_volume_gb,
							})
						}
					/>

					{/* ---- instance types ---------------------------------------- */}
					<InstanceTypePicker
						env={config.env}
						selected={instanceTypes}
						onChange={(types) => onChange(index, { instance_types: types })}
						spotOpen={!isOnDemand}
					/>

					{/* ---- capacity type: mode control, then two exclusive panels -- */}
					<div className="space-y-3">
						<div className="space-y-2">
							<Label htmlFor={`${uid}-capacity-type`}>Capacity Type</Label>
							<Select
								value={capacityType}
								onValueChange={(value: string) => {
									const next = value as ComputeCapacityType;
									// Cross-field auto-correction, mirroring generate-time
									// rule 3: on_demand_base means nothing under on_demand,
									// and a config that reads "one guaranteed instance, the
									// rest spot" would quietly bill full price for all of
									// them.
									onChange(index, {
										capacity_type: next,
										...(next === "on_demand"
											? { on_demand_base: undefined }
											: {}),
									});
								}}
							>
								<SelectTrigger
									id={`${uid}-capacity-type`}
									className="bg-gray-800 border-gray-600"
								>
									<SelectValue />
								</SelectTrigger>
								<SelectContent>
									{(
										Object.keys(CAPACITY_TYPE_LABELS) as ComputeCapacityType[]
									).map((value) => (
										<SelectItem key={value} value={value}>
											{CAPACITY_TYPE_LABELS[value]}
										</SelectItem>
									))}
								</SelectContent>
							</Select>
						</div>

						{isOnDemand && (
							<div className="p-3 bg-gray-800 rounded-lg space-y-1">
								<h4 className="text-sm font-semibold text-gray-300">
									On-Demand Capacity
								</h4>
								<p className="text-xs text-gray-400">
									Every instance in this pool is on-demand. Predictable, never
									reclaimed, and the most expensive of the three. AWS will not
									interrupt these instances.
								</p>
							</div>
						)}

						{!isOnDemand && (
							<div className="p-3 bg-gray-800 rounded-lg space-y-3">
								<div>
									<h4 className="text-sm font-semibold text-gray-300">
										Spot Capacity
									</h4>
									<p className="text-xs text-gray-400 mt-1">
										AWS can reclaim a spot instance with two minutes' notice.
										ECS drains the instance and reschedules its tasks, so a
										service with more than one task survives it — a single-task
										service does not.
									</p>
								</div>

								{capacityType === "spot_with_base" && (
									<div className="space-y-2">
										<Label htmlFor={`${uid}-on-demand-base`}>
											On-Demand Base (instances)
										</Label>
										<Input
											id={`${uid}-on-demand-base`}
											type="number"
											min="0"
											max={maxSize}
											value={pool.on_demand_base ?? 0}
											onChange={(e) => {
												const parsed = Number.parseInt(e.target.value, 10);
												onChange(index, {
													on_demand_base: Number.isNaN(parsed)
														? 0
														: Math.max(0, parsed),
												});
											}}
											className="bg-gray-900 border-gray-600"
										/>
										<p className="text-xs text-gray-500">
											Counts INSTANCES, not tasks: this many instances are held
											on-demand before the pool buys any spot at all.
										</p>
									</div>
								)}
							</div>
						)}

						{/* Rule 3, surfaced inline rather than at generate time. */}
						{isOnDemand && pool.on_demand_base !== undefined && (
							<Alert className="border-yellow-600 bg-yellow-900/20">
								<AlertCircle className="h-4 w-4 text-yellow-400" />
								<AlertDescription className="text-yellow-200 text-xs">
									on_demand_base is set to {pool.on_demand_base} but the
									capacity type is on-demand, so the value is ignored. Generate
									will refuse this combination.{" "}
									<button
										type="button"
										onClick={() =>
											onChange(index, { on_demand_base: undefined })
										}
										className="underline hover:text-white"
									>
										Remove it
									</button>
									.
								</AlertDescription>
							</Alert>
						)}
					</div>

					{/* ---- sizing ------------------------------------------------ */}
					<div className="space-y-3">
						<div className="grid grid-cols-2 gap-4">
							<div className="space-y-2">
								<Label htmlFor={`${uid}-min-size`}>Min Instances</Label>
								<Input
									id={`${uid}-min-size`}
									type="number"
									min="0"
									value={minSize}
									onChange={(e) => {
										const parsed = Number.parseInt(e.target.value, 10);
										const nextMin = Number.isNaN(parsed)
											? 0
											: Math.max(0, parsed);
										// Auto-correct the ceiling rather than let the user
										// save an ASG that AWS refuses to create outright.
										onChange(index, {
											min_size: nextMin,
											...(nextMin > maxSize ? { max_size: nextMin } : {}),
										});
									}}
									className="bg-gray-800 border-gray-600"
								/>
							</div>
							<div className="space-y-2">
								<Label htmlFor={`${uid}-max-size`}>Max Instances</Label>
								<Input
									id={`${uid}-max-size`}
									type="number"
									min="0"
									value={maxSize}
									onChange={(e) => {
										const parsed = Number.parseInt(e.target.value, 10);
										onChange(index, {
											max_size: Number.isNaN(parsed) ? 0 : Math.max(0, parsed),
										});
									}}
									className="bg-gray-800 border-gray-600"
								/>
							</div>
						</div>

						{sizeInverted && (
							<Alert className="border-red-600 bg-red-900/20">
								<AlertCircle className="h-4 w-4 text-red-400" />
								<AlertDescription className="text-red-200 text-xs">
									Min ({minSize}) is above max ({maxSize}). AWS rejects
									CreateAutoScalingGroup outright, so the pool never exists and
									every task placed on it sits in PROVISIONING with no capacity
									to run on.
								</AlertDescription>
							</Alert>
						)}

						<div className="space-y-2">
							<Label htmlFor={`${uid}-target-capacity`}>
								Target Capacity (%)
							</Label>
							<Input
								id={`${uid}-target-capacity`}
								type="number"
								min="1"
								max="100"
								value={pool.target_capacity ?? 100}
								onChange={(e) => {
									const parsed = Number.parseInt(e.target.value, 10);
									onChange(index, {
										target_capacity: Number.isNaN(parsed)
											? 100
											: Math.min(100, Math.max(1, parsed)),
									});
								}}
								className="bg-gray-800 border-gray-600"
							/>
							<p className="text-xs text-gray-500">
								How full ECS keeps the pool before adding an instance. 100 packs
								tightly and scales late; lower values keep spare room for a task
								to place immediately.
							</p>
						</div>
					</div>

					{/* ---- networking -------------------------------------------- */}
					<div className="space-y-3">
						<div className="space-y-2">
							<Label
								htmlFor={`${uid}-network-mode`}
								className="flex items-center gap-2"
							>
								<Network className="w-4 h-4 text-blue-400" />
								Network Mode
							</Label>
							<Select
								value={networkMode}
								onValueChange={(value: string) =>
									onChange(index, {
										network_mode: value as ComputeNetworkMode,
									})
								}
							>
								<SelectTrigger
									id={`${uid}-network-mode`}
									className="bg-gray-800 border-gray-600"
								>
									<SelectValue />
								</SelectTrigger>
								<SelectContent>
									<SelectItem value="bridge">
										bridge — tasks share the instance's network
									</SelectItem>
									<SelectItem value="awsvpc">
										awsvpc — each task gets its own interface
									</SelectItem>
								</SelectContent>
							</Select>
							<p className="text-xs text-gray-500">
								{networkMode === "bridge"
									? "A task egresses through the instance's own public interface, so outbound calls work with no NAT gateway. Host ports are assigned dynamically at placement, and services on this pool use instance-type target groups and SRV service discovery."
									: "Each task gets its own interface and its own address, so host ports are fixed and target groups are ip-type. That interface is private: the pool needs an egress path you provide."}
							</p>
						</div>

						{networkMode !== savedNetworkMode.current && (
							<Alert className="border-yellow-600 bg-yellow-900/20">
								<AlertTriangle className="h-4 w-4 text-yellow-400" />
								<AlertDescription className="text-yellow-200 text-xs">
									{NETWORK_MODE_WARNING}
								</AlertDescription>
							</Alert>
						)}

						{networkMode === "awsvpc" && (
							<div className="p-3 bg-gray-800 rounded-lg space-y-3">
								<div className="flex items-start justify-between gap-3">
									<div className="space-y-1">
										<Label className="text-sm font-medium">
											This environment has an egress path
										</Label>
										<p className="text-xs text-gray-400">
											An assertion, not a switch that creates anything. meroku
											builds no NAT gateway: this VPC is public-subnet-only by
											design.
										</p>
									</div>
									<Switch
										checked={pool.assume_egress === true}
										onCheckedChange={(checked) =>
											onChange(index, { assume_egress: checked })
										}
										aria-label="Assert an egress path exists"
									/>
								</div>

								{pool.assume_egress !== true && (
									<Alert className="border-red-600 bg-red-900/20">
										<AlertCircle className="h-4 w-4 text-red-400" />
										<AlertDescription className="text-red-200 text-xs space-y-2">
											<p>
												Generate will refuse this pool. Under awsvpc each task
												gets its own interface and AWS gives that interface a
												private address only — assign_public_ip is Fargate-only,
												and the subnet's map_public_ip_on_launch applies to the
												instance's primary interface, not the task's.
											</p>
											<p>
												The task starts, passes its health check and serves
												traffic, and every outbound call it makes — S3, SES,
												SQS, Cognito, Secrets Manager, ECS Exec, any third-party
												API — times out silently.
											</p>
											<p>
												Either switch back to bridge, or turn the assertion
												above on to declare that the subnets you place tasks in
												route 0.0.0.0/0 to a NAT gateway you manage yourself.
											</p>
										</AlertDescription>
									</Alert>
								)}
							</div>
						)}
					</div>

					{/* ---- image ------------------------------------------------- */}
					<div className="space-y-3">
						<div className="space-y-2">
							<Label htmlFor={`${uid}-ami-family`}>AMI Family</Label>
							<Select
								value={amiFamily}
								onValueChange={(value: string) =>
									onChange(index, { ami_family: value as ComputeAMIFamily })
								}
							>
								<SelectTrigger
									id={`${uid}-ami-family`}
									className="bg-gray-800 border-gray-600"
								>
									<SelectValue />
								</SelectTrigger>
								<SelectContent>
									{(Object.keys(AMI_FAMILY_LABELS) as ComputeAMIFamily[]).map(
										(value) => (
											<SelectItem key={value} value={value}>
												{AMI_FAMILY_LABELS[value]}
											</SelectItem>
										),
									)}
								</SelectContent>
							</Select>
						</div>

						{archMismatches.length > 0 && (
							<Alert className="border-red-600 bg-red-900/20">
								<AlertCircle className="h-4 w-4 text-red-400" />
								<AlertDescription className="text-red-200 text-xs">
									{archMismatches.join(", ")} do not run{" "}
									{AMI_FAMILY_ARCH[amiFamily]}. An AMI only boots on the
									architecture it was built for: the Auto Scaling group launches
									the instance, EC2 rejects it, and the failure is recorded as a
									scaling activity — not as a Terraform error and not as an ECS
									event. The pool simply never gains capacity.
								</AlertDescription>
							</Alert>
						)}

						{gpuMismatches.length > 0 && (
							<Alert className="border-yellow-600 bg-yellow-900/20">
								<AlertCircle className="h-4 w-4 text-yellow-400" />
								<AlertDescription className="text-yellow-200 text-xs">
									{gpuMismatches.join(", ")} carry no GPU, so the GPU AMI's
									drivers have nothing to bind to. Use the plain al2023 family
									or choose a GPU instance type.
								</AlertDescription>
							</Alert>
						)}

						<div className="space-y-2">
							<Label htmlFor={`${uid}-ami-id`}>
								AMI ID (optional override)
							</Label>
							<Input
								id={`${uid}-ami-id`}
								value={pool.ami_id ?? ""}
								placeholder="ami-0123456789abcdef0"
								onChange={(e) =>
									onChange(index, { ami_id: e.target.value || undefined })
								}
								className="bg-gray-800 border-gray-600 font-mono"
							/>
							<p className="text-xs text-gray-500">
								Wins over the family above. The AMI must have the ECS agent
								installed and must match your instance types' architecture —
								neither is checked for you.
							</p>
						</div>

						<div className="space-y-2">
							<Label
								htmlFor={`${uid}-root-volume`}
								className="flex items-center gap-2"
							>
								<HardDrive className="w-4 h-4 text-blue-400" />
								Root Volume (GB)
							</Label>
							<Input
								id={`${uid}-root-volume`}
								type="number"
								min="8"
								value={pool.root_volume_gb ?? 30}
								onChange={(e) => {
									const parsed = Number.parseInt(e.target.value, 10);
									onChange(index, {
										root_volume_gb: Number.isNaN(parsed)
											? 30
											: Math.max(8, parsed),
									});
								}}
								className="bg-gray-800 border-gray-600"
							/>
							<p className="text-xs text-gray-500">
								Holds the OS and every container image the instance pulls. 30 GB
								is the default; image-heavy workloads need more.
							</p>
						</div>

						<div className="space-y-2">
							<Label
								htmlFor={`${uid}-user-data`}
								className="flex items-center gap-2"
							>
								<Terminal className="w-4 h-4 text-blue-400" />
								Extra User Data
							</Label>
							<Textarea
								id={`${uid}-user-data`}
								value={pool.user_data_extra ?? ""}
								rows={4}
								placeholder={
									"#!/bin/bash\necho 'runs after the ECS agent is configured'"
								}
								onChange={(e) =>
									onChange(index, {
										user_data_extra: e.target.value || undefined,
									})
								}
								className="bg-gray-800 border-gray-600 font-mono text-xs"
							/>
							<p className="text-xs text-gray-500">
								Appended after meroku's own bootstrap, which registers the
								instance with the cluster. A script that fails here leaves the
								instance running but never registered.
							</p>
						</div>
					</div>

					{/* ---- what this pool carries -------------------------------- */}
					<div className="p-3 bg-gray-800 rounded-lg">
						<h4 className="text-sm font-semibold text-gray-300 flex items-center gap-2">
							<Info className="w-4 h-4 text-blue-400" />
							Workloads On This Pool
						</h4>
						{assignedUnits.length === 0 ? (
							<p className="text-xs text-gray-400 mt-1">
								Nothing is assigned to this pool. Its instances will run and
								bill without carrying any tasks until a service sets
								<span className="font-mono"> runtime: ec2</span> and names it.
							</p>
						) : (
							<ul className="mt-2 space-y-1">
								{assignedUnits.map((name) => (
									<li key={name} className="text-xs font-mono text-gray-300">
										{name}
									</li>
								))}
							</ul>
						)}
					</div>
				</div>
			)}
		</div>
	);
}
