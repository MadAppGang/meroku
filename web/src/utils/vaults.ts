import type {
	VaultConnection,
	YamlInfrastructureConfig,
} from "../types/yamlConfig";

// Same rules as app/vaults.go and modules/vaults.
const VAULT_NAME_PATTERN = /^[a-z0-9]([a-z0-9_-]{0,30}[a-z0-9])?$/;
const VAULT_PREFIX_PATTERN = /^[A-Z][A-Z0-9_]*$/;

/** The SSM path of a vault, with its trailing slash. */
export function vaultPath(config: YamlInfrastructureConfig, name: string) {
	return `/${config.env}/${config.project}/vault/${name}/`;
}

/** The names of the vaults declared in the top-level `vaults` list. */
export function vaultNames(config: YamlInfrastructureConfig): string[] {
	return (config.vaults || []).map((v) => v.name);
}

/** Why a new vault name cannot be used, or `null` when it can. */
export function vaultNameError(
	name: string,
	existing: string[],
): string | null {
	if (name === "") return null;
	if (!VAULT_NAME_PATTERN.test(name)) {
		return "Use 1 to 32 lower-case letters, digits, hyphens and underscores, starting and ending with a letter or a digit.";
	}
	if (existing.includes(name)) {
		return `A vault named "${name}" already exists.`;
	}
	return null;
}

/** Why a connection prefix cannot be used, or `null` when it can. */
export function vaultPrefixError(prefix: string): string | null {
	if (prefix === "" || VAULT_PREFIX_PATTERN.test(prefix)) return null;
	return "Use upper-case letters, digits and underscores, starting with a letter.";
}

/** The secret name a parameter becomes through a connection. */
export function vaultSecretName(prefix: string | undefined, parameter: string) {
	const name = parameter.toUpperCase();
	return prefix ? `${prefix}_${name}` : name;
}

/** One workload's connection to a vault, with the workload named. */
export interface WorkloadVaultConnection extends VaultConnection {
	workload: string;
}

/** Every workload connected to the vault, backend first. */
export function vaultConnectionsOf(
	config: YamlInfrastructureConfig,
	vault: string,
): WorkloadVaultConnection[] {
	const out: WorkloadVaultConnection[] = [];
	const add = (workload: string, connections?: VaultConnection[]) => {
		for (const c of connections || []) {
			if (c.name === vault) out.push({ ...c, workload });
		}
	};
	add("backend", config.workload?.vaults);
	for (const s of config.services || []) add(s.name, s.vaults);
	for (const t of config.scheduled_tasks || []) add(t.name, t.vaults);
	for (const t of config.event_processor_tasks || []) add(t.name, t.vaults);
	return out;
}
