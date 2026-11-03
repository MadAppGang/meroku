import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React, { useState } from "react";
import type {
	VPCCIDRCandidateResult,
	VPCCIDRReport,
} from "../api/infrastructure";
import { VPCRangeCard } from "./VPCRangeCard";

/**
 * "Network range" card, one story per state.
 *
 * `window.fetch` is replaced for this file only and answers GET /api/vpc-cidr
 * from a scenario keyed by the `env` arg. All ranges and env names are
 * synthetic. The manual-entry stories answer `candidate=` with a tiny fixture
 * table so every field state (keep typing, rule error with a fix, clash,
 * works) can be reached by typing; the real rules live in the Go validator.
 */

const SUGGESTIONS = [
	"10.137.0.0/16",
	"10.42.0.0/16",
	"10.201.0.0/16",
	"10.18.0.0/16",
	"10.77.0.0/16",
];

function report(over: Partial<VPCCIDRReport> = {}): VPCCIDRReport {
	return {
		env: "dev",
		useDefaultVpc: false,
		override: false,
		configured: "10.137.0.0/16",
		effective: "10.137.0.0/16",
		warnings: [],
		suggestions: SUGGESTIONS,
		awsChecked: true,
		deployed: "no",
		deployedReason: "remote state not found",
		...over,
	};
}

const CLASH_STAGING = {
	message: '10.0.0.0/16 overlaps env "staging" (staging.yaml) 10.0.0.0/16',
	hint: "Clashes with staging — you won't be able to connect these two later",
	source: 'env "staging"',
};

type Scenario = {
	value: string;
	/** Mount report; `"loading"` never answers, a string answers 400 with it. */
	answer: VPCCIDRReport | "loading" | string;
};

const SCENARIOS: Record<string, Scenario> = {
	ready: { value: "10.137.0.0/16", answer: report() },
	clash: {
		value: "10.0.0.0/16",
		answer: report({
			configured: "10.0.0.0/16",
			effective: "10.0.0.0/16",
			warnings: [CLASH_STAGING],
		}),
	},
	"invalid-deployed": {
		value: "172.20.0.0/16",
		answer: report({
			configured: "172.20.0.0/16",
			effective: "172.20.0.0/16",
			deployed: "yes",
			deployedReason: "remote state has aws_vpc",
			error: {
				code: "reserved_range",
				message:
					"172.20.0.0/16 is inside 172.16.0.0/12, reserved for Docker and AWS internal networks",
				hint: "This range is used by Docker/AWS internally — pick one of the suggestions",
			},
		}),
	},
	deployed: {
		value: "10.137.0.0/16",
		answer: report({
			deployed: "yes",
			deployedReason: "remote state has aws_vpc",
		}),
	},
	unchecked: {
		value: "10.137.0.0/16",
		answer: report({
			awsChecked: false,
			awsNote:
				"Couldn't reach AWS — these avoid your other meroku environments only",
			deployed: "unknown",
			deployedReason: "remote state unreadable",
		}),
	},
	override: {
		value: "172.20.0.0/16",
		answer: report({
			configured: "172.20.0.0/16",
			effective: "172.20.0.0/16",
			override: true,
			deployed: "yes",
			deployedReason: "remote state has aws_vpc",
			warnings: [
				{
					message: "172.20.0.0/16 overlaps VPC in account",
					hint: "Clashes with another network in this AWS account — you won't be able to connect these two later",
					source: "another network in this AWS account",
				},
			],
		}),
	},
	"pool-exhausted": {
		value: "10.137.0.0/16",
		answer: report({ suggestions: [] }),
	},
	loading: { value: "10.137.0.0/16", answer: "loading" },
	"load-error": {
		value: "10.137.0.0/16",
		answer: 'unknown environment "load-error"',
	},
};

function candidateFixture(value: string): VPCCIDRCandidateResult {
	const [address, size] = value.split("/");
	if (size === undefined || address.split(".").length < 4) {
		return { value, valid: false, incomplete: true, warnings: [] };
	}
	if (value === "10.5.1.0/16") {
		return {
			value,
			valid: false,
			incomplete: false,
			warnings: [],
			error: {
				code: "not_network_address",
				message:
					"10.5.1.0/16 is not a network address (did you mean 10.5.0.0/16?)",
				hint: "Use 10.5.0.0/16 instead — the last two numbers must be 0",
				fix: "10.5.0.0/16",
			},
		};
	}
	if (address.startsWith("172.")) {
		return {
			value,
			valid: false,
			incomplete: false,
			warnings: [],
			error: {
				code: "reserved_range",
				message: `${value} is inside 172.16.0.0/12`,
				hint: "This range is used by Docker/AWS internally — pick one of the suggestions",
			},
		};
	}
	if (Number(size) > 19 || Number(size) < 16) {
		return {
			value,
			valid: false,
			incomplete: false,
			warnings: [],
			error: {
				code: "size",
				message: `${value}: size must be between /16 and /19`,
				hint: "This range is the wrong size for your services — use a number between 16 and 19 at the end",
			},
		};
	}
	if (value === "10.0.0.0/16") {
		return {
			value,
			valid: true,
			incomplete: false,
			warnings: [CLASH_STAGING],
		};
	}
	return { value, valid: true, incomplete: false, warnings: [] };
}

function json(body: unknown, status = 200): Response {
	return new Response(JSON.stringify(body), {
		status,
		headers: { "Content-Type": "application/json" },
	});
}

let realFetch: typeof window.fetch | null = null;

function patchFetch() {
	if (!realFetch) realFetch = window.fetch.bind(window);
	const passthrough = realFetch;
	window.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
		const url = new URL(
			typeof input === "string" ? input : input.toString(),
			window.location.origin,
		);
		if (url.pathname !== "/api/vpc-cidr") return passthrough(input, init);
		const env = url.searchParams.get("env") ?? "";
		const scenario = SCENARIOS[env];
		if (!scenario) return json({ error: `unknown environment "${env}"` }, 400);
		const candidate = url.searchParams.get("candidate");
		if (candidate !== null) {
			const base =
				typeof scenario.answer === "object" ? scenario.answer : report();
			return json({
				...base,
				awsChecked: false,
				candidate: candidateFixture(candidate),
			});
		}
		if (scenario.answer === "loading") return new Promise<Response>(() => {});
		if (typeof scenario.answer === "string") {
			return json({ error: scenario.answer }, 400);
		}
		const withAws = url.searchParams.get("aws") === "1";
		return json({
			...scenario.answer,
			awsChecked: withAws && scenario.answer.awsChecked,
			awsNote: withAws ? scenario.answer.awsNote : undefined,
			deployed: withAws ? scenario.answer.deployed : "unchecked",
		});
	}) as typeof window.fetch;
}

/** Holds the saved value so Apply behaves like the real panel's save. */
function Harness({ env }: { env: string }) {
	const [value, setValue] = useState(SCENARIOS[env]?.value ?? "");
	return (
		<div className="max-w-xl">
			<VPCRangeCard env={env} value={value} onApply={setValue} />
		</div>
	);
}

const meta = {
	title: "Components/VPCRangeCard",
	component: Harness,
	parameters: { layout: "padded" },
	decorators: [
		(Story) => {
			// Installed during render: the card fetches on mount.
			patchFetch();
			return <Story />;
		},
	],
} satisfies Meta<typeof Harness>;

export default meta;
type Story = StoryObj<typeof meta>;

/** Valid, no clashes, AWS checked. "Pick a safe range for me" fills the dropdown. */
export const Ready: Story = { args: { env: "ready" } };

/** The saved range overlaps a sibling env: warning in plain words. */
export const ClashesWithAnotherEnvironment: Story = { args: { env: "clash" } };

/**
 * The saved range breaks a rule on a running env: error, plus the hint to keep
 * it with `allow_vpc_cidr_override` instead of rebuilding.
 */
export const InvalidOnRunningEnvironment: Story = {
	args: { env: "invalid-deployed" },
};

/** Running env: after picking, Apply stays disabled until "I understand" is ticked. */
export const RunningEnvironmentGuard: Story = { args: { env: "deployed" } };

/** AWS unreachable on mount: plain note, guard says "may already be running". */
export const AWSNotChecked: Story = { args: { env: "unchecked" } };

/** `allow_vpc_cidr_override: true`: neutral status, overlap warnings still shown. */
export const ChecksTurnedOff: Story = { args: { env: "override" } };

/** The pool has nothing left to suggest. Click "Pick a safe range for me". */
export const NoSuggestionsLeft: Story = { args: { env: "pool-exhausted" } };

/** Mount request in flight. */
export const Loading: Story = { args: { env: "loading" } };

/** Backend 400: plain message and "Try again". */
export const LoadError: Story = { args: { env: "load-error" } };

/**
 * Manual entry. Open "Advanced: enter a range yourself" and type:
 * `10.4` (keep typing), `10.5.1.0/16` (error + "Use this"), `172.20.0.0/16`
 * (reserved), `10.42.0.0/20` (size), `10.0.0.0/16` (clash), `10.42.0.0/16` (works).
 */
export const ManualEntry: Story = { args: { env: "ready" } };
