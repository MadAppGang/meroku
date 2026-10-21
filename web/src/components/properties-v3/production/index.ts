/**
 * properties-v3 production entry point — the ONLY module the shipped app may
 * import from properties-v3 (enforced by check-property-panel-v3-boundaries).
 * The panel itself is lazy so the V3 code and CSS load only when enabled.
 */

import { lazy } from "react";

export {
	CONFIG_PANEL_BINDINGS,
	type ConfigPanelBinding,
} from "./configAdapter";
export { isPropertyPanelsV3Enabled } from "./panelFlag";

export const LazyConfigPropertyPanel = lazy(
	() => import("./ConfigPropertyPanel"),
);
