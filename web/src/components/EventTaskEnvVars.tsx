import { ExternalLink, Key, Settings, Zap } from "lucide-react";
import type { AccountInfo } from "../api/infrastructure";
import type { ComponentNode } from "../types";
import type { YamlInfrastructureConfig } from "../types/yamlConfig";
import {
	reservedWorkloadEnvError,
	workloadYamlEnv,
} from "../utils/reservedEnv";
import { Button } from "./ui/button";
import { StyledSection } from "./ui/styled-section";

interface EventTaskEnvVarsProps {
	config: YamlInfrastructureConfig;
	node: ComponentNode;
	accountInfo?: AccountInfo;
}

export function EventTaskEnvVars({
	config,
	node,
	accountInfo,
}: EventTaskEnvVarsProps) {
	const taskName = node.id.replace("event-", "");
	const parameterStorePath = `/${config.env}/${config.project}/task/${taskName}/`;
	const hasSqsEnvVar = config.sqs?.enabled;
	const workload = { kind: "event-task", name: taskName } as const;
	const yamlVars = Object.entries(workloadYamlEnv(config, workload));

	return (
		<div className="space-y-4">
			{/* Static Environment Variables - only show if SQS enabled */}
			{hasSqsEnvVar && (
				<StyledSection
					title="Injected Variables"
					description="Automatically set by infrastructure"
					icon={Zap}
					iconColor="text-amber-400"
				>
					<div className="p-3 bg-gray-900 rounded-lg">
						<div className="flex items-center justify-between">
							<code className="text-sm font-mono text-blue-400">
								SQS_QUEUE_URL
							</code>
						</div>
						<div className="text-xs font-mono text-gray-400 mt-2 break-all">
							https://sqs.{config.region}.amazonaws.com/
							{accountInfo?.accountId ||
								config.ecr_account_id ||
								"<ACCOUNT_ID>"}
							/{config.project}-{config.env}-{config.sqs?.name || "queue"}
						</div>
					</div>
				</StyledSection>
			)}

			{yamlVars.length > 0 && (
				<StyledSection
					title="YAML Variables"
					description="environment_variables in the YAML"
					icon={Settings}
					iconColor="text-green-400"
				>
					<div className="space-y-2">
						{yamlVars.map(([name, value]) => {
							const nameError = reservedWorkloadEnvError(
								config,
								workload,
								name,
							);
							return (
								<div key={name} className="p-3 bg-gray-900 rounded-lg">
									<code className="text-sm font-mono text-green-400">
										{name}
									</code>
									<div className="text-xs font-mono text-gray-400 mt-2 break-all">
										{value}
									</div>
									{nameError && (
										<p className="mt-1 text-xs text-danger" role="alert">
											{nameError} Remove it from environment_variables; generate
											refuses it.
										</p>
									)}
								</div>
							);
						})}
					</div>
				</StyledSection>
			)}

			{/* Parameter Store - always show */}
			<StyledSection
				title="Custom Variables"
				description="Stored in AWS Parameter Store"
				icon={Key}
				iconColor="text-orange-400"
				actions={
					<Button
						size="sm"
						variant="outline"
						className="h-7 text-xs"
						onClick={() => {
							const region = config.region;
							window.open(
								`https://${region}.console.aws.amazon.com/systems-manager/parameters?region=${region}&tab=Table&path=${encodeURIComponent(parameterStorePath)}`,
								"_blank",
							);
						}}
					>
						<ExternalLink className="w-3 h-3 mr-1" />
						AWS Console
					</Button>
				}
			>
				<div className="p-3 bg-gray-900 rounded-lg">
					<code className="text-sm text-orange-400 font-mono">
						{parameterStorePath}*
					</code>
					<p className="text-xs text-gray-500 mt-2">
						Parameters created here become environment variables
					</p>
				</div>
			</StyledSection>
		</div>
	);
}
