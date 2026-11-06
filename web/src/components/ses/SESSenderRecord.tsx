import { AlertCircle, Info } from "lucide-react";
import { useEffect, useId, useState } from "react";
import { infrastructureApi, type SESDNSReport } from "../../api/infrastructure";
import {
	issuesFor,
	SES_DEFAULT_SPF_RECORD,
	type SESDNSIssue,
	type SESMailFromDefaults,
	validateSESDomainDNS,
	validateSPFRecord,
} from "../../lib/sesDnsValidation";
import type { SESDomain } from "../../types/yamlConfig";
import { Badge } from "../ui/badge";
import { Input } from "../ui/input";
import { Label } from "../ui/label";
import { RadioGroup, RadioGroupItem } from "../ui/radio-group";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "../ui/select";

export const SPF_GOOGLE =
	"v=spf1 include:_spf.google.com include:amazonses.com ~all";
export const SPF_MICROSOFT =
	"v=spf1 include:spf.protection.outlook.com include:amazonses.com ~all";

export interface SESSenderRecordProps {
	env: string;
	domain: SESDomain;
	sesDefaults: SESMailFromDefaults;
	/** Called only with values the Go validator accepts. */
	onUpdate: (updates: Partial<SESDomain>) => void;
}

/** One line per issue: what is wrong, then what to do. */
export function IssueList({ issues }: { issues: SESDNSIssue[] }) {
	if (issues.length === 0) return null;
	return (
		<ul className="space-y-1">
			{issues.map((i) => (
				<li
					key={`${i.field}-${i.code}`}
					className="text-xs text-red-400 flex gap-1"
				>
					<AlertCircle className="w-3 h-3 mt-0.5 shrink-0" />
					<span>
						{i.message} {i.hint}
					</span>
				</li>
			))}
		</ul>
	);
}

/** What meroku does with records it did not write, by zone origin. */
export function ZoneOwnershipNote({
	origin,
}: {
	origin: SESDNSReport["zone_origin"];
}) {
	if (origin === "created") {
		return (
			<p className="text-xs text-blue-400">
				<Info className="w-3 h-3 inline mr-1" />
				meroku created this domain's DNS zone, so the records below are the
				complete list. Anything else at these names is removed when you deploy.
			</p>
		);
	}
	if (origin === "existing") {
		return (
			<p className="text-xs text-blue-400">
				<Info className="w-3 h-3 inline mr-1" />
				This DNS zone was set up outside meroku. meroku adds the records below
				and leaves everything else alone. If you remove a record here, or remove
				the domain, meroku deletes only what it added.
			</p>
		);
	}
	return (
		<p className="text-xs text-yellow-400">
			<AlertCircle className="w-3 h-3 inline mr-1" />
			This domain has no Route53 zone in this environment. Add the records below
			at your DNS provider by hand.
		</p>
	);
}

type SenderChoice = "only" | "existing" | "google" | "microsoft" | "custom";

function choiceFor(spf: string | undefined, suggested: string): SenderChoice {
	if (!spf || spf === SES_DEFAULT_SPF_RECORD) return "only";
	if (spf === SPF_GOOGLE) return "google";
	if (spf === SPF_MICROSOFT) return "microsoft";
	if (spf === suggested) return "existing";
	return "custom";
}

const TTL_PRESETS = ["300", "600", "3600"];
const TTL_LABELS: Record<string, string> = {
	"300": "5 minutes",
	"600": "10 minutes",
	"3600": "1 hour",
};

const describeTTL = (s: number) =>
	TTL_LABELS[String(s)] ??
	(s % 3600 === 0 ? `${s / 3600} hours` : `${s} seconds`);

/**
 * "Who sends email from this domain?": the domain's one sender (SPF)
 * record, and how long other servers may remember the domain's TXT records.
 */
export function SESSenderRecord({
	env,
	domain,
	sesDefaults,
	onUpdate,
}: SESSenderRecordProps) {
	const groupId = useId();
	const customId = useId();
	const ttlId = useId();

	const [report, setReport] = useState<SESDNSReport | null>(null);
	const [loadError, setLoadError] = useState<string | null>(null);

	useEffect(() => {
		if (!env || !domain.domain) return;
		const ctrl = new AbortController();
		setLoadError(null);
		infrastructureApi
			.getSESDNS(env, domain.domain, { signal: ctrl.signal })
			.then(setReport)
			.catch((e: unknown) => {
				if (ctrl.signal.aborted) return;
				setLoadError(e instanceof Error ? e.message : String(e));
			});
		return () => ctrl.abort();
	}, [env, domain.domain]);

	const live = report?.live;
	const liveSPF =
		live && !live.read_error && live.spf.length > 0 ? live.spf : [];
	const suggested = report?.suggested_spf ?? SES_DEFAULT_SPF_RECORD;
	const liveIsForeign =
		liveSPF.length > 0 && suggested !== SES_DEFAULT_SPF_RECORD;

	const [choice, setChoice] = useState<SenderChoice>(() =>
		choiceFor(domain.spf_record, suggested),
	);
	// The suggested value is known only once the report is in.
	// biome-ignore lint/correctness/useExhaustiveDependencies: re-derive only when a new report arrives, never on an edit.
	useEffect(() => {
		if (report) setChoice(choiceFor(domain.spf_record, report.suggested_spf));
	}, [report]);

	const [customDraft, setCustomDraft] = useState(domain.spf_record ?? "");
	const customIssues =
		choice === "custom" ? validateSPFRecord(customDraft.trim() || "") : [];
	const customEmpty = choice === "custom" && customDraft.trim() === "";

	const pick = (next: SenderChoice) => {
		setChoice(next);
		switch (next) {
			case "only":
				onUpdate({ spf_record: undefined });
				break;
			case "existing":
				onUpdate({ spf_record: suggested });
				break;
			case "google":
				onUpdate({ spf_record: SPF_GOOGLE });
				break;
			case "microsoft":
				onUpdate({ spf_record: SPF_MICROSOFT });
				break;
			case "custom":
				if (
					customDraft.trim() !== "" &&
					validateSPFRecord(customDraft.trim()).length === 0
				) {
					onUpdate({ spf_record: customDraft.trim() });
				}
				break;
		}
	};

	const typeCustom = (text: string) => {
		setCustomDraft(text);
		const value = text.trim();
		if (value !== "" && validateSPFRecord(value).length === 0) {
			onUpdate({ spf_record: value });
		}
	};

	const effective = domain.spf_record || SES_DEFAULT_SPF_RECORD;
	const replacesLive =
		liveSPF.length > 0 && liveSPF.some((s) => s !== effective);

	// TTL: unset keeps what an existing zone uses now, else 600.
	const keepsLiveTTL =
		report?.zone_origin === "existing" &&
		live?.apex_ttl !== null &&
		live?.apex_ttl !== undefined &&
		!live.read_error;
	const ttlValue =
		domain.spf_ttl === undefined
			? "default"
			: TTL_PRESETS.includes(String(domain.spf_ttl))
				? String(domain.spf_ttl)
				: "custom";
	const [ttlMode, setTtlMode] = useState(ttlValue);
	const [ttlDraft, setTtlDraft] = useState(
		domain.spf_ttl === undefined ? "" : String(domain.spf_ttl),
	);
	const ttlIssues =
		ttlMode === "custom" && ttlDraft.trim() !== ""
			? issuesFor(
					validateSESDomainDNS(sesDefaults, {
						domain: domain.domain,
						spf_ttl: Number(ttlDraft),
					}),
					"spf_ttl",
				)
			: [];

	const pickTTL = (mode: string) => {
		setTtlMode(mode);
		if (mode === "default") onUpdate({ spf_ttl: undefined });
		else if (mode !== "custom") onUpdate({ spf_ttl: Number(mode) });
	};

	const typeTTL = (text: string) => {
		setTtlDraft(text);
		const n = Number(text);
		if (text.trim() !== "" && Number.isInteger(n) && n >= 0) {
			onUpdate({ spf_ttl: n });
		}
	};

	const options: { value: SenderChoice; label: string; badge?: string }[] = [
		{ value: "only", label: "Only this app" },
		...(liveIsForeign
			? [
					{
						value: "existing" as const,
						label: "This app and the services your domain already uses",
						badge: "Recommended",
					},
				]
			: []),
		{ value: "google", label: "This app and Google Workspace" },
		{ value: "microsoft", label: "This app and Microsoft 365" },
		{ value: "custom", label: "Write the record myself (advanced)" },
	];

	return (
		<div className="space-y-3 rounded-md border border-gray-700 p-3">
			<div className="space-y-1">
				<h4 className="text-sm font-medium">Email sender records</h4>
				{report ? (
					<ZoneOwnershipNote origin={report.zone_origin} />
				) : loadError ? (
					<p className="text-xs text-gray-500">
						Could not check this domain: {loadError}
					</p>
				) : (
					<p className="text-xs text-gray-500">Checking the domain…</p>
				)}
			</div>

			<div className="space-y-2">
				<Label id={groupId} className="text-sm">
					Who sends email from this domain?
				</Label>
				<RadioGroup
					aria-labelledby={groupId}
					value={choice}
					onValueChange={(v) => pick(v as SenderChoice)}
					className="space-y-1"
				>
					{options.map((o) => (
						<div key={o.value} className="flex items-center gap-2">
							<RadioGroupItem value={o.value} id={`${groupId}-${o.value}`} />
							<Label
								htmlFor={`${groupId}-${o.value}`}
								className="text-sm font-normal"
							>
								{o.label}
							</Label>
							{o.badge && (
								<Badge variant="outline" className="text-xs">
									{o.badge}
								</Badge>
							)}
						</div>
					))}
				</RadioGroup>

				{choice === "existing" && (
					<code className="block text-xs text-gray-400 break-all">
						{suggested}
					</code>
				)}
				{choice === "custom" && (
					<div className="space-y-1">
						<Label htmlFor={customId} className="text-xs">
							Sender record
						</Label>
						<Input
							id={customId}
							value={customDraft}
							onChange={(e) => typeCustom(e.target.value)}
							placeholder={SES_DEFAULT_SPF_RECORD}
						/>
						{customEmpty && (
							<p className="text-xs text-gray-500">
								Type the whole record, starting with v=spf1.
							</p>
						)}
						<IssueList issues={customEmpty ? [] : customIssues} />
					</div>
				)}

				{live?.read_error && (
					<p className="text-xs text-gray-500">
						<Info className="w-3 h-3 inline mr-1" />
						Could not read what the domain publishes now: {live.read_error}
					</p>
				)}
				{liveSPF.length > 0 && (
					<p className="text-xs text-gray-400">
						Your domain publishes now:{" "}
						{liveSPF.map((s) => (
							<code key={s} className="break-all">
								{s}
							</code>
						))}
					</p>
				)}
				{replacesLive && (
					<p className="text-xs text-yellow-400">
						<AlertCircle className="w-3 h-3 inline mr-1" />A domain can have
						only one sender record, so this replaces the one above.
					</p>
				)}
			</div>

			<div className="space-y-2">
				<Label htmlFor={ttlId} className="text-sm">
					How long other servers may remember it
				</Label>
				<Select value={ttlMode} onValueChange={pickTTL}>
					<SelectTrigger id={ttlId}>
						<SelectValue />
					</SelectTrigger>
					<SelectContent>
						<SelectItem value="default">
							{keepsLiveTTL && live?.apex_ttl != null
								? `Keep what the domain uses now (${describeTTL(live.apex_ttl)})`
								: "10 minutes (default)"}
						</SelectItem>
						{TTL_PRESETS.map((s) => (
							<SelectItem key={s} value={s}>
								{TTL_LABELS[s]}
							</SelectItem>
						))}
						<SelectItem value="custom">Other (seconds)</SelectItem>
					</SelectContent>
				</Select>
				{ttlMode === "custom" && (
					<Input
						inputMode="numeric"
						aria-label="Seconds"
						value={ttlDraft}
						onChange={(e) => typeTTL(e.target.value)}
						placeholder="600"
					/>
				)}
				<IssueList issues={ttlIssues} />
			</div>
		</div>
	);
}
