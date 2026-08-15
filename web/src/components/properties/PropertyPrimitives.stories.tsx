import type { Meta, StoryObj } from "@storybook/react-vite";
import { Braces, Container, Gauge, Settings2 } from "lucide-react";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React from "react";
import { expect, within } from "storybook/test";
import { PropertyPanel } from "./PropertyPanel";
import type {
	PropertyFieldDefinition,
	PropertyPanelDefinition,
	PropertySectionDefinition,
} from "./types";

const makeDefinition = (
	name: string,
	sections: PropertySectionDefinition[],
): PropertyPanelDefinition => ({
	nodeType: "primitive-review",
	name,
	displayName: "Property primitives",
	icon: Settings2,
	status: "Review",
	statusTone: "info",
	context: "storybook · reference extraction",
	lastUpdated: "Last updated just now",
	views: [{ id: "setup", label: "Setup", sections }],
});

const section = (
	id: string,
	title: string,
	fields: PropertyFieldDefinition[],
	overrides: Partial<PropertySectionDefinition> = {},
): PropertySectionDefinition => ({ id, title, fields, ...overrides });

const allFields = makeDefinition("All field primitives", [
	section(
		"fields-runtime",
		"Runtime & Network",
		[
			{ id: "port", label: "Port", kind: "number", value: 8080, span: "half" },
			{
				id: "health",
				label: "Health path",
				kind: "text",
				value: "/health",
				mono: true,
				span: "half",
			},
			{
				id: "cpu",
				label: "CPU",
				kind: "select",
				value: "512",
				options: [
					{ label: "256 (0.25 vCPU)", value: "256" },
					{ label: "512 (0.5 vCPU)", value: "512" },
					{ label: "1024 (1 vCPU)", value: "1024" },
				],
				span: "half",
			},
			{
				id: "memory",
				label: "Memory",
				kind: "select",
				value: "1024",
				options: [
					{ label: "512 MB", value: "512" },
					{ label: "1024 MB", value: "1024" },
					{ label: "2048 MB", value: "2048" },
				],
				span: "half",
			},
			{
				id: "timeout",
				label: "Deploy timeout",
				kind: "number",
				value: 15,
				unit: "min",
				span: "half",
			},
			{
				id: "tracing",
				label: "Distributed tracing",
				kind: "toggle",
				value: true,
				description: "Run the X-Ray collector",
				span: "half",
			},
		],
		{
			icon: Gauge,
			iconTone: "warning",
			description: "Paired fields stay on one compact row",
		},
	),
	section(
		"fields-values",
		"Values & Secrets",
		[
			{
				id: "repository",
				label: "Managed repository",
				kind: "readonly",
				value: "123456789.dkr.ecr.ap-southeast-2.amazonaws.com/circl_backend",
				mono: true,
			},
			{
				id: "secret",
				label: "API key",
				kind: "secret",
				value: "sk_live_example",
			},
			{
				id: "tags",
				label: "Allowed subjects",
				kind: "tags",
				value: ["repo:madappgang/circl:*", "main"],
			},
			{
				id: "status",
				label: "Runtime state",
				kind: "status",
				value: "Healthy",
			},
			{
				id: "notes",
				label: "Notes",
				kind: "textarea",
				value: "Compact multi-line content.",
				rows: 3,
			},
		],
		{ icon: Container, iconTone: "info" },
	),
	section(
		"fields-kv",
		"Environment Variables",
		[
			{
				id: "variables",
				label: "Variables",
				kind: "key-value",
				value: [
					{
						id: "env-node",
						key: "NODE_ENV",
						value: "production",
						readOnly: true,
					},
					{
						id: "env-host",
						key: "DB_HOST",
						value: "postgres.internal",
						readOnly: true,
					},
					{
						id: "env-key",
						key: "API_KEY",
						value: "sk_live_example",
						secret: true,
					},
				],
			},
		],
		{ icon: Braces, iconTone: "success" },
	),
]);

const stateFields = makeDefinition("Field states", [
	section(
		"states",
		"Input States",
		[
			{
				id: "default",
				label: "Default",
				kind: "text",
				value: "Editable value",
				span: "half",
			},
			{
				id: "placeholder",
				label: "Empty",
				kind: "text",
				value: "",
				placeholder: "Enter a value",
				span: "half",
			},
			{
				id: "required",
				label: "Required",
				kind: "text",
				value: "",
				required: true,
				error: "This value is required",
				span: "half",
			},
			{
				id: "disabled",
				label: "Disabled",
				kind: "text",
				value: "Unavailable",
				disabled: true,
				span: "half",
			},
			{
				id: "loading",
				label: "Loading",
				kind: "text",
				value: "Resolving…",
				readOnly: true,
				loading: true,
				span: "half",
			},
			{
				id: "readonly",
				label: "Read only",
				kind: "readonly",
				value: "generated-by-terraform",
				mono: true,
				span: "half",
			},
		],
		{
			icon: Settings2,
			iconTone: "danger",
			description: "Default, empty, error, disabled, loading, and read-only",
		},
	),
]);

const advancedFields = makeDefinition("Advanced disclosure", [
	section(
		"common",
		"Common Settings",
		[
			{
				id: "name",
				label: "Service name",
				kind: "text",
				value: "terminator",
				span: "half",
			},
			{ id: "port", label: "Port", kind: "number", value: 8080, span: "half" },
		],
		{ icon: Container, iconTone: "info" },
	),
	section(
		"advanced",
		"Advanced Settings",
		[
			{
				id: "host-port",
				label: "Host port",
				kind: "number",
				value: 8080,
				span: "half",
			},
			{
				id: "timeout",
				label: "Deploy timeout",
				kind: "number",
				value: 15,
				unit: "min",
				span: "half",
			},
			{
				id: "exec",
				label: "Remote access",
				kind: "toggle",
				value: false,
				description: "Enable ECS Exec",
			},
		],
		{ advanced: true },
	),
]);

const meta = {
	title: "Properties/Primitives",
	component: PropertyPanel,
	parameters: { layout: "fullscreen" },
	decorators: [
		(Story) => (
			<div className="mp-story-stage">
				<Story />
			</div>
		),
	],
	argTypes: {
		definition: { control: false },
		saveState: {
			control: "select",
			options: ["clean", "dirty", "saving", "saved", "error"],
		},
	},
} satisfies Meta<typeof PropertyPanel>;

export default meta;
type Story = StoryObj<typeof meta>;

export const AllFieldTypes: Story = {
	args: { definition: allFields, saveState: "dirty" },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const cpu = canvas.getByLabelText("CPU").getBoundingClientRect();
		const memory = canvas.getByLabelText("Memory").getBoundingClientRect();
		await expect(Math.abs(cpu.top - memory.top)).toBeLessThan(2);
	},
};
export const DefaultEmptyErrorDisabledLoading: Story = {
	args: { definition: stateFields, saveState: "clean" },
};
export const AdvancedClosed: Story = {
	args: { definition: advancedFields, openAdvanced: false, saveState: "clean" },
};
export const AdvancedOpen: Story = {
	args: { definition: advancedFields, openAdvanced: true, saveState: "clean" },
};
export const Unsaved: Story = {
	args: { definition: allFields, saveState: "dirty" },
};
export const Saving: Story = {
	args: { definition: allFields, saveState: "saving" },
};
export const Saved: Story = {
	args: { definition: allFields, saveState: "saved" },
};
export const SaveError: Story = {
	args: { definition: allFields, saveState: "error" },
};
