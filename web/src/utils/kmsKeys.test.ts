import { describe, expect, test } from "bun:test";
import type { YamlInfrastructureConfig } from "../types/yamlConfig";
import {
	activeKmsKeyNames,
	isHmacKeySpec,
	KMS_HMAC_KEY_ACCESS_LABELS,
	KMS_KEY_SPEC_CHOICES,
	kmsKeyConnectionsOf,
	kmsKeyEnvNames,
	kmsKeyEnvVariables,
	kmsKeyIsHmac,
	kmsKeyNameError,
	kmsKeySpecChoice,
	nextKmsKeyName,
} from "./kmsKeys";
import { reservedBackendEnvError } from "./reservedEnv";
import {
	removeKmsKeyReferences,
	removeVaultReferences,
	removeWorkloadReferences,
	renameWorkloadReferences,
} from "./workloadReferences";

const config = {
	project: "myapp",
	env: "dev",
	kms_keys: [
		{ name: "auth", key_spec: "ECC_NIST_P256" },
		{ name: "old", retired: true },
	],
	vaults: [{ name: "auth", secrets: [] }],
	workload: {
		kms_keys: [{ name: "auth", access: "sign" }],
		vaults: [{ name: "auth" }],
	},
	services: [
		{ name: "api", kms_keys: [{ name: "auth", access: "verify" }] },
		{ name: "auth", kms_keys: [{ name: "auth", access: "both" }] },
	],
	scheduled_tasks: [
		{ name: "nightly", kms_keys: [{ name: "auth", access: "both" }] },
	],
	event_processor_tasks: [
		{ name: "mailer", kms_keys: [{ name: "auth", access: "verify" }] },
	],
} as unknown as YamlInfrastructureConfig;

describe("nextKmsKeyName", () => {
	test("starts at key1", () => {
		expect(nextKmsKeyName([])).toBe("key1");
		expect(nextKmsKeyName(["auth"])).toBe("key1");
	});

	test("follows the highest key<n>, not the list length", () => {
		expect(nextKmsKeyName(["key1", "key7", "auth"])).toBe("key8");
		expect(nextKmsKeyName(["key3"])).toBe("key4");
	});

	test("ignores names that only look like key<n>", () => {
		expect(nextKmsKeyName(["key", "key2a", "mykey9"])).toBe("key1");
	});
});

describe("kmsKeyNameError", () => {
	test("accepts the documented shape", () => {
		expect(kmsKeyNameError("auth", [])).toBe(null);
		expect(kmsKeyNameError(`a${"b".repeat(30)}`, [])).toBe(null);
	});

	test("refuses empty, bad and duplicate names", () => {
		expect(kmsKeyNameError("", [])).toBe("Give the key a name.");
		expect(kmsKeyNameError("Auth", [])).toMatch(/lower-case/);
		expect(kmsKeyNameError("1key", [])).toMatch(/lower-case/);
		expect(kmsKeyNameError("my-key", [])).toMatch(/lower-case/);
		expect(kmsKeyNameError(`a${"b".repeat(31)}`, [])).toMatch(/lower-case/);
		expect(kmsKeyNameError("old", ["old"])).toBe(
			'A key named "old" already exists.',
		);
	});
});

describe("kms key helpers", () => {
	test("env names are upper-cased", () => {
		expect(kmsKeyEnvNames("auth_v2")).toEqual({
			arn: "KMS_AUTH_V2_KEY_ARN",
			publicKey: "KMS_AUTH_V2_PUBLIC_KEY",
		});
	});

	test("an absent spec reads as ECC P-256", () => {
		expect(kmsKeySpecChoice(undefined).spec).toBe("ECC_NIST_P256");
		expect(kmsKeySpecChoice("RSA_2048").label).toBe("RSA 2048 (RS256)");
	});

	test("offers the HMAC types after the signing types", () => {
		expect(KMS_KEY_SPEC_CHOICES.map((c) => c.spec)).toEqual([
			"ECC_NIST_P256",
			"ECC_NIST_P384",
			"ECC_NIST_P521",
			"ECC_SECG_P256K1",
			"RSA_2048",
			"RSA_3072",
			"RSA_4096",
			"HMAC_224",
			"HMAC_256",
			"HMAC_384",
			"HMAC_512",
		]);
		expect(kmsKeySpecChoice("HMAC_256")).toEqual({
			spec: "HMAC_256",
			label: "HMAC-256 (HS256)",
			short: "HS256",
			hint: "shared secret for service-to-service requests",
		});
	});

	test("tells HMAC keys from signing keys by their type", () => {
		expect(isHmacKeySpec("HMAC_384")).toBe(true);
		expect(isHmacKeySpec("RSA_2048")).toBe(false);
		expect(isHmacKeySpec(undefined)).toBe(false);
		const hmac = {
			...config,
			kms_keys: [{ name: "auth" }, { name: "requests", key_spec: "HMAC_256" }],
		} as unknown as YamlInfrastructureConfig;
		expect(kmsKeyIsHmac(hmac, "requests")).toBe(true);
		expect(kmsKeyIsHmac(hmac, "auth")).toBe(false);
		expect(KMS_HMAC_KEY_ACCESS_LABELS.sign).toBe("Create MAC");
	});

	test("an HMAC key gives only its ARN variable", () => {
		const hmac = {
			...config,
			kms_keys: [{ name: "auth" }, { name: "requests", key_spec: "HMAC_256" }],
		} as unknown as YamlInfrastructureConfig;
		expect(kmsKeyEnvVariables(hmac, "requests")).toEqual([
			"KMS_REQUESTS_KEY_ARN",
		]);
		expect(kmsKeyEnvVariables(hmac, "auth")).toEqual([
			"KMS_AUTH_KEY_ARN",
			"KMS_AUTH_PUBLIC_KEY",
		]);
	});

	test("retired keys are not offered", () => {
		expect(activeKmsKeyNames(config)).toEqual(["auth"]);
	});

	test("lists every workload connected to a key, backend first", () => {
		expect(
			kmsKeyConnectionsOf(config, "auth").map((c) => [c.workload, c.access]),
		).toEqual([
			["backend", "sign"],
			["api", "verify"],
			["auth", "both"],
			["nightly", "both"],
			["mailer", "verify"],
		]);
	});
});

describe("KMS key references", () => {
	test("removing a key drops every connection and nothing else", () => {
		const updates = removeKmsKeyReferences(config, "auth");
		expect(updates.workload?.kms_keys).toBeUndefined();
		expect(updates.workload?.vaults).toEqual([{ name: "auth" }]);
		expect(updates.services?.map((s) => s.kms_keys)).toEqual([
			undefined,
			undefined,
		]);
		expect(updates.scheduled_tasks?.[0]?.kms_keys).toBeUndefined();
		expect(updates.event_processor_tasks?.[0]?.kms_keys).toBeUndefined();
	});

	test("removing a vault or a workload named like a key leaves key connections", () => {
		expect(removeVaultReferences(config, "auth").services).toBeUndefined();
		const updates = removeWorkloadReferences(config, "auth");
		expect(updates.workload).toBeUndefined();
		expect(updates.services).toBeUndefined();
	});

	test("renaming a workload keeps its key connections", () => {
		const updates = renameWorkloadReferences(config, "api", "orders");
		expect(updates.services).toBeUndefined();
	});
});

describe("reserved KMS env names", () => {
	test("the backend may not set what its key connection sets", () => {
		expect(reservedBackendEnvError(config, "KMS_AUTH_KEY_ARN")).toMatch(
			/^KMS_AUTH_KEY_ARN is reserved: .*KMS key auth/,
		);
		expect(reservedBackendEnvError(config, "KMS_AUTH_PUBLIC_KEY")).toBeTruthy();
		expect(reservedBackendEnvError(config, "KMS_OLD_KEY_ARN")).toBe(null);
	});
});
