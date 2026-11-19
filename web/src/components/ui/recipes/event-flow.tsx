import { cva, type VariantProps } from "class-variance-authority";
import { ArrowDown, ArrowRight } from "lucide-react";
import type { ReactNode } from "react";

/** A box of the flow: what it is, and its AWS name. */
const flowNodeVariants = cva("min-w-0 rounded-md border px-2 py-1.5", {
	variants: {
		tone: {
			default: "bg-muted/40",
			// A dead-letter queue: where failed messages end up.
			danger: "border-danger/50 bg-danger/10",
			// Nothing delivers through it: none yet, or retired.
			idle: "border-dashed text-muted-foreground",
		},
	},
	defaultVariants: {
		tone: "default",
	},
});

function FlowNode({
	kind,
	name,
	title,
	tone,
}: {
	kind: string;
	name: ReactNode;
	title?: string;
} & VariantProps<typeof flowNodeVariants>) {
	return (
		<div className={flowNodeVariants({ tone })} title={title}>
			<div className="text-xs text-muted-foreground">{kind}</div>
			<code className="block truncate font-mono text-xs">{name}</code>
		</div>
	);
}

function FlowDown({ label }: { label: string }) {
	return (
		<div className="flex items-center gap-1 py-1 text-xs text-muted-foreground">
			<ArrowDown className="h-3 w-3 shrink-0" aria-hidden="true" />
			{label}
		</div>
	);
}

function FlowRight() {
	return (
		<ArrowRight
			className="h-4 w-4 shrink-0 text-muted-foreground"
			aria-hidden="true"
		/>
	);
}

/** One way events reach the queue: a bus and what delivers from it. */
export interface EventFlowPath {
	/** The bus the events are on, as AWS names it. */
	bus: string;
	/** What delivers them: "Rules", "Subscribers", "Ordered subscriber". */
	delivery: string;
	/** How many entries it delivers, in words: "2 subscriptions". */
	summary: string;
}

/** A named AWS resource of the flow. */
export interface EventFlowResource {
	kind: string;
	name: string;
	/** Shown on hover, e.g. the ARN. */
	title?: string;
}

export interface EventFlowProps {
	/** The figure's accessible name. */
	label: string;
	/**
	 * The ways events reach the queue, one row each. Empty means nothing
	 * delivers to it: `idle` says why.
	 */
	paths: EventFlowPath[];
	/** Shown in place of the paths when there are none: none yet, or retired. */
	idle?: EventFlowResource;
	queue: EventFlowResource;
	/** The workload that reads the queue. */
	app: string;
	/** Where a delivery that fails lands. */
	deliveryDlq: EventFlowResource;
	/** Where a message the app fails `maxReceiveCount` times lands. */
	dlq: EventFlowResource;
	maxReceiveCount: number;
	/** The topic both dead-letter queues alarm to. */
	alerts: string;
	"data-testid"?: string;
}

// The queue, its arrow and the app stand beside every path row.
const ROW_SPAN: Record<number, string> = {
	1: "row-span-1",
	2: "row-span-2",
	3: "row-span-3",
};

/**
 * How events reach a workload's queue: each bus and what delivers from it,
 * the queue and the app, with the two dead-letter queues and the alarm
 * topic below.
 */
export function EventFlow({
	label,
	paths,
	idle,
	queue,
	app,
	deliveryDlq,
	dlq,
	maxReceiveCount,
	alerts,
	"data-testid": testId,
}: EventFlowProps) {
	const rows = Math.max(1, paths.length);
	const span = ROW_SPAN[Math.min(rows, 3)];
	const beside = (
		<>
			<div className={`${span} self-center`}>
				<FlowNode kind={queue.kind} name={queue.name} title={queue.title} />
			</div>
			<div className={`${span} self-center`}>
				<FlowRight />
			</div>
			<div className={`${span} self-center`}>
				<FlowNode kind="Your app" name={app} />
			</div>
		</>
	);
	return (
		<figure
			aria-label={label}
			data-testid={testId}
			className="grid grid-cols-event-flow items-center gap-x-2 gap-y-1 text-sm"
		>
			{paths.length === 0 ? (
				<>
					<div />
					<div />
					{idle ? (
						<FlowNode kind={idle.kind} name={idle.name} tone="idle" />
					) : (
						<div />
					)}
					<div />
					{beside}
				</>
			) : (
				paths.map((path, i) => (
					<PathRow
						key={`${path.bus}:${path.delivery}`}
						path={path}
						beside={i === 0 ? beside : null}
					/>
				))
			)}

			<div />
			<div />
			<FlowDown label="delivery fails" />
			<div />
			<FlowDown label={`fails ${maxReceiveCount}×`} />
			<div />
			<div />

			<div />
			<div />
			<FlowNode
				kind={deliveryDlq.kind}
				name={deliveryDlq.name}
				title={deliveryDlq.title}
				tone="danger"
			/>
			<div />
			<FlowNode
				kind={dlq.kind}
				name={dlq.name}
				title={dlq.title}
				tone="danger"
			/>
			<FlowRight />
			<FlowNode kind="Both DLQs alarm" name={alerts} />
		</figure>
	);
}

function PathRow({ path, beside }: { path: EventFlowPath; beside: ReactNode }) {
	return (
		<>
			<FlowNode kind="EventBridge bus" name={path.bus} />
			<FlowRight />
			<FlowNode kind={path.delivery} name={path.summary} />
			<FlowRight />
			{beside}
		</>
	);
}
