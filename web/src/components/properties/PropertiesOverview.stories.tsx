import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React from "react";
import { PROPERTY_INVENTORY } from "./nodeCatalog";

function PropertiesInventory() {
	return (
		<div className="mp-story-stage mp-story-stage--inventory">
			<div className="mp-inventory">
				<h1>Meroku property panels</h1>
				<p>
					Complete live component inventory, recomposed into fewer compact views
					from the supplied reference.
				</p>
				<table>
					<thead>
						<tr>
							<th>Node type</th>
							<th>Current component</th>
							<th>Current tabs</th>
							<th>Target views</th>
							<th>State</th>
						</tr>
					</thead>
					<tbody>
						{PROPERTY_INVENTORY.map((item) => (
							<tr key={item.type}>
								<td>
									<code>{item.type}</code>
								</td>
								<td>{item.component}</td>
								<td>{item.currentViews.join(" · ")}</td>
								<td>{item.targetViews.join(" · ")}</td>
								<td>
									<span
										className="mp-inventory__coverage"
										data-coverage={item.coverage}
									>
										{item.coverage}
									</span>
								</td>
							</tr>
						))}
					</tbody>
				</table>
			</div>
		</div>
	);
}

const meta = {
	title: "Properties/Overview",
	component: PropertiesInventory,
	parameters: { layout: "fullscreen" },
} satisfies Meta<typeof PropertiesInventory>;

export default meta;
type Story = StoryObj<typeof meta>;

export const FullComponentList: Story = {};
