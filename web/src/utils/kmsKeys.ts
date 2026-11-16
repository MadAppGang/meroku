import type {
	KmsKeyAccess,
	KmsKeyConnection,
	KmsKeyDefinition,
	KmsKeySpec,
	YamlInfrastructureConfig,
} from "../types/yamlConfig";

// Same rule as app/kms_keys.go and modules/kms_keys.
const KMS_KEY_NAME_PATTERN = /^[a-z][a-z0-9_]{0,30}$/;
const DEFAULT_NAME_PATTERN = /^key(\d+)$/;

/** The key type a key without `key_spec` has. */
export const DEFAULT_KMS_KEY_SPEC: KmsKeySpec = "ECC_NIST_P256";

/** One key type, as the KMS keys node offers it. */
export interface KmsKeySpecChoice {
	spec: KmsKeySpec;
	label: string;
	/** The badge text: the JWT algorithm, or the RSA key size. */
	short: string;
	hint: string;
}

/** Every key type: the signing types, the recommended one first, then the HMAC types. */
export const KMS_KEY_SPEC_CHOICES: KmsKeySpecChoice[] = [
	{
		spec: "ECC_NIST_P256",
		label: "ECC P-256 (ES256)",
		short: "ES256",
		hint: "recommended for JWTs",
	},
	{
		spec: "ECC_NIST_P384",
		label: "ECC P-384 (ES384)",
		short: "ES384",
		hint: "when ES384 is required",
	},
	{
		spec: "ECC_NIST_P521",
		label: "ECC P-521 (ES512)",
		short: "ES512",
		hint: "when ES512 is required",
	},
	{
		spec: "ECC_SECG_P256K1",
		label: "ECC secp256k1 (ES256K)",
		short: "ES256K",
		hint: "blockchain signatures, such as Ethereum",
	},
	{
		spec: "RSA_2048",
		label: "RSA 2048 (RS256)",
		short: "RSA 2048",
		hint: "RS256 compatibility",
	},
	{
		spec: "RSA_3072",
		label: "RSA 3072 (RS256)",
		short: "RSA 3072",
		hint: "RS256 compatibility, longer key",
	},
	{
		spec: "RSA_4096",
		label: "RSA 4096 (RS256)",
		short: "RSA 4096",
		hint: "RS256 compatibility, longest key, slowest",
	},
	{
		spec: "HMAC_224",
		label: "HMAC-224",
		short: "HMAC-224",
		hint: "when that size is required",
	},
	{
		spec: "HMAC_256",
		label: "HMAC-256 (HS256)",
		short: "HS256",
		hint: "shared secret for service-to-service requests",
	},
	{
		spec: "HMAC_384",
		label: "HMAC-384 (HS384)",
		short: "HS384",
		hint: "when HS384 is required",
	},
	{
		spec: "HMAC_512",
		label: "HMAC-512 (HS512)",
		short: "HS512",
		hint: "when HS512 is required",
	},
];

/** Whether `spec` is an HMAC key type. An HMAC key has no public key. */
export function isHmacKeySpec(spec: KmsKeySpec | undefined): boolean {
	return spec?.startsWith("HMAC_") ?? false;
}

/** Whether the key `name` is declared as an HMAC key. */
export function kmsKeyIsHmac(
	config: YamlInfrastructureConfig,
	name: string,
): boolean {
	return isHmacKeySpec(kmsKeyOf(config, name)?.key_spec);
}

/** The choice for `spec`; an unknown or absent spec reads as the default. */
export function kmsKeySpecChoice(
	spec: KmsKeySpec | undefined,
): KmsKeySpecChoice {
	return (
		KMS_KEY_SPEC_CHOICES.find((c) => c.spec === spec) ??
		(KMS_KEY_SPEC_CHOICES[0] as KmsKeySpecChoice)
	);
}

/** What each access lets a workload do with a signing key, in plain words. */
export const KMS_KEY_ACCESS_LABELS: Record<KmsKeyAccess, string> = {
	sign: "Sign",
	verify: "Verify",
	both: "Sign and verify",
};

/** What each access lets a workload do with an HMAC key, in plain words. */
export const KMS_HMAC_KEY_ACCESS_LABELS: Record<KmsKeyAccess, string> = {
	sign: "Create MAC",
	verify: "Check MAC",
	both: "Create and check MAC",
};

/** The names of the keys in the top-level `kms_keys` list, retired ones included. */
export function kmsKeyNames(config: YamlInfrastructureConfig): string[] {
	return (config.kms_keys || []).map((k) => k.name);
}

/** The keys a workload may connect to: every key that is not retired. */
export function activeKmsKeyNames(config: YamlInfrastructureConfig): string[] {
	return (config.kms_keys || []).filter((k) => !k.retired).map((k) => k.name);
}

/**
 * The name a new key gets by default: `key<n>`, where n is one more than the
 * highest number among the names shaped `key<n>`, or 1 when there is none.
 */
export function nextKmsKeyName(names: string[]): string {
	let highest = 0;
	for (const name of names) {
		const match = DEFAULT_NAME_PATTERN.exec(name);
		if (match) highest = Math.max(highest, Number(match[1]));
	}
	return `key${highest + 1}`;
}

/** Why a new key name cannot be used, or `null` when it can. */
export function kmsKeyNameError(
	name: string,
	existing: string[],
): string | null {
	if (name === "") return "Give the key a name.";
	if (!KMS_KEY_NAME_PATTERN.test(name)) {
		return "Use 1 to 31 lower-case letters, digits and underscores, starting with a letter.";
	}
	if (existing.includes(name)) {
		return `A key named "${name}" already exists.`;
	}
	return null;
}

/**
 * The names of the variables a key can give a workload. An HMAC key gives only
 * `arn`; see kmsKeyEnvVariables.
 */
export function kmsKeyEnvNames(name: string): {
	arn: string;
	publicKey: string;
} {
	const upper = name.toUpperCase();
	return {
		arn: `KMS_${upper}_KEY_ARN`,
		publicKey: `KMS_${upper}_PUBLIC_KEY`,
	};
}

/**
 * The variables a connection to the key `name` gives a workload: its ARN, and
 * its public key unless the key is an HMAC key.
 */
export function kmsKeyEnvVariables(
	config: YamlInfrastructureConfig,
	name: string,
): string[] {
	const env = kmsKeyEnvNames(name);
	return kmsKeyIsHmac(config, name) ? [env.arn] : [env.arn, env.publicKey];
}

/** One workload's connection to a key, with the workload named. */
export interface WorkloadKmsKeyConnection extends KmsKeyConnection {
	workload: string;
}

function eachWorkloadConnection(
	config: YamlInfrastructureConfig,
	visit: (
		workload: string,
		connections: KmsKeyConnection[] | undefined,
	) => void,
) {
	visit("backend", config.workload?.kms_keys);
	for (const s of config.services || []) visit(s.name, s.kms_keys);
	for (const t of config.scheduled_tasks || []) visit(t.name, t.kms_keys);
	for (const t of config.event_processor_tasks || []) visit(t.name, t.kms_keys);
}

/** Every workload connected to the key, backend first. */
export function kmsKeyConnectionsOf(
	config: YamlInfrastructureConfig,
	key: string,
): WorkloadKmsKeyConnection[] {
	const out: WorkloadKmsKeyConnection[] = [];
	eachWorkloadConnection(config, (workload, connections) => {
		for (const c of connections || []) {
			if (c.name === key) out.push({ ...c, workload });
		}
	});
	return out;
}

/** The key `name`, or undefined when the config does not declare it. */
export function kmsKeyOf(
	config: YamlInfrastructureConfig,
	name: string,
): KmsKeyDefinition | undefined {
	return config.kms_keys?.find((k) => k.name === name);
}

/** How many workloads hold each access to a key. */
export interface KmsKeyAccessCounts {
	full: number;
	sign: number;
	verify: number;
}

/** Counts connections by access: full (sign and verify), sign only, verify only. */
export function kmsKeyAccessCounts(
	connections: KmsKeyConnection[],
): KmsKeyAccessCounts {
	const counts: KmsKeyAccessCounts = { full: 0, sign: 0, verify: 0 };
	for (const c of connections) {
		if (c.access === "both") counts.full += 1;
		else if (c.access === "sign") counts.sign += 1;
		else counts.verify += 1;
	}
	return counts;
}
