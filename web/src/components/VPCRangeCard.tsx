import {
	ChevronDown,
	Cloud,
	Loader2,
	Network,
	RefreshCw,
	Sparkles,
} from "lucide-react";
import { type ReactNode, useEffect, useId, useRef, useState } from "react";
import {
	infrastructureApi,
	type VPCCIDRCandidateResult,
	type VPCCIDRIssue,
	type VPCCIDRReport,
	type VPCCIDRWarning,
} from "../api/infrastructure";
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
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "./ui/select";
import { NoteCode, NoteDetail, StatusNote } from "./ui/status-note";

/**
 * "Network range" — the env's private address range (YAML `vpc_cidr`).
 *
 * Written for people who are wary of IP addresses: the surface never says
 * "CIDR", "mask" or "prefix", and a range is only ever shown as a small
 * monospace value, never as a headline. Every rule comes from the Go
 * validator behind GET /api/vpc-cidr; this card renders its `hint` and keeps
 * the technical `message` in a tooltip.
 *
 * Picking a range is STAGED: the dropdown, the manual field and the "Use this"
 * fix only change local state. The single "Apply this range" button is the
 * one call to `onApply`, because the caller saves immediately and a deployed
 * environment must pass the "I understand" guard first.
 */
export interface VPCRangeCardProps {
	env: string;
	/** The YAML `vpc_cidr` as currently saved; "" when the key is absent. */
	value: string;
	onApply: (vpcCidr: string) => void;
}

const SUGGESTION_COUNT = 5;

function Range({ value }: { value: string }) {
	return <NoteCode>{value}</NoteCode>;
}

const Detail = NoteDetail;

function WarningList({ warnings }: { warnings: VPCCIDRWarning[] }) {
	return (
		<>
			{warnings.map((w) => (
				<StatusNote
					key={`${w.source}|${w.message}`}
					tone="warning"
					title={w.message}
				>
					<p>{w.hint}</p>
				</StatusNote>
			))}
		</>
	);
}

function OverrideHint() {
	return (
		<Detail>
			If this environment is already running, you can keep this range instead of
			rebuilding: add{" "}
			<code className="font-mono">allow_vpc_cidr_override: true</code> to its
			settings.
		</Detail>
	);
}

function IssueNote({
	issue,
	onUseFix,
}: {
	issue: VPCCIDRIssue;
	onUseFix: (fix: string) => void;
}) {
	return (
		<StatusNote tone="error" title={issue.message}>
			<p>{issue.hint}</p>
			{issue.fix && (
				<Button
					type="button"
					variant="outline"
					size="sm"
					onClick={() => onUseFix(issue.fix as string)}
				>
					Use this: <Range value={issue.fix} />
				</Button>
			)}
		</StatusNote>
	);
}

function isAbort(err: unknown): boolean {
	return err instanceof DOMException && err.name === "AbortError";
}

function errorText(err: unknown): string {
	return err instanceof Error && err.message
		? err.message
		: "Something went wrong while checking the network range.";
}

type Choice = { value: string; source: "suggestion" | "manual" };

export function VPCRangeCard({ env, value, onApply }: VPCRangeCardProps) {
	const uid = useId();

	// Mount report: the ONE aws=1 request (status, deployed, AWS overlaps).
	const [report, setReport] = useState<VPCCIDRReport | null>(null);
	const [loading, setLoading] = useState(true);
	const [loadError, setLoadError] = useState<string | null>(null);
	const [reloadKey, setReloadKey] = useState(0);

	// Check of the saved value when it no longer matches the report (after Apply).
	const [currentCheck, setCurrentCheck] =
		useState<VPCCIDRCandidateResult | null>(null);
	const [currentCheckFailed, setCurrentCheckFailed] = useState(false);

	// Suggestions ("Pick a safe range for me" / "Check my AWS accounts too").
	const [suggestions, setSuggestions] = useState<string[] | null>(null);
	const [picking, setPicking] = useState<"local" | "aws" | null>(null);
	const [pickError, setPickError] = useState<string | null>(null);
	const [pickNote, setPickNote] = useState<string | null>(null);
	const [pickAwsChecked, setPickAwsChecked] = useState(false);
	const pickCtrl = useRef<AbortController | null>(null);

	// Manual entry, validated on every keystroke.
	const [manualOpen, setManualOpen] = useState(false);
	const [manual, setManual] = useState("");
	const [candidate, setCandidate] = useState<VPCCIDRCandidateResult | null>(
		null,
	);
	const [candidatePending, setCandidatePending] = useState(false);
	const [candidateError, setCandidateError] = useState<string | null>(null);
	const candidateCtrl = useRef<AbortController | null>(null);

	// Staged selection — nothing is saved until "Apply this range".
	const [choice, setChoice] = useState<Choice | null>(null);
	const [understood, setUnderstood] = useState(false);

	// biome-ignore lint/correctness/useExhaustiveDependencies: reloadKey re-runs the mount request on "Try again".
	useEffect(() => {
		const ctrl = new AbortController();
		setReport(null);
		setLoading(true);
		setLoadError(null);
		setSuggestions(null);
		setPickError(null);
		setPickNote(null);
		setChoice(null);
		setManual("");
		setCandidate(null);
		setUnderstood(false);
		infrastructureApi
			.getVPCCIDRReport(env, {
				aws: true,
				count: SUGGESTION_COUNT,
				signal: ctrl.signal,
			})
			.then((r) => setReport(r))
			.catch((err) => {
				if (!isAbort(err)) setLoadError(errorText(err));
			})
			.finally(() => {
				if (!ctrl.signal.aborted) setLoading(false);
			});
		return () => ctrl.abort();
	}, [env, reloadKey]);

	useEffect(
		() => () => {
			pickCtrl.current?.abort();
			candidateCtrl.current?.abort();
		},
		[],
	);

	const reportConfigured = report ? report.configured : null;
	const reportMatches = reportConfigured !== null && value === reportConfigured;

	useEffect(() => {
		setCurrentCheck(null);
		setCurrentCheckFailed(false);
		if (reportConfigured === null || value === reportConfigured || value === "")
			return;
		const ctrl = new AbortController();
		infrastructureApi
			.getVPCCIDRReport(env, {
				candidate: value,
				count: 1,
				signal: ctrl.signal,
			})
			.then((r) => setCurrentCheck(r.candidate ?? null))
			.catch((err) => {
				if (!isAbort(err)) setCurrentCheckFailed(true);
			});
		return () => ctrl.abort();
	}, [env, value, reportConfigured]);

	async function pick(withAws: boolean) {
		pickCtrl.current?.abort();
		const ctrl = new AbortController();
		pickCtrl.current = ctrl;
		setPicking(withAws ? "aws" : "local");
		setPickError(null);
		setPickNote(null);
		try {
			const r = await infrastructureApi.getVPCCIDRReport(env, {
				aws: withAws,
				count: SUGGESTION_COUNT,
				signal: ctrl.signal,
			});
			setSuggestions(r.suggestions);
			setPickAwsChecked(r.awsChecked);
			if (withAws && (!r.awsChecked || r.awsNote)) {
				setPickNote(
					r.awsNote ??
						"Couldn't reach AWS — these avoid your other meroku environments only.",
				);
			}
			if (withAws) {
				setReport((prev) =>
					prev
						? {
								...prev,
								deployed: r.deployed,
								deployedReason: r.deployedReason,
							}
						: prev,
				);
			}
			const first = r.suggestions[0];
			setChoice(first ? { value: first, source: "suggestion" } : null);
		} catch (err) {
			if (!isAbort(err)) setPickError(errorText(err));
		} finally {
			if (pickCtrl.current === ctrl) setPicking(null);
		}
	}

	function checkManual(raw: string) {
		setManual(raw);
		const typed = raw.trim();
		candidateCtrl.current?.abort();
		setCandidateError(null);
		if (typed === "") {
			candidateCtrl.current = null;
			setCandidate(null);
			setCandidatePending(false);
			setChoice((prev) => (prev?.source === "manual" ? null : prev));
			return;
		}
		setChoice({ value: typed, source: "manual" });
		const ctrl = new AbortController();
		candidateCtrl.current = ctrl;
		setCandidatePending(true);
		infrastructureApi
			.getVPCCIDRReport(env, {
				candidate: typed,
				count: 1,
				signal: ctrl.signal,
			})
			.then((r) => {
				if (candidateCtrl.current !== ctrl) return;
				setCandidate(r.candidate ?? null);
				setCandidatePending(false);
			})
			.catch((err) => {
				if (isAbort(err) || candidateCtrl.current !== ctrl) return;
				setCandidate(null);
				setCandidatePending(false);
				setCandidateError(errorText(err));
			});
	}

	if (loading) {
		return (
			<VPCRangeShell>
				<StatusNote tone="pending">
					<p>Checking your network range…</p>
				</StatusNote>
			</VPCRangeShell>
		);
	}

	if (loadError || !report) {
		return (
			<VPCRangeShell>
				<StatusNote tone="error">
					<p>We couldn't check the network range.</p>
					<Detail>{loadError ?? "No answer from meroku."}</Detail>
				</StatusNote>
				<Button
					type="button"
					variant="outline"
					size="sm"
					onClick={() => setReloadKey((k) => k + 1)}
				>
					<RefreshCw aria-hidden="true" />
					Try again
				</Button>
			</VPCRangeShell>
		);
	}

	const deployed = report.deployed;
	const currentRange = value || report.effective;
	const currentError = reportMatches ? report.error : currentCheck?.error;
	const currentWarnings = reportMatches
		? report.warnings
		: (currentCheck?.warnings ?? []);
	const currentPending =
		!reportMatches && value !== "" && !currentCheck && !currentCheckFailed;
	const currentUnchecked = !reportMatches && currentCheckFailed;
	const awsCovered = reportMatches && report.awsChecked;

	const manualFresh =
		candidate !== null &&
		candidate.value === manual.trim() &&
		!candidatePending;
	const choiceValid =
		choice !== null &&
		(choice.source === "suggestion" ||
			(manualFresh &&
				candidate.valid &&
				!candidate.incomplete &&
				candidate.value === choice.value));
	const needsGuard = deployed !== "no";
	const canApply =
		choice !== null &&
		choiceValid &&
		choice.value !== currentRange &&
		(!needsGuard || understood);

	function apply() {
		if (!canApply || !choice) return;
		onApply(choice.value);
		setChoice(null);
		setUnderstood(false);
		setManual("");
		setCandidate(null);
	}

	const manualInvalid = manualFresh && !!candidate.error;
	const manualHintId = `${uid}-manual-hint`;

	return (
		<VPCRangeShell>
			{/* Current range and its status */}
			<div className="space-y-2">
				{currentPending ? (
					<StatusNote tone="pending">
						<p>Checking the range you applied…</p>
					</StatusNote>
				) : currentUnchecked ? (
					<StatusNote tone="neutral">
						<p>Saved. We couldn't re-check this range just now.</p>
					</StatusNote>
				) : report.override ? (
					<StatusNote tone="neutral">
						<p>Custom range — checks turned off for this environment</p>
					</StatusNote>
				) : currentError ? (
					<StatusNote tone="error" title={currentError.message}>
						<p>This range can't be used</p>
						<Detail>{currentError.hint}</Detail>
						{deployed !== "no" && <OverrideHint />}
					</StatusNote>
				) : currentWarnings.length === 0 ? (
					<StatusNote tone="success">
						<p>
							{awsCovered
								? "Ready — doesn't clash with your other environments or your AWS accounts"
								: "Ready — doesn't clash with your other environments"}
						</p>
					</StatusNote>
				) : null}
				{!currentPending && <WarningList warnings={currentWarnings} />}
				<Detail>
					Current range: <Range value={currentRange} />
				</Detail>
				{reportMatches && report.awsNote && (
					<StatusNote tone="neutral">
						<Detail>{report.awsNote}</Detail>
					</StatusNote>
				)}
			</div>

			{/* Pick for me */}
			<div className="space-y-2">
				<div className="flex flex-wrap gap-2">
					<Button
						type="button"
						size="sm"
						disabled={picking !== null}
						onClick={() => pick(false)}
					>
						{picking === "local" ? (
							<Loader2
								className="animate-spin motion-reduce:animate-none"
								aria-hidden="true"
							/>
						) : (
							<Sparkles aria-hidden="true" />
						)}
						Pick a safe range for me
					</Button>
					<Button
						type="button"
						size="sm"
						variant="outline"
						disabled={picking !== null}
						onClick={() => pick(true)}
					>
						{picking === "aws" ? (
							<Loader2
								className="animate-spin motion-reduce:animate-none"
								aria-hidden="true"
							/>
						) : (
							<Cloud aria-hidden="true" />
						)}
						Check my AWS accounts too
					</Button>
				</div>
				<Detail>
					Also avoids networks that already exist in the AWS accounts your
					environments use.
				</Detail>
				{pickError && (
					<StatusNote tone="error">
						<p>We couldn't pick a range right now.</p>
						<Detail>{pickError}</Detail>
					</StatusNote>
				)}
				{pickNote && (
					<StatusNote tone="neutral">
						<Detail>{pickNote}</Detail>
					</StatusNote>
				)}
				{suggestions !== null && suggestions.length === 0 && (
					<StatusNote tone="warning">
						<p>
							No free ranges are left to suggest. Enter one yourself under
							Advanced.
						</p>
					</StatusNote>
				)}
				{suggestions !== null && suggestions.length > 0 && (
					<div className="space-y-1">
						<Label htmlFor={`${uid}-suggestion`}>Safe ranges</Label>
						<Select
							value={choice?.source === "suggestion" ? choice.value : ""}
							onValueChange={(v) =>
								setChoice({ value: v, source: "suggestion" })
							}
						>
							<SelectTrigger id={`${uid}-suggestion`}>
								<SelectValue placeholder="Choose one of the safe ranges" />
							</SelectTrigger>
							<SelectContent>
								{suggestions.map((s, i) => (
									<SelectItem key={s} value={s}>
										Option {i + 1} — <Range value={s} /> · free
									</SelectItem>
								))}
							</SelectContent>
						</Select>
						<Detail>
							{pickAwsChecked
								? "All of these are free in your other environments and your AWS accounts."
								: "All of these are free in your other environments."}
						</Detail>
					</div>
				)}
			</div>

			{/* Advanced: manual entry */}
			<Collapsible open={manualOpen} onOpenChange={setManualOpen}>
				<CollapsibleTrigger asChild>
					<Button
						type="button"
						variant="ghost"
						size="sm"
						aria-expanded={manualOpen}
					>
						<ChevronDown aria-hidden="true" />
						Advanced: enter a range yourself
					</Button>
				</CollapsibleTrigger>
				<CollapsibleContent>
					<div className="space-y-2 pt-2">
						<Label htmlFor={`${uid}-manual`}>Your range</Label>
						<Input
							id={`${uid}-manual`}
							value={manual}
							placeholder="10.42.0.0/16"
							autoComplete="off"
							spellCheck={false}
							aria-invalid={manualInvalid}
							aria-describedby={manualHintId}
							onChange={(e) => checkManual(e.target.value)}
						/>
						<div id={manualHintId} aria-live="polite" className="space-y-2">
							{manual.trim() === "" ? null : candidateError ? (
								<StatusNote tone="neutral">
									<Detail>
										We couldn't check this range right now: {candidateError}
									</Detail>
								</StatusNote>
							) : !manualFresh ? (
								<StatusNote tone="pending">
									<Detail>Checking…</Detail>
								</StatusNote>
							) : candidate.incomplete ? (
								<StatusNote tone="neutral">
									<Detail>
										Keep typing — for example <Range value="10.42.0.0/16" />
									</Detail>
								</StatusNote>
							) : candidate.error ? (
								<IssueNote
									issue={candidate.error}
									onUseFix={(fix) => checkManual(fix)}
								/>
							) : (
								<>
									{candidate.warnings.length === 0 && (
										<StatusNote tone="success">
											<p>This range works</p>
										</StatusNote>
									)}
									<WarningList warnings={candidate.warnings} />
								</>
							)}
						</div>
					</div>
				</CollapsibleContent>
			</Collapsible>

			{/* Apply */}
			<div className="space-y-3">
				{choice && choiceValid && (
					<Detail>
						Your choice: <Range value={choice.value} />
						{choice.value === currentRange && " (already in use)"}
					</Detail>
				)}
				{needsGuard && choice && choiceValid && (
					<StatusNote tone="warning" title={report.deployedReason}>
						<p>
							{deployed === "yes"
								? "This environment is already running."
								: "This environment may already be running — we couldn't confirm it."}{" "}
							Changing its network range rebuilds everything in it (servers,
							database connection, load balancer). Only do this if you know you
							need to.
						</p>
						<div className="flex items-center gap-2">
							<Checkbox
								id={`${uid}-understood`}
								checked={understood}
								onCheckedChange={(c) => setUnderstood(c === true)}
							/>
							<Label htmlFor={`${uid}-understood`}>I understand</Label>
						</div>
					</StatusNote>
				)}
				<Button type="button" disabled={!canApply} onClick={apply}>
					Apply this range
				</Button>
			</div>
		</VPCRangeShell>
	);
}

function VPCRangeShell({ children }: { children: ReactNode }) {
	return (
		<Card>
			<CardHeader>
				<CardTitle className="flex items-center gap-2">
					<Network className="size-4" aria-hidden="true" />
					Network range
				</CardTitle>
				<CardDescription>
					Each environment needs its own private address range so you can
					connect environments to each other later. We pick a safe one for you.
				</CardDescription>
			</CardHeader>
			<CardContent className="space-y-5">{children}</CardContent>
		</Card>
	);
}
