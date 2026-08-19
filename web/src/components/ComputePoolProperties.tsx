import {
	AlertCircle,
	CloudOff,
	Cpu,
	Info,
	Plus,
	Server,
	Sparkles,
	Wand2,
} from "lucide-react";
import { useCallback, useMemo, useState } from "react";
import type { ComputeInstanceTypesResponse } from "../api/infrastructure";
import { useInstanceTypes } from "../hooks/use-instance-types";
import type {
	ComputePool,
	YamlInfrastructureConfig,
} from "../types/yamlConfig";
import { ComputePoolEditor } from "./ComputePoolEditor";
import { Alert, AlertDescription } from "./ui/alert";
import { Button } from "./ui/button";
import {
	Card,
	CardContent,
	CardDescription,
	CardHeader,
	CardTitle,
} from "./ui/card";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "./ui/dialog";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "./ui/select";

interface ComputePoolPropertiesProps {
	config: YamlInfrastructureConfig;
	onConfigChange: (config: Partial<YamlInfrastructureConfig>) => void;
}

/**
 * `synthesizeDefaultComputePool`'s pool, mirrored (§5.5 / FR-28).
 *
 * Deliberately `on_demand` rather than spot and `bridge` rather than awsvpc: a
 * pool a user did not write is not the place to opt them into spot interruption,
 * and it is certainly not the place to opt them into a network mode with no
 * egress path (DEV-13, D-6).
 */
const SYNTHESIZED_DEFAULT_POOL: ComputePool = {
	name: "default",
	enabled: true,
	instance_types: ["m7i-flex.large", "m6i.large", "m6a.large"],
	capacity_type: "on_demand",
	min_size: 1,
	max_size: 6,
	target_capacity: 100,
	network_mode: "bridge",
	ami_family: "al2023",
	root_volume_gb: 30,
};

/** A blank pool. Sized like the synthesized one so "create" never means "broken". */
function blankPool(name: string): ComputePool {
	return { ...SYNTHESIZED_DEFAULT_POOL, name };
}

/** Instance-hours, not summed task cpu/memory: one instance is billed once (FR-56). */
const HOURS_PER_MONTH = 730;

/**
 * The server's `notice` is one sentence with no trailing stop, and it is
 * followed here by a second sentence. Without this the two run together.
 */
function withFullStop(sentence: string): string {
	return /[.!?]$/.test(sentence.trim()) ? sentence : `${sentence.trim()}.`;
}

/** Said in our own words when the server sent no notice of its own. */
function degradedFallback(catalog: ComputeInstanceTypesResponse): string {
	switch (catalog.credentialsState) {
		case "missing":
			return "No AWS credentials were found, so instance types and prices come from built-in data";
		case "expired":
			return "Your AWS credentials have expired, so instance types and prices come from built-in data";
		case "denied":
			return "This role cannot read EC2 or Pricing, so instance types and prices come from built-in data";
		default:
			return "Instance types and prices come from built-in data";
	}
}

function uniquePoolName(existing: readonly ComputePool[]): string {
	const taken = new Set(existing.map((p) => p.name));
	if (!taken.has("default")) return "default";
	for (let n = 2; n < 100; n++) {
		const candidate = `pool-${n}`;
		if (!taken.has(candidate)) return candidate;
	}
	return `pool-${Date.now()}`;
}

export function ComputePoolProperties({
	config,
	onConfigChange,
}: ComputePoolPropertiesProps) {
	const pools = useMemo(() => config.compute?.pools ?? [], [config.compute]);
	const [pendingDelete, setPendingDelete] = useState<number | null>(null);
	const [reassignTo, setReassignTo] = useState<string>("");

	// FR-45: this hook's failure must never remove the surface. Pool CRUD is a
	// YAML write and is not gated on any AWS response — the catalog is read only
	// to price the list and to fill the picker.
	const { data: catalog } = useInstanceTypes(config.env);

	// -----------------------------------------------------------------------
	// THE single owner of the pools array (architecture.md §8.4, C-3).
	//
	// `mergeConfigUpdates` merges one level deep, so `compute.pools` is replaced
	// wholesale by whatever is emitted here. Every write below maps over the FULL
	// current array. No other component in this codebase may construct one:
	// `ComputePoolEditor` takes `(index, patch)` and never sees the array, which
	// makes a partial emit a compile error rather than a convention.
	// -----------------------------------------------------------------------
	const writePools = useCallback(
		(next: ComputePool[]) => {
			onConfigChange({ compute: { ...config.compute, pools: next } });
		},
		[config.compute, onConfigChange],
	);

	const handlePoolChange = useCallback(
		(index: number, patch: Partial<ComputePool>) => {
			const current = config.compute?.pools ?? [];
			writePools(current.map((p, i) => (i === index ? { ...p, ...patch } : p)));
		},
		[config.compute, writePools],
	);

	const handleAddPool = useCallback(() => {
		const current = config.compute?.pools ?? [];
		writePools([...current, blankPool(uniquePoolName(current))]);
	}, [config.compute, writePools]);

	const handleWriteSynthesizedDefault = useCallback(() => {
		writePools([SYNTHESIZED_DEFAULT_POOL]);
	}, [writePools]);

	// Which workloads name a given pool. Both the delete guard (FR-36) and the
	// per-pool summary read this.
	const referencesFor = useCallback(
		(poolName: string) => {
			const names: string[] = [];
			if (
				config.workload?.backend_runtime === "ec2" &&
				config.workload?.backend_compute_pool === poolName
			) {
				names.push("backend");
			}
			for (const service of config.services ?? []) {
				if (service.runtime === "ec2" && service.compute_pool === poolName) {
					names.push(service.name);
				}
			}
			return names;
		},
		[config.workload, config.services],
	);

	const ec2Units = useMemo(() => {
		const names: string[] = [];
		if (config.workload?.backend_runtime === "ec2") names.push("backend");
		for (const service of config.services ?? []) {
			if (service.runtime === "ec2") names.push(service.name);
		}
		return names;
	}, [config.workload, config.services]);

	const handleDeleteRequest = useCallback((index: number) => {
		setReassignTo("");
		setPendingDelete(index);
	}, []);

	const pendingPool = pendingDelete === null ? null : pools[pendingDelete];
	const pendingRefs = pendingPool ? referencesFor(pendingPool.name) : [];
	const reassignTargets = pendingPool
		? pools.filter((p) => p.name !== pendingPool.name && p.enabled !== false)
		: [];

	/**
	 * Deleting a pool that a workload still names is prohibited by every path
	 * (FR-36), so the delete and the resolution are ONE config change: reassign
	 * the workloads or move them to Fargate, and drop the pool, together. Two
	 * changes would leave a window where the YAML on disk names a pool that does
	 * not exist, and every save here writes the whole file.
	 */
	const commitDelete = useCallback(
		(resolution: "reassign" | "fargate" | "none") => {
			if (pendingDelete === null) return;
			const current = config.compute?.pools ?? [];
			const doomed = current[pendingDelete];
			if (!doomed) return;

			const updates: Partial<YamlInfrastructureConfig> = {
				compute: {
					...config.compute,
					pools: current.filter((_, i) => i !== pendingDelete),
				},
			};

			if (resolution !== "none") {
				const toFargate = resolution === "fargate";

				if (
					config.workload?.backend_runtime === "ec2" &&
					config.workload?.backend_compute_pool === doomed.name
				) {
					updates.workload = toFargate
						? {
								backend_runtime: "fargate",
								backend_compute_pool: undefined,
							}
						: { backend_compute_pool: reassignTo };
				}

				const services = config.services ?? [];
				if (
					services.some(
						(s) => s.runtime === "ec2" && s.compute_pool === doomed.name,
					)
				) {
					updates.services = services.map((service) =>
						service.runtime === "ec2" && service.compute_pool === doomed.name
							? toFargate
								? {
										...service,
										runtime: "fargate" as const,
										compute_pool: undefined,
									}
								: { ...service, compute_pool: reassignTo }
							: service,
					);
				}
			}

			onConfigChange(updates);
			setPendingDelete(null);
			setReassignTo("");
		},
		[pendingDelete, config, reassignTo, onConfigChange],
	);

	/**
	 * The pool's floor cost: instances × hourly × hours. Instance-hours, never
	 * summed task cpu/memory — one instance is billed once no matter how many
	 * tasks land on it (FR-56). Null when the catalog has no price for the first
	 * listed type, which renders as an absence rather than a zero.
	 */
	const monthlyFloor = useCallback(
		(pool: ComputePool): number | null => {
			const first = pool.instance_types?.[0];
			if (!first || !catalog) return null;
			const info = catalog.instanceTypes.find((t) => t.instanceType === first);
			if (!info || info.onDemandHourly === null) return null;
			return (pool.min_size ?? 1) * info.onDemandHourly * HOURS_PER_MONTH;
		},
		[catalog],
	);

	const degraded =
		catalog !== null &&
		(catalog.source !== "aws_api" || catalog.credentialsState !== "ok");

	return (
		<div className="space-y-6">
			<Card>
				<CardHeader>
					<CardTitle className="flex items-center gap-2">
						<Cpu className="w-5 h-5 text-blue-400" />
						Compute Pools
					</CardTitle>
					<CardDescription>
						EC2 capacity for this cluster. A pool is one Auto Scaling group
						fronted by an ECS capacity provider; services with{" "}
						<span className="font-mono">runtime: ec2</span> are placed on the
						pool they name. With no pools, this cluster is Fargate-only and no
						EC2 resource is created at all.
					</CardDescription>
				</CardHeader>

				<CardContent className="space-y-4">
					{/* Degradation, stated before anything priced by it. */}
					{degraded && catalog && (
						<Alert className="border-yellow-600 bg-yellow-900/20">
							<CloudOff className="h-4 w-4 text-yellow-400" />
							<AlertDescription className="text-yellow-200 text-xs space-y-1">
								{/* The server's notice is one sentence with no trailing stop,
								    so it gets its own paragraph rather than being run into
								    the reassurance that follows it. */}
								<p>
									{withFullStop(catalog.notice ?? degradedFallback(catalog))}
								</p>
								<p>
									Editing pools does not need AWS — every field below still
									writes to {config.env}.yaml.
								</p>
							</AlertDescription>
						</Alert>
					)}

					{/* §5.5 — the zero-config path, and the one-click way out of it. */}
					{pools.length === 0 && ec2Units.length > 0 && (
						<Alert className="border-blue-600 bg-blue-900/20">
							<Sparkles className="h-4 w-4 text-blue-400" />
							<AlertDescription className="text-blue-200 text-xs space-y-2">
								<p>
									{ec2Units.join(", ")} {ec2Units.length === 1 ? "runs" : "run"}{" "}
									on EC2 but this environment defines no pool. Generation will
									synthesize one named{" "}
									<span className="font-mono">default</span> —{" "}
									{SYNTHESIZED_DEFAULT_POOL.instance_types?.join(", ")},
									on-demand, bridge networking,{" "}
									{SYNTHESIZED_DEFAULT_POOL.min_size}–
									{SYNTHESIZED_DEFAULT_POOL.max_size} instances — and point them
									at it.
								</p>
								<p>
									A synthesized pool never appears in{" "}
									<span className="font-mono">git diff</span>. Write it down to
									see it and to change it.
								</p>
								<Button
									type="button"
									size="sm"
									onClick={handleWriteSynthesizedDefault}
									className="mt-1"
								>
									<Wand2 className="w-3.5 h-3.5 mr-1.5" />
									Write the default pool into {config.env}.yaml
								</Button>
							</AlertDescription>
						</Alert>
					)}

					{/* Workloads pointing at a pool that no longer exists. Generate
					    refuses this; saying so here is cheaper than finding out then. */}
					{pools.length > 0 &&
						ec2Units.filter((unit) => {
							const poolName =
								unit === "backend"
									? config.workload?.backend_compute_pool
									: config.services?.find((s) => s.name === unit)?.compute_pool;
							return !poolName || !pools.some((p) => p.name === poolName);
						}).length > 0 && (
							<Alert className="border-red-600 bg-red-900/20">
								<AlertCircle className="h-4 w-4 text-red-400" />
								<AlertDescription className="text-red-200 text-xs">
									{ec2Units
										.filter((unit) => {
											const poolName =
												unit === "backend"
													? config.workload?.backend_compute_pool
													: config.services?.find((s) => s.name === unit)
															?.compute_pool;
											return (
												!poolName || !pools.some((p) => p.name === poolName)
											);
										})
										.join(", ")}{" "}
									runs on EC2 but names no pool that exists here. Terraform
									indexes its pool map with that name and the plan fails with{" "}
									<span className="font-mono">Error: Invalid index</span>,
									pointing at an expression that is in no file you wrote.
								</AlertDescription>
							</Alert>
						)}

					{pools.length === 0 ? (
						<div className="text-center py-8 px-4 rounded-lg border border-dashed border-gray-700">
							<Server className="w-8 h-8 mx-auto text-gray-600" />
							<p className="text-sm text-gray-300 mt-3">
								No compute pools — this cluster runs on Fargate alone.
							</p>
							<p className="text-xs text-gray-500 mt-1 max-w-md mx-auto">
								Add a pool when you want to pick the hardware: memory-dense
								instances for a cache, Graviton for cost, GPUs for inference, or
								spot for anything that tolerates being interrupted.
							</p>
							<Button
								type="button"
								onClick={handleAddPool}
								className="mt-4"
								size="sm"
							>
								<Plus className="w-4 h-4 mr-1.5" />
								Add a pool
							</Button>
						</div>
					) : (
						<div className="space-y-3">
							{pools.map((pool, index) => (
								<div
									// Keyed by position, not by name: keying by name would
									// remount the editor on every keystroke of a rename and
									// lose both focus and the saved-value refs it compares
									// against.
									// biome-ignore lint/suspicious/noArrayIndexKey: position is the stable identity here; see above
									key={index}
									className="space-y-1"
								>
									<ComputePoolEditor
										pool={pool}
										index={index}
										allPools={pools}
										config={config}
										onChange={handlePoolChange}
										onDelete={handleDeleteRequest}
									/>
									{monthlyFloor(pool) !== null && (
										<p className="text-xs text-gray-500 px-3">
											~${(monthlyFloor(pool) ?? 0).toFixed(2)}/month at the
											floor of {pool.min_size ?? 1} instance
											{(pool.min_size ?? 1) === 1 ? "" : "s"}
											{catalog?.pricingRegion
												? ` (indicative ${catalog.pricingRegion} pricing)`
												: ""}
											. Tasks placed on it cost nothing further.
										</p>
									)}
								</div>
							))}

							<Button
								type="button"
								variant="outline"
								onClick={handleAddPool}
								size="sm"
							>
								<Plus className="w-4 h-4 mr-1.5" />
								Add another pool
							</Button>
						</div>
					)}

					<Alert>
						<Info className="h-4 w-4" />
						<AlertDescription className="text-xs">
							Pools are written straight to {config.env}.yaml. They create no
							AWS resource until{" "}
							<span className="font-mono">task infra-apply</span> runs, and an
							environment with no pools produces no EC2 plan diff at all.
						</AlertDescription>
					</Alert>
				</CardContent>
			</Card>

			{/* FR-36 — deleting a referenced pool is prohibited by every path. */}
			<Dialog
				open={pendingDelete !== null}
				onOpenChange={(open) => {
					if (!open) setPendingDelete(null);
				}}
			>
				<DialogContent>
					<DialogHeader>
						<DialogTitle>
							Delete pool {pendingPool?.name ? `"${pendingPool.name}"` : ""}?
						</DialogTitle>
						<DialogDescription>
							{pendingRefs.length === 0
								? "Nothing is placed on this pool, so removing it changes no running service."
								: `${pendingRefs.join(", ")} ${pendingRefs.length === 1 ? "is" : "are"} placed on this pool. Deleting it without moving them fails the plan with "Error: Invalid index", so choose where they go.`}
						</DialogDescription>
					</DialogHeader>

					{pendingRefs.length > 0 && (
						<div className="space-y-3">
							{reassignTargets.length > 0 && (
								<div className="space-y-2 p-3 bg-gray-800 rounded-lg">
									<p className="text-sm text-gray-200">
										Move them to another pool
									</p>
									<Select value={reassignTo} onValueChange={setReassignTo}>
										<SelectTrigger className="bg-gray-900 border-gray-600">
											<SelectValue placeholder="Choose a pool" />
										</SelectTrigger>
										<SelectContent>
											{reassignTargets.map((target) => (
												<SelectItem key={target.name} value={target.name}>
													{target.name}
												</SelectItem>
											))}
										</SelectContent>
									</Select>
									<p className="text-xs text-gray-500">
										The services keep{" "}
										<span className="font-mono">runtime: ec2</span> and move to
										the chosen pool's capacity provider. Terraform updates each
										service in place with a rolling deployment; every task is
										replaced.
									</p>
								</div>
							)}

							<div className="space-y-2 p-3 bg-gray-800 rounded-lg">
								<p className="text-sm text-gray-200">
									{reassignTargets.length > 0
										? "Or move them back to Fargate"
										: "Move them back to Fargate"}
								</p>
								<p className="text-xs text-gray-500">
									Changing the runtime of a service that is already deployed
									will recreate the ECS service, with downtime. Terraform
									destroys the old service and creates a new one; the service
									name is unique per cluster, so there is no
									create-before-destroy path. Plan this for a maintenance
									window.
								</p>
							</div>
						</div>
					)}

					<DialogFooter>
						<Button
							type="button"
							variant="ghost"
							onClick={() => setPendingDelete(null)}
						>
							Cancel
						</Button>
						{pendingRefs.length === 0 ? (
							<Button
								type="button"
								variant="destructive"
								onClick={() => commitDelete("none")}
							>
								Delete pool
							</Button>
						) : (
							<>
								{reassignTargets.length > 0 && (
									<Button
										type="button"
										disabled={!reassignTo}
										onClick={() => commitDelete("reassign")}
									>
										Reassign and delete
									</Button>
								)}
								<Button
									type="button"
									variant="destructive"
									onClick={() => commitDelete("fargate")}
								>
									Switch to Fargate and delete
								</Button>
							</>
						)}
					</DialogFooter>
				</DialogContent>
			</Dialog>
		</div>
	);
}
