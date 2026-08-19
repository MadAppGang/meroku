import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React, { useState } from "react";
import { expect, userEvent, waitFor, within } from "storybook/test";
import type {
	ComputeCandidate,
	ComputePosture,
	ComputeRecommendationResponse,
	ComputeServiceSignal,
	ComputeSuggestedPool,
} from "../api/infrastructure";
import { ComputeRecommendation } from "./ComputeRecommendation";

/**
 * The three states that decide how much the panel is allowed to claim.
 *
 * `Measured` is the strong case; `NoCloudWatchHistory` is the weak one and is
 * the reason this file exists — the panel used to report "no measurement at
 * all" as the four grey words "configured requests", which reads as a footnote
 * rather than as the caveat that governs the whole answer. `FallbackPricing`
 * pins C-18: an indicative us-east-1 figure must never render as the
 * environment's own price.
 */

/**
 * The suggestion, and the numbers behind it.
 *
 * Every figure here is DERIVED, not decorative, because the four sub-scores are
 * the contract this file exists to pin. The fleet is six tasks of 0.5 vCPU /
 * 4 GiB (the `configured` signal below), so on a 2 vCPU / 16 GiB r7i.large:
 * k = 3 tasks fit, N = ceil(6/3) = 2 instances, achieved occupancy k_eff = 3.0,
 * U = mean(3×0.5/2, 3×4/15) = 0.775 against performance-first's 0.70 target, so
 * `utilisation` = 1 − (0.775−0.70)/0.30 = 0.75. `fit` = 1 − |ln(8/7.26)|/ln 4 =
 * 0.93. `cost` is 0.0317 (the cheapest survivor's cost per task) over this
 * candidate's 0.0336. `total` is the performance-first weighted sum
 * 0.30/0.35/0.15/0.20. A fixture whose numbers cannot be reproduced from the
 * scorer is the same failure as a fixture with the wrong field names, one step
 * harder to see.
 */
function candidate(
	instanceType: string,
	over: Partial<ComputeCandidate> = {},
): ComputeCandidate {
	return {
		instanceType,
		vcpu: 2,
		memoryMiB: 16384,
		architecture: "x86_64",
		scores: { fit: 0.93, utilisation: 0.75, cost: 0.94, modernity: 1 },
		total: 0.8825,
		effectiveHourly: 0.1008,
		tasksPerInstance: 3,
		instancesAtFloor: 2,
		costPerTask: 0.0336,
		spotMedianHourly: 0.0361,
		reason: `${instanceType} holds 3 of the 6 tasks per instance at the workload's 8 GiB per vCPU, filling each one a little past the 70% this posture targets.`,
		...over,
	};
}

function service(
	name: string,
	over: Partial<ComputeServiceSignal> = {},
): ComputeServiceSignal {
	return {
		name,
		datapoints: 336,
		cpuAvg: 21,
		cpuPeak: 64,
		memAvg: 47,
		memPeak: 71,
		status: "ok",
		...over,
	};
}

const SUGGESTED_POOL: ComputeSuggestedPool = {
	name: "general",
	enabled: true,
	instance_types: ["r7i.large"],
	capacity_type: "on_demand",
	on_demand_base: 0,
	min_size: 2,
	max_size: 6,
	target_capacity: 80,
	network_mode: "awsvpc",
	ami_family: "al2023",
	root_volume_gb: 30,
	downgraded: false,
};

function response(
	over: Partial<ComputeRecommendationResponse> = {},
): ComputeRecommendationResponse {
	const primary = candidate("r7i.large");
	return {
		region: "eu-west-1",
		source: "aws_api",
		credentialsState: "ok",
		posture: "performance-first",
		classification: "memory_heavy",
		basis: "measured",
		unsatisfiable: false,
		constraint: null,
		primary,
		ranked: [
			primary,
			// The runner-up is the same shape and the same packing one generation
			// back: identical fit and utilisation, cheaper per task, and it pays
			// for that in modernity. That is a trade a user can actually overrule
			// from knowledge the scorer does not have.
			candidate("r6i.large", {
				scores: { fit: 0.93, utilisation: 0.75, cost: 1, modernity: 0.7 },
				total: 0.8315,
				effectiveHourly: 0.0952,
				costPerTask: 0.0317,
				reason:
					"r6i.large matches the ratio and packs identically for $0.002 less per task, but it is one generation back.",
			}),
			// The wrong-shaped candidate, and the reason costPerTaskSlot had to
			// go: at 8 GiB it holds ONE task, so the pool needs six instances.
			// Priced per slot it looked like the cheap option; priced per task it
			// is nearly three times the cost.
			candidate("m7i.large", {
				memoryMiB: 8192,
				scores: { fit: 0.57, utilisation: 0.56, cost: 0.33, modernity: 1 },
				total: 0.6165,
				effectiveHourly: 0.0954,
				tasksPerInstance: 1,
				instancesAtFloor: 6,
				costPerTask: 0.0954,
				reason:
					"m7i.large is cheaper per hour, but 8 GiB holds one of these tasks, so the pool needs six instances and costs far more per task.",
			}),
		],
		signals: {
			configured: { vcpu: 3, memGiB: 24, ratio: 8, taskCount: 6 },
			actual: { vcpu: 2.6, memGiB: 18.4, ratio: 7.1 },
			coverage: 0.82,
			weights: { configured: 0.18, actual: 0.82 },
			ratio: {
				raw: 7.26,
				effective: 7.26,
				catalogMin: 0.5,
				catalogMax: 32,
				clampedTo: "none",
			},
			cloudwatch: "ok",
			networkMode: "awsvpc",
			trunking: "disabled",
			densityBasis: "max_enis_minus_one",
			dropped: [],
			services: [service("backend"), service("worker")],
		},
		suggestedPool: SUGGESTED_POOL,
		pricingRegion: null,
		notice: null,
		...over,
	};
}

const NO_HISTORY = response({
	basis: "configured",
	classification: "balanced",
	signals: {
		...response().signals,
		actual: null,
		coverage: 0,
		weights: { configured: 1, actual: 0 },
		cloudwatch: "no_data",
		services: [
			service("backend", { datapoints: 0, status: "no_data" }),
			service("worker", { datapoints: 0, status: "no_data" }),
		],
	},
});

const FALLBACK_PRICING = response({
	source: "fallback",
	credentialsState: "missing",
	pricingRegion: "us-east-1",
	notice:
		"The EC2 Pricing API could not be reached; built-in prices were used.",
});

/**
 * `window.fetch` is replaced for this story file only — Vitest gives each test
 * file its own browser context, so the patch cannot reach another file's
 * stories. The fixture is chosen by the `env` query parameter, which is the one
 * request input the harness controls.
 */
const FIXTURES: Record<string, ComputeRecommendationResponse> = {
	measured: response(),
	"no-history": NO_HISTORY,
	fallback: FALLBACK_PRICING,
};

let patched = false;

function patchFetch() {
	if (patched) return;
	patched = true;
	window.fetch = (input: RequestInfo | URL) => {
		const url = typeof input === "string" ? input : input.toString();
		const params = new URL(url, window.location.origin).searchParams;
		const fixture = FIXTURES[params.get("env") ?? ""] ?? response();
		const posture = (params.get("posture") ?? "performance-first") as
			| ComputePosture
			| "cost-first";

		// Cost-first is the one posture whose answer differs enough to be worth
		// modelling: it targets full occupancy rather than performance-first's
		// 70%, buys spot, and stops paying for the current generation. Without
		// this the posture story would assert on a value that never moved.
		const body: ComputeRecommendationResponse =
			posture === "cost-first"
				? {
						...fixture,
						posture: "cost-first",
						suggestedPool: {
							...fixture.suggestedPool,
							instance_types: ["r6i.large"],
							capacity_type: "spot",
							target_capacity: 100,
							min_size: 2,
							max_size: 4,
						},
					}
				: { ...fixture, posture: posture as ComputePosture };

		return Promise.resolve(
			new Response(JSON.stringify(body), {
				status: 200,
				headers: { "Content-Type": "application/json" },
			}),
		);
	};
}

/** Owns `posture` the way ComputePoolEditor does, so the buttons really re-fetch. */
function Harness({ env }: { env: string }) {
	const [posture, setPosture] = useState<ComputePosture>("performance-first");
	const [accepted, setAccepted] = useState<string | null>(null);

	return (
		<div className="w-full max-w-xl">
			<ComputeRecommendation
				env={env}
				pool="general"
				posture={posture}
				onPostureChange={setPosture}
				networkMode="awsvpc"
				amiFamily="al2023"
				onAccept={(suggested) =>
					setAccepted(suggested.instance_types.join(", "))
				}
			/>
			<output data-testid="accepted" className="text-xs text-gray-300">
				{accepted ?? "nothing accepted"}
			</output>
		</div>
	);
}

const meta = {
	title: "Components/ComputeRecommendation",
	component: Harness,
	parameters: { layout: "padded" },
	decorators: [
		(Story) => {
			// Installed during render: the hook fetches on mount, which happens
			// before any play function runs.
			patchFetch();
			return <Story />;
		},
	],
} satisfies Meta<typeof Harness>;

export default meta;
type Story = StoryObj<typeof meta>;

/**
 * The strong case. CloudWatch covers most of the fleet, so the panel is allowed
 * to say "based on measured usage" and to name the blend it used.
 */
export const Measured: Story = {
	args: { env: "measured" },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// Framing: a suggestion, not a verdict. The badge is the gate — it only
		// exists once the response has landed.
		await waitFor(() =>
			expect(canvas.getByText("suggested")).toBeInTheDocument(),
		);
		await expect(
			canvas.getByRole("heading", { name: /suggested starting point/i }),
		).toBeInTheDocument();

		// The basis, and the blend behind it.
		await expect(
			canvas.getByText(/based on measured usage/i),
		).toBeInTheDocument();
		await expect(canvas.getByText(/82% of the fleet/i)).toBeInTheDocument();

		// The runner-up, and the sub-scores it lost and won on — stated in the
		// four dimensions the scorer actually returns.
		await expect(canvas.getByText("Closest alternative")).toBeInTheDocument();
		await expect(canvas.getByText("r6i.large")).toBeInTheDocument();
		await expect(
			canvas.getByText(/loses on modernity \(0\.70 against 1\.00\)/i),
		).toBeInTheDocument();
		await expect(
			canvas.getByText(/wins on cost \(1\.00 against 0\.94\)/i),
		).toBeInTheDocument();
		// fit and utilisation are identical between the two, so the panel must
		// NOT invent a difference there: below SCORE_DELTA_FLOOR they are dropped.
		await expect(canvas.queryByText(/loses on fit/i)).not.toBeInTheDocument();
		// Cost is per task, not per task slot — the slot price was the defect.
		await expect(canvas.getAllByText(/per task$/).length).toBeGreaterThan(0);

		// Sub-scores are reachable but not front-and-centre: present in the DOM,
		// collapsed until asked for. `toBeVisible` is the assertion that matters —
		// <details> keeps its children mounted, so a query-by-label would find the
		// bars whether or not the user can see them.
		const summaries = canvas.getAllByText("Score breakdown");
		await expect(summaries.length).toBeGreaterThan(1);
		const bars = canvas.getAllByLabelText("utilisation score");
		await expect(bars[0]).not.toBeVisible();
		await userEvent.click(summaries[0]);
		await waitFor(() => expect(bars[0]).toBeVisible());

		// One click still accepts.
		await userEvent.click(canvas.getByRole("button", { name: /accept/i }));
		await waitFor(() =>
			expect(canvas.getByTestId("accepted")).toHaveTextContent("r7i.large"),
		);
	},
};

/**
 * The weak case, and the reason for the whole change: no CloudWatch history at
 * all. The panel must say so in words, and must say which way the answer is
 * likely to be wrong — configured requests are habitually generous, so the
 * suggestion skews large.
 */
export const NoCloudWatchHistory: Story = {
	args: { env: "no-history" },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await waitFor(() =>
			expect(
				canvas.getByText(/no measured usage behind this suggestion/i),
			).toBeInTheDocument(),
		);
		await expect(
			canvas.getByText(/no service has enough cloudwatch history yet/i),
		).toBeInTheDocument();
		// The direction of the likely error, not just its existence.
		await expect(
			canvas.getByText(
				/bigger — and dearer — than the workload actually needs/i,
			),
		).toBeInTheDocument();
		await expect(
			canvas.queryByText(/based on measured usage/i),
		).not.toBeInTheDocument();
	},
};

/**
 * C-18: prices came from us-east-1 because the Pricing API was unreachable. The
 * figures must be labelled indicative, and the label must survive next to the
 * per-hour numbers the ranking is drawn from.
 */
export const FallbackPricing: Story = {
	args: { env: "fallback" },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await waitFor(() =>
			expect(
				canvas.getByText(/indicative us-east-1 list prices/i),
			).toBeInTheDocument(),
		);
		await expect(canvas.getByText(/not eu-west-1 prices/i)).toBeInTheDocument();
		await expect(
			canvas.getByText(/prices from us-east-1, indicative only/i),
		).toBeInTheDocument();
		await expect(canvas.getByText(/no AWS credentials/i)).toBeInTheDocument();
	},
};

/**
 * Posture is a choice with visible consequences, not a hidden knob: switching
 * it changes the stated trade-off AND the pool the panel would write.
 */
export const PostureChangesTheAnswer: Story = {
	args: { env: "measured" },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await expect(canvas.getByText(/favours slack/i)).toBeInTheDocument();
		await waitFor(() =>
			expect(
				canvas.getByText(/on-demand · 2–6 instances · 80% target capacity/i),
			).toBeInTheDocument(),
		);

		await userEvent.click(canvas.getByRole("button", { name: /cost first/i }));

		await expect(canvas.getByText(/favours packing/i)).toBeInTheDocument();
		await waitFor(() =>
			expect(
				canvas.getByText(/spot · 2–4 instances · 100% target capacity/i),
			).toBeInTheDocument(),
		);
	},
};
