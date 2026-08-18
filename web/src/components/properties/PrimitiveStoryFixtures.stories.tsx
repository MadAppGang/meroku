import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React, { type ReactNode, useState } from "react";
import { expect, userEvent, within } from "storybook/test";
import {
	PropertyCapabilityToggle,
	PropertyEditableField,
	PropertyFieldFrame,
	PropertySaveBar,
	PropertyTagList,
} from "./PropertyPrimitives";

function PrimitiveStage({
	title,
	children,
	variant = "default",
}: {
	title: string;
	children: ReactNode;
	variant?: "default" | "field";
}) {
	return (
		<div
			className={`mp-primitive-demo mp-compact-panel${
				variant === "field" ? " mp-primitive-demo--field" : ""
			}`}
			data-surface="light"
			data-theme="light"
		>
			<div className="mp-primitive-demo__title">{title}</div>
			{children}
		</div>
	);
}

function EditableFieldExample({ type = "text" }: { type?: "text" | "number" }) {
	const [value, setValue] = useState<string | number>(
		type === "number" ? 15 : "worker-service",
	);

	return (
		<PrimitiveStage
			title={type === "number" ? "Number field" : "Edit field"}
			variant="field"
		>
			<PropertyEditableField
				id={`primitive-${type}`}
				label={type === "number" ? "Deploy timeout" : "Service name"}
				type={type}
				value={value}
				unit={type === "number" ? "min" : undefined}
				labelPlacement="inline"
				onChange={setValue}
			/>
		</PrimitiveStage>
	);
}

function CheckboxExample() {
	const [checked, setChecked] = useState(true);
	return (
		<PrimitiveStage title="Checkbox field">
			<PropertyCapabilityToggle
				item={{
					id: "primitive-xray",
					label: "X-Ray",
					checked,
					description: "Run distributed tracing",
					onCheckedChange: setChecked,
				}}
			/>
		</PrimitiveStage>
	);
}

function TagListExample() {
	const [values, setValues] = useState(["repo:madappgang/circl:*"]);
	return (
		<PrimitiveStage title="Tag list field">
			<PropertyFieldFrame label="Allowed subjects" span="full" kind="tags">
				<PropertyTagList
					label="Allowed subjects"
					values={values}
					itemLabel="subject"
					onChange={setValues}
				/>
			</PropertyFieldFrame>
		</PrimitiveStage>
	);
}

function SaveBarExample() {
	const [state, setState] = useState<
		"clean" | "dirty" | "saving" | "saved" | "error"
	>("dirty");

	return (
		<PrimitiveStage title="Save bar">
			<fieldset className="mp-primitive-state-picker" aria-label="Save state">
				{(["dirty", "saving", "saved", "error"] as const).map((item) => (
					<button
						type="button"
						key={item}
						aria-pressed={state === item}
						onClick={() => setState(item)}
					>
						{item}
					</button>
				))}
			</fieldset>
			<PropertySaveBar
				state={state}
				onDiscard={() => setState("clean")}
				onApply={() => setState("saving")}
			/>
		</PrimitiveStage>
	);
}

const meta = {
	title: "Components/Properties/Primitives",
	component: PropertyEditableField,
	tags: ["!autodocs"],
	parameters: {
		backgrounds: { disable: true },
		layout: "fullscreen",
	},
	decorators: [
		(Story) => (
			<div
				className="mp-story-stage mp-primitive-stage"
				data-surface="light"
				data-theme="light"
			>
				<Story />
			</div>
		),
	],
} satisfies Meta<typeof PropertyEditableField>;

export default meta;
type Story = StoryObj<typeof meta>;

export const EditField: Story = {
	render: () => <EditableFieldExample />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const field = canvas.getByLabelText("Service name");
		await userEvent.clear(field);
		await userEvent.type(field, "api-service");
		await expect(field).toHaveValue("api-service");
	},
};

export const NumberField: Story = {
	render: () => <EditableFieldExample type="number" />,
};

export const CheckboxField: Story = {
	render: () => <CheckboxExample />,
	play: async ({ canvasElement }) => {
		const checkbox = within(canvasElement).getByRole("checkbox", {
			name: "X-Ray",
		});
		await expect(checkbox).toBeChecked();
		await userEvent.click(checkbox);
		await expect(checkbox).not.toBeChecked();
	},
};

export const TagListField: Story = {
	render: () => <TagListExample />,
};

export const SaveBar: Story = {
	render: () => <SaveBarExample />,
};
