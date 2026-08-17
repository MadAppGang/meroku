import type { Meta, StoryObj } from "@storybook/react-vite";
import { Braces, Container, Gauge, Settings2 } from "lucide-react";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React from "react";
import { expect, userEvent, within } from "storybook/test";
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

const autoscalingFields = makeDefinition("Autoscaling primitive", [
	section(
		"autoscaling-runtime",
		"Runtime & Scaling",
		[
			{
				id: "autoscaling-desired",
				label: "Desired tasks",
				kind: "number",
				value: 2,
			},
			{
				id: "autoscaling-enabled",
				label: "Autoscaling",
				kind: "toggle",
				value: true,
			},
			{
				id: "autoscaling-minimum",
				label: "Minimum tasks",
				kind: "number",
				value: 1,
			},
			{
				id: "autoscaling-maximum",
				label: "Maximum tasks",
				kind: "number",
				value: 5,
			},
		],
		{
			icon: Gauge,
			description:
				"One shared on/off control with target, desired, min, and max",
		},
	),
]);

const environmentFields = makeDefinition("Environment variables primitive", [
	section(
		"environment-only",
		"Environment Variables",
		[
			{
				id: "environment-variables",
				label: "Variables",
				kind: "key-value",
				value: [
					{
						id: "env-system",
						key: "NODE_ENV",
						value: "production",
						readOnly: true,
					},
					{
						id: "env-service",
						key: "DB_HOST",
						value: "postgres.internal",
						readOnly: true,
					},
					{
						id: "env-manual",
						key: "LOG_LEVEL",
						value: "info",
					},
					{
						id: "env-secret",
						key: "API_KEY",
						value: "sk_live_example",
						secret: true,
					},
				],
			},
		],
		{
			icon: Braces,
			description: "Flat rows with origin badges beside each key",
		},
	),
]);

const tagListFields = makeDefinition("Tag list primitive", [
	section(
		"tag-list",
		"Repository Access",
		[
			{
				id: "allowed-subjects",
				label: "Allowed subjects",
				kind: "tags",
				value: [
					"repo:madappgang/circl:*",
					"repo:madappgang/backend:ref:refs/heads/main",
				],
			},
		],
		{
			icon: Settings2,
			description: "Compact add and remove controls for long structured values",
		},
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

const layoutFields = makeDefinition("Layout patterns", [
	section(
		"layout-pairs",
		"Two-column Layout",
		[
			{ id: "layout-port", label: "Port", kind: "number", value: 8080 },
			{
				id: "layout-health",
				label: "Health path",
				kind: "text",
				value: "/health",
				mono: true,
			},
			{
				id: "layout-cpu",
				label: "CPU",
				kind: "select",
				value: "512",
				options: [{ label: "512 (0.5 vCPU)", value: "512" }],
			},
			{
				id: "layout-memory",
				label: "Memory",
				kind: "select",
				value: "1024",
				options: [{ label: "1024 MB", value: "1024" }],
			},
		],
		{
			icon: Gauge,
			description: "Compatible fields pair automatically",
			layout: "two-column",
		},
	),
	section(
		"layout-vertical",
		"Vertical Layout",
		[
			{
				id: "layout-name",
				label: "Deployment display name",
				kind: "text",
				value: "worker-service",
			},
			{
				id: "layout-timeout",
				label: "Deploy timeout",
				kind: "number",
				value: 15,
				unit: "min",
			},
			{
				id: "layout-command",
				label: "Command override",
				kind: "text",
				value: "npm, start",
				mono: true,
			},
		],
		{
			icon: Container,
			description: "Long or sequential fields get a full row",
			layout: "vertical",
		},
	),
]);

const panelShell: PropertyPanelDefinition = {
	nodeType: "primitive-review",
	name: "Panel shell",
	displayName: "Property primitives",
	icon: Settings2,
	status: "Review",
	statusTone: "info",
	context: "storybook · reference extraction",
	lastUpdated: "Last updated just now",
	deletable: true,
	views: [
		{
			id: "setup",
			label: "Setup",
			sections: [
				section("shell-setup", "Setup", [
					{
						id: "shell-name",
						label: "Name",
						kind: "text",
						value: "worker",
					},
					{
						id: "shell-enabled",
						label: "Enabled",
						kind: "toggle",
						value: true,
					},
				]),
			],
		},
		{
			id: "details",
			label: "Details",
			sections: [
				section("shell-details", "Details", [
					{
						id: "shell-id",
						label: "Resource ID",
						kind: "readonly",
						value: "worker-dev-01",
						mono: true,
					},
				]),
			],
		},
	],
};

const meta = {
	title: "Components/Properties/Layouts",
	component: PropertyPanel,
	tags: ["!autodocs"],
	args: {
		compact: true,
		surface: "light",
	},
	parameters: {
		backgrounds: { disable: true },
		layout: "fullscreen",
	},
	decorators: [
		(Story) => (
			<div className="mp-story-stage" data-surface="light" data-theme="light">
				<Story />
			</div>
		),
	],
	argTypes: {
		definition: { control: false },
		compact: { control: false },
		surface: { control: false },
		saveState: {
			control: "select",
			options: ["clean", "dirty", "saving", "saved", "error"],
		},
	},
} satisfies Meta<typeof PropertyPanel>;

export default meta;
type Story = StoryObj<typeof meta>;

export const PanelShellAndTabs: Story = {
	name: "Panel Shell & Tabs",
	args: { definition: panelShell, saveState: "clean" },
};

export const AllFieldTypes: Story = {
	name: "All Field Types",
	args: { definition: allFields, saveState: "dirty" },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const cpu = canvas.getByLabelText("CPU").getBoundingClientRect();
		const memory = canvas.getByLabelText("Memory").getBoundingClientRect();
		await expect(Math.abs(cpu.top - memory.top)).toBeLessThan(2);
		await expect(canvas.getByRole("checkbox", { name: "X-Ray" })).toBeChecked();
		await expect(canvas.queryByRole("switch")).toBeNull();
		const timeoutLabel = canvas
			.getByText("Deploy timeout")
			.getBoundingClientRect();
		const timeoutControl = canvas
			.getByLabelText("Deploy timeout")
			.getBoundingClientRect();
		await expect(timeoutLabel.bottom).toBeLessThanOrEqual(timeoutControl.top);
		const panel =
			canvasElement.querySelector<HTMLElement>(".meroku-properties");
		const content = canvasElement.querySelector<HTMLElement>(
			".mp-compact-content",
		);
		if (!panel || !content) throw new Error("Property panel shell is missing");
		await expect(panel.clientHeight).toBeLessThanOrEqual(window.innerHeight);
		await expect(getComputedStyle(content).overflowY).toBe("auto");
	},
};

export const LayoutPatterns: Story = {
	name: "Layout Patterns",
	args: { definition: layoutFields, saveState: "clean" },
	play: async ({ canvasElement }) => {
		const layouts = canvasElement.querySelectorAll<HTMLElement>(
			".mp-property-layout",
		);
		await expect(layouts).toHaveLength(2);
		await expect(layouts[0]).toHaveAttribute("data-layout", "two-column");
		await expect(layouts[1]).toHaveAttribute("data-layout", "vertical");
	},
};

export const Autoscaling: Story = {
	args: { definition: autoscalingFields, saveState: "clean" },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const autoscaling = canvas.getByLabelText("Autoscaling");
		await expect(autoscaling).toBeChecked();
		await expect(canvas.getByLabelText("Min")).toHaveValue(1);
		await expect(canvas.getByLabelText("Max")).toHaveValue(5);
		await expect(canvas.queryByLabelText("Desired")).toBeNull();
	},
};

export const EnvironmentVariables: Story = {
	name: "Environment Variables",
	args: { definition: environmentFields, saveState: "clean" },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("System")).toBeVisible();
		await expect(canvas.getByText("postgres")).toBeVisible();
		await expect(canvas.getByText("Manual")).toBeVisible();
		await expect(canvas.getByText("shared/prod")).toBeVisible();
	},
};

export const TagList: Story = {
	name: "Tag List",
	args: { definition: tagListFields, saveState: "clean" },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("button", { name: "Add subject" }),
		).toBeVisible();
		await expect(canvas.queryByRole("button", { name: /^Edit / })).toBeNull();
		await userEvent.click(canvas.getByRole("button", { name: "Add subject" }));
		await userEvent.type(
			canvas.getByLabelText("New subject"),
			"repo:madappgang/storybook:*",
		);
		await userEvent.click(canvas.getByRole("button", { name: "Save subject" }));
		await expect(canvas.getByText("repo:madappgang/storybook:*")).toBeVisible();
		const removeAddedSubject = canvas.getByRole("button", {
			name: "Remove repo:madappgang/storybook:*",
		});
		removeAddedSubject.focus();
		await userEvent.click(removeAddedSubject);
		const removeSubject = canvas.getByRole("button", {
			name: "Remove repo:madappgang/circl:*",
		});
		removeSubject.focus();
		await expect(removeSubject).toBeVisible();
	},
};

export const SharedComponentMap: Story = {
	name: "Shared Component Map",
	args: { definition: panelShell, saveState: "clean" },
	render: () => (
		<div className="mp-inventory">
			<h1>Shared property components</h1>
			<p>One implementation is composed by Service and every draft node.</p>
			<table>
				<thead>
					<tr>
						<th>Component</th>
						<th>Used by</th>
						<th>Overlap</th>
					</tr>
				</thead>
				<tbody>
					{[
						[
							"PropertyPanelShell",
							"Service + all 28 node drafts",
							"Shared once",
						],
						["PropertyPanelHeader", "Every panel", "Shared once"],
						["PropertyPanelTabs", "Setup / Details", "Shared once"],
						["PropertyGroup", "All property groups", "Shared once"],
						[
							"PropertyFieldFrame",
							"Text, number, select, secret",
							"Shared once",
						],
						[
							"PropertyFieldRow",
							"Aligned runtime and paired fields",
							"Shared once",
						],
						[
							"PropertyImageSource",
							"Service containers + ECR registry strategy",
							"Shared once",
						],
						[
							"PropertyContainerProcess",
							"Service + container-backed nodes",
							"Shared once",
						],
						[
							"PropertyAutoscalingGroup",
							"Service + scalable nodes",
							"Shared once",
						],
						[
							"PropertyAutoscalingTarget",
							"Service + scalable nodes",
							"Shared once",
						],
						[
							"PropertyCapabilities",
							"X-Ray, SSH, and boolean capabilities",
							"Shared once",
						],
						[
							"PropertyEnvironmentVariables",
							"Service + env-enabled nodes",
							"Shared once",
						],
						[
							"PropertyTagList",
							"GitHub subjects + every tag-list field",
							"Shared once",
						],
						[
							"PropertyReadonlyRows",
							"Service + generated detail summaries",
							"Shared once",
						],
						[
							"PropertyAdvancedSettings",
							"Service + all advanced fields",
							"Shared once",
						],
						["PropertySaveBar", "Service + all node drafts", "Shared once"],
					].map(([component, usage, overlap]) => (
						<tr key={component}>
							<td>
								<code>{component}</code>
							</td>
							<td>{usage}</td>
							<td>{overlap}</td>
						</tr>
					))}
				</tbody>
			</table>
		</div>
	),
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
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const status = canvas.getByRole("status");
		await expect(status).toHaveTextContent("Unsaved changes");
		await expect(status.closest("footer")).toHaveAttribute(
			"data-state",
			"dirty",
		);
		await expect(
			canvas.getByRole("button", { name: "Apply Changes" }),
		).toBeEnabled();
	},
};
export const Saving: Story = {
	args: { definition: allFields, saveState: "saving" },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const status = canvas.getByRole("status");
		await expect(status).toHaveTextContent("Applying changes…");
		await expect(status.closest("footer")).toHaveAttribute(
			"data-state",
			"saving",
		);
		await expect(
			canvas.getByRole("button", { name: "Applying…" }),
		).toBeDisabled();
	},
};
export const Saved: Story = {
	args: { definition: allFields, saveState: "saved" },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const status = canvas.getByRole("status");
		await expect(status).toHaveTextContent("Changes applied");
		await expect(status.closest("footer")).toHaveAttribute(
			"data-state",
			"saved",
		);
		await expect(
			canvas.getByRole("button", { name: "Apply Changes" }),
		).toBeDisabled();
	},
};
export const SaveError: Story = {
	args: { definition: allFields, saveState: "error" },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const status = canvas.getByRole("status");
		await expect(status).toHaveTextContent("Apply failed — changes preserved");
		await expect(status.closest("footer")).toHaveAttribute(
			"data-state",
			"error",
		);
		await expect(canvas.getByRole("button", { name: "Retry" })).toBeEnabled();
	},
};
