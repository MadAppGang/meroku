import type { YamlInfrastructureConfig } from "../types/yamlConfig";
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

const RESERVED_BACKEND_ENV: ReservedBackendEnv[] = [
	{
		name: "ENV",
		reason:
			"meroku creates the SSM parameter <path>/env for every workload and passes it as ENV. Set it with aws ssm put-parameter",
	},
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
	return null;
}
