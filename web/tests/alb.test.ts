import { describe, expect, test } from "bun:test";
import {
	MAX_ALB_IDLE_TIMEOUT,
	MIN_ALB_IDLE_TIMEOUT,
	parseAlbIdleTimeout,
} from "../src/utils/alb";

describe("parseAlbIdleTimeout", () => {
	test("accepts whole seconds inside the AWS ALB range", () => {
		expect(parseAlbIdleTimeout("300")).toBe(300);
		expect(parseAlbIdleTimeout(` ${MIN_ALB_IDLE_TIMEOUT} `)).toBe(
			MIN_ALB_IDLE_TIMEOUT,
		);
		expect(parseAlbIdleTimeout(String(MAX_ALB_IDLE_TIMEOUT))).toBe(
			MAX_ALB_IDLE_TIMEOUT,
		);
	});

	test("rejects empty, fractional, and out-of-range values", () => {
		expect(parseAlbIdleTimeout("")).toBeNull();
		expect(parseAlbIdleTimeout("1.5")).toBeNull();
		expect(parseAlbIdleTimeout("0")).toBeNull();
		expect(parseAlbIdleTimeout(String(MAX_ALB_IDLE_TIMEOUT + 1))).toBeNull();
	});
});
