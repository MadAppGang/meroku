import type { Decorator } from "@storybook/react-vite";

export const fullscreenParameters = {
	backgrounds: { disable: true },
	layout: "fullscreen",
} as const;

export const lightPanelDecorator: Decorator = (Story) => (
	<div className="mp-story-stage" data-surface="light" data-theme="light">
		<Story />
	</div>
);

export const darkPanelDecorator: Decorator = (Story) => (
	<div className="mp-story-stage" data-theme="dark">
		<Story />
	</div>
);

export const primitiveDecorator: Decorator = (Story) => (
	<div
		className="mp-story-stage mp-primitive-stage"
		data-surface="light"
		data-theme="light"
	>
		<Story />
	</div>
);

export const panelArgTypes = {
	definition: { control: false },
	surface: { control: false },
	compact: { control: false },
	initialView: { control: "text" },
	openAdvanced: { control: "boolean" },
	saveState: {
		control: "select",
		options: ["clean", "dirty", "saving", "saved", "error"],
	},
} as const;
