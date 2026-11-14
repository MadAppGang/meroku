import { ChevronDown, Database, KeyRound, X } from "lucide-react";
import { type ReactNode, useEffect, useId, useRef, useState } from "react";
import { infrastructureApi } from "../api/infrastructure";
import type {
	NeonAccessLevel,
	NeonAccessTarget,
	NeonSchemasOptions,
	NeonSchemasResponse,
} from "../types/neon";
import type {
	YamlInfrastructureConfig,
	YamlNeonAccess,
} from "../types/yamlConfig";
import { neonSchemaChoices, neonTargetLabel } from "../utils/neonChoices";
import {
	buildNeonAccess,
	buildNeonAccessUpdate,
	getNeonAccess,
	type NeonAccessValues,
	neonAccessReplacingChange,
	neonAccessValuesFromConfig,
} from "../utils/neonConfigUpdate";
import { neonRoleName, neonTargetKey } from "../utils/neonRoleName";
import {
	checkNeonDatabaseName,
	checkNeonSchemaName,
	NEON_DEFAULT_OWNER_ROLE,
	NEON_DEFAULT_SCHEMAS,
	NEON_MIN_ACCESS_PG_VERSION,
	neonProblemsFor,
	validateNeonConfig,
} from "../utils/neonValidation";
import {
	AlertDialog,
	AlertDialogAction,
	AlertDialogCancel,
	AlertDialogContent,
	AlertDialogDescription,
	AlertDialogFooter,
	AlertDialogHeader,
	AlertDialogTitle,
} from "./ui/alert-dialog";
import { Badge } from "./ui/badge";
import { Button } from "./ui/button";
import {
	Card,
	CardContent,
	CardDescription,
	CardHeader,
	CardTitle,
} from "./ui/card";
import { Checkbox } from "./ui/checkbox";
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
 * DATABASE_URL. Databases come from the env's `neon` block; schemas come live
 * from `GET /api/neon/schemas` when the database can be read, with manual entry
 * under Advanced (and as the fallback when live schemas are unavailable). Valid
 * changes write immediately through `onConfigChange`.
 */

export type NeonSchemasFn = (
	env: string,
	database: string,
	opts: NeonSchemasOptions,
) => Promise<NeonSchemasResponse>;

export interface ServiceDatabaseTabProps {
	config: YamlInfrastructureConfig;
	onConfigChange: (config: Partial<YamlInfrastructureConfig>) => void;
	target: NeonAccessTarget;
	/** Reads a database's existing schemas. Defaults to GET /api/neon/schemas. */
	lookupSchemas?: NeonSchemasFn;
}

type SchemasLookupState =
	| { status: "idle" }
	| { status: "loading" }
	| { status: "failed"; error: string }
	| { status: "done"; data: NeonSchemasResponse };

const defaultLookupSchemas: NeonSchemasFn = (env, database, opts) =>
	infrastructureApi.getNeonSchemas(env, database, opts);

function isAbort(err: unknown): boolean {
	return err instanceof DOMException && err.name === "AbortError";
}

function withAccess(
	config: YamlInfrastructureConfig,
	target: NeonAccessTarget,
	access: YamlNeonAccess | undefined,
): YamlInfrastructureConfig {
	return {
		...config,
		...buildNeonAccessUpdate(config, target, access),
		workload:
			target.kind === "backend"
				? { ...config.workload, backend_neon: access }
				: config.workload,
	} as YamlInfrastructureConfig;
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
	lookupSchemas = defaultLookupSchemas,
}: ServiceDatabaseTabProps) {
	const uid = useId();
	const neon = config.neon;
	const saved = getNeonAccess(config, target);
	const savedKey = JSON.stringify(saved ?? null);
	const enabled = saved !== undefined;
	const databases = neon?.databases ?? [];
	const firstDatabase = databases[0]?.name ?? "";
	const values = neonAccessValuesFromConfig(saved, firstDatabase);

	const [advancedOpen, setAdvancedOpen] = useState(false);
	const [manualSchema, setManualSchema] = useState("");
	const [roleName, setRoleName] = useState<string | null>(null);
	const [schemasLookup, setSchemasLookup] = useState<SchemasLookupState>({
		status: "idle",
	});
	const [pendingDatabase, setPendingDatabase] = useState<{
		database: string;
		message: string;
	} | null>(null);
	const confirmedDbRef = useRef(false);

	// biome-ignore lint/correctness/useExhaustiveDependencies: savedKey resets pending inputs when saved config changes.
	useEffect(() => {
		setManualSchema("");
		setPendingDatabase(null);
	}, [savedKey]);

	const chosenDatabase =
		enabled && databases.some((d) => d.name === values.database)
			? values.database
			: "";
	const serviceKey = neonTargetKey(target);
	const hasNeon = neon !== undefined;

	useEffect(() => {
		if (!hasNeon || !enabled || chosenDatabase === "") {
			setRoleName(null);
			return;
		}
		let cancelled = false;
		setRoleName(null);
		neonRoleName(config.project, config.env, serviceKey, chosenDatabase)
			.then((name) => {
				if (!cancelled) setRoleName(name);
			})
			.catch(() => {
				if (!cancelled) setRoleName(null);
			});
		return () => {
			cancelled = true;
		};
	}, [
		hasNeon,
		enabled,
		config.project,
		config.env,
		serviceKey,
		chosenDatabase,
	]);

	useEffect(() => {
		if (!hasNeon || !enabled || chosenDatabase === "") {
			setSchemasLookup({ status: "idle" });
			return;
		}
		const ctrl = new AbortController();
		setSchemasLookup({ status: "loading" });
		lookupSchemas(config.env, chosenDatabase, { signal: ctrl.signal })
			.then((data) => setSchemasLookup({ status: "done", data }))
			.catch((err) => {
				if (isAbort(err)) return;
				setSchemasLookup({
					status: "failed",
					error:
						err instanceof Error && err.message
							? err.message
							: "No answer from meroku.",
				});
			});
		return () => ctrl.abort();
	}, [hasNeon, enabled, config.env, chosenDatabase, lookupSchemas]);

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

	const defaultEnableAccess =
		firstDatabase !== ""
			? buildNeonAccess(
					neonAccessValuesFromConfig(undefined, firstDatabase),
					undefined,
				)
			: undefined;
	const preEnableProblems =
		!enabled && defaultEnableAccess
			? neonProblemsFor(
					validateNeonConfig(withAccess(config, target, defaultEnableAccess)),
					target,
				)
			: [];
	const problems = enabled
		? neonProblemsFor(validateNeonConfig(config), target)
		: preEnableProblems;

	const liveSchemas =
		schemasLookup.status === "done" && schemasLookup.data.note === ""
			? schemasLookup.data.schemas
			: null;
	const liveSchemasSet = liveSchemas ? new Set(liveSchemas) : null;
	const newLiveSchemas = liveSchemasSet
		? values.schemas.filter((schema) => !liveSchemasSet.has(schema))
		: [];
	const selectableSchemas = liveSchemas
		? [...liveSchemas, ...newLiveSchemas]
		: [];

	const schemaChoices = neonSchemaChoices(config, values.database, target);
	const addableSchemas = schemaChoices.filter(
		(schema) => !values.schemas.includes(schema),
	);
	const isLoadingSchemas = schemasLookup.status === "loading";

	const manualName = manualSchema.trim();
	const manualError =
		manualName === ""
			? null
			: values.schemas.includes(manualName)
				? "This schema is already in the list."
				: checkNeonSchemaName(manualName);

	function commitAccess(nextValues: NeonAccessValues) {
		const next = buildNeonAccess(nextValues, saved);
		if (checkNeonDatabaseName(next.database, false) !== null) return;
		if (!databases.some((d) => d.name === next.database)) return;
		const schemas = next.schemas ?? NEON_DEFAULT_SCHEMAS;
		if (
			schemas.length === 0 ||
			schemas.some((schema) => checkNeonSchemaName(schema) !== null)
		) {
			return;
		}

		const replacingMessage = neonAccessReplacingChange(
			saved,
			next,
			databases.map((d) => d.name),
			label,
		);
		if (replacingMessage !== null) {
			setPendingDatabase({
				database: next.database,
				message: replacingMessage,
			});
			return;
		}
		onConfigChange(buildNeonAccessUpdate(config, target, next));
	}

	function handleToggle(nextEnabled: boolean) {
		if (!nextEnabled) {
			onConfigChange(buildNeonAccessUpdate(config, target, undefined));
			return;
		}
		if (
			!defaultEnableAccess ||
			reusedOldPostgres ||
			preEnableProblems.length > 0
		) {
			return;
		}
		onConfigChange(buildNeonAccessUpdate(config, target, defaultEnableAccess));
	}

	function setSchemas(schemas: string[]) {
		commitAccess({ ...values, schemas });
	}

	function addManualSchema() {
		if (manualName === "" || manualError !== null) return;
		setSchemas([...values.schemas, manualName]);
		setManualSchema("");
	}

	function confirmDatabaseChange() {
		if (!pendingDatabase) return;
		confirmedDbRef.current = true;
		const next = buildNeonAccess(
			{ ...values, database: pendingDatabase.database },
			saved,
		);
		setPendingDatabase(null);
		onConfigChange(buildNeonAccessUpdate(config, target, next));
	}

	const postgres14Warning = (
		<StatusNote tone="warning">
			<p>
				This project runs Postgres {neon.pg_version}, where any service could
				create its own tables. meroku gives services access only on Postgres 15
				or later. Upgrade the project in Neon first.
			</p>
		</StatusNote>
	);

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
						disabled={
							!enabled &&
							(databases.length === 0 ||
								reusedOldPostgres ||
								preEnableProblems.length > 0)
						}
						onCheckedChange={handleToggle}
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
				{!enabled && reusedOldPostgres && postgres14Warning}

				{enabled && (
					<>
						<div className="space-y-1.5">
							<Label htmlFor={`${uid}-database`}>Database</Label>
							<Select
								value={chosenDatabase}
								onValueChange={(database) =>
									commitAccess({ ...values, database })
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
							{chosenDatabase !== "" && roleName !== null && (
								<div className="space-y-0.5 pt-0.5">
									<NoteDetail>
										Login role: <NoteCode>{roleName}</NoteCode>
									</NoteDetail>
									<NoteDetail>
										meroku creates this role at the next deploy if it does not
										exist yet. It changes if the database changes.
									</NoteDetail>
								</div>
							)}
						</div>

						<div className="space-y-2">
							<Label id={`${uid}-schemas`}>Schemas</Label>
							<NoteDetail>
								{label} can use the tables in these schemas, and finds them
								without a schema prefix.
							</NoteDetail>

							{liveSchemas !== null ? (
								<>
									<ul className="space-y-2" aria-labelledby={`${uid}-schemas`}>
										{selectableSchemas.map((schema, index) => {
											const checked = values.schemas.includes(schema);
											const isNew = !liveSchemasSet?.has(schema);
											const checkboxId = `${uid}-schema-${index}`;
											const badgeId = `${uid}-schema-${index}-new`;
											return (
												<li
													key={schema}
													className="flex flex-wrap items-center gap-2"
												>
													<Checkbox
														id={checkboxId}
														checked={checked}
														disabled={checked && values.schemas.length === 1}
														aria-describedby={isNew ? badgeId : undefined}
														onCheckedChange={(nextChecked) => {
															if (nextChecked === true) {
																if (!checked) {
																	setSchemas([...values.schemas, schema]);
																}
															} else if (checked && values.schemas.length > 1) {
																setSchemas(
																	values.schemas.filter((s) => s !== schema),
																);
															}
														}}
													/>
													<Label
														htmlFor={checkboxId}
														className="font-mono font-normal"
													>
														{schema}
													</Label>
													{isNew && (
														<Badge id={badgeId} variant="secondary">
															new — meroku creates it
														</Badge>
													)}
												</li>
											);
										})}
									</ul>
									{newLiveSchemas.length > 0 && (
										<NoteDetail>
											New schemas are owned by <NoteCode>{ownerRole}</NoteCode>.
										</NoteDetail>
									)}
								</>
							) : (
								<>
									{isLoadingSchemas && (
										<StatusNote tone="pending">
											<NoteDetail>Loading schemas…</NoteDetail>
										</StatusNote>
									)}
									{schemasLookup.status === "done" &&
										schemasLookup.data.note !== "" && (
											<StatusNote tone="neutral">
												<p>{schemasLookup.data.note}</p>
											</StatusNote>
										)}
									{schemasLookup.status === "failed" && (
										<StatusNote tone="neutral">
											<p>We couldn't read this database's schemas just now.</p>
											<NoteDetail>{schemasLookup.error}</NoteDetail>
										</StatusNote>
									)}
									<ul
										className="flex flex-wrap gap-2"
										aria-label="Chosen schemas"
									>
										{values.schemas.map((schema) => {
											const isNew =
												!isLoadingSchemas && !schemaChoices.includes(schema);
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
														disabled={values.schemas.length === 1}
														onClick={() =>
															setSchemas(
																values.schemas.filter((s) => s !== schema),
															)
														}
													>
														<X aria-hidden="true" />
													</Button>
												</li>
											);
										})}
									</ul>
									{!isLoadingSchemas &&
										values.schemas
											.filter((schema) => !schemaChoices.includes(schema))
											.map((schema) => (
												<NoteDetail key={schema}>
													<NoteCode>{schema}</NoteCode> is created by meroku if
													it is missing, owned by{" "}
													<NoteCode>{ownerRole}</NoteCode>.
												</NoteDetail>
											))}
									{!isLoadingSchemas && addableSchemas.length > 0 && (
										<Select
											value=""
											onValueChange={(schema) =>
												setSchemas([...values.schemas, schema])
											}
										>
											<SelectTrigger
												id={`${uid}-add-schema`}
												aria-label="Add a schema"
											>
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
								</>
							)}
						</div>

						<div className="space-y-2">
							<Label id={`${uid}-access`}>Access</Label>
							<RadioGroup
								value={values.access}
								aria-labelledby={`${uid}-access`}
								disabled={reusedOldPostgres}
								onValueChange={(value) =>
									commitAccess({ ...values, access: value as NeonAccessLevel })
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
							{reusedOldPostgres && postgres14Warning}
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
												onKeyDown={(event) => {
													if (event.key === "Enter") {
														event.preventDefault();
														addManualSchema();
													}
												}}
											/>
											<Button
												type="button"
												variant="outline"
												disabled={manualName === "" || manualError !== null}
												onClick={addManualSchema}
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
											Password version {values.password_version ?? 1}. A new
											password takes effect at the next deploy.
										</NoteDetail>
										<Button
											type="button"
											variant="outline"
											size="sm"
											onClick={() =>
												commitAccess({
													...values,
													password_version: (saved?.password_version ?? 1) + 1,
												})
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

			{problems.length > 0 && (
				<div className="space-y-2" aria-live="polite">
					{problems.map((problem) => (
						<StatusNote key={problem.path + problem.rule} tone="error">
							<p>{problem.message}</p>
						</StatusNote>
					))}
				</div>
			)}

			<AlertDialog
				open={pendingDatabase !== null}
				onOpenChange={(open) => {
					if (!open) {
						if (confirmedDbRef.current) {
							confirmedDbRef.current = false;
						}
						setPendingDatabase(null);
					}
				}}
			>
				<AlertDialogContent>
					<AlertDialogHeader>
						<AlertDialogTitle>
							Change {label}'s database to {pendingDatabase?.database}?
						</AlertDialogTitle>
						<AlertDialogDescription>
							{pendingDatabase?.message}
						</AlertDialogDescription>
					</AlertDialogHeader>
					<AlertDialogFooter>
						<AlertDialogCancel>Cancel</AlertDialogCancel>
						<AlertDialogAction onClick={confirmDatabaseChange}>
							Change database
						</AlertDialogAction>
					</AlertDialogFooter>
				</AlertDialogContent>
			</AlertDialog>
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
