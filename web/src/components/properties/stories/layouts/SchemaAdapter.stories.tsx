import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React, { useState } from "react";
import {
	PropertyFieldLayout,
	PropertySchemaField,
} from "../../PropertyLayouts";
import "../../property-panel.css";
import type { PropertyFieldDefinition } from "../../types";
import { fullscreenParameters, primitiveDecorator } from "../storyConfig";

const fields: PropertyFieldDefinition[] = [
	{ id: "adapter-name", label: "Service name", kind: "text" },
	{
		id: "adapter-timeout",
		label: "Deploy timeout",
		kind: "number",
		unit: "min",
	},
	{
		id: "adapter-cpu",
		label: "CPU",
		kind: "select",
		options: [{ value: "512", label: "512 (0.5 vCPU)" }],
	},
	{ id: "adapter-xray", label: "X-Ray", kind: "toggle" },
];

function SchemaAdapterExample() {
	const [values, setValues] = useState<
		Record<string, PropertyFieldDefinition["value"]>
	>({
		"adapter-name": "worker-service",
		"adapter-timeout": 15,
		"adapter-cpu": "512",
		"adapter-xray": true,
	});

	return (
		<div
			className="mp-layout-demo mp-compact-panel"
			data-surface="light"
			data-theme="light"
		>
			<div className="mp-layout-demo__section">
				<PropertyFieldLayout>
					{fields.map((field) => (
						<PropertySchemaField
							key={field.id}
							field={field}
							value={values[field.id]}
							theme="light"
							onChange={(value) =>
								setValues((current) => ({ ...current, [field.id]: value }))
							}
						/>
					))}
				</PropertyFieldLayout>
			</div>
		</div>
	);
}

const meta = {
	title: "Layouts/Schema Adapter",
	component: PropertySchemaField,
	tags: ["!autodocs"],
	args: {
		field: fields[0],
		value: "worker-service",
		theme: "light",
		onChange: () => undefined,
	},
	parameters: fullscreenParameters,
	decorators: [primitiveDecorator],
} satisfies Meta<typeof PropertySchemaField>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = { render: () => <SchemaAdapterExample /> };
