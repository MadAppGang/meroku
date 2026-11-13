import { Check, Shield, X } from "lucide-react";
import type { ComponentNode } from "../types";
import type {
	IAMStatement,
	YamlInfrastructureConfig,
} from "../types/yamlConfig";
import { workloadBucketGrants } from "../utils/dataBucket";
import {
	isEventBusEnabled,
	projectBusName,
	resolveBus,
} from "../utils/workloadEvents";
import { withPolicy } from "../utils/workloadPolicy";
import { BACKEND_WORKLOAD_NAME } from "../utils/workloadReferences";
import { Badge } from "./ui/badge";
import {
	Card,
	CardContent,
	CardDescription,
	CardHeader,
	CardTitle,
} from "./ui/card";
import { WorkloadPolicyEditor } from "./WorkloadPolicyEditor";

interface BackendIAMPermissionsProps {
	config: YamlInfrastructureConfig;
	node?: ComponentNode;
	onConfigChange?: (config: Partial<YamlInfrastructureConfig>) => void;
}

export function BackendIAMPermissions({
	config,
	node,
	onConfigChange,
}: BackendIAMPermissionsProps) {
	// Determine if this is for a service or backend
	const isService = node?.type === "service";
	const serviceName = isService ? node.id.replace("service-", "") : null;
	const serviceConfig =
		isService && serviceName
			? config.services?.find((s) => s.name === serviceName)
			: null;
	const taskRoleName = isService
		? `${config.project}_service_${serviceName}_task_${config.env}`
		: `${config.project}_backend_task_${config.env}`;
	const policyStatements: IAMStatement[] =
		(isService ? serviceConfig?.policy : config.workload?.policy) ?? [];

	// The editor hands back the whole list with every field it does not edit
	// (conditions included) carried through, so only the list is replaced here.
	const handlePolicyChange = (policy: IAMStatement[]) => {
		if (!onConfigChange) return;
		if (!isService) {
			onConfigChange({
				workload: { policy: policy.length > 0 ? policy : undefined },
			});
			return;
		}
		if (!config.services) return;
		onConfigChange({
			services: config.services.map((service) =>
				service.name === serviceName ? withPolicy(service, policy) : service,
			),
		});
	};

	const workloadName = isService ? (serviceName ?? "") : BACKEND_WORKLOAD_NAME;
	const bucketGrants = workloadBucketGrants(config, workloadName).map(
		(grant) => ({ ...grant, type: "custom" as const }),
	);

	// events:PutEvents comes only from events.produces, one grant per bus the
	// entries resolve to.
	const produces =
		(isService
			? serviceConfig?.events?.produces
			: config.workload?.events?.produces) ?? [];
	const busEnabled = isEventBusEnabled(config);
	const producesBuses = [
		...new Set(
			produces.map((entry) =>
				resolveBus(entry.bus, busEnabled) === "project"
					? projectBusName(config)
					: "default",
			),
		),
	];

	// Unconditional permissions (always applied)
	const unconditionalPermissions = [
		{
			name: "CloudWatch Full Access",
			type: "managed",
			arn: "arn:aws:iam::aws:policy/CloudWatchFullAccess",
			description: "Applied to both task role and execution role",
		},
		...bucketGrants,
		{
			name: "SES Email Sending",
			type: "custom",
			actions: ["ses:SendEmail", "ses:SendRawEmail"],
			resources: ["*"],
			description: "Send emails via Amazon SES",
		},
		{
			name: "SSM Parameter Access",
			type: "custom",
			actions: [
				"ssm:GetParameter",
				"ssm:GetParameters",
				"ssm:GetParametersByPath",
			],
			resources: isService
				? [
						`arn:aws:ssm:*:*:parameter/${config.env}/${config.project}/${serviceName}/*`,
					]
				: [
						`arn:aws:ssm:*:*:parameter/${config.env}/${config.project}/backend/*`,
					],
			description: isService
				? `Read parameters under /${config.env}/${config.project}/${serviceName}/*`
				: "Read parameters from Parameter Store",
		},
		// Only include X-Ray for backend
		...(!isService
			? [
					{
						name: "X-Ray Write Access",
						type: "managed",
						arn: "arn:aws:iam::aws:policy/AWSXrayWriteOnlyAccess",
						description: "Send traces to AWS X-Ray",
					},
				]
			: []),
	];

	// Conditional permissions
	const conditionalPermissions = [
		{
			name: "EventBridge PutEvents",
			condition: "Events → Produces is not empty",
			enabled: produces.length > 0,
			type: "custom",
			actions: ["events:PutEvents"],
			resources: producesBuses.map(
				(bus) =>
					`arn:aws:events:${config.region}:${config.account_id || "<account>"}:event-bus/${bus}`,
			),
			description:
				"Publish only the detail-types listed under Events → Produces, with this workload's own source",
		},
		{
			name: "SQS Access",
			condition: "sqs_enable = true",
			enabled: !!config.sqs?.enabled,
			type: "policy_arn",
			policyArn: undefined,
			description: "Attaches the policy specified in sqs_policy_arn",
		},
		{
			name: "ECS Execute Command",
			condition: isService
				? "remote_access = true"
				: "backend_remote_access = true",
			enabled: isService
				? serviceConfig?.remote_access || false
				: config.workload?.backend_remote_access !== false,
			type: "custom",
			actions: [
				"ssmmessages:CreateControlChannel",
				"ssmmessages:CreateDataChannel",
				"ssmmessages:OpenControlChannel",
				"ssmmessages:OpenDataChannel",
			],
			resources: ["*"],
			description: "Remote shell access to containers",
		},
		{
			name: "S3 Environment Files",
			condition: "env_files_s3 is not empty",
			enabled: isService
				? (serviceConfig?.env_files_s3?.length || 0) > 0
				: (config.workload?.env_files_s3?.length || 0) > 0,
			type: "custom",
			actions: isService ? ["s3:GetObject"] : ["s3:*"],
			resources: isService
				? serviceConfig?.env_files_s3?.map(
						(f) => `arn:aws:s3:::${f.bucket}/${f.key}`,
					) || []
				: config.workload?.env_files_s3?.map(
						(f) => `arn:aws:s3:::${f.bucket}/${f.key}`,
					) || [],
			description: isService
				? "Read specific S3 environment files"
				: "Access to specific S3 environment files",
		},
		// Only include EFS for backend
		...(!isService
			? [
					{
						name: "EFS Access",
						condition: "backend_efs_mounts is not empty",
						enabled: (config.workload?.efs?.length || 0) > 0,
						type: "custom",
						actions: [
							"elasticfilesystem:ClientMount",
							"elasticfilesystem:ClientWrite",
							"elasticfilesystem:DescribeMountTargets",
							"elasticfilesystem:ClientRootAccess",
						],
						resources:
							config.workload?.efs?.map(
								(efs) =>
									`arn:aws:elasticfilesystem:*:*:file-system/${efs.name}`,
							) || [],
						description: "Mount and access EFS file systems",
					},
				]
			: []),
	];

	// Task Execution Role permissions
	const executionRolePermissions = [
		{
			name: "ECS Task Execution Policy",
			type: "managed",
			arn: "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy",
			description: "Basic ECS task execution permissions",
		},
		{
			name: "CloudWatch Full Access",
			type: "managed",
			arn: "arn:aws:iam::aws:policy/CloudWatchFullAccess",
			description: "Write logs and metrics",
		},
		{
			name: "SSM Parameter Access",
			type: "custom",
			actions: [
				"ssm:GetParameter",
				"ssm:GetParameters",
				"ssm:GetParametersByPath",
			],
			resources: isService
				? [
						`arn:aws:ssm:*:*:parameter/${config.env}/${config.project}/${serviceName}/*`,
					]
				: [
						`arn:aws:ssm:*:*:parameter/${config.env}/${config.project}/backend/*`,
					],
			description: "Read parameters during task startup",
		},
	];

	// Add S3 env files to execution role if configured
	const envFilesS3 = isService
		? serviceConfig?.env_files_s3
		: config.workload?.env_files_s3;
	if ((envFilesS3?.length || 0) > 0) {
		executionRolePermissions.push({
			name: "S3 Environment Files Access",
			type: "custom",
			actions: ["s3:GetObject"],
			resources:
				envFilesS3?.map((f) => `arn:aws:s3:::${f.bucket}/${f.key}`) || [],
			description: "Read environment files during task startup",
		});
	}

	return (
		<div className="space-y-4">
			<Card>
				<CardHeader>
					<CardTitle className="flex items-center gap-2">
						<Shield className="w-5 h-5" />
						IAM Roles
					</CardTitle>
					<CardDescription>
						Roles used by the ECS{" "}
						{isService ? `${serviceName} service` : "backend service"}
					</CardDescription>
				</CardHeader>
				<CardContent>
					<div className="space-y-3">
						<div className="p-3 bg-gray-800 rounded">
							<p className="text-sm font-medium text-gray-200 mb-1">
								Task Role
							</p>
							<code className="text-sm font-mono text-blue-400">
								{isService
									? `${config.project}_service_${serviceName}_task_${config.env}`
									: `${config.project}_backend_task_${config.env}`}
							</code>
							<p className="text-xs text-gray-400 mt-1">
								Used by the running container to access AWS services
							</p>
						</div>
						<div className="p-3 bg-gray-800 rounded">
							<p className="text-sm font-medium text-gray-200 mb-1">
								Task Execution Role
							</p>
							<code className="text-sm font-mono text-blue-400">
								{isService
									? `${config.project}_service_${serviceName}_task_execution_${config.env}`
									: `${config.project}_backend_task_execution_${config.env}`}
							</code>
							<p className="text-xs text-gray-400 mt-1">
								Used by ECS to start the task (pull images, write logs)
							</p>
						</div>
					</div>
				</CardContent>
			</Card>

			{/* Unconditional Permissions */}
			<Card>
				<CardHeader>
					<CardTitle>Service Permissions</CardTitle>
					<CardDescription>
						These permissions are always attached to the task role
					</CardDescription>
				</CardHeader>
				<CardContent>
					<div className="space-y-3">
						{unconditionalPermissions.map((perm) => (
							<div
								key={perm.name}
								className="border border-gray-700 rounded-lg p-3 space-y-2"
							>
								<div className="flex items-start justify-between">
									<div className="flex items-center gap-2">
										<Check className="w-4 h-4 text-green-400" />
										<span className="font-medium text-sm">{perm.name}</span>
									</div>
									<Badge
										variant={perm.type === "managed" ? "default" : "secondary"}
									>
										{perm.type === "managed"
											? "Managed Policy"
											: "Custom Policy"}
									</Badge>
								</div>
								<p className="text-xs text-gray-400">{perm.description}</p>
								{perm.type === "managed" ? (
									<code className="text-xs text-gray-500 font-mono block">
										{perm.arn}
									</code>
								) : (
									<div className="space-y-2">
										<div>
											<p className="text-xs font-medium text-gray-500">
												Actions:
											</p>
											<div className="flex flex-wrap gap-1 mt-1">
												{perm.actions?.map((action) => (
													<Badge key={action} variant="code">
														{action}
													</Badge>
												))}
											</div>
										</div>
										<div>
											<p className="text-xs font-medium text-gray-500">
												Resources:
											</p>
											<div className="space-y-1 mt-1">
												{perm.resources?.map((resource) => (
													<code
														key={resource}
														className="text-xs text-gray-400 block font-mono"
													>
														{resource}
													</code>
												))}
											</div>
										</div>
									</div>
								)}
							</div>
						))}
					</div>
				</CardContent>
			</Card>

			<WorkloadPolicyEditor
				statements={policyStatements}
				onChange={handlePolicyChange}
				roleName={taskRoleName}
			/>

			{/* Optional Permissions */}
			<Card>
				<CardHeader>
					<CardTitle>Optional Permissions</CardTitle>
					<CardDescription>
						Permissions applied based on configuration
					</CardDescription>
				</CardHeader>
				<CardContent>
					<div className="space-y-3">
						{conditionalPermissions.map((perm) => (
							<div
								key={perm.name}
								className={`border rounded-lg p-3 space-y-2 ${
									perm.enabled
										? "border-gray-700"
										: "border-gray-800 opacity-60"
								}`}
							>
								<div className="flex items-start justify-between">
									<div className="flex items-center gap-2">
										{perm.enabled ? (
											<Check className="w-4 h-4 text-green-400" />
										) : (
											<X className="w-4 h-4 text-gray-500" />
										)}
										<span className="font-medium text-sm">{perm.name}</span>
									</div>
									<Badge variant={perm.enabled ? "default" : "outline"}>
										{perm.enabled ? "Active" : "Inactive"}
									</Badge>
								</div>
								<p className="text-xs text-blue-400">
									Condition: <code className="font-mono">{perm.condition}</code>
								</p>
								<p className="text-xs text-gray-400">{perm.description}</p>

								{perm.enabled && (
									<div className="pt-2">
										{perm.type === "policy_arn" && perm.policyArn && (
											<code className="text-xs text-gray-500 font-mono block">
												Policy ARN: {perm.policyArn}
											</code>
										)}
										{perm.type === "custom" && perm.actions && (
											<div className="space-y-2">
												<div>
													<p className="text-xs font-medium text-gray-500">
														Actions:
													</p>
													<div className="flex flex-wrap gap-1 mt-1">
														{perm.actions.map((action) => (
															<Badge key={action} variant="code">
																{action}
															</Badge>
														))}
													</div>
												</div>
												{perm.resources && perm.resources.length > 0 && (
													<div>
														<p className="text-xs font-medium text-gray-500">
															Resources:
														</p>
														<div className="space-y-1 mt-1">
															{perm.resources.map((resource) => (
																<code
																	key={resource}
																	className="text-xs text-gray-400 block font-mono"
																>
																	{resource}
																</code>
															))}
														</div>
													</div>
												)}
											</div>
										)}
									</div>
								)}
							</div>
						))}
					</div>
				</CardContent>
			</Card>

			{/* Task Execution Role */}
			<Card>
				<CardHeader>
					<CardTitle className="flex items-center gap-2">
						<Shield className="w-5 h-5" />
						Task Execution Role Permissions
					</CardTitle>
					<CardDescription>
						<code className="font-mono text-sm">
							{isService
								? `${config.project}_service_${serviceName}_task_execution_${config.env}`
								: `${config.project}_backend_task_execution_${config.env}`}
						</code>
					</CardDescription>
				</CardHeader>
				<CardContent>
					<div className="space-y-3">
						<div className="p-3 bg-gray-800 rounded mb-3">
							<p className="text-sm text-gray-300">
								The execution role is used by ECS to pull images, write logs,
								and read secrets during task startup.
							</p>
						</div>

						{executionRolePermissions.map((perm) => (
							<div
								key={perm.name}
								className="border border-gray-700 rounded-lg p-3 space-y-2"
							>
								<div className="flex items-start justify-between">
									<div className="flex items-center gap-2">
										<Check className="w-4 h-4 text-green-400" />
										<span className="font-medium text-sm">{perm.name}</span>
									</div>
									<Badge
										variant={perm.type === "managed" ? "default" : "secondary"}
									>
										{perm.type === "managed"
											? "Managed Policy"
											: "Custom Policy"}
									</Badge>
								</div>
								<p className="text-xs text-gray-400">{perm.description}</p>
								{perm.type === "managed" ? (
									<code className="text-xs text-gray-500 font-mono block">
										{perm.arn}
									</code>
								) : (
									<div className="space-y-2">
										<div>
											<p className="text-xs font-medium text-gray-500">
												Actions:
											</p>
											<div className="flex flex-wrap gap-1 mt-1">
												{perm.actions?.map((action) => (
													<Badge key={action} variant="code">
														{action}
													</Badge>
												))}
											</div>
										</div>
										{perm.resources && (
											<div>
												<p className="text-xs font-medium text-gray-500">
													Resources:
												</p>
												<div className="space-y-1 mt-1">
													{perm.resources.map((resource) => (
														<code
															key={resource}
															className="text-xs text-gray-400 block font-mono"
														>
															{resource}
														</code>
													))}
												</div>
											</div>
										)}
									</div>
								)}
							</div>
						))}
					</div>
				</CardContent>
			</Card>
		</div>
	);
}
