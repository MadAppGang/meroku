import {
	AlertCircle,
	Check,
	CheckCircle2,
	ChevronDown,
	Clock3,
	Copy,
	Eye,
	EyeOff,
	Loader2,
	Lock,
	Plus,
	Trash2,
	X,
} from "lucide-react";
import {
	type ChangeEvent,
	type ReactNode,
	useEffect,
	useMemo,
	useState,
} from "react";
import { Button } from "../ui/button";
import {
	Collapsible,
	CollapsibleContent,
	CollapsibleTrigger,
} from "../ui/collapsible";
import { Input } from "../ui/input";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "../ui/select";
import { Switch } from "../ui/switch";
import { Textarea } from "../ui/textarea";
import "./property-panel.css";
import type {
	PropertyFieldDefinition,
	PropertyKeyValue,
	PropertyPanelDefinition,
	PropertySaveState,
	PropertySectionDefinition,
	PropertyStatusTone,
} from "./types";

type PropertyValue = PropertyFieldDefinition["value"];

interface PropertyPanelProps {
	definition: PropertyPanelDefinition;
	initialView?: string;
	saveState?: PropertySaveState;
	openAdvanced?: boolean;
	onClose?: () => void;
	onDelete?: () => void;
	onDiscard?: () => void;
	onApply?: (values: Record<string, PropertyValue>) => void | Promise<void>;
}

interface PropertySectionProps {
	section: PropertySectionDefinition;
	values: Record<string, PropertyValue>;
	onChange: (fieldId: string, value: PropertyValue) => void;
}

interface PropertyFieldProps {
	field: PropertyFieldDefinition;
	value: PropertyValue;
	onChange: (value: PropertyValue) => void;
}

interface PropertyKeyValueEditorProps {
	rows: PropertyKeyValue[];
	disabled?: boolean;
	onChange: (rows: PropertyKeyValue[]) => void;
}

interface PropertySaveBarProps {
	state: PropertySaveState;
	lastUpdated?: string;
	onDiscard: () => void;
	onApply: () => void;
}

const cloneValue = (value: PropertyValue): PropertyValue => {
	if (Array.isArray(value)) {
		return value.map((item) =>
			typeof item === "object" && item !== null ? { ...item } : item,
		) as PropertyValue;
	}
	return value;
};

function getInitialValues(
	definition: PropertyPanelDefinition,
): Record<string, PropertyValue> {
	const values: Record<string, PropertyValue> = {};
	for (const view of definition.views) {
		for (const section of view.sections) {
			for (const field of section.fields) {
				values[field.id] = cloneValue(field.value);
			}
		}
	}
	return values;
}

export function PropertyStatus({
	label,
	tone = "neutral",
}: {
	label: string;
	tone?: PropertyStatusTone;
}) {
	return (
		<span className="mp-status" data-tone={tone}>
			<span className="mp-status__dot" aria-hidden="true" />
			<span>{label}</span>
		</span>
	);
}

export function PropertyPanel({
	definition,
	initialView,
	saveState,
	openAdvanced = false,
	onClose,
	onDelete,
	onDiscard,
	onApply,
}: PropertyPanelProps) {
	const initialValues = useMemo(
		() => getInitialValues(definition),
		[definition],
	);
	const [values, setValues] =
		useState<Record<string, PropertyValue>>(initialValues);
	const [activeView, setActiveView] = useState(
		initialView ?? definition.views[0]?.id ?? "setup",
	);
	const [internalSaveState, setInternalSaveState] =
		useState<PropertySaveState>("clean");

	useEffect(() => {
		setValues(initialValues);
		setActiveView(initialView ?? definition.views[0]?.id ?? "setup");
		setInternalSaveState("clean");
	}, [definition, initialValues, initialView]);

	const resolvedSaveState = saveState ?? internalSaveState;
	const view =
		definition.views.find((candidate) => candidate.id === activeView) ??
		definition.views[0];

	const handleFieldChange = (fieldId: string, value: PropertyValue) => {
		setValues((current) => ({ ...current, [fieldId]: value }));
		if (saveState === undefined) setInternalSaveState("dirty");
	};

	const handleDiscard = () => {
		setValues(getInitialValues(definition));
		if (saveState === undefined) setInternalSaveState("clean");
		onDiscard?.();
	};

	const handleApply = async () => {
		if (resolvedSaveState === "clean" || resolvedSaveState === "saving") return;
		if (saveState === undefined) setInternalSaveState("saving");
		try {
			await onApply?.(values);
			if (!onApply) {
				await new Promise((resolve) => window.setTimeout(resolve, 450));
			}
			if (saveState === undefined) {
				setInternalSaveState("saved");
				window.setTimeout(() => setInternalSaveState("clean"), 1200);
			}
		} catch {
			if (saveState === undefined) setInternalSaveState("error");
		}
	};

	const Icon = definition.icon;

	return (
		<aside
			className="meroku-properties"
			aria-label={`${definition.displayName} properties`}
		>
			<header className="mp-header">
				<div className="mp-header__identity">
					<span className="mp-header__icon" aria-hidden="true">
						<Icon />
					</span>
					<div className="mp-header__copy">
						<div className="mp-header__title-row">
							<h2 className="mp-header__title">{definition.name}</h2>
							<PropertyStatus
								label={definition.status}
								tone={definition.statusTone}
							/>
						</div>
						<div className="mp-header__context">{definition.context}</div>
					</div>
				</div>
				<div className="mp-header__actions">
					{definition.deletable && (
						<button
							type="button"
							className="mp-icon-button mp-icon-button--danger"
							onClick={onDelete}
							aria-label={`Delete ${definition.name}`}
							title="Delete"
						>
							<Trash2 />
						</button>
					)}
					<button
						type="button"
						className="mp-icon-button"
						onClick={onClose}
						aria-label="Close properties"
						title="Close"
					>
						<X />
					</button>
				</div>
			</header>

			{definition.views.length > 1 && (
				<nav className="mp-tabs" aria-label="Property views">
					{definition.views.map((item) => (
						<button
							type="button"
							key={item.id}
							className="mp-tab"
							data-active={item.id === view?.id}
							onClick={() => setActiveView(item.id)}
						>
							{item.label}
						</button>
					))}
				</nav>
			)}

			<div className="mp-content">
				{view?.sections.length ? (
					view.sections.map((section) =>
						section.advanced ? (
							<PropertyAdvancedDisclosure
								key={section.id}
								section={section}
								defaultOpen={openAdvanced || section.defaultOpen}
							>
								<PropertySection
									section={{ ...section, advanced: false }}
									values={values}
									onChange={handleFieldChange}
								/>
							</PropertyAdvancedDisclosure>
						) : (
							<PropertySection
								key={section.id}
								section={section}
								values={values}
								onChange={handleFieldChange}
							/>
						),
					)
				) : (
					<PropertyEmptyState
						title="Nothing to configure"
						description="This node is generated from the environment and is currently read-only."
					/>
				)}
			</div>

			<PropertySaveBar
				state={resolvedSaveState}
				lastUpdated={definition.lastUpdated}
				onDiscard={handleDiscard}
				onApply={handleApply}
			/>
		</aside>
	);
}

export function PropertySection({
	section,
	values,
	onChange,
}: PropertySectionProps) {
	const Icon = section.icon;
	return (
		<section className="mp-section" aria-labelledby={`${section.id}-title`}>
			<header className="mp-section__header">
				<div className="mp-section__title-group">
					{Icon && (
						<span
							className="mp-section__icon"
							data-tone={section.iconTone ?? "neutral"}
							aria-hidden="true"
						>
							<Icon />
						</span>
					)}
					<div>
						<h3 className="mp-section__title" id={`${section.id}-title`}>
							{section.title}
						</h3>
						{section.description && (
							<p className="mp-section__description">{section.description}</p>
						)}
					</div>
				</div>
			</header>
			<div className="mp-section__body">
				<div className="mp-fields">
					{section.fields.map((field) => (
						<PropertyField
							key={field.id}
							field={field}
							value={values[field.id]}
							onChange={(value) => onChange(field.id, value)}
						/>
					))}
				</div>
			</div>
		</section>
	);
}

export function PropertyField({ field, value, onChange }: PropertyFieldProps) {
	const [revealed, setRevealed] = useState(false);
	const span = field.span ?? "full";
	const fieldId = `property-${field.id}`;

	const inputFrame = (control: ReactNode) =>
		field.prefix || field.unit ? (
			<div className="mp-input-frame">
				{field.prefix && (
					<span className="mp-input-frame__prefix">{field.prefix}</span>
				)}
				{control}
				{field.unit && (
					<span className="mp-input-frame__unit">{field.unit}</span>
				)}
			</div>
		) : (
			control
		);

	const renderControl = () => {
		switch (field.kind) {
			case "number":
			case "text":
				return inputFrame(
					<Input
						id={fieldId}
						type={field.kind}
						value={(value as string | number | undefined) ?? ""}
						placeholder={field.placeholder}
						disabled={field.disabled}
						readOnly={field.readOnly}
						aria-invalid={Boolean(field.error)}
						className={`mp-control ${field.mono ? "mp-mono" : ""}`}
						onChange={(event: ChangeEvent<HTMLInputElement>) =>
							onChange(
								field.kind === "number"
									? Number(event.target.value)
									: event.target.value,
							)
						}
					/>,
				);
			case "textarea":
				return (
					<Textarea
						id={fieldId}
						value={(value as string | undefined) ?? ""}
						placeholder={field.placeholder}
						disabled={field.disabled}
						readOnly={field.readOnly}
						rows={field.rows ?? 3}
						aria-invalid={Boolean(field.error)}
						className={`mp-control ${field.mono ? "mp-mono" : ""}`}
						onChange={(event) => onChange(event.target.value)}
					/>
				);
			case "select":
				return (
					<Select
						value={String(value ?? "")}
						disabled={field.disabled}
						onValueChange={onChange}
					>
						<SelectTrigger id={fieldId} className="mp-control">
							<SelectValue placeholder={field.placeholder} />
						</SelectTrigger>
						<SelectContent className="mp-select-content">
							{field.options?.map((option) => (
								<SelectItem key={option.value} value={option.value}>
									{option.label}
								</SelectItem>
							))}
						</SelectContent>
					</Select>
				);
			case "toggle":
				return (
					<div className="mp-toggle-row">
						<div className="mp-toggle-row__copy">
							<div className="mp-toggle-row__label">{field.label}</div>
							{field.description && (
								<div className="mp-toggle-row__description">
									{field.description}
								</div>
							)}
						</div>
						<Switch
							id={fieldId}
							aria-label={field.label}
							checked={Boolean(value)}
							disabled={field.disabled}
							className="mp-switch"
							onCheckedChange={onChange}
						/>
					</div>
				);
			case "readonly":
				return (
					<div className={`mp-readonly ${field.mono ? "mp-mono" : ""}`}>
						<Lock aria-hidden="true" />
						<span className="mp-readonly__value">
							{String(value ?? "Not available")}
						</span>
						<button
							type="button"
							className="mp-icon-button"
							aria-label={`Copy ${field.label}`}
							title="Copy"
							onClick={() =>
								navigator.clipboard?.writeText(String(value ?? ""))
							}
						>
							<Copy />
						</button>
					</div>
				);
			case "secret":
				return (
					<div className="mp-input-frame">
						<Input
							id={fieldId}
							type={revealed ? "text" : "password"}
							value={(value as string | undefined) ?? ""}
							disabled={field.disabled}
							className="mp-control mp-mono"
							onChange={(event) => onChange(event.target.value)}
						/>
						<button
							type="button"
							className="mp-icon-button"
							onClick={() => setRevealed((current) => !current)}
							aria-label={
								revealed ? `Hide ${field.label}` : `Reveal ${field.label}`
							}
						>
							{revealed ? <EyeOff /> : <Eye />}
						</button>
					</div>
				);
			case "tags":
				return (
					<fieldset className="mp-tags" aria-label={field.label}>
						{((value as string[] | undefined) ?? []).map((item) => (
							<span className="mp-tag" key={item}>
								{item}
							</span>
						))}
					</fieldset>
				);
			case "key-value":
				return (
					<PropertyKeyValueEditor
						rows={(value as PropertyKeyValue[] | undefined) ?? []}
						disabled={field.disabled}
						onChange={onChange}
					/>
				);
			case "status":
				return (
					<div className="mp-readonly">
						<PropertyStatus label={String(value ?? "Unknown")} tone="success" />
					</div>
				);
		}
	};

	const hidesExternalLabel = field.kind === "toggle";

	return (
		<div className={`mp-field mp-field--${span}`}>
			{!hidesExternalLabel && (
				<div className="mp-field__label-row">
					<label className="mp-field__label" htmlFor={fieldId}>
						{field.label}
						{field.required && <span className="mp-field__required"> *</span>}
					</label>
					{field.loading && (
						<Loader2 aria-label="Loading" className="size-3 animate-spin" />
					)}
				</div>
			)}
			{renderControl()}
			{field.error ? (
				<p className="mp-field__error" id={`${fieldId}-error`}>
					{field.error}
				</p>
			) : (
				field.description &&
				field.kind !== "toggle" && (
					<p className="mp-field__description">{field.description}</p>
				)
			)}
		</div>
	);
}

export function PropertyKeyValueEditor({
	rows,
	disabled,
	onChange,
}: PropertyKeyValueEditorProps) {
	const [revealed, setRevealed] = useState<Record<string, boolean>>({});

	const updateRow = (id: string, updates: Partial<PropertyKeyValue>) => {
		onChange(rows.map((row) => (row.id === id ? { ...row, ...updates } : row)));
	};

	const removeRow = (id: string) =>
		onChange(rows.filter((row) => row.id !== id));
	const addRow = () =>
		onChange([
			...rows,
			{ id: `row-${Date.now()}`, key: "", value: "", secret: false },
		]);

	return (
		<div className="mp-kv">
			<div className="mp-kv__header" aria-hidden="true">
				<span>Key</span>
				<span>Value</span>
				<span />
			</div>
			{rows.map((row) => (
				<div className="mp-kv__row" key={row.id}>
					<Input
						aria-label="Variable key"
						className="mp-kv__input"
						value={row.key}
						disabled={disabled || row.readOnly}
						onChange={(event) => updateRow(row.id, { key: event.target.value })}
					/>
					<Input
						aria-label={`${row.key || "Variable"} value`}
						className="mp-kv__input"
						type={row.secret && !revealed[row.id] ? "password" : "text"}
						value={row.value}
						disabled={disabled || row.readOnly}
						onChange={(event) =>
							updateRow(row.id, { value: event.target.value })
						}
					/>
					<div className="mp-kv__actions">
						{row.secret && (
							<button
								type="button"
								className="mp-icon-button"
								onClick={() =>
									setRevealed((current) => ({
										...current,
										[row.id]: !current[row.id],
									}))
								}
								aria-label={revealed[row.id] ? "Hide value" : "Reveal value"}
							>
								{revealed[row.id] ? <EyeOff /> : <Eye />}
							</button>
						)}
						{row.readOnly ? (
							<Lock
								aria-label="System variable"
								className="size-3 text-zinc-600"
							/>
						) : (
							<button
								type="button"
								className="mp-icon-button mp-icon-button--danger"
								disabled={disabled}
								onClick={() => removeRow(row.id)}
								aria-label={`Remove ${row.key || "variable"}`}
							>
								<Trash2 />
							</button>
						)}
					</div>
				</div>
			))}
			<button
				type="button"
				className="mp-kv__add"
				disabled={disabled}
				onClick={addRow}
			>
				<Plus /> Add variable
			</button>
		</div>
	);
}

export function PropertyAdvancedDisclosure({
	section,
	defaultOpen,
	children,
}: {
	section: PropertySectionDefinition;
	defaultOpen?: boolean;
	children: ReactNode;
}) {
	return (
		<Collapsible className="mp-advanced" defaultOpen={defaultOpen}>
			<CollapsibleTrigger className="mp-advanced__trigger">
				<span>{section.title}</span>
				<ChevronDown aria-hidden="true" />
			</CollapsibleTrigger>
			<CollapsibleContent className="mp-advanced__content">
				{children}
			</CollapsibleContent>
		</Collapsible>
	);
}

export function PropertySaveBar({
	state,
	lastUpdated = "Last updated just now",
	onDiscard,
	onApply,
}: PropertySaveBarProps) {
	const status = {
		clean: { icon: Clock3, label: lastUpdated },
		dirty: { icon: AlertCircle, label: "Unsaved changes" },
		saving: { icon: Loader2, label: "Applying changes…" },
		saved: { icon: CheckCircle2, label: "Changes applied" },
		error: { icon: AlertCircle, label: "Apply failed — changes preserved" },
	}[state];
	const StatusIcon = status.icon;

	return (
		<footer className="mp-savebar">
			<output className="mp-savebar__status" data-state={state}>
				<StatusIcon className={state === "saving" ? "animate-spin" : ""} />
				<span>{status.label}</span>
			</output>
			<div className="mp-savebar__actions">
				<Button
					type="button"
					variant="ghost"
					className="mp-button mp-button--ghost"
					disabled={state === "clean" || state === "saving"}
					onClick={onDiscard}
				>
					Discard
				</Button>
				<Button
					type="button"
					className="mp-button mp-button--primary"
					disabled={state === "clean" || state === "saving"}
					onClick={onApply}
				>
					{state === "saving"
						? "Applying…"
						: state === "error"
							? "Retry"
							: "Apply Changes"}
				</Button>
			</div>
		</footer>
	);
}

export function PropertyEmptyState({
	title,
	description,
}: {
	title: string;
	description: string;
}) {
	return (
		<div className="mp-empty">
			<Check aria-hidden="true" />
			<div className="mp-empty__title">{title}</div>
			<div className="mp-empty__description">{description}</div>
		</div>
	);
}
