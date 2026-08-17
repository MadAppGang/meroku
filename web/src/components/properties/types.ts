import type { LucideIcon } from "lucide-react";

export type PropertyStatusTone =
	| "neutral"
	| "success"
	| "warning"
	| "danger"
	| "info";

export type PropertySaveState =
	| "clean"
	| "dirty"
	| "saving"
	| "saved"
	| "error";

export type PropertyFieldKind =
	| "text"
	| "number"
	| "select"
	| "toggle"
	| "textarea"
	| "readonly"
	| "secret"
	| "tags"
	| "key-value"
	| "status";

export type PropertySectionLayout = "auto" | "two-column" | "vertical";

export interface PropertyOption {
	label: string;
	value: string;
}

export interface PropertyKeyValue {
	id: string;
	key: string;
	value: string;
	secret?: boolean;
	readOnly?: boolean;
	origin?: "system" | "custom" | "service" | "secret";
	badge?: string;
	title?: string;
}

export interface PropertyFieldDefinition {
	id: string;
	label: string;
	kind: PropertyFieldKind;
	value?: string | number | boolean | string[] | PropertyKeyValue[];
	description?: string;
	placeholder?: string;
	options?: PropertyOption[];
	unit?: string;
	prefix?: string;
	mono?: boolean;
	disabled?: boolean;
	readOnly?: boolean;
	required?: boolean;
	error?: string;
	loading?: boolean;
	span?: "full" | "half";
	rows?: number;
}

export interface PropertySectionDefinition {
	id: string;
	title: string;
	description?: string;
	icon?: LucideIcon;
	iconTone?: PropertyStatusTone;
	fields: PropertyFieldDefinition[];
	layout?: PropertySectionLayout;
	advanced?: boolean;
	defaultOpen?: boolean;
}

export interface PropertyViewDefinition {
	id: string;
	label: string;
	sections: PropertySectionDefinition[];
}

export interface PropertyPanelDefinition {
	nodeType: string;
	name: string;
	displayName: string;
	icon: LucideIcon;
	status: string;
	statusTone?: PropertyStatusTone;
	context: string;
	deletable?: boolean;
	views: PropertyViewDefinition[];
	lastUpdated?: string;
}

export interface PropertyInventoryEntry {
	type: string;
	component: string;
	currentViews: string[];
	targetViews: string[];
	coverage: "editable" | "read-only" | "disabled";
}
