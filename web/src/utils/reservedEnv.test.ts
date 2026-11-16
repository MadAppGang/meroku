import { describe, expect, test } from "bun:test";
import type { YamlInfrastructureConfig } from "../types/yamlConfig";
import {
	kmsKeyEnvCollisions,
	reservedBackendEnvError,
	reservedWorkloadEnvError,
	workloadYamlEnv,
} from "./reservedEnv";

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
		expect(reservedBackendEnvError(base, "EVENT_DLQ_URL")).toBeFalsy();
		expect(reservedBackendEnvError(base, "VALKEY_URL")).toBeFalsy();
		const wired = {
			...base,
			workload: { events: { subscribes: [{ detail_type: "order.created" }] } },
			valkey: { enabled: true, access: ["backend"] },
		} as unknown as YamlInfrastructureConfig;
		expect(reservedBackendEnvError(wired, "EVENT_QUEUE_URL")).toBeTruthy();
		expect(reservedBackendEnvError(wired, "EVENT_DLQ_URL")).toBeTruthy();
		expect(reservedBackendEnvError(wired, "VALKEY_URL")).toBeTruthy();
	});

	test("allows every other name", () => {
		expect(reservedBackendEnvError(base, "LOG_LEVEL")).toBeFalsy();
		expect(reservedBackendEnvError(base, "ENVIRONMENT")).toBeFalsy();
	});
});

describe("reservedWorkloadEnvError", () => {
	const config = {
		...base,
		services: [
			{ name: "orders", kms_keys: [{ name: "auth", access: "sign" }] },
		],
		scheduled_tasks: [
			{ name: "nightly", kms_keys: [{ name: "auth", access: "verify" }] },
		],
		event_processor_tasks: [
			{ name: "ingest", kms_keys: [{ name: "webhooks", access: "both" }] },
		],
	} as unknown as YamlInfrastructureConfig;
	const orders = { kind: "service", name: "orders" } as const;

	test("refuses the variables of a key the workload connects to", () => {
		expect(
			reservedWorkloadEnvError(config, orders, "KMS_AUTH_KEY_ARN"),
		).toMatch(/^KMS_AUTH_KEY_ARN is reserved: .*KMS key auth/);
		expect(
			reservedWorkloadEnvError(
				config,
				{ kind: "scheduled-task", name: "nightly" },
				"KMS_AUTH_PUBLIC_KEY",
			),
		).toBeTruthy();
		expect(
			reservedWorkloadEnvError(
				config,
				{ kind: "event-task", name: "ingest" },
				"KMS_WEBHOOKS_KEY_ARN",
			),
		).toBeTruthy();
	});

	test("allows the variables of a key another workload connects to", () => {
		expect(
			reservedWorkloadEnvError(
				config,
				{ kind: "event-task", name: "ingest" },
				"KMS_AUTH_KEY_ARN",
			),
		).toBeFalsy();
	});

	test("refuses ENV everywhere and leaves backend-only names free", () => {
		expect(reservedWorkloadEnvError(config, orders, "ENV")).toMatch(
			/^ENV is reserved/,
		);
		expect(
			reservedWorkloadEnvError(config, orders, "AWS_S3_BUCKET"),
		).toBeFalsy();
	});
});

describe("kmsKeyEnvCollisions", () => {
	test("names the key's variables present among the workload's", () => {
		expect(
			kmsKeyEnvCollisions("auth", ["LOG_LEVEL", "KMS_AUTH_PUBLIC_KEY"]),
		).toEqual(["KMS_AUTH_PUBLIC_KEY"]);
		expect(kmsKeyEnvCollisions("auth", ["KMS_WEBHOOKS_KEY_ARN"])).toEqual([]);
	});
});

describe("workloadYamlEnv", () => {
	test("reads each workload's own variable map", () => {
		const config = {
			...base,
			workload: { backend_env_variables: { A: "1" } },
			services: [{ name: "orders", env_vars: { B: "2" } }],
			scheduled_tasks: [{ name: "nightly", environment_variables: { C: "3" } }],
		} as unknown as YamlInfrastructureConfig;
		expect(
			workloadYamlEnv(config, { kind: "backend", name: "backend" }),
		).toEqual({ A: "1" });
		expect(
			workloadYamlEnv(config, { kind: "service", name: "orders" }),
		).toEqual({ B: "2" });
		expect(
			workloadYamlEnv(config, { kind: "scheduled-task", name: "nightly" }),
		).toEqual({ C: "3" });
		expect(
			workloadYamlEnv(config, { kind: "event-task", name: "ingest" }),
		).toEqual({});
	});
});
