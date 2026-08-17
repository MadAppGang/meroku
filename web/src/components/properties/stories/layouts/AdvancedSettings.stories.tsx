import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React, { useId, useState } from "react";
import {
	PropertyAdvancedSettings,
	PropertyFieldLayout,
} from "../../PropertyLayouts";
import { PropertyEditableField } from "../../PropertyPrimitives";
import "../../property-panel.css";
import { fullscreenParameters, primitiveDecorator } from "../storyConfig";

function AdvancedSettingsExample({ initialOpen }: { initialOpen: boolean }) {
	const id = useId();
	const [open, setOpen] = useState(initialOpen);
	const [hostPort, setHostPort] = useState(8080);
	const [deployTimeout, setDeployTimeout] = useState(15);

	return (
		<div
			className="mp-layout-demo mp-compact-panel"
			data-surface="light"
			data-theme="light"
		>
			<PropertyAdvancedSettings open={open} onOpenChange={setOpen}>
				<PropertyFieldLayout>
					<PropertyEditableField
						id={`${id}-host-port`}
						label="Host port"
						type="number"
						value={hostPort}
						onChange={(value) => setHostPort(Number(value))}
					/>
					<PropertyEditableField
						id={`${id}-deploy-timeout`}
						label="Deploy timeout"
						type="number"
						value={deployTimeout}
						unit="min"
						onChange={(value) => setDeployTimeout(Number(value))}
					/>
				</PropertyFieldLayout>
			</PropertyAdvancedSettings>
		</div>
	);
}

const meta = {
	title: "Layouts/Advanced Disclosure",
	component: PropertyAdvancedSettings,
	tags: ["!autodocs"],
	args: {
		open: true,
		onOpenChange: () => undefined,
		children: null,
	},
	parameters: fullscreenParameters,
	decorators: [primitiveDecorator],
} satisfies Meta<typeof PropertyAdvancedSettings>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Open: Story = {
	render: () => <AdvancedSettingsExample initialOpen />,
};
export const Closed: Story = {
	render: () => <AdvancedSettingsExample initialOpen={false} />,
};
