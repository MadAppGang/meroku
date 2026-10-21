import { describe, expect, it } from "vitest";
import {
	PROPERTY_PANELS_STORAGE_KEY,
	resolvePropertyPanelsVersion,
} from "../panelFlag";

function memoryStorage(initial?: string) {
	const values = new Map<string, string>();
	if (initial) values.set(PROPERTY_PANELS_STORAGE_KEY, initial);
	return {
		values,
		getItem: (key: string) => values.get(key) ?? null,
		setItem: (key: string, value: string) => {
			values.set(key, value);
		},
	};
}

describe("resolvePropertyPanelsVersion", () => {
	it("defaults to the shipped panels", () => {
		expect(resolvePropertyPanelsVersion("", memoryStorage())).toBe("legacy");
		expect(resolvePropertyPanelsVersion("", null)).toBe("legacy");
	});

	it("turns V3 on from the URL and remembers it", () => {
		const storage = memoryStorage();
		expect(resolvePropertyPanelsVersion("?panels=v3", storage)).toBe("v3");
		expect(resolvePropertyPanelsVersion("", storage)).toBe("v3");
	});

	it("turns V3 off again with ?panels=legacy", () => {
		const storage = memoryStorage("v3");
		expect(resolvePropertyPanelsVersion("?panels=legacy", storage)).toBe(
			"legacy",
		);
		expect(storage.values.get(PROPERTY_PANELS_STORAGE_KEY)).toBe("legacy");
	});

	it("ignores unknown values and falls back to legacy when storage throws", () => {
		const throwing = {
			getItem: () => {
				throw new Error("blocked");
			},
			setItem: () => {
				throw new Error("blocked");
			},
		};
		expect(resolvePropertyPanelsVersion("?panels=v9", throwing)).toBe("legacy");
		expect(resolvePropertyPanelsVersion("?panels=v3", throwing)).toBe("v3");
	});
});
