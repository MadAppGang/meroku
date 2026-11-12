import { Send } from "lucide-react";
import type {
	EventProduce,
	WorkloadEvents,
	YamlInfrastructureConfig,
} from "../types/yamlConfig";
import {
	compactEvents,
	eventSourceOf,
	isEventBusEnabled,
	projectBusName,
} from "../utils/workloadEvents";
import { EventProducesEditor } from "./EventProducesEditor";
import {
	Card,
	CardContent,
	CardDescription,
	CardHeader,
	CardTitle,
} from "./ui/card";

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
			<CardContent>
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
