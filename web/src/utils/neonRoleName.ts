import type { NeonAccessTarget } from "../types/neon";

/**
 * Postgres login role name for one `(project, env, service, database)` tuple.
 *
 * Mirrors `neonSanitize` and `neonRoleName` in `app/neon_config.go` and is
 * pinned against `app/testdata/neon_role_name_vectors.json`.
 */

/** The backend's service key in role names and SSM paths (`neonBackendKey` in Go). */
export const NEON_BACKEND_KEY = "backend";

/**
 * Maximum byte length of the readable prefix: 63 bytes (`NAMEDATALEN - 1`)
 * minus `"_"` and the 8-hex SHA-256 id (`neonRoleNameBudget` in Go).
 */
export const NEON_ROLE_NAME_BUDGET = 54;

const NEON_SANITIZE_CHARS = /[^a-z0-9]/gu;
const NEON_SANITIZE_RUNS = /_+/g;

/** Service key passed to `neonRoleName`: `"backend"` or the named service's name. */
export function neonTargetKey(target: NeonAccessTarget): string {
	return target.kind === "backend" ? NEON_BACKEND_KEY : target.name;
}

/**
 * Lower-cases `s`, replaces every character outside `[a-z0-9]` with `_`, and
 * collapses runs of `_` to one.
 */
export function neonSanitize(s: string): string {
	return s
		.toLowerCase()
		.replace(NEON_SANITIZE_CHARS, "_")
		.replace(NEON_SANITIZE_RUNS, "_");
}

/**
 * Computes the Postgres login role name of `(project, env, svc, db)` using
 * Web Crypto SHA-256 over `project + "\0" + env + "\0" + svc + "\0" + db`.
 */
export async function neonRoleName(
	project: string,
	env: string,
	svc: string,
	db: string,
): Promise<string> {
	const raw = new TextEncoder().encode(`${project}\0${env}\0${svc}\0${db}`);
	const digest = await crypto.subtle.digest("SHA-256", raw);
	const id = Array.from(new Uint8Array(digest).subarray(0, 4), (byte) =>
		byte.toString(16).padStart(2, "0"),
	).join("");

	let prefix = [
		neonSanitize(project),
		neonSanitize(env),
		neonSanitize(svc),
		neonSanitize(db),
	].join("_");
	if (prefix !== "" && prefix[0] >= "0" && prefix[0] <= "9") {
		prefix = `r_${prefix}`;
	}
	if (prefix.length > NEON_ROLE_NAME_BUDGET) {
		prefix = prefix.slice(0, NEON_ROLE_NAME_BUDGET);
	}
	return `${prefix.replace(/_+$/, "")}_${id}`;
}
