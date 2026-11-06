import { Info, Plus, Trash2 } from "lucide-react";
import { useState } from "react";
import {
	isSPFValue,
	issuesFor,
	type SESMailFromDefaults,
	validateSESDomainDNS,
} from "../../lib/sesDnsValidation";
import type { SESDomain, SESTXTRecord } from "../../types/yamlConfig";
import { Button } from "../ui/button";
import { Input } from "../ui/input";
import { Label } from "../ui/label";
import { Textarea } from "../ui/textarea";
import { IssueList } from "./SESSenderRecord";

export interface SESTextRecordsEditorProps {
	domain: SESDomain;
	sesDefaults: SESMailFromDefaults;
	/** Called only when every value in the section is valid. */
	onUpdate: (updates: Partial<SESDomain>) => void;
}

interface DraftValue {
	key: number;
	text: string;
}

interface DraftRecord {
	key: number;
	name: string;
	values: string;
	ttl: string;
	/** Added through the Google preset: ask for the key. */
	google?: boolean;
}

let nextKey = 0;

const linesOf = (text: string) =>
	text
		.split("\n")
		.map((l) => l.trim())
		.filter((l) => l !== "");

const toDraft = (r: SESTXTRecord): DraftRecord => ({
	key: nextKey++,
	name: r.name,
	values: r.values.join("\n"),
	ttl: r.ttl === undefined ? "" : String(r.ttl),
});

const fromDraft = (d: DraftRecord): SESTXTRecord => {
	const record: SESTXTRecord = {
		name: d.name.trim(),
		values: linesOf(d.values),
	};
	if (d.ttl.trim() !== "") record.ttl = Number(d.ttl);
	return record;
};

/** "Long value" note: DNS strings hold 255 characters; meroku splits. */
function LongValueNote({ text }: { text: string }) {
	if (text.length <= 255 || text.includes('" "')) return null;
	return (
		<p className="text-xs text-gray-500">
			<Info className="w-3 h-3 inline mr-1" />
			Long value, meroku will split it for DNS automatically.
		</p>
	);
}

/**
 * "Other text records at the domain" (apex_txt_values) and "Extra text
 * records" at names under it (txt_records). Drafts are local; the YAML gets
 * them only once they pass the checks `meroku generate` runs.
 */
export function SESTextRecordsEditor({
	domain,
	sesDefaults,
	onUpdate,
}: SESTextRecordsEditorProps) {
	const [apex, setApex] = useState<DraftValue[]>(() =>
		(domain.apex_txt_values ?? []).map((text) => ({ key: nextKey++, text })),
	);
	const [records, setRecords] = useState<DraftRecord[]>(() =>
		(domain.txt_records ?? []).map(toDraft),
	);

	const check = (apexValues: string[], txt: SESTXTRecord[]) =>
		validateSESDomainDNS(sesDefaults, {
			domain: domain.domain,
			enable_mail_from: domain.enable_mail_from,
			mail_from_subdomain: domain.mail_from_subdomain,
			apex_txt_values: apexValues,
			txt_records: txt,
		});

	const apexValues = apex.map((v) => v.text.trim());
	const issues = check(apexValues, records.map(fromDraft));

	const commitApex = (next: DraftValue[]) => {
		setApex(next);
		const values = next.map((v) => v.text.trim());
		if (issuesFor(check(values, []), "apex_txt_values").length === 0) {
			onUpdate({ apex_txt_values: values.length > 0 ? values : undefined });
		}
	};

	const commitRecords = (next: DraftRecord[]) => {
		setRecords(next);
		const txt = next.map(fromDraft);
		if (issuesFor(check([], txt), "txt_records").length === 0) {
			onUpdate({ txt_records: txt.length > 0 ? txt : undefined });
		}
	};

	const changeRecord = (key: number, patch: Partial<DraftRecord>) =>
		commitRecords(records.map((d) => (d.key === key ? { ...d, ...patch } : d)));

	const suffix = domain.domain ? `.${domain.domain}` : "";

	return (
		<div className="space-y-4 rounded-md border border-gray-700 p-3">
			<div className="space-y-2">
				<h4 className="text-sm font-medium">
					Other text records at the domain
				</h4>
				<p className="text-xs text-gray-500">
					Values such as google-site-verification=… that{" "}
					{domain.domain || "the domain"} itself must keep publishing.
				</p>
				{apex.map((v, i) => {
					const own =
						v.text.trim() === ""
							? []
							: issuesFor(issues, `apex_txt_values[${i}]`);
					const spf = isSPFValue(v.text);
					return (
						<div key={v.key} className="space-y-1">
							<div className="flex gap-2">
								<Input
									aria-label={`Text value ${i + 1}`}
									value={v.text}
									onChange={(e) =>
										commitApex(
											apex.map((x) =>
												x.key === v.key ? { ...x, text: e.target.value } : x,
											),
										)
									}
									placeholder="google-site-verification=…"
								/>
								<Button
									type="button"
									variant="ghost"
									size="sm"
									className="h-9 w-9 p-0"
									aria-label={`Remove text value ${i + 1}`}
									onClick={() =>
										commitApex(apex.filter((x) => x.key !== v.key))
									}
								>
									<Trash2 className="w-4 h-4 text-red-400" />
								</Button>
							</div>
							{spf ? (
								<p className="text-xs text-red-400">
									This is a sender record. Choose it under "Who sends email"
									instead.
								</p>
							) : (
								<IssueList issues={own} />
							)}
							<LongValueNote text={v.text} />
						</div>
					);
				})}
				<Button
					type="button"
					variant="outline"
					size="sm"
					onClick={() => setApex([...apex, { key: nextKey++, text: "" }])}
				>
					<Plus className="w-4 h-4 mr-1" />
					Add
				</Button>
			</div>

			<div className="space-y-2 border-t border-gray-800 pt-3">
				<h4 className="text-sm font-medium">Extra text records</h4>
				<p className="text-xs text-gray-500">
					TXT records at names under {domain.domain || "the domain"}.
				</p>

				{records.map((d, i) => {
					const blank =
						d.name.trim() === "" && d.values.trim() === "" && d.ttl === "";
					const own = blank ? [] : issuesFor(issues, `txt_records[${i}]`);
					return (
						<div
							key={d.key}
							className="space-y-2 rounded border border-gray-800 p-2"
						>
							<div className="flex items-end gap-2">
								<div className="flex-1 space-y-1">
									<Label className="text-xs" htmlFor={`txt-name-${d.key}`}>
										Name
									</Label>
									<div className="flex items-center gap-1">
										<Input
											id={`txt-name-${d.key}`}
											value={d.name}
											onChange={(e) =>
												changeRecord(d.key, { name: e.target.value })
											}
											placeholder="google._domainkey"
										/>
										{suffix && (
											<span className="text-xs text-gray-500 whitespace-nowrap">
												{suffix}
											</span>
										)}
									</div>
								</div>
								<div className="w-24 space-y-1">
									<Label className="text-xs" htmlFor={`txt-ttl-${d.key}`}>
										TTL (seconds)
									</Label>
									<Input
										id={`txt-ttl-${d.key}`}
										inputMode="numeric"
										value={d.ttl}
										onChange={(e) =>
											changeRecord(d.key, { ttl: e.target.value })
										}
										placeholder="600"
									/>
								</div>
								<Button
									type="button"
									variant="ghost"
									size="sm"
									className="h-9 w-9 p-0"
									aria-label={`Remove record ${d.name || i + 1}`}
									onClick={() =>
										commitRecords(records.filter((x) => x.key !== d.key))
									}
								>
									<Trash2 className="w-4 h-4 text-red-400" />
								</Button>
							</div>
							<div className="space-y-1">
								<Label className="text-xs" htmlFor={`txt-values-${d.key}`}>
									{d.google
										? "Paste the key from Google Admin (Apps > Gmail > Authenticate email)"
										: "Value(s), one per line"}
								</Label>
								<Textarea
									id={`txt-values-${d.key}`}
									value={d.values}
									onChange={(e) =>
										changeRecord(d.key, { values: e.target.value })
									}
									rows={2}
									placeholder="v=DKIM1; k=rsa; p=…"
								/>
								{linesOf(d.values).map((line) => (
									<LongValueNote key={line} text={line} />
								))}
							</div>
							<IssueList issues={own} />
						</div>
					);
				})}

				<div className="flex flex-wrap gap-2">
					<Button
						type="button"
						variant="outline"
						size="sm"
						onClick={() =>
							setRecords([
								...records,
								{ key: nextKey++, name: "", values: "", ttl: "" },
							])
						}
					>
						<Plus className="w-4 h-4 mr-1" />
						Add a text record
					</Button>
					<Button
						type="button"
						variant="outline"
						size="sm"
						disabled={records.some(
							(r) => r.name.trim() === "google._domainkey",
						)}
						onClick={() =>
							setRecords([
								...records,
								{
									key: nextKey++,
									name: "google._domainkey",
									values: "",
									ttl: "",
									google: true,
								},
							])
						}
					>
						<Plus className="w-4 h-4 mr-1" />
						Add a Google Workspace signing key (DKIM)
					</Button>
				</div>
			</div>
		</div>
	);
}
