import { Calendar, Cloud, Container, Shield } from "lucide-react";
import type { ComponentNode } from "../types";
import type {
	IAMStatement,
	YamlInfrastructureConfig,
} from "../types/yamlConfig";
import { withPolicy } from "../utils/workloadPolicy";
import { Badge } from "./ui/badge";
import { StyledSection } from "./ui/styled-section";
import { WorkloadPolicyEditor } from "./WorkloadPolicyEditor";

interface ScheduledTaskIAMPermissionsProps {
	config: YamlInfrastructureConfig;
	node: ComponentNode;
	/** Absent shows the roles read-only, without the policy editor. */
	onConfigChange?: (config: Partial<YamlInfrastructureConfig>) => void;
}

export function ScheduledTaskIAMPermissions({
	config,
	node,
	onConfigChange,
}: ScheduledTaskIAMPermissionsProps) {
	const taskName = node.id.replace(/^(scheduled|event)-/, "");
	const paramPath = `/${config.env}/${config.project}/task/${taskName}/*`;
	const isEventTask = node.type === "event-task";
	const task = isEventTask
		? config.event_processor_tasks?.find((t) => t.name === taskName)
		: config.scheduled_tasks?.find((t) => t.name === taskName);

	// Scheduled and event tasks live in two lists; each change rewrites only
	// this task's entry in its own list.
	const handlePolicyChange = (policy: IAMStatement[]) => {
		if (!onConfigChange) return;
		if (isEventTask) {
			if (!config.event_processor_tasks) return;
			onConfigChange({
				event_processor_tasks: config.event_processor_tasks.map((t) =>
					t.name === taskName ? withPolicy(t, policy) : t,
				),
			});
			return;
		}
		if (!config.scheduled_tasks) return;
		onConfigChange({
			scheduled_tasks: config.scheduled_tasks.map((t) =>
				t.name === taskName ? withPolicy(t, policy) : t,
			),
		});
	};

	const roles = [
		{
			name: `${config.project}_${taskName}_task_${config.env}`,
			type: "Task Role",
			icon: Container,
			iconColor: "text-cyan-400",
			trustedBy: "ecs-tasks.amazonaws.com",
			permissions: ["CloudWatch Logs (write)", `SSM Parameters (${paramPath})`],
		},
		{
			name: `${config.project}_scheduler_${taskName}_task_execution_${config.env}`,
			type: "Execution Role",
			icon: Cloud,
			iconColor: "text-sky-400",
			trustedBy: "ecs-tasks.amazonaws.com",
			permissions: [
				"ECR (pull images)",
				"CloudWatch Logs (create)",
				"SSM (read secrets)",
			],
		},
		{
			name: `${config.project}_scheduler_${taskName}_role_${config.env}`,
			type: "Scheduler Role",
			icon: Calendar,
			iconColor: "text-violet-400",
			trustedBy: "scheduler.amazonaws.com",
			permissions: ["ECS (run task)", "IAM (pass role)"],
		},
	];

	return (
		<div className="space-y-4">
			<StyledSection
				title="IAM Roles"
				description="Permissions for this task"
				icon={Shield}
				iconColor="text-green-400"
			>
				<div className="space-y-4">
					{roles.map((role) => (
						<div
							key={role.name}
							className="p-3 bg-gray-900 rounded-lg space-y-2"
						>
							<div className="flex items-center gap-2">
								<div className={`p-1 rounded bg-gray-800 ${role.iconColor}`}>
									<role.icon className="w-3.5 h-3.5" />
								</div>
								<span className="text-sm font-medium text-white">
									{role.type}
								</span>
							</div>

							<code className="text-xs text-gray-500 font-mono block truncate">
								{role.name}
							</code>

							<div className="flex flex-wrap gap-1.5 pt-1">
								{role.permissions.map((perm) => (
									<Badge
										key={perm}
										variant="secondary"
										className="text-xs font-normal"
									>
										{perm}
									</Badge>
								))}
							</div>

							<p className="text-xs text-gray-600">Trusted: {role.trustedBy}</p>
						</div>
					))}
				</div>
			</StyledSection>

			{onConfigChange && task && (
				<WorkloadPolicyEditor
					statements={task.policy ?? []}
					onChange={handlePolicyChange}
					roleName={roles[0].name}
				/>
			)}
		</div>
	);
}
