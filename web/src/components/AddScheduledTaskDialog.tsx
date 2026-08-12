import type React from "react";
import { useId, useState } from "react";
import { useFargateOptions } from "../hooks/use-fargate-options";
import type { ScheduledTask } from "../types/components";
import { parseContainerCommand } from "../utils/taskSettings";
import { Button } from "./ui/button";
import {
	Dialog,
	DialogContent,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "./ui/dialog";
import { Input } from "./ui/input";
import { Label } from "./ui/label";
import { RadioGroup, RadioGroupItem } from "./ui/radio-group";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "./ui/select";
import { Textarea } from "./ui/textarea";

interface AddScheduledTaskDialogProps {
	open: boolean;
	onClose: () => void;
	onAdd: (task: ScheduledTask) => void;
	existingTasks: string[];
}

export function AddScheduledTaskDialog({
	open,
	onClose,
	onAdd,
	existingTasks,
}: AddScheduledTaskDialogProps) {
	const uid = useId();
	const {
		options: fargateOptions,
		getMemoryOptions,
		formatMemory,
	} = useFargateOptions();
	const [formData, setFormData] = useState({
		name: "",
		schedule_type: "rate",
		rate_value: "1",
		rate_unit: "hours",
		cron_expression: "",
		docker_image: "",
		container_command: "",
		cpu: 256,
		memory: 512,
		environment_variables: "",
	});

	const [errors, setErrors] = useState<Record<string, string>>({});

	const getScheduleExpression = () => {
		if (formData.schedule_type === "rate") {
			return `rate(${formData.rate_value} ${formData.rate_unit})`;
		}
		return `cron(${formData.cron_expression})`;
	};

	const handleSubmit = (e: React.FormEvent) => {
		e.preventDefault();

		const newErrors: Record<string, string> = {};

		if (!formData.name) {
			newErrors.name = "Task name is required";
		} else if (!/^[a-z0-9-]+$/.test(formData.name)) {
			newErrors.name =
				"Task name must contain only lowercase letters, numbers, and hyphens";
		} else if (existingTasks.includes(formData.name)) {
			newErrors.name = "A scheduled task with this name already exists";
		}

		if (formData.schedule_type === "cron" && !formData.cron_expression) {
			newErrors.cron_expression = "Cron expression is required";
		}

		if (Object.keys(newErrors).length > 0) {
			setErrors(newErrors);
			return;
		}

		const task: ScheduledTask = {
			name: formData.name,
			schedule: getScheduleExpression(),
			cpu: formData.cpu,
			memory: formData.memory,
		};

		if (formData.docker_image) {
			task.docker_image = formData.docker_image;
		}

		// container_command is list(string) as of schema v25. The editor takes a
		// comma-separated line, so it has to be parsed into an array here — a
		// bare string reaches the Go loader as a scalar and is only rescued by
		// the migration on the next load.
		const containerCommand = parseContainerCommand(formData.container_command);
		if (containerCommand) {
			task.container_command = containerCommand;
		}

		if (formData.environment_variables) {
			try {
				const envVars = formData.environment_variables
					.split("\n")
					.map((line) => line.trim())
					.filter((line) => line?.includes("="))
					.reduce(
						(acc, line) => {
							const [key, ...valueParts] = line.split("=");
							acc[key.trim()] = valueParts.join("=").trim();
							return acc;
						},
						{} as Record<string, string>,
					);

				if (Object.keys(envVars).length > 0) {
					task.environment_variables = envVars;
				}
			} catch (_error) {
				newErrors.environment_variables =
					"Invalid environment variables format";
				setErrors(newErrors);
				return;
			}
		}

		onAdd(task);
		handleClose();
	};

	const handleClose = () => {
		setFormData({
			name: "",
			schedule_type: "rate",
			rate_value: "1",
			rate_unit: "hours",
			cron_expression: "",
			docker_image: "",
			container_command: "",
			cpu: 256,
			memory: 512,
			environment_variables: "",
		});
		setErrors({});
		onClose();
	};

	return (
		<Dialog open={open} onOpenChange={handleClose}>
			<DialogContent className="sm:max-w-[600px]">
				<DialogHeader>
					<DialogTitle>Add Scheduled Task</DialogTitle>
				</DialogHeader>
				<form onSubmit={handleSubmit}>
					<div className="grid gap-4 py-4">
						<div className="grid gap-2">
							<Label htmlFor={`${uid}-name`}>Task Name</Label>
							<Input
								id={`${uid}-name`}
								value={formData.name}
								onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
									setFormData({ ...formData, name: e.target.value })
								}
								placeholder="daily-cleanup"
							/>
							{errors.name && (
								<p className="text-sm text-red-500">{errors.name}</p>
							)}
						</div>

						<div className="grid gap-2">
							<Label>Schedule Type</Label>
							<RadioGroup
								value={formData.schedule_type}
								onValueChange={(value: string) =>
									setFormData({ ...formData, schedule_type: value })
								}
							>
								<div className="flex items-center space-x-2">
									<RadioGroupItem value="rate" id={`${uid}-rate`} />
									<Label htmlFor={`${uid}-rate`}>
										Rate-based (e.g., every X hours)
									</Label>
								</div>
								<div className="flex items-center space-x-2">
									<RadioGroupItem value="cron" id={`${uid}-cron`} />
									<Label htmlFor={`${uid}-cron`}>Cron expression</Label>
								</div>
							</RadioGroup>
						</div>

						{formData.schedule_type === "rate" ? (
							<div className="grid grid-cols-2 gap-4">
								<div className="grid gap-2">
									<Label htmlFor={`${uid}-rate_value`}>Every</Label>
									<Input
										id={`${uid}-rate_value`}
										type="number"
										value={formData.rate_value}
										onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
											setFormData({ ...formData, rate_value: e.target.value })
										}
										min="1"
									/>
								</div>
								<div className="grid gap-2">
									<Label htmlFor={`${uid}-rate_unit`}>Unit</Label>
									<Select
										value={formData.rate_unit}
										onValueChange={(value: string) =>
											setFormData({ ...formData, rate_unit: value })
										}
									>
										<SelectTrigger id={`${uid}-rate_unit`}>
											<SelectValue />
										</SelectTrigger>
										<SelectContent>
											<SelectItem value="minutes">Minutes</SelectItem>
											<SelectItem value="hours">Hours</SelectItem>
											<SelectItem value="days">Days</SelectItem>
										</SelectContent>
									</Select>
								</div>
							</div>
						) : (
							<div className="grid gap-2">
								<Label htmlFor={`${uid}-cron_expression`}>
									Cron Expression
								</Label>
								<Input
									id={`${uid}-cron_expression`}
									value={formData.cron_expression}
									onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
										setFormData({
											...formData,
											cron_expression: e.target.value,
										})
									}
									placeholder="0 0 * * *"
								/>
								{errors.cron_expression && (
									<p className="text-sm text-red-500">
										{errors.cron_expression}
									</p>
								)}
								<p className="text-sm text-muted-foreground">
									Format: minute hour day-of-month month day-of-week
								</p>
							</div>
						)}

						<div className="grid gap-2">
							<Label htmlFor={`${uid}-docker_image`}>
								Docker Image (optional)
							</Label>
							<Input
								id={`${uid}-docker_image`}
								value={formData.docker_image}
								onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
									setFormData({ ...formData, docker_image: e.target.value })
								}
								placeholder="Leave empty to use backend image"
							/>
						</div>

						<div className="grid gap-2">
							<Label htmlFor={`${uid}-container_command`}>
								Container Command (comma-separated)
							</Label>
							<Input
								id={`${uid}-container_command`}
								value={formData.container_command}
								onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
									setFormData({
										...formData,
										container_command: e.target.value,
									})
								}
								placeholder="node, scripts/cleanup.js"
							/>
						</div>

						<div className="grid grid-cols-2 gap-4">
							<div className="grid gap-2">
								<Label htmlFor={`${uid}-cpu`}>CPU (units)</Label>
								<Select
									value={formData.cpu.toString()}
									onValueChange={(value: string) => {
										const newCpu = Number.parseInt(value, 10);
										const validMemory = getMemoryOptions(newCpu);
										const newMemory = validMemory.includes(formData.memory)
											? formData.memory
											: validMemory[0] || 512;
										setFormData({
											...formData,
											cpu: newCpu,
											memory: newMemory,
										});
									}}
								>
									<SelectTrigger id={`${uid}-cpu`}>
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

							<div className="grid gap-2">
								<Label htmlFor={`${uid}-memory`}>Memory (MB)</Label>
								<Select
									value={formData.memory.toString()}
									onValueChange={(value: string) =>
										setFormData({
											...formData,
											memory: Number.parseInt(value, 10),
										})
									}
								>
									<SelectTrigger id={`${uid}-memory`}>
										<SelectValue />
									</SelectTrigger>
									<SelectContent>
										{getMemoryOptions(formData.cpu).map((mem) => (
											<SelectItem key={mem} value={mem.toString()}>
												{formatMemory(mem)}
											</SelectItem>
										))}
									</SelectContent>
								</Select>
							</div>
						</div>

						<div className="grid gap-2">
							<Label htmlFor={`${uid}-environment_variables`}>
								Environment Variables (KEY=VALUE, one per line)
							</Label>
							<Textarea
								id={`${uid}-environment_variables`}
								value={formData.environment_variables}
								onChange={(e: React.ChangeEvent<HTMLTextAreaElement>) =>
									setFormData({
										...formData,
										environment_variables: e.target.value,
									})
								}
								placeholder="TASK_TYPE=cleanup&#10;LOG_LEVEL=info"
								rows={4}
							/>
							{errors.environment_variables && (
								<p className="text-sm text-red-500">
									{errors.environment_variables}
								</p>
							)}
						</div>
					</div>

					<DialogFooter>
						<Button type="button" variant="outline" onClick={handleClose}>
							Cancel
						</Button>
						<Button type="submit">Add Task</Button>
					</DialogFooter>
				</form>
			</DialogContent>
		</Dialog>
	);
}
