import { Plus, Trash2 } from "lucide-react";
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
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "./ui/table";

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
			<SelectTrigger aria-label={ariaLabel}>
				<SelectValue />
			</SelectTrigger>
			<SelectContent>
				<SelectItem value="follow">
					Environment bus ({busEnabled ? projectBusName : "default"})
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

/**
 * The produces list: what a workload may publish. Each entry is one PutEvents
 * grant. Without an entry the workload cannot publish at all.
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
	const [newBus, setNewBus] = useState<EventBusChoice | undefined>(undefined);

	const parsed = parseDetailTypes(newTypes);
	const canAdd = newAny || parsed.length > 0;

	const handleAdd = () => {
		if (!canAdd) return;
		const entry: EventProduce = newAny
			? { any: true }
			: { detail_types: parsed };
		if (newBus) entry.bus = newBus;
		onEntryAdd(entry);
		setNewTypes("");
		setNewAny(false);
		setNewBus(undefined);
	};

	return (
		<div className="space-y-3" data-testid="event-produces-editor">
			{produces.length === 0 ? (
				<p className="text-sm text-muted-foreground">
					Publishes nothing. Add an entry to allow PutEvents.
				</p>
			) : (
				<Table>
					<TableHeader>
						<TableRow>
							<TableHead>Detail-types</TableHead>
							<TableHead>Own source only</TableHead>
							<TableHead>Bus</TableHead>
							<TableHead>
								<span className="sr-only">Actions</span>
							</TableHead>
						</TableRow>
					</TableHeader>
					<TableBody>
						{produces.map((entry, index) => (
							<TableRow
								// biome-ignore lint/suspicious/noArrayIndexKey: entries have no identity of their own
								key={index}
								data-testid={`produces-row-${index}`}
							>
								<TableCell variant="code">
									{entry.any ? (
										<Badge variant="destructive">any detail-type</Badge>
									) : (
										(entry.detail_types || []).join(", ")
									)}
								</TableCell>
								<TableCell>
									<Switch
										aria-label={`Own source only for entry ${index + 1}`}
										checked={entry.own_source_only !== false}
										onCheckedChange={(checked) =>
											onEntryChange(index, {
												...entry,
												own_source_only: checked ? undefined : false,
											})
										}
									/>
								</TableCell>
								<TableCell>
									<EventBusSelect
										ariaLabel={`Bus for entry ${index + 1}`}
										value={entry.bus}
										busEnabled={busEnabled}
										projectBusName={projectBusName}
										onChange={(bus) => onEntryChange(index, { ...entry, bus })}
									/>
								</TableCell>
								<TableCell>
									<Button
										variant="ghost"
										size="sm"
										aria-label={`Remove produces entry ${index + 1}`}
										onClick={() => onEntryRemove(index)}
									>
										<Trash2 className="w-4 h-4" />
									</Button>
								</TableCell>
							</TableRow>
						))}
					</TableBody>
				</Table>
			)}

			<p className="text-xs text-muted-foreground">
				With own source only, events must carry the source{" "}
				<span className="font-mono">{eventSource}</span>.
			</p>

			<div className="space-y-2">
				<Label htmlFor={`${uid}-types`}>New entry</Label>
				<div className="flex flex-wrap items-center gap-2">
					<Input
						id={`${uid}-types`}
						placeholder="order.created, order.cancelled"
						value={newTypes}
						disabled={newAny}
						onChange={(e) => setNewTypes(e.target.value)}
						onKeyDown={(e) => {
							if (e.key === "Enter") handleAdd();
						}}
					/>
					<div className="flex items-center gap-2">
						<Switch
							id={`${uid}-any`}
							checked={newAny}
							onCheckedChange={setNewAny}
						/>
						<Label htmlFor={`${uid}-any`}>Any detail-type</Label>
					</div>
					<EventBusSelect
						ariaLabel="Bus for the new entry"
						value={newBus}
						busEnabled={busEnabled}
						projectBusName={projectBusName}
						onChange={setNewBus}
					/>
					<Button onClick={handleAdd} disabled={!canAdd}>
						<Plus className="w-4 h-4" />
						Add
					</Button>
				</div>
				{newAny && (
					<p className="text-sm text-danger">
						Any detail-type lets the workload publish every event type to the
						bus; generate warns about it.
					</p>
				)}
			</div>
		</div>
	);
}
