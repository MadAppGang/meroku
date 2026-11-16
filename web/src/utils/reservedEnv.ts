import type {
	KmsKeyConnection,
	YamlInfrastructureConfig,
} from "../types/yamlConfig";
import { kmsKeyEnvNames } from "./kmsKeys";
import { type WorkloadRef, workloadConnections } from "./workloadConnections";
import { BACKEND_WORKLOAD_NAME } from "./workloadReferences";

/**
 * Container variable names `meroku generate` refuses in
 * `workload.backend_env_variables`, mirroring app/env_reserved.go (ENV) and
 * app/workload_env_reserved.go (the names meroku sets for the backend itself).
 * A YAML variable of the same name would put the name in the container twice.
 */
interface ReservedBackendEnv {
	name: string;
	prefix?: boolean;
	/** Why the name is taken, finishing "… is reserved: <reason>." */
	reason: string;
	applies?: (config: YamlInfrastructureConfig) => boolean;
}

const backendSubscribes = (config: YamlInfrastructureConfig) =>
	(config.workload?.events?.subscribes ?? []).length > 0;

const backendHasValkey = (config: YamlInfrastructureConfig) =>
	config.valkey?.enabled === true &&
	(config.valkey.access ?? []).includes(BACKEND_WORKLOAD_NAME);

const RESERVED_ENV: ReservedBackendEnv = {
	name: "ENV",
	reason:
		"meroku creates the SSM parameter <path>/env for every workload and passes it as ENV. Set it with aws ssm put-parameter",
};

const RESERVED_BACKEND_ENV: ReservedBackendEnv[] = [
	RESERVED_ENV,
	{
		name: "AWS_S3_BUCKET",
		reason: "meroku sets it to the backend bucket",
	},
	{ name: "DATA_BUCKET_NAME", reason: "meroku sets it for the data bucket" },
	{ name: "DATA_BUCKET_PATH", reason: "meroku sets it for the data bucket" },
	{
		name: "DATA_BUCKET_SHARED_",
		prefix: true,
		reason: "meroku sets these for the data bucket's shared folders",
	},
	{
		name: "EVENT_QUEUE_URL",
		reason: "meroku sets it for the backend's event queue",
		applies: backendSubscribes,
	},
	{
		name: "EVENT_DLQ_URL",
		reason: "meroku sets it for the backend's event dead-letter queue",
		applies: backendSubscribes,
	},
	...["VALKEY_URL", "VALKEY_HOST", "VALKEY_PORT"].map((name) => ({
		name,
		reason: "meroku sets it because valkey.access lists backend",
		applies: backendHasValkey,
	})),
];

/**
 * The error `meroku generate` would give for `name` in the backend's
 * variables, or null when the name is free.
 */
export function reservedBackendEnvError(
	config: YamlInfrastructureConfig,
	name: string,
): string | null {
	for (const rule of RESERVED_BACKEND_ENV) {
		const hit = rule.prefix ? name.startsWith(rule.name) : name === rule.name;
		if (!hit || (rule.applies && !rule.applies(config))) continue;
		const shown = rule.prefix ? `${rule.name}*` : rule.name;
		return `${shown} is reserved: ${rule.reason}.`;
	}
	return kmsKeyEnvError(config.workload?.kms_keys, name);
}

/**
 * The error `meroku generate` gives for a workload variable that a KMS key
 * connection of that workload sets, or null when no connection sets it.
 */
export function kmsKeyEnvError(
	connections: KmsKeyConnection[] | undefined,
	name: string,
): string | null {
	for (const connection of connections ?? []) {
		const env = kmsKeyEnvNames(connection.name);
		if (name === env.arn || name === env.publicKey) {
			return `${name} is reserved: meroku sets it because this workload uses the KMS key ${connection.name}.`;
		}
	}
	return null;
}

/**
 * The error `meroku generate` would give for `name` in the YAML variables of
 * `workload`, or null when the name is free. The backend has its own list;
 * every other workload has ENV and the names of the KMS keys it connects to.
 */
export function reservedWorkloadEnvError(
	config: YamlInfrastructureConfig,
	workload: WorkloadRef,
	name: string,
): string | null {
	if (workload.kind === "backend") return reservedBackendEnvError(config, name);
	if (name === RESERVED_ENV.name) {
		return `${RESERVED_ENV.name} is reserved: ${RESERVED_ENV.reason}.`;
	}
	return kmsKeyEnvError(
		workloadConnections(config, workload, "kms_keys"),
		name,
	);
}

/**
 * The workload's own YAML variables: `backend_env_variables` for the backend,
 * `env_vars` for a service, `environment_variables` for a task. `{}` when
 * absent.
 */
export function workloadYamlEnv(
	config: YamlInfrastructureConfig,
	workload: WorkloadRef,
): Record<string, string> {
	switch (workload.kind) {
		case "backend":
			return config.workload?.backend_env_variables ?? {};
		case "service":
			return (
				config.services?.find((s) => s.name === workload.name)?.env_vars ?? {}
			);
		case "scheduled-task":
			return (
				config.scheduled_tasks?.find((t) => t.name === workload.name)
					?.environment_variables ?? {}
			);
		case "event-task":
			return (
				config.event_processor_tasks?.find((t) => t.name === workload.name)
					?.environment_variables ?? {}
			);
	}
}

/**
 * The names among `envNames` that a connection to the KMS key `key` sets,
 * in the order KMS_<NAME>_KEY_ARN, KMS_<NAME>_PUBLIC_KEY.
 */
export function kmsKeyEnvCollisions(
	key: string,
	envNames: Iterable<string>,
): string[] {
	const present = new Set(envNames);
	const env = kmsKeyEnvNames(key);
	return [env.arn, env.publicKey].filter((n) => present.has(n));
}
