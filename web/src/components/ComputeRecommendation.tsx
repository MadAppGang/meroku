import {
	AlertCircle,
	AlertTriangle,
	Ban,
	Check,
	Info,
	Loader2,
	Sparkles,
	Wand2,
} from "lucide-react";
import { useId, useState } from "react";
import {
	COMPUTE_POSTURES,
	type ComputeCandidate,
	type ComputeCandidateScores,
	type ComputePosture,
	type ComputeRecommendationResponse,
	type ComputeSuggestedPool,
} from "../api/infrastructure";
import { useComputeRecommendation } from "../hooks/use-compute-recommendation";
import { Alert, AlertDescription } from "./ui/alert";
import { Button } from "./ui/button";
import { Progress } from "./ui/progress";

/**
 * The sizing panel.
 *
 * It presents a SUGGESTION, not a verdict. The distinction is not decoration:
 * the ranking weights in `app/recommend` have never been calibrated against a
 * real fleet, and on the full 903-type catalog they have already produced one
 * confidently wrong answer. A panel that prints "r7i.large" and stops transfers
 * the user's judgement to a scorer that has not earned it; a panel that shows
 * what the answer was computed FROM lets the user apply what the scorer cannot
 * know — a launch next week, a workload about to triple, a service that is
 * about to be deleted.
 *
 * So everything the response already carries and the previous revision threw
 * away is on screen: the basis (and, loudly, when there is no CloudWatch
 * history at all), the runner-up with the sub-scores it lost and won on, and
 * the posture's effect stated as the knobs it actually moved.
 *
 * Accepting is still one click. Framing the answer honestly is not the same as
 * making it inconvenient.
 */
interface ComputeRecommendationProps {
	env: string;
	/** The pool the recommendation is scoped to. Empty for a pool being created. */
	pool?: string;
	posture: ComputePosture;
	onPostureChange: (posture: ComputePosture) => void;
	/**
	 * Accepting writes values, it does not lock them: every field stays
	 * individually editable afterwards (FR-42).
	 */
	onAccept: (suggested: ComputeSuggestedPool) => void;
	/** The pool's own network mode and AMI family, so the answer matches what it will render with. */
	networkMode?: string;
	amiFamily?: string;
}

const POSTURE_LABELS: Record<ComputePosture, string> = {
	"performance-first": "Performance first",
	balanced: "Balanced",
	"cost-first": "Cost first",
};

/**
 * The axis each posture pulls on, not its weights.
 *
 * The weight table lives in `app/recommend/score.go` and is still being
 * calibrated, so printing "0.35 utilisation" here would be stale the first time
 * anyone tunes it. The direction is the stable, useful part.
 */
const POSTURE_HINTS: Record<ComputePosture, string> = {
	"performance-first": "Slack over price",
	balanced: "No strong lean",
	"cost-first": "Price over slack",
};

const POSTURE_EFFECTS: Record<ComputePosture, string> = {
	"performance-first":
		"Favours slack. It leaves spare capacity on every instance, skips burstable families, prefers current-generation types and provisions on demand. Expect a larger, dearer instance than the workload strictly needs — and room to absorb a spike.",
	balanced:
		"Weighs how well the instance fits, how much of it goes unused, and what it costs, with no strong lean either way.",
	"cost-first":
		"Favours packing. It fills each instance as tightly as the workload allows, accepts burstable families and prefers spot. Expect the cheapest type that still holds the workload — and little room to absorb a spike.",
};

const CLASSIFICATION_LABELS: Record<string, string> = {
	gpu: "GPU",
	burstable: "Burstable",
	memory_heavy: "Memory-heavy",
	balanced: "Balanced",
	cpu_heavy: "CPU-heavy",
};

const DENSITY_BASIS_LABELS: Record<string, string> = {
	cpu_memory_only:
		"CPU and memory only (bridge mode places no ENI limit on tasks)",
	max_enis_minus_one:
		"the instance's ENI limit minus one, since each awsvpc task takes an interface",
	trunked_table:
		"the ENI trunking table, which raises the per-instance task limit",
};

const SERVICE_STATUS_LABELS: Record<string, string> = {
	ok: "reporting",
	no_data: "no datapoints yet",
	unavailable: "metrics unavailable",
	timeout: "metric read timed out",
};

const SOURCE_LABELS: Record<string, string> = {
	aws_api: "Live AWS catalog and pricing",
	partial: "Partly built-in data — one or more AWS reads failed",
	fallback: "Built-in catalog and prices — AWS was not reached",
};

const CREDENTIALS_LABELS: Record<string, string> = {
	missing: "no AWS credentials",
	expired: "expired AWS credentials",
	denied: "AWS credentials without permission for this read",
};

type ScoreKey = keyof ComputeCandidateScores;

const SCORE_KEYS: readonly ScoreKey[] = [
	"fit",
	"utilisation",
	"cost",
	"modernity",
];

const SCORE_HINTS: Record<ScoreKey, string> = {
	fit: "How closely the instance's memory-per-vCPU matches the workload's.",
	utilisation:
		"How full each instance actually ends up — the tasks it really carries, not the tasks it could hold — against the fill this posture targets. Under and over both cost you.",
	cost: "What the pool this type implies costs per hour per task, against the cheapest candidate.",
	modernity: "Preference for current-generation families.",
};

/** A sub-score where the alternative and the suggestion meaningfully differ. */
interface ScoreDelta {
	key: ScoreKey;
	suggested: number;
	alternative: number;
	delta: number;
}

/**
 * Below this the two candidates are saying the same thing, and reporting it as
 * a difference would invent a reason the ranking did not actually use.
 */
const SCORE_DELTA_FLOOR = 0.02;

function scoreDeltas(
	suggested: ComputeCandidate,
	alternative: ComputeCandidate,
): ScoreDelta[] {
	return SCORE_KEYS.map((key) => ({
		key,
		suggested: suggested.scores[key],
		alternative: alternative.scores[key],
		delta: alternative.scores[key] - suggested.scores[key],
	}))
		.filter((d) => Math.abs(d.delta) >= SCORE_DELTA_FLOOR)
		.sort((a, b) => Math.abs(b.delta) - Math.abs(a.delta));
}

function formatHourly(hourly: number): string {
	return `$${hourly.toFixed(hourly < 1 ? 4 : 3)}/hr`;
}

function formatMemory(memoryMiB: number): string {
	const gib = memoryMiB / 1024;
	return `${Number.isInteger(gib) ? gib : gib.toFixed(1)} GiB`;
}

function pct(fraction: number): string {
	return `${Math.round(fraction * 100)}%`;
}

function plural(n: number, word: string): string {
	return `${n} ${word}${n === 1 ? "" : "s"}`;
}

/**
 * The knobs the posture moved, read off the suggestion it produced rather than
 * off a table copied from the Go core. Clicking through the three postures
 * changes this line, which is the point: the effect is visible without having
 * to try each one and remember what the last one said.
 */
function postureEffect(pool: ComputeSuggestedPool): string {
	const capacity =
		pool.capacity_type === "on_demand"
			? "on-demand"
			: pool.capacity_type === "spot"
				? "spot"
				: `spot with ${plural(pool.on_demand_base, "on-demand instance")} always held`;
	return `${capacity} · ${pool.min_size}–${pool.max_size} instances · ${pool.target_capacity}% target capacity`;
}

function ScoreBar({ label, value }: { label: string; value: number }) {
	const filled = Math.max(0, Math.min(1, value)) * 100;
	return (
		<div className="flex items-center gap-2">
			<span className="text-xs text-gray-400 w-20 shrink-0">{label}</span>
			<Progress
				value={filled}
				aria-label={`${label} score`}
				className="h-1.5 flex-1 bg-gray-700"
			/>
			<span className="text-xs text-gray-300 w-8 text-right tabular-nums">
				{value.toFixed(2)}
			</span>
		</div>
	);
}

/**
 * The four sub-scores, one level down.
 *
 * They are real evidence and they stay reachable, but they are not the headline:
 * a row of bars printed above the fold reads as precision the scorer does not
 * have, and the plain-English `reason` above them does more for a decision.
 */
function ScoreBreakdown({ candidate }: { candidate: ComputeCandidate }) {
	return (
		<details className="mt-2">
			<summary className="text-xs text-gray-400 cursor-pointer hover:text-gray-200">
				Score breakdown
			</summary>
			<div className="mt-2 space-y-1">
				{SCORE_KEYS.map((key) => (
					<ScoreBar key={key} label={key} value={candidate.scores[key]} />
				))}
				<div className="flex items-center gap-2 pt-1">
					<span className="text-xs text-gray-400 w-20 shrink-0">total</span>
					<span className="text-xs text-gray-200 tabular-nums">
						{candidate.total.toFixed(3)}
					</span>
				</div>
			</div>
			<dl className="mt-2 space-y-1">
				{SCORE_KEYS.map((key) => (
					<div key={key} className="flex gap-2">
						<dt className="text-xs text-gray-400 w-20 shrink-0">{key}</dt>
						<dd className="text-xs text-gray-400 flex-1">{SCORE_HINTS[key]}</dd>
					</div>
				))}
			</dl>
			<p className="text-xs text-gray-400 mt-2 leading-relaxed">
				Each sub-score runs 0–1 and the total is their weighted sum. The weights
				are the posture's, and they are set from what each term means rather
				than from a fleet anyone has measured — treat a total of 0.71 against
				0.68 as a tie, not as a ranking.
			</p>
		</details>
	);
}

function CandidateCard({
	candidate,
	suggested,
}: {
	candidate: ComputeCandidate;
	suggested: boolean;
}) {
	return (
		<div
			className={`rounded-lg p-3 border ${
				suggested
					? "bg-blue-900/20 border-blue-700"
					: "bg-gray-900 border-gray-700"
			}`}
		>
			<div className="flex items-start justify-between gap-3">
				<div>
					<div className="flex items-center gap-2">
						<span className="font-mono text-sm text-gray-100">
							{candidate.instanceType}
						</span>
						{suggested && (
							<span className="text-xs px-1.5 py-0.5 rounded bg-blue-600 text-white">
								suggested
							</span>
						)}
					</div>
					<p className="text-xs text-gray-400 mt-0.5">
						{candidate.vcpu} vCPU · {formatMemory(candidate.memoryMiB)} ·{" "}
						{candidate.architecture}
					</p>
				</div>
				<div className="text-right shrink-0">
					<p className="text-sm text-gray-200">
						{formatHourly(candidate.effectiveHourly)}
					</p>
					<p className="text-xs text-gray-400">
						{plural(candidate.tasksPerInstance, "task")}/instance
					</p>
				</div>
			</div>

			<p className="text-xs text-gray-300 mt-2 leading-relaxed">
				{candidate.reason}
			</p>

			<div className="flex flex-wrap gap-x-4 gap-y-1 mt-2 text-xs text-gray-400">
				<span>
					{plural(candidate.instancesAtFloor, "instance")} at the floor
				</span>
				<span>{formatHourly(candidate.costPerTask)} per task</span>
				{candidate.spotMedianHourly !== null && (
					<span className="text-green-400">
						spot median {formatHourly(candidate.spotMedianHourly)}
					</span>
				)}
			</div>

			<ScoreBreakdown candidate={candidate} />
		</div>
	);
}

/**
 * The runner-up, and why it lost.
 *
 * FR-40 gave the user the sub-scores and no comparison. One named alternative
 * with the sub-scores it lost AND won on does more for trust than any number of
 * bars, because the win is where the user's own knowledge enters: "cheaper per
 * task, one generation back" is a trade the user can overrule from facts the
 * scorer does not have.
 */
function RunnerUp({
	suggestion,
	alternative,
}: {
	suggestion: ComputeCandidate;
	alternative: ComputeCandidate;
}) {
	const deltas = scoreDeltas(suggestion, alternative);
	const losses = deltas.filter((d) => d.delta < 0).slice(0, 2);
	const wins = deltas.filter((d) => d.delta > 0).slice(0, 2);

	const phrase = (list: ScoreDelta[]) =>
		list
			.map(
				(d) =>
					`${d.key} (${d.alternative.toFixed(2)} against ${d.suggested.toFixed(2)})`,
			)
			.join(" and ");

	const sentences: string[] = [
		`Scored ${alternative.total.toFixed(2)} to ${suggestion.instanceType}'s ${suggestion.total.toFixed(2)}.`,
	];
	if (losses.length > 0) sentences.push(`It loses on ${phrase(losses)}.`);
	if (wins.length > 0) sentences.push(`It wins on ${phrase(wins)}.`);
	if (losses.length === 0 && wins.length === 0) {
		sentences.push(
			"Every sub-score is within a rounding error of the suggestion's — this was a tie broken by the posture's family preference, not a clear win.",
		);
	}

	return (
		<div className="rounded-lg p-3 border border-gray-700 bg-gray-900">
			<div className="flex items-start justify-between gap-3">
				<div>
					<p className="text-xs text-gray-400">Closest alternative</p>
					<span className="font-mono text-sm text-gray-100">
						{alternative.instanceType}
					</span>
					<p className="text-xs text-gray-400 mt-0.5">
						{alternative.vcpu} vCPU · {formatMemory(alternative.memoryMiB)} ·{" "}
						{alternative.architecture}
					</p>
				</div>
				<div className="text-right shrink-0">
					<p className="text-sm text-gray-200">
						{formatHourly(alternative.effectiveHourly)}
					</p>
					<p className="text-xs text-gray-400">
						{formatHourly(alternative.costPerTask)} per task
					</p>
				</div>
			</div>

			<p className="text-xs text-gray-300 mt-2 leading-relaxed">
				{sentences.join(" ")}
			</p>
			<p className="text-xs text-gray-400 mt-1 leading-relaxed">
				{alternative.reason}
			</p>

			<ScoreBreakdown candidate={alternative} />

			<p className="text-xs text-gray-400 mt-2">
				To use it instead, pick it in the instance types list below — nothing
				here writes it for you.
			</p>
		</div>
	);
}

/**
 * What the suggestion was computed from, stated before the answer.
 *
 * The `configured` branch is the loud one on purpose. It is the case where the
 * advice is weakest — no measurement at all, only the cpu/memory numbers
 * somebody typed into the YAML, which are habitually generous — and the
 * previous revision reported it as the four grey words "configured requests".
 */
function EvidenceBasis({ data }: { data: ComputeRecommendationResponse }) {
	const { signals, basis } = data;
	const services = signals.services;
	const withData = services.filter((s) => s.datapoints > 0).length;
	const tasks = signals.configured.taskCount ?? 0;

	if (basis === "default") {
		return (
			<Alert className="border-gray-600 bg-gray-900">
				<Info className="h-4 w-4 text-gray-300" />
				<AlertDescription className="text-gray-300 text-xs leading-relaxed">
					<span className="font-medium">Nothing runs on this pool yet.</span>{" "}
					There is no workload to size against, measured or configured, so this
					is a conventional small starting point rather than an answer about
					your environment. Come back once services are assigned to the pool.
				</AlertDescription>
			</Alert>
		);
	}

	if (basis === "configured") {
		const why =
			signals.cloudwatch === "unavailable"
				? "CloudWatch could not be read for this environment"
				: signals.cloudwatch === "timeout"
					? "the CloudWatch read timed out"
					: "no service has enough CloudWatch history yet";
		return (
			<Alert className="border-yellow-600 bg-yellow-900/20">
				<AlertTriangle className="h-4 w-4 text-yellow-400" />
				<AlertDescription className="text-yellow-100 text-xs leading-relaxed">
					<span className="font-medium">
						No measured usage behind this suggestion.
					</span>{" "}
					It is sized from the cpu and memory{" "}
					{plural(services.length, "service")}
					{tasks > 0 ? ` (${plural(tasks, "task")})` : ""} request in your
					config, because {why}. Configured requests are usually set generously,
					so expect this to be bigger — and dearer — than the workload actually
					needs. This is the case where your own knowledge of the workload beats
					the suggestion; revisit it after a week of real traffic.
				</AlertDescription>
			</Alert>
		);
	}

	return (
		<Alert className="border-gray-600 bg-gray-900">
			<Check className="h-4 w-4 text-green-400" />
			<AlertDescription className="text-gray-300 text-xs leading-relaxed">
				<span className="font-medium text-gray-100">
					Based on measured usage.
				</span>{" "}
				{withData} of {plural(services.length, "service")} are reporting
				CloudWatch metrics, covering {pct(signals.coverage)} of the fleet&apos;s
				configured vCPU. The shape used to size this pool is{" "}
				{pct(signals.weights.actual)} what the services measure and{" "}
				{pct(signals.weights.configured)} what they request
				{signals.actual
					? `, landing on ${signals.actual.vcpu.toFixed(1)} vCPU and ${signals.actual.memGiB.toFixed(1)} GiB measured against ${signals.configured.vcpu.toFixed(1)} vCPU and ${signals.configured.memGiB.toFixed(1)} GiB configured`
					: ""}
				.
			</AlertDescription>
		</Alert>
	);
}

/** The two derivations that silently change the ranking, said out loud. */
function DerivationNotes({ data }: { data: ComputeRecommendationResponse }) {
	const { signals } = data;
	return (
		<p className="text-xs text-gray-400 leading-relaxed">
			{signals.ratio.clampedTo === "none"
				? `The workload wants ${signals.ratio.effective.toFixed(1)} GiB per vCPU.`
				: `The workload wants ${signals.ratio.raw.toFixed(1)} GiB per vCPU, clamped to the ${signals.ratio.clampedTo === "max" ? "widest" : "narrowest"} ratio any eligible type offers, ${signals.ratio.effective.toFixed(1)}.`}{" "}
			Tasks per instance were counted from{" "}
			{DENSITY_BASIS_LABELS[signals.densityBasis] ?? signals.densityBasis}.
		</p>
	);
}

function Unsatisfiable({ data }: { data: ComputeRecommendationResponse }) {
	return (
		<Alert className="border-red-600 bg-red-900/20">
			<Ban className="h-4 w-4 text-red-400" />
			<AlertDescription className="text-red-100 text-xs space-y-2">
				<p>
					{data.constraint ??
						"No instance type in this region can hold this workload."}
				</p>
				{data.nearestMisses && data.nearestMisses.length > 0 && (
					<div>
						<p className="text-red-200">Closest types, and what they lack:</p>
						<ul className="mt-1 space-y-0.5">
							{data.nearestMisses.map((miss) => (
								<li key={miss.instanceType} className="font-mono">
									{miss.instanceType} — {miss.failedRule}: needs {miss.needed}{" "}
									{miss.unit}, has {miss.available} {miss.unit}
								</li>
							))}
						</ul>
					</div>
				)}
				<p className="text-red-200">
					Reduce a service&apos;s cpu/memory request, or split the workload
					across more tasks.
				</p>
			</AlertDescription>
		</Alert>
	);
}

export function ComputeRecommendation({
	env,
	pool,
	posture,
	onPostureChange,
	onAccept,
	networkMode,
	amiFamily,
}: ComputeRecommendationProps) {
	const uid = useId();
	const [showAlternatives, setShowAlternatives] = useState(false);
	const { data, loading, error } = useComputeRecommendation({
		env,
		poolName: pool,
		posture,
		networkMode,
		amiFamily,
	});

	const alternatives =
		data?.ranked.filter(
			(candidate) => candidate.instanceType !== data.primary?.instanceType,
		) ?? [];
	const runnerUp = alternatives[0];
	const remaining = alternatives.slice(1);

	return (
		<div className="space-y-3 p-3 bg-gray-800 rounded-lg">
			<div className="flex items-start justify-between gap-3">
				<div>
					<h4 className="text-sm font-medium text-gray-200 flex items-center gap-2">
						<Sparkles className="w-4 h-4 text-purple-400" />
						Suggested starting point
					</h4>
					<p className="text-xs text-gray-400 mt-0.5">
						A size to consider, with the evidence it came from. You decide —
						nothing here is applied until you accept it.
					</p>
				</div>
				{loading && (
					<Loader2
						className="w-4 h-4 animate-spin text-gray-400 shrink-0"
						aria-label="Loading recommendation"
					/>
				)}
			</div>

			{/* Posture. Three buttons rather than a select: the choice is a
			    trade-off the user should see all of, not one they hunt for. */}
			<fieldset className="space-y-1.5">
				<legend className="sr-only">Sizing posture</legend>
				<div className="grid grid-cols-3 gap-1.5">
					{COMPUTE_POSTURES.map((option) => (
						<button
							key={option}
							type="button"
							aria-pressed={posture === option}
							onClick={() => onPostureChange(option)}
							className={`px-2 py-1.5 rounded-md border text-left transition-colors ${
								posture === option
									? "bg-purple-900/30 border-purple-600"
									: "bg-gray-900 border-gray-700 hover:border-gray-500"
							}`}
						>
							<span className="block text-xs font-medium text-gray-200">
								{POSTURE_LABELS[option]}
							</span>
							<span className="block text-xs text-gray-400 mt-0.5 leading-tight">
								{POSTURE_HINTS[option]}
							</span>
						</button>
					))}
				</div>
				<p className="text-xs text-gray-400 leading-relaxed">
					{POSTURE_EFFECTS[posture]}
				</p>
				{data && (
					<p className="text-xs text-gray-400">
						<span className="text-gray-300">This posture asks for:</span>{" "}
						{postureEffect(data.suggestedPool)}
					</p>
				)}
			</fieldset>

			{error && (
				<Alert className="border-yellow-600 bg-yellow-900/20">
					<AlertCircle className="h-4 w-4 text-yellow-400" />
					<AlertDescription className="text-yellow-100 text-xs">
						{error} — every field below stays editable by hand.
					</AlertDescription>
				</Alert>
			)}

			{!data && loading && (
				<div className="space-y-2">
					<div className="h-24 rounded-lg bg-gray-900 border border-gray-800 animate-pulse" />
					<div className="h-4 w-2/3 rounded bg-gray-900 animate-pulse" />
				</div>
			)}

			{data && (
				<>
					{data.pricingRegion && (
						<Alert className="border-yellow-600 bg-yellow-900/20">
							<AlertCircle className="h-4 w-4 text-yellow-400" />
							<AlertDescription className="text-yellow-100 text-xs">
								This ranking was priced with indicative {data.pricingRegion}{" "}
								list prices, not {data.region} prices — the Pricing API could
								not be reached. The shapes are right; the dollar figures are not
								this region&apos;s, and the cost sub-score inherits that.
							</AlertDescription>
						</Alert>
					)}

					{data.notice && !data.pricingRegion && (
						<Alert className="border-yellow-600 bg-yellow-900/20">
							<Info className="h-4 w-4 text-yellow-400" />
							<AlertDescription className="text-yellow-100 text-xs">
								{data.notice}
							</AlertDescription>
						</Alert>
					)}

					<EvidenceBasis data={data} />

					{data.classificationSuppressed && (
						<Alert className="border-yellow-600 bg-yellow-900/20">
							<AlertTriangle className="h-4 w-4 text-yellow-400" />
							<AlertDescription className="text-yellow-100 text-xs leading-relaxed">
								Measured utilisation reads as a{" "}
								{CLASSIFICATION_LABELS[data.classificationSuppressed] ??
									data.classificationSuppressed}{" "}
								workload, but {data.region} offers no type that could serve it.
								Sizing fell back to{" "}
								{CLASSIFICATION_LABELS[data.classification] ??
									data.classification}
								, read off the configured cpu:memory ratio instead.
							</AlertDescription>
						</Alert>
					)}

					<div className="flex items-center gap-2 flex-wrap">
						<span className="text-xs px-2 py-0.5 rounded-full bg-gray-700 text-gray-200">
							{CLASSIFICATION_LABELS[data.classification] ??
								data.classification}
						</span>
						{data.signals.networkMode === "awsvpc" &&
							data.signals.trunking !== "not_applicable" && (
								<span className="text-xs text-gray-400">
									ENI trunking {data.signals.trunking}
								</span>
							)}
					</div>

					{data.unsatisfiable ? (
						<Unsatisfiable data={data} />
					) : data.primary ? (
						<CandidateCard candidate={data.primary} suggested />
					) : (
						<Alert className="border-gray-600 bg-gray-900">
							<Info className="h-4 w-4 text-gray-300" />
							<AlertDescription className="text-gray-300 text-xs">
								No candidate survived the filters, so there is no type to
								suggest. Set the fields below by hand.
							</AlertDescription>
						</Alert>
					)}

					{/* The runner-up is shown, not hidden behind a toggle: a single
					    alternative with the reason it lost does more for trust than
					    five scores on the winner. */}
					{data.primary && runnerUp && (
						<RunnerUp suggestion={data.primary} alternative={runnerUp} />
					)}

					<DerivationNotes data={data} />

					<details className="text-xs text-gray-400">
						<summary className="cursor-pointer hover:text-gray-200">
							Per-service metric coverage
						</summary>
						<ul className="mt-1.5 space-y-1">
							{data.signals.services.map((service) => (
								<li key={service.name} className="flex justify-between gap-3">
									<span className="font-mono text-gray-300">
										{service.name}
									</span>
									<span className="text-right">
										{service.datapoints > 0
											? `${service.datapoints} points · cpu ${service.cpuAvg.toFixed(0)}% avg / ${service.cpuPeak.toFixed(0)}% peak · mem ${service.memAvg.toFixed(0)}% avg / ${service.memPeak.toFixed(0)}% peak`
											: (service.reason ??
												SERVICE_STATUS_LABELS[service.status] ??
												service.status)}
									</span>
								</li>
							))}
							{data.signals.services.length === 0 && (
								<li>No services are in scope for this pool.</li>
							)}
						</ul>
					</details>

					{remaining.length > 0 && (
						<div>
							<button
								type="button"
								onClick={() => setShowAlternatives((v) => !v)}
								aria-expanded={showAlternatives}
								aria-controls={`${uid}-further-candidates`}
								className="text-xs text-blue-400 hover:text-blue-300"
							>
								{showAlternatives ? "Hide" : "Show"} {remaining.length} further
								candidate{remaining.length === 1 ? "" : "s"}
							</button>
							{showAlternatives && (
								<div
									id={`${uid}-further-candidates`}
									className="space-y-2 mt-2"
								>
									{remaining.map((candidate) => (
										<CandidateCard
											key={candidate.instanceType}
											candidate={candidate}
											suggested={false}
										/>
									))}
								</div>
							)}
						</div>
					)}

					{data.suggestedPool.downgraded && (
						<Alert className="border-yellow-600 bg-yellow-900/20">
							<AlertCircle className="h-4 w-4 text-yellow-400" />
							<AlertDescription className="text-yellow-100 text-xs">
								Spot was requested but has no market for{" "}
								{data.suggestedPool.instance_types[0] ?? "the chosen type"} in{" "}
								{data.region}, so this suggestion falls back to on-demand.
							</AlertDescription>
						</Alert>
					)}

					{!data.unsatisfiable && (
						<div className="flex items-center justify-between gap-3 pt-1">
							<p className="text-xs text-gray-400">
								Accepting fills the fields below. Nothing is locked — change any
								of them afterwards.
							</p>
							<Button
								type="button"
								size="sm"
								onClick={() => onAccept(data.suggestedPool)}
								className="shrink-0"
							>
								<Wand2 className="w-3.5 h-3.5 mr-1.5" />
								Accept
							</Button>
						</div>
					)}

					<p className="text-xs text-gray-400 flex items-center gap-1">
						<Check className="w-3 h-3 shrink-0" />
						Would set {data.suggestedPool.instance_types.join(", ")} ·{" "}
						{data.suggestedPool.capacity_type} · {data.suggestedPool.min_size}–
						{data.suggestedPool.max_size} instances ·{" "}
						{data.suggestedPool.target_capacity}% target
					</p>

					<p className="text-xs text-gray-400">
						{SOURCE_LABELS[data.source] ?? data.source}
						{data.credentialsState !== "ok"
							? ` · ${CREDENTIALS_LABELS[data.credentialsState] ?? data.credentialsState}`
							: ""}
						{data.pricingRegion
							? ` · prices from ${data.pricingRegion}, indicative only`
							: ` · ${data.region}`}
					</p>
				</>
			)}
		</div>
	);
}
