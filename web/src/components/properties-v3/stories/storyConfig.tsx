/**
 * properties-v3 — story configuration.
 *
 * The panel-specification rail used on the Codex branch
 * (PropertyPanelReviewLayout + panelSpecifications.generated) was never
 * merged to main, so V3 stories render the panel on a themed stage only.
 */

import type { Decorator } from "@storybook/react-vite";

export const fullscreenParameters = {
	backgrounds: { disable: true },
	layout: "fullscreen",
} as const;

export const panelDecorator: Decorator = (Story, context) => {
	const theme = (context.parameters?.theme as string | undefined) ?? "light";
	return (
		<div className="mp-story-stage" data-surface={theme} data-theme={theme}>
			<Story />
		</div>
	);
};

export const panelArgTypes = {
	definition: { control: false },
	instance: { control: false },
	saveBehavior: {
		control: "select",
		options: ["succeed", "fail"],
		description: "Mocked save adapter outcome — no AWS calls, ever.",
	},
} as const;
