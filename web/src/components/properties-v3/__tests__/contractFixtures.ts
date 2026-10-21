// Synthetic fixtures built solely from api-contracts.md; no production catalog imports.
import type {
	PropertyFieldDefinition,
	PropertyPanelContext,
	PropertyPanelDefinition,
} from "../types";

export const context: PropertyPanelContext = {
	nodeInstanceId: "synthetic-node-1",
	lifecycleState: "created",
	cloudWatchDestination: null,
};

export function field(
	id: string,
	overrides: Partial<PropertyFieldDefinition> = {},
): PropertyFieldDefinition {
	return {
		id,
		property: `synthetic.${id}`,
		label: `Synthetic ${id}`,
		mutability: "TF",
		valueKind: "editable",
		controlType: "text",
		section: "source",
		...overrides,
	};
}

export function panel(
	fields: PropertyFieldDefinition[],
	overrides: Partial<PropertyPanelDefinition> = {},
): PropertyPanelDefinition {
	const keys = [...new Set(fields.map((item) => item.section))];
	return {
		id: "synthetic-panel",
		title: "Synthetic contract panel",
		fields,
		sections: keys.map((key) => ({
			key,
			archetype: "standard",
			fieldIds: fields
				.filter((item) => item.section === key)
				.map((item) => item.id),
		})),
		...overrides,
	};
}

export function freeze<T>(value: T): T {
	if (value !== null && typeof value === "object") {
		for (const item of Object.values(value)) freeze(item);
		Object.freeze(value);
	}
	return value;
}

export const databaseFields: PropertyFieldDefinition[] = [
	field("mode", {
		property: "postgres.aurora",
		mutability: "CREATE / TF-REPLACE",
		controlType: "boolean",
		section: "storage-data",
	}),
	field("instance", {
		property: "postgres.instance_class",
		section: "compute-scaling",
		visibleWhen: {
			all: [{ kind: "equals", key: "postgres.aurora", value: false }],
		},
	}),
	field("capacity", {
		property: "postgres.min_capacity",
		controlType: "number",
		section: "compute-scaling",
		visibleWhen: { all: [{ kind: "equals", key: "mode", value: true }] },
	}),
	field("dbname", {
		property: "postgres.dbname",
		mutability: "TF-REPLACE",
		section: "storage-data",
	}),
	field("username", {
		property: "postgres.username",
		mutability: "TF-REPLACE",
		section: "storage-data",
	}),
];

export function parameterField(
	overrides: Partial<PropertyFieldDefinition> = {},
): PropertyFieldDefinition {
	return field("parameters", {
		property: "task.parameters",
		mutability: "LIVE",
		section: "environment-secrets",
		controlType: "key-value-table",
		collection: {
			recordKeys: true,
			diffRules: {
				valueProperties: ["value"],
				valueEditImpact: "restart-to-observe",
				valueEditChannel: "live",
				membershipImpact: "rolling-restart",
			},
		},
		...overrides,
	});
}
