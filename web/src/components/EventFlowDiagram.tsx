import { ArrowDown, ArrowRight } from "lucide-react";
import type { ReactNode } from "react";
import type { YamlInfrastructureConfig } from "../types/yamlConfig";
import { eventQueueArn, eventQueueName } from "../utils/workloadEvents";

interface EventFlowDiagramProps {
	config: YamlInfrastructureConfig;
	workload: string;
	busName: string;
	ruleCount: number;
	maxReceiveCount: number;
}

/** One box of the diagram: what it is, and its AWS name. */
function Node({
	kind,
	name,
	title,
	tone = "default",
}: {
	kind: string;
	name: ReactNode;
	title?: string;
	tone?: "default" | "danger";
}) {
	return (
		<div
			className={`min-w-0 rounded-md border px-2 py-1.5 ${
				tone === "danger" ? "border-danger/50 bg-danger/10" : "bg-muted/40"
			}`}
			title={title}
		>
			<div className="text-xs text-muted-foreground">{kind}</div>
			<code className="block truncate font-mono text-xs">{name}</code>
		</div>
	);
}

function Down({ label }: { label: string }) {
	return (
		<div className="flex items-center gap-1 py-1 text-xs text-muted-foreground">
			<ArrowDown className="h-3 w-3 shrink-0" />
			{label}
		</div>
	);
}

const right = (
	<ArrowRight
		className="h-4 w-4 shrink-0 text-muted-foreground"
		aria-hidden="true"
	/>
);

/**
 * How events reach a consumer: the bus, its rules, the queue and the
 * workload, with the two dead-letter queues modules/events builds. A delivery
 * EventBridge gives up on lands in the delivery DLQ; a message the workload
 * fails max_receive_count times lands in the DLQ. Both alarm to the alerts topic.
 */
export function EventFlowDiagram({
	config,
	workload,
	busName,
	ruleCount,
	maxReceiveCount,
}: EventFlowDiagramProps) {
	const named = (kind: "queue" | "dlq" | "delivery-dlq") =>
		eventQueueName(config, workload, kind) ?? "named at apply";
	const queue = eventQueueName(config, workload);
	const queueArn = queue ? eventQueueArn(config, queue) : null;
	return (
		<figure
			aria-label={`How events reach ${workload}`}
			data-testid="event-flow"
			className="grid grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)_auto_minmax(0,1.6fr)_auto_minmax(0,1fr)] items-center gap-x-2 text-sm"
		>
			<Node kind="EventBridge bus" name={busName} />
			{right}
			<Node
				kind="Rules"
				name={`${ruleCount} subscription${ruleCount === 1 ? "" : "s"}`}
			/>
			{right}
			<Node
				kind="SQS queue"
				name={named("queue")}
				title={queueArn ?? undefined}
			/>
			{right}
			<Node kind="Your app" name={workload} />

			<div />
			<div />
			<Down label="delivery fails" />
			<div />
			<Down label={`fails ${maxReceiveCount}×`} />
			<div />
			<div />

			<div />
			<div />
			<Node kind="Delivery DLQ" name={named("delivery-dlq")} tone="danger" />
			<div />
			<Node kind="DLQ" name={named("dlq")} tone="danger" />
			{right}
			<Node
				kind="Both DLQs alarm"
				name={`${config.project}-${config.env}-alerts`}
			/>
		</figure>
	);
}
