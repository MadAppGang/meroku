import { Calendar, Cloud, Container, Shield } from "lucide-react";
import type { ComponentNode } from "../types";
import type { YamlInfrastructureConfig } from "../types/yamlConfig";
import { Badge } from "./ui/badge";
import { StyledSection } from "./ui/styled-section";

interface ScheduledTaskIAMPermissionsProps {
	config: YamlInfrastructureConfig;
	node: ComponentNode;
}

export function ScheduledTaskIAMPermissions({
	config,
	node,
}: ScheduledTaskIAMPermissionsProps) {
	const taskName = node.id.replace(/^(scheduled|event)-/, "");
	const paramPath = `/${config.env}/${config.project}/task/${taskName}/*`;

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
					{roles.map((role, index) => (
						<div key={index} className="p-3 bg-gray-900 rounded-lg space-y-2">
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
								{role.permissions.map((perm, i) => (
									<Badge
										key={i}
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
		</div>
	);
}
