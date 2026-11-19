import {
	ChevronDown,
	ChevronRight,
	Filter,
	Inbox,
	Plus,
	Send,
	Trash2,
	X,
} from "lucide-react";
import { useEffect, useId, useState } from "react";
import type {
	EventBusChoice,
	EventProduce,
	EventSubscription,
	EventSubscriptionFilter,
	WorkloadEvents,
	YamlInfrastructureConfig,
} from "../types/yamlConfig";
import {
	compactEvents,
	enhancedDetailTypeError,
	enhancedDetailTypeNote,
	eventFlows,
	eventQueueName,
	eventSourceOf,
	eventsHaveOrdered,
	filterFieldsNest,
	isCatchAllWildcard,
	isEnhancedBus,
	isEventBusEnabled,
	nestedFilterFieldError,
	orderedSubscriptionError,
	parseFilterValues,
	projectBusName,
	resolveBus,
	SUBSCRIPTION_FILTER_FIELD_PATTERN,
	SUBSCRIPTION_MAX_FILTER_VALUES,
	SUBSCRIPTION_MAX_FILTERS,
	SUBSCRIPTION_NAME_PATTERN,
	subscriptionErrors,
	subscriptionEventTypeDisplay,
	subscriptionFilterSummary,
	subscriptionKey,
	withOrderingCleared,
} from "../utils/workloadEvents";
import { EventProducesEditor } from "./EventProducesEditor";
import { Alert, AlertDescription } from "./ui/alert";
import {
	AlertDialog,
	AlertDialogAction,
	AlertDialogCancel,
	AlertDialogContent,
	AlertDialogDescription,
	AlertDialogFooter,
	AlertDialogHeader,
	AlertDialogTitle,
} from "./ui/alert-dialog";
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
import { EventFlow } from "./ui/recipes/event-flow";
import { NoteDetail, StatusNote } from "./ui/status-note";
import { Switch } from "./ui/switch";

/** One column layout for subscription rows and the add form, so they line up. */
const SUBSCRIPTION_GRID = "grid grid-cols-subscription-row items-center gap-2";

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
	/**
	 * Whether this workload's ordered queue may already be deployed, so that
	 * turning its last ordered subscription off keeps it with fifo_retired.
	 * Defaults to true when the owner cannot tell.
	 */
	orderedQueueMayExist?: boolean;
}

const VISIBILITY_MAX = 43200;
const MAX_RECEIVE_MAX = 1000;

interface SubscriptionRowEditorProps {
	sub: EventSubscription;
	index: number;
	allSubscribes: EventSubscription[];
	config: YamlInfrastructureConfig;
	busEnabled: boolean;
	onUpdate: (index: number, patch: Partial<EventSubscription>) => void;
	onRemove: (index: number) => void;
}

/**
 * One saved subscription entry inside its bus section: event type (detail-type,
 * with `*` wildcard support), a collapsed-by-default filter (source + detail.*
 * field conditions), a secondary Advanced toggle for the rule key name, and,
 * on the enhanced project bus, the Keep order switch.
 */
function SubscriptionRowEditor({
	sub,
	index,
	allSubscribes,
	config,
	busEnabled,
	onUpdate,
	onRemove,
}: SubscriptionRowEditorProps) {
	const uid = useId();
	const key = subscriptionKey(sub, busEnabled);
	const displayType = subscriptionEventTypeDisplay(sub);
	const enhancedProject =
		isEnhancedBus(config) && resolveBus(sub.bus, busEnabled) === "project";
	// Shown whenever ordered is set, so an entry that cannot keep order can
	// always be turned back.
	const showKeepOrder = enhancedProject || sub.ordered === true;
	const orderedError = orderedSubscriptionError(sub, config);
	const errors = [
		...subscriptionErrors(sub, index, allSubscribes, busEnabled),
		...(orderedError ? [orderedError] : []),
	];
	const hasFilterError = errors.some(
		(e) => e.includes("detail.") || e.includes("filter") || e.includes("Field"),
	);
	const hasNameError = errors.some(
		(e) => e.includes("name") || e.includes("key"),
	);

	const [typeDraft, setTypeDraft] = useState(displayType);
	const [sourceDraft, setSourceDraft] = useState(sub.source ?? "");
	const [nameDraft, setNameDraft] = useState(sub.name ?? "");
	const [filterOpen, setFilterOpen] = useState(false);
	const [advancedOpen, setAdvancedOpen] = useState(
		Boolean(sub.name) || hasNameError,
	);
	const [newField, setNewField] = useState("");
	const [newValues, setNewValues] = useState("");

	useEffect(() => {
		setTypeDraft(displayType);
	}, [displayType]);

	useEffect(() => {
		setSourceDraft(sub.source ?? "");
	}, [sub.source]);

	useEffect(() => {
		setNameDraft(sub.name ?? "");
	}, [sub.name]);

	const commitType = () => {
		const trimmed = typeDraft.trim();
		if (trimmed === "" || trimmed === displayType) {
			if (trimmed === "") setTypeDraft(displayType);
			return;
		}
		onUpdate(index, {
			detail_type: trimmed,
			detail_type_prefix: undefined,
		});
	};

	const commitSource = () => {
		const trimmed = sourceDraft.trim();
		const current = sub.source ?? "";
		if (trimmed === current) return;
		onUpdate(index, {
			source: trimmed === "" ? undefined : trimmed,
		});
	};

	const commitName = () => {
		const trimmed = nameDraft.trim();
		const current = sub.name ?? "";
		if (trimmed === current) return;
		if (trimmed !== "" && !SUBSCRIPTION_NAME_PATTERN.test(trimmed)) return;
		onUpdate(index, {
			name: trimmed === "" ? undefined : trimmed,
		});
	};

	const filters = sub.filters ?? [];
	const parsedNewValues = parseFilterValues(newValues);
	const trimmedNewField = newField.trim();

	let newFilterError: string | null = null;
	if (
		trimmedNewField !== "" &&
		!SUBSCRIPTION_FILTER_FIELD_PATTERN.test(trimmedNewField)
	) {
		newFilterError =
			"Field path must start with detail. (e.g. detail.status or detail.order.type).";
	} else if (
		trimmedNewField !== "" &&
		filters.some((f) => f.field.trim() === trimmedNewField)
	) {
		newFilterError = `Field "${trimmedNewField}" is already filtered on this subscription.`;
	} else if (parsedNewValues.length > SUBSCRIPTION_MAX_FILTER_VALUES) {
		newFilterError = `Enter at most ${SUBSCRIPTION_MAX_FILTER_VALUES} values.`;
	} else if (filters.length >= SUBSCRIPTION_MAX_FILTERS) {
		newFilterError = `A subscription can have at most ${SUBSCRIPTION_MAX_FILTERS} field filters.`;
	}

	const canAddFilter =
		trimmedNewField !== "" &&
		parsedNewValues.length > 0 &&
		newFilterError === null &&
		filters.length < SUBSCRIPTION_MAX_FILTERS;

	const handleAddFilter = () => {
		if (!canAddFilter) return;
		const nextFilters: EventSubscriptionFilter[] = [
			...filters,
			{ field: trimmedNewField, values: parsedNewValues },
		];
		onUpdate(index, { filters: nextFilters });
		setNewField("");
		setNewValues("");
	};

	const handleRemoveFilter = (filterIndex: number) => {
		const nextFilters = filters.filter((_, fi) => fi !== filterIndex);
		onUpdate(index, {
			filters: nextFilters.length > 0 ? nextFilters : undefined,
		});
	};

	const handleRemoveFilterValue = (filterIndex: number, valueIndex: number) => {
		const target = filters[filterIndex];
		if (!target) return;
		const nextValues = target.values.filter((_, vi) => vi !== valueIndex);
		if (nextValues.length === 0) {
			handleRemoveFilter(filterIndex);
			return;
		}
		const nextFilters = filters.map((f, fi) =>
			fi === filterIndex ? { ...f, values: nextValues } : f,
		);
		onUpdate(index, { filters: nextFilters });
	};

	const nameInlineError =
		nameDraft.trim() !== "" && !SUBSCRIPTION_NAME_PATTERN.test(nameDraft.trim())
			? "A name is 1 to 32 lower-case letters, digits and hyphens, starting with a letter or a digit."
			: null;

	const summary = subscriptionFilterSummary(sub);
	const rowLabel = key || displayType || `subscription-${index + 1}`;
	const trimmedTypeDraft = typeDraft.trim();
	const enhancedTypeNote = enhancedDetailTypeNote(trimmedTypeDraft);
	const enhancedTypeError =
		trimmedTypeDraft === "" ? null : enhancedDetailTypeError(trimmedTypeDraft);

	return (
		<Card size="compact" data-testid={`subscription-${key || index}`}>
			<div className={SUBSCRIPTION_GRID}>
				<div className="flex min-w-0 items-center gap-2">
					<Input
						aria-label={`Event type for ${rowLabel}`}
						variant="code"
						value={typeDraft}
						placeholder="job.completed or job.*"
						onChange={(e) => setTypeDraft(e.target.value)}
						onBlur={commitType}
						onKeyDown={(e) => {
							if (e.key === "Enter") commitType();
						}}
					/>
					{sub.detail_type?.includes("*") && (
						<Badge variant="kms">wildcard</Badge>
					)}
					{sub.detail_type_prefix !== undefined && (
						<Badge variant="outline">prefix</Badge>
					)}
				</div>
				<Button
					type="button"
					variant="outline"
					size="sm"
					className="w-full justify-between"
					aria-expanded={filterOpen || hasFilterError}
					aria-label={`Filter for ${rowLabel}`}
					onClick={() => setFilterOpen((prev) => !prev)}
				>
					<Filter className="w-3.5 h-3.5" />
					{summary}
					{filterOpen || hasFilterError ? (
						<ChevronDown className="w-3.5 h-3.5" />
					) : (
						<ChevronRight className="w-3.5 h-3.5" />
					)}
				</Button>
				<div className="flex min-w-0 items-center justify-between gap-1">
					<Button
						type="button"
						variant="ghost"
						size="sm"
						className="min-w-0"
						aria-expanded={advancedOpen}
						aria-label={`Advanced settings for ${rowLabel}`}
						onClick={() => setAdvancedOpen((prev) => !prev)}
					>
						<span className="truncate font-mono text-xs text-muted-foreground">
							{key || "key"}
						</span>
					</Button>
					<Button
						type="button"
						variant="ghost"
						size="sm"
						aria-label={`Remove subscription ${key || rowLabel}`}
						onClick={() => onRemove(index)}
					>
						<Trash2 className="w-4 h-4" />
					</Button>
				</div>
			</div>

			{/* Compact filter chips preview when collapsed */}
			{!filterOpen && !hasFilterError && (sub.source || filters.length > 0) && (
				<div className="flex flex-wrap items-center gap-1.5">
					{sub.source && <Badge variant="code">source = {sub.source}</Badge>}
					{filters.map((f, fi) => (
						<Badge key={`${f.field}-${fi}`} variant="code">
							{f.field} = {(f.values ?? []).join(", ")}
						</Badge>
					))}
				</div>
			)}

			{isCatchAllWildcard(typeDraft) && !enhancedProject && (
				<StatusNote tone="warning">
					Subscribes to every event on this bus. Narrow it with a prefix like
					job.* or add a filter unless this workload handles all events.
				</StatusNote>
			)}

			{enhancedProject && enhancedTypeNote && (
				<NoteDetail>{enhancedTypeNote}</NoteDetail>
			)}
			{enhancedProject && enhancedTypeError && (
				<StatusNote tone="error">{enhancedTypeError}</StatusNote>
			)}

			{errors.map((err) => (
				<StatusNote key={err} tone="error">
					{err}
				</StatusNote>
			))}

			{showKeepOrder && (
				<div
					className="flex items-start justify-between gap-4"
					data-testid={`subscription-ordered-${key || index}`}
				>
					<div className="space-y-1">
						<Label htmlFor={`${uid}-ordered`}>Keep order</Label>
						<NoteDetail>
							Deliver events with the same group in the order they were sent.
							They go to a separate ordered queue, EVENT_FIFO_QUEUE_URL.
						</NoteDetail>
					</div>
					<Switch
						id={`${uid}-ordered`}
						aria-label={`Keep order for ${rowLabel}`}
						checked={sub.ordered === true}
						onCheckedChange={(checked) =>
							onUpdate(index, { ordered: checked ? true : undefined })
						}
					/>
				</div>
			)}

			{(filterOpen || hasFilterError) && (
				<div
					className="rounded-md border bg-muted/30 p-3 space-y-3"
					data-testid={`subscription-filter-${key || index}`}
				>
					<div className="space-y-1">
						<Label htmlFor={`${uid}-source`}>Source (optional)</Label>
						<Input
							id={`${uid}-source`}
							aria-label={`Source for ${rowLabel}`}
							variant="code"
							placeholder="any source, or e.g. example.payments"
							value={sourceDraft}
							onChange={(e) => setSourceDraft(e.target.value)}
							onBlur={commitSource}
							onKeyDown={(e) => {
								if (e.key === "Enter") commitSource();
							}}
						/>
						<NoteDetail>
							Leave blank to receive matching events from any publisher.
						</NoteDetail>
					</div>

					<div className="space-y-2">
						<Label>Event field conditions (optional)</Label>
						{filters.length > 0 && (
							<div className="space-y-1.5">
								{filters.map((f, fi) => (
									<div
										key={`${f.field}-${fi}`}
										className="flex flex-wrap items-center justify-between gap-2 rounded-md border bg-background px-2.5 py-1.5"
										data-testid={`filter-row-${f.field}`}
									>
										<div className="flex flex-wrap items-center gap-1.5">
											<Badge variant="code">{f.field}</Badge>
											<span className="text-xs text-muted-foreground">=</span>
											{(f.values ?? []).map((val, vi) => (
												<Badge
													// biome-ignore lint/suspicious/noArrayIndexKey: value chip
													key={`${val}-${vi}`}
													variant="secondary"
													className="font-mono gap-1"
												>
													{val}
													<button
														type="button"
														aria-label={`Remove value ${val} from ${f.field}`}
														onClick={() => handleRemoveFilterValue(fi, vi)}
													>
														<X className="w-3 h-3" />
													</button>
												</Badge>
											))}
										</div>
										<Button
											type="button"
											variant="ghost"
											size="sm"
											aria-label={`Remove filter ${f.field}`}
											onClick={() => handleRemoveFilter(fi)}
										>
											<Trash2 className="w-3.5 h-3.5" />
										</Button>
									</div>
								))}
							</div>
						)}

						<div className="flex flex-wrap items-start gap-2">
							<div className="flex-1 min-w-40">
								<Input
									aria-label={`Field path for ${rowLabel}`}
									variant="code"
									placeholder="detail.status"
									value={newField}
									aria-invalid={newFilterError !== null}
									onChange={(e) => setNewField(e.target.value)}
									onKeyDown={(e) => {
										if (e.key === "Enter") {
											e.preventDefault();
											handleAddFilter();
										}
									}}
								/>
							</div>
							<div className="flex-1 min-w-40">
								<Input
									aria-label={`Field values for ${rowLabel}`}
									variant="code"
									placeholder="done, failed"
									value={newValues}
									onChange={(e) => setNewValues(e.target.value)}
									onKeyDown={(e) => {
										if (e.key === "Enter") {
											e.preventDefault();
											handleAddFilter();
										}
									}}
								/>
							</div>
							<Button
								type="button"
								variant="outline"
								size="sm"
								aria-label={`Add filter to ${rowLabel}`}
								disabled={!canAddFilter}
								onClick={handleAddFilter}
							>
								<Plus className="w-4 h-4" />
								Add condition
							</Button>
						</div>
						{newFilterError && (
							<StatusNote tone="error">{newFilterError}</StatusNote>
						)}
						<NoteDetail>
							Match nested event payload fields (starting with{" "}
							<code className="font-mono">detail.</code>). Separate multiple
							allowed values with commas.
						</NoteDetail>
					</div>
				</div>
			)}

			{advancedOpen && (
				<div
					className="rounded-md border bg-muted/30 p-3 space-y-1"
					data-testid={`subscription-advanced-${key || index}`}
				>
					<Label htmlFor={`${uid}-name`}>
						{enhancedProject
							? "Subscription key name (optional)"
							: "Rule key name (optional)"}
					</Label>
					<Input
						id={`${uid}-name`}
						aria-label={`${enhancedProject ? "Subscription" : "Rule"} key name for ${rowLabel}`}
						variant="code"
						placeholder={key || "custom-key"}
						value={nameDraft}
						aria-invalid={nameInlineError !== null}
						onChange={(e) => setNameDraft(e.target.value)}
						onBlur={commitName}
						onKeyDown={(e) => {
							if (e.key === "Enter") commitName();
						}}
					/>
					{nameInlineError && (
						<StatusNote tone="error">{nameInlineError}</StatusNote>
					)}
					<NoteDetail>
						Identifies this subscription's EventBridge{" "}
						{enhancedProject ? "subscriber" : "rule"}. Leave blank to derive it
						from the event type ({key || "none"}).
					</NoteDetail>
				</div>
			)}
		</Card>
	);
}

interface BusSubscriptionsSectionProps {
	title: string;
	description: string;
	testId: string;
	targetBus: EventBusChoice;
	config: YamlInfrastructureConfig;
	busEnabled: boolean;
	allSubscribes: EventSubscription[];
	/** False hides the add form, leaving existing entries editable. */
	allowAdd?: boolean;
	onAdd: (entry: EventSubscription) => void;
	onUpdate: (index: number, patch: Partial<EventSubscription>) => void;
	onRemove: (index: number) => void;
}

/**
 * One event-bus section ("Project bus" or "Default bus"): lists its
 * subscriptions and provides its own "Add subscription" form with an optional,
 * collapsed-by-default filter and secondary rule-key name.
 */
function BusSubscriptionsSection({
	title,
	description,
	testId,
	targetBus,
	config,
	busEnabled,
	allSubscribes,
	allowAdd = true,
	onAdd,
	onUpdate,
	onRemove,
}: BusSubscriptionsSectionProps) {
	const uid = useId();
	// Subscriber filters of an enhanced bus match a fixed start or end only.
	const enhancedProject = targetBus === "project" && isEnhancedBus(config);
	const items = allSubscribes
		.map((sub, index) => ({ sub, index }))
		.filter(({ sub }) => resolveBus(sub.bus, busEnabled) === targetBus);

	const [newType, setNewType] = useState("");
	const [filterOpen, setFilterOpen] = useState(false);
	const [newSource, setNewSource] = useState("");
	const [draftFilters, setDraftFilters] = useState<EventSubscriptionFilter[]>(
		[],
	);
	const [filterField, setFilterField] = useState("");
	const [filterValues, setFilterValues] = useState("");
	const [advancedOpen, setAdvancedOpen] = useState(false);
	const [newName, setNewName] = useState("");

	const trimmedType = newType.trim();
	const trimmedSource = newSource.trim();
	const trimmedName = newName.trim();
	const trimmedFilterField = filterField.trim();
	const parsedFilterValues = parseFilterValues(filterValues);

	// Include any half-typed pending filter row if both field and values are filled
	let pendingFilterError: string | null = null;
	if (trimmedFilterField !== "" || filterValues.trim() !== "") {
		if (trimmedFilterField === "") {
			pendingFilterError =
				"Enter a field path starting with detail., such as detail.status.";
		} else if (!SUBSCRIPTION_FILTER_FIELD_PATTERN.test(trimmedFilterField)) {
			pendingFilterError =
				"Field path must start with detail. (e.g. detail.status or detail.order.type).";
		} else if (
			draftFilters.some((f) => f.field.trim() === trimmedFilterField)
		) {
			pendingFilterError = `Field "${trimmedFilterField}" is already in the filter list.`;
		} else if (
			draftFilters.some((f) =>
				filterFieldsNest(f.field.trim(), trimmedFilterField),
			)
		) {
			const other = draftFilters.find((f) =>
				filterFieldsNest(f.field.trim(), trimmedFilterField),
			);
			pendingFilterError = nestedFilterFieldError(
				trimmedFilterField,
				other?.field.trim() ?? "",
			);
		} else if (parsedFilterValues.length === 0) {
			pendingFilterError = `Enter at least one value for ${trimmedFilterField}.`;
		} else if (parsedFilterValues.length > SUBSCRIPTION_MAX_FILTER_VALUES) {
			pendingFilterError = `Enter at most ${SUBSCRIPTION_MAX_FILTER_VALUES} values.`;
		} else if (draftFilters.length >= SUBSCRIPTION_MAX_FILTERS) {
			pendingFilterError = `A subscription can have at most ${SUBSCRIPTION_MAX_FILTERS} field filters.`;
		}
	}

	const effectiveFilters: EventSubscriptionFilter[] =
		trimmedFilterField !== "" &&
		parsedFilterValues.length > 0 &&
		pendingFilterError === null
			? [
					...draftFilters,
					{ field: trimmedFilterField, values: parsedFilterValues },
				]
			: draftFilters;

	const busField: EventBusChoice | undefined =
		targetBus === "default" ? (busEnabled ? "default" : undefined) : undefined;

	const draft: EventSubscription = {
		detail_type: trimmedType,
		...(trimmedSource ? { source: trimmedSource } : {}),
		...(effectiveFilters.length > 0 ? { filters: effectiveFilters } : {}),
		...(trimmedName ? { name: trimmedName } : {}),
		...(busField ? { bus: busField } : {}),
	};

	const draftKey = trimmedType ? subscriptionKey(draft, busEnabled) : "";
	const existingKeys = allSubscribes.map((s) => subscriptionKey(s, busEnabled));

	let draftError: string | null = null;
	if (trimmedName && !SUBSCRIPTION_NAME_PATTERN.test(trimmedName)) {
		draftError =
			"A name is 1 to 32 lower-case letters, digits and hyphens, starting with a letter or a digit.";
	} else if (pendingFilterError) {
		draftError = pendingFilterError;
	} else if (enhancedProject && enhancedDetailTypeError(trimmedType)) {
		draftError = enhancedDetailTypeError(trimmedType);
	} else if (trimmedType && draftKey === "") {
		draftError =
			"No key can be derived from this event type; open Advanced and give it a name.";
	} else if (draftKey && existingKeys.includes(draftKey)) {
		draftError = `Another subscription already has the key "${draftKey}"; open Advanced and give this one a distinct name.`;
	}

	const canAdd = trimmedType !== "" && draftError === null;

	const canAddDraftFilter =
		trimmedFilterField !== "" &&
		parsedFilterValues.length > 0 &&
		pendingFilterError === null &&
		draftFilters.length < SUBSCRIPTION_MAX_FILTERS;

	const handleAddDraftFilter = () => {
		if (!canAddDraftFilter) return;
		setDraftFilters([
			...draftFilters,
			{ field: trimmedFilterField, values: parsedFilterValues },
		]);
		setFilterField("");
		setFilterValues("");
	};

	const handleRemoveDraftFilter = (fi: number) => {
		setDraftFilters(draftFilters.filter((_, i) => i !== fi));
	};

	const handleAdd = () => {
		if (!canAdd) return;
		onAdd(draft);
		setNewType("");
		setNewSource("");
		setDraftFilters([]);
		setFilterField("");
		setFilterValues("");
		setNewName("");
		setFilterOpen(false);
		setAdvancedOpen(false);
	};

	const draftFilterCount = (trimmedSource ? 1 : 0) + effectiveFilters.length;
	const draftFilterLabel =
		draftFilterCount === 0
			? "Filter (optional)"
			: draftFilterCount === 1
				? "1 filter"
				: `${draftFilterCount} filters`;

	const sectionLabel = targetBus === "project" ? "project bus" : "default bus";

	return (
		<div
			className="rounded-xl border-2 bg-muted/40 p-4 space-y-4"
			data-testid={testId}
		>
			<div className="space-y-1">
				<h4 className="text-base font-semibold">{title}</h4>
				<p className="text-xs text-muted-foreground">{description}</p>
			</div>

			{items.length === 0 ? (
				<NoteDetail>No subscriptions on this bus yet.</NoteDetail>
			) : (
				<div className="space-y-2">
					{items.map(({ sub, index }) => (
						<SubscriptionRowEditor
							key={subscriptionKey(sub, busEnabled) || index}
							sub={sub}
							index={index}
							allSubscribes={allSubscribes}
							config={config}
							busEnabled={busEnabled}
							onUpdate={onUpdate}
							onRemove={onRemove}
						/>
					))}
				</div>
			)}

			{allowAdd && (
				<div className="space-y-2 border-t pt-4">
					<Label htmlFor={`${uid}-type`}>New subscription</Label>
					<div className={SUBSCRIPTION_GRID}>
						<div className="min-w-0">
							<Input
								id={`${uid}-type`}
								aria-label={`Event type for ${sectionLabel}`}
								variant="code"
								placeholder={
									targetBus === "default"
										? "ECS Task State Change or job.*"
										: "job.completed or job.*"
								}
								value={newType}
								aria-invalid={draftError !== null}
								onChange={(e) => setNewType(e.target.value)}
								onKeyDown={(e) => {
									if (e.key === "Enter") {
										e.preventDefault();
										handleAdd();
									}
								}}
							/>
						</div>
						<Button
							type="button"
							variant="outline"
							size="sm"
							className="w-full justify-between"
							aria-expanded={filterOpen}
							aria-label={`Filter new subscription on ${sectionLabel}`}
							onClick={() => setFilterOpen((prev) => !prev)}
						>
							<Filter className="w-3.5 h-3.5" />
							{draftFilterLabel}
							{filterOpen ? (
								<ChevronDown className="w-3.5 h-3.5" />
							) : (
								<ChevronRight className="w-3.5 h-3.5" />
							)}
						</Button>
						<Button
							type="button"
							className="w-full"
							aria-label={`Add subscription to ${sectionLabel}`}
							onClick={handleAdd}
							disabled={!canAdd}
						>
							<Plus className="w-4 h-4" />
							Add
						</Button>
					</div>
					<div className="flex items-center justify-between gap-2">
						<NoteDetail>
							{enhancedProject
								? "Use * at the start or the end: job.* matches every job event, *.created every created event."
								: "Use * as a wildcard: job.* matches every job event."}
						</NoteDetail>
						<Button
							type="button"
							variant="link"
							size="sm"
							className="h-auto p-0 text-xs"
							aria-expanded={advancedOpen}
							aria-label={`Advanced options for new subscription on ${sectionLabel}`}
							onClick={() => setAdvancedOpen((prev) => !prev)}
						>
							Advanced
						</Button>
					</div>

					{isCatchAllWildcard(newType) && !enhancedProject && (
						<StatusNote tone="warning">
							Subscribes to every event on this bus. Narrow it with a prefix
							like job.* or add a filter unless this workload handles all
							events.
						</StatusNote>
					)}

					{filterOpen && (
						<div
							className="rounded-md border bg-muted/30 p-3 space-y-3"
							data-testid={`new-filter-${targetBus}`}
						>
							<div className="space-y-1">
								<Label htmlFor={`${uid}-source`}>Source (optional)</Label>
								<Input
									id={`${uid}-source`}
									aria-label={`Source for new subscription on ${sectionLabel}`}
									variant="code"
									placeholder={
										targetBus === "default"
											? "aws.ecs or example.payments"
											: "example.payments"
									}
									value={newSource}
									onChange={(e) => setNewSource(e.target.value)}
								/>
								<NoteDetail>
									Only receive events published with this exact source.
								</NoteDetail>
							</div>

							<div className="space-y-2">
								<Label>Event field conditions (optional)</Label>
								{draftFilters.length > 0 && (
									<div className="space-y-1.5">
										{draftFilters.map((f, fi) => (
											<div
												key={`${f.field}-${fi}`}
												className="flex flex-wrap items-center justify-between gap-2 rounded-md border bg-background px-2.5 py-1.5"
											>
												<div className="flex flex-wrap items-center gap-1.5">
													<Badge variant="code">{f.field}</Badge>
													<span className="text-xs text-muted-foreground">
														=
													</span>
													{f.values.map((v) => (
														<Badge
															key={v}
															variant="secondary"
															className="font-mono"
														>
															{v}
														</Badge>
													))}
												</div>
												<Button
													type="button"
													variant="ghost"
													size="sm"
													aria-label={`Remove draft filter ${f.field}`}
													onClick={() => handleRemoveDraftFilter(fi)}
												>
													<Trash2 className="w-3.5 h-3.5" />
												</Button>
											</div>
										))}
									</div>
								)}
								<div className="flex flex-wrap items-start gap-2">
									<div className="flex-1 min-w-40">
										<Input
											aria-label={`Filter field for new subscription on ${sectionLabel}`}
											variant="code"
											placeholder="detail.status"
											value={filterField}
											aria-invalid={pendingFilterError !== null}
											onChange={(e) => setFilterField(e.target.value)}
											onKeyDown={(e) => {
												if (e.key === "Enter") {
													e.preventDefault();
													handleAddDraftFilter();
												}
											}}
										/>
									</div>
									<div className="flex-1 min-w-40">
										<Input
											aria-label={`Filter values for new subscription on ${sectionLabel}`}
											variant="code"
											placeholder="done, failed"
											value={filterValues}
											onChange={(e) => setFilterValues(e.target.value)}
											onKeyDown={(e) => {
												if (e.key === "Enter") {
													e.preventDefault();
													handleAddDraftFilter();
												}
											}}
										/>
									</div>
									<Button
										type="button"
										variant="outline"
										size="sm"
										aria-label={`Add filter condition on ${sectionLabel}`}
										disabled={!canAddDraftFilter}
										onClick={handleAddDraftFilter}
									>
										<Plus className="w-4 h-4" />
										Add condition
									</Button>
								</div>
								<NoteDetail>
									Filter by payload fields starting with{" "}
									<code className="font-mono">detail.</code> (e.g.{" "}
									<code className="font-mono">detail.status</code> ={" "}
									<code className="font-mono">done, failed</code>).
								</NoteDetail>
							</div>
						</div>
					)}

					{advancedOpen && (
						<div
							className="rounded-md border bg-muted/30 p-3 space-y-1"
							data-testid={`new-advanced-${targetBus}`}
						>
							<Label htmlFor={`${uid}-name`}>
								{enhancedProject
									? "Subscription key name (optional)"
									: "Rule key name (optional)"}
							</Label>
							<Input
								id={`${uid}-name`}
								aria-label={`${enhancedProject ? "Subscription" : "Rule"} key name for new subscription on ${sectionLabel}`}
								variant="code"
								placeholder={draftKey ? `Derived: ${draftKey}` : "custom-key"}
								value={newName}
								onChange={(e) => setNewName(e.target.value)}
							/>
							<NoteDetail>
								Set a custom key when two subscriptions would otherwise derive
								the same rule key.
							</NoteDetail>
						</div>
					)}

					{enhancedProject && enhancedDetailTypeNote(trimmedType) && (
						<NoteDetail>{enhancedDetailTypeNote(trimmedType)}</NoteDetail>
					)}
					{draftError && <StatusNote tone="error">{draftError}</StatusNote>}
				</div>
			)}
		</div>
	);
}

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
	orderedQueueMayExist = true,
}: ServiceEventsTabProps) {
	const uid = useId();
	const busEnabled = isEventBusEnabled(config);
	const busName = projectBusName(config);
	const subscribes = events?.subscribes || [];
	const produces = events?.produces || [];
	const source = eventSourceOf(config.project, kind, workloadName);
	// Entries naming the project bus while it is off must stay visible to be fixed.
	const hasStrandedProjectEntries =
		!busEnabled && subscribes.some((sub) => sub.bus === "project");
	const enhanced = isEnhancedBus(config);
	const hasOrdered = eventsHaveOrdered(events);
	const fifoRetired = events?.queue?.fifo_retired === true;
	// A retired ordered queue keeps the workload a consumer with its queues.
	const isConsumer = subscribes.length > 0 || fifoRetired;
	const hasFifo = hasOrdered || fifoRetired;
	const workloadNoun = kind === "backend" ? "backend" : "service";
	const [confirmFifoDelete, setConfirmFifoDelete] = useState(false);

	// Every edit goes through withOrderingCleared, so none leaves an ordered
	// queue that may be deployed without fifo_retired, or fifo_retired beside
	// an ordered entry.
	const save = (next: WorkloadEvents) =>
		onEventsChange(
			compactEvents(withOrderingCleared(events, next, orderedQueueMayExist)),
		);

	// Writes go through index + patch against the current subscribes list.
	const handleAddSubscription = (entry: EventSubscription) => {
		save({ ...events, subscribes: [...subscribes, entry] });
	};

	const handleUpdateSubscription = (
		index: number,
		patch: Partial<EventSubscription>,
	) => {
		save({
			...events,
			subscribes: subscribes.map((sub, i) => {
				if (i !== index) return sub;
				const merged: EventSubscription = { ...sub, ...patch };
				const cleaned: EventSubscription = {};
				if (merged.name !== undefined && merged.name !== "")
					cleaned.name = merged.name;
				if (merged.detail_type !== undefined && merged.detail_type !== "")
					cleaned.detail_type = merged.detail_type;
				if (
					merged.detail_type_prefix !== undefined &&
					merged.detail_type_prefix !== ""
				)
					cleaned.detail_type_prefix = merged.detail_type_prefix;
				if (merged.source !== undefined && merged.source !== "")
					cleaned.source = merged.source;
				if (merged.filters !== undefined && merged.filters.length > 0)
					cleaned.filters = merged.filters;
				if (merged.bus !== undefined) cleaned.bus = merged.bus;
				if (merged.ordered === true) cleaned.ordered = true;
				return cleaned;
			}),
		});
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
		// Only the two fields this card edits; fifo_retired is kept as it is.
		const next = {
			...queue,
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

	const deleteRetiredFifo = () =>
		save({ ...events, queue: { ...queue, fifo_retired: undefined } });

	return (
		<div className="space-y-4" data-testid={`events-tab-${workloadName}`}>
			<Card>
				<CardHeader>
					<CardTitle className="flex items-center gap-2">
						<Send className="w-5 h-5" />
						Events
					</CardTitle>
					<CardDescription>
						Pick the events this {kind === "backend" ? "backend" : "service"}{" "}
						receives and the events it may send. Each event is handled once,
						however many copies run.
					</CardDescription>
				</CardHeader>
				<CardContent>
					<dl className="grid grid-cols-label-value items-center gap-x-3 gap-y-2 text-sm">
						<dt className="text-muted-foreground">Bus</dt>
						<dd>
							<code className="font-mono">
								{busEnabled ? busName : "default"}
							</code>
							{enhanced && (
								<Badge variant="outline" className="ml-2">
									Enhanced
								</Badge>
							)}
						</dd>
						<dt className="text-muted-foreground">Env vars</dt>
						<dd className="flex flex-wrap gap-2">
							<Badge variant="code">EVENT_SOURCE={source}</Badge>
							<Badge variant="code">
								EVENT_BUS_NAME={busEnabled ? busName : "default"}
							</Badge>
							{enhanced && <Badge variant="code">EVENT_BUS_ARN</Badge>}
							{isConsumer && (
								<>
									<Badge variant="code">EVENT_QUEUE_URL</Badge>
									<Badge variant="code">EVENT_DLQ_URL</Badge>
								</>
							)}
							{hasFifo && (
								<>
									<Badge variant="code">EVENT_FIFO_QUEUE_URL</Badge>
									<Badge variant="code">EVENT_FIFO_DLQ_URL</Badge>
								</>
							)}
						</dd>
					</dl>
					{isConsumer && (
						<div className="mt-4 space-y-4">
							{eventFlows(config, workloadName, events).map((flow) => (
								<EventFlow key={flow["data-testid"]} {...flow} />
							))}
						</div>
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
						{enhanced
							? `Each entry gets its own subscriber; this ${workloadNoun}'s ordered entries share one ordered subscriber. Default-bus entries get one rule each.`
							: "One rule per entry, delivering matching events to this workload's queue."}
					</CardDescription>
				</CardHeader>
				<CardContent className="space-y-4">
					{subscribes.length === 0 && !fifoRetired && (
						<p className="text-sm text-muted-foreground">
							Subscribes to nothing, so it has no queue.
						</p>
					)}
					{subscribes.length === 0 && fifoRetired && (
						<p className="text-sm text-muted-foreground">
							Subscribes to nothing. Its queues stay until you delete the
							retired ordered queue in Queue settings.
						</p>
					)}
					{fifoRetired && !hasOrdered && (
						<StatusNote tone="warning">
							The ordered queue is kept, with any events still in it. Delete it
							in Queue settings once your {workloadNoun} has read them.
						</StatusNote>
					)}

					{(busEnabled || hasStrandedProjectEntries) && (
						<BusSubscriptionsSection
							title={`Project bus (${busName})`}
							description={
								busEnabled
									? "Events published by your backend, services and tasks in this environment."
									: "The project bus is off, so these subscriptions cannot be deployed. Turn the bus on in the EventBridge node, or remove them."
							}
							testId="subscribes-section-project"
							targetBus="project"
							config={config}
							busEnabled={busEnabled}
							allSubscribes={subscribes}
							allowAdd={busEnabled}
							onAdd={handleAddSubscription}
							onUpdate={handleUpdateSubscription}
							onRemove={handleRemoveSubscription}
						/>
					)}

					<BusSubscriptionsSection
						title="Default bus (AWS services, CI)"
						description="AWS services (such as ECS, ECR and S3) and CI events publish only to the default bus."
						testId="subscribes-section-default"
						targetBus="default"
						config={config}
						busEnabled={busEnabled}
						allSubscribes={subscribes}
						onAdd={handleAddSubscription}
						onUpdate={handleUpdateSubscription}
						onRemove={handleRemoveSubscription}
					/>
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

			{isConsumer && (
				<Card data-testid="queue-settings">
					{fifoRetired && (
						<CardHeader data-testid="fifo-retired">
							<CardTitle>Ordered queue (retired)</CardTitle>
							<CardDescription>
								No subscription delivers to{" "}
								<code className="font-mono">
									{eventQueueName(config, workloadName, "queue", true) ??
										"the ordered queue"}
								</code>{" "}
								any more. It is kept, with its dead-letter queues, so your{" "}
								{workloadNoun} can read what is still in it through
								EVENT_FIFO_QUEUE_URL.
							</CardDescription>
							<div>
								<Button
									type="button"
									variant="destructive"
									size="sm"
									onClick={() => setConfirmFifoDelete(true)}
								>
									<Trash2 className="w-4 h-4" />
									Delete the ordered queue
								</Button>
							</div>
						</CardHeader>
					)}
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

			{isConsumer && (
				<Alert>
					<AlertDescription>
						Messages that fail go to a dead-letter queue, and an alarm publishes
						to the topic {config.project}-{config.env}-alerts. Nothing
						subscribes to that topic until you add a subscription; custom
						Terraform reads its ARN as local.bridge.alerts_topic_arn.
					</AlertDescription>
				</Alert>
			)}

			<AlertDialog open={confirmFifoDelete} onOpenChange={setConfirmFifoDelete}>
				<AlertDialogContent>
					<AlertDialogHeader>
						<AlertDialogTitle>Delete the ordered queue?</AlertDialogTitle>
						<AlertDialogDescription>
							The next deploy deletes the ordered queue and its dead-letter
							queues. Deleting it also deletes any events still in it.
						</AlertDialogDescription>
					</AlertDialogHeader>
					{subscribes.length === 0 && (
						<StatusNote tone="warning">
							This {workloadNoun} has no subscriptions left, so its standard
							queue is deleted as well.
						</StatusNote>
					)}
					<AlertDialogFooter>
						<AlertDialogCancel>Cancel</AlertDialogCancel>
						<AlertDialogAction
							variant="destructive"
							onClick={() => {
								setConfirmFifoDelete(false);
								deleteRetiredFifo();
							}}
						>
							Delete the ordered queue
						</AlertDialogAction>
					</AlertDialogFooter>
				</AlertDialogContent>
			</AlertDialog>
		</div>
	);
}
