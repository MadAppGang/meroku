import {
	createContext,
	type ReactNode,
	useContext,
	useEffect,
	useMemo,
	useState,
} from "react";
import {
	PropertyAdvancedSettings,
	PropertyAutoscalingGroup,
	PropertyCapabilities,
	PropertyContainerProcess,
	PropertyEnvironmentVariables,
	PropertyFieldLayout,
	PropertyGroup,
	PropertyImageSource,
	PropertyPanelMeta,
	PropertyPanelShell,
	PropertyReadonlyRows,
	PropertySchemaField,
	toPropertyEnvironmentVariable,
} from "./PropertyLayouts";
import {
	PropertyActionButton,
	PropertyAutoscalingTarget,
	PropertyCheckboxField,
	PropertyEmptyState,
	type PropertyImageMode,
	PropertySaveBar,
} from "./PropertyPrimitives";
import type {
	PropertyFieldDefinition,
	PropertyKeyValue,
	PropertyPanelDefinition,
	PropertySaveState,
	PropertySectionDefinition,
} from "./types";

type PropertyValue = PropertyFieldDefinition["value"];
type PropertySurface = "dark" | "light";

const PropertySurfaceContext = createContext<PropertySurface>("dark");

interface PropertyPanelProps {
	definition: PropertyPanelDefinition;
	surface?: PropertySurface;
	compact?: boolean;
	initialView?: string;
	saveState?: PropertySaveState;
	openAdvanced?: boolean;
	onClose?: () => void;
	onDelete?: () => void;
	onDiscard?: () => void;
	onApply?: (values: Record<string, PropertyValue>) => void | Promise<void>;
}

interface CompactPropertyViewProps {
	sections: PropertySectionDefinition[];
	values: Record<string, PropertyValue>;
	onChange: (fieldId: string, value: PropertyValue) => void;
	openAdvanced?: boolean;
	fallbackIcon: ReactNode;
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

function getCompactViews(definition: PropertyPanelDefinition) {
	const [setup, ...secondary] = definition.views;
	if (!setup || secondary.length === 0) return definition.views;
	const containerSections = setup.sections.filter(isContainerProcessSection);
	const setupSections = setup.sections.filter(
		(section) => !isContainerProcessSection(section),
	);

	return [
		{ ...setup, label: "Setup", sections: setupSections },
		{
			id: "details",
			label: "Details",
			sections: [
				...containerSections,
				...secondary.flatMap((view) => view.sections),
			],
		},
	];
}

function isContainerProcessSection(section: PropertySectionDefinition) {
	const hasImage = section.fields.some((field) =>
		/image|container/i.test(`${field.id} ${field.label}`),
	);
	const hasCommand = section.fields.some((field) =>
		/command|entrypoint/i.test(`${field.id} ${field.label}`),
	);
	return hasImage && hasCommand;
}

function isRegistryStrategySection(section: PropertySectionDefinition) {
	const hasImageSource = section.fields.some(
		(field) => field.kind === "select" && /image source/i.test(field.label),
	);
	const hasRepository = section.fields.some((field) =>
		/repository/i.test(`${field.id} ${field.label}`),
	);
	return hasImageSource && hasRepository;
}

export function PropertyPanel({
	definition,
	surface = "dark",
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

	const compactViews = getCompactViews(definition);
	const compactView =
		compactViews.find((candidate) => candidate.id === activeView) ??
		compactViews[0];

	return (
		<PropertySurfaceContext.Provider value={surface}>
			<PropertyPanelShell
				name={definition.name}
				ariaLabel={`${definition.displayName} properties`}
				theme={surface}
				deletable={definition.deletable}
				onDelete={onDelete}
				onClose={onClose}
				views={compactViews.map(({ id, label }) => ({ id, label }))}
				activeView={compactView?.id ?? "setup"}
				onViewChange={setActiveView}
				meta={
					<PropertyPanelMeta
						status={definition.status}
						context={definition.context}
						tone={definition.statusTone}
					/>
				}
				footer={
					<PropertySaveBar
						state={resolvedSaveState}
						lastUpdated={definition.lastUpdated}
						onDiscard={handleDiscard}
						onApply={handleApply}
					/>
				}
			>
				{compactView?.sections.length ? (
					<CompactPropertyView
						sections={compactView.sections}
						values={values}
						onChange={handleFieldChange}
						openAdvanced={openAdvanced}
						fallbackIcon={<Icon />}
					/>
				) : (
					<PropertyEmptyState
						title="Nothing to configure"
						description="This node is generated from the environment and is currently read-only."
					/>
				)}
			</PropertyPanelShell>
		</PropertySurfaceContext.Provider>
	);
}

const normalizeCompactField = (
	field: PropertyFieldDefinition,
): PropertyFieldDefinition => ({
	...field,
	span:
		field.span ??
		(["textarea", "code", "key-value", "tags"].includes(field.kind)
			? "full"
			: "half"),
});

function CompactPropertyView({
	sections,
	values,
	onChange,
	openAdvanced,
	fallbackIcon,
}: CompactPropertyViewProps) {
	const primarySections = sections.filter((section) => !section.advanced);
	const advancedSections = sections.filter((section) => section.advanced);
	const advancedFields = advancedSections.flatMap((section) => section.fields);
	const [advancedOpen, setAdvancedOpen] = useState(
		openAdvanced || advancedSections.some((section) => section.defaultOpen),
	);

	return (
		<>
			{primarySections.map((section) => {
				if (isRegistryStrategySection(section)) {
					return (
						<CompactRegistryStrategySection
							key={section.id}
							section={section}
							values={values}
							onChange={onChange}
						/>
					);
				}
				if (isContainerProcessSection(section)) {
					return (
						<CompactContainerProcessSection
							key={section.id}
							section={section}
							values={values}
							onChange={onChange}
						/>
					);
				}
				const SectionIcon = section.icon;
				const isEnvironment = section.fields.some(
					(field) => field.kind === "key-value",
				);
				const isScaling = section.fields.some(
					(field) => field.kind === "toggle" && /autoscal/i.test(field.id),
				);
				const headerToggle = section.fields.some(
					(field) => field.kind === "tags",
				)
					? section.fields.find((field) => field.kind === "toggle")
					: undefined;
				const bodySection = headerToggle
					? {
							...section,
							fields: section.fields.filter(
								(field) => field.id !== headerToggle.id,
							),
						}
					: section;
				return (
					<PropertyGroup
						key={section.id}
						title={
							isEnvironment
								? "Environment"
								: isScaling
									? "Runtime & Scaling"
									: section.title
						}
						description={
							isEnvironment
								? "Injected values and overrides"
								: isScaling
									? "Network, resources, and task count"
									: section.description
						}
						icon={SectionIcon ? <SectionIcon /> : fallbackIcon}
						action={
							isEnvironment ? (
								<PropertyActionButton>+ Add variable</PropertyActionButton>
							) : headerToggle ? (
								<PropertyCheckboxField
									item={{
										id: headerToggle.id,
										label: headerToggle.label,
										checked: Boolean(values[headerToggle.id]),
										description: headerToggle.description,
										disabled: headerToggle.disabled,
										onCheckedChange: (checked) =>
											onChange(headerToggle.id, checked),
									}}
									label="Enabled"
									placement="header"
								/>
							) : undefined
						}
					>
						<CompactSectionFields
							section={bodySection}
							values={values}
							onChange={onChange}
						/>
					</PropertyGroup>
				);
			})}

			{advancedFields.length > 0 && (
				<PropertyAdvancedSettings
					open={advancedOpen}
					onOpenChange={setAdvancedOpen}
				>
					<CompactSectionFields
						section={{
							id: "compact-advanced",
							title: "Advanced settings",
							fields: advancedFields,
						}}
						values={values}
						onChange={onChange}
					/>
				</PropertyAdvancedSettings>
			)}
		</>
	);
}

function CompactRegistryStrategySection({
	section,
	values,
	onChange,
}: {
	section: PropertySectionDefinition;
	values: Record<string, PropertyValue>;
	onChange: (fieldId: string, value: PropertyValue) => void;
}) {
	const modeField = section.fields.find(
		(field) => field.kind === "select" && /image source/i.test(field.label),
	);
	const repositoryField = section.fields.find((field) =>
		/repository/i.test(`${field.id} ${field.label}`),
	);
	if (!modeField || !repositoryField) return null;

	const repository = String(values[repositoryField.id] ?? "Not configured");
	const account = String(
		Object.entries(values).find(([id]) => /ecr-account$/i.test(id))?.[1] ??
			"123456789012",
	);
	const region = String(
		Object.entries(values).find(([id]) => /ecr-region$/i.test(id))?.[1] ??
			"ap-southeast-2",
	);
	const SectionIcon = section.icon;

	return (
		<PropertyImageSource
			title="Registry Strategy"
			description="Repository ownership and source account"
			icon={SectionIcon ? <SectionIcon /> : undefined}
			source={String(
				values[modeField.id] ?? modeField.options?.[0]?.value ?? "",
			)}
			sources={(modeField.options ?? []).map((option) => ({
				id: option.value,
				label: option.label,
				value: /cross/i.test(option.value)
					? `${account}.dkr.ecr.${region}.amazonaws.com/${repository}`
					: repository,
			}))}
			onSourceChange={(source) => onChange(modeField.id, source)}
		/>
	);
}

function splitImageReference(reference: string) {
	const lastSlash = reference.lastIndexOf("/");
	const lastColon = reference.lastIndexOf(":");
	if (lastColon > lastSlash) {
		return {
			image: reference.slice(0, lastColon),
			tag: reference.slice(lastColon + 1) || "latest",
		};
	}
	return { image: reference, tag: "latest" };
}

function CompactContainerProcessSection({
	section,
	values,
	onChange,
}: {
	section: PropertySectionDefinition;
	values: Record<string, PropertyValue>;
	onChange: (fieldId: string, value: PropertyValue) => void;
}) {
	const imageField = section.fields.find((field) =>
		/image|container/i.test(`${field.id} ${field.label}`),
	);
	const commandField = section.fields.find((field) =>
		/command|entrypoint/i.test(`${field.id} ${field.label}`),
	);
	const initialReference = String(
		(imageField ? values[imageField.id] : undefined) ?? "Not available",
	);
	const [{ tag, image: defaultImage }] = useState(() =>
		splitImageReference(initialReference),
	);
	const [imageMode, setImageMode] = useState<PropertyImageMode>("default");
	const imageName = defaultImage.split("/").filter(Boolean).at(-1) ?? "service";
	const [imageValues] = useState<Record<PropertyImageMode, string>>(() => ({
		default: defaultImage,
		custom: `docker.io/library/${imageName}`,
		shared: `shared/${imageName}`,
	}));

	if (!imageField || !commandField) return null;

	return (
		<PropertyContainerProcess
			imageMode={imageMode}
			imageValues={imageValues}
			tag={tag}
			commandValue={String(values[commandField.id] ?? "")}
			onImageModeChange={(mode) => {
				setImageMode(mode);
				onChange(imageField.id, `${imageValues[mode]}:${tag}`);
			}}
			onCommandChange={(value) => onChange(commandField.id, value)}
		/>
	);
}

function CompactSectionFields({
	section,
	values,
	onChange,
}: {
	section: PropertySectionDefinition;
	values: Record<string, PropertyValue>;
	onChange: (fieldId: string, value: PropertyValue) => void;
}) {
	const surface = useContext(PropertySurfaceContext);
	const [targetUtilization, setTargetUtilization] = useState(70);
	const autoscaling = section.fields.find(
		(field) => field.kind === "toggle" && /autoscal/i.test(field.id),
	);
	const desired = section.fields.find((field) => /desired/i.test(field.id));
	const minimum = section.fields.find((field) =>
		/minimum|\bmin\b/i.test(field.id),
	);
	const maximum = section.fields.find((field) =>
		/maximum|\bmax\b/i.test(field.id),
	);
	const scalingFields = [autoscaling, desired, minimum, maximum].filter(
		(field): field is PropertyFieldDefinition => Boolean(field),
	);
	const hasScalingGroup = scalingFields.length === 4;
	const firstScalingIndex = hasScalingGroup
		? Math.min(...scalingFields.map((field) => section.fields.indexOf(field)))
		: -1;
	const scalingIds = new Set(scalingFields.map((field) => field.id));
	const beforeScaling = hasScalingGroup
		? section.fields
				.slice(0, firstScalingIndex)
				.filter((field) => !scalingIds.has(field.id))
		: section.fields;
	const afterScaling = hasScalingGroup
		? section.fields
				.slice(firstScalingIndex)
				.filter((field) => !scalingIds.has(field.id))
		: [];

	const fieldLayout = (
		items: PropertyFieldDefinition[],
		mode: "two-column" | "vertical" = "two-column",
	) => {
		if (items.length === 0) return null;

		return (
			<PropertyFieldLayout mode={mode}>
				{items.map((field) => (
					<PropertySchemaField
						key={field.id}
						field={normalizeCompactField(field)}
						value={values[field.id]}
						theme={surface}
						onChange={(value) => onChange(field.id, value)}
					/>
				))}
			</PropertyFieldLayout>
		);
	};

	const capabilities = (items: PropertyFieldDefinition[]) =>
		items.length > 0 && (
			<PropertyCapabilities
				items={items.map((field) => ({
					id: field.id,
					label: /x.?ray|distributed tracing/i.test(
						`${field.id} ${field.label}`,
					)
						? "X-Ray"
						: field.label,
					checked: Boolean(values[field.id]),
					description: field.description,
					disabled: field.disabled,
					onCheckedChange: (checked) => onChange(field.id, checked),
				}))}
			/>
		);

	const environmentFields = section.fields.filter(
		(field) => field.kind === "key-value",
	);
	const environmentTables = (items: PropertyFieldDefinition[]) =>
		items.map((field) => (
			<PropertyEnvironmentVariables
				key={field.id}
				variables={(
					(values[field.id] as PropertyKeyValue[] | undefined) ?? []
				).map(toPropertyEnvironmentVariable)}
			/>
		));
	if (section.fields.length === 1 && environmentFields.length === 1) {
		return <>{environmentTables(environmentFields)}</>;
	}
	const readonlyFields = section.fields.filter((field) =>
		["readonly", "status"].includes(field.kind),
	);
	const readonlyRows = (items: PropertyFieldDefinition[]) =>
		items.length > 0 && (
			<PropertyReadonlyRows
				rows={items.map((field) => [
					field.label,
					String(values[field.id] ?? "Not available"),
				])}
			/>
		);
	if (
		section.fields.length > 0 &&
		readonlyFields.length === section.fields.length
	) {
		return <>{readonlyRows(readonlyFields)}</>;
	}

	if (!hasScalingGroup || !autoscaling || !desired || !minimum || !maximum) {
		const toggleFields = section.fields.filter(
			(field) => field.kind === "toggle",
		);
		const ordinaryFields = section.fields.filter(
			(field) =>
				!["toggle", "key-value", "readonly", "status"].includes(field.kind),
		);
		const layoutMode =
			section.layout === "vertical" ? "vertical" : "two-column";
		return (
			<>
				{fieldLayout(ordinaryFields, layoutMode)}
				{capabilities(toggleFields)}
				{environmentTables(environmentFields)}
				{readonlyRows(readonlyFields)}
			</>
		);
	}

	const enabled = Boolean(values[autoscaling.id]);
	const scalingField = (field: PropertyFieldDefinition, label: string) => (
		<PropertySchemaField
			key={field.id}
			field={{ ...normalizeCompactField(field), label }}
			value={values[field.id]}
			theme={surface}
			onChange={(value) => onChange(field.id, value)}
		/>
	);

	return (
		<>
			{fieldLayout(
				beforeScaling.filter(
					(field) =>
						!["key-value", "readonly", "status", "toggle"].includes(field.kind),
				),
				section.layout === "vertical" ? "vertical" : "two-column",
			)}
			<PropertyAutoscalingGroup
				enabled={enabled}
				onEnabledChange={(next) => onChange(autoscaling.id, next)}
				target={
					<PropertyAutoscalingTarget
						value={targetUtilization}
						theme={surface}
						onChange={(value) => {
							setTargetUtilization(value);
							onChange(autoscaling.id, enabled);
						}}
					/>
				}
				enabledFields={
					<>
						{scalingField(minimum, "Min")}
						{scalingField(maximum, "Max")}
					</>
				}
				disabledFields={scalingField(desired, "Desired")}
			/>
			{fieldLayout(
				afterScaling.filter(
					(field) =>
						!["toggle", "key-value", "readonly", "status"].includes(field.kind),
				),
				section.layout === "vertical" ? "vertical" : "two-column",
			)}
			{capabilities(afterScaling.filter((field) => field.kind === "toggle"))}
			{environmentTables(
				section.fields.filter((field) => field.kind === "key-value"),
			)}
			{readonlyRows(
				section.fields.filter((field) =>
					["readonly", "status"].includes(field.kind),
				),
			)}
		</>
	);
}
