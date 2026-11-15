import { Plus, Trash2, X } from "lucide-react";
import { useId, useState } from "react";
import type { EventBusChoice, EventProduce } from "../types/yamlConfig";
import { Badge } from "./ui/badge";
import { Button } from "./ui/button";
import { Input } from "./ui/input";
import { Label } from "./ui/label";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "./ui/select";
import { Switch } from "./ui/switch";

/** One column layout for produces rows and the add form, so they line up. */
const PRODUCES_GRID =
	"grid grid-cols-[minmax(0,1fr)_11rem_11rem] items-center gap-2";

/** The bus an entry or rule is on, as the select shows it. */
export type EventBusSelectValue = "follow" | EventBusChoice;

export function busSelectValue(bus: EventBusChoice | undefined) {
	return (bus ?? "follow") as EventBusSelectValue;
}

export function busFromSelect(
	value: EventBusSelectValue,
): EventBusChoice | undefined {
	return value === "follow" ? undefined : value;
}

/** What an entry's bus resolves to, as app/events.go resolves it. */
export function resolvedBusLabel(
	bus: EventBusChoice | undefined,
	busEnabled: boolean,
	projectBusName: string,
): string {
	if (bus === "default") return "default";
	if (bus === "project") return projectBusName;
	return busEnabled ? projectBusName : "default";
}

interface EventBusSelectProps {
	value: EventBusChoice | undefined;
	busEnabled: boolean;
	projectBusName: string;
	onChange: (bus: EventBusChoice | undefined) => void;
	ariaLabel: string;
}

/** A bus picker: follow the environment, the project bus, or the default bus. */
export function EventBusSelect({
	value,
	busEnabled,
	projectBusName,
	onChange,
	ariaLabel,
}: EventBusSelectProps) {
	return (
		<Select
			value={busSelectValue(value)}
			onValueChange={(v) => onChange(busFromSelect(v as EventBusSelectValue))}
		>
			<SelectTrigger aria-label={ariaLabel} className="font-mono">
				<SelectValue>
					{resolvedBusLabel(value, busEnabled, projectBusName)}
				</SelectValue>
			</SelectTrigger>
			<SelectContent>
				<SelectItem value="follow">
					Automatic ({busEnabled ? projectBusName : "default bus"})
				</SelectItem>
				<SelectItem value="project" disabled={!busEnabled}>
					Project bus ({projectBusName})
				</SelectItem>
				<SelectItem value="default">Default bus</SelectItem>
			</SelectContent>
		</Select>
	);
}

interface EventProducesEditorProps {
	produces: EventProduce[];
	busEnabled: boolean;
	projectBusName: string;
	/** The workload's EVENT_SOURCE, shown for own_source_only. */
	eventSource: string;
	/** Emits one entry's change; the owner maps it into the current list. */
	onEntryChange: (index: number, entry: EventProduce) => void;
	onEntryAdd: (entry: EventProduce) => void;
	onEntryRemove: (index: number) => void;
}

function parseDetailTypes(text: string): string[] {
	return text
		.split(",")
		.map((s) => s.trim())
		.filter((s) => s !== "");
}

/** A type made only of * matches everything; that is what "Any event" is for. */
function isLoneWildcard(type: string): boolean {
	return /^\*+$/.test(type);
}

/** Merges typed text into an entry's list: no duplicates, no lone *. */
function mergeTypes(current: string[], text: string): string[] {
	const out = [...current];
	for (const t of parseDetailTypes(text)) {
		if (!isLoneWildcard(t) && !out.includes(t)) out.push(t);
	}
	return out;
}

/** The source switch and its label: on means only the workload's own source. */
function SourceOnly({
	id,
	ariaLabel,
	checked,
	eventSource,
	onCheckedChange,
}: {
	id: string;
	ariaLabel: string;
	checked: boolean;
	eventSource: string;
	onCheckedChange: (checked: boolean) => void;
}) {
	return (
		<div className="flex min-w-0 items-center gap-2 text-xs">
			<Switch
				id={id}
				aria-label={ariaLabel}
				checked={checked}
				onCheckedChange={onCheckedChange}
			/>
			<Label
				htmlFor={id}
				className={`min-w-0 truncate text-xs font-normal ${checked ? "" : "text-muted-foreground"}`}
			>
				Source required{checked ? ":" : ""}
				{checked && <code className="ml-1 font-mono">{eventSource}</code>}
			</Label>
		</div>
	);
}

/** One entry: its event types as removable chips, plus an input to add more. */
function ProducesTypes({
	entry,
	index,
	onChange,
}: {
	entry: EventProduce;
	index: number;
	onChange: (entry: EventProduce) => void;
}) {
	const [draft, setDraft] = useState("");
	const types = entry.detail_types || [];

	if (entry.any) {
		return <Badge variant="warning">any event</Badge>;
	}

	const commit = () => {
		const next = mergeTypes(types, draft);
		setDraft("");
		if (next.length !== types.length) {
			onChange({ ...entry, detail_types: next });
		}
	};

	return (
		<div className="flex min-w-0 flex-wrap items-center gap-1.5">
			{types.map((t) => (
				<Badge
					key={t}
					variant={t.includes("*") ? "kms" : "code"}
					className="gap-1"
				>
					<span className="font-mono">{t}</span>
					{types.length > 1 && (
						<button
							type="button"
							aria-label={`Remove ${t} from entry ${index + 1}`}
							className="rounded-sm opacity-70 hover:opacity-100"
							onClick={() =>
								onChange({
									...entry,
									detail_types: types.filter((x) => x !== t),
								})
							}
						>
							<X className="h-3 w-3" />
						</button>
					)}
				</Badge>
			))}
			<input
				aria-label={`Add event type to entry ${index + 1}`}
				className="h-6 w-28 min-w-0 rounded-sm bg-transparent px-1 font-mono text-xs outline-none placeholder:text-muted-foreground focus:ring-1 focus:ring-ring"
				placeholder="+ add"
				value={draft}
				onChange={(e) => setDraft(e.target.value)}
				onBlur={commit}
				onKeyDown={(e) => {
					if (e.key === "Enter" || e.key === ",") {
						e.preventDefault();
						commit();
					}
				}}
			/>
		</div>
	);
}

/**
 * The produces list: what a workload may publish. Each entry is one PutEvents
 * grant on its task role. Without an entry the workload cannot publish at all.
 */
export function EventProducesEditor({
	produces,
	busEnabled,
	projectBusName,
	eventSource,
	onEntryChange,
	onEntryAdd,
	onEntryRemove,
}: EventProducesEditorProps) {
	const uid = useId();
	const [newTypes, setNewTypes] = useState("");
	const [newAny, setNewAny] = useState(false);
	const [newOwnOnly, setNewOwnOnly] = useState(false);
	const [newBus, setNewBus] = useState<EventBusChoice | undefined>(undefined);

	const parsed = parseDetailTypes(newTypes);
	const loneWildcard = !newAny && parsed.some(isLoneWildcard);
	const canAdd = newAny || (parsed.length > 0 && !loneWildcard);

	const handleAdd = () => {
		if (!canAdd) return;
		const entry: EventProduce = newAny
			? { any: true }
			: { detail_types: parsed };
		if (newBus) entry.bus = newBus;
		if (!newOwnOnly) entry.own_source_only = false;
		onEntryAdd(entry);
		setNewTypes("");
		setNewAny(false);
		setNewOwnOnly(false);
		setNewBus(undefined);
	};

	return (
		<div className="space-y-3" data-testid="event-produces-editor">
			<p className="text-sm text-muted-foreground">
				It can send only the events listed here.
			</p>
			{produces.length === 0 ? (
				<p className="text-sm text-muted-foreground">Sends nothing yet.</p>
			) : (
				<div className="space-y-2">
					<div
						className={`${PRODUCES_GRID} px-3 text-xs font-medium text-muted-foreground`}
						aria-hidden="true"
					>
						<span>Events it may send</span>
						<span>Bus</span>
						<span>Source</span>
					</div>
					{produces.map((entry, index) => (
						<div
							// biome-ignore lint/suspicious/noArrayIndexKey: entries have no identity of their own
							key={index}
							data-testid={`produces-row-${index}`}
							className={`${PRODUCES_GRID} rounded-lg border bg-card p-3 shadow-sm`}
						>
							<ProducesTypes
								entry={entry}
								index={index}
								onChange={(next) => onEntryChange(index, next)}
							/>
							<EventBusSelect
								ariaLabel={`Bus for entry ${index + 1}`}
								value={entry.bus}
								busEnabled={busEnabled}
								projectBusName={projectBusName}
								onChange={(bus) => onEntryChange(index, { ...entry, bus })}
							/>
							<div className="flex min-w-0 items-center justify-between gap-2">
								<SourceOnly
									id={`${uid}-own-${index}`}
									ariaLabel={`Own source only for entry ${index + 1}`}
									checked={entry.own_source_only !== false}
									eventSource={eventSource}
									onCheckedChange={(checked) =>
										onEntryChange(index, {
											...entry,
											own_source_only: checked ? undefined : false,
										})
									}
								/>
								<Button
									variant="ghost"
									size="sm"
									aria-label={`Remove produces entry ${index + 1}`}
									onClick={() => onEntryRemove(index)}
								>
									<Trash2 className="w-4 h-4" />
								</Button>
							</div>
						</div>
					))}
				</div>
			)}

			<div className="space-y-2 border-t pt-4">
				<Label htmlFor={`${uid}-types`}>New events</Label>
				<div className={PRODUCES_GRID}>
					<Input
						id={`${uid}-types`}
						aria-label="New entry"
						variant="code"
						placeholder={newAny ? "any event" : "order.created, order.*"}
						value={newAny ? "" : newTypes}
						disabled={newAny}
						aria-invalid={loneWildcard}
						onChange={(e) => setNewTypes(e.target.value)}
						onKeyDown={(e) => {
							if (e.key === "Enter") handleAdd();
						}}
					/>
					<EventBusSelect
						ariaLabel="Bus for the new entry"
						value={newBus}
						busEnabled={busEnabled}
						projectBusName={projectBusName}
						onChange={setNewBus}
					/>
					<Button className="w-full" onClick={handleAdd} disabled={!canAdd}>
						<Plus className="w-4 h-4" />
						Add
					</Button>
				</div>
				<div className="flex flex-wrap items-center gap-x-6 gap-y-2">
					<div className="flex items-center gap-2">
						<Switch
							id={`${uid}-any`}
							checked={newAny}
							onCheckedChange={setNewAny}
						/>
						<Label htmlFor={`${uid}-any`} className="text-xs font-normal">
							Any event
						</Label>
					</div>
					<SourceOnly
						id={`${uid}-new-own`}
						ariaLabel="Own source only for the new entry"
						checked={newOwnOnly}
						eventSource={eventSource}
						onCheckedChange={setNewOwnOnly}
					/>
					<span className="text-xs text-muted-foreground">
						* works as a wildcard: order.* is every order event.
					</span>
				</div>
				{loneWildcard && (
					<p className="text-sm text-danger">
						* alone matches every event. Turn on "Any event" instead.
					</p>
				)}
				{newAny && (
					<p className="text-sm text-danger">
						Lets it send every event to the bus. Generate warns about it.
					</p>
				)}
			</div>
		</div>
	);
}
