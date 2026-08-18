import type { Preview } from "@storybook/react-vite";
import "../src/index.css";
import "../src/components/properties/property-panel.css";

const preview: Preview = {
	parameters: {
		a11y: {
			test: "error",
		},
		backgrounds: {
			options: {
				dark: { name: "dark", value: "#09090b" },
				light: { name: "light", value: "#ffffff" },
			},
		},
		controls: {
			matchers: {
				color: /(background|color)$/i,
				date: /Date$/i,
			},
		},
		docs: {
			toc: true,
		},
		layout: "padded",
		options: {
			storySort: {
				order: ["Components", "Layouts", "Panels"],
			},
		},
	},
	initialGlobals: {
		backgrounds: {
			value: "dark",
		},
	},
	tags: ["autodocs", "test"],
};

export default preview;
