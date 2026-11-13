import { ChevronDown, Inbox, Plus, Send, Trash2 } from "lucide-react";
import { useEffect, useId, useState } from "react";
import type {
	EventBusChoice,
	EventProduce,
	EventSubscription,
	WorkloadEvents,
	YamlInfrastructureConfig,
} from "../types/yamlConfig";
import {
	compactEvents,
	eventSourceOf,
	isEventBusEnabled,
	projectBusName,
	SUBSCRIPTION_NAME_PATTERN,
	subscriptionKey,
} from "../utils/workloadEvents";
import {
	EventBusSelect,
	EventProducesEditor,
	resolvedBusLabel,
} from "./EventProducesEditor";
import { Alert, AlertDescription } from "./ui/alert";
import { Badge } from "./ui/badge";
import { Button } from "./ui/button";
import {
	Card,
	CardContent,
	CardDescription,
	CardHeader,
	CardTitle,
} from "./ui/card";
import {
	Collapsible,
	CollapsibleContent,
	CollapsibleTrigger,
} from "./ui/collapsible";
import { Input } from "./ui/input";
import { Label } from "./ui/label";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "./ui/select";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "./ui/table";

interface ServiceEventsTabProps {
	config: YamlInfrastructureConfig;
	/** "backend" or the service name. */
	workloadName: string;
	kind: "backend" | "service";
	events: WorkloadEvents | undefined;
	/**
	 * Emits the workload's whole events block. The owner writes it into the
	 * current config: `{ workload: { events } }` for the backend, or the one
	 * service's entry for a service.
	 */
	onEventsChange: (events: WorkloadEvents | undefined) => void;
}

type MatchKind = "exact" | "prefix";

const VISIBILITY_MAX = 43200;
const MAX_RECEIVE_MAX = 1000;

/**
 * The Events tab of the backend and of every service: what the workload
 * subscribes to (its own queue), what it may publish, and the queue settings.
 * See ai_docs/EVENTS.md.
 */
export function ServiceEventsTab({
	config,
	workloadName,
	kind,
	events,
	onEventsChange,
}: ServiceEventsTabProps) {
	const uid = useId();
	const busEnabled = isEventBusEnabled(config);
	const busName = projectBusName(config);
	const subscribes = events?.subscribes || [];
	const produces = events?.produces || [];
	const source = eventSourceOf(config.project, kind, workloadName);

	// The add form is local until "Add", so a half-typed row is never saved.
	const [matchKind, setMatchKind] = useState<MatchKind>("exact");
	const [newType, setNewType] = useState("");
	const [newSource, setNewSource] = useState("");
	const [newName, setNewName] = useState("");
	const [newBus, setNewBus] = useState<EventBusChoice | undefined>(undefined);

	const save = (next: WorkloadEvents) => onEventsChange(compactEvents(next));

	const draft: EventSubscription = {
		...(matchKind === "exact"
			? { detail_type: newType.trim() }
			: { detail_type_prefix: newType.trim() }),
		...(newSource.trim() ? { source: newSource.trim() } : {}),
		...(newName.trim() ? { name: newName.trim() } : {}),
		...(newBus ? { bus: newBus } : {}),
	};
	const draftKey = newType.trim() ? subscriptionKey(draft, busEnabled) : "";
	const existingKeys = subscribes.map((s) => subscriptionKey(s, busEnabled));
	let draftError: string | null = null;
	if (newName.trim() && !SUBSCRIPTION_NAME_PATTERN.test(newName.trim())) {
		draftError =
			"A name is 1 to 32 lower-case letters, digits and hyphens, starting with a letter or a digit.";
	} else if (newType.trim() && draftKey === "") {
		draftError = "No key can be derived from this detail-type; give it a name.";
	} else if (draftKey && existingKeys.includes(draftKey)) {
		draftError = `Another subscription already has the key "${draftKey}"; give this one a distinct name.`;
	} else if (newBus === "project" && !busEnabled) {
		draftError = "Turn the project bus on in the EventBridge node first.";
	}
	const canAdd = newType.trim() !== "" && draftError === null;

	const handleAddSubscription = () => {
		if (!canAdd) return;
		save({ ...events, subscribes: [...subscribes, draft] });
		setNewType("");
		setNewSource("");
		setNewName("");
		setNewBus(undefined);
	};

	const handleRemoveSubscription = (index: number) => {
		save({ ...events, subscribes: subscribes.filter((_, i) => i !== index) });
	};

	const handleProduceChange = (index: number, entry: EventProduce) => {
		save({
			...events,
			produces: produces.map((p, i) => (i === index ? entry : p)),
		});
	};
	const handleProduceAdd = (entry: EventProduce) => {
		save({ ...events, produces: [...produces, entry] });
	};
	const handleProduceRemove = (index: number) => {
		save({ ...events, produces: produces.filter((_, i) => i !== index) });
	};

	// Queue settings are committed on blur, when the value is in range.
	const queue = events?.queue;
	const [visibility, setVisibility] = useState(
		queue?.visibility_timeout?.toString() ?? "",
	);
	const [maxReceive, setMaxReceive] = useState(
		queue?.max_receive_count?.toString() ?? "",
	);
	useEffect(() => {
		setVisibility(queue?.visibility_timeout?.toString() ?? "");
		setMaxReceive(queue?.max_receive_count?.toString() ?? "");
	}, [queue?.visibility_timeout, queue?.max_receive_count]);

	const rangeError = (text: string, min: number, max: number) => {
		if (text === "") return null;
		const n = Number(text);
		if (!Number.isInteger(n) || n < min || n > max)
			return `A whole number from ${min} to ${max}.`;
		return null;
	};
	const visibilityError = rangeError(visibility, 0, VISIBILITY_MAX);
	const maxReceiveError = rangeError(maxReceive, 1, MAX_RECEIVE_MAX);

	const commitQueue = () => {
		if (visibilityError || maxReceiveError) return;
		const next = {
			visibility_timeout: visibility === "" ? undefined : Number(visibility),
			max_receive_count: maxReceive === "" ? undefined : Number(maxReceive),
		};
		if (
			next.visibility_timeout === queue?.visibility_timeout &&
			next.max_receive_count === queue?.max_receive_count
		)
			return;
		save({ ...events, queue: next });
	};

	return (
		<div className="space-y-4" data-testid={`events-tab-${workloadName}`}>
			<Card>
				<CardHeader>
					<CardTitle className="flex items-center gap-2">
						<Send className="w-5 h-5" />
						Events
					</CardTitle>
					<CardDescription>
						Events reach this {kind === "backend" ? "backend" : "service"} on
						its own queue, shared by all its running copies, so each event is
						handled once. It may publish only what it lists under Produces.
					</CardDescription>
				</CardHeader>
				<CardContent className="space-y-2">
					<div className="flex flex-wrap items-center gap-2">
						<Badge variant={busEnabled ? "default" : "secondary"}>
							{busEnabled ? `Project bus ${busName}` : "Default bus"}
						</Badge>
						<Badge variant="code">EVENT_SOURCE {source}</Badge>
						{subscribes.length > 0 && (
							<Badge variant="code">EVENT_QUEUE_URL</Badge>
						)}
					</div>
					{!busEnabled && (
						<p className="text-xs text-muted-foreground">
							The project bus is off; entries without a bus use the default bus.
							Turn it on in the EventBridge node.
						</p>
					)}
				</CardContent>
			</Card>

			<Card>
				<CardHeader>
					<CardTitle className="flex items-center gap-2">
						<Inbox className="w-5 h-5" />
						Subscribes
					</CardTitle>
					<CardDescription>
						One rule per entry, delivering to this workload's queue.
					</CardDescription>
				</CardHeader>
				<CardContent className="space-y-4">
					{subscribes.length === 0 ? (
						<p className="text-sm text-muted-foreground">
							Subscribes to nothing, so it has no queue.
						</p>
					) : (
						<Table>
							<TableHeader>
								<TableRow>
									<TableHead>Detail-type</TableHead>
									<TableHead>Source</TableHead>
									<TableHead>Bus</TableHead>
									<TableHead>Key</TableHead>
									<TableHead>
										<span className="sr-only">Actions</span>
									</TableHead>
								</TableRow>
							</TableHeader>
							<TableBody>
								{subscribes.map((sub, index) => {
									const key = existingKeys[index];
									return (
										<TableRow
											key={key || index}
											data-testid={`subscription-${key}`}
										>
											<TableCell variant="code">
												{sub.detail_type ?? `${sub.detail_type_prefix}*`}
											</TableCell>
											<TableCell variant="code">
												{sub.source || "any"}
											</TableCell>
											<TableCell variant="compact">
												{resolvedBusLabel(sub.bus, busEnabled, busName)}
											</TableCell>
											<TableCell variant="code">{key}</TableCell>
											<TableCell>
												<Button
													variant="ghost"
													size="sm"
													aria-label={`Remove subscription ${key}`}
													onClick={() => handleRemoveSubscription(index)}
												>
													<Trash2 className="w-4 h-4" />
												</Button>
											</TableCell>
										</TableRow>
									);
								})}
							</TableBody>
						</Table>
					)}

					<div className="space-y-2">
						<Label htmlFor={`${uid}-type`}>New subscription</Label>
						<div className="grid grid-cols-2 gap-2">
							<Select
								value={matchKind}
								onValueChange={(v) => setMatchKind(v as MatchKind)}
							>
								<SelectTrigger aria-label="Match">
									<SelectValue />
								</SelectTrigger>
								<SelectContent>
									<SelectItem value="exact">Detail-type is</SelectItem>
									<SelectItem value="prefix">
										Detail-type starts with
									</SelectItem>
								</SelectContent>
							</Select>
							<Input
								id={`${uid}-type`}
								placeholder={
									matchKind === "exact" ? "payment.completed" : "user."
								}
								value={newType}
								onChange={(e) => setNewType(e.target.value)}
							/>
							<Input
								aria-label="Source"
								placeholder="Source (optional), e.g. example.payments"
								value={newSource}
								onChange={(e) => setNewSource(e.target.value)}
							/>
							<Input
								aria-label="Name"
								placeholder={
									draftKey ? `Name (optional): ${draftKey}` : "Name (optional)"
								}
								value={newName}
								onChange={(e) => setNewName(e.target.value)}
							/>
							<EventBusSelect
								ariaLabel="Bus for the new subscription"
								value={newBus}
								busEnabled={busEnabled}
								projectBusName={busName}
								onChange={setNewBus}
							/>
							<Button onClick={handleAddSubscription} disabled={!canAdd}>
								<Plus className="w-4 h-4" />
								Add
							</Button>
						</div>
						{draftError && <p className="text-sm text-danger">{draftError}</p>}
						<p className="text-xs text-muted-foreground">
							Events from AWS services are published to the default bus only.
						</p>
					</div>
				</CardContent>
			</Card>

			<Card>
				<CardHeader>
					<CardTitle className="flex items-center gap-2">
						<Send className="w-5 h-5" />
						Produces
					</CardTitle>
					<CardDescription>
						Each entry allows PutEvents for its detail-types on one bus.
					</CardDescription>
				</CardHeader>
				<CardContent>
					<EventProducesEditor
						produces={produces}
						busEnabled={busEnabled}
						projectBusName={busName}
						eventSource={source}
						onEntryChange={handleProduceChange}
						onEntryAdd={handleProduceAdd}
						onEntryRemove={handleProduceRemove}
					/>
				</CardContent>
			</Card>

			{subscribes.length > 0 && (
				<Card>
					<Collapsible>
						<CardHeader>
							<CollapsibleTrigger asChild>
								<Button variant="ghost" className="justify-between w-full">
									Advanced queue settings
									<ChevronDown className="w-4 h-4" />
								</Button>
							</CollapsibleTrigger>
						</CardHeader>
						<CollapsibleContent>
							<CardContent className="space-y-4">
								<div className="grid grid-cols-2 gap-4">
									<div className="space-y-1">
										<Label htmlFor={`${uid}-visibility`}>
											Visibility timeout (seconds)
										</Label>
										<Input
											id={`${uid}-visibility`}
											inputMode="numeric"
											placeholder="30"
											value={visibility}
											aria-invalid={visibilityError !== null}
											onChange={(e) => setVisibility(e.target.value.trim())}
											onBlur={commitQueue}
										/>
										{visibilityError && (
											<p className="text-sm text-danger">{visibilityError}</p>
										)}
									</div>
									<div className="space-y-1">
										<Label htmlFor={`${uid}-receives`}>
											Receives before the dead-letter queue
										</Label>
										<Input
											id={`${uid}-receives`}
											inputMode="numeric"
											placeholder="5"
											value={maxReceive}
											aria-invalid={maxReceiveError !== null}
											onChange={(e) => setMaxReceive(e.target.value.trim())}
											onBlur={commitQueue}
										/>
										{maxReceiveError && (
											<p className="text-sm text-danger">{maxReceiveError}</p>
										)}
									</div>
								</div>
								<p className="text-xs text-muted-foreground">
									Set the visibility timeout above the longest time one message
									takes to handle.
								</p>
							</CardContent>
						</CollapsibleContent>
					</Collapsible>
				</Card>
			)}

			{subscribes.length > 0 && (
				<Alert>
					<AlertDescription>
						Messages that fail go to a dead-letter queue, and an alarm publishes
						to the topic {config.project}-{config.env}-alerts. Nothing
						subscribes to that topic until you add a subscription; custom
						Terraform reads its ARN as local.bridge.alerts_topic_arn.
					</AlertDescription>
				</Alert>
			)}
		</div>
	);
}
