/**
 * Opt-in switch for the properties-v3 node panels in the shipped app.
 *
 * `?panels=v3` turns V3 on and remembers it; `?panels=legacy` turns it off.
 * Without the parameter the remembered choice applies; default is legacy.
 */

export type PropertyPanelsVersion = "v3" | "legacy";

export const PROPERTY_PANELS_STORAGE_KEY = "meroku.propertyPanels";

type FlagStorage = Pick<Storage, "getItem" | "setItem">;

export function resolvePropertyPanelsVersion(
	search: string,
	storage: FlagStorage | null,
): PropertyPanelsVersion {
	const requested = new URLSearchParams(search).get("panels");
	if (requested === "v3" || requested === "legacy") {
		try {
			storage?.setItem(PROPERTY_PANELS_STORAGE_KEY, requested);
		} catch {
			// Storage can be blocked; the URL still applies to this page load.
		}
		return requested;
	}
	try {
		return storage?.getItem(PROPERTY_PANELS_STORAGE_KEY) === "v3"
			? "v3"
			: "legacy";
	} catch {
		return "legacy";
	}
}

function browserStorage(): FlagStorage | null {
	try {
		return window.localStorage;
	} catch {
		return null;
	}
}

export function isPropertyPanelsV3Enabled(): boolean {
	if (typeof window === "undefined") return false;
	return (
		resolvePropertyPanelsVersion(window.location.search, browserStorage()) ===
		"v3"
	);
}
