import { Workflow } from "lucide-react";
import { useId } from "react";
import type { YamlInfrastructureConfig } from "../types/yamlConfig";
import { isEventBusEnabled, projectBusName } from "../utils/workloadEvents";
import { Alert, AlertDescription } from "./ui/alert";
import {
	Card,
	CardContent,
	CardDescription,
	CardHeader,
	CardTitle,
} from "./ui/card";
import { Label } from "./ui/label";
import { Switch } from "./ui/switch";

interface EventBusSettingsProps {
	config: YamlInfrastructureConfig;
	onConfigChange: (config: Partial<YamlInfrastructureConfig>) => void;
}

/** Entries and event-task rules without a bus field, which follow the switch. */
function followingEntries(config: YamlInfrastructureConfig): string[] {
	const out: string[] = [];
	const count = (label: string, list: Array<{ bus?: string }> | undefined) => {
		const n = (list || []).filter((e) => !e.bus).length;
		if (n > 0) out.push(`${label}: ${n}`);
	};
	count("backend subscribes", config.workload?.events?.subscribes);
	count("backend produces", config.workload?.events?.produces);
	for (const s of config.services || []) {
		count(`${s.name} subscribes`, s.events?.subscribes);
		count(`${s.name} produces`, s.events?.produces);
	}
	for (const t of config.scheduled_tasks || []) {
		count(`${t.name} produces`, t.events?.produces);
	}
	for (const t of config.event_processor_tasks || []) {
		count(`${t.name} produces`, t.events?.produces);
		if (t.rules && t.rules.length > 0) count(`${t.name} rules`, t.rules);
		else if (t.rule_name) out.push(`${t.name} rule: 1`);
	}
	return out;
}

/** The Settings tab of the EventBridge node: the project bus switch. */
export function EventBusSettings({
	config,
	onConfigChange,
}: EventBusSettingsProps) {
	const uid = useId();
	const enabled = isEventBusEnabled(config);
	const busName = projectBusName(config);
	const following = followingEntries(config);

	return (
		<div className="space-y-4">
			<Card>
				<CardHeader>
					<CardTitle className="flex items-center gap-2">
						<Workflow className="w-5 h-5" />
						Project bus
					</CardTitle>
					<CardDescription>
						A bus of this environment's own, {busName}. When it is on, every
						container's EVENT_BUS_NAME names it, and entries and event-task
						rules without a bus use it.
					</CardDescription>
				</CardHeader>
				<CardContent className="space-y-4">
					<div className="flex items-center justify-between gap-4">
						<Label htmlFor={`${uid}-bus`}>Use the project bus</Label>
						<Switch
							id={`${uid}-bus`}
							checked={enabled}
							onCheckedChange={(checked) =>
								onConfigChange({ event_bus: { enabled: checked } })
							}
						/>
					</div>
					<Alert>
						<AlertDescription>
							<span>
								Switching moves every entry without a bus field to the{" "}
								{enabled ? "default" : "project"} bus; event-task rules move by
								replacement. Events from AWS services stay on the default bus.
								{following.length > 0
									? ` Following the switch: ${following.join(", ")}.`
									: " No entry follows the switch."}
							</span>
						</AlertDescription>
					</Alert>
				</CardContent>
			</Card>
		</div>
	);
}
