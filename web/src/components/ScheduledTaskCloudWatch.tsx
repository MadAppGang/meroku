import { Cloud, Copy, ExternalLink } from "lucide-react";
import { useState } from "react";
import type { ComponentNode } from "../types";
import type { YamlInfrastructureConfig } from "../types/yamlConfig";
import { Button } from "./ui/button";
import { StyledSection } from "./ui/styled-section";

interface ScheduledTaskCloudWatchProps {
	config: YamlInfrastructureConfig;
	node: ComponentNode;
}

export function ScheduledTaskCloudWatch({
	config,
	node,
}: ScheduledTaskCloudWatchProps) {
	const [copiedCommand, setCopiedCommand] = useState<string | null>(null);

	const taskName = node.id.replace(/^(scheduled|event)-/, "");
	const logGroupName = `${config.project}_task_${taskName}_${config.env}`;

	const copyCommand = (command: string, id: string) => {
		navigator.clipboard.writeText(command);
		setCopiedCommand(id);
		setTimeout(() => setCopiedCommand(null), 2000);
	};

	const openCloudWatchConsole = () => {
		const region = config.region;
		window.open(
			`https://${region}.console.aws.amazon.com/cloudwatch/home?region=${region}#logsV2:log-groups/log-group/${encodeURIComponent(logGroupName)}`,
			"_blank",
		);
	};

	const tailCommand = `aws logs tail ${logGroupName} --follow`;
	const recentCommand = `aws logs filter-log-events --log-group-name "${logGroupName}" --start-time $(date -d '1 hour ago' +%s)000`;

	return (
		<div className="space-y-4">
			<StyledSection
				title="CloudWatch Logs"
				description={logGroupName}
				icon={Cloud}
				iconColor="text-sky-400"
				actions={
					<Button
						size="sm"
						variant="outline"
						className="h-7 text-xs"
						onClick={openCloudWatchConsole}
					>
						<ExternalLink className="w-3 h-3 mr-1" />
						Open Console
					</Button>
				}
			>
				<div className="space-y-3">
					<div className="p-3 bg-gray-900 rounded-lg">
						<div className="flex items-center justify-between mb-2">
							<span className="text-xs text-gray-400">Tail logs (live)</span>
							<Button
								size="sm"
								variant="ghost"
								onClick={() => copyCommand(tailCommand, "tail")}
								className="h-6 px-2 text-xs"
							>
								<Copy className="w-3 h-3 mr-1" />
								{copiedCommand === "tail" ? "Copied!" : "Copy"}
							</Button>
						</div>
						<code className="text-xs text-gray-300 font-mono block overflow-x-auto">
							{tailCommand}
						</code>
					</div>

					<div className="p-3 bg-gray-900 rounded-lg">
						<div className="flex items-center justify-between mb-2">
							<span className="text-xs text-gray-400">Last hour</span>
							<Button
								size="sm"
								variant="ghost"
								onClick={() => copyCommand(recentCommand, "recent")}
								className="h-6 px-2 text-xs"
							>
								<Copy className="w-3 h-3 mr-1" />
								{copiedCommand === "recent" ? "Copied!" : "Copy"}
							</Button>
						</div>
						<code className="text-xs text-gray-300 font-mono block overflow-x-auto whitespace-pre-wrap">
							{recentCommand}
						</code>
					</div>
				</div>
			</StyledSection>
		</div>
	);
}
