import { describe, expect, test } from "bun:test";
import {
	formatContainerCommand,
	isValidScheduleTimezone,
	isValidSqsQueueArn,
	parseContainerCommand,
	parseMaxRetryAttempts,
} from "../src/utils/taskSettings";

describe("scheduled task settings", () => {
	test("formats list commands and legacy JSON-array scalars", () => {
		expect(formatContainerCommand(["bun", "run", "worker.ts"])).toBe(
			'["bun","run","worker.ts"]',
		);
		expect(formatContainerCommand('["bun","run","worker.ts"]')).toBe(
			'["bun","run","worker.ts"]',
		);
		expect(formatContainerCommand("bin/worker")).toBe("bin/worker");
	});

	test("round-trips command arguments containing commas", () => {
		const command = ["sh", "-c", "echo alpha,beta"];
		expect(parseContainerCommand(formatContainerCommand(command))).toEqual(
			command,
		);
	});

	test("always saves command input as a list", () => {
		expect(parseContainerCommand("bun, run, worker.ts")).toEqual([
			"bun",
			"run",
			"worker.ts",
		]);
		expect(parseContainerCommand('["bun", "run", "worker.ts"]')).toEqual([
			"bun",
			"run",
			"worker.ts",
		]);
		expect(parseContainerCommand("")).toBeUndefined();
	});

	test("validates IANA timezones", () => {
		expect(isValidScheduleTimezone("Australia/Sydney")).toBe(true);
		expect(isValidScheduleTimezone("UTC")).toBe(true);
		expect(isValidScheduleTimezone("Sydney")).toBe(false);
	});

	test("validates the EventBridge Scheduler retry range", () => {
		expect(parseMaxRetryAttempts("0")).toEqual({ value: 0 });
		expect(parseMaxRetryAttempts("185")).toEqual({ value: 185 });
		expect(parseMaxRetryAttempts("")).toEqual({
			error: "Retry attempts must be a whole number from 0 to 185",
		});
		expect(parseMaxRetryAttempts("186")).toEqual({
			error: "Retry attempts must be a whole number from 0 to 185",
		});
		expect(parseMaxRetryAttempts("1.5")).toEqual({
			error: "Retry attempts must be a whole number from 0 to 185",
		});
	});

	test("accepts optional valid SQS queue ARNs", () => {
		expect(isValidSqsQueueArn("")).toBe(true);
		expect(
			isValidSqsQueueArn(
				"arn:aws:sqs:ap-southeast-2:123456789012:scheduled-task-dlq",
			),
		).toBe(true);
		expect(isValidSqsQueueArn("arn:aws:sns:ap-southeast-2:123:topic")).toBe(
			false,
		);
	});
});
