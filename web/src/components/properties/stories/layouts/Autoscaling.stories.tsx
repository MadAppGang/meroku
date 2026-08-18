import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React, { useId, useState } from "react";
import { PropertyAutoscalingGroup } from "../../PropertyLayouts";
import {
	PropertyAutoscalingTarget,
	PropertyEditableField,
} from "../../PropertyPrimitives";
import { fullscreenParameters, primitiveDecorator } from "../storyConfig";

function AutoscalingExample() {
	const id = useId();
	const [enabled, setEnabled] = useState(true);
	const [target, setTarget] = useState(70);
	const [minimum, setMinimum] = useState(1);
	const [maximum, setMaximum] = useState(5);
	const [desired, setDesired] = useState(2);

	return (
		<div
			className="mp-layout-demo mp-compact-panel"
			data-surface="light"
			data-theme="light"
		>
			<div className="mp-layout-demo__section">
				<PropertyAutoscalingGroup
					enabled={enabled}
					onEnabledChange={setEnabled}
					target={
						<PropertyAutoscalingTarget
							value={target}
							theme="light"
							onChange={setTarget}
						/>
					}
					enabledFields={
						<>
							<PropertyEditableField
								id={`${id}-minimum`}
								label="Min"
								type="number"
								value={minimum}
								onChange={(value) => setMinimum(Number(value))}
							/>
							<PropertyEditableField
								id={`${id}-maximum`}
								label="Max"
								type="number"
								value={maximum}
								onChange={(value) => setMaximum(Number(value))}
							/>
						</>
					}
					disabledFields={
						<PropertyEditableField
							id={`${id}-desired`}
							label="Desired"
							type="number"
							value={desired}
							onChange={(value) => setDesired(Number(value))}
						/>
					}
				/>
			</div>
		</div>
	);
}

const meta = {
	title: "Layouts/Autoscaling",
	component: PropertyAutoscalingGroup,
	tags: ["!autodocs"],
	args: {
		enabled: true,
		onEnabledChange: () => undefined,
		enabledFields: null,
		disabledFields: null,
	},
	parameters: fullscreenParameters,
	decorators: [primitiveDecorator],
} satisfies Meta<typeof PropertyAutoscalingGroup>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = { render: () => <AutoscalingExample /> };
