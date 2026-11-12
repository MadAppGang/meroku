import type { NeonAccessTarget } from "../types/neon";
import type {
	YamlInfrastructureConfig,
	YamlNeonAccess,
} from "../types/yamlConfig";
import {
	NEON_DEFAULT_SCHEMAS,
	NEON_PROJECT_ID_PATTERN,
} from "./neonValidation";

/**
 * The generated choices the Neon panels offer: regions, a default project
 * name, and what the env's services already use. Everything here is read from
 * the env YAML or a static list; live Neon values come from the lookup.
 */

export interface NeonRegionChoice {
	id: string;
	label: string;
}

/** Neon's regions, with the place each one is in. */
export const NEON_REGIONS: readonly NeonRegionChoice[] = [
	{ id: "aws-us-east-1", label: "US East (N. Virginia)" },
	{ id: "aws-us-east-2", label: "US East (Ohio)" },
	{ id: "aws-us-west-2", label: "US West (Oregon)" },
	{ id: "aws-eu-central-1", label: "Europe (Frankfurt)" },
	{ id: "aws-eu-west-2", label: "Europe (London)" },
	{ id: "aws-ap-southeast-1", label: "Asia Pacific (Singapore)" },
	{ id: "aws-ap-southeast-2", label: "Asia Pacific (Sydney)" },
	{ id: "aws-sa-east-1", label: "South America (São Paulo)" },
	{ id: "azure-eastus2", label: "Azure East US 2 (Virginia)" },
	{ id: "azure-westus3", label: "Azure West US 3 (Arizona)" },
	{ id: "azure-gwc", label: "Azure Germany West Central (Frankfurt)" },
];

/** The Neon region in the same AWS region as the env, when Neon has one. */
export function neonRegionForAwsRegion(
	awsRegion: string | undefined,
): string | undefined {
	if (!awsRegion) return undefined;
	const id = `aws-${awsRegion}`;
	return NEON_REGIONS.some((region) => region.id === id) ? id : undefined;
}

/** A label for a region id, including ids not in the static list. */
export function neonRegionLabel(id: string): string {
	const region = NEON_REGIONS.find((choice) => choice.id === id);
	return region ? `${region.label} · ${id}` : id;
}

/**
 * A project name for a new env: `<project>-<env>`, lower-case, with anything
 * a Neon project id would not accept turned into dashes.
 */
export function defaultNeonProjectName(project: string, env: string): string {
	const name = `${project}-${env}`
		.toLowerCase()
		.replace(/[^a-z0-9-]+/g, "-")
		.replace(/-+/g, "-")
		.replace(/^-|-$/g, "");
	return NEON_PROJECT_ID_PATTERN.test(name) ? name : "";
}

/** What a service is called in the panels. */
export function neonTargetLabel(target: NeonAccessTarget): string {
	return target.kind === "backend" ? "Backend" : target.name;
}

export interface NeonAccessEntry {
	target: NeonAccessTarget;
	access: YamlNeonAccess;
}

/** Every service in the env that has a Neon database. */
export function neonAccessEntries(
	config: YamlInfrastructureConfig,
): NeonAccessEntry[] {
	const entries: NeonAccessEntry[] = [];
	const backend = config.workload?.backend_neon;
	if (backend) entries.push({ target: { kind: "backend" }, access: backend });
	for (const service of config.services ?? []) {
		if (service.neon) {
			entries.push({
				target: { kind: "service", name: service.name },
				access: service.neon,
			});
		}
	}
	return entries;
}

function sameTarget(a: NeonAccessTarget, b: NeonAccessTarget): boolean {
	if (a.kind === "backend" || b.kind === "backend") return a.kind === b.kind;
	return a.name === b.name;
}

/** The services that use a database, by label. */
export function neonServicesUsingDatabase(
	config: YamlInfrastructureConfig,
	database: string,
): string[] {
	return neonAccessEntries(config)
		.filter((entry) => entry.access.database === database)
		.map((entry) => neonTargetLabel(entry.target));
}

/**
 * Schema choices for a service on a database: `public`, then every schema
 * the env's other services use on that database, sorted.
 */
export function neonSchemaChoices(
	config: YamlInfrastructureConfig,
	database: string,
	exclude: NeonAccessTarget,
): string[] {
	const used = new Set<string>();
	for (const entry of neonAccessEntries(config)) {
		if (sameTarget(entry.target, exclude)) continue;
		if (entry.access.database !== database) continue;
		for (const schema of entry.access.schemas ?? NEON_DEFAULT_SCHEMAS) {
			used.add(schema);
		}
	}
	const rest = [...used]
		.filter((schema) => !NEON_DEFAULT_SCHEMAS.includes(schema))
		.sort();
	return [...NEON_DEFAULT_SCHEMAS, ...rest];
}
