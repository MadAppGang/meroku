import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React, { useState } from "react";
import {
	PropertyContainerProcess,
	PropertyImageSource,
} from "../../PropertyLayouts";
import "../../property-panel.css";
import { fullscreenParameters, primitiveDecorator } from "../storyConfig";

const sources = [
	{
		id: "default",
		label: "Default",
		value: "123456789.dkr.ecr.ap-southeast-2.amazonaws.com/circl",
	},
	{
		id: "custom",
		label: "Custom",
		value: "public.ecr.aws/example/api",
	},
];

function ImageSourceExample() {
	const [source, setSource] = useState("default");
	const [command, setCommand] = useState("npm, start");
	return (
		<div
			className="mp-layout-demo mp-compact-panel"
			data-surface="light"
			data-theme="light"
		>
			<PropertyImageSource
				title="Registry Strategy"
				description="Repository ownership and source account"
				source={source}
				sources={sources}
				suffix="latest"
				commandValue={command}
				onSourceChange={setSource}
				onCommandChange={setCommand}
			/>
		</div>
	);
}

function ContainerProcessExample() {
	const [mode, setMode] = useState<"default" | "custom" | "shared">("default");
	const [command, setCommand] = useState("npm, start");
	return (
		<div
			className="mp-layout-demo mp-compact-panel"
			data-surface="light"
			data-theme="light"
		>
			<PropertyContainerProcess
				imageMode={mode}
				imageValues={{
					default: "123456789.dkr.ecr.ap-southeast-2.amazonaws.com/circl",
					custom: "public.ecr.aws/example/api",
					shared: "shared/api",
				}}
				commandValue={command}
				onImageModeChange={setMode}
				onCommandChange={setCommand}
			/>
		</div>
	);
}

const meta = {
	title: "Layouts/Image Source",
	component: PropertyImageSource,
	tags: ["!autodocs"],
	args: {
		title: "Registry Strategy",
		description: "Repository ownership and source account",
		source: "default",
		sources,
		onSourceChange: () => undefined,
	},
	parameters: fullscreenParameters,
	decorators: [primitiveDecorator],
} satisfies Meta<typeof PropertyImageSource>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Registry: Story = { render: () => <ImageSourceExample /> };
export const ContainerProcess: Story = {
	name: "Container process",
	render: () => <ContainerProcessExample />,
};
