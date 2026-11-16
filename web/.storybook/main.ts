import type { StorybookConfig } from "@storybook/react-vite";

const config: StorybookConfig = {
	stories: [
		"../src/components/properties/stories/**/*.stories.@(js|jsx|mjs|ts|tsx)",
		"../src/components/ui/**/*.stories.@(ts|tsx)",
		"../src/components/NeonNodeProperties.stories.@(ts|tsx)",
		"../src/components/ServiceDatabaseTab.stories.@(ts|tsx)",
		"../src/components/ConfigWarningList.stories.@(ts|tsx)",
		"../src/components/ConfigWarningsStatus.stories.@(ts|tsx)",
		"../src/components/EnvWarningsPanel.stories.@(ts|tsx)",
		"../src/components/ServiceNode.stories.@(ts|tsx)",
	],
	addons: [
		"@storybook/addon-docs",
		"@storybook/addon-a11y",
		"@storybook/addon-vitest",
	],
	framework: {
		name: "@storybook/react-vite",
		options: {},
	},
};

export default config;
