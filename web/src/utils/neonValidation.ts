import type { NeonAccessTarget } from "../types/neon";

/**
 * Generate-time validation of an env's Neon settings: the same rules
 * `meroku generate` applies (validateNeonConfigMap in app/), except V-N16
 * (two equal role names), which only Go checks.
 *
 * Both validators read the shared cases in
 * `app/testdata/neon_validation_vectors.json` (Vite alias `@neon-vectors`), so
 * a rule that drifts on one side breaks the other's cases.
 *
 * The input is a loosely typed env document, as loaded from YAML: the YAML can
 * hold any value, and the rules exist to catch exactly the values the types
 * rule out. A typed `YamlInfrastructureConfig` is accepted as is.
 */

export type NeonRuleId =
	| "V-N1"
	| "V-N2"
	| "V-N3"
	| "V-N8"
	| "V-N9"
	| "V-N10"
	| "V-N11"
	| "V-N13"
	| "V-N14"
	| "V-N15"
	| "V-N28";

export interface NeonProblem {
	rule: NeonRuleId;
	/** YAML path of the offending value, e.g. `services[0].neon.schemas[1]`. */
	path: string;
	/** Plain-language problem and fix, for display next to the field. */
	message: string;
	/** The service the problem belongs to, when it is in an access block. */
	target?: NeonAccessTarget;
}

/** A Neon project id, e.g. `fake-project-000000`. */
export const NEON_PROJECT_ID_PATTERN = /^[a-z0-9][a-z0-9-]{2,62}$/;
/** A Neon organization id, e.g. `org-fake-000000`. */
export const NEON_ORG_ID_PATTERN = /^org-[a-z0-9-]{3,64}$/;
/** A Neon region id, e.g. `aws-us-east-1`. */
export const NEON_REGION_ID_PATTERN = /^(aws|azure)-[a-z0-9-]+$/;
/** A Postgres name meroku accepts for databases, schemas and roles. */
export const NEON_IDENTIFIER_PATTERN = /^[a-z_][a-z0-9_]{0,62}$/;

export const NEON_DEFAULT_OWNER_ROLE = "neondb_owner";
export const NEON_DEFAULT_SCHEMAS: readonly string[] = ["public"];
/** The database every created project starts with. */
export const NEON_PROJECT_DEFAULT_DATABASE = "neondb";
export const NEON_DEFAULT_CREATE_PG_VERSION = 17;
export const NEON_CREATE_PG_VERSIONS = [15, 16, 17, 18] as const;
export const NEON_REUSE_PG_VERSIONS = [14, 15, 16, 17, 18] as const;
/** Below this version a reused project cannot give services access. */
export const NEON_MIN_ACCESS_PG_VERSION = 15;

export const NEON_ACCESS_LEVELS = ["readonly", "readwrite"] as const;

// ---------------------------------------------------------------------------
// Single-field checks, for instant inline errors on Advanced manual entry.
// Each returns a plain-language message, or null when the value is fine.
// ---------------------------------------------------------------------------

export function checkNeonProjectId(value: string): string | null {
	if (value.trim() === "") return "Enter the Neon project id.";
	if (!NEON_PROJECT_ID_PATTERN.test(value)) {
		return "A project id has 3 to 63 lower-case letters, digits and dashes, and starts with a letter or digit. Copy it from the Neon console's project settings.";
	}
	return null;
}

export function checkNeonProjectName(value: string): string | null {
	if (value.trim() === "") return "Enter a name for the new project.";
	return null;
}

export function checkNeonOrgId(value: string): string | null {
	if (value.trim() === "") return "Enter the Neon organization id.";
	if (!NEON_ORG_ID_PATTERN.test(value)) {
		return "An organization id starts with org- followed by lower-case letters, digits and dashes. Copy it from the Neon console's organization settings.";
	}
	return null;
}

export function checkNeonRegionId(value: string): string | null {
	if (value.trim() === "") return "Choose a region.";
	if (!NEON_REGION_ID_PATTERN.test(value)) {
		return "A region id starts with aws- or azure-, e.g. aws-us-east-1.";
	}
	return null;
}

/**
 * A database, schema or role name. `what` names the thing in the message,
 * e.g. "database name".
 */
export function checkNeonIdentifier(
	value: string,
	what: string,
): string | null {
	if (value === "") return `Enter a ${what}.`;
	if (!NEON_IDENTIFIER_PATTERN.test(value)) {
		return `A ${what} has up to 63 lower-case letters, digits and underscores, and starts with a letter or underscore.`;
	}
	return null;
}

export function checkNeonSchemaName(value: string): string | null {
	const identifier = checkNeonIdentifier(value, "schema name");
	if (identifier) return identifier;
	if (value.startsWith("pg_") || value === "information_schema") {
		return `${value} is a Postgres system schema. Choose another name.`;
	}
	return null;
}

// ---------------------------------------------------------------------------
// Whole-config validation.
// ---------------------------------------------------------------------------

type Doc = Record<string, unknown>;

function isDoc(value: unknown): value is Doc {
	return value !== null && typeof value === "object" && !Array.isArray(value);
}

function isSet(value: unknown): boolean {
	return value !== undefined && value !== null;
}

function isString(value: unknown): value is string {
	return typeof value === "string";
}

function isIntegerIn(value: unknown, allowed: readonly number[]): boolean {
	return typeof value === "number" && allowed.includes(value);
}

interface AccessEntry {
	/** Null when the block is not a mapping. */
	access: Doc | null;
	path: string;
	target: NeonAccessTarget;
	/** The service's own env vars, to check for a DATABASE_URL of its own. */
	envVarNames: string[];
}

function envVarNamesOf(...sources: unknown[]): string[] {
	const names: string[] = [];
	for (const source of sources) {
		if (Array.isArray(source)) {
			for (const entry of source) {
				if (isDoc(entry) && isString(entry.name)) names.push(entry.name);
			}
		} else if (isDoc(source)) {
			names.push(...Object.keys(source));
		}
	}
	return names;
}

/**
 * Every service that has a `neon` access block, enabled or not: a disabled
 * service keeps its role and its SSM parameter.
 */
function collectAccessEntries(config: Doc): AccessEntry[] {
	const entries: AccessEntry[] = [];
	const workload = isDoc(config.workload) ? config.workload : undefined;
	if (workload && isSet(workload.backend_neon)) {
		entries.push({
			access: isDoc(workload.backend_neon) ? workload.backend_neon : null,
			path: "workload.backend_neon",
			target: { kind: "backend" },
			envVarNames: envVarNamesOf(workload.backend_env_variables),
		});
	}
	const services = Array.isArray(config.services) ? config.services : [];
	services.forEach((service, index) => {
		if (!isDoc(service) || !isSet(service.neon)) return;
		entries.push({
			access: isDoc(service.neon) ? service.neon : null,
			path: `services[${index}].neon`,
			target: {
				kind: "service",
				name: isString(service.name) ? service.name : `#${index + 1}`,
			},
			envVarNames: envVarNamesOf(service.env_vars, service.env_variables),
		});
	});
	return entries;
}

function serviceLabel(target: NeonAccessTarget): string {
	return target.kind === "backend" ? "The backend" : `Service ${target.name}`;
}

function validateProjectBlock(neon: Doc, problems: NeonProblem[]): void {
	const mode = neon.mode;
	if (mode !== "create" && mode !== "reuse") {
		problems.push({
			rule: "V-N1",
			path: "neon.mode",
			message: isSet(mode)
				? `Unknown project mode "${String(mode)}". Choose create (meroku creates a project) or reuse (use an existing one).`
				: "Choose whether meroku creates a new Neon project or uses an existing one.",
		});
		return;
	}

	if (mode === "reuse") {
		if (
			!isString(neon.project_id) ||
			!NEON_PROJECT_ID_PATTERN.test(neon.project_id)
		) {
			problems.push({
				rule: "V-N2",
				path: "neon.project_id",
				message:
					checkNeonProjectId(
						isString(neon.project_id) ? neon.project_id : "",
					) ?? "Enter the Neon project id.",
			});
		}
		if (!isIntegerIn(neon.pg_version, NEON_REUSE_PG_VERSIONS)) {
			problems.push({
				rule: "V-N2",
				path: "neon.pg_version",
				message: isSet(neon.pg_version)
					? `Postgres ${String(neon.pg_version)} is not supported. Choose the project's version: 14, 15, 16, 17 or 18.`
					: "Choose the existing project's Postgres version.",
			});
		}
		if (isSet(neon.project)) {
			problems.push({
				rule: "V-N2",
				path: "neon.project",
				message:
					"New-project settings are not used when reusing a project. Remove them.",
			});
		}
		return;
	}

	if (isSet(neon.pg_version)) {
		problems.push({
			rule: "V-N2",
			path: "neon.pg_version",
			message:
				"A new project's Postgres version is set under the project's settings. Remove this one.",
		});
	}
	if (isSet(neon.project_id)) {
		problems.push({
			rule: "V-N3",
			path: "neon.project_id",
			message:
				"A project id is only used when reusing a project. Remove it, or switch to reusing that project.",
		});
	}
	if (!isDoc(neon.project)) {
		problems.push({
			rule: "V-N3",
			path: "neon.project",
			message: "Give the new project a name, an organization and a region.",
		});
		return;
	}
	const project = neon.project;
	const name = isString(project.name) ? project.name : "";
	const nameProblem = checkNeonProjectName(name);
	if (nameProblem) {
		problems.push({
			rule: "V-N3",
			path: "neon.project.name",
			message: nameProblem,
		});
	}
	const orgId = isString(project.org_id) ? project.org_id : "";
	const orgProblem = checkNeonOrgId(orgId);
	if (orgProblem) {
		problems.push({
			rule: "V-N3",
			path: "neon.project.org_id",
			message: orgProblem,
		});
	}
	const regionId = isString(project.region_id) ? project.region_id : "";
	const regionProblem = checkNeonRegionId(regionId);
	if (regionProblem) {
		problems.push({
			rule: "V-N3",
			path: "neon.project.region_id",
			message: regionProblem,
		});
	}
	if (
		isSet(project.pg_version) &&
		!isIntegerIn(project.pg_version, NEON_CREATE_PG_VERSIONS)
	) {
		problems.push({
			rule: "V-N3",
			path: "neon.project.pg_version",
			message: `meroku creates projects on Postgres 15, 16, 17 or 18, not ${String(project.pg_version)}.`,
		});
	}
}

/** Validates the databases list and returns the names it declares. */
function validateDatabases(neon: Doc, problems: NeonProblem[]): Set<string> {
	const declared = new Set<string>();
	if (!isSet(neon.databases)) return declared;
	if (!Array.isArray(neon.databases)) {
		problems.push({
			rule: "V-N8",
			path: "neon.databases",
			message: "The databases must be a list, each with a name.",
		});
		return declared;
	}
	neon.databases.forEach((database, index) => {
		const path = `neon.databases[${index}]`;
		if (!isDoc(database)) {
			problems.push({
				rule: "V-N8",
				path,
				message: "Each database needs a name.",
			});
			return;
		}
		const name = isString(database.name) ? database.name : "";
		const nameProblem = checkNeonIdentifier(name, "database name");
		if (nameProblem) {
			problems.push({
				rule: "V-N8",
				path: `${path}.name`,
				message: nameProblem,
			});
			return;
		}
		if (declared.has(name)) {
			problems.push({
				rule: "V-N8",
				path: `${path}.name`,
				message: `The database ${name} is listed twice. Keep one.`,
			});
			return;
		}
		declared.add(name);
		if (isSet(database.create) && typeof database.create !== "boolean") {
			problems.push({
				rule: "V-N8",
				path: `${path}.create`,
				message:
					"Choose whether meroku creates this database or it already exists.",
			});
			return;
		}
		if (
			neon.mode === "create" &&
			name === NEON_PROJECT_DEFAULT_DATABASE &&
			database.create === true
		) {
			problems.push({
				rule: "V-N9",
				path: `${path}.create`,
				message: `A new project already comes with the database ${NEON_PROJECT_DEFAULT_DATABASE}. Mark it as existing instead of new.`,
			});
		}
	});
	return declared;
}

function validateAccess(
	entry: AccessEntry,
	/** The declared database names, or null when they are not checked. */
	declared: Set<string> | null,
	problems: NeonProblem[],
): void {
	const { access, path, target } = entry;
	const who = serviceLabel(target);

	if (access === null) {
		problems.push({
			rule: "V-N10",
			path,
			target,
			message: "Choose a database and an access level.",
		});
		return;
	}

	if (access.access !== "readonly" && access.access !== "readwrite") {
		problems.push({
			rule: "V-N10",
			path: `${path}.access`,
			target,
			message: isSet(access.access)
				? `Unknown access "${String(access.access)}". Choose read only or read and write.`
				: "Choose read only or read and write.",
		});
	}

	const database = isString(access.database) ? access.database : "";
	const databaseProblem = checkNeonIdentifier(database, "database name");
	if (databaseProblem) {
		problems.push({
			rule: "V-N10",
			path: `${path}.database`,
			target,
			message: databaseProblem,
		});
	} else if (declared !== null && !declared.has(database)) {
		problems.push({
			rule: "V-N11",
			path: `${path}.database`,
			target,
			message: `The database ${database} is not in the Neon project's database list. Add it there, or choose another.`,
		});
	}

	if (isSet(access.schemas)) {
		const schemas = access.schemas;
		if (!Array.isArray(schemas) || schemas.length === 0) {
			problems.push({
				rule: "V-N10",
				path: `${path}.schemas`,
				target,
				message: "Choose at least one schema, or leave the default (public).",
			});
		} else {
			const seen = new Set<string>();
			schemas.forEach((schema, index) => {
				const schemaPath = `${path}.schemas[${index}]`;
				const name = isString(schema) ? schema : "";
				const schemaProblem = checkNeonSchemaName(name);
				if (schemaProblem) {
					problems.push({
						rule: "V-N10",
						path: schemaPath,
						target,
						message: schemaProblem,
					});
				} else if (seen.has(name)) {
					problems.push({
						rule: "V-N10",
						path: schemaPath,
						target,
						message: `The schema ${name} is listed twice. Keep one.`,
					});
				}
				seen.add(name);
			});
		}
	}

	if (isSet(access.password_version)) {
		const version = access.password_version;
		if (
			typeof version !== "number" ||
			!Number.isInteger(version) ||
			version < 1
		) {
			problems.push({
				rule: "V-N10",
				path: `${path}.password_version`,
				target,
				message: "The password version is a whole number, 1 or more.",
			});
		}
	}

	if (entry.envVarNames.includes("DATABASE_URL")) {
		problems.push({
			rule: "V-N14",
			path,
			target,
			message: `${who} also sets DATABASE_URL in its environment variables. meroku sets it from Neon; remove the variable.`,
		});
	}
}

/**
 * Every generate-time Neon problem in an env document, all at once. An empty
 * list means `meroku generate` accepts the Neon part of the config.
 */
export function validateNeonConfig(config: unknown): NeonProblem[] {
	const problems: NeonProblem[] = [];
	if (!isDoc(config)) return problems;

	const entries = collectAccessEntries(config);

	// With no Neon project, a service's access is not checked further: it
	// cannot be valid until the project exists.
	if (!isSet(config.neon)) {
		for (const entry of entries) {
			problems.push({
				rule: "V-N13",
				path: entry.path,
				target: entry.target,
				message: `${serviceLabel(entry.target)} has a Neon database, but the environment has no Neon project. Set up the Neon project first, or remove this access.`,
			});
		}
		return problems;
	}
	if (!isDoc(config.neon)) {
		problems.push({
			rule: "V-N1",
			path: "neon",
			message:
				"The Neon settings must name a project mode, the project and its databases.",
		});
		return problems;
	}
	const neon = config.neon;

	validateProjectBlock(neon, problems);
	if (isSet(neon.owner_role)) {
		// Empty means the default owner role.
		const ownerRole = neon.owner_role;
		if (
			!isString(ownerRole) ||
			(ownerRole !== "" && !NEON_IDENTIFIER_PATTERN.test(ownerRole))
		) {
			problems.push({
				rule: "V-N15",
				path: "neon.owner_role",
				message:
					checkNeonIdentifier(
						isString(ownerRole) ? ownerRole : "",
						"owner role name",
					) ?? "Choose the owner role.",
			});
		}
	}

	// A service's database is checked against the list only when the list has
	// no problems, so a misspelt or doubled entry is reported once, where it
	// is, not again on every service that uses it.
	const before = problems.length;
	const names = validateDatabases(neon, problems);
	const declared = problems.length === before ? names : null;

	for (const entry of entries) {
		validateAccess(entry, declared, problems);
	}

	if (
		neon.mode === "reuse" &&
		typeof neon.pg_version === "number" &&
		neon.pg_version < NEON_MIN_ACCESS_PG_VERSION &&
		entries.length > 0
	) {
		problems.push({
			rule: "V-N28",
			path: "neon.pg_version",
			message: `On Postgres ${neon.pg_version} any service could create its own tables, so meroku gives services access only on Postgres 15 or later. Upgrade the project in Neon, or remove the services' database access.`,
		});
	}

	return problems;
}

function sameTarget(a: NeonAccessTarget, b: NeonAccessTarget): boolean {
	if (a.kind === "backend" || b.kind === "backend") return a.kind === b.kind;
	return a.name === b.name;
}

/** The problems that belong to one service's access block. */
export function neonProblemsFor(
	problems: readonly NeonProblem[],
	target: NeonAccessTarget,
): NeonProblem[] {
	return problems.filter(
		(problem) =>
			problem.target !== undefined && sameTarget(problem.target, target),
	);
}

/** The problems in the env's `neon` block (project, owner role, databases). */
export function neonProjectProblems(
	problems: readonly NeonProblem[],
): NeonProblem[] {
	return problems.filter((problem) => problem.path.startsWith("neon."));
}

/** One problem in the `meroku generate` message format. */
export function formatNeonProblem(problem: NeonProblem): string {
	return `neon: ${problem.path}: ${problem.message}`;
}
