import {
	AlertCircle,
	Check,
	CheckCircle2,
	ChevronDown,
	Clock3,
	Container,
	Copy,
	Eye,
	EyeOff,
	Link2,
	Loader2,
	Lock,
	Plus,
	Trash2,
	X,
} from "lucide-react";
import { type ReactNode, useId, useState } from "react";
import { Button } from "../ui/button";
import { Checkbox } from "../ui/checkbox";
import { Input } from "../ui/input";
import { Popover, PopoverContent, PopoverTrigger } from "../ui/popover";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "../ui/select";
import { Slider } from "../ui/slider";
import { Textarea } from "../ui/textarea";
import { PropertyFieldRow } from "./PropertyFieldLayouts";
import type {
	PropertyFieldDefinition,
	PropertyKeyValue,
	PropertySaveState,
	PropertyStatusTone,
} from "./types";

export interface PropertyPanelTab {
	id: string;
	label: string;
}

export type PropertyEnvironmentOrigin =
	| "system"
	| "custom"
	| "service"
	| "secret";

export interface PropertyEnvironmentVariable {
	id: string;
	key: string;
	value: string;
	origin: PropertyEnvironmentOrigin;
	badge: string;
	title: string;
}

export type PropertyImageMode = "default" | "custom" | "shared";

export interface PropertyCapability {
	id: string;
	label: string;
	checked: boolean;
	description?: string;
	disabled?: boolean;
	onCheckedChange: (checked: boolean) => void;
}

export interface PropertyImageSourceOption {
	id: string;
	label: string;
	value: string;
}

export interface PropertySelectOption {
	label: string;
	value: string;
}

type PropertyValue = PropertyFieldDefinition["value"];

export function toPropertyEnvironmentVariable(
	row: PropertyKeyValue,
): PropertyEnvironmentVariable {
	const serviceName = row.value.split(/[.:/]/)[0] || "service";
	const origin =
		row.origin ??
		(row.secret
			? "secret"
			: row.readOnly && /(_HOST|_URL|_ENDPOINT)$/i.test(row.key)
				? "service"
				: row.readOnly
					? "system"
					: "custom");
	const badge =
		row.badge ??
		(origin === "secret"
			? "shared/prod"
			: origin === "service"
				? serviceName
				: origin === "system"
					? "System"
					: "Manual");
	const title =
		row.title ??
		(origin === "secret"
			? `Inherited from the ${badge} secret group`
			: origin === "service"
				? `Assigned automatically by the ${badge} service`
				: origin === "system"
					? "Assigned automatically by Meroku"
					: "Configured manually for this service");

	return {
		id: row.id,
		key: row.key,
		value: row.value,
		origin,
		badge,
		title,
	};
}

function propertyTagItemLabel(label: string) {
	if (/subject/i.test(label)) return "subject";
	if (/bucket/i.test(label)) return "bucket";
	if (/source/i.test(label)) return "source";
	if (/detail type/i.test(label)) return "detail type";
	if (/recipient/i.test(label)) return "recipient";
	if (/resolver/i.test(label)) return "resolver";
	if (/branch/i.test(label)) return "branch";
	if (/domain|alias/i.test(label)) return "alias";
	if (/reference/i.test(label)) return "reference";
	if (/handle/i.test(label)) return "handle";
	return "value";
}

export function PropertyPanelHeader({
	name,
	meta,
	deletable = false,
	onDelete,
	onClose,
}: {
	name: string;
	meta: ReactNode;
	deletable?: boolean;
	onDelete?: () => void;
	onClose?: () => void;
}) {
	return (
		<header className="mp-compact-header">
			<div>
				<h2>{name}</h2>
				<div className="mp-compact-header__meta">{meta}</div>
			</div>
			<div className="mp-compact-header__actions">
				{deletable && (
					<button
						type="button"
						className="mp-icon-button mp-icon-button--danger"
						aria-label={`Delete ${name}`}
						onClick={onDelete}
					>
						<Trash2 />
					</button>
				)}
				<button
					type="button"
					className="mp-icon-button"
					aria-label="Close properties"
					onClick={onClose}
				>
					<X />
				</button>
			</div>
		</header>
	);
}

export function PropertyPanelTabs({
	views,
	activeView,
	onViewChange,
}: {
	views: PropertyPanelTab[];
	activeView: string;
	onViewChange: (view: string) => void;
}) {
	if (views.length < 2) return null;

	return (
		<nav className="mp-tabs" aria-label="Property views">
			{views.map((view) => (
				<button
					type="button"
					key={view.id}
					className="mp-tab"
					data-active={activeView === view.id}
					aria-current={activeView === view.id ? "page" : undefined}
					onClick={() => onViewChange(view.id)}
				>
					{view.label}
				</button>
			))}
		</nav>
	);
}

export function PropertyPanelShell({
	name,
	meta,
	ariaLabel,
	theme,
	mode,
	views,
	activeView,
	onViewChange,
	deletable,
	onDelete,
	onClose,
	children,
	footer,
}: {
	name: string;
	meta: ReactNode;
	ariaLabel: string;
	theme: "dark" | "light";
	mode?: string;
	views: PropertyPanelTab[];
	activeView: string;
	onViewChange: (view: string) => void;
	deletable?: boolean;
	onDelete?: () => void;
	onClose?: () => void;
	children: ReactNode;
	footer: ReactNode;
}) {
	const contentEndId = useId();

	return (
		<aside
			className="meroku-properties mp-compact-panel"
			data-density="compact"
			data-mode={mode}
			data-surface={theme}
			data-theme={theme}
			aria-label={ariaLabel}
		>
			<PropertyPanelHeader
				name={name}
				meta={meta}
				deletable={deletable}
				onDelete={onDelete}
				onClose={onClose}
			/>
			<PropertyPanelTabs
				views={views}
				activeView={activeView}
				onViewChange={onViewChange}
			/>
			<section
				className="mp-compact-content"
				aria-label={`${ariaLabel} content`}
			>
				<a className="mp-sr-only" href={`#${contentEndId}`}>
					Skip to end of properties
				</a>
				<div className="mp-compact-sheet">{children}</div>
				<span id={contentEndId} aria-hidden="true" />
			</section>
			{footer}
		</aside>
	);
}

export function PropertyGroup({
	title,
	description,
	icon,
	action,
	children,
}: {
	title: string;
	description?: string;
	icon?: ReactNode;
	action?: ReactNode;
	children: ReactNode;
}) {
	return (
		<section className="mp-compact-group">
			<header className="mp-compact-group__header">
				<div className="mp-compact-group__identity">
					{icon && (
						<span className="mp-compact-group__icon" aria-hidden="true">
							{icon}
						</span>
					)}
					<div>
						<h3>{title}</h3>
						{description && <p>{description}</p>}
					</div>
				</div>
				{action}
			</header>
			<div className="mp-compact-group__body">{children}</div>
		</section>
	);
}

export function PropertyFieldFrame({
	label,
	span = "half",
	kind,
	labelPlacement = "auto",
	children,
}: {
	label: string;
	span?: "full" | "half";
	kind?: string;
	labelPlacement?: "auto" | "inline" | "stacked";
	children: ReactNode;
}) {
	const resolvedLabelPlacement =
		labelPlacement === "auto"
			? kind === "tags" || kind === "textarea"
				? "stacked"
				: "inline"
			: labelPlacement;

	return (
		<div
			className="mp-compact-field"
			data-kind={kind}
			data-label-placement={resolvedLabelPlacement}
			data-span={span}
		>
			<div className="mp-compact-field__label">{label}</div>
			{children}
		</div>
	);
}

export function PropertyEditableField({
	id,
	label,
	type = "text",
	value,
	prefix,
	unit,
	placeholder,
	mono = false,
	disabled = false,
	readOnly = false,
	error,
	span = "half",
	labelPlacement = "auto",
	onChange,
}: {
	id: string;
	label: string;
	type?: "text" | "number";
	value: string | number;
	prefix?: string;
	unit?: string;
	placeholder?: string;
	mono?: boolean;
	disabled?: boolean;
	readOnly?: boolean;
	error?: string;
	span?: "full" | "half";
	labelPlacement?: "auto" | "inline" | "stacked";
	onChange: (value: string | number) => void;
}) {
	const control = (
		<Input
			id={id}
			aria-label={label}
			aria-invalid={Boolean(error)}
			type={type}
			value={value}
			placeholder={placeholder}
			disabled={disabled}
			readOnly={readOnly}
			className={`mp-control ${mono ? "mp-mono" : ""}`}
			onChange={(event) =>
				onChange(
					type === "number" ? Number(event.target.value) : event.target.value,
				)
			}
		/>
	);

	return (
		<PropertyFieldFrame
			label={label}
			span={span}
			kind={type}
			labelPlacement={labelPlacement}
		>
			{prefix || unit ? (
				<div className="mp-input-frame">
					{prefix && <span className="mp-input-frame__prefix">{prefix}</span>}
					{control}
					{unit && <span className="mp-input-frame__unit">{unit}</span>}
				</div>
			) : (
				control
			)}
			{error && <span className="mp-compact-field__error">{error}</span>}
		</PropertyFieldFrame>
	);
}

export function PropertySelectField({
	id,
	label,
	value,
	options,
	theme,
	placeholder,
	disabled = false,
	span = "half",
	labelPlacement = "auto",
	onChange,
}: {
	id: string;
	label: string;
	value: string;
	options: PropertySelectOption[];
	theme: "dark" | "light";
	placeholder?: string;
	disabled?: boolean;
	span?: "full" | "half";
	labelPlacement?: "auto" | "inline" | "stacked";
	onChange: (value: string) => void;
}) {
	return (
		<PropertyFieldFrame
			label={label}
			span={span}
			kind="select"
			labelPlacement={labelPlacement}
		>
			<Select value={value} disabled={disabled} onValueChange={onChange}>
				<SelectTrigger id={id} aria-label={label} className="mp-control">
					<SelectValue placeholder={placeholder} />
				</SelectTrigger>
				<SelectContent
					className="mp-select-content"
					data-surface={theme}
					data-theme={theme}
				>
					{options.map((option) => (
						<SelectItem key={option.value} value={option.value}>
							{option.label}
						</SelectItem>
					))}
				</SelectContent>
			</Select>
		</PropertyFieldFrame>
	);
}

export function PropertyTextareaField({
	id,
	label,
	value,
	placeholder,
	mono = false,
	disabled = false,
	readOnly = false,
	error,
	rows = 3,
	span = "full",
	onChange,
}: {
	id: string;
	label: string;
	value: string;
	placeholder?: string;
	mono?: boolean;
	disabled?: boolean;
	readOnly?: boolean;
	error?: string;
	rows?: number;
	span?: "full" | "half";
	onChange: (value: string) => void;
}) {
	return (
		<PropertyFieldFrame label={label} span={span} kind="textarea">
			<Textarea
				id={id}
				aria-label={label}
				aria-invalid={Boolean(error)}
				value={value}
				placeholder={placeholder}
				disabled={disabled}
				readOnly={readOnly}
				rows={rows}
				className={`mp-control ${mono ? "mp-mono" : ""}`}
				onChange={(event) => onChange(event.target.value)}
			/>
			{error && <span className="mp-compact-field__error">{error}</span>}
		</PropertyFieldFrame>
	);
}

export function PropertySecretField({
	id,
	label,
	value,
	disabled = false,
	span = "half",
	onChange,
}: {
	id: string;
	label: string;
	value: string;
	disabled?: boolean;
	span?: "full" | "half";
	onChange: (value: string) => void;
}) {
	const [revealed, setRevealed] = useState(false);

	return (
		<PropertyFieldFrame label={label} span={span} kind="secret">
			<div className="mp-input-frame">
				<Input
					id={id}
					aria-label={label}
					type={revealed ? "text" : "password"}
					value={value}
					disabled={disabled}
					className="mp-control mp-mono"
					onChange={(event) => onChange(event.target.value)}
				/>
				<button
					type="button"
					className="mp-icon-button"
					onClick={() => setRevealed((current) => !current)}
					aria-label={revealed ? `Hide ${label}` : `Reveal ${label}`}
				>
					{revealed ? <EyeOff /> : <Eye />}
				</button>
			</div>
		</PropertyFieldFrame>
	);
}

export function PropertyReadonlyField({
	label,
	value,
	mono = false,
	span = "half",
}: {
	label: string;
	value: string;
	mono?: boolean;
	span?: "full" | "half";
}) {
	return (
		<PropertyFieldFrame label={label} span={span} kind="readonly">
			<div className={`mp-readonly ${mono ? "mp-mono" : ""}`}>
				<Lock aria-hidden="true" />
				<span className="mp-readonly__value">{value || "Not available"}</span>
				<button
					type="button"
					className="mp-icon-button"
					aria-label={`Copy ${label}`}
					title="Copy"
					onClick={() => navigator.clipboard?.writeText(value)}
				>
					<Copy />
				</button>
			</div>
		</PropertyFieldFrame>
	);
}

export function PropertyStatusField({
	label,
	value,
	tone = "neutral",
	span = "half",
}: {
	label: string;
	value: string;
	tone?: PropertyStatusTone;
	span?: "full" | "half";
}) {
	return (
		<PropertyFieldFrame label={label} span={span} kind="status">
			<span className="mp-status" data-tone={tone}>
				<span className="mp-status__dot" aria-hidden="true" />
				<span>{value}</span>
			</span>
		</PropertyFieldFrame>
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

export function PropertySchemaField({
	field,
	value,
	theme,
	onChange,
}: {
	field: PropertyFieldDefinition;
	value: PropertyValue;
	theme: "dark" | "light";
	onChange: (value: PropertyValue) => void;
}) {
	const span = field.span ?? "full";
	const fieldId = `property-${field.id}`;

	switch (field.kind) {
		case "text":
		case "number":
			return (
				<PropertyEditableField
					id={fieldId}
					label={field.label}
					type={field.kind}
					value={(value as string | number | undefined) ?? ""}
					prefix={field.prefix}
					unit={field.unit}
					placeholder={field.placeholder}
					mono={field.mono}
					disabled={field.disabled}
					readOnly={field.readOnly}
					error={field.error}
					span={span}
					onChange={onChange}
				/>
			);
		case "select":
			return (
				<PropertySelectField
					id={fieldId}
					label={field.label}
					value={String(value ?? "")}
					options={field.options ?? []}
					theme={theme}
					placeholder={field.placeholder}
					disabled={field.disabled}
					span={span}
					onChange={onChange}
				/>
			);
		case "toggle":
			return (
				<PropertyCheckboxField
					item={{
						id: field.id,
						label: field.label,
						checked: Boolean(value),
						description: field.description,
						disabled: field.disabled,
						onCheckedChange: onChange,
					}}
					span={span}
				/>
			);
		case "textarea":
			return (
				<PropertyTextareaField
					id={fieldId}
					label={field.label}
					value={String(value ?? "")}
					placeholder={field.placeholder}
					mono={field.mono}
					disabled={field.disabled}
					readOnly={field.readOnly}
					error={field.error}
					rows={field.rows}
					span={span}
					onChange={onChange}
				/>
			);
		case "readonly":
			return (
				<PropertyReadonlyField
					label={field.label}
					value={String(value ?? "Not available")}
					mono={field.mono}
					span={span}
				/>
			);
		case "secret":
			return (
				<PropertySecretField
					id={fieldId}
					label={field.label}
					value={String(value ?? "")}
					disabled={field.disabled}
					span={span}
					onChange={onChange}
				/>
			);
		case "tags":
			return (
				<PropertyFieldFrame label={field.label} span={span} kind="tags">
					<PropertyTagList
						label={field.label}
						values={(value as string[] | undefined) ?? []}
						itemLabel={propertyTagItemLabel(field.label)}
						disabled={field.disabled}
						onChange={onChange}
					/>
				</PropertyFieldFrame>
			);
		case "key-value":
			return (
				<div className="mp-compact-environment-field" data-span="full">
					<PropertyEnvironmentVariables
						variables={((value as PropertyKeyValue[] | undefined) ?? []).map(
							toPropertyEnvironmentVariable,
						)}
					/>
				</div>
			);
		case "status":
			return (
				<PropertyStatusField
					label={field.label}
					value={String(value ?? "Unknown")}
					tone="success"
					span={span}
				/>
			);
	}
}

export function PropertyTagList({
	label,
	values,
	itemLabel = "value",
	disabled,
	onChange,
}: {
	label: string;
	values: string[];
	itemLabel?: string;
	disabled?: boolean;
	onChange: (values: string[]) => void;
}) {
	const inputId = useId();
	const [adding, setAdding] = useState(false);
	const [draft, setDraft] = useState("");

	const cancelAdding = () => {
		setAdding(false);
		setDraft("");
	};

	const commitAdding = () => {
		const nextValue = draft.trim();
		if (!nextValue) return;

		if (!values.includes(nextValue)) onChange([...values, nextValue]);
		cancelAdding();
	};

	return (
		<fieldset className="mp-compact-tag-list" aria-label={label}>
			<div className="mp-compact-tag-list__rows">
				{values.map((value, index) => (
					<div className="mp-compact-tag-list__row" key={value}>
						<code title={value}>{value}</code>
						<button
							type="button"
							className="mp-icon-button"
							aria-label={`Remove ${value}`}
							disabled={disabled}
							onClick={() =>
								onChange(values.filter((_, itemIndex) => itemIndex !== index))
							}
						>
							<X />
						</button>
					</div>
				))}
				{values.length === 0 && !adding && (
					<div className="mp-compact-tag-list__empty">
						No {itemLabel}s added
					</div>
				)}
				{adding ? (
					<div className="mp-compact-tag-list__add-row">
						<Input
							id={inputId}
							autoFocus
							aria-label={`New ${itemLabel}`}
							placeholder={`Enter ${itemLabel}`}
							className="mp-control mp-mono"
							value={draft}
							onChange={(event) => setDraft(event.target.value)}
							onKeyDown={(event) => {
								if (event.key === "Enter") commitAdding();
								if (event.key === "Escape") cancelAdding();
							}}
						/>
						<button
							type="button"
							className="mp-icon-button mp-compact-tag-list__confirm"
							aria-label={`Save ${itemLabel}`}
							disabled={!draft.trim()}
							onClick={commitAdding}
						>
							<Check />
						</button>
					</div>
				) : (
					<button
						type="button"
						className="mp-compact-tag-list__add"
						aria-label={`Add ${itemLabel}`}
						title={`Add ${itemLabel}`}
						disabled={disabled}
						onClick={() => setAdding(true)}
					>
						<Plus aria-hidden="true" />
						Add {itemLabel}
					</button>
				)}
			</div>
		</fieldset>
	);
}

export function PropertyImageSource({
	title,
	description,
	icon,
	source,
	sources,
	suffix,
	commandValue,
	onSourceChange,
	onCommandChange,
}: {
	title: string;
	description: string;
	icon?: ReactNode;
	source: string;
	sources: PropertyImageSourceOption[];
	suffix?: string;
	commandValue?: string;
	onSourceChange: (source: string) => void;
	onCommandChange?: (value: string) => void;
}) {
	const selectedSource =
		sources.find((candidate) => candidate.id === source) ?? sources[0];

	return (
		<PropertyGroup
			title={title}
			description={description}
			icon={icon ?? <Container />}
			action={
				<fieldset className="mp-compact-segments" aria-label="Image source">
					{sources.map((candidate) => (
						<button
							type="button"
							key={candidate.id}
							data-active={source === candidate.id}
							aria-pressed={source === candidate.id}
							onClick={() => onSourceChange(candidate.id)}
						>
							{candidate.label}
						</button>
					))}
				</fieldset>
			}
		>
			<div className="mp-compact-image-row">
				<Lock aria-hidden="true" />
				<code>{selectedSource?.value ?? "Not configured"}</code>
				{suffix && <span>: {suffix}</span>}
			</div>
			{commandValue !== undefined && onCommandChange && (
				<PropertyFieldRow variant="single">
					<PropertyFieldFrame label="Command override">
						<Input
							aria-label="Command override"
							className="mp-control mp-mono"
							value={commandValue}
							onChange={(event) => onCommandChange(event.target.value)}
						/>
					</PropertyFieldFrame>
				</PropertyFieldRow>
			)}
		</PropertyGroup>
	);
}

export function PropertyContainerProcess({
	imageMode,
	imageValues,
	commandValue,
	tag = "latest",
	onImageModeChange,
	onCommandChange,
}: {
	imageMode: PropertyImageMode;
	imageValues: Record<PropertyImageMode, string>;
	commandValue: string;
	tag?: string;
	onImageModeChange: (mode: PropertyImageMode) => void;
	onCommandChange: (value: string) => void;
}) {
	return (
		<PropertyImageSource
			title="Container & Process"
			description="Setup-time image and entrypoint"
			source={imageMode}
			sources={(["default", "custom", "shared"] as PropertyImageMode[]).map(
				(mode) => ({
					id: mode,
					label: mode[0].toUpperCase() + mode.slice(1),
					value: imageValues[mode],
				}),
			)}
			suffix={tag}
			commandValue={commandValue}
			onSourceChange={(source) =>
				onImageModeChange(source as PropertyImageMode)
			}
			onCommandChange={onCommandChange}
		/>
	);
}

export function PropertyAutoscalingGroup({
	enabled,
	onEnabledChange,
	ariaLabel = "Autoscaling",
	target,
	enabledFields,
	disabledFields,
}: {
	enabled: boolean;
	onEnabledChange: (enabled: boolean) => void;
	ariaLabel?: string;
	target?: ReactNode;
	enabledFields: ReactNode;
	disabledFields: ReactNode;
}) {
	return (
		<div className="mp-autoscaling-group" data-enabled={enabled}>
			<PropertyCheckboxField
				className="mp-autoscaling-group__toggle"
				item={{
					id: "autoscaling",
					label: ariaLabel,
					checked: enabled,
					onCheckedChange: onEnabledChange,
				}}
				label="Autoscaling"
			/>
			{enabled && target}
			<div className="mp-autoscaling-group__fields">
				{enabled ? enabledFields : disabledFields}
			</div>
		</div>
	);
}

export function PropertyAutoscalingTarget({
	value,
	theme,
	onChange,
}: {
	value: number;
	theme: "dark" | "light";
	onChange: (value: number) => void;
}) {
	const [open, setOpen] = useState(false);
	const surfaceTheme = theme === "dark" ? "dark" : "light";

	return (
		<Popover open={open} onOpenChange={setOpen}>
			<PopoverTrigger asChild>
				<button
					type="button"
					className="mp-compact-target"
					aria-label="Edit autoscaling target"
				>
					Target {value}%
				</button>
			</PopoverTrigger>
			<PopoverContent
				align="center"
				className="mp-target-popover"
				data-surface={surfaceTheme}
				data-theme={theme}
				role="dialog"
				aria-label="Autoscaling target"
			>
				<div className="mp-target-popover__heading">
					<div className="mp-target-popover__title">Target utilization</div>
					<output>{value}%</output>
				</div>
				<p>Average CPU before scaling · 5% steps</p>
				<div className="mp-target-popover__editor">
					<Slider
						aria-label="Autoscaling CPU target"
						className="mp-target-slider"
						max={100}
						min={5}
						step={5}
						value={[value]}
						onValueChange={([nextValue]) => onChange(nextValue)}
					/>
					<div className="mp-target-popover__scale">
						<span>5%</span>
						<span>100%</span>
					</div>
				</div>
			</PopoverContent>
		</Popover>
	);
}

export function PropertyCapabilities({
	items,
}: {
	items: PropertyCapability[];
}) {
	return (
		<div className="mp-compact-runtime-row mp-compact-runtime-row--features">
			<div className="mp-compact-capabilities">
				{items.map((item) => (
					<PropertyCheckboxField item={item} key={item.id} />
				))}
			</div>
		</div>
	);
}

export function PropertyCheckboxField({
	item,
	label = item.label,
	placement = "body",
	className,
	span,
}: {
	item: PropertyCapability;
	label?: string;
	placement?: "body" | "header";
	className?: string;
	span?: "full" | "half";
}) {
	const controlId = `property-capability-${item.id}`;

	return (
		<div
			className={`mp-compact-capability${className ? ` ${className}` : ""}`}
			data-placement={placement}
			data-span={span}
			data-disabled={item.disabled ? "true" : undefined}
			title={item.description}
		>
			<Checkbox
				id={controlId}
				aria-label={item.label}
				checked={item.checked}
				disabled={item.disabled}
				className="mp-compact-checkbox"
				onCheckedChange={(checked) => item.onCheckedChange(Boolean(checked))}
			/>
			<label htmlFor={controlId}>{label}</label>
		</div>
	);
}

/** Compatibility name retained while panels migrate to the catalog name. */
export const PropertyCapabilityToggle = PropertyCheckboxField;

export function PropertyAdvancedSettings({
	open,
	onOpenChange,
	children,
}: {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	children: ReactNode;
}) {
	return (
		<div className="mp-compact-advanced">
			<button
				type="button"
				aria-expanded={open}
				onClick={() => onOpenChange(!open)}
			>
				<span>Advanced settings</span>
				<ChevronDown data-open={open} />
			</button>
			{open && <div className="mp-compact-advanced__body">{children}</div>}
		</div>
	);
}

export function PropertyReadonlyRows({ rows }: { rows: string[][] }) {
	return (
		<div className="mp-compact-readonly">
			{rows.map(([label, value]) => (
				<div className="mp-compact-readonly__row" key={label}>
					<span>{label}</span>
					<code>{value}</code>
				</div>
			))}
		</div>
	);
}

export function PropertySaveBar({
	state,
	lastUpdated = "Last updated just now",
	cleanLabel,
	discardLabel = "Discard",
	primaryLabel,
	allowCleanActions = false,
	showStatusIcon = true,
	onDiscard,
	onApply,
}: {
	state: PropertySaveState;
	lastUpdated?: string;
	cleanLabel?: string;
	discardLabel?: string;
	primaryLabel?: string;
	allowCleanActions?: boolean;
	showStatusIcon?: boolean;
	onDiscard: () => void;
	onApply: () => void;
}) {
	const status = {
		clean: { icon: Clock3, label: cleanLabel ?? lastUpdated },
		dirty: { icon: AlertCircle, label: "Unsaved changes" },
		saving: { icon: Loader2, label: "Applying changes…" },
		saved: { icon: CheckCircle2, label: "Changes applied" },
		error: { icon: AlertCircle, label: "Apply failed — changes preserved" },
	}[state];
	const StatusIcon = status.icon;
	const actionsDisabled =
		state === "saving" ||
		state === "saved" ||
		(state === "clean" && !allowCleanActions);

	return (
		<footer className="mp-savebar" data-state={state}>
			<output
				className="mp-savebar__status"
				data-state={state}
				aria-live="polite"
			>
				{showStatusIcon && (
					<StatusIcon className={state === "saving" ? "animate-spin" : ""} />
				)}
				<span>{status.label}</span>
			</output>
			<div className="mp-savebar__actions">
				<Button
					type="button"
					variant="ghost"
					className="mp-button mp-button--ghost"
					disabled={actionsDisabled}
					onClick={onDiscard}
				>
					{discardLabel}
				</Button>
				<Button
					type="button"
					className="mp-button mp-button--primary"
					disabled={actionsDisabled}
					onClick={onApply}
				>
					{state === "saving"
						? "Applying…"
						: state === "error"
							? "Retry"
							: (primaryLabel ?? "Apply Changes")}
				</Button>
			</div>
		</footer>
	);
}

export function PropertyEnvironmentVariables({
	variables,
	visibleLimit = 5,
}: {
	variables: PropertyEnvironmentVariable[];
	visibleLimit?: number;
}) {
	const rowsId = useId();
	const [expanded, setExpanded] = useState(false);
	const [revealed, setRevealed] = useState<Record<string, boolean>>({});
	const visibleVariables = expanded
		? variables
		: variables.slice(0, visibleLimit);
	const hiddenCount = Math.max(variables.length - visibleLimit, 0);

	return (
		<>
			<table className="mp-compact-env" aria-label="Environment variables">
				<thead>
					<tr className="mp-compact-env__head">
						<th scope="col">Key</th>
						<th scope="col">Value</th>
						<th scope="col">
							<span className="mp-sr-only">Actions</span>
						</th>
					</tr>
				</thead>
				<tbody id={rowsId}>
					{visibleVariables.map((variable) => (
						<tr
							className="mp-compact-env__row"
							data-origin={variable.origin}
							key={variable.id}
						>
							<td>
								<div className="mp-env-item">
									<code>{variable.key}</code>
									<span className="mp-env-item-badge" title={variable.title}>
										{variable.origin === "service" && (
											<Link2 aria-hidden="true" />
										)}
										{variable.origin === "secret" && (
											<Lock aria-hidden="true" />
										)}
										{variable.badge}
									</span>
								</div>
							</td>
							<td>
								<code>
									{variable.origin === "secret" && !revealed[variable.id]
										? "••••••••••••••"
										: variable.value}
								</code>
							</td>
							<td>
								{variable.origin === "secret" && (
									<button
										type="button"
										className="mp-icon-button"
										aria-label={
											revealed[variable.id]
												? `Hide ${variable.key}`
												: `Reveal ${variable.key}`
										}
										onClick={() =>
											setRevealed((current) => ({
												...current,
												[variable.id]: !current[variable.id],
											}))
										}
									>
										{revealed[variable.id] ? <EyeOff /> : <Eye />}
									</button>
								)}
							</td>
						</tr>
					))}
				</tbody>
			</table>
			{hiddenCount > 0 && (
				<button
					type="button"
					className="mp-compact-env-toggle"
					aria-controls={rowsId}
					aria-expanded={expanded}
					onClick={() => setExpanded((current) => !current)}
				>
					<span>
						{expanded
							? "Show fewer variables"
							: `Show ${hiddenCount} more variables`}
					</span>
					<ChevronDown aria-hidden="true" />
				</button>
			)}
		</>
	);
}
