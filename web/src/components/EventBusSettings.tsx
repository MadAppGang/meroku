import { Plus, Share2, Trash2, Workflow } from "lucide-react";
import { useEffect, useId, useState } from "react";
import { infrastructureApi } from "../api/infrastructure";
import type {
	EventBusShare,
	EventBusShareAccess,
	YamlInfrastructureConfig,
} from "../types/yamlConfig";
import {
	clearExplicitProjectBus,
	clearOrdering,
	ENHANCED_EVENT_BUS_REGIONS,
	type EventBusShareKind,
	enhancedBusAvailableIn,
	eventTaskRulesOnProjectBus,
	explicitProjectBusRefs,
	isEnhancedBus,
	isEventBusEnabled,
	isSubscribeCapable,
	projectBusName,
	type ShareTargetInfo,
	shareDuplicateError,
	shareEntryKind,
	shareEntryLabel,
	sharePrincipalKey,
	shareSubscribersLost,
	shareTargetProblem,
	shareValueError,
} from "../utils/workloadEvents";
import { Alert, AlertDescription } from "./ui/alert";
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
import { Button } from "./ui/button";
import {
	Card,
	CardContent,
	CardDescription,
	CardHeader,
	CardTitle,
} from "./ui/card";
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
import { Separator } from "./ui/separator";
import { NoteDetail, StatusNote } from "./ui/status-note";
import { Switch } from "./ui/switch";

/** Lists the environments the bus can be shared with. */
export type LoadShareTargets = (
	env: string,
	signal: AbortSignal,
) => Promise<ShareTargetInfo[]>;

interface EventBusSettingsProps {
	config: YamlInfrastructureConfig;
	onConfigChange: (config: Partial<YamlInfrastructureConfig>) => void;
	/** The share-targets API by default; stories pass a fixed list. */
	loadShareTargets?: LoadShareTargets;
	/**
	 * The env YAML as loaded from disk, which tells an ordered queue that may
	 * be deployed from one added since. Absent: every one may be deployed.
	 */
	loadedConfig?: YamlInfrastructureConfig;
}

const defaultLoadShareTargets: LoadShareTargets = (env, signal) =>
	infrastructureApi.getEventBusShareTargets(env, { signal });

/** Entries and event-task rules without a bus field, which follow the switch. */
function followingEntries(config: YamlInfrastructureConfig): string[] {
	const out: string[] = [];
	const count = (label: string, list: Array<{ bus?: string }> | undefined) => {
		const n = (list || []).filter((e) => !e.bus).length;
		if (n > 0) out.push(`${label}: ${n}`);
	};
	count("backend subscribes", config.workload?.events?.subscribes);
	count("backend produces", config.workload?.events?.produces);
	for (const s of config.services || []) {
		count(`${s.name} subscribes`, s.events?.subscribes);
		count(`${s.name} produces`, s.events?.produces);
	}
	for (const t of config.scheduled_tasks || []) {
		count(`${t.name} produces`, t.events?.produces);
	}
	for (const t of config.event_processor_tasks || []) {
		count(`${t.name} produces`, t.events?.produces);
		if (t.rules && t.rules.length > 0) count(`${t.name} rules`, t.rules);
		else if (t.rule_name) out.push(`${t.name} rule: 1`);
	}
	return out;
}

const WHO_LABELS: Record<EventBusShareKind, string> = {
	env: "Another environment of this project",
	organization: "Whole organization",
	ou: "Organizational unit",
	account: "Account",
};
const WHO_ORDER: EventBusShareKind[] = ["env", "organization", "ou", "account"];

const ACCESS_LABELS: Record<EventBusShareAccess, string> = {
	publish: "Send events",
	subscribe: "Subscribe to events",
	full: "Send and subscribe",
};

const REVOKE_NOTICE =
	"At the next deploy, every subscriber this account created on the bus is revoked. Revoked subscribers stop for good; they must create new ones if you share again.";

/** A share entry of `kind` naming `value`. */
function shareEntryOf(
	kind: EventBusShareKind,
	value: string,
	access: EventBusShareAccess,
): EventBusShare {
	switch (kind) {
		case "organization":
			return { organization: true, access };
		case "ou":
			return { ou: value, access };
		case "account":
			return { account: value, access };
		case "env":
			return { env: value, access };
	}
}

function shareEntryValue(entry: EventBusShare): string {
	return entry.ou ?? entry.account ?? entry.env ?? "";
}

interface ShareWhoFieldsProps {
	label: string;
	kind: EventBusShareKind;
	value: string;
	valueError: string | null;
	targets: ShareTargetInfo[] | undefined;
	targetsError: string | null;
	project: string;
	onKindChange: (kind: EventBusShareKind) => void;
	/** Typing in the OU or account field. */
	onValueDraft: (value: string) => void;
	/** Leaving the field, pressing Enter, or picking an environment. */
	onValueCommit: (value: string) => void;
}

/** "Who" for one share entry: the kind, then the environment, OU or account. */
function ShareWhoFields({
	label,
	kind,
	value,
	valueError,
	targets,
	targetsError,
	project,
	onKindChange,
	onValueDraft,
	onValueCommit,
}: ShareWhoFieldsProps) {
	const uid = useId();
	return (
		<div className="space-y-2">
			<div className="space-y-1">
				<Label htmlFor={`${uid}-who`}>Who</Label>
				<Select
					value={kind}
					onValueChange={(v) => onKindChange(v as EventBusShareKind)}
				>
					<SelectTrigger id={`${uid}-who`} aria-label={`Who for ${label}`}>
						<SelectValue />
					</SelectTrigger>
					<SelectContent>
						{WHO_ORDER.map((k) => (
							<SelectItem key={k} value={k}>
								{WHO_LABELS[k]}
							</SelectItem>
						))}
					</SelectContent>
				</Select>
			</div>

			{kind === "env" && (
				<div className="space-y-1">
					<Label htmlFor={`${uid}-env`}>Environment</Label>
					<Select
						value={value === "" ? undefined : value}
						onValueChange={onValueCommit}
						disabled={targets === undefined && targetsError === null}
					>
						<SelectTrigger
							id={`${uid}-env`}
							aria-label={`Environment for ${label}`}
							aria-invalid={valueError !== null}
						>
							<SelectValue
								placeholder={
									targets === undefined && targetsError === null
										? "Loading environments…"
										: "Choose an environment"
								}
							/>
						</SelectTrigger>
						<SelectContent>
							{(targets ?? []).map((t) => {
								const problem = shareTargetProblem(t, project);
								return (
									<SelectItem
										key={t.env}
										value={t.env}
										disabled={problem !== null}
									>
										{problem === null
											? `${t.env} (account ${t.accountId}, ${t.region})`
											: `${t.env}: ${problem}`}
									</SelectItem>
								);
							})}
						</SelectContent>
					</Select>
					{targetsError !== null && (
						<StatusNote tone="error">
							The other environments could not be listed: {targetsError}
						</StatusNote>
					)}
					{targets !== undefined && targets.length === 0 && (
						<NoteDetail>
							No other environment of project {project} was found next to this
							one.
						</NoteDetail>
					)}
				</div>
			)}

			{(kind === "ou" || kind === "account") && (
				<div className="space-y-1">
					<Label htmlFor={`${uid}-value`}>
						{kind === "ou" ? "Organizational unit id" : "Account id"}
					</Label>
					<Input
						id={`${uid}-value`}
						aria-label={`${kind === "ou" ? "Organizational unit id" : "Account id"} for ${label}`}
						variant="code"
						value={value}
						placeholder={kind === "ou" ? "ou-ab12-cd34ef56" : "000000000000"}
						inputMode={kind === "account" ? "numeric" : undefined}
						autoComplete="off"
						spellCheck={false}
						aria-invalid={valueError !== null}
						onChange={(e) => onValueDraft(e.target.value.trim())}
						onBlur={() => onValueCommit(value)}
						onKeyDown={(e) => {
							if (e.key === "Enter") onValueCommit(value);
						}}
					/>
				</div>
			)}

			{valueError !== null && (
				<StatusNote tone="error">{valueError}</StatusNote>
			)}
		</div>
	);
}

function AccessSelect({
	label,
	value,
	onChange,
}: {
	label: string;
	value: EventBusShareAccess;
	onChange: (access: EventBusShareAccess) => void;
}) {
	const uid = useId();
	return (
		<div className="space-y-1">
			<Label htmlFor={`${uid}-access`}>What they can do</Label>
			<Select
				value={value}
				onValueChange={(v) => onChange(v as EventBusShareAccess)}
			>
				<SelectTrigger id={`${uid}-access`} aria-label={`What ${label} can do`}>
					<SelectValue />
				</SelectTrigger>
				<SelectContent>
					{(Object.keys(ACCESS_LABELS) as EventBusShareAccess[]).map((a) => (
						<SelectItem key={a} value={a}>
							{ACCESS_LABELS[a]}
						</SelectItem>
					))}
				</SelectContent>
			</Select>
			{isSubscribeCapable(value) && (
				<NoteDetail>They can read every event on this bus.</NoteDetail>
			)}
		</div>
	);
}

interface ShareRowProps {
	entry: EventBusShare;
	index: number;
	share: EventBusShare[];
	initialShare: EventBusShare[];
	config: YamlInfrastructureConfig;
	targets: ShareTargetInfo[] | undefined;
	targetsError: string | null;
	onChange: (index: number, entry: EventBusShare) => void;
	onRemove: (index: number) => void;
}

/**
 * One saved share entry. A kind or value is written only once it is valid;
 * until then the draft shows its error inline.
 */
function ShareRow({
	entry,
	index,
	share,
	initialShare,
	config,
	targets,
	targetsError,
	onChange,
	onRemove,
}: ShareRowProps) {
	const savedKind = shareEntryKind(entry);
	const savedValue = shareEntryValue(entry);
	const [kind, setKind] = useState(savedKind);
	const [value, setValue] = useState(savedValue);
	useEffect(() => {
		setKind(savedKind);
		setValue(savedValue);
	}, [savedKind, savedValue]);

	const draft = shareEntryOf(kind, value, entry.access);
	const valueError = shareValueError(draft, config, targets);
	// The list as it would be saved with this row's draft in place.
	const proposedWith = (next: EventBusShare) =>
		share.map((e, i) => (i === index ? next : e));
	const duplicate =
		valueError === null
			? shareDuplicateError(proposedWith(draft), index, targets)
			: null;
	const label = `share ${index + 1}`;
	// Shared for subscribing when the panel opened, and now only for sending.
	const downgraded =
		!isSubscribeCapable(entry.access) &&
		initialShare.some(
			(old) =>
				isSubscribeCapable(old.access) &&
				sharePrincipalKey(old, targets) === sharePrincipalKey(entry, targets),
		);

	const commit = (nextKind: EventBusShareKind, nextValue: string) => {
		const next = shareEntryOf(nextKind, nextValue, entry.access);
		if (shareValueError(next, config, targets) !== null) return;
		if (shareDuplicateError(proposedWith(next), index, targets) !== null)
			return;
		if (nextKind === savedKind && nextValue === savedValue) return;
		onChange(index, next);
	};

	return (
		<Card size="compact" className="gap-3" data-testid={`share-row-${index}`}>
			<div className="flex items-start justify-between gap-2">
				<div className="min-w-0 flex-1">
					<ShareWhoFields
						label={label}
						kind={kind}
						value={value}
						valueError={valueError}
						targets={targets}
						targetsError={targetsError}
						project={config.project}
						onKindChange={(k) => {
							setKind(k);
							setValue("");
							if (k === "organization") commit(k, "");
						}}
						onValueDraft={setValue}
						onValueCommit={(v) => {
							setValue(v);
							commit(kind, v);
						}}
					/>
				</div>
				<Button
					type="button"
					variant="ghost"
					size="sm"
					aria-label={`Remove ${label}`}
					onClick={() => onRemove(index)}
				>
					<Trash2 className="w-4 h-4" />
				</Button>
			</div>
			<AccessSelect
				label={label}
				value={entry.access}
				onChange={(access) => onChange(index, { ...entry, access })}
			/>
			{duplicate !== null && <StatusNote tone="error">{duplicate}</StatusNote>}
			{downgraded && <StatusNote tone="warning">{REVOKE_NOTICE}</StatusNote>}
		</Card>
	);
}

interface ShareAddFormProps {
	share: EventBusShare[];
	config: YamlInfrastructureConfig;
	targets: ShareTargetInfo[] | undefined;
	targetsError: string | null;
	onAdd: (entry: EventBusShare) => void;
}

/** A new share entry, added only once it names a valid principal. */
function ShareAddForm({
	share,
	config,
	targets,
	targetsError,
	onAdd,
}: ShareAddFormProps) {
	const [kind, setKind] = useState<EventBusShareKind>("env");
	const [value, setValue] = useState("");
	const [access, setAccess] = useState<EventBusShareAccess>("publish");
	const draft = shareEntryOf(kind, value, access);
	const touched = kind === "organization" || value !== "";
	const valueError = touched ? shareValueError(draft, config, targets) : null;
	const duplicate = touched
		? shareDuplicateError([...share, draft], share.length, targets)
		: null;
	const canAdd =
		shareValueError(draft, config, targets) === null &&
		duplicate === null &&
		(kind !== "env" || targets !== undefined);

	return (
		<div className="space-y-3" data-testid="share-add">
			<Label>Share with</Label>
			<ShareWhoFields
				label="the new share"
				kind={kind}
				value={value}
				valueError={valueError}
				targets={targets}
				targetsError={targetsError}
				project={config.project}
				onKindChange={(k) => {
					setKind(k);
					setValue("");
				}}
				onValueDraft={setValue}
				onValueCommit={setValue}
			/>
			<AccessSelect label="the new share" value={access} onChange={setAccess} />
			{duplicate !== null && <StatusNote tone="error">{duplicate}</StatusNote>}
			<Button
				type="button"
				variant="outline"
				size="sm"
				disabled={!canAdd}
				onClick={() => {
					if (!canAdd) return;
					onAdd(draft);
					setValue("");
				}}
			>
				<Plus className="w-4 h-4" />
				Add share
			</Button>
		</div>
	);
}

/**
 * The Settings tab of the EventBridge node: the project bus switch, its kind
 * (Standard or Enhanced), and, for an Enhanced bus, the accounts it is shared
 * with.
 */
export function EventBusSettings({
	config,
	onConfigChange,
	loadShareTargets = defaultLoadShareTargets,
	loadedConfig,
}: EventBusSettingsProps) {
	const uid = useId();
	const enabled = isEventBusEnabled(config);
	const enhanced = isEnhancedBus(config);
	const busName = projectBusName(config);
	const following = followingEntries(config);
	const explicitRefs = explicitProjectBusRefs(config);
	const offBlocked = enabled && explicitRefs.length > 0;
	const available = enhancedBusAvailableIn(config.region);
	const taskConflicts = enhanced ? eventTaskRulesOnProjectBus(config) : [];
	const share = config.event_bus?.share ?? [];

	// The shares this panel opened with, to tell which accounts lose their
	// subscribers when a row is removed or downgraded.
	const [initialShare] = useState<EventBusShare[]>(
		() => config.event_bus?.share ?? [],
	);
	const [targets, setTargets] = useState<ShareTargetInfo[] | undefined>();
	const [targetsError, setTargetsError] = useState<string | null>(null);
	const [pendingLeave, setPendingLeave] = useState<"standard" | "off" | null>(
		null,
	);

	useEffect(() => {
		if (!enhanced) return;
		const controller = new AbortController();
		setTargetsError(null);
		loadShareTargets(config.env, controller.signal)
			.then((list) => setTargets(list))
			.catch((err: unknown) => {
				if (controller.signal.aborted) return;
				setTargetsError(err instanceof Error ? err.message : String(err));
			});
		return () => controller.abort();
	}, [enhanced, config.env, loadShareTargets]);

	const leaving =
		pendingLeave !== null ? clearOrdering(config, loadedConfig) : null;
	const sharedForSubscribe = share.some((e) => isSubscribeCapable(e.access));
	// Subscribe-capable entries removed since the panel opened; a downgraded
	// entry shows its notice on its own row.
	const removedSubscribers = shareSubscribersLost(
		initialShare,
		share,
		targets,
	).filter(
		(old) =>
			!share.some(
				(e) =>
					sharePrincipalKey(e, targets) === sharePrincipalKey(old, targets),
			),
	);

	const leaveEnhanced = (target: "standard" | "off") => {
		const { updates } = clearOrdering(config, loadedConfig);
		onConfigChange({
			...updates,
			event_bus: {
				enabled: target === "standard",
				type: undefined,
				share: undefined,
			},
		});
	};

	const writeShare = (next: EventBusShare[]) =>
		onConfigChange({
			event_bus: { share: next.length > 0 ? next : undefined },
		});

	return (
		<div className="space-y-4">
			<Card>
				<CardHeader>
					<CardTitle className="flex items-center gap-2">
						<Workflow className="w-5 h-5" />
						Project bus
					</CardTitle>
					<CardDescription>
						A bus of this environment's own, {busName}. When it is on, every
						container's EVENT_BUS_NAME names it, and entries and event-task
						rules without a bus use it.
					</CardDescription>
				</CardHeader>
				<CardContent className="space-y-4">
					<div className="flex items-center justify-between gap-4">
						<Label htmlFor={`${uid}-bus`}>Use the project bus</Label>
						<Switch
							id={`${uid}-bus`}
							checked={enabled}
							disabled={offBlocked}
							onCheckedChange={(checked) => {
								if (!checked && enhanced) {
									setPendingLeave("off");
									return;
								}
								onConfigChange({ event_bus: { enabled: checked } });
							}}
						/>
					</div>

					{offBlocked && (
						<div className="space-y-2" data-testid="bus-off-blocked">
							<StatusNote tone="warning">
								These entries name the project bus, so it cannot be turned off:{" "}
								{explicitRefs.map((r) => r.label).join(", ")}.
							</StatusNote>
							<Button
								type="button"
								variant="outline"
								size="sm"
								onClick={() => onConfigChange(clearExplicitProjectBus(config))}
							>
								Let them follow the switch
							</Button>
							<NoteDetail>
								They then use the project bus while it is on, and the default
								bus once it is off.
							</NoteDetail>
						</div>
					)}

					{enabled && (
						<div className="space-y-2">
							<Label>Kind of bus</Label>
							<RadioGroup
								value={enhanced ? "enhanced" : "classic"}
								aria-label="Kind of bus"
								onValueChange={(value) => {
									if (value === "enhanced" && !enhanced) {
										onConfigChange({ event_bus: { type: "enhanced" } });
									} else if (value === "classic" && enhanced) {
										setPendingLeave("standard");
									}
								}}
							>
								<div className="flex items-start gap-2">
									<RadioGroupItem id={`${uid}-kind-classic`} value="classic" />
									<div className="space-y-1">
										<Label htmlFor={`${uid}-kind-classic`}>Standard</Label>
										<NoteDetail>
											Works in every AWS region. Events can arrive in any order.
										</NoteDetail>
									</div>
								</div>
								<div className="flex items-start gap-2">
									<RadioGroupItem
										id={`${uid}-kind-enhanced`}
										value="enhanced"
										disabled={!available && !enhanced}
									/>
									<div className="space-y-1">
										<Label htmlFor={`${uid}-kind-enhanced`}>Enhanced</Label>
										<NoteDetail>
											Can deliver events in order and be shared with other AWS
											accounts in your organization. Priced by data volume
											instead of per event.
										</NoteDetail>
										{!available && !enhanced && (
											<NoteDetail>
												Not available in {config.region || "this region"} yet.
											</NoteDetail>
										)}
									</div>
								</div>
							</RadioGroup>
							{enhanced && !available && (
								<StatusNote tone="error">
									The Enhanced bus is not available in{" "}
									{config.region || "this region"}; it is offered in{" "}
									{ENHANCED_EVENT_BUS_REGIONS.join(", ")}. Choose Standard, or
									deploy this environment to one of those regions.
								</StatusNote>
							)}
							{enhanced && (
								<NoteDetail>
									Apps publish to it with the EventBridge V2 client and
									EVENT_BUS_ARN.
								</NoteDetail>
							)}
							{taskConflicts.length > 0 && (
								<StatusNote tone="error">
									An Enhanced bus cannot start event tasks. Put these rules on
									the default bus in each event task:{" "}
									{taskConflicts.map((r) => r.label).join(", ")}.
								</StatusNote>
							)}
						</div>
					)}

					<Alert>
						<AlertDescription>
							<span>
								Switching moves every entry without a bus field to the{" "}
								{enabled ? "default" : "project"} bus; event-task rules move by
								replacement. Events from AWS services stay on the default bus.
								{following.length > 0
									? ` Following the switch: ${following.join(", ")}.`
									: " No entry follows the switch."}
							</span>
						</AlertDescription>
					</Alert>
				</CardContent>
			</Card>

			{enhanced && (
				<Card data-testid="share-card">
					<CardHeader>
						<CardTitle className="flex items-center gap-2">
							<Share2 className="w-5 h-5" />
							Share with other accounts
						</CardTitle>
						<CardDescription>
							Let other AWS accounts send events to this bus or subscribe to it.
							An account outside your organization gets an invitation it has to
							accept.
						</CardDescription>
					</CardHeader>
					<CardContent className="space-y-3">
						{share.length === 0 && (
							<NoteDetail>Not shared with any other account.</NoteDetail>
						)}
						{share.map((entry, index) => (
							<ShareRow
								// biome-ignore lint/suspicious/noArrayIndexKey: share entries have no other identity
								key={index}
								entry={entry}
								index={index}
								share={share}
								initialShare={initialShare}
								config={config}
								targets={targets}
								targetsError={targetsError}
								onChange={(i, next) =>
									writeShare(share.map((e, j) => (j === i ? next : e)))
								}
								onRemove={(i) => writeShare(share.filter((_, j) => j !== i))}
							/>
						))}
						{removedSubscribers.length > 0 && (
							<StatusNote tone="warning">
								<p>
									No longer shared for subscribing:{" "}
									{removedSubscribers.map(shareEntryLabel).join(", ")}.
								</p>
								<NoteDetail>{REVOKE_NOTICE}</NoteDetail>
							</StatusNote>
						)}
						<Separator />
						<ShareAddForm
							share={share}
							config={config}
							targets={targets}
							targetsError={targetsError}
							onAdd={(entry) => writeShare([...share, entry])}
						/>
					</CardContent>
				</Card>
			)}

			<AlertDialog
				open={pendingLeave !== null}
				onOpenChange={(open) => {
					if (!open) setPendingLeave(null);
				}}
			>
				<AlertDialogContent>
					<AlertDialogHeader>
						<AlertDialogTitle>
							{pendingLeave === "off"
								? "Turn the Enhanced bus off?"
								: "Change to a Standard bus?"}
						</AlertDialogTitle>
						<AlertDialogDescription>
							Changing the kind replaces the bus. Every subscription on it is
							recreated, and events sent during the deploy are lost. Apps must
							publish with EVENT_BUS_ARN (Enhanced) or EVENT_BUS_NAME
							(Standard).
						</AlertDialogDescription>
					</AlertDialogHeader>
					{leaving !== null && leaving.affected.length > 0 && (
						<StatusNote tone="warning">
							These services lose ordered delivery:{" "}
							{leaving.affected.join(", ")}.
							{leaving.kept.length > 0 &&
								(leaving.kept.length === leaving.affected.length
									? " Their ordered queues are kept until you delete them in each service's Events tab."
									: ` The ordered queues of ${leaving.kept.join(", ")} are kept until you delete them in each service's Events tab.`)}
						</StatusNote>
					)}
					{sharedForSubscribe && (
						<StatusNote tone="warning">
							Accounts you shared the bus with lose their subscribers for good.
						</StatusNote>
					)}
					<AlertDialogFooter>
						<AlertDialogCancel>Cancel</AlertDialogCancel>
						<AlertDialogAction
							variant="destructive"
							onClick={() => {
								const target = pendingLeave;
								setPendingLeave(null);
								if (target !== null) leaveEnhanced(target);
							}}
						>
							{pendingLeave === "off" ? "Turn it off" : "Use a Standard bus"}
						</AlertDialogAction>
					</AlertDialogFooter>
				</AlertDialogContent>
			</AlertDialog>
		</div>
	);
}
