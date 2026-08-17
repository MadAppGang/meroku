import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React, { useState } from "react";
import {
	PropertyGroup,
	PropertyPanelShell,
	PropertyReadonlyRows,
} from "../../PropertyLayouts";
import { PropertySaveBar } from "../../PropertyPrimitives";
import "../../property-panel.css";
import { fullscreenParameters, lightPanelDecorator } from "../storyConfig";

function PanelShellExample({ long = false }: { long?: boolean }) {
	const [activeView, setActiveView] = useState("setup");
	const groups = long
		? Array.from({ length: 8 }, (_, index) => index + 1)
		: [1];

	return (
		<PropertyPanelShell
			name="terminator"
			meta={
				<>
					<span className="mp-compact-header__running">● Running</span>
					<span aria-hidden="true">|</span>
					<span>ap-southeast-2</span>
				</>
			}
			ariaLabel="Panel shell example"
			theme="light"
			views={[
				{ id: "setup", label: "Setup" },
				{ id: "details", label: "Details" },
			]}
			activeView={activeView}
			onViewChange={setActiveView}
			footer={
				<PropertySaveBar
					state="clean"
					onDiscard={() => undefined}
					onApply={() => undefined}
				/>
			}
		>
			{groups.map((group) => (
				<PropertyGroup key={group} title={`Shared group ${group}`}>
					<PropertyReadonlyRows
						rows={[
							["Region", "ap-southeast-2"],
							["State", activeView === "setup" ? "Editable" : "Healthy"],
						]}
					/>
				</PropertyGroup>
			))}
		</PropertyPanelShell>
	);
}

const meta = {
	title: "Layouts/Panel Shell",
	component: PropertyPanelShell,
	tags: ["!autodocs"],
	args: {
		name: "terminator",
		meta: null,
		ariaLabel: "Panel shell example",
		theme: "light",
		views: [],
		activeView: "setup",
		onViewChange: () => undefined,
		children: null,
		footer: null,
	},
	parameters: fullscreenParameters,
	decorators: [lightPanelDecorator],
} satisfies Meta<typeof PropertyPanelShell>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Tabs: Story = { render: () => <PanelShellExample /> };
export const ScrollableContent: Story = {
	name: "Scrollable content",
	render: () => <PanelShellExample long />,
};
