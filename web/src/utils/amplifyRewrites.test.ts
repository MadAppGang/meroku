import { describe, expect, test } from "bun:test";
import { amplifyTargetPath, shadowingService } from "./amplifyRewrites";

describe("amplifyTargetPath", () => {
	test("keeps the backend source as written", () => {
		expect(amplifyTargetPath({ source: "/api/<*>", workload: "backend" })).toBe(
			"/api/<*>",
		);
	});

	test("drops a service's own prefix", () => {
		expect(amplifyTargetPath({ source: "/api/<*>", workload: "api" })).toBe(
			"/<*>",
		);
	});

	test("an explicit path wins", () => {
		expect(
			amplifyTargetPath({
				source: "/api/<*>",
				workload: "backend",
				path: "/v1/<*>",
			}),
		).toBe("/v1/<*>");
	});
});

describe("shadowingService", () => {
	const routed = ["api", "files"];

	const shadowed: Array<[string, string]> = [
		["/api/<*>", "api"],
		["/api", "api"],
		["/api<*>", "api"],
		["/files/upload", "files"],
	];
	for (const [path, want] of shadowed) {
		test(`${path} reaches ${want}`, () => {
			expect(shadowingService(path, routed)).toBe(want);
		});
	}

	for (const path of ["/v1/<*>", "/apis/<*>", "/<*>", ""]) {
		test(`"${path}" reaches the backend`, () => {
			expect(shadowingService(path, routed)).toBe(null);
		});
	}

	test("a service without a route shadows nothing", () => {
		expect(shadowingService("/api/<*>", [])).toBe(null);
	});
});
