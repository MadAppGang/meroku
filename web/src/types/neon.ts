/**
 * `GET /api/neon/lookup?env=<name>[&project_id=<id>][&org_id=<id>]`
 * (app/api_neon.go). Live Neon choices for the Neon panels. The response never
 * carries the Neon token or a password.
 */

export type NeonAuthState =
	| "ok"
	| "no_credentials"
	| "neonctl_missing"
	| "expired"
	| "error";

export interface NeonLookupResponse {
	auth: {
		state: NeonAuthState;
		source?: "env" | "neonctl";
		expires_in_seconds?: number;
		message?: string;
	};
	organizations?: { id: string; name: string }[];
	projects?: {
		id: string;
		name: string;
		region_id: string;
		pg_version: number;
		org_id: string;
	}[];
	project?: { id: string; name: string; region_id: string; pg_version: number };
	/** Databases on the project's default branch. */
	databases?: { name: string; owner: string }[];
	/** Roles on the project's default branch, for `owner_role`. */
	roles?: { name: string }[];
	/** Why some of the above is missing, e.g. a Neon API failure. */
	note?: string;
}

/** Query options for `GET /api/neon/lookup`, beyond the env name. */
export interface NeonLookupOptions {
	/** Look up this project instead of the one the env YAML names. */
	projectId?: string;
	/** List this organization's projects. */
	orgId?: string;
	signal?: AbortSignal;
}

/** The two access levels a service can have on its Neon database. */
export type NeonAccessLevel = "readonly" | "readwrite";

/**
 * Which service a Neon access block belongs to: the backend
 * (`workload.backend_neon`) or a named service (`services[].neon`).
 */
export type NeonAccessTarget =
	| { kind: "backend" }
	| { kind: "service"; name: string };
