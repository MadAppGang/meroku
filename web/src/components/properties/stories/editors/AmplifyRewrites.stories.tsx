import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React from "react";
import { expect, within } from "storybook/test";
import type { YamlInfrastructureConfig } from "../../../../types/yamlConfig";
import { AmplifyRewrites } from "../../../AmplifyRewrites";
import {
	baseConfig,
	ConfigHarness,
	configWith,
	editorDecorator,
	editorParameters,
} from "./editorStoryKit";

const meta = {
	title: "Editors/Amplify Rewrites",
	tags: ["!autodocs"],
	parameters: editorParameters,
	decorators: [editorDecorator],
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

function withRules(
	rules: NonNullable<
		NonNullable<
			YamlInfrastructureConfig["amplify_apps"]
		>[number]["custom_rules"]
	>,
	overrides: Partial<YamlInfrastructureConfig> = {},
): YamlInfrastructureConfig {
	const [app] = baseConfig.amplify_apps ?? [];
	return configWith({
		...overrides,
		amplify_apps: [{ ...app, custom_rules: rules }],
	});
}

function Editor({ initial }: { initial: YamlInfrastructureConfig }) {
	return (
		<ConfigHarness initial={initial}>
			{(config, onChange) => (
				<AmplifyRewrites
					config={config}
					nodeId="amplify-web"
					onConfigChange={onChange}
				/>
			)}
		</ConfigHarness>
	);
}

export const Empty: Story = {
	render: () => <Editor initial={baseConfig} />,
};

export const WorkloadAndTargetRules: Story = {
	name: "Workload and target rules",
	render: () => (
		<Editor
			initial={withRules([
				{ source: "/api/<*>", workload: "backend" },
				{ source: "/orders/<*>", workload: "orders" },
				{ source: "/pricing", target: "/plans", status: "301" },
			])}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			await canvas.findByText(/https:\/\/api\.dev\.example\.com\/orders/),
		).toBeInTheDocument();
	},
};

export const InvalidRules: Story = {
	name: "Invalid rules",
	render: () => (
		<Editor
			initial={withRules([
				{ source: "/<*>", target: "/index.html" },
				{ source: "/a/<*>/b", workload: "backend" },
				{ source: "/jobs/<*>", workload: "worker" },
				{ source: "/docs", target: "" },
			])}
		/>
	),
};

export const SelfHostRefused: Story = {
	name: "API host is the app host",
	render: () => (
		<Editor
			initial={withRules([{ source: "/api/<*>", workload: "backend" }], {
				domain: {
					enabled: true,
					domain_name: "example.com",
					api_domain_prefix: "app",
				},
			})}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			await canvas.findByText(/send the app to itself/),
		).toBeInTheDocument();
	},
};

export const DelegatedZone: Story = {
	name: "Delegated subdomain",
	render: () => (
		<Editor
			initial={withRules([{ source: "/api/<*>", workload: "backend" }], {
				domain: {
					enabled: true,
					domain_name: "example.com",
					api_domain_prefix: "api",
					add_env_domain_prefix: false,
					root_zone_id: "Z0000000000000000000",
				},
			})}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			await canvas.findByText(/https:\/\/api\.dev\.example\.com/),
		).toBeInTheDocument();
	},
};
