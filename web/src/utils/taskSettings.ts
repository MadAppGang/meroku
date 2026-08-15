/**
 * Turns the scheduled-task command editor's text into the `list(string)` the
 * config and Terraform expect.
 *
 * Two accepted inputs, in this order:
 *
 *   1. A JSON array — `["npm","run","cron"]` — which round-trips exactly. This
 *      is what a config written by any other tool looks like, and what earlier
 *      meroku users typed by hand to make the old raw template render valid HCL.
 *   2. Otherwise a comma-separated line — `npm, run, cron`.
 *
 * COMMA IS A SEPARATOR HERE, and deliberately is NOT in migration v25
 * (app/migrations.go). That is not an oversight, but it is a real difference
 * worth stating: the migration converts values already on disk, where it cannot
 * know whether a comma was a separator or part of an argument, so it keeps the
 * whole string as one argument. This function converts what a person just typed
 * into a field documented as comma-separated, so the comma is intentional.
 *
 * The consequence: a single argument that itself contains a comma cannot be
 * typed as bare text — write it as a JSON array instead, which is why the JSON
 * form is tried first.
 *
 * Empty input returns undefined rather than [], so the key is omitted from the
 * config and the container's own ENTRYPOINT still applies.
 */
export function parseContainerCommand(value: string): string[] | undefined {
	const trimmed = value.trim();
	if (!trimmed) {
		return undefined;
	}

	if (trimmed.startsWith("[") && trimmed.endsWith("]")) {
		try {
			const parsed = JSON.parse(trimmed);
			if (
				Array.isArray(parsed) &&
				parsed.every((argument) => typeof argument === "string")
			) {
				return parsed;
			}
		} catch {
			// Bracketed but not decodable JSON. Fall through and treat it as the
			// comma-separated editor format rather than guessing.
		}
	}

	return trimmed
		.split(",")
		.map((argument) => argument.trim())
		.filter(Boolean);
}
