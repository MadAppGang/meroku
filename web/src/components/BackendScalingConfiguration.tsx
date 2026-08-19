import {
	Activity,
	AlertCircle,
	AlertTriangle,
	Cpu,
	Info,
	Server,
	Users,
} from "lucide-react";
import { useCallback, useEffect, useId, useRef, useState } from "react";
import { useFargateOptions } from "../hooks/use-fargate-options";
import type {
	ComputeRuntime,
	YamlInfrastructureConfig,
} from "../types/yamlConfig";
import { Alert, AlertDescription } from "./ui/alert";
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
import { Slider } from "./ui/slider";
import { Switch } from "./ui/switch";

interface BackendScalingConfigurationProps {
	config: YamlInfrastructureConfig;
	onConfigChange: (config: Partial<YamlInfrastructureConfig>) => void;
	isService?: boolean;
	serviceName?: string;
}

export function BackendScalingConfiguration({
	config,
	onConfigChange,
	isService = false,
	serviceName,
}: BackendScalingConfigurationProps) {
	const uid = useId();
	const {
		options: fargateOptions,
		getMemoryOptions,
		formatMemory,
	} = useFargateOptions();

	// Get service config if this is for a service
	const serviceConfig =
		isService && serviceName
			? config.services?.find((s) => s.name === serviceName)
			: null;

	const [cpu, setCpu] = useState(
		isService && serviceConfig
			? serviceConfig.cpu?.toString() || "256"
			: config.workload?.backend_cpu || "256",
	);
	const [memory, setMemory] = useState(
		isService && serviceConfig
			? serviceConfig.memory?.toString() || "512"
			: config.workload?.backend_memory || "512",
	);
	const [desiredCount, setDesiredCount] = useState(
		isService && serviceConfig
			? (serviceConfig.desired_count ?? 1)
			: (config.workload?.backend_desired_count ?? 1),
	);
	const [autoscalingEnabled, setAutoscalingEnabled] = useState(
		isService ? false : config.workload?.backend_autoscaling_enabled || false,
	);
	const [minCapacity, setMinCapacity] = useState(
		config.workload?.backend_autoscaling_min_capacity || 1,
	);
	const [maxCapacity, setMaxCapacity] = useState(
		config.workload?.backend_autoscaling_max_capacity || 10,
	);
	const [cpuTarget, setCpuTarget] = useState(70);
	const [memoryTarget, setMemoryTarget] = useState(80);
	const [requestBasedScaling, setRequestBasedScaling] = useState(false);
	const [requestsPerTarget, setRequestsPerTarget] = useState(1000);

	// Declared above the effect that lists it as a dependency, since dep arrays
	// are evaluated during render.
	const handleWorkloadChange = useCallback(
		(updates: Partial<YamlInfrastructureConfig["workload"]>) => {
			if (isService && serviceName) {
				// Update service configuration
				const updatedServices =
					config.services?.map((service) =>
						service.name === serviceName
							? {
									...service,
									cpu: updates?.backend_cpu
										? Number.parseInt(updates.backend_cpu, 10)
										: service.cpu,
									memory: updates?.backend_memory
										? Number.parseInt(updates.backend_memory, 10)
										: service.memory,
									desired_count:
										updates?.backend_desired_count ?? service.desired_count,
								}
							: service,
					) || [];

				onConfigChange({ services: updatedServices });
			} else {
				// Update backend configuration — send only changed fields.
				// App.tsx deep-merges with prevConfig.workload to avoid stale overwrites.
				onConfigChange({
					workload: updates,
				});
			}
		},
		[isService, serviceName, config.services, onConfigChange],
	);

	// Where this workload runs (schema v26). Absent means Fargate, which is what
	// keeps a pre-v26 config rendering exactly as it did.
	const runtime: ComputeRuntime =
		isService && serviceConfig
			? (serviceConfig.runtime ?? "fargate")
			: (config.workload?.backend_runtime ?? "fargate");
	const computePool =
		isService && serviceConfig
			? (serviceConfig.compute_pool ?? "")
			: (config.workload?.backend_compute_pool ?? "");
	const pools = config.compute?.pools ?? [];

	// The values as they stand in {env}.yaml. Every keystroke rewrites `config`,
	// so "changed away from the saved value" cannot be read back out of it.
	const savedRuntime = useRef(runtime);
	const savedPool = useRef(computePool);

	const handlePlacementChange = useCallback(
		(patch: { runtime?: ComputeRuntime; compute_pool?: string }) => {
			if (isService && serviceName) {
				onConfigChange({
					services: (config.services ?? []).map((service) =>
						service.name === serviceName ? { ...service, ...patch } : service,
					),
				});
				return;
			}
			onConfigChange({
				workload: {
					...(patch.runtime !== undefined
						? { backend_runtime: patch.runtime }
						: {}),
					...("compute_pool" in patch
						? { backend_compute_pool: patch.compute_pool }
						: {}),
				},
			});
		},
		[isService, serviceName, config.services, onConfigChange],
	);

	// Adjust memory when CPU changes - also persist to YAML config.
	//
	// Fargate ONLY. The table this snaps to is the Fargate cpu/memory matrix,
	// and on EC2 it does not apply: a task on an instance may request any
	// combination the instance can hold. Left ungated, this effect silently
	// rewrites a valid EC2 memory value the moment the Fargate table finishes
	// loading, on a save the user never triggered.
	useEffect(() => {
		if (runtime === "ec2") return;

		const availableMemory = getMemoryOptions(Number.parseInt(cpu, 10));
		if (
			availableMemory.length > 0 &&
			!availableMemory.includes(Number.parseInt(memory, 10))
		) {
			const newMemory = availableMemory[0].toString();
			setMemory(newMemory);
			handleWorkloadChange({ backend_memory: newMemory });
		}
	}, [runtime, cpu, memory, getMemoryOptions, handleWorkloadChange]);

	// X-Ray no longer affects resource allocation

	return (
		<div className="space-y-6">
			{/* Compute Placement — where this workload runs (schema v26). */}
			<Card>
				<CardHeader>
					<CardTitle className="flex items-center gap-2">
						<Server className="w-5 h-5 text-blue-400" />
						Compute Placement
					</CardTitle>
					<CardDescription>
						Fargate runs this workload on capacity AWS manages. EC2 runs it on
						instances from a compute pool you define, which is how you choose
						the hardware.
					</CardDescription>
				</CardHeader>
				<CardContent className="space-y-4">
					<div>
						<Label htmlFor={`${uid}-runtime`}>Runtime</Label>
						<Select
							value={runtime}
							onValueChange={(value: string) => {
								const next = value as ComputeRuntime;
								// Moving to EC2 with exactly one pool defined picks it: a
								// runtime of ec2 that names no pool fails the plan with
								// "Error: Invalid index", and there is only one answer.
								const onlyPool =
									next === "ec2" && !computePool && pools.length === 1
										? pools[0].name
										: undefined;
								handlePlacementChange({
									runtime: next,
									...(next === "fargate"
										? { compute_pool: undefined }
										: onlyPool
											? { compute_pool: onlyPool }
											: {}),
								});
							}}
						>
							<SelectTrigger
								id={`${uid}-runtime`}
								className="mt-1 bg-gray-800 border-gray-600"
							>
								<SelectValue />
							</SelectTrigger>
							<SelectContent>
								<SelectItem value="fargate">Fargate (serverless)</SelectItem>
								<SelectItem value="ec2">EC2 (compute pool)</SelectItem>
							</SelectContent>
						</Select>
					</div>

					{runtime === "ec2" && (
						<div className="p-3 bg-gray-800 rounded-lg space-y-2">
							<Label htmlFor={`${uid}-compute-pool`}>Compute Pool</Label>
							{pools.length === 0 ? (
								<p className="text-xs text-gray-400">
									No pool is defined yet. Generation will synthesize one named{" "}
									<span className="font-mono">default</span> and point this
									workload at it — or define your own on the ECS node's Compute
									tab.
								</p>
							) : (
								<>
									<Select
										value={computePool}
										onValueChange={(value: string) =>
											handlePlacementChange({ compute_pool: value })
										}
									>
										<SelectTrigger
											id={`${uid}-compute-pool`}
											className="bg-gray-900 border-gray-600"
										>
											<SelectValue placeholder="Choose a pool" />
										</SelectTrigger>
										<SelectContent>
											{pools.map((pool) => (
												<SelectItem key={pool.name} value={pool.name}>
													{pool.name}
													{pool.enabled === false ? " (disabled)" : ""}
												</SelectItem>
											))}
										</SelectContent>
									</Select>
									<p className="text-xs text-gray-500">
										Edit the pool's instance types, sizing and networking on the
										ECS node's Compute tab.
									</p>
								</>
							)}

							{pools.length > 0 &&
								computePool &&
								!pools.some((pool) => pool.name === computePool) && (
									<Alert className="border-red-600 bg-red-900/20">
										<AlertCircle className="h-4 w-4 text-red-400" />
										<AlertDescription className="text-red-200 text-xs">
											No pool named{" "}
											<span className="font-mono">{computePool}</span> exists.
											Terraform indexes its pool map with this value and the
											plan fails with{" "}
											<span className="font-mono">Error: Invalid index</span>.
										</AlertDescription>
									</Alert>
								)}
						</div>
					)}

					{/* D-2 / §0.1, verbatim. Not "may be recreated": under the pinned
					    hashicorp/aws ~> 5.0 the capacity-provider diff function takes
					    its ForceNew branch and it will. */}
					{runtime !== savedRuntime.current && (
						<Alert className="border-yellow-600 bg-yellow-900/20">
							<AlertTriangle className="h-4 w-4 text-yellow-400" />
							<AlertDescription className="text-yellow-200 text-xs">
								Changing the runtime of a service that is already deployed will
								recreate the ECS service, with downtime. Terraform destroys the
								old service and creates a new one; the service name is unique
								per cluster, so there is no create-before-destroy path. Plan
								this for a maintenance window.
							</AlertDescription>
						</Alert>
					)}

					{runtime === savedRuntime.current &&
						computePool !== savedPool.current && (
							<Alert className="border-yellow-600 bg-yellow-900/20">
								<AlertTriangle className="h-4 w-4 text-yellow-400" />
								<AlertDescription className="text-yellow-200 text-xs">
									Reassigning a deployed workload to another pool changes its
									capacity provider strategy. Terraform moves the service with a
									rolling deployment and every task is replaced; a service that
									predates this feature is recreated instead, with downtime.
									Plan this for a maintenance window.
								</AlertDescription>
							</Alert>
						)}
				</CardContent>
			</Card>

			{/* Resource Configuration */}
			<Card>
				<CardHeader>
					<CardTitle className="flex items-center gap-2">
						<Cpu className="w-5 h-5 text-blue-400" />
						Resource Configuration
					</CardTitle>
					<CardDescription>
						Configure CPU and memory resources for each instance
					</CardDescription>
				</CardHeader>
				<CardContent className="space-y-4">
					{/* On EC2 the Fargate cpu/memory matrix does not apply: a task may
					    request any combination the instance can hold, so these are free
					    numbers rather than a menu of eleven legal pairs. */}
					{runtime === "ec2" ? (
						<div className="grid grid-cols-2 gap-4">
							<div>
								<Label htmlFor={`${uid}-cpu-units`}>CPU Units</Label>
								<Input
									id={`${uid}-cpu-units`}
									type="number"
									min="0"
									step="128"
									value={cpu}
									onChange={(e: React.ChangeEvent<HTMLInputElement>) => {
										const value = e.target.value;
										setCpu(value);
										handleWorkloadChange({ backend_cpu: value });
									}}
									className="mt-1 bg-gray-800 border-gray-600 text-white"
								/>
								<p className="text-xs text-gray-500 mt-1">
									1024 units is one vCPU. 0 leaves the task unbounded on the
									instance's CPU.
								</p>
							</div>

							<div>
								<Label htmlFor={`${uid}-memory`}>Memory (MB)</Label>
								<Input
									id={`${uid}-memory`}
									type="number"
									min="0"
									step="128"
									value={memory}
									onChange={(e: React.ChangeEvent<HTMLInputElement>) => {
										const value = e.target.value;
										setMemory(value);
										handleWorkloadChange({ backend_memory: value });
									}}
									className="mt-1 bg-gray-800 border-gray-600 text-white"
								/>
								<p className="text-xs text-gray-500 mt-1">
									A hard limit. No instance in the pool may be smaller than this
									or the task never places.
								</p>
							</div>
						</div>
					) : (
						<div className="grid grid-cols-2 gap-4">
							<div>
								<Label htmlFor={`${uid}-cpu-units`}>CPU Units</Label>
								<Select
									value={cpu}
									onValueChange={(value: string) => {
										setCpu(value);
										// Auto-adjust memory if current value is invalid for new CPU
										const availableMemory = getMemoryOptions(
											Number.parseInt(value, 10),
										);
										const currentMem = Number.parseInt(memory, 10);
										if (
											availableMemory.length > 0 &&
											!availableMemory.includes(currentMem)
										) {
											const newMemory = availableMemory[0].toString();
											setMemory(newMemory);
											handleWorkloadChange({
												backend_cpu: value,
												backend_memory: newMemory,
											});
										} else {
											handleWorkloadChange({ backend_cpu: value });
										}
									}}
								>
									<SelectTrigger
										id={`${uid}-cpu-units`}
										className="mt-1 bg-gray-800 border-gray-600"
									>
										<SelectValue />
									</SelectTrigger>
									<SelectContent>
										{fargateOptions.map((opt) => (
											<SelectItem key={opt.cpu} value={opt.cpu.toString()}>
												{opt.cpu} ({opt.vcpu} vCPU)
											</SelectItem>
										))}
									</SelectContent>
								</Select>
							</div>

							<div>
								<Label htmlFor={`${uid}-memory`}>Memory (MB)</Label>
								<Select
									value={memory}
									onValueChange={(value: string) => {
										setMemory(value);
										handleWorkloadChange({ backend_memory: value });
									}}
								>
									<SelectTrigger
										id={`${uid}-memory`}
										className="mt-1 bg-gray-800 border-gray-600"
									>
										<SelectValue />
									</SelectTrigger>
									<SelectContent>
										{getMemoryOptions(Number.parseInt(cpu, 10)).map((mem) => (
											<SelectItem key={mem} value={mem.toString()}>
												{formatMemory(mem)}
											</SelectItem>
										))}
									</SelectContent>
								</Select>
							</div>
						</div>
					)}

					<div>
						<Label htmlFor={`${uid}-base-count`}>Base Instance Count</Label>
						<Input
							id={`${uid}-base-count`}
							type="number"
							min="1"
							max="100"
							value={desiredCount}
							onChange={(e: React.ChangeEvent<HTMLInputElement>) => {
								const value = Number.parseInt(e.target.value, 10) || 1;
								setDesiredCount(value);
								handleWorkloadChange({ backend_desired_count: value });
							}}
							className="mt-1 bg-gray-800 border-gray-600 text-white"
						/>
						<p className="text-xs text-gray-500 mt-1">
							Number of instances to run when autoscaling is disabled
						</p>
					</div>

					<div className="bg-gray-800 rounded-lg p-4">
						<h4 className="text-sm font-medium text-gray-300 mb-2">
							Resource Guidelines
						</h4>
						<div className="space-y-2 text-xs text-gray-400">
							<p>
								• <strong>256 CPU:</strong> Light workloads, simple APIs
							</p>
							<p>
								• <strong>512 CPU:</strong> Standard web applications
							</p>
							<p>
								• <strong>1024 CPU:</strong> CPU-intensive processing
							</p>
							<p>
								• <strong>2048+ CPU:</strong> High-performance applications
							</p>
						</div>
					</div>
				</CardContent>
			</Card>

			{/* Autoscaling Configuration - Only show for backend, not services */}
			{!isService && (
				<Card>
					<CardHeader>
						<CardTitle className="flex items-center gap-2">
							<Activity className="w-5 h-5 text-green-400" />
							Autoscaling Configuration
						</CardTitle>
						<CardDescription>
							Configure automatic scaling based on metrics
						</CardDescription>
					</CardHeader>
					<CardContent className="space-y-4">
						<div className="flex items-center justify-between">
							<div className="flex-1">
								<Label htmlFor={`${uid}-enable-autoscaling`}>
									Enable Autoscaling
								</Label>
								<p className="text-xs text-gray-500 mt-1">
									Automatically adjust the number of instances based on load
								</p>
							</div>
							<Switch
								id={`${uid}-enable-autoscaling`}
								checked={autoscalingEnabled}
								onCheckedChange={(checked) => {
									setAutoscalingEnabled(checked);
									handleWorkloadChange({
										backend_autoscaling_enabled: checked,
									});
								}}
								className="data-[state=checked]:bg-green-500 data-[state=unchecked]:bg-gray-600"
							/>
						</div>

						{autoscalingEnabled && (
							<>
								<div className="grid grid-cols-2 gap-4">
									<div>
										<Label htmlFor={`${uid}-min-instances`}>
											Minimum Instances
										</Label>
										<Input
											id={`${uid}-min-instances`}
											type="number"
											min="1"
											max={maxCapacity}
											value={minCapacity}
											onChange={(e: React.ChangeEvent<HTMLInputElement>) => {
												const value = Number.parseInt(e.target.value, 10) || 1;
												setMinCapacity(value);
												handleWorkloadChange({
													backend_autoscaling_min_capacity: value,
												});
											}}
											className="mt-1 bg-gray-800 border-gray-600 text-white"
										/>
									</div>

									<div>
										<Label htmlFor={`${uid}-max-instances`}>
											Maximum Instances
										</Label>
										<Input
											id={`${uid}-max-instances`}
											type="number"
											min={minCapacity}
											max="100"
											value={maxCapacity}
											onChange={(e: React.ChangeEvent<HTMLInputElement>) => {
												const value = Number.parseInt(e.target.value, 10) || 1;
												setMaxCapacity(value);
												handleWorkloadChange({
													backend_autoscaling_max_capacity: value,
												});
											}}
											className="mt-1 bg-gray-800 border-gray-600 text-white"
										/>
									</div>
								</div>

								<div className="space-y-4">
									<h4 className="text-sm font-medium text-gray-300">
										Scaling Triggers
									</h4>

									<div>
										<div className="flex items-center justify-between mb-2">
											<Label>CPU Target</Label>
											<span className="text-sm text-gray-400">
												{cpuTarget}%
											</span>
										</div>
										<Slider
											value={[cpuTarget]}
											onValueChange={([value]: number[]) => {
												setCpuTarget(value);
												// Note: cpu_target is not persisted in config
											}}
											min={0}
											max={100}
											step={5}
											className="mt-1"
										/>
										<p className="text-xs text-gray-500 mt-1">
											Scale up when average CPU utilization exceeds this
											threshold
										</p>
									</div>

									<div>
										<div className="flex items-center justify-between mb-2">
											<Label>Memory Target</Label>
											<span className="text-sm text-gray-400">
												{memoryTarget}%
											</span>
										</div>
										<Slider
											value={[memoryTarget]}
											onValueChange={([value]: number[]) => {
												setMemoryTarget(value);
												// Note: memory_target is not persisted in config
											}}
											min={0}
											max={100}
											step={5}
											className="mt-1"
										/>
										<p className="text-xs text-gray-500 mt-1">
											Scale up when average memory utilization exceeds this
											threshold
										</p>
									</div>

									<div className="space-y-2">
										<div className="flex items-center gap-2">
											<Checkbox
												id={`${uid}-request-scaling`}
												checked={requestBasedScaling}
												onCheckedChange={(checked) => {
													setRequestBasedScaling(checked as boolean);
													// Note: request_based is not persisted in config
												}}
											/>
											<Label
												htmlFor={`${uid}-request-scaling`}
												className="text-sm font-normal cursor-pointer"
											>
												Enable Request-based Scaling (requires ALB)
											</Label>
										</div>

										{requestBasedScaling && (
											<div className="ml-6">
												<Label htmlFor={`${uid}-requests-per-instance`}>
													Requests per Instance
												</Label>
												<Input
													id={`${uid}-requests-per-instance`}
													type="number"
													min="100"
													max="10000"
													value={requestsPerTarget}
													onChange={(
														e: React.ChangeEvent<HTMLInputElement>,
													) => {
														const value =
															Number.parseInt(e.target.value, 10) || 1000;
														setRequestsPerTarget(value);
														// Note: requests_per_target is not persisted in config
													}}
													className="mt-1 bg-gray-800 border-gray-600 text-white"
												/>
												<p className="text-xs text-gray-500 mt-1">
													Target number of requests each instance should handle
												</p>
											</div>
										)}
									</div>
								</div>

								<div className="bg-green-900/20 border border-green-700 rounded-lg p-4">
									<div className="flex items-start gap-2">
										<Info className="w-4 h-4 text-green-400 mt-0.5" />
										<div className="flex-1">
											<h4 className="text-sm font-medium text-green-400 mb-1">
												Scaling Behavior
											</h4>
											<p className="text-xs text-gray-300">
												The service will scale between {minCapacity} and{" "}
												{maxCapacity} instances based on the configured metrics.
												Scaling decisions are made every 60 seconds.
											</p>
										</div>
									</div>
								</div>
							</>
						)}

						{!autoscalingEnabled && (
							<div className="bg-yellow-900/20 border border-yellow-700 rounded-lg p-4">
								<div className="flex items-start gap-2">
									<AlertCircle className="w-4 h-4 text-yellow-400 mt-0.5" />
									<div className="flex-1">
										<h4 className="text-sm font-medium text-yellow-400 mb-1">
											Manual Scaling
										</h4>
										<p className="text-xs text-gray-300">
											With autoscaling disabled, your service will always run
											exactly {desiredCount} instance
											{desiredCount > 1 ? "s" : ""}. You'll need to manually
											adjust this value to handle load changes.
										</p>
									</div>
								</div>
							</div>
						)}
					</CardContent>
				</Card>
			)}

			{/* Resource Summary */}
			<Card>
				<CardHeader>
					<CardTitle className="flex items-center gap-2">
						<Users className="w-5 h-5 text-purple-400" />
						Resource Summary
					</CardTitle>
				</CardHeader>
				<CardContent>
					<div className="space-y-3">
						<div className="grid grid-cols-2 gap-4 text-sm">
							<div className="bg-gray-800 rounded-lg p-3">
								<p className="text-xs text-gray-400 mb-1">Per Instance</p>
								<p className="text-gray-300">
									{cpu} CPU / {memory} MB
								</p>
							</div>

							<div className="bg-gray-800 rounded-lg p-3">
								<p className="text-xs text-gray-400 mb-1">Instance Range</p>
								<p className="text-gray-300">
									{autoscalingEnabled
										? `${minCapacity} - ${maxCapacity}`
										: desiredCount}{" "}
									instances
								</p>
							</div>
						</div>

						<div className="bg-gray-800 rounded-lg p-3">
							<p className="text-xs text-gray-400 mb-1">Maximum Resources</p>
							<p className="text-sm text-gray-300">
								{Number.parseInt(cpu, 10) *
									(autoscalingEnabled ? maxCapacity : desiredCount)}{" "}
								CPU units /{" "}
								{(
									(Number.parseInt(memory, 10) *
										(autoscalingEnabled ? maxCapacity : desiredCount)) /
									1024
								).toFixed(1)}{" "}
								GB memory
							</p>
						</div>

						<div className="text-xs text-gray-500">
							<p>• Fargate pricing is based on CPU and memory allocated</p>
							<p>• You only pay for what you use (per-second billing)</p>
							<p>
								• Data transfer and other AWS services incur additional charges
							</p>
						</div>
					</div>
				</CardContent>
			</Card>
		</div>
	);
}
