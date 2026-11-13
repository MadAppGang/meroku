import type { NeonAccessLevel, NeonAccessTarget } from "../types/neon";
import type {
	NeonCreatePgVersion,
	NeonReusePgVersion,
	YamlInfrastructureConfig,
	YamlNeonAccess,
	YamlNeonConfig,
	YamlNeonDatabase,
} from "../types/yamlConfig";
import {
	NEON_DEFAULT_CREATE_PG_VERSION,
	NEON_DEFAULT_OWNER_ROLE,
	NEON_DEFAULT_SCHEMAS,
} from "./neonValidation";

/**
 * Building the config updates the Neon panels emit.
 *
 * `neon` is not one of the merged sections in `configMerge.ts`, so an update
 * replaces it whole. Every emitter therefore sends the complete object from
 * `buildNeonUpdate`, and a switch from create to reuse leaves no `project`
 * behind. Access blocks are replaced whole too; removing one sets the key to
 * `undefined`, which the save path prunes before writing the YAML.
 *
 * The panels hold a draft and save it with one Save: the project fields, the
 * owner role and database names force Terraform to replace what they name, so
 * they are never written per keystroke.
 */

/**
 * The editable state of the env's Neon block. It keeps both modes' fields, so
 * flipping the mode back and forth in the panel loses nothing; only the
 * active mode's fields are written.
 */
export interface NeonDraft {
	mode: "create" | "reuse";
	create: {
		name: string;
		org_id: string;
		region_id: string;
		pg_version: NeonCreatePgVersion;
	};
	reuse: {
		project_id: string;
		/** Unset until the project is looked up or chosen under Advanced. */
		pg_version?: NeonReusePgVersion;
	};
	/** The effective owner role: `neondb_owner` when the YAML has none. */
	owner_role: string;
	databases: YamlNeonDatabase[];
}

/** Defaults for a new Neon block, e.g. a project named after the env. */
export interface NeonDraftDefaults {
	projectName?: string;
	orgId?: string;
	regionId?: string;
}

/** The draft for an env's current `neon` block, or for a new one. */
export function neonDraftFromConfig(
	neon: YamlNeonConfig | undefined,
	defaults: NeonDraftDefaults = {},
): NeonDraft {
	return {
		mode: neon?.mode ?? "create",
		create: {
			name: neon?.project?.name ?? defaults.projectName ?? "",
			org_id: neon?.project?.org_id ?? defaults.orgId ?? "",
			region_id: neon?.project?.region_id ?? defaults.regionId ?? "",
			pg_version:
				neon?.project?.pg_version ??
				(NEON_DEFAULT_CREATE_PG_VERSION as NeonCreatePgVersion),
		},
		reuse: {
			project_id: neon?.project_id ?? "",
			pg_version: neon?.pg_version,
		},
		owner_role: neon?.owner_role ?? NEON_DEFAULT_OWNER_ROLE,
		databases: (neon?.databases ?? []).map((database) => ({ ...database })),
	};
}

/**
 * The complete `neon` object for a draft, keys in the YAML reference order.
 *
 * `saved` is the env's current `neon` block: `owner_role` is written when it
 * differs from `neondb_owner` or when the YAML already names it, so opening and
 * saving the panel does not add a line the user never wrote.
 */
export function buildNeonUpdate(
	draft: NeonDraft,
	saved?: YamlNeonConfig,
): YamlNeonConfig {
	const neon: YamlNeonConfig = { mode: draft.mode };

	if (draft.mode === "reuse") {
		neon.project_id = draft.reuse.project_id.trim();
		if (draft.reuse.pg_version !== undefined) {
			neon.pg_version = draft.reuse.pg_version;
		}
	} else {
		neon.project = {
			name: draft.create.name.trim(),
			org_id: draft.create.org_id.trim(),
			region_id: draft.create.region_id.trim(),
			pg_version: draft.create.pg_version,
		};
	}

	const ownerRole = draft.owner_role.trim();
	if (
		ownerRole !== "" &&
		(ownerRole !== NEON_DEFAULT_OWNER_ROLE || saved?.owner_role !== undefined)
	) {
		neon.owner_role = ownerRole;
	}

	neon.databases = draft.databases.map((database) => ({
		name: database.name.trim(),
		create: database.create,
	}));

	return neon;
}

/** The access block for one service, or undefined when it has none. */
export function getNeonAccess(
	config: YamlInfrastructureConfig,
	target: NeonAccessTarget,
): YamlNeonAccess | undefined {
	if (target.kind === "backend") return config.workload?.backend_neon;
	return config.services?.find((service) => service.name === target.name)?.neon;
}

/** The editable state of one service's access block. */
export interface NeonAccessDraft {
	database: string;
	schemas: string[];
	access: NeonAccessLevel;
	/** Kept as saved; absent means 1. */
	password_version?: number;
}

export function neonAccessDraftFromConfig(
	access: YamlNeonAccess | undefined,
	defaultDatabase = "",
): NeonAccessDraft {
	return {
		database: access?.database ?? defaultDatabase,
		schemas: [...(access?.schemas ?? NEON_DEFAULT_SCHEMAS)],
		access: access?.access ?? "readonly",
		password_version: access?.password_version,
	};
}

function isDefaultSchemas(schemas: readonly string[]): boolean {
	return (
		schemas.length === NEON_DEFAULT_SCHEMAS.length &&
		schemas.every((schema, index) => schema === NEON_DEFAULT_SCHEMAS[index])
	);
}

/**
 * The complete access block for a draft. `schemas` is left out when it is the
 * default (`[public]`) and the YAML did not list it.
 */
export function buildNeonAccess(
	draft: NeonAccessDraft,
	saved?: YamlNeonAccess,
): YamlNeonAccess {
	const schemas = draft.schemas.map((schema) => schema.trim());
	const writeSchemas =
		!isDefaultSchemas(schemas) || saved?.schemas !== undefined;
	return {
		database: draft.database.trim(),
		...(writeSchemas ? { schemas } : {}),
		access: draft.access,
		...(draft.password_version !== undefined
			? { password_version: draft.password_version }
			: {}),
	};
}

/**
 * The config update that sets one service's access block, or removes it when
 * `access` is undefined. `workload` is merged field by field, so the backend's
 * update carries only `backend_neon`; a named service's update carries the
 * whole `services` list, because lists are replaced on merge.
 */
export function buildNeonAccessUpdate(
	config: YamlInfrastructureConfig,
	target: NeonAccessTarget,
	access: YamlNeonAccess | undefined,
): Partial<YamlInfrastructureConfig> {
	if (target.kind === "backend") {
		return { workload: { backend_neon: access } };
	}
	return {
		services: (config.services ?? []).map((service) =>
			service.name === target.name ? { ...service, neon: access } : service,
		),
	};
}

/**
 * A saved change that replaces or releases something in Neon, for the
 * "changing this replaces …" alerts. Which ones apply depends on whether the
 * project is deployed; the panel decides that from the lookup.
 */
export type NeonReplacingChange =
	| {
			kind: "project_field";
			field: "org_id" | "region_id" | "pg_version";
			message: string;
	  }
	| { kind: "owner_role"; message: string }
	| { kind: "database_renamed"; from: string; to: string; message: string }
	| { kind: "database_removed"; name: string; message: string }
	| { kind: "mode_to_reuse"; message: string };

const PROJECT_FIELD_LABELS = {
	org_id: "organization",
	region_id: "region",
	pg_version: "Postgres version",
} as const;

/**
 * The changes between the saved `neon` block and the next one that would
 * replace a created project or database, or release one meroku created.
 * meroku never deletes Neon data: a replacement is refused at deploy, and a
 * released database stays in Neon.
 */
export function neonReplacingChanges(
	saved: YamlNeonConfig | undefined,
	next: YamlNeonConfig,
): NeonReplacingChange[] {
	if (!saved) return [];
	const changes: NeonReplacingChange[] = [];

	if (saved.mode === "create" && next.mode === "reuse") {
		changes.push({
			kind: "mode_to_reuse",
			message:
				"The project meroku created stays in Neon with its data; meroku stops managing it and uses the project you chose.",
		});
	}

	if (saved.mode === "create" && next.mode === "create") {
		const fields = ["org_id", "region_id", "pg_version"] as const;
		for (const field of fields) {
			const before =
				field === "pg_version"
					? (saved.project?.pg_version ?? NEON_DEFAULT_CREATE_PG_VERSION)
					: saved.project?.[field];
			const after =
				field === "pg_version"
					? (next.project?.pg_version ?? NEON_DEFAULT_CREATE_PG_VERSION)
					: next.project?.[field];
			if (before !== after) {
				changes.push({
					kind: "project_field",
					field,
					message: `Changing the ${PROJECT_FIELD_LABELS[field]} replaces the project and its data, so a deployed project refuses it. To start over, disconnect Neon from this environment and deploy first.`,
				});
			}
		}
	}

	const savedOwner = saved.owner_role ?? NEON_DEFAULT_OWNER_ROLE;
	const nextOwner = next.owner_role ?? NEON_DEFAULT_OWNER_ROLE;
	if (savedOwner !== nextOwner) {
		changes.push({
			kind: "owner_role",
			message:
				"Changing the owner role replaces the databases meroku created, so a deployed project refuses it while services use the database. Remove the services' database access and deploy first.",
		});
	}

	const savedDatabases = saved.databases ?? [];
	const nextDatabases = next.databases ?? [];
	const nextNames = new Set(nextDatabases.map((database) => database.name));
	const savedNames = new Set(savedDatabases.map((database) => database.name));
	savedDatabases.forEach((database, index) => {
		if (!database.create || nextNames.has(database.name)) return;
		const successor = nextDatabases[index];
		if (successor?.create && !savedNames.has(successor.name)) {
			changes.push({
				kind: "database_renamed",
				from: database.name,
				to: successor.name,
				message: `The database ${database.name} is kept in Neon with its data, and a new empty database ${successor.name} is created.`,
			});
			return;
		}
		changes.push({
			kind: "database_removed",
			name: database.name,
			message: `The database ${database.name} is kept in Neon with its data; meroku stops managing it.`,
		});
	});

	return changes;
}
