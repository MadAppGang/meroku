/**
 * Checks for the DNS values of one ses.domains[] entry. Mirrors
 * validateSESDomainDNS in app/ses_dns_validate.go, which refuses the same
 * values at `meroku generate`; keep the two in step.
 */
import type { SESDomain } from "../types/yamlConfig";

export type SESDNSIssueCode =
	| "not_spf"
	| "bad_ttl"
	| "spf_in_list"
	| "empty_value"
	| "non_printable"
	| "stray_quote"
	| "split_inside_escape"
	| "apex_name"
	| "absolute_name"
	| "bad_name"
	| "reserved_name"
	| "name_too_long"
	| "duplicate_name"
	| "no_values"
	| "not_a_list"
	| "not_a_record";

export interface SESDNSIssue {
	field: string;
	code: SESDNSIssueCode;
	message: string;
	hint: string;
}

/** The parts of the ses block the checks read. */
export interface SESMailFromDefaults {
	enable_mail_from?: boolean;
	mail_from_subdomain?: string;
}

export const SES_DEFAULT_SPF_RECORD = "v=spf1 include:amazonses.com ~all";

const TXT_DELIMITER = '" "';
const MAX_NAME_LENGTH = 253;
const SPF_PREFIX = /^v=spf1( |$)/;
const SPF_PREFIX_ANY_CASE = /^v=spf1( |$)/i;
const PRINTABLE = /^[\x20-\x7E]*$/;
const ESCAPE_AT_PART_END = /\\[0-9]{0,2}$/;
const RECORD_NAME =
	/^[a-z0-9_]([a-z0-9_-]{0,61}[a-z0-9_])?(\.[a-z0-9_]([a-z0-9_-]{0,61}[a-z0-9_])?)*$/;

const RESERVED_NAMES: Record<string, string> = {
	_amazonses: "Amazon SES uses this name to check that you own the domain.",
	_dmarc:
		"meroku writes the domain's DMARC policy here; set dmarc_policy instead.",
};

const issue = (
	field: string,
	code: SESDNSIssueCode,
	message: string,
	hint: string,
): SESDNSIssue => ({ field, code, message, hint });

const joined = (value: string) => value.split(TXT_DELIMITER).join("");

export const isSPFValue = (value: string) =>
	SPF_PREFIX_ANY_CASE.test(joined(value));

/** The rules every apex or named TXT value shares. */
export function validateSESTXTValue(
	field: string,
	value: string,
): SESDNSIssue[] {
	if (joined(value) === "") {
		return [
			issue(
				field,
				"empty_value",
				"The value is empty.",
				"Type the text the record should hold, or remove the entry.",
			),
		];
	}
	const issues: SESDNSIssue[] = [];
	if (!PRINTABLE.test(value)) {
		issues.push(
			issue(
				field,
				"non_printable",
				"The value holds a character other than plain letters, digits, spaces and punctuation.",
				"Use only plain letters, digits, spaces and punctuation. Write any other character as a DNS escape such as \\344.",
			),
		);
	}
	if (
		joined(value).includes('"') ||
		value.startsWith('"') ||
		value.endsWith('"')
	) {
		issues.push(
			issue(
				field,
				"stray_quote",
				"The value holds a quote character.",
				'Remove the quote characters. meroku adds the quotes DNS needs. The only quotes allowed are the three characters " " that join two parts of one long value.',
			),
		);
	}
	partLoop: for (const part of value.split(TXT_DELIMITER)) {
		for (let i = 0; i + 255 < part.length; i += 255) {
			if (ESCAPE_AT_PART_END.test(part.slice(i, i + 255))) {
				issues.push(
					issue(
						field,
						"split_inside_escape",
						"The value is longer than 255 characters and would be cut in the middle of a backslash escape.",
						'Split it yourself: put the three characters " " at a point outside any backslash escape.',
					),
				);
				break partLoop;
			}
		}
	}
	return issues;
}

const isWholeNonNegative = (v: unknown) =>
	typeof v === "number" && Number.isInteger(v) && v >= 0;

/** Checks a candidate sender record; an empty one means the default. */
export function validateSPFRecord(spf: string | undefined): SESDNSIssue[] {
	if (!spf) return [];
	if (!SPF_PREFIX.test(spf)) {
		return [
			issue(
				"spf_record",
				"not_spf",
				`spf_record "${spf}" is not a sender (SPF) record.`,
				"A sender record starts with v=spf1 in lower case, for example: v=spf1 include:amazonses.com ~all",
			),
		];
	}
	return validateSESTXTValue("spf_record", spf);
}

export function validateSESDomainDNS(
	ses: SESMailFromDefaults,
	d: SESDomain,
): SESDNSIssue[] {
	const issues: SESDNSIssue[] = [];
	const domain = (d.domain || "").toLowerCase();

	issues.push(...validateSPFRecord(d.spf_record));
	if (d.spf_ttl !== undefined && d.spf_ttl !== null) {
		if (!isWholeNonNegative(d.spf_ttl)) {
			issues.push(
				issue(
					"spf_ttl",
					"bad_ttl",
					`spf_ttl ${d.spf_ttl} is not a whole number of seconds.`,
					"Use a whole number of seconds, 0 or more. 600 is ten minutes.",
				),
			);
		}
	}

	(d.apex_txt_values ?? []).forEach((v, i) => {
		const field = `apex_txt_values[${i}]`;
		if (isSPFValue(v)) {
			issues.push(
				issue(
					field,
					"spf_in_list",
					"This is a sender (SPF) record.",
					"Set it as spf_record instead (in the web app: Who sends email from this domain). A domain can have only one.",
				),
			);
			return;
		}
		issues.push(...validateSESTXTValue(field, v));
	});

	const mailFromOn = d.enable_mail_from ?? ses.enable_mail_from ?? true;
	const mailFromSub =
		d.mail_from_subdomain || ses.mail_from_subdomain || "bounce";
	const seen = new Map<string, number>();

	(d.txt_records ?? []).forEach((r, i) => {
		const base = `txt_records[${i}]`;
		const name = r.name ?? "";
		const nameField = `${base}.name`;
		if (name === "" || name === "@") {
			issues.push(
				issue(
					nameField,
					"apex_name",
					"The name is empty or @, which is the domain itself.",
					"Values at the domain itself go under the other text records at the domain (apex_txt_values).",
				),
			);
		} else if (
			domain !== "" &&
			(name === domain || name.endsWith(`.${domain}`))
		) {
			issues.push(
				issue(
					nameField,
					"absolute_name",
					`The name "${name}" already ends with the domain ${domain}.`,
					`Write only the part before .${domain}, for example google._domainkey.`,
				),
			);
		} else if (!RECORD_NAME.test(name)) {
			issues.push(
				issue(
					nameField,
					"bad_name",
					`The name "${name}" is not a valid DNS name.`,
					"Use lower-case letters, digits, hyphens and underscores, with dots between the parts, for example google._domainkey. No dot at the end.",
				),
			);
		} else {
			const why = RESERVED_NAMES[name];
			const dup = seen.get(name);
			if (why) {
				issues.push(
					issue(
						nameField,
						"reserved_name",
						`meroku writes the record ${name}.${domain} itself.`,
						`${why} Pick another name.`,
					),
				);
			} else if (mailFromOn && name === mailFromSub) {
				issues.push(
					issue(
						nameField,
						"reserved_name",
						`meroku writes the record ${name}.${domain} itself.`,
						"This is the bounce address Amazon SES sends from. Pick another name.",
					),
				);
			} else if (name.length + 1 + domain.length > MAX_NAME_LENGTH) {
				issues.push(
					issue(
						nameField,
						"name_too_long",
						`${name}.${domain} is longer than 253 characters.`,
						"Use a shorter name.",
					),
				);
			} else if (dup !== undefined) {
				issues.push(
					issue(
						nameField,
						"duplicate_name",
						`The name "${name}" is also used by extra record ${dup + 1}.`,
						"Put all values for one name in one entry.",
					),
				);
			}
			seen.set(name, i);
		}

		const values = r.values ?? [];
		if (values.length === 0) {
			issues.push(
				issue(
					`${base}.values`,
					"no_values",
					"The record has no values.",
					"Add the text the record should hold.",
				),
			);
		}
		values.forEach((v, j) => {
			const field = `${base}.values[${j}]`;
			if (isSPFValue(v)) {
				issues.push(
					issue(
						field,
						"spf_in_list",
						"This is a sender (SPF) record.",
						"meroku writes sender records only at the domain itself, through spf_record.",
					),
				);
				return;
			}
			issues.push(...validateSESTXTValue(field, v));
		});
		if (r.ttl !== undefined && r.ttl !== null && !isWholeNonNegative(r.ttl)) {
			issues.push(
				issue(
					`${base}.ttl`,
					"bad_ttl",
					`ttl ${r.ttl} is not a whole number of seconds.`,
					"Use a whole number of seconds, 0 or more. 3600 is one hour.",
				),
			);
		}
	});

	return issues;
}

/** Issues whose field starts with prefix, for showing next to an input. */
export const issuesFor = (issues: SESDNSIssue[], prefix: string) =>
	issues.filter(
		(i) =>
			i.field === prefix ||
			i.field.startsWith(`${prefix}.`) ||
			i.field.startsWith(`${prefix}[`),
	);
