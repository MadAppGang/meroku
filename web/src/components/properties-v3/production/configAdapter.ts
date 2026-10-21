/**
 * properties-v3 — production binding between a panel definition and the
 * environment YAML (`YamlInfrastructureConfig`).
 *
 * Pure functions only. A panel is bound to ONE top-level YAML block (e.g.
 * `postgres`); a field is config-backed when its first property alias is a
 * direct scalar child of that block (`postgres.dbname`). Everything else —
 * AWS-assigned endpoints, runtime status — has no YAML source and stays
 * unresolved rather than being invented.
 */

import type { YamlInfrastructureConfig } from "../../../types/yamlConfig";
import { getFieldPropertyAliases } from "../helpers";
import type {
	AffectedResource,
	PropertyChange,
	PropertyFieldDefinition,
	PropertyPanelDefinition,
	PropertyPanelInstance,
} from "../types";

export interface ConfigPanelBinding {
	/** Panel definition id in NODE_CATALOG_BY_ID_V3. */
	definitionId: string;
	/** Top-level YAML block the panel edits. */
	root: keyof YamlInfrastructureConfig & string;
	/**
	 * Values the Handlebars template falls back to when the YAML omits a key
	 * (`{{default postgres.instance_class "db.t4g.micro"}}`). Shown as the
	 * baseline so the panel displays what will actually be deployed; only
	 * written to YAML when the user changes them.
	 */
	templateDefaults?: Readonly<Record<string, unknown>>;
	/**
	 * The AWS resource a replacement change destroys, as named in the
	 * destructive confirmation. Computed from the saved (baseline) config:
	 * that is the resource that exists today.
	 */
	resourceName: (config: YamlInfrastructureConfig) => string;
}

/** Bindings that are wired into the shipped Sidebar. */
export const CONFIG_PANEL_BINDINGS = {
	postgres: {
		definitionId: "postgres",
		root: "postgres",
		// env/main.hbs: instance_class / allocated_storage use `default`.
		templateDefaults: {
			instance_class: "db.t4g.micro",
			allocated_storage: 20,
		},
		// modules/postgres/naming.tf legacy forms (RDS identifier cap is 63;
		// anything longer is cascaded by modules/naming, so name it generically).
		resourceName: (config) => {
			const aurora = config.postgres?.aurora === true;
			const name = `${config.project}-${aurora ? "aurora" : "postgres"}-${config.env}`;
			const kind = aurora ? "Aurora cluster" : "RDS instance";
			return name.length <= 63 ? `${name} (${kind})` : `PostgreSQL ${kind}`;
		},
	},
} satisfies Record<string, ConfigPanelBinding>;

/** The YAML key inside `binding.root` a field writes, or null if unbound. */
export function configKeyFor(
	field: PropertyFieldDefinition,
	binding: ConfigPanelBinding,
): string | null {
	if (field.valueKind !== "editable") return null;
	const [alias] = getFieldPropertyAliases(field);
	if (!alias) return null;
	const segments = alias.split(".");
	if (segments.length !== 2 || segments[0] !== binding.root) return null;
	const key = segments[1];
	return key && !key.includes("[") ? key : null;
}

function rootBlock(
	config: YamlInfrastructureConfig,
	binding: ConfigPanelBinding,
): Record<string, unknown> {
	const block = (config as unknown as Record<string, unknown>)[binding.root];
	return block !== null && typeof block === "object" && !Array.isArray(block)
		? (block as Record<string, unknown>)
		: {};
}

/** Build the panel instance (baseline keyed by field.id) from YAML. */
export function buildConfigInstance(
	definition: PropertyPanelDefinition,
	config: YamlInfrastructureConfig,
	binding: ConfigPanelBinding,
): PropertyPanelInstance {
	const block = rootBlock(config, binding);
	const baseline: Record<string, unknown> = {};
	for (const field of definition.fields ?? []) {
		const key = configKeyFor(field, binding);
		if (key === null) continue;
		const value = Object.hasOwn(block, key)
			? block[key]
			: binding.templateDefaults?.[key];
		if (value !== undefined && value !== null) baseline[field.id] = value;
	}
	return {
		nodeInstanceId: `${config.project}/${config.env}/${binding.definitionId}`,
		// The web UI cannot tell whether the resource exists in AWS. Assume it
		// does, so replacement changes are always gated behind confirmation.
		lifecycleState: "created",
		baseline,
		cloudWatchDestination: null,
		resource: { id: binding.definitionId, name: binding.resourceName(config) },
	};
}

/**
 * The catalog names affected resources with fixture identities (Storybook
 * demo data). In the app every replacement field destroys the bound node's
 * real resource, so point them all at `resource`.
 */
export function bindDefinitionResources(
	definition: PropertyPanelDefinition,
	resource: AffectedResource,
): PropertyPanelDefinition {
	return {
		...definition,
		fields: definition.fields?.map((field) =>
			field.affectedResources
				? { ...field, affectedResources: [resource] }
				: field,
		),
	};
}

export type ConfigUpdateResult =
	| { ok: true; updates: Partial<YamlInfrastructureConfig> }
	| { ok: false; message: string };

/**
 * Translate reviewed configuration-channel changes into the partial update
 * `onConfigChange` deep-merges. Rejects anything it cannot write faithfully:
 * unbound fields, collection operations, and removals (the App merge cannot
 * delete a key, so "cleared" would silently keep the old value).
 */
export function configUpdatesFromChanges(
	definition: PropertyPanelDefinition,
	config: YamlInfrastructureConfig,
	binding: ConfigPanelBinding,
	changes: readonly PropertyChange[],
): ConfigUpdateResult {
	const fields = new Map(
		(definition.fields ?? []).map((field) => [field.id, field]),
	);
	const next: Record<string, unknown> = { ...rootBlock(config, binding) };
	for (const change of changes) {
		const field = fields.get(change.fieldId);
		const key = field ? configKeyFor(field, binding) : null;
		if (!field || key === null) {
			return {
				ok: false,
				message: `${change.propertyPath} is not stored in the environment YAML`,
			};
		}
		if (change.operation === "remove" || change.after === undefined) {
			return {
				ok: false,
				message: `${field.label} cannot be cleared here; set a value instead`,
			};
		}
		if (change.operation !== "set" && change.operation !== "add") {
			return {
				ok: false,
				message: `${field.label}: ${change.operation} is not supported`,
			};
		}
		next[key] = change.after;
	}
	return {
		ok: true,
		updates: { [binding.root]: next } as Partial<YamlInfrastructureConfig>,
	};
}
