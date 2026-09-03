import { Info, Plus, X } from "lucide-react";
import { useEffect, useId, useState } from "react";
import type {
	GitHubOIDCDegradedReason,
	GitHubOIDCSubjectConflict,
	GitHubOIDCSubjectConflictsResponse,
} from "../api/infrastructure";
import {
	infrastructureApi,
	isGitHubOIDCNotApplicable,
} from "../api/infrastructure";
import type { YamlInfrastructureConfig } from "../types/yamlConfig";
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
import { Switch } from "./ui/switch";

interface GitHubNodePropertiesProps {
	config: YamlInfrastructureConfig;
	onConfigChange: (config: Partial<YamlInfrastructureConfig>) => void;
}

/**
 * Editing a subject is a keystroke at a time, and each scan paginates IAM. Wait
 * for the edit to settle before asking.
 */
const SUBJECT_SCAN_DEBOUNCE_MS = 500;

/** Plain English for the reasons that mean the scan did not see everything. */
const DEGRADED_REASON_TEXT: Partial<Record<GitHubOIDCDegradedReason, string>> =
	{
		no_account_id:
			"this environment declares no account_id, so there was no account to scan",
		no_credentials: "no AWS credentials were available for this environment",
		wrong_account:
			"the credentials resolved to a different account than this environment declares",
		access_denied: "these credentials may not list IAM roles (iam:ListRoles)",
		throttled: "AWS throttled the request",
		timeout: "the scan ran out of time",
		env_unreadable: "another environment's config could not be read",
		unparseable_policy: "a role's trust policy could not be parsed",
		pattern_too_long: "a subject pattern was too long to compare",
		pagination_incomplete: "AWS returned an incomplete list of roles",
		unevaluatable_claims: "some roles restrict access by claims other than sub",
		pair_budget_exhausted: "there were too many subject pairs to compare",
	};

/**
 * A witness is a `sub` claim both patterns accept. `*` against `*` legitimately
 * produces the empty string, which must read as "anything" rather than as a gap
 * in the sentence.
 */
function Witness({ value }: { value: string }) {
	if (value === "") {
		return <span className="italic text-red-300">any subject at all</span>;
	}
	return <code className="break-all text-red-300">{value}</code>;
}

/** Who owns the conflicting role, and how confidently we know. */
function conflictOwner(conflict: GitHubOIDCSubjectConflict): string {
	if (conflict.owner_project) {
		return conflict.owner_env
			? `It belongs to the ${conflict.owner_project} project (${conflict.owner_env}).`
			: `It belongs to the ${conflict.owner_project} project.`;
	}
	if (conflict.attribution === "unavailable") {
		return "Its tags could not be read, so its owner is unknown.";
	}
	return "It carries no meroku tags, so its owner is unknown.";
}

export function GitHubNodeProperties({
	config,
	onConfigChange,
}: GitHubNodePropertiesProps) {
	const uid = useId();
	const isEnabled = config.workload?.enable_github_oidc ?? false;
	const subjects = config.workload?.github_oidc_subjects || [];

	// AWS holds one GitHub OIDC provider per account, so enabling this in a
	// second project that shares an account used to fail the apply with
	// EntityAlreadyExists and offered nothing to do about it. Resolve it here,
	// while the user is looking at the setting, rather than at deploy time.
	const [providerStatus, setProviderStatus] = useState<Awaited<
		ReturnType<typeof infrastructureApi.getGitHubOIDCStatus>
	> | null>(null);
	const [providerError, setProviderError] = useState<string | null>(null);

	// The provider is shared by design. The subjects are not: they are the only
	// thing deciding which repository may assume this project's role, and
	// nothing in AWS stops a second project claiming the same ones.
	const [conflictScan, setConflictScan] =
		useState<GitHubOIDCSubjectConflictsResponse | null>(null);
	const [conflictError, setConflictError] = useState<string | null>(null);
	const [conflictScanning, setConflictScanning] = useState(false);

	useEffect(() => {
		if (!isEnabled || !config.env) {
			setProviderStatus(null);
			setProviderError(null);
			return;
		}

		// Guards against a slow response for one environment landing after the
		// user has already switched to another.
		let current = true;

		infrastructureApi
			.getGitHubOIDCStatus(config.env)
			.then((status) => {
				if (!current) return;
				setProviderStatus(status);
				setProviderError(null);
			})
			.catch((err: Error) => {
				if (!current) return;
				// A failed read is not evidence that no provider exists, so this
				// reports the failure and changes nothing.
				setProviderStatus(null);
				setProviderError(err.message);
			});

		return () => {
			current = false;
		};
	}, [isEnabled, config.env]);

	// Serialised so the effect keys off the subjects themselves. The array
	// identity changes on every render when the field is absent, which would
	// otherwise rescan forever.
	const subjectsKey = JSON.stringify(subjects);

	useEffect(() => {
		if (!isEnabled || !config.env) {
			setConflictScan(null);
			setConflictError(null);
			setConflictScanning(false);
			return;
		}

		const env = config.env;
		const ownSubjects = JSON.parse(subjectsKey) as string[];

		// A result describes the subjects it was asked about and nothing else.
		// Anything already on screen answers an older question, so it goes now
		// rather than sitting there as reassurance for a subject list the user
		// has since broadened.
		setConflictScan(null);
		setConflictError(null);
		setConflictScanning(true);

		const controller = new AbortController();
		const timer = window.setTimeout(() => {
			infrastructureApi
				// Send what the user is looking at, not what is on disk: the config
				// save is asynchronous and an unsaved edit must still be checked.
				.getGitHubOIDCSubjectConflicts(env, ownSubjects, controller.signal)
				.then((result) => {
					if (controller.signal.aborted) return;
					setConflictScan(result);
					setConflictError(null);
					setConflictScanning(false);
				})
				.catch((err: Error) => {
					if (controller.signal.aborted) return;
					// Transport failure, non-2xx, or unparseable JSON. None of them
					// is evidence that no conflict exists, so none of them is silent.
					setConflictScan(null);
					setConflictError(err.message);
					setConflictScanning(false);
				});
		}, SUBJECT_SCAN_DEBOUNCE_MS);

		// Abort rather than merely ignore: an older in-flight scan can never
		// overwrite a newer one, and the request itself is not left running.
		return () => {
			window.clearTimeout(timer);
			controller.abort();
		};
	}, [isEnabled, config.env, subjectsKey]);

	const conflicts = conflictScan?.conflicts ?? [];
	// Our own roles that trust all of GitHub. Not a conflict and not a
	// degradation: a complete scan reports these with `checked: true`, which is
	// why nothing below reads them through the checked/degraded machinery.
	const ownUnrestrictedRoles = conflictScan?.own_unrestricted_roles ?? [];
	// Our own subjects that accept a whole organisation. Same standing as the
	// roles above — a finding on a complete scan, not a degradation — and one
	// tier quieter.
	const ownOrgWideSubjects = conflictScan?.own_org_wide_subjects ?? [];
	const excludedRoles = conflictScan?.excluded_roles ?? [];
	const unevaluatedRoles = conflictScan?.unevaluated_roles ?? [];
	const degraded = conflictScan?.degraded ?? [];
	// The partition: a reason is either "the scan did not need to run" or "the
	// scan did not see everything". The server decides which and says so in
	// `kind`; the reason list is only a fallback for a response without it.
	// Anything unrecognised counts as the latter.
	const notApplicable = degraded.filter(isGitHubOIDCNotApplicable);
	const scanIncomplete = degraded.filter(
		(entry) => !isGitHubOIDCNotApplicable(entry),
	);

	// Verified means the account was scanned to completion. `checked` alone is
	// not trusted: a degraded or unevaluated role contradicts it, and the UI
	// must not render a clean state over a role nobody looked at.
	const scanComplete =
		conflictScan !== null &&
		conflictError === null &&
		conflictScan.checked &&
		scanIncomplete.length === 0 &&
		unevaluatedRoles.length === 0;
	// verifiedClean is what licenses the blue panel's isolation claim, so an own
	// role that trusts all of GitHub has to disqualify it: "scoped to the
	// subjects below" is false of a role with no subject condition, and printing
	// it directly under the red panel would contradict it on the same screen.
	//
	// An org-wide subject disqualifies it for the same reason and needs saying
	// separately: the subject condition genuinely IS scoped to the subjects
	// below, so the sentence is not false in the way the unrestricted case makes
	// it false — it is merely worthless, because one of those subjects is an
	// entire organisation.
	const verifiedClean =
		scanComplete &&
		conflicts.length === 0 &&
		ownUnrestrictedRoles.length === 0 &&
		ownOrgWideSubjects.length === 0;
	// Not applicable is silent, not yellow: OIDC switched off must not paint a
	// permanent "could not verify" banner.
	const notApplicableOnly =
		conflictScan !== null &&
		conflictError === null &&
		!conflictScan.checked &&
		conflicts.length === 0 &&
		scanIncomplete.length === 0 &&
		unevaluatedRoles.length === 0 &&
		notApplicable.length > 0;

	// The top tier: it outranks the conflict list, including a foreign
	// unrestricted one, and renders regardless of `checked` for the same reason
	// the conflict tier does — the finding stands whether or not the walk that
	// produced it also missed something.
	const showOwnUnrestricted = isEnabled && ownUnrestrictedRoles.length > 0;
	// Ranked between the two, and rendered on the same terms: the finding stands
	// whether or not the walk that produced it also missed something.
	const showOwnOrgWide = isEnabled && ownOrgWideSubjects.length > 0;
	// True when any of them is meroku's untouched default, which decides the
	// headline: that case is a grant to a third party rather than a wide grant
	// inside an organisation the user controls.
	const anyShippedDefault = ownOrgWideSubjects.some(
		(subject) => subject.shipped_default,
	);
	const showConflicts = isEnabled && conflicts.length > 0;
	// `!scanComplete` rather than `!verifiedClean`: with conflicts.length === 0
	// the two were identical before an own unrestricted role could disqualify
	// verifiedClean, and this keeps the yellow tier out of a case the scan
	// verified perfectly well.
	const showUnverified =
		isEnabled && conflicts.length === 0 && !scanComplete && !notApplicableOnly;

	// Loudest first: a role that trusts all of GitHub outranks a narrow overlap.
	const orderedConflicts = [...conflicts].sort(
		(a, b) => Number(b.unrestricted) - Number(a.unrestricted),
	);
	const unrestrictedCount = conflicts.filter(
		(conflict) => conflict.unrestricted,
	).length;

	const handleToggleOIDC = (checked: boolean) => {
		onConfigChange({
			workload: {
				enable_github_oidc: checked,
				github_oidc_subjects:
					checked && subjects.length === 0
						? ["repo:Owner/Repo:ref:refs/heads/main"]
						: subjects,
			},
		});
	};

	const handleAddSubject = () => {
		const newSubjects = [...subjects, "repo:Owner/Repo:ref:refs/heads/main"];
		onConfigChange({
			workload: {
				github_oidc_subjects: newSubjects,
			},
		});
	};

	const handleRemoveSubject = (index: number) => {
		const newSubjects = subjects.filter((_, i) => i !== index);
		onConfigChange({
			workload: {
				github_oidc_subjects: newSubjects,
			},
		});
	};

	const handleUpdateSubject = (index: number, value: string) => {
		const newSubjects = [...subjects];
		newSubjects[index] = value;
		onConfigChange({
			workload: {
				github_oidc_subjects: newSubjects,
			},
		});
	};

	return (
		<div className="space-y-6">
			<Card>
				<CardHeader>
					<CardTitle>GitHub Actions Configuration</CardTitle>
					<CardDescription>
						Configure GitHub OIDC for passwordless deployments
					</CardDescription>
				</CardHeader>
				<CardContent className="space-y-4">
					<div className="flex items-center justify-between">
						<div className="space-y-1">
							<Label htmlFor={`${uid}-github-oidc`}>Enable GitHub OIDC</Label>
							<p className="text-xs text-gray-400">
								Allow GitHub Actions to deploy without credentials
							</p>
						</div>
						<Switch
							id={`${uid}-github-oidc`}
							checked={isEnabled}
							onCheckedChange={handleToggleOIDC}
						/>
					</div>

					{showOwnUnrestricted && (
						<div className="rounded-lg border border-red-500/40 bg-red-500/10 p-3">
							<div className="flex gap-2">
								<Info className="w-4 h-4 text-red-400 shrink-0 mt-0.5" />
								<div className="space-y-3 text-xs">
									<p className="font-medium text-red-200">
										{ownUnrestrictedRoles.length === 1
											? "This project's own deploy role trusts every repository on GitHub."
											: `${ownUnrestrictedRoles.length} of this project's own deploy roles trust every repository on GitHub.`}
									</p>

									{ownUnrestrictedRoles.map((role) => (
										<div
											key={role.role_arn ?? role.role_name}
											className="space-y-1"
										>
											<p className="text-gray-200">
												<code className="break-all text-red-300">
													{role.role_name}
												</code>
												{role.env ? ` (${role.env})` : ""} has no subject
												condition on its trust policy — no{" "}
												<code className="text-red-300">sub</code>, nothing.
											</p>
											{role.role_arn && (
												<code className="block break-all text-gray-400">
													{role.role_arn}
												</code>
											)}
										</div>
									))}

									<p className="text-gray-300">
										Any repository on GitHub, belonging to anybody, can assume
										this role and obtain <code>iam:PassRole</code> over this
										project's task roles, ECR push and{" "}
										<code>ecs:UpdateService</code> — the whole deploy.
									</p>
									<p className="text-gray-400">
										meroku refuses to deploy while this is true, with no prompt
										and no override. An overlap between two projects can be
										deliberate; a role trusting all of GitHub is not. Give it a
										subject condition — set{" "}
										<code className="text-red-300">github_oidc_subjects</code>{" "}
										for the environment that owns it — and apply.
									</p>
								</div>
							</div>
						</div>
					)}

					{showOwnOrgWide && (
						<div className="rounded-lg border border-red-500/40 bg-red-500/10 p-3">
							<div className="flex gap-2">
								<Info className="w-4 h-4 text-red-400 shrink-0 mt-0.5" />
								<div className="space-y-3 text-xs">
									<p className="font-medium text-red-200">
										{anyShippedDefault
											? "This project still carries meroku's default subject, which trusts a third-party organisation."
											: ownOrgWideSubjects.length === 1
												? "One of this project's subjects trusts an entire GitHub organisation."
												: `${ownOrgWideSubjects.length} of this project's subjects trust an entire GitHub organisation.`}
									</p>

									{ownOrgWideSubjects.map((subject) => (
										<div key={subject.subject} className="space-y-1">
											<code className="block break-all text-red-300">
												{subject.subject}
											</code>
											{subject.shipped_default ? (
												<p className="text-gray-200">
													This is meroku's default, unchanged. It matches every
													token issued to a workflow in{" "}
													<strong className="text-red-200">MadAppGang's</strong>{" "}
													own repositories — an organisation that is not yours.
													Unless you are MadAppGang, it grants a third party the
													ability to assume this project's deploy role in{" "}
													<strong className="text-red-200">your</strong> AWS
													account.
												</p>
											) : (
												<p className="text-gray-200">
													It matches every repository in{" "}
													{subject.org ? (
														<>
															the{" "}
															<code className="text-red-300">
																{subject.org}
															</code>{" "}
															organisation
														</>
													) : (
														"any organisation it matches"
													)}
													, on every branch, tag and pull request — not only the
													repositories this project deploys from.
												</p>
											)}
										</div>
									))}

									<p className="text-gray-300">
										Any workflow matching one of the subjects above can assume
										this project's deploy role, which grants{" "}
										<code>iam:PassRole</code> over its task roles, ECR push and{" "}
										<code>ecs:UpdateService</code> — the whole deploy.
									</p>
									<p className="text-gray-400">
										Narrow{" "}
										<code className="text-red-300">github_oidc_subjects</code>{" "}
										to the repositories that actually deploy this project — for
										example{" "}
										<code className="break-all text-red-300">
											repo:your-org/your-repo:ref:refs/heads/main
										</code>
										. meroku asks before deploying past this rather than
										refusing: an organisation-wide subject can be deliberate.
									</p>
									{scanIncomplete.length > 0 && (
										<p className="text-gray-400">
											This scan did not finish, so there may be more than what
											is listed here.
										</p>
									)}
								</div>
							</div>
						</div>
					)}

					{showConflicts && (
						<div className="rounded-lg border border-red-500/40 bg-red-500/10 p-3">
							<div className="flex gap-2">
								<Info className="w-4 h-4 text-red-400 shrink-0 mt-0.5" />
								<div className="space-y-3 text-xs">
									<p className="font-medium text-red-200">
										{unrestrictedCount > 0
											? unrestrictedCount === 1
												? "A role in this AWS account trusts every GitHub repository."
												: `${unrestrictedCount} roles in this AWS account trust every GitHub repository.`
											: conflicts.length === 1
												? "Another role in this AWS account accepts the same GitHub tokens as this project."
												: `${conflicts.length} other roles in this AWS account accept the same GitHub tokens as this project.`}
									</p>

									{orderedConflicts.map((conflict) => (
										<div key={conflict.role_arn} className="space-y-1">
											<p className="text-gray-200">
												<code className="break-all text-red-300">
													{conflict.role_name}
												</code>{" "}
												{conflict.unrestricted
													? "has no subject condition at all, so it accepts a token from any GitHub repository."
													: "accepts a GitHub token that this project's subjects also accept."}{" "}
												{conflictOwner(conflict)}
											</p>
											<code className="block break-all text-gray-400">
												{conflict.role_arn}
											</code>
											{conflict.overlaps.length > 0 && (
												<ul className="space-y-1 text-gray-300">
													{conflict.overlaps.map((overlap) => (
														<li
															key={`${overlap.own_subject}|${overlap.other_subject}|${overlap.witness}`}
														>
															• Your{" "}
															<code className="break-all text-red-300">
																{overlap.own_subject}
															</code>{" "}
															and its{" "}
															<code className="break-all text-red-300">
																{overlap.other_subject}
															</code>{" "}
															both accept <Witness value={overlap.witness} />.
														</li>
													))}
												</ul>
											)}
											<p className="text-gray-300">
												A workflow whose{" "}
												<code className="text-red-300">sub</code> claim is{" "}
												{conflict.overlaps.length > 0 ? (
													<Witness value={conflict.overlaps[0].witness} />
												) : (
													<span className="italic text-red-300">
														any subject at all
													</span>
												)}{" "}
												can assume both this project's deploy role and{" "}
												<code className="break-all text-red-300">
													{conflict.role_name}
												</code>
												, which grants <code>iam:PassRole</code> over that
												project's task roles, ECR push and{" "}
												<code>ecs:UpdateService</code>.
											</p>
										</div>
									))}

									<p className="text-gray-400">
										This takes effect wherever both projects have GitHub OIDC
										enabled — <code>enable_github_oidc</code> defaults to false,
										so an overlap may be latent rather than live today.
									</p>
									<p className="text-gray-400">
										meroku will not change either subject list for you: it
										cannot tell which of the two is the mistake. Narrow one of
										them until they no longer overlap.
									</p>
									{scanIncomplete.length > 0 && (
										<p className="text-gray-400">
											This scan did not finish, so there may be more than what
											is listed here.
										</p>
									)}
								</div>
							</div>
						</div>
					)}

					{isEnabled &&
						providerStatus &&
						!providerStatus.owned_by_this_env &&
						providerStatus.exists && (
							<div className="rounded-lg border border-blue-500/40 bg-blue-500/10 p-3">
								<div className="flex gap-2">
									<Info className="w-4 h-4 text-blue-400 shrink-0 mt-0.5" />
									<div className="space-y-2 text-xs">
										<p className="text-gray-200">
											This AWS account already trusts GitHub
											{providerStatus.owner_project
												? `, through the ${providerStatus.owner_project} project`
												: ""}
											. AWS allows one GitHub identity provider per account, so
											this environment will reuse it instead of creating a
											second.
										</p>
										{providerStatus.arn && (
											<code className="block break-all text-blue-300">
												{providerStatus.arn}
											</code>
										)}
										{/* The isolation claim is only made once a completed scan
										    has actually established it. Unverified, this states the
										    mechanism and stops short of the outcome. */}
										<p className="text-gray-400">
											{verifiedClean
												? "Deployments from this project stay isolated: they use their own role, scoped to the subjects below, and no other role in this account accepts those subjects."
												: "Deployments from this project use their own role, scoped to the subjects below."}
										</p>
										{providerStatus.changed && (
											<p className="text-gray-400">
												Set{" "}
												<code className="text-blue-300">
													github_oidc_create_provider: false
												</code>{" "}
												in{" "}
												<code className="text-blue-300">{config.env}.yaml</code>
												{providerStatus.backup
													? ` (backup: ${providerStatus.backup})`
													: ""}
												.
											</p>
										)}
									</div>
								</div>
							</div>
						)}

					{isEnabled && providerError && (
						<div className="rounded-lg border border-yellow-500/40 bg-yellow-500/10 p-3">
							<div className="flex gap-2">
								<Info className="w-4 h-4 text-yellow-400 shrink-0 mt-0.5" />
								<div className="space-y-1 text-xs">
									<p className="text-gray-200">
										Could not check this account's GitHub identity provider.
									</p>
									<p className="text-gray-400">{providerError}</p>
									<p className="text-gray-400">
										If the deploy fails with{" "}
										<code className="text-yellow-300">EntityAlreadyExists</code>
										, another project in this account owns the provider. Set{" "}
										<code className="text-yellow-300">
											github_oidc_create_provider: false
										</code>{" "}
										in{" "}
										<code className="text-yellow-300">{config.env}.yaml</code>.
									</p>
								</div>
							</div>
						</div>
					)}

					{showUnverified && (
						<div className="rounded-lg border border-yellow-500/40 bg-yellow-500/10 p-3">
							<div className="flex gap-2">
								<Info className="w-4 h-4 text-yellow-400 shrink-0 mt-0.5" />
								<div className="space-y-1 text-xs">
									<p className="text-gray-200">
										{conflictScanning
											? "Checking whether another project in this account claims these subjects…"
											: "Could not verify that no other project in this account claims these subjects."}
									</p>
									{conflictError && (
										<p className="text-gray-400">{conflictError}</p>
									)}
									{scanIncomplete.length > 0 && (
										<ul className="space-y-1 text-gray-400">
											{scanIncomplete.map((entry) => (
												<li key={`${entry.reason}|${entry.detail ?? ""}`}>
													• {DEGRADED_REASON_TEXT[entry.reason] ?? entry.reason}
													{entry.detail ? ` — ${entry.detail}` : ""}
												</li>
											))}
										</ul>
									)}
									{unevaluatedRoles.length > 0 && (
										<>
											<p className="text-gray-400">
												This check reasons about the{" "}
												<code className="text-yellow-300">sub</code> claim only.
												These roles restrict access by other claims and were not
												evaluated:
											</p>
											<ul className="space-y-1 text-gray-400">
												{unevaluatedRoles.map((role) => (
													<li key={role.role_name}>
														•{" "}
														<code className="break-all text-yellow-300">
															{role.role_name}
														</code>
														{role.claim_keys.length > 0
															? ` — restricted by ${role.claim_keys.join(", ")}`
															: ""}
													</li>
												))}
											</ul>
										</>
									)}
									{!conflictScanning && (
										<p className="text-gray-400">
											Treat this as unverified rather than clear. An overlap
											would let another project's workflow assume this project's
											deploy role, and this scan did not rule one out.
										</p>
									)}
								</div>
							</div>
						</div>
					)}

					{isEnabled && excludedRoles.length > 0 && (
						<div className="rounded-lg bg-gray-800 p-3">
							<div className="flex gap-2">
								<Info className="w-4 h-4 text-gray-400 shrink-0 mt-0.5" />
								<div className="space-y-1 text-xs">
									<p className="text-gray-300">
										{conflictScan?.excluded_note ||
											"This project's own roles in this account are not compared against each other; sharing a repository between your environments is expected."}
									</p>
									<div className="flex flex-wrap gap-x-2 gap-y-1">
										{excludedRoles.map((roleName) => (
											<code key={roleName} className="break-all text-gray-400">
												{roleName}
											</code>
										))}
									</div>
								</div>
							</div>
						</div>
					)}

					{isEnabled && (
						<div className="space-y-4 pt-4 border-t border-gray-700">
							<div>
								<div className="flex items-center justify-between mb-2">
									<Label>OIDC Subjects</Label>
									<Button
										size="sm"
										variant="outline"
										onClick={handleAddSubject}
										className="h-7 text-xs"
									>
										<Plus className="w-3 h-3 mr-1" />
										Add Subject
									</Button>
								</div>
								<p className="text-xs text-gray-400 mb-3">
									Define which GitHub repositories and branches can deploy
								</p>

								{subjects.length === 0 ? (
									<div className="text-sm text-gray-500 italic">
										No subjects configured. Add one to enable deployments.
									</div>
								) : (
									<div className="space-y-2">
										{subjects.map((subject, index) => (
											<div
												key={`subject-${index}-${subject}`}
												className="flex items-center gap-2"
											>
												<Input
													value={subject}
													onChange={(e) =>
														handleUpdateSubject(index, e.target.value)
													}
													placeholder="repo:Owner/Repo:ref:refs/heads/main"
													className="flex-1 bg-gray-800 border-gray-600 text-white text-sm"
												/>
												<Button
													size="icon"
													variant="ghost"
													onClick={() => handleRemoveSubject(index)}
													className="h-8 w-8 text-gray-400 hover:text-red-400"
												>
													<X className="w-4 h-4" />
												</Button>
											</div>
										))}
									</div>
								)}
							</div>

							<div className="rounded-lg bg-gray-800 p-3">
								<h4 className="text-sm font-medium text-gray-300 mb-2">
									Subject Format Examples:
								</h4>
								<ul className="space-y-1 text-xs text-gray-400">
									<li>
										•{" "}
										<code className="text-blue-400">
											repo:Owner/Repo:ref:refs/heads/main
										</code>{" "}
										- Main branch only
									</li>
									<li>
										•{" "}
										<code className="text-blue-400">
											repo:Owner/Repo:ref:refs/heads/*
										</code>{" "}
										- All branches
									</li>
									<li>
										•{" "}
										<code className="text-blue-400">
											repo:Owner/Repo:ref:refs/tags/*
										</code>{" "}
										- All tags
									</li>
									<li>
										•{" "}
										<code className="text-blue-400">
											repo:Owner/Repo:environment:production
										</code>{" "}
										- Specific environment
									</li>
								</ul>
							</div>
						</div>
					)}
				</CardContent>
			</Card>
		</div>
	);
}
