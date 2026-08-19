import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React, { useState } from "react";
import { expect, userEvent, waitFor, within } from "storybook/test";
import type {
	ComputeInstanceTypeInfo,
	ComputeInstanceTypesResponse,
} from "../api/infrastructure";
import { useInstanceTypes } from "./use-instance-types";

/**
 * The instance catalog's caching behaviour, driven through a harness because
 * the invariant under test is about REQUESTS, not pixels: a Refresh click must
 * force exactly one cold read, and must not turn every later environment switch
 * into one. The cold path costs ~7s and 6.5MB on the server (a full
 * DescribeInstanceTypes + GetProducts), so "forced" leaking into the session is
 * a performance defect no visual story would catch.
 */

const SAMPLE_TYPE: ComputeInstanceTypeInfo = {
	instanceType: "m7i-flex.large",
	vcpu: 2,
	memoryMiB: 8192,
	architectures: ["x86_64"],
	currentGeneration: true,
	networkPerformance: "Up to 12.5 Gigabit",
	baselineBandwidthMbps: 390,
	maximumNetworkInterfaces: 3,
	gpuCount: 0,
	gpuMemoryMiB: null,
	gpuName: null,
	burstable: false,
	bareMetal: false,
	supportedUsageClasses: ["on-demand", "spot"],
	onDemandHourly: 0.0958,
	priceSource: "aws_api",
};

function catalogFor(env: string): ComputeInstanceTypesResponse {
	return {
		region: "us-east-1",
		source: "aws_api",
		credentialsState: "ok",
		filtered: true,
		totalAvailable: 1,
		cachedAt: null,
		pricingDate: "2026-08-19",
		pricingRegion: null,
		instanceDataDate: "2026-08-19",
		availabilityVerified: true,
		notice: `catalog for ${env}`,
		instanceTypes: [SAMPLE_TYPE],
	};
}

/**
 * Every request the harness caused, in order.
 *
 * `window.fetch` is replaced for this story file only. Vitest gives each test
 * file its own browser context, so the patch cannot reach another file's
 * stories, and Storybook's dev server is never asked for /api anyway.
 */
const requests: string[] = [];
let patched = false;

function patchFetch() {
	if (patched) return;
	patched = true;
	window.fetch = (input: RequestInfo | URL) => {
		const url = typeof input === "string" ? input : input.toString();
		requests.push(url);
		const env = new URL(url, window.location.origin).searchParams.get("env");
		return Promise.resolve(
			new Response(JSON.stringify(catalogFor(env ?? "")), {
				status: 200,
				headers: { "Content-Type": "application/json" },
			}),
		);
	};
}

/**
 * Environment names are unique per page load because `catalogCache` is a
 * module-level Map that outlives any single story. Fixed names would let one
 * run's cache decide the next run's request count.
 */
const RUN = `sb-${Date.now()}`;
const ENV_A = `${RUN}-a`;
const ENV_B = `${RUN}-b`;

function CatalogHarness() {
	const [env, setEnv] = useState(ENV_A);
	const { instanceTypes, loading, error, refresh } = useInstanceTypes(env);

	return (
		<div className="flex flex-col gap-2 text-sm text-gray-200">
			<div className="flex gap-2">
				<button
					type="button"
					className="rounded border border-gray-600 px-2 py-1"
					onClick={() => setEnv(ENV_A)}
				>
					env A
				</button>
				<button
					type="button"
					className="rounded border border-gray-600 px-2 py-1"
					onClick={() => setEnv(ENV_B)}
				>
					env B
				</button>
				<button
					type="button"
					className="rounded border border-gray-600 px-2 py-1"
					onClick={refresh}
				>
					Refresh
				</button>
			</div>
			<output data-testid="status">
				{loading
					? "loading"
					: error
						? `error: ${error}`
						: `${instanceTypes.length} types`}
			</output>
			<output data-testid="requests">{requests.length} requests</output>
		</div>
	);
}

const meta = {
	title: "Hooks/useInstanceTypes",
	component: CatalogHarness,
	parameters: { layout: "centered" },
	decorators: [
		(Story) => {
			// Installed during render, because the hook fetches on mount — which
			// happens before any play function runs.
			patchFetch();
			return <Story />;
		},
	],
} satisfies Meta<typeof CatalogHarness>;

export default meta;
type Story = StoryObj<typeof meta>;

/**
 * Forced applies to the click, not to the session.
 *
 * The regression this pins: a Refresh click used to raise a token that never
 * came down, so every later environment switch skipped the client cache AND
 * sent `refresh=true`, re-paying the cold AWS read on each switch.
 */
export const RefreshForcesOneReadOnly: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const status = canvas.getByTestId("status");

		// 1. First mount: one plain request for env A.
		await waitFor(() => expect(status).toHaveTextContent("1 types"));
		await expect(requests).toHaveLength(1);
		await expect(requests[0]).toContain(`env=${ENV_A}`);
		await expect(requests[0]).not.toContain("refresh=true");

		// 2. Refresh: one forced request, for this environment.
		await userEvent.click(canvas.getByRole("button", { name: "Refresh" }));
		await waitFor(() => expect(requests).toHaveLength(2));
		await expect(requests[1]).toContain(`env=${ENV_A}`);
		await expect(requests[1]).toContain("refresh=true");

		// 3. Switching environments reads normally. Before the fix this request
		//    carried refresh=true and bypassed both server caches.
		await userEvent.click(canvas.getByRole("button", { name: "env B" }));
		await waitFor(() => expect(requests).toHaveLength(3));
		await expect(requests[2]).toContain(`env=${ENV_B}`);
		await expect(requests[2]).not.toContain("refresh=true");

		// 4. Switching back is served from the client cache: no request at all.
		await userEvent.click(canvas.getByRole("button", { name: "env A" }));
		await waitFor(() => expect(status).toHaveTextContent("1 types"));
		await expect(requests).toHaveLength(3);
	},
};
