import {
	ChevronDown,
	Database,
	Plus,
	RefreshCw,
	Trash2,
	Unlink,
} from "lucide-react";
import { type ReactNode, useEffect, useId, useState } from "react";
import { infrastructureApi } from "../api/infrastructure";
import type { NeonLookupOptions, NeonLookupResponse } from "../types/neon";
import type {
	NeonCreatePgVersion,
	NeonReusePgVersion,
	YamlInfrastructureConfig,
	YamlNeonConfig,
} from "../types/yamlConfig";
import {
	defaultNeonProjectName,
	NEON_REGIONS,
	neonAccessEntries,
	neonRegionForAwsRegion,
	neonRegionLabel,
	neonServicesUsingDatabase,
} from "../utils/neonChoices";
import {
	buildNeonUpdate,
	type NeonDraft,
	neonDraftFromConfig,
	neonReplacingChanges,
} from "../utils/neonConfigUpdate";
import {
	checkNeonDatabaseName,
	checkNeonIdentifier,
	checkNeonOrgId,
	checkNeonProjectId,
	checkNeonProjectName,
	checkNeonRegionId,
	NEON_CREATE_PG_VERSIONS,
	NEON_DEFAULT_OWNER_ROLE,
	NEON_MIN_ACCESS_PG_VERSION,
	NEON_PROJECT_DEFAULT_DATABASE,
	NEON_REUSE_PG_VERSIONS,
	neonProjectProblems,
	validateNeonConfig,
} from "../utils/neonValidation";
import { Alert, AlertDescription, AlertTitle } from "./ui/alert";
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

/**
 * The env's Neon Postgres settings: the project ("Project" tab) and the
 * databases on its default branch ("Databases" tab).
 *
 * Every choice is a generated list: the env YAML, the live values of
 * GET /api/neon/lookup when the user is logged in to Neon, and static lists
 * (regions, Postgres versions). Typing an id is an Advanced fallback with an
 * inline error on every keystroke.
 *
 * Edits are a draft held here and shared by both tabs. The project fields,
 * the owner role and database names replace what they name in Neon, so
 * nothing is written per keystroke: one Save writes the complete `neon`
 * block, after a confirmation when the change replaces or releases something
 * in a deployed project.
 */

export type NeonPanelSection = "settings" | "databases";

export type NeonLookupFn = (
	env: string,
	opts: NeonLookupOptions,
) => Promise<NeonLookupResponse>;

export interface NeonNodePropertiesProps {
	config: YamlInfrastructureConfig;
	onConfigChange: (config: Partial<YamlInfrastructureConfig>) => void;
	/** Which tab is showing. The draft survives switching tabs. */
	section: NeonPanelSection;
	/** Reads live Neon choices. Defaults to GET /api/neon/lookup. */
	lookupNeon?: NeonLookupFn;
}

type LookupState =
	| { status: "loading" }
	| { status: "failed"; error: string }
	| { status: "done"; data: NeonLookupResponse };

/** "yes": the lookup found the project in Neon. "unknown": no lookup. */
type Deployed = "yes" | "no" | "unknown";

const NEW_DATABASE = "__new__";

const defaultLookup: NeonLookupFn = (env, opts) =>
	infrastructureApi.getNeonLookup(env, opts);

function isAbort(err: unknown): boolean {
	return err instanceof DOMException && err.name === "AbortError";
}

function freshDraft(config: YamlInfrastructureConfig): NeonDraft {
	return neonDraftFromConfig(config.neon, {
		projectName: defaultNeonProjectName(config.project, config.env),
		regionId: neonRegionForAwsRegion(config.region),
	});
}

function nextNewDatabaseName(taken: Set<string>): string {
	if (!taken.has("app")) return "app";
	let index = 2;
	while (taken.has(`app_${index}`)) index++;
	return `app_${index}`;
}

function isReusePgVersion(value: number): value is NeonReusePgVersion {
	return (NEON_REUSE_PG_VERSIONS as readonly number[]).includes(value);
}

export function NeonNodeProperties({
	config,
	onConfigChange,
	section,
	lookupNeon = defaultLookup,
}: NeonNodePropertiesProps) {
	const uid = useId();
	const saved = config.neon;
	const savedKey = JSON.stringify(saved ?? null);

	const [enabled, setEnabled] = useState(saved !== undefined);
	const [draft, setDraft] = useState<NeonDraft>(() => freshDraft(config));
	const [reuseOrg, setReuseOrg] = useState("");
	const [advancedOpen, setAdvancedOpen] = useState(false);
	const [manualDatabase, setManualDatabase] = useState("");
	const [confirmOpen, setConfirmOpen] = useState(false);
	const [removeOpen, setRemoveOpen] = useState(false);

	const [lookup, setLookup] = useState<LookupState>({ status: "loading" });
	const [reloadKey, setReloadKey] = useState(0);

	// A save, a discard or another panel changed the saved block: start over
	// from what is on disk.
	// biome-ignore lint/correctness/useExhaustiveDependencies: savedKey stands for config.neon by value.
	useEffect(() => {
		setEnabled(config.neon !== undefined);
		setDraft(freshDraft(config));
	}, [savedKey]);

	const lookupOrg =
		draft.mode === "create"
			? checkNeonOrgId(draft.create.org_id) === null
				? draft.create.org_id
				: undefined
			: reuseOrg || undefined;
	const lookupProject =
		draft.mode === "reuse" &&
		checkNeonProjectId(draft.reuse.project_id) === null
			? draft.reuse.project_id
			: undefined;

	// biome-ignore lint/correctness/useExhaustiveDependencies: reloadKey re-runs the lookup on Refresh.
	useEffect(() => {
		if (!enabled) return;
		const ctrl = new AbortController();
		setLookup({ status: "loading" });
		lookupNeon(config.env, {
			orgId: lookupOrg,
			projectId: lookupProject,
			signal: ctrl.signal,
		})
			.then((data) => setLookup({ status: "done", data }))
			.catch((err) => {
				if (isAbort(err)) return;
				setLookup({
					status: "failed",
					error:
						err instanceof Error && err.message
							? err.message
							: "No answer from meroku.",
				});
			});
		return () => ctrl.abort();
	}, [enabled, config.env, lookupOrg, lookupProject, reloadKey, lookupNeon]);

	const live = lookup.status === "done" ? lookup.data : undefined;
	const authOk = live?.auth.state === "ok";
	const organizations = authOk ? (live?.organizations ?? []) : [];
	const projects = authOk
		? (live?.projects ?? []).filter(
				(project) => !reuseOrg || project.org_id === reuseOrg,
			)
		: [];
	const liveDatabases = authOk ? (live?.databases ?? []) : [];
	const deployed: Deployed = !authOk ? "unknown" : live?.project ? "yes" : "no";

	// One organization: choose it, there is nothing to decide.
	useEffect(() => {
		if (organizations.length !== 1) return;
		const only = organizations[0].id;
		if (draft.mode === "create" && draft.create.org_id === "") {
			setDraft((d) => ({ ...d, create: { ...d.create, org_id: only } }));
		}
		if (draft.mode === "reuse" && reuseOrg === "") setReuseOrg(only);
	}, [organizations, draft.mode, draft.create.org_id, reuseOrg]);

	const next: YamlNeonConfig | undefined = enabled
		? buildNeonUpdate(draft, saved)
		: undefined;
	const dirty = JSON.stringify(next ?? null) !== savedKey;
	const problems = next
		? neonProjectProblems(validateNeonConfig({ ...config, neon: next }))
		: [];
	const replacing =
		saved && next && deployed !== "no" ? neonReplacingChanges(saved, next) : [];
	const accessEntries = neonAccessEntries(config);

	function update(change: (d: NeonDraft) => NeonDraft) {
		setDraft((d) => change(d));
	}

	function save() {
		if (!next) return;
		onConfigChange({ neon: next });
	}

	function requestSave() {
		if (replacing.length > 0) setConfirmOpen(true);
		else save();
	}

	function discard() {
		setEnabled(saved !== undefined);
		setDraft(freshDraft(config));
		setManualDatabase("");
	}

	function removeNeon() {
		onConfigChange({ neon: undefined });
	}

	if (!enabled) {
		return (
			<PanelCard
				title="Neon Postgres"
				description="A serverless Postgres database from Neon. meroku sets up the project, the databases and a login for each service that needs one."
			>
				<NoteDetail>
					Each service gets its own database login and finds the address in
					DATABASE_URL. meroku never deletes Neon data.
				</NoteDetail>
				<Button type="button" onClick={() => setEnabled(true)}>
					<Database aria-hidden="true" />
					Set up Neon for this environment
				</Button>
			</PanelCard>
		);
	}

	const lookupStatus = (
		<LookupStatus
			lookup={lookup}
			onRefresh={() => setReloadKey((k) => k + 1)}
		/>
	);

	const footer = (
		<SaveSection
			dirty={dirty}
			problems={problems.map((problem) => problem.message)}
			replacing={replacing.map((change) => change.message)}
			deployed={deployed}
			onSave={requestSave}
			onDiscard={discard}
		/>
	);

	const confirmDialog = (
		<AlertDialog open={confirmOpen} onOpenChange={setConfirmOpen}>
			<AlertDialogContent>
				<AlertDialogHeader>
					<AlertDialogTitle>
						Save a change that replaces Neon data?
					</AlertDialogTitle>
					<AlertDialogDescription>
						{deployed === "yes"
							? "This project already exists in Neon."
							: "We couldn't check whether this project already exists in Neon."}{" "}
						Read what happens at the next deploy:
					</AlertDialogDescription>
				</AlertDialogHeader>
				<div className="space-y-2">
					{replacing.map((change) => (
						<StatusNote key={change.kind + change.message} tone="warning">
							<p>{change.message}</p>
						</StatusNote>
					))}
				</div>
				<AlertDialogFooter>
					<AlertDialogCancel>Keep editing</AlertDialogCancel>
					<AlertDialogAction onClick={save}>Save anyway</AlertDialogAction>
				</AlertDialogFooter>
			</AlertDialogContent>
		</AlertDialog>
	);

	if (section === "databases") {
		const taken = new Set(draft.databases.map((database) => database.name));
		const addChoices = [
			...(draft.mode === "create" && !taken.has(NEON_PROJECT_DEFAULT_DATABASE)
				? [
						{
							value: NEON_PROJECT_DEFAULT_DATABASE,
							label: `${NEON_PROJECT_DEFAULT_DATABASE} — comes with the new project`,
						},
					]
				: []),
			...liveDatabases
				.filter((database) => !taken.has(database.name))
				.map((database) => ({
					value: database.name,
					label: `${database.name} — already in Neon`,
				})),
			{ value: NEW_DATABASE, label: "New database…" },
		];
		const manualName = manualDatabase.trim();
		const manualError =
			manualName === ""
				? null
				: taken.has(manualName)
					? "This database is already in the list."
					: checkNeonDatabaseName(manualName, false);

		function addDatabase(value: string) {
			update((d) => {
				const names = new Set(d.databases.map((database) => database.name));
				const database =
					value === NEW_DATABASE
						? { name: nextNewDatabaseName(names), create: true }
						: { name: value, create: false };
				return { ...d, databases: [...d.databases, database] };
			});
		}

		return (
			<div className="space-y-4">
				{lookupStatus}
				<PanelCard
					title="Databases"
					description="The databases your services can use. A database you add as new is created by meroku; an existing one is used as it is."
				>
					{draft.mode === "create" && (
						<NoteDetail>
							A new project comes with a database called{" "}
							<NoteCode>{NEON_PROJECT_DEFAULT_DATABASE}</NoteCode>.
						</NoteDetail>
					)}
					{draft.databases.length === 0 && (
						<StatusNote tone="neutral">
							<p>No databases yet. Add one so your services can connect.</p>
						</StatusNote>
					)}
					<div className="space-y-4">
						{draft.databases.map((database, index) => {
							const usedBy = neonServicesUsingDatabase(config, database.name);
							const savedHere = saved?.databases?.some(
								(entry) => entry.name === database.name,
							);
							const nameError = checkNeonDatabaseName(
								database.name,
								database.create === true,
							);
							const rowId = `${uid}-database-${index}`;
							return (
								<div key={rowId} className="space-y-1">
									<div className="flex items-center gap-2">
										{database.create ? (
											<Input
												id={rowId}
												aria-label={`Name of new database ${index + 1}`}
												value={database.name}
												autoComplete="off"
												spellCheck={false}
												aria-invalid={nameError !== null}
												aria-describedby={`${rowId}-hint`}
												onChange={(event) => {
													const name = event.target.value;
													update((d) => ({
														...d,
														databases: d.databases.map((entry, i) =>
															i === index ? { ...entry, name } : entry,
														),
													}));
												}}
											/>
										) : (
											<NoteCode>{database.name}</NoteCode>
										)}
										<Badge variant={database.create ? "secondary" : "outline"}>
											{database.create ? "New" : "Existing"}
										</Badge>
										<Button
											type="button"
											variant="ghost"
											size="icon"
											aria-label={`Remove database ${database.name}`}
											disabled={usedBy.length > 0}
											onClick={() =>
												update((d) => ({
													...d,
													databases: d.databases.filter((_, i) => i !== index),
												}))
											}
										>
											<Trash2 aria-hidden="true" />
										</Button>
									</div>
									<div
										id={`${rowId}-hint`}
										aria-live="polite"
										className="space-y-1"
									>
										{database.create && nameError && (
											<StatusNote tone="error">
												<p>{nameError}</p>
											</StatusNote>
										)}
										<NoteDetail>
											{database.create
												? savedHere
													? "Created by meroku."
													: "meroku creates it at the next deploy."
												: "Already in Neon; meroku uses it and never changes it."}
											{usedBy.length > 0 &&
												` Used by ${usedBy.join(", ")} — remove their database access before removing it.`}
										</NoteDetail>
									</div>
								</div>
							);
						})}
					</div>
					<div className="space-y-1">
						<Label htmlFor={`${uid}-add-database`}>Add a database</Label>
						<Select value="" onValueChange={addDatabase}>
							<SelectTrigger id={`${uid}-add-database`}>
								<SelectValue placeholder="Choose a database to add" />
							</SelectTrigger>
							<SelectContent>
								{addChoices.map((choice) => (
									<SelectItem key={choice.value} value={choice.value}>
										{choice.label}
									</SelectItem>
								))}
							</SelectContent>
						</Select>
						{!authOk && draft.mode === "reuse" && (
							<NoteDetail>
								Log in to Neon to pick from the project's databases, or enter a
								name under Advanced.
							</NoteDetail>
						)}
					</div>
					<AdvancedSection
						open={advancedOpen}
						onOpenChange={setAdvancedOpen}
						label="Advanced: add an existing database by name"
					>
						<div className="space-y-1">
							<Label htmlFor={`${uid}-manual-database`}>
								Existing database name
							</Label>
							<div className="flex items-center gap-2">
								<Input
									id={`${uid}-manual-database`}
									value={manualDatabase}
									placeholder="app"
									autoComplete="off"
									spellCheck={false}
									aria-invalid={manualError !== null}
									aria-describedby={`${uid}-manual-database-hint`}
									onChange={(event) => setManualDatabase(event.target.value)}
								/>
								<Button
									type="button"
									variant="outline"
									disabled={manualName === "" || manualError !== null}
									onClick={() => {
										addDatabase(manualName);
										setManualDatabase("");
									}}
								>
									<Plus aria-hidden="true" />
									Add
								</Button>
							</div>
							<div id={`${uid}-manual-database-hint`} aria-live="polite">
								{manualError ? (
									<StatusNote tone="error">
										<p>{manualError}</p>
									</StatusNote>
								) : (
									<NoteDetail>
										The database must already exist on the project's main
										branch. meroku checks it at deploy.
									</NoteDetail>
								)}
							</div>
						</div>
					</AdvancedSection>
				</PanelCard>
				{footer}
				{confirmDialog}
			</div>
		);
	}

	// Project tab.
	const pgVersion =
		draft.mode === "reuse" ? draft.reuse.pg_version : draft.create.pg_version;
	const chosenProject = projects.find(
		(project) => project.id === draft.reuse.project_id,
	);
	const projectIdError =
		draft.mode === "reuse" && draft.reuse.project_id !== ""
			? checkNeonProjectId(draft.reuse.project_id)
			: null;
	const nameError =
		draft.mode === "create" ? checkNeonProjectName(draft.create.name) : null;
	const orgError =
		draft.mode === "create" && draft.create.org_id !== ""
			? checkNeonOrgId(draft.create.org_id)
			: null;
	const regionError =
		draft.mode === "create" && draft.create.region_id !== ""
			? checkNeonRegionId(draft.create.region_id)
			: null;
	const ownerError =
		draft.owner_role === ""
			? null
			: checkNeonIdentifier(draft.owner_role, "owner role name");
	const regionChoices = NEON_REGIONS.some(
		(region) => region.id === draft.create.region_id,
	)
		? NEON_REGIONS
		: draft.create.region_id && regionError === null
			? [
					...NEON_REGIONS,
					{ id: draft.create.region_id, label: draft.create.region_id },
				]
			: NEON_REGIONS;
	const ownerChoices = [
		...new Set([
			NEON_DEFAULT_OWNER_ROLE,
			...liveDatabases.map((database) => database.owner),
			...(ownerError === null && draft.owner_role ? [draft.owner_role] : []),
		]),
	];
	const servicesBlocking = accessEntries.length > 0;

	return (
		<div className="space-y-4">
			{lookupStatus}
			<PanelCard
				title="Neon project"
				description="A Neon project is a trust zone. Give prod its own project."
			>
				<RadioGroup
					value={draft.mode}
					aria-label="Neon project"
					onValueChange={(value) =>
						update((d) => ({ ...d, mode: value as NeonDraft["mode"] }))
					}
				>
					<div className="flex items-start gap-2">
						<RadioGroupItem id={`${uid}-mode-create`} value="create" />
						<div className="space-y-1">
							<Label htmlFor={`${uid}-mode-create`}>Create a new project</Label>
							<NoteDetail>
								meroku creates the project at the next deploy.
							</NoteDetail>
						</div>
					</div>
					<div className="flex items-start gap-2">
						<RadioGroupItem id={`${uid}-mode-reuse`} value="reuse" />
						<div className="space-y-1">
							<Label htmlFor={`${uid}-mode-reuse`}>
								Use an existing project
							</Label>
							<NoteDetail>
								meroku adds databases and logins to a project you already have.
							</NoteDetail>
						</div>
					</div>
				</RadioGroup>

				{draft.mode === "create" ? (
					<>
						<div className="space-y-1">
							<Label htmlFor={`${uid}-project-name`}>Project name</Label>
							<Input
								id={`${uid}-project-name`}
								value={draft.create.name}
								autoComplete="off"
								spellCheck={false}
								aria-invalid={nameError !== null}
								aria-describedby={`${uid}-project-name-hint`}
								onChange={(event) => {
									const name = event.target.value;
									update((d) => ({ ...d, create: { ...d.create, name } }));
								}}
							/>
							<div id={`${uid}-project-name-hint`} aria-live="polite">
								{nameError ? (
									<StatusNote tone="error">
										<p>{nameError}</p>
									</StatusNote>
								) : (
									<NoteDetail>The name you see in the Neon console.</NoteDetail>
								)}
							</div>
						</div>

						<div className="space-y-1">
							<Label htmlFor={`${uid}-org`}>Organization</Label>
							{organizations.length > 0 ? (
								<Select
									value={draft.create.org_id}
									onValueChange={(org_id) =>
										update((d) => ({ ...d, create: { ...d.create, org_id } }))
									}
								>
									<SelectTrigger id={`${uid}-org`}>
										<SelectValue placeholder="Choose your Neon organization" />
									</SelectTrigger>
									<SelectContent>
										{organizations.map((org) => (
											<SelectItem key={org.id} value={org.id}>
												{org.name}
											</SelectItem>
										))}
									</SelectContent>
								</Select>
							) : draft.create.org_id && orgError === null ? (
								<NoteCode id={`${uid}-org`}>{draft.create.org_id}</NoteCode>
							) : (
								<NoteDetail id={`${uid}-org`}>
									Log in to Neon to choose your organization, or enter its id
									under Advanced.
								</NoteDetail>
							)}
						</div>

						<div className="space-y-1">
							<Label htmlFor={`${uid}-region`}>Region</Label>
							<Select
								value={regionError === null ? draft.create.region_id : ""}
								onValueChange={(region_id) =>
									update((d) => ({ ...d, create: { ...d.create, region_id } }))
								}
							>
								<SelectTrigger id={`${uid}-region`}>
									<SelectValue placeholder="Choose where the data lives" />
								</SelectTrigger>
								<SelectContent>
									{regionChoices.map((region) => (
										<SelectItem key={region.id} value={region.id}>
											{neonRegionLabel(region.id)}
										</SelectItem>
									))}
								</SelectContent>
							</Select>
							<NoteDetail>
								Pick the region closest to this environment's servers
								{neonRegionForAwsRegion(config.region)
									? ` (${neonRegionForAwsRegion(config.region)}).`
									: "."}
							</NoteDetail>
						</div>
					</>
				) : (
					<div className="space-y-3">
						{authOk && organizations.length > 1 && (
							<div className="space-y-1">
								<Label htmlFor={`${uid}-reuse-org`}>Organization</Label>
								<Select value={reuseOrg} onValueChange={setReuseOrg}>
									<SelectTrigger id={`${uid}-reuse-org`}>
										<SelectValue placeholder="Choose your Neon organization" />
									</SelectTrigger>
									<SelectContent>
										{organizations.map((org) => (
											<SelectItem key={org.id} value={org.id}>
												{org.name}
											</SelectItem>
										))}
									</SelectContent>
								</Select>
							</div>
						)}
						<div className="space-y-1">
							<Label htmlFor={`${uid}-project`}>Project</Label>
							{authOk ? (
								<Select
									value={chosenProject ? chosenProject.id : ""}
									onValueChange={(projectId) => {
										const project = projects.find((p) => p.id === projectId);
										update((d) => ({
											...d,
											reuse: {
												project_id: projectId,
												pg_version:
													project && isReusePgVersion(project.pg_version)
														? project.pg_version
														: d.reuse.pg_version,
											},
										}));
									}}
								>
									<SelectTrigger id={`${uid}-project`}>
										<SelectValue placeholder="Choose one of your projects" />
									</SelectTrigger>
									<SelectContent>
										{projects.map((project) => (
											<SelectItem key={project.id} value={project.id}>
												{project.name} · {neonRegionLabel(project.region_id)}
											</SelectItem>
										))}
									</SelectContent>
								</Select>
							) : draft.reuse.project_id && projectIdError === null ? (
								<NoteCode id={`${uid}-project`}>
									{draft.reuse.project_id}
								</NoteCode>
							) : (
								<NoteDetail id={`${uid}-project`}>
									Log in to Neon to choose a project, or enter its id under
									Advanced.
								</NoteDetail>
							)}
						</div>
						{draft.reuse.project_id !== "" && (
							<div className="flex flex-wrap items-center gap-2">
								<NoteDetail>Postgres version:</NoteDetail>
								{pgVersion !== undefined ? (
									<Badge variant="outline">Postgres {pgVersion}</Badge>
								) : (
									<Badge variant="secondary">unknown — checked at deploy</Badge>
								)}
							</div>
						)}
						{pgVersion !== undefined &&
							pgVersion < NEON_MIN_ACCESS_PG_VERSION && (
								<StatusNote tone="warning">
									<p>
										On Postgres {pgVersion} services cannot get database access.
										Upgrade the project in Neon to 15 or later.
									</p>
								</StatusNote>
							)}
					</div>
				)}

				<AdvancedSection
					open={advancedOpen}
					onOpenChange={setAdvancedOpen}
					label="Advanced: enter ids yourself"
				>
					<div className="space-y-4">
						{draft.mode === "reuse" ? (
							<>
								<ManualField
									id={`${uid}-manual-project`}
									label="Project id"
									value={draft.reuse.project_id}
									placeholder="fake-project-000000"
									error={projectIdError}
									hint="Copy it from the project's settings in the Neon console."
									onChange={(project_id) =>
										update((d) => ({
											...d,
											reuse: { ...d.reuse, project_id },
										}))
									}
								/>
								<div className="space-y-1">
									<Label htmlFor={`${uid}-reuse-pg`}>Postgres version</Label>
									<Select
										value={
											draft.reuse.pg_version !== undefined
												? String(draft.reuse.pg_version)
												: ""
										}
										onValueChange={(value) =>
											update((d) => ({
												...d,
												reuse: {
													...d.reuse,
													pg_version: Number(value) as NeonReusePgVersion,
												},
											}))
										}
									>
										<SelectTrigger id={`${uid}-reuse-pg`}>
											<SelectValue placeholder="unknown — checked at deploy" />
										</SelectTrigger>
										<SelectContent>
											{NEON_REUSE_PG_VERSIONS.map((version) => (
												<SelectItem key={version} value={String(version)}>
													Postgres {version}
												</SelectItem>
											))}
										</SelectContent>
									</Select>
									<NoteDetail>
										Must match the project's version; meroku checks it at
										deploy.
									</NoteDetail>
								</div>
							</>
						) : (
							<>
								<ManualField
									id={`${uid}-manual-org`}
									label="Organization id"
									value={draft.create.org_id}
									placeholder="org-fake-000000"
									error={orgError}
									hint="Copy it from your organization's settings in the Neon console."
									onChange={(org_id) =>
										update((d) => ({ ...d, create: { ...d.create, org_id } }))
									}
								/>
								<ManualField
									id={`${uid}-manual-region`}
									label="Region id"
									value={draft.create.region_id}
									placeholder="aws-us-east-1"
									error={regionError}
									hint="For a region not in the list above."
									onChange={(region_id) =>
										update((d) => ({
											...d,
											create: { ...d.create, region_id },
										}))
									}
								/>
								<div className="space-y-1">
									<Label htmlFor={`${uid}-create-pg`}>Postgres version</Label>
									<Select
										value={String(draft.create.pg_version)}
										onValueChange={(value) =>
											update((d) => ({
												...d,
												create: {
													...d.create,
													pg_version: Number(value) as NeonCreatePgVersion,
												},
											}))
										}
									>
										<SelectTrigger id={`${uid}-create-pg`}>
											<SelectValue />
										</SelectTrigger>
										<SelectContent>
											{NEON_CREATE_PG_VERSIONS.map((version) => (
												<SelectItem key={version} value={String(version)}>
													Postgres {version}
												</SelectItem>
											))}
										</SelectContent>
									</Select>
								</div>
							</>
						)}
						<div className="space-y-1">
							<Label htmlFor={`${uid}-owner`}>Owner role</Label>
							<Select
								value={ownerError === null ? draft.owner_role : ""}
								onValueChange={(owner_role) =>
									update((d) => ({ ...d, owner_role }))
								}
							>
								<SelectTrigger id={`${uid}-owner`}>
									<SelectValue placeholder="Choose the owner role" />
								</SelectTrigger>
								<SelectContent>
									{ownerChoices.map((role) => (
										<SelectItem key={role} value={role}>
											{role}
										</SelectItem>
									))}
								</SelectContent>
							</Select>
							<NoteDetail>
								Owns every table and schema. Run your migrations as this role.
							</NoteDetail>
						</div>
						<ManualField
							id={`${uid}-manual-owner`}
							label="Or type a role name"
							value={draft.owner_role}
							placeholder={NEON_DEFAULT_OWNER_ROLE}
							error={ownerError}
							hint="A role that owns the project's databases."
							onChange={(owner_role) => update((d) => ({ ...d, owner_role }))}
						/>
					</div>
				</AdvancedSection>
			</PanelCard>

			{footer}

			{saved && (
				<PanelCard
					title="Disconnect Neon"
					description="Stops using Neon in this environment. meroku never deletes Neon data: the project and its databases stay in Neon."
				>
					{servicesBlocking && (
						<NoteDetail>
							Turn off the database access of{" "}
							{accessEntries
								.map((entry) =>
									entry.target.kind === "backend"
										? "Backend"
										: entry.target.name,
								)
								.join(", ")}{" "}
							first.
						</NoteDetail>
					)}
					<Button
						type="button"
						variant="outline"
						disabled={servicesBlocking}
						onClick={() => setRemoveOpen(true)}
					>
						<Unlink aria-hidden="true" />
						Disconnect Neon from this environment
					</Button>
					<AlertDialog open={removeOpen} onOpenChange={setRemoveOpen}>
						<AlertDialogContent>
							<AlertDialogHeader>
								<AlertDialogTitle>
									Disconnect Neon from this environment?
								</AlertDialogTitle>
								<AlertDialogDescription>
									The project and its databases stay in Neon with their data. At
									the next deploy meroku stops managing them and prints the
									command to delete them yourself.
								</AlertDialogDescription>
							</AlertDialogHeader>
							<AlertDialogFooter>
								<AlertDialogCancel>Cancel</AlertDialogCancel>
								<AlertDialogAction onClick={removeNeon}>
									Disconnect
								</AlertDialogAction>
							</AlertDialogFooter>
						</AlertDialogContent>
					</AlertDialog>
				</PanelCard>
			)}
			{confirmDialog}
		</div>
	);
}

function PanelCard({
	title,
	description,
	children,
}: {
	title: string;
	description: string;
	children: ReactNode;
}) {
	return (
		<Card>
			<CardHeader>
				<CardTitle className="flex items-center gap-2">
					<Database className="size-4" aria-hidden="true" />
					{title}
				</CardTitle>
				<CardDescription>{description}</CardDescription>
			</CardHeader>
			<CardContent className="space-y-5">{children}</CardContent>
		</Card>
	);
}

function AdvancedSection({
	open,
	onOpenChange,
	label,
	children,
}: {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	label: string;
	children: ReactNode;
}) {
	return (
		<Collapsible open={open} onOpenChange={onOpenChange}>
			<CollapsibleTrigger asChild>
				<Button type="button" variant="ghost" size="sm" aria-expanded={open}>
					<ChevronDown aria-hidden="true" />
					{label}
				</Button>
			</CollapsibleTrigger>
			<CollapsibleContent>
				<div className="pt-2">{children}</div>
			</CollapsibleContent>
		</Collapsible>
	);
}

/** A typed value with its inline error, checked on every keystroke. */
function ManualField({
	id,
	label,
	value,
	placeholder,
	error,
	hint,
	onChange,
}: {
	id: string;
	label: string;
	value: string;
	placeholder: string;
	error: string | null;
	hint: string;
	onChange: (value: string) => void;
}) {
	return (
		<div className="space-y-1">
			<Label htmlFor={id}>{label}</Label>
			<Input
				id={id}
				value={value}
				placeholder={placeholder}
				autoComplete="off"
				spellCheck={false}
				aria-invalid={error !== null}
				aria-describedby={`${id}-hint`}
				onChange={(event) => onChange(event.target.value.trim())}
			/>
			<div id={`${id}-hint`} aria-live="polite">
				{error ? (
					<StatusNote tone="error">
						<p>{error}</p>
					</StatusNote>
				) : (
					<NoteDetail>{hint}</NoteDetail>
				)}
			</div>
		</div>
	);
}

function LookupStatus({
	lookup,
	onRefresh,
}: {
	lookup: LookupState;
	onRefresh: () => void;
}) {
	const refresh = (
		<Button type="button" variant="outline" size="sm" onClick={onRefresh}>
			<RefreshCw aria-hidden="true" />
			Refresh
		</Button>
	);

	if (lookup.status === "loading") {
		return (
			<StatusNote tone="pending">
				<p>Looking up your Neon projects…</p>
			</StatusNote>
		);
	}
	if (lookup.status === "failed") {
		return (
			<StatusNote tone="neutral">
				<p>We couldn't read your Neon projects just now.</p>
				<NoteDetail>{lookup.error}</NoteDetail>
				{refresh}
			</StatusNote>
		);
	}
	const { auth, note } = lookup.data;
	if (auth.state !== "ok") {
		return (
			<StatusNote tone="neutral" title={auth.message}>
				<p>
					Log in to Neon with the Neon CLI to pick from your projects: run{" "}
					<NoteCode>neon login</NoteCode> in a terminal, then Refresh.
				</p>
				{auth.state === "neonctl_missing" && (
					<NoteDetail>
						The Neon CLI is not installed. Install it with{" "}
						<NoteCode>npm i -g neon</NoteCode>.
					</NoteDetail>
				)}
				{auth.state === "expired" && (
					<NoteDetail>Your Neon login has expired.</NoteDetail>
				)}
				{refresh}
			</StatusNote>
		);
	}
	if (note) {
		return (
			<StatusNote tone="neutral">
				<p>{note}</p>
				{refresh}
			</StatusNote>
		);
	}
	return null;
}

function SaveSection({
	dirty,
	problems,
	replacing,
	deployed,
	onSave,
	onDiscard,
}: {
	dirty: boolean;
	problems: string[];
	replacing: string[];
	deployed: Deployed;
	onSave: () => void;
	onDiscard: () => void;
}) {
	if (!dirty && problems.length === 0) return null;
	return (
		<div className="space-y-3">
			{problems.length > 0 && (
				<div className="space-y-2" aria-live="polite">
					{problems.map((problem) => (
						<StatusNote key={problem} tone="error">
							<p>{problem}</p>
						</StatusNote>
					))}
				</div>
			)}
			{dirty && replacing.length > 0 && (
				<Alert>
					<AlertTitle>
						{deployed === "yes"
							? "This change replaces data in a deployed project"
							: "If this project is deployed, this change replaces data"}
					</AlertTitle>
					<AlertDescription>
						{replacing.map((message) => (
							<p key={message}>{message}</p>
						))}
					</AlertDescription>
				</Alert>
			)}
			{dirty && (
				<div className="flex flex-wrap gap-2">
					<Button type="button" disabled={problems.length > 0} onClick={onSave}>
						Save Neon settings
					</Button>
					<Button type="button" variant="outline" onClick={onDiscard}>
						Discard changes
					</Button>
				</div>
			)}
		</div>
	);
}
