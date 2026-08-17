import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React from "react";
import { PropertyEnvironmentVariables } from "../../PropertyLayouts";
import "../../property-panel.css";
import { fullscreenParameters, primitiveDecorator } from "../storyConfig";

const variables = [
	{
		id: "port",
		key: "PORT",
		value: "8080",
		origin: "system" as const,
		badge: "System",
		title: "Assigned automatically by Meroku",
	},
	{
		id: "environment",
		key: "ENVIRONMENT",
		value: "production",
		origin: "system" as const,
		badge: "System",
		title: "Assigned automatically by Meroku",
	},
	{
		id: "log-level",
		key: "LOG_LEVEL",
		value: "info",
		origin: "custom" as const,
		badge: "Manual",
		title: "Configured on this service",
	},
	{
		id: "db-host",
		key: "DB_HOST",
		value: "postgres.internal",
		origin: "service" as const,
		badge: "Postgres",
		title: "Assigned by the linked Postgres service",
	},
	{
		id: "api-key",
		key: "API_KEY",
		value: "••••••••••••",
		origin: "secret" as const,
		badge: "shared/prod",
		title: "Inherited from a shared secret group",
	},
];

const meta = {
	title: "Layouts/Environment Variables",
	component: PropertyEnvironmentVariables,
	tags: ["!autodocs"],
	args: { variables },
	parameters: fullscreenParameters,
	decorators: [primitiveDecorator],
	render: (args) => (
		<div
			className="mp-layout-demo mp-compact-panel"
			data-surface="light"
			data-theme="light"
		>
			<div className="mp-layout-demo__section">
				<PropertyEnvironmentVariables {...args} />
			</div>
		</div>
	),
} satisfies Meta<typeof PropertyEnvironmentVariables>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
