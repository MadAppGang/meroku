import { ChevronDown, Database, KeyRound, X } from "lucide-react";
import { type ReactNode, useEffect, useId, useState } from "react";
import type { NeonAccessLevel, NeonAccessTarget } from "../types/neon";
import type { YamlInfrastructureConfig } from "../types/yamlConfig";
import { neonSchemaChoices, neonTargetLabel } from "../utils/neonChoices";
import {
	buildNeonAccess,
	buildNeonAccessUpdate,
	getNeonAccess,
	type NeonAccessDraft,
	neonAccessDraftFromConfig,
} from "../utils/neonConfigUpdate";
import {
	checkNeonSchemaName,
	NEON_DEFAULT_OWNER_ROLE,
	NEON_MIN_ACCESS_PG_VERSION,
	neonProblemsFor,
	validateNeonConfig,
} from "../utils/neonValidation";
import { Badge } from "./ui/badge";
import { Button } from "./ui/button";
import {
	Card,
	CardContent,
	CardDescription,
	CardHeader,
	CardTitle,
} from "./ui/card";
import {
	Collapsible,
	CollapsibleContent,
	CollapsibleTrigger,
} from "./ui/collapsible";
import { Input } from "./ui/input";
import { Label } from "./ui/label";
import { RadioGroup, RadioGroupItem } from "./ui/radio-group";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "./ui/select";
import { NoteCode, NoteDetail, StatusNote } from "./ui/status-note";
import { Switch } from "./ui/switch";

/**
 * A service's Neon database: which of the env's databases it uses, the
 * schemas its login gets rights on, and whether it may change data.
 *
 * The service gets its own login and finds the connection string in
 * DATABASE_URL. Choices come from the env's `neon` block; a schema name can be
 * typed under Advanced, with an inline error on every keystroke. Edits are a
 * draft saved with one button, like the env's Neon panel.
 */
export interface ServiceDatabaseTabProps {
	config: YamlInfrastructureConfig;
	onConfigChange: (config: Partial<YamlInfrastructureConfig>) => void;
	target: NeonAccessTarget;
}

const ACCESS_CHOICES: {
	value: NeonAccessLevel;
	label: string;
	detail: string;
}[] = [
	{
		value: "readonly",
		label: "Read only",
		detail: "Can read data. Cannot change it.",
	},
	{
		value: "readwrite",
		label: "Read and write data",
		detail:
			"Can read, add, change and delete rows. Cannot create or drop tables: run migrations as the owner role.",
	},
];

export function ServiceDatabaseTab({
	config,
	onConfigChange,
	target,
}: ServiceDatabaseTabProps) {
	const uid = useId();
	const neon = config.neon;
	const saved = getNeonAccess(config, target);
	const savedKey = JSON.stringify(saved ?? null);
	const databases = neon?.databases ?? [];
	const firstDatabase = databases[0]?.name ?? "";

	const [enabled, setEnabled] = useState(saved !== undefined);
	const [draft, setDraft] = useState<NeonAccessDraft>(() =>
		neonAccessDraftFromConfig(saved, firstDatabase),
	);
	const [advancedOpen, setAdvancedOpen] = useState(false);
	const [manualSchema, setManualSchema] = useState("");

	// biome-ignore lint/correctness/useExhaustiveDependencies: savedKey stands for the saved block by value.
	useEffect(() => {
		setEnabled(saved !== undefined);
		setDraft(neonAccessDraftFromConfig(saved, firstDatabase));
		setManualSchema("");
	}, [savedKey, firstDatabase]);

	const label = neonTargetLabel(target);

	if (!neon) {
		return (
			<TabCard>
				<StatusNote tone="neutral">
					<p>This environment has no Neon database yet.</p>
					<NoteDetail>
						Open the Neon Postgres node, set up a project and add a database.
						Then come back here to connect {label}.
					</NoteDetail>
				</StatusNote>
			</TabCard>
		);
	}

	const ownerRole = neon.owner_role || NEON_DEFAULT_OWNER_ROLE;
	const reusedOldPostgres =
		neon.mode === "reuse" &&
		neon.pg_version !== undefined &&
		neon.pg_version < NEON_MIN_ACCESS_PG_VERSION;

	const next = enabled ? buildNeonAccess(draft, saved) : undefined;
	const dirty = JSON.stringify(next ?? null) !== savedKey;
	const nextConfig = {
		...config,
		...buildNeonAccessUpdate(config, target, next),
		workload:
			target.kind === "backend"
				? { ...config.workload, backend_neon: next }
				: config.workload,
	} as YamlInfrastructureConfig;
	const problems = enabled
		? neonProblemsFor(validateNeonConfig(nextConfig), target)
		: [];

	const schemaChoices = neonSchemaChoices(config, draft.database, target);
	const addableSchemas = schemaChoices.filter(
		(schema) => !draft.schemas.includes(schema),
	);
	const manualName = manualSchema.trim();
	const manualError =
		manualName === ""
			? null
			: draft.schemas.includes(manualName)
				? "This schema is already in the list."
				: checkNeonSchemaName(manualName);

	function setSchemas(schemas: string[]) {
		setDraft((d) => ({ ...d, schemas }));
	}

	function save() {
		onConfigChange(buildNeonAccessUpdate(config, target, next));
	}

	function discard() {
		setEnabled(saved !== undefined);
		setDraft(neonAccessDraftFromConfig(saved, firstDatabase));
		setManualSchema("");
	}

	return (
		<div className="space-y-4">
			<TabCard>
				<div className="flex items-start justify-between gap-4">
					<div className="space-y-1">
						<Label htmlFor={`${uid}-enabled`}>Give {label} a database</Label>
						<NoteDetail>
							{label} gets its own login and finds the connection string in
							DATABASE_URL.
						</NoteDetail>
					</div>
					<Switch
						id={`${uid}-enabled`}
						checked={enabled}
						disabled={databases.length === 0 && !enabled}
						onCheckedChange={setEnabled}
					/>
				</div>
				{databases.length === 0 && (
					<StatusNote tone="neutral">
						<p>The Neon project has no databases yet.</p>
						<NoteDetail>
							Add one on the Neon Postgres node's Databases tab first.
						</NoteDetail>
					</StatusNote>
				)}

				{enabled && (
					<>
						<div className="space-y-1">
							<Label htmlFor={`${uid}-database`}>Database</Label>
							<Select
								value={
									databases.some((d) => d.name === draft.database)
										? draft.database
										: ""
								}
								onValueChange={(database) =>
									setDraft((d) => ({ ...d, database }))
								}
							>
								<SelectTrigger id={`${uid}-database`}>
									<SelectValue placeholder="Choose a database" />
								</SelectTrigger>
								<SelectContent>
									{databases.map((database) => (
										<SelectItem key={database.name} value={database.name}>
											{database.name}
										</SelectItem>
									))}
								</SelectContent>
							</Select>
						</div>

						<div className="space-y-2">
							<Label htmlFor={`${uid}-add-schema`}>Schemas</Label>
							<NoteDetail>
								{label} can use the tables in these schemas, and finds them
								without a schema prefix.
							</NoteDetail>
							<ul className="flex flex-wrap gap-2" aria-label="Chosen schemas">
								{draft.schemas.map((schema) => {
									const isNew = !schemaChoices.includes(schema);
									return (
										<li key={schema} className="flex items-center gap-1">
											<Badge variant={isNew ? "secondary" : "outline"}>
												{schema}
											</Badge>
											<Button
												type="button"
												variant="ghost"
												size="icon"
												aria-label={`Remove schema ${schema}`}
												disabled={draft.schemas.length === 1}
												onClick={() =>
													setSchemas(draft.schemas.filter((s) => s !== schema))
												}
											>
												<X aria-hidden="true" />
											</Button>
										</li>
									);
								})}
							</ul>
							{draft.schemas
								.filter((schema) => !schemaChoices.includes(schema))
								.map((schema) => (
									<NoteDetail key={schema}>
										<NoteCode>{schema}</NoteCode> is created by meroku if it is
										missing, owned by <NoteCode>{ownerRole}</NoteCode>.
									</NoteDetail>
								))}
							{addableSchemas.length > 0 && (
								<Select
									value=""
									onValueChange={(schema) =>
										setSchemas([...draft.schemas, schema])
									}
								>
									<SelectTrigger id={`${uid}-add-schema`}>
										<SelectValue placeholder="Add a schema" />
									</SelectTrigger>
									<SelectContent>
										{addableSchemas.map((schema) => (
											<SelectItem key={schema} value={schema}>
												{schema}
											</SelectItem>
										))}
									</SelectContent>
								</Select>
							)}
						</div>

						<div className="space-y-2">
							<Label id={`${uid}-access`}>Access</Label>
							<RadioGroup
								value={draft.access}
								aria-labelledby={`${uid}-access`}
								disabled={reusedOldPostgres}
								onValueChange={(value) =>
									setDraft((d) => ({ ...d, access: value as NeonAccessLevel }))
								}
							>
								{ACCESS_CHOICES.map((choice) => (
									<div key={choice.value} className="flex items-start gap-2">
										<RadioGroupItem
											id={`${uid}-access-${choice.value}`}
											value={choice.value}
										/>
										<div className="space-y-1">
											<Label htmlFor={`${uid}-access-${choice.value}`}>
												{choice.label}
											</Label>
											<NoteDetail>{choice.detail}</NoteDetail>
										</div>
									</div>
								))}
							</RadioGroup>
							{reusedOldPostgres && (
								<StatusNote tone="warning">
									<p>
										This project runs Postgres {neon.pg_version}, where any
										service could create its own tables. meroku gives services
										access only on Postgres 15 or later. Upgrade the project in
										Neon first.
									</p>
								</StatusNote>
							)}
						</div>

						<Collapsible open={advancedOpen} onOpenChange={setAdvancedOpen}>
							<CollapsibleTrigger asChild>
								<Button
									type="button"
									variant="ghost"
									size="sm"
									aria-expanded={advancedOpen}
								>
									<ChevronDown aria-hidden="true" />
									Advanced
								</Button>
							</CollapsibleTrigger>
							<CollapsibleContent>
								<div className="space-y-4 pt-2">
									<div className="space-y-1">
										<Label htmlFor={`${uid}-manual-schema`}>
											Another schema
										</Label>
										<div className="flex items-center gap-2">
											<Input
												id={`${uid}-manual-schema`}
												value={manualSchema}
												placeholder="reports"
												autoComplete="off"
												spellCheck={false}
												aria-invalid={manualError !== null}
												aria-describedby={`${uid}-manual-schema-hint`}
												onChange={(event) =>
													setManualSchema(event.target.value)
												}
											/>
											<Button
												type="button"
												variant="outline"
												disabled={manualName === "" || manualError !== null}
												onClick={() => {
													setSchemas([...draft.schemas, manualName]);
													setManualSchema("");
												}}
											>
												Add
											</Button>
										</div>
										<div id={`${uid}-manual-schema-hint`} aria-live="polite">
											{manualError ? (
												<StatusNote tone="error">
													<p>{manualError}</p>
												</StatusNote>
											) : (
												<NoteDetail>
													A schema that is not in the list is created by meroku,
													owned by <NoteCode>{ownerRole}</NoteCode>.
												</NoteDetail>
											)}
										</div>
									</div>
									<div className="space-y-1">
										<NoteDetail>
											Password version {draft.password_version ?? 1}. A new
											password takes effect at the next deploy.
										</NoteDetail>
										<Button
											type="button"
											variant="outline"
											size="sm"
											onClick={() =>
												setDraft((d) => ({
													...d,
													password_version: (saved?.password_version ?? 1) + 1,
												}))
											}
										>
											<KeyRound aria-hidden="true" />
											Issue a new password
										</Button>
									</div>
								</div>
							</CollapsibleContent>
						</Collapsible>
					</>
				)}
			</TabCard>

			{(dirty || problems.length > 0) && (
				<div className="space-y-3">
					{problems.length > 0 && (
						<div className="space-y-2" aria-live="polite">
							{problems.map((problem) => (
								<StatusNote key={problem.path + problem.rule} tone="error">
									<p>{problem.message}</p>
								</StatusNote>
							))}
						</div>
					)}
					{!enabled && saved && (
						<StatusNote tone="warning">
							<p>
								Saving removes {label}'s database login at the next deploy. The
								data stays in Neon.
							</p>
						</StatusNote>
					)}
					{dirty && (
						<div className="flex flex-wrap gap-2">
							<Button
								type="button"
								disabled={problems.length > 0}
								onClick={save}
							>
								Save database access
							</Button>
							<Button type="button" variant="outline" onClick={discard}>
								Discard changes
							</Button>
						</div>
					)}
				</div>
			)}
		</div>
	);
}

function TabCard({ children }: { children: ReactNode }) {
	return (
		<Card>
			<CardHeader>
				<CardTitle className="flex items-center gap-2">
					<Database className="size-4" aria-hidden="true" />
					Neon database
				</CardTitle>
				<CardDescription>
					Connect this service to a database on the environment's Neon project.
				</CardDescription>
			</CardHeader>
			<CardContent className="space-y-5">{children}</CardContent>
		</Card>
	);
}
