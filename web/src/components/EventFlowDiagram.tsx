import { ArrowDown, ArrowRight } from "lucide-react";
import type { ReactNode } from "react";
import type { YamlInfrastructureConfig } from "../types/yamlConfig";
import { eventQueueArn, eventQueueName } from "../utils/workloadEvents";

interface EventFlowDiagramProps {
	config: YamlInfrastructureConfig;
	workload: string;
	busName: string;
	/** Subscriptions delivered to the standard queue. */
	ruleCount: number;
	maxReceiveCount: number;
	/** The bus is the enhanced kind: subscribers instead of rules. */
	enhanced?: boolean;
	/** The consumer has an ordered (FIFO) queue set, active or retired. */
	fifo?: boolean;
	/** Ordered subscriptions; 0 with `fifo` means the ordered queue is retired. */
	orderedCount?: number;
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

function plural(n: number, word: string): string {
	return `${n} ${word}${n === 1 ? "" : "s"}`;
}

/** One queue set: bus, delivery, queue and app, with both dead-letter queues. */
function QueueFlow({
	config,
	workload,
	busName,
	deliveryKind,
	deliveryName,
	queueKind,
	fifo,
	maxReceiveCount,
	testId,
}: {
	config: YamlInfrastructureConfig;
	workload: string;
	busName: string;
	deliveryKind: string;
	deliveryName: string;
	queueKind: string;
	fifo: boolean;
	maxReceiveCount: number;
	testId: string;
}) {
	const named = (kind: "queue" | "dlq" | "delivery-dlq") =>
		eventQueueName(config, workload, kind, fifo) ?? "named at apply";
	const queue = eventQueueName(config, workload, "queue", fifo);
	const queueArn = queue ? eventQueueArn(config, queue) : null;
	return (
		<figure
			aria-label={`How ${fifo ? "ordered " : ""}events reach ${workload}`}
			data-testid={testId}
			className="grid grid-cols-event-flow items-center gap-x-2 text-sm"
		>
			<Node kind="EventBridge bus" name={busName} />
			{right}
			<Node kind={deliveryKind} name={deliveryName} />
			{right}
			<Node
				kind={queueKind}
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
			<Node
				kind={fifo ? "Ordered delivery DLQ" : "Delivery DLQ"}
				name={named("delivery-dlq")}
				tone="danger"
			/>
			<div />
			<Node
				kind={fifo ? "Ordered DLQ" : "DLQ"}
				name={named("dlq")}
				tone="danger"
			/>
			{right}
			<Node
				kind="Both DLQs alarm"
				name={`${config.project}-${config.env}-alerts`}
			/>
		</figure>
	);
}

/**
 * How events reach a consumer: the bus, its rules (subscribers on the
 * enhanced bus), the queue and the workload, with the two dead-letter queues
 * modules/events builds. A delivery EventBridge gives up on lands in the
 * delivery DLQ; a message the workload fails max_receive_count times lands in
 * the DLQ. Both alarm to the alerts topic. A consumer with an ordered queue
 * has a second, ordered (FIFO) set, fed by one ordered subscriber.
 */
export function EventFlowDiagram({
	config,
	workload,
	busName,
	ruleCount,
	maxReceiveCount,
	enhanced = false,
	fifo = false,
	orderedCount = 0,
}: EventFlowDiagramProps) {
	const standard = (
		<QueueFlow
			config={config}
			workload={workload}
			busName={busName}
			deliveryKind={enhanced ? "Subscribers" : "Rules"}
			deliveryName={plural(ruleCount, "subscription")}
			queueKind="SQS queue"
			fifo={false}
			maxReceiveCount={maxReceiveCount}
			testId="event-flow"
		/>
	);
	if (!fifo) return standard;
	return (
		<div className="space-y-4">
			{standard}
			<QueueFlow
				config={config}
				workload={workload}
				busName={busName}
				deliveryKind="Ordered subscriber"
				deliveryName={
					orderedCount > 0
						? plural(orderedCount, "ordered subscription")
						: "retired, delivers nothing"
				}
				queueKind="Ordered SQS queue"
				fifo
				maxReceiveCount={maxReceiveCount}
				testId="event-flow-fifo"
			/>
		</div>
	);
}
