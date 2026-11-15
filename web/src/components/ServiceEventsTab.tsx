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
	eventSourceOf,
	filterFieldsNest,
	isCatchAllWildcard,
	isEventBusEnabled,
	nestedFilterFieldError,
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
} from "../utils/workloadEvents";
import { EventProducesEditor } from "./EventProducesEditor";
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
import { NoteDetail, StatusNote } from "./ui/status-note";

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

const VISIBILITY_MAX = 43200;
const MAX_RECEIVE_MAX = 1000;

interface SubscriptionRowEditorProps {
	sub: EventSubscription;
	index: number;
	allSubscribes: EventSubscription[];
	busEnabled: boolean;
	onUpdate: (index: number, patch: Partial<EventSubscription>) => void;
	onRemove: (index: number) => void;
}

/**
 * One saved subscription entry inside its bus section: event type (detail-type,
 * with `*` wildcard support), a collapsed-by-default filter (source + detail.*
 * field conditions), and a secondary Advanced toggle for the rule key name.
 */
function SubscriptionRowEditor({
	sub,
	index,
	allSubscribes,
	busEnabled,
	onUpdate,
	onRemove,
}: SubscriptionRowEditorProps) {
	const uid = useId();
	const key = subscriptionKey(sub, busEnabled);
	const displayType = subscriptionEventTypeDisplay(sub);
	const errors = subscriptionErrors(sub, index, allSubscribes, busEnabled);
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

	return (
		<div
			className="rounded-lg border p-3 space-y-2"
			data-testid={`subscription-${key || index}`}
		>
			<div className="flex flex-wrap items-center gap-2">
				<div className="flex-1 min-w-[180px]">
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
				</div>
				{sub.detail_type?.includes("*") && (
					<Badge variant="secondary">wildcard</Badge>
				)}
				{sub.detail_type_prefix !== undefined && (
					<Badge variant="outline">prefix</Badge>
				)}
				<Button
					type="button"
					variant="outline"
					size="sm"
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
				<Button
					type="button"
					variant="ghost"
					size="sm"
					aria-expanded={advancedOpen}
					aria-label={`Advanced settings for ${rowLabel}`}
					onClick={() => setAdvancedOpen((prev) => !prev)}
				>
					<span className="font-mono text-xs text-muted-foreground">
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

			{isCatchAllWildcard(typeDraft) && (
				<StatusNote tone="warning">
					Subscribes to every event on this bus. Narrow it with a prefix like
					job.* or add a filter unless this workload handles all events.
				</StatusNote>
			)}

			{errors.map((err) => (
				<StatusNote key={err} tone="error">
					{err}
				</StatusNote>
			))}

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
							<div className="flex-1 min-w-[160px]">
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
							<div className="flex-1 min-w-[160px]">
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
					<Label htmlFor={`${uid}-name`}>Rule key name (optional)</Label>
					<Input
						id={`${uid}-name`}
						aria-label={`Rule key name for ${rowLabel}`}
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
						Identifies this subscription's EventBridge rule. Leave blank to
						derive it from the event type ({key || "none"}).
					</NoteDetail>
				</div>
			)}
		</div>
	);
}

interface BusSubscriptionsSectionProps {
	title: string;
	description: string;
	testId: string;
	targetBus: EventBusChoice;
	busEnabled: boolean;
	allSubscribes: EventSubscription[];
	/** False hides the add form, leaving existing entries editable. */
	allowAdd?: boolean;
	onAdd: (entry: EventSubscription) => void;
	onUpdate: (index: number, patch: Partial<EventSubscription>) => void;
	onRemove: (index: number) => void;
}

/**
 * One event-bus section ("Environment bus" or "Default bus"): lists its
 * subscriptions and provides its own "Add subscription" form with an optional,
 * collapsed-by-default filter and secondary rule-key name.
 */
function BusSubscriptionsSection({
	title,
	description,
	testId,
	targetBus,
	busEnabled,
	allSubscribes,
	allowAdd = true,
	onAdd,
	onUpdate,
	onRemove,
}: BusSubscriptionsSectionProps) {
	const uid = useId();
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

	const sectionLabel =
		targetBus === "project" ? "environment bus" : "default bus";

	return (
		<div className="rounded-xl border p-4 space-y-4" data-testid={testId}>
			<div className="space-y-1">
				<h4 className="text-sm font-medium">{title}</h4>
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
							busEnabled={busEnabled}
							onUpdate={onUpdate}
							onRemove={onRemove}
						/>
					))}
				</div>
			)}

			{allowAdd && (
				<div className="space-y-2 pt-1">
					<Label htmlFor={`${uid}-type`}>Event type</Label>
					<div className="flex flex-wrap items-center gap-2">
						<div className="flex-1 min-w-[200px]">
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
							variant="ghost"
							size="sm"
							aria-expanded={advancedOpen}
							aria-label={`Advanced options for new subscription on ${sectionLabel}`}
							onClick={() => setAdvancedOpen((prev) => !prev)}
						>
							Advanced
						</Button>
						<Button
							type="button"
							aria-label={`Add subscription to ${sectionLabel}`}
							onClick={handleAdd}
							disabled={!canAdd}
						>
							<Plus className="w-4 h-4" />
							Add subscription
						</Button>
					</div>
					<NoteDetail>
						Use * as a wildcard: job.* matches every job event.
					</NoteDetail>

					{isCatchAllWildcard(newType) && (
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
									<div className="flex-1 min-w-[160px]">
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
									<div className="flex-1 min-w-[160px]">
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
							<Label htmlFor={`${uid}-name`}>Rule key name (optional)</Label>
							<Input
								id={`${uid}-name`}
								aria-label={`Name for new subscription on ${sectionLabel}`}
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

	const save = (next: WorkloadEvents) => onEventsChange(compactEvents(next));

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
						One rule per entry, delivering matching events to this workload's
						queue.
					</CardDescription>
				</CardHeader>
				<CardContent className="space-y-4">
					{subscribes.length === 0 && (
						<p className="text-sm text-muted-foreground">
							Subscribes to nothing, so it has no queue.
						</p>
					)}

					{(busEnabled || hasStrandedProjectEntries) && (
						<BusSubscriptionsSection
							title={`Environment bus (${busName})`}
							description={
								busEnabled
									? "Events published by your backend, services and tasks in this environment."
									: "The environment bus is off, so these subscriptions cannot be deployed. Turn the bus on in the EventBridge node, or remove them."
							}
							testId="subscribes-section-project"
							targetBus="project"
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
