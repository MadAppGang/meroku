import { Send } from "lucide-react";
import type {
	EventProduce,
	WorkloadEvents,
	YamlInfrastructureConfig,
} from "../types/yamlConfig";
import {
	compactEvents,
	eventSourceOf,
	isEnhancedBus,
	isEventBusEnabled,
	projectBusName,
} from "../utils/workloadEvents";
import { EventProducesEditor } from "./EventProducesEditor";
import { Badge } from "./ui/badge";
import {
	Card,
	CardContent,
	CardDescription,
	CardHeader,
	CardTitle,
} from "./ui/card";
import { NoteDetail } from "./ui/status-note";

interface TaskEventsTabProps {
	config: YamlInfrastructureConfig;
	taskName: string;
	kind: "scheduled_task" | "event_task";
	events: WorkloadEvents | undefined;
	/** Emits the task's whole events block; the owner writes it into the current task list. */
	onEventsChange: (events: WorkloadEvents | undefined) => void;
}

/**
 * The Events tab of a scheduled or event task: what it may publish. A task
 * cannot subscribe; an event task runs from its rules.
 */
export function TaskEventsTab({
	config,
	taskName,
	kind,
	events,
	onEventsChange,
}: TaskEventsTabProps) {
	const produces = events?.produces || [];
	const save = (next: EventProduce[]) =>
		onEventsChange(compactEvents({ ...events, produces: next }));

	return (
		<Card data-testid={`task-events-${taskName}`}>
			<CardHeader>
				<CardTitle className="flex items-center gap-2">
					<Send className="w-5 h-5" />
					Produces
				</CardTitle>
				<CardDescription>
					Each entry allows PutEvents for its detail-types on one bus. Without
					one the task cannot publish.
				</CardDescription>
			</CardHeader>
			<CardContent className="space-y-4">
				{isEnhancedBus(config) && (
					<div className="space-y-1" data-testid="task-events-bus-arn">
						<div className="flex flex-wrap gap-2">
							<Badge variant="code">
								EVENT_BUS_NAME={projectBusName(config)}
							</Badge>
							<Badge variant="code">EVENT_BUS_ARN</Badge>
						</div>
						<NoteDetail>
							The project bus is Enhanced: publish to it with the EventBridge V2
							client and EVENT_BUS_ARN.
						</NoteDetail>
					</div>
				)}
				<EventProducesEditor
					produces={produces}
					busEnabled={isEventBusEnabled(config)}
					projectBusName={projectBusName(config)}
					eventSource={eventSourceOf(config.project, kind, taskName)}
					onEntryChange={(index, entry) =>
						save(produces.map((p, i) => (i === index ? entry : p)))
					}
					onEntryAdd={(entry) => save([...produces, entry])}
					onEntryRemove={(index) =>
						save(produces.filter((_, i) => i !== index))
					}
				/>
			</CardContent>
		</Card>
	);
}
