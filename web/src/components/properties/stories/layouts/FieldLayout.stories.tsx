import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React, { useId, useState } from "react";
import { PropertyFieldLayout } from "../../PropertyLayouts";
import {
	PropertyEditableField,
	PropertySelectField,
} from "../../PropertyPrimitives";
import { fullscreenParameters, primitiveDecorator } from "../storyConfig";

const cpuOptions = [{ value: "512", label: "512 (0.5 vCPU)" }];
const memoryOptions = [{ value: "1024", label: "1024 MB" }];

function FieldLayoutExample({ mode }: { mode: "two-column" | "vertical" }) {
	const id = useId();
	const [port, setPort] = useState(8080);
	const [healthPath, setHealthPath] = useState("/health");
	const [cpu, setCpu] = useState("512");
	const [memory, setMemory] = useState("1024");

	return (
		<div
			className="mp-layout-demo mp-compact-panel"
			data-surface="light"
			data-theme="light"
		>
			<div className="mp-layout-demo__section">
				<PropertyFieldLayout mode={mode}>
					<PropertyEditableField
						id={`${id}-port`}
						label="Port"
						type="number"
						value={port}
						onChange={(value) => setPort(Number(value))}
					/>
					<PropertyEditableField
						id={`${id}-health-path`}
						label="Health path"
						value={healthPath}
						mono
						onChange={(value) => setHealthPath(String(value))}
					/>
					<PropertySelectField
						id={`${id}-cpu`}
						label="CPU"
						value={cpu}
						options={cpuOptions}
						theme="light"
						onChange={setCpu}
					/>
					<PropertySelectField
						id={`${id}-memory`}
						label="Memory"
						value={memory}
						options={memoryOptions}
						theme="light"
						onChange={setMemory}
					/>
				</PropertyFieldLayout>
			</div>
		</div>
	);
}

const meta = {
	title: "Layouts/Field Layout",
	component: PropertyFieldLayout,
	tags: ["!autodocs"],
	args: { mode: "two-column", children: null },
	parameters: fullscreenParameters,
	decorators: [primitiveDecorator],
} satisfies Meta<typeof PropertyFieldLayout>;

export default meta;
type Story = StoryObj<typeof meta>;

export const TwoColumnAndVertical: Story = {
	name: "Two columns",
	render: () => <FieldLayoutExample mode="two-column" />,
};

export const Vertical: Story = {
	render: () => <FieldLayoutExample mode="vertical" />,
};
