import { describe, expect, test } from "bun:test";
import type { YamlInfrastructureConfig } from "../types/yamlConfig";
import { reservedBackendEnvError } from "./reservedEnv";

const base = {
	project: "myapp",
	env: "dev",
} as unknown as YamlInfrastructureConfig;

describe("reservedBackendEnvError", () => {
	test("refuses ENV and AWS_S3_BUCKET on the backend", () => {
		expect(reservedBackendEnvError(base, "ENV")).toMatch(/^ENV is reserved/);
		expect(reservedBackendEnvError(base, "AWS_S3_BUCKET")).toMatch(
			/^AWS_S3_BUCKET is reserved: meroku sets it to the backend bucket/,
		);
	});

	test("refuses the data bucket names, the shared ones by prefix", () => {
		expect(reservedBackendEnvError(base, "DATA_BUCKET_NAME")).toBeTruthy();
		expect(reservedBackendEnvError(base, "DATA_BUCKET_SHARED_UPLOADS")).toMatch(
			/^DATA_BUCKET_SHARED_\* is reserved/,
		);
	});

	test("refuses queue and Valkey names only when meroku sets them", () => {
		expect(reservedBackendEnvError(base, "EVENT_QUEUE_URL")).toBeFalsy();
		expect(reservedBackendEnvError(base, "VALKEY_URL")).toBeFalsy();
		const wired = {
			...base,
			workload: { events: { subscribes: [{ detail_type: "order.created" }] } },
			valkey: { enabled: true, access: ["backend"] },
		} as unknown as YamlInfrastructureConfig;
		expect(reservedBackendEnvError(wired, "EVENT_QUEUE_URL")).toBeTruthy();
		expect(reservedBackendEnvError(wired, "VALKEY_URL")).toBeTruthy();
	});

	test("allows every other name", () => {
		expect(reservedBackendEnvError(base, "LOG_LEVEL")).toBeFalsy();
		expect(reservedBackendEnvError(base, "ENVIRONMENT")).toBeFalsy();
	});
});
