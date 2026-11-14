import {
	ChevronDown,
	Database,
	Plus,
	RefreshCw,
	Trash2,
	Unlink,
} from "lucide-react";
import { type ReactNode, useEffect, useId, useRef, useState } from "react";
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
	type NeonConfigFields,
	type NeonReplacingChange,
	neonReplacingChanges,
	neonValuesFromConfig,
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
 * Valid changes write immediately through `onConfigChange`. Changes that
 * replace or release existing Neon resources ask for confirmation first, and
 * revert the control when cancelled.
 */

export type NeonPanelSection = "settings" | "databases";

export type NeonLookupFn = (
	env: string,
	opts: NeonLookupOptions,
) => Promise<NeonLookupResponse>;

export interface NeonNodePropertiesProps {
	config: YamlInfrastructureConfig;
	onConfigChange: (config: Partial<YamlInfrastructureConfig>) => void;
	/** Which tab is showing. */
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

interface PendingConfirm {
	next: YamlNeonConfig;
	replacing: NeonReplacingChange[];
}

const NEW_DATABASE = "__new__";

const defaultLookup: NeonLookupFn = (env, opts) =>
	infrastructureApi.getNeonLookup(env, opts);

function isAbort(err: unknown): boolean {
	return err instanceof DOMException && err.name === "AbortError";
}

function baseValues(config: YamlInfrastructureConfig): NeonConfigFields {
	return neonValuesFromConfig(config.neon, {
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

	const [settingUp, setSettingUp] = useState(false);
	const enabled = saved !== undefined || settingUp;

	const [fields, setFields] = useState<NeonConfigFields>(() =>
		baseValues(config),
	);
	const fieldsRef = useRef(fields);
	fieldsRef.current = fields;

	const [reuseOrg, setReuseOrg] = useState("");
	const [advancedOpen, setAdvancedOpen] = useState(false);
	const [manualDatabase, setManualDatabase] = useState("");
	const [blockedProblems, setBlockedProblems] = useState<string[]>([]);
	const [pendingConfirm, setPendingConfirm] = useState<PendingConfirm | null>(
		null,
	);
	const pendingConfirmRef = useRef<PendingConfirm | null>(null);
	pendingConfirmRef.current = pendingConfirm;
	const confirmedRef = useRef(false);
	const [removeOpen, setRemoveOpen] = useState(false);

	const [lookup, setLookup] = useState<LookupState>({ status: "loading" });
	const [reloadKey, setReloadKey] = useState(0);

	// Sync local input buffers when the saved YAML block changes externally.
	// biome-ignore lint/correctness/useExhaustiveDependencies: savedKey stands for config.neon by value.
	useEffect(() => {
		setSettingUp(false);
		setBlockedProblems([]);
		setFields((current) => {
			if (!config.neon) {
				const fresh = baseValues(config);
				fieldsRef.current = fresh;
				return fresh;
			}
			if (JSON.stringify(buildNeonUpdate(current, config.neon)) === savedKey) {
				return current;
			}
			const fresh = baseValues(config);
			fieldsRef.current = fresh;
			return fresh;
		});
	}, [savedKey]);

	const lookupOrg =
		fields.mode === "create"
			? checkNeonOrgId(fields.create.org_id) === null
				? fields.create.org_id
				: undefined
			: reuseOrg || undefined;
	const lookupProject =
		fields.mode === "reuse" &&
		checkNeonProjectId(fields.reuse.project_id) === null
			? fields.reuse.project_id
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
	const deployedDbSet = authOk
		? new Set(liveDatabases.map((database) => database.name))
		: undefined;

	function openConfirm(next: YamlNeonConfig, replacing: NeonReplacingChange[]) {
		const pending = { next, replacing };
		pendingConfirmRef.current = pending;
		setPendingConfirm(pending);
	}

	function commitDiscrete(nextFields: NeonConfigFields) {
		setBlockedProblems([]);
		const next = buildNeonUpdate(nextFields, saved);
		const problems = neonProjectProblems(
			validateNeonConfig({ ...config, neon: next }),
		);
		if (problems.length > 0) {
			const crossField = problems
				.filter(
					(problem) => problem.rule === "V-N28" || problem.rule === "V-N9",
				)
				.map((problem) => problem.message);
			if (crossField.length > 0) {
				setBlockedProblems(crossField);
				return;
			}
			fieldsRef.current = nextFields;
			setFields(nextFields);
			return;
		}

		if (JSON.stringify(next) === savedKey) {
			fieldsRef.current = nextFields;
			setFields(nextFields);
			return;
		}

		const replacing =
			saved && deployed !== "no"
				? neonReplacingChanges(saved, next, deployedDbSet)
				: [];
		if (replacing.length > 0) {
			openConfirm(next, replacing);
			return;
		}

		fieldsRef.current = nextFields;
		setFields(nextFields);
		onConfigChange({ neon: next });
	}

	function handleTextChange(nextFields: NeonConfigFields) {
		setBlockedProblems([]);
		fieldsRef.current = nextFields;
		setFields(nextFields);

		const next = buildNeonUpdate(nextFields, saved);
		const problems = neonProjectProblems(
			validateNeonConfig({ ...config, neon: next }),
		);
		if (problems.length > 0 || JSON.stringify(next) === savedKey) return;

		const replacing =
			saved && deployed !== "no"
				? neonReplacingChanges(saved, next, deployedDbSet)
				: [];
		if (replacing.length > 0) return;

		onConfigChange({ neon: next });
	}

	function commitPendingText() {
		if (pendingConfirmRef.current !== null) return;
		const currentFields = fieldsRef.current;
		const next = buildNeonUpdate(currentFields, saved);
		const problems = neonProjectProblems(
			validateNeonConfig({ ...config, neon: next }),
		);
		if (problems.length > 0 || JSON.stringify(next) === savedKey) return;

		const replacing =
			saved && deployed !== "no"
				? neonReplacingChanges(saved, next, deployedDbSet)
				: [];
		if (replacing.length > 0) {
			openConfirm(next, replacing);
			return;
		}

		onConfigChange({ neon: next });
	}

	function confirmPendingChange() {
		const pending = pendingConfirmRef.current;
		if (!pending) return;
		confirmedRef.current = true;
		pendingConfirmRef.current = null;
		setPendingConfirm(null);
		const nextFields = neonValuesFromConfig(pending.next, {
			projectName: defaultNeonProjectName(config.project, config.env),
			regionId: neonRegionForAwsRegion(config.region),
		});
		fieldsRef.current = nextFields;
		setFields(nextFields);
		onConfigChange({ neon: pending.next });
	}

	function cancelPendingChange() {
		pendingConfirmRef.current = null;
		setPendingConfirm(null);
		const restored = baseValues(config);
		fieldsRef.current = restored;
		setFields(restored);
	}

	// One organization: choose it, there is nothing to decide.
	// biome-ignore lint/correctness/useExhaustiveDependencies: auto-selects the single organization once loaded.
	useEffect(() => {
		if (!enabled || organizations.length !== 1) return;
		const only = organizations[0].id;
		if (fields.mode === "create" && fields.create.org_id === "") {
			commitDiscrete({
				...fieldsRef.current,
				create: { ...fieldsRef.current.create, org_id: only },
			});
		}
		if (fields.mode === "reuse" && reuseOrg === "") setReuseOrg(only);
	}, [enabled, organizations, fields.mode, fields.create.org_id, reuseOrg]);

	const savedProblems = saved
		? neonProjectProblems(validateNeonConfig(config)).map(
				(problem) => problem.message,
			)
		: [];
	const displayedProblems = [
		...new Set([...savedProblems, ...blockedProblems]),
	];
	const accessEntries = neonAccessEntries(config);

	function removeNeon() {
		setSettingUp(false);
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
				<Button type="button" onClick={() => setSettingUp(true)}>
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

	const problemsList =
		displayedProblems.length > 0 ? (
			<div className="space-y-2" aria-live="polite">
				{displayedProblems.map((problem) => (
					<StatusNote key={problem} tone="error">
						<p>{problem}</p>
					</StatusNote>
				))}
			</div>
		) : null;

	const confirmDialog = (
		<AlertDialog
			open={pendingConfirm !== null}
			onOpenChange={(open) => {
				if (!open) {
					if (confirmedRef.current) {
						confirmedRef.current = false;
						pendingConfirmRef.current = null;
						setPendingConfirm(null);
					} else {
						cancelPendingChange();
					}
				}
			}}
		>
			<AlertDialogContent>
				<AlertDialogHeader>
					<AlertDialogTitle>
						Make a change that replaces Neon data?
					</AlertDialogTitle>
					<AlertDialogDescription>
						{deployed === "yes"
							? "This project already exists in Neon."
							: "We couldn't check whether this project already exists in Neon."}{" "}
						Read what happens at the next deploy:
					</AlertDialogDescription>
				</AlertDialogHeader>
				<div className="space-y-2">
					{(pendingConfirm?.replacing ?? []).map((change) => (
						<StatusNote key={change.kind + change.message} tone="warning">
							<p>{change.message}</p>
						</StatusNote>
					))}
				</div>
				<AlertDialogFooter>
					<AlertDialogCancel>Cancel</AlertDialogCancel>
					<AlertDialogAction onClick={confirmPendingChange}>
						Confirm
					</AlertDialogAction>
				</AlertDialogFooter>
			</AlertDialogContent>
		</AlertDialog>
	);

	if (section === "databases") {
		if (!saved) {
			return (
				<div className="space-y-4">
					{lookupStatus}
					<PanelCard
						title="Databases"
						description="The databases your services can use. A database you add as new is created by meroku; an existing one is used as it is."
					>
						<StatusNote tone="neutral">
							<p>Finish setting up the Neon project first.</p>
							<NoteDetail>
								Choose the project on the Project tab, then come back here to
								add databases.
							</NoteDetail>
						</StatusNote>
					</PanelCard>
				</div>
			);
		}

		const taken = new Set(fields.databases.map((database) => database.name));
		const addChoices = [
			...(fields.mode === "create" && !taken.has(NEON_PROJECT_DEFAULT_DATABASE)
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
			const current = fieldsRef.current;
			const names = new Set(current.databases.map((database) => database.name));
			const database =
				value === NEW_DATABASE
					? { name: nextNewDatabaseName(names), create: true }
					: { name: value, create: false };
			commitDiscrete({
				...current,
				databases: [...current.databases, database],
			});
		}

		function addManualDatabase() {
			if (manualName === "" || manualError !== null) return;
			addDatabase(manualName);
			setManualDatabase("");
		}

		return (
			<div className="space-y-4">
				{lookupStatus}
				<PanelCard
					title="Databases"
					description="The databases your services can use. A database you add as new is created by meroku; an existing one is used as it is."
				>
					{fields.mode === "create" && (
						<NoteDetail>
							A new project comes with a database called{" "}
							<NoteCode>{NEON_PROJECT_DEFAULT_DATABASE}</NoteCode>.
						</NoteDetail>
					)}
					{fields.databases.length === 0 && (
						<StatusNote tone="neutral">
							<p>No databases yet. Add one so your services can connect.</p>
						</StatusNote>
					)}
					<div className="space-y-4">
						{fields.databases.map((database, index) => {
							const usedBy = neonServicesUsingDatabase(config, database.name);
							const trimmedName = database.name.trim();
							const isDuplicate = fields.databases.some(
								(entry, i) => i !== index && entry.name.trim() === trimmedName,
							);
							const nameError =
								checkNeonDatabaseName(trimmedName, database.create === true) ??
								(isDuplicate
									? `The database ${trimmedName} is listed twice. Keep one.`
									: null);
							const savedDb = saved.databases?.[index];
							const isDeployedDb =
								savedDb?.create === true &&
								(deployed === "unknown" ||
									Boolean(deployedDbSet?.has(savedDb.name)));
							const renameWarning =
								database.create &&
								nameError === null &&
								deployed !== "no" &&
								isDeployedDb &&
								savedDb &&
								savedDb.name !== trimmedName
									? `The database ${savedDb.name} is kept in Neon with its data, and a new empty database ${trimmedName} is created.`
									: null;
							const alreadyInNeon = Boolean(deployedDbSet?.has(trimmedName));
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
													const current = fieldsRef.current;
													handleTextChange({
														...current,
														databases: current.databases.map((entry, i) =>
															i === index ? { ...entry, name } : entry,
														),
													});
												}}
												onBlur={commitPendingText}
												onKeyDown={(event) => {
													if (event.key === "Enter") {
														event.preventDefault();
														commitPendingText();
													}
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
											onClick={() => {
												const current = fieldsRef.current;
												commitDiscrete({
													...current,
													databases: current.databases.filter(
														(_, i) => i !== index,
													),
												});
											}}
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
										{renameWarning && (
											<StatusNote tone="warning">
												<p>{renameWarning}</p>
											</StatusNote>
										)}
										<NoteDetail>
											{database.create
												? alreadyInNeon
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
						{!authOk && fields.mode === "reuse" && (
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
									onKeyDown={(event) => {
										if (event.key === "Enter") {
											event.preventDefault();
											addManualDatabase();
										}
									}}
								/>
								<Button
									type="button"
									variant="outline"
									disabled={manualName === "" || manualError !== null}
									onClick={addManualDatabase}
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
				{problemsList}
				{confirmDialog}
			</div>
		);
	}

	// Project tab.
	const pgVersion =
		fields.mode === "reuse"
			? fields.reuse.pg_version
			: fields.create.pg_version;
	const chosenProject = projects.find(
		(project) => project.id === fields.reuse.project_id,
	);
	const projectIdError =
		fields.mode === "reuse" && fields.reuse.project_id !== ""
			? checkNeonProjectId(fields.reuse.project_id)
			: null;
	const nameError =
		fields.mode === "create" ? checkNeonProjectName(fields.create.name) : null;
	const orgError =
		fields.mode === "create" && fields.create.org_id !== ""
			? checkNeonOrgId(fields.create.org_id)
			: null;
	const regionError =
		fields.mode === "create" && fields.create.region_id !== ""
			? checkNeonRegionId(fields.create.region_id)
			: null;
	const ownerError =
		fields.owner_role === ""
			? null
			: checkNeonIdentifier(fields.owner_role, "owner role name");
	const regionChoices = NEON_REGIONS.some(
		(region) => region.id === fields.create.region_id,
	)
		? NEON_REGIONS
		: fields.create.region_id && regionError === null
			? [
					...NEON_REGIONS,
					{ id: fields.create.region_id, label: fields.create.region_id },
				]
			: NEON_REGIONS;
	const ownerChoices = [
		...new Set([
			NEON_DEFAULT_OWNER_ROLE,
			...liveDatabases.map((database) => database.owner),
			...(ownerError === null && fields.owner_role ? [fields.owner_role] : []),
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
					value={fields.mode}
					aria-label="Neon project"
					onValueChange={(value) => {
						const mode = value as NeonConfigFields["mode"];
						const current = fieldsRef.current;
						const nextCreate =
							mode === "create" &&
							current.create.org_id === "" &&
							reuseOrg !== ""
								? { ...current.create, org_id: reuseOrg }
								: current.create;
						commitDiscrete({ ...current, mode, create: nextCreate });
					}}
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

				{fields.mode === "create" ? (
					<>
						<div className="space-y-1">
							<Label htmlFor={`${uid}-project-name`}>Project name</Label>
							<Input
								id={`${uid}-project-name`}
								value={fields.create.name}
								autoComplete="off"
								spellCheck={false}
								aria-invalid={nameError !== null}
								aria-describedby={`${uid}-project-name-hint`}
								onChange={(event) => {
									const name = event.target.value;
									const current = fieldsRef.current;
									handleTextChange({
										...current,
										create: { ...current.create, name },
									});
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
									value={fields.create.org_id}
									onValueChange={(org_id) => {
										const current = fieldsRef.current;
										commitDiscrete({
											...current,
											create: { ...current.create, org_id },
										});
									}}
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
							) : fields.create.org_id && orgError === null ? (
								<NoteCode id={`${uid}-org`}>{fields.create.org_id}</NoteCode>
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
								value={regionError === null ? fields.create.region_id : ""}
								onValueChange={(region_id) => {
									const current = fieldsRef.current;
									commitDiscrete({
										...current,
										create: { ...current.create, region_id },
									});
								}}
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
						{saved?.mode === "create" && deployed !== "no" && (
							<StatusNote tone="warning">
								<p>
									Choose an existing project to switch. The project meroku
									created stays in Neon with its data; meroku stops managing it.
								</p>
							</StatusNote>
						)}
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
										const current = fieldsRef.current;
										commitDiscrete({
											...current,
											reuse: {
												project_id: projectId,
												pg_version:
													project && isReusePgVersion(project.pg_version)
														? project.pg_version
														: current.reuse.pg_version,
											},
										});
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
							) : fields.reuse.project_id && projectIdError === null ? (
								<NoteCode id={`${uid}-project`}>
									{fields.reuse.project_id}
								</NoteCode>
							) : (
								<NoteDetail id={`${uid}-project`}>
									Log in to Neon to choose a project, or enter its id under
									Advanced.
								</NoteDetail>
							)}
						</div>
						{fields.reuse.project_id !== "" && (
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
						{fields.mode === "reuse" ? (
							<>
								<ManualField
									id={`${uid}-manual-project`}
									label="Project id"
									value={fields.reuse.project_id}
									placeholder="fake-project-000000"
									error={projectIdError}
									hint="Copy it from the project's settings in the Neon console."
									onChange={(project_id) => {
										const current = fieldsRef.current;
										handleTextChange({
											...current,
											reuse: { ...current.reuse, project_id },
										});
									}}
									onCommit={commitPendingText}
								/>
								<div className="space-y-1">
									<Label htmlFor={`${uid}-reuse-pg`}>Postgres version</Label>
									<Select
										value={
											fields.reuse.pg_version !== undefined
												? String(fields.reuse.pg_version)
												: ""
										}
										onValueChange={(value) => {
											const current = fieldsRef.current;
											commitDiscrete({
												...current,
												reuse: {
													...current.reuse,
													pg_version: Number(value) as NeonReusePgVersion,
												},
											});
										}}
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
									value={fields.create.org_id}
									placeholder="org-fake-000000"
									error={orgError}
									hint="Copy it from your organization's settings in the Neon console."
									onChange={(org_id) => {
										const current = fieldsRef.current;
										handleTextChange({
											...current,
											create: { ...current.create, org_id },
										});
									}}
									onCommit={commitPendingText}
								/>
								<ManualField
									id={`${uid}-manual-region`}
									label="Region id"
									value={fields.create.region_id}
									placeholder="aws-us-east-1"
									error={regionError}
									hint="For a region not in the list above."
									onChange={(region_id) => {
										const current = fieldsRef.current;
										handleTextChange({
											...current,
											create: { ...current.create, region_id },
										});
									}}
									onCommit={commitPendingText}
								/>
								<div className="space-y-1">
									<Label htmlFor={`${uid}-create-pg`}>Postgres version</Label>
									<Select
										value={String(fields.create.pg_version)}
										onValueChange={(value) => {
											const current = fieldsRef.current;
											commitDiscrete({
												...current,
												create: {
													...current.create,
													pg_version: Number(value) as NeonCreatePgVersion,
												},
											});
										}}
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
								value={ownerError === null ? fields.owner_role : ""}
								onValueChange={(owner_role) => {
									const current = fieldsRef.current;
									commitDiscrete({ ...current, owner_role });
								}}
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
							value={fields.owner_role}
							placeholder={NEON_DEFAULT_OWNER_ROLE}
							error={ownerError}
							hint="A role that owns the project's databases."
							onChange={(owner_role) => {
								const current = fieldsRef.current;
								handleTextChange({ ...current, owner_role });
							}}
							onCommit={commitPendingText}
						/>
					</div>
				</AdvancedSection>
			</PanelCard>

			{problemsList}

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
	onCommit,
}: {
	id: string;
	label: string;
	value: string;
	placeholder: string;
	error: string | null;
	hint: string;
	onChange: (value: string) => void;
	onCommit?: () => void;
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
				onBlur={onCommit}
				onKeyDown={(event) => {
					if (event.key === "Enter") {
						event.preventDefault();
						onCommit?.();
					}
				}}
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
