import type { YamlNeonAccess } from "../types/yamlConfig";
import { NoteCode, NoteDetail, StatusNote } from "./ui/status-note";

/** The SSM parameter name meroku writes a service's Neon connection string to. */
export const NEON_DATABASE_URL_PARAM = "database_url";

/**
 * Tells the user where a Neon-connected service's DATABASE_URL lives. Before the
 * first deploy the parameter does not exist yet, so the note says when it will.
 */
export function NeonDatabaseUrlNote({
	access,
	parameterPath,
	deployed,
}: {
	access: YamlNeonAccess;
	parameterPath: string;
	/** The parameter already exists in SSM. */
	deployed: boolean;
}) {
	return (
		<StatusNote tone={deployed ? "success" : "neutral"}>
			<p>
				DATABASE_URL for the Neon database {access.database} (
				{access.access === "readwrite" ? "read and write" : "read only"}).
			</p>
			<NoteDetail>
				{deployed
					? "meroku manages it from the Database tab and rewrites it on every deploy."
					: "meroku creates it at the next deploy, from the Database tab."}{" "}
				<NoteCode>
					{parameterPath}/{NEON_DATABASE_URL_PARAM}
				</NoteCode>
			</NoteDetail>
		</StatusNote>
	);
}
