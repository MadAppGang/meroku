import { Database, ExternalLink, Info, Share2 } from "lucide-react";
import { useMemo, memo } from "react";
import type { ECRConfig, YamlInfrastructureConfig } from "../types/yamlConfig";
import { Alert, AlertDescription } from "./ui/alert";
import { Badge } from "./ui/badge";
import { Label } from "./ui/label";
import { Separator } from "./ui/separator";
import { ECRConfigSection } from "./ECRConfigSection";

interface ECRConfigEditorProps {
	config: YamlInfrastructureConfig;
	currentServiceName: string;
	currentServiceType: "services" | "event_processor_tasks" | "scheduled_tasks";
	ecrConfig: ECRConfig | undefined;
	onEcrConfigChange: (config: ECRConfig | undefined) => void;
	accountInfo?: { accountId?: string };
}

export const ECRConfigEditor = memo(function ECRConfigEditor({
	config,
	currentServiceName,
	currentServiceType,
	ecrConfig,
	onEcrConfigChange,
	accountInfo,
}: ECRConfigEditorProps) {
	// Build ECR repository URI based on service type
	const repoName = currentServiceType === "services"
		? `${config.project}_service_${currentServiceName}`
		: `${config.project}_task_${currentServiceName}`;

	const ecrRepoUri = `${accountInfo?.accountId || config.ecr_account_id || "<ACCOUNT_ID>"}.dkr.ecr.${config.region}.amazonaws.com/${repoName}`;

	// Build available ECR sources from all service types - MEMOIZED
	const availableSources = useMemo(() => {
		const sources: Array<{
			name: string;
			type: "services" | "event_processor_tasks" | "scheduled_tasks";
			displayType: string;
		}> = [];

		// Add services with create_ecr mode
		config.services?.forEach(svc => {
			const isCurrentService = currentServiceType === "services" && svc.name === currentServiceName;
			if (!isCurrentService && (!svc.ecr_config || svc.ecr_config.mode === "create_ecr")) {
				sources.push({
					name: svc.name,
					type: "services",
					displayType: "Service",
				});
			}
		});

		// Add event processors with create_ecr mode
		config.event_processor_tasks?.forEach(ep => {
			const isCurrentService = currentServiceType === "event_processor_tasks" && ep.name === currentServiceName;
			if (!isCurrentService && (!ep.ecr_config || ep.ecr_config.mode === "create_ecr")) {
				sources.push({
					name: ep.name,
					type: "event_processor_tasks",
					displayType: "Event Processor",
				});
			}
		});

		// Add scheduled tasks with create_ecr mode
		config.scheduled_tasks?.forEach(st => {
			const isCurrentService = currentServiceType === "scheduled_tasks" && st.name === currentServiceName;
			if (!isCurrentService && (!st.ecr_config || st.ecr_config.mode === "create_ecr")) {
				sources.push({
					name: st.name,
					type: "scheduled_tasks",
					displayType: "Cron Job",
				});
			}
		});

		return sources;
	}, [config.services, config.event_processor_tasks, config.scheduled_tasks, currentServiceType, currentServiceName]);

	const currentConfig = useMemo(() =>
		ecrConfig || { mode: "create_ecr" as const }
	, [ecrConfig]);

	return (
		<div className="space-y-4">
			{/* ECR Configuration Display (Read-only) */}
			{ecrConfig && (
				<div className="p-3 bg-gray-900/50 border border-gray-700 rounded-lg space-y-3">
					<div className="flex items-start gap-2">
						<Database className="w-4 h-4 text-gray-400 mt-0.5 flex-shrink-0" />
						<div className="flex-1 space-y-2">
							<div className="flex items-center gap-2">
								<Label className="text-xs text-gray-300">ECR Configuration:</Label>
								{ecrConfig.mode === "create_ecr" && (
									<Badge variant="default" className="text-xs">
										Dedicated Repository
									</Badge>
								)}
								{ecrConfig.mode === "manual_repo" && (
									<Badge variant="secondary" className="text-xs flex items-center gap-1">
										<ExternalLink className="w-3 h-3" />
										Manual Repository
									</Badge>
								)}
								{ecrConfig.mode === "use_existing" && (
									<Badge variant="outline" className="text-xs flex items-center gap-1">
										<Share2 className="w-3 h-3" />
										Shared Repository
									</Badge>
								)}
							</div>

							{ecrConfig.mode === "create_ecr" && (
								<div>
									<p className="text-xs text-gray-400 font-mono break-all">
										{ecrRepoUri}
									</p>
									<p className="text-xs text-gray-500 mt-1">
										A dedicated ECR repository will be created
									</p>
								</div>
							)}

							{ecrConfig.mode === "manual_repo" && ecrConfig.repository_uri && (
								<div>
									<p className="text-xs text-gray-400 font-mono break-all">
										{ecrConfig.repository_uri}
									</p>
									<p className="text-xs text-gray-500 mt-1">
										Using manually specified ECR repository
									</p>
								</div>
							)}

							{ecrConfig.mode === "use_existing" && (
								<div>
									<p className="text-xs text-gray-300">
										Source: <span className="font-mono text-gray-400">
											{ecrConfig.source_service_type?.replace("_", " ")} / {ecrConfig.source_service_name}
										</span>
									</p>
									<p className="text-xs text-gray-500 mt-1">
										Sharing ECR repository from another service
									</p>
								</div>
							)}
						</div>
					</div>
				</div>
			)}

			{/* Legacy default ECR info (when no ecr_config) */}
			{!ecrConfig && (
				<Alert>
					<Info className="h-4 h-4" />
					<AlertDescription className="text-xs">
						Default ECR: <code className="text-blue-400">{ecrRepoUri}</code>
					</AlertDescription>
				</Alert>
			)}

			<Separator />

			{/* ECR Configuration Editor */}
			<ECRConfigSection
				config={currentConfig}
				onChange={onEcrConfigChange}
				availableSources={availableSources}
				currentServiceName={currentServiceName}
			/>
		</div>
	);
});
