import type { Meta, StoryObj } from "@storybook/react-vite";
// biome-ignore lint/correctness/noUnusedImports: Storybook's browser-test transform requires React in scope for JSX.
import React, { useState } from "react";
import type { SESDNSReport } from "../../api/infrastructure";
import type { SESDomain } from "../../types/yamlConfig";
import { SESSenderRecord } from "./SESSenderRecord";
import { SESTextRecordsEditor } from "./SESTextRecordsEditor";

/**
 * The SES domain DNS editors, one story per zone kind.
 *
 * `window.fetch` is replaced for this file only and answers GET /api/ses-dns
 * from a scenario keyed by the `env` arg. All domains and values are
 * synthetic.
 */

const SCENARIOS: Record<string, SESDNSReport | "loading" | string> = {
	existing: {
		domain: "example.com",
		zone_origin: "existing",
		zone_id: "Z000000000000A",
		live: {
			apex_ttl: 3600,
			spf: ["v=spf1 include:_spf.google.com ~all"],
			apex_other: ["google-site-verification=fake-token-do-not-use"],
		},
		suggested_spf: "v=spf1 include:_spf.google.com include:amazonses.com ~all",
		issues: [],
	},
	created: {
		domain: "example.com",
		zone_origin: "created",
		zone_id: "",
		live: { apex_ttl: 600, spf: [], apex_other: [] },
		suggested_spf: "v=spf1 include:amazonses.com ~all",
		issues: [],
	},
	unreadable: {
		domain: "example.com",
		zone_origin: "existing",
		zone_id: "Z000000000000A",
		live: {
			apex_ttl: null,
			spf: [],
			apex_other: [],
			read_error: "no AWS credentials for profile fake-profile",
		},
		suggested_spf: "v=spf1 include:amazonses.com ~all",
		issues: [],
	},
	"no-zone": {
		domain: "example.com",
		zone_origin: "",
		zone_id: "",
		live: null,
		suggested_spf: "v=spf1 include:amazonses.com ~all",
		issues: [],
	},
	loading: "loading",
};

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
		if (url.pathname !== "/api/ses-dns") return passthrough(input, init);
		const answer = SCENARIOS[url.searchParams.get("env") ?? ""];
		if (answer === undefined)
			return json({ error: "unknown environment" }, 400);
		if (answer === "loading") return new Promise<Response>(() => {});
		if (typeof answer === "string") return json({ error: answer }, 400);
		return json(answer);
	}) as typeof window.fetch;
}

/** Holds the saved domain so the editors behave as in the real panel. */
function Harness({ env, initial }: { env: string; initial: SESDomain }) {
	const [domain, setDomain] = useState<SESDomain>(initial);
	const update = (u: Partial<SESDomain>) => setDomain((d) => ({ ...d, ...u }));
	return (
		<div className="max-w-xl space-y-4">
			<SESSenderRecord
				env={env}
				domain={domain}
				sesDefaults={{}}
				onUpdate={update}
			/>
			<SESTextRecordsEditor
				domain={domain}
				sesDefaults={{}}
				onUpdate={update}
			/>
			<pre className="text-xs text-gray-400">
				{JSON.stringify(domain, null, 2)}
			</pre>
		</div>
	);
}

const meta = {
	title: "Components/SES/DomainRecords",
	component: Harness,
	parameters: { layout: "padded" },
	decorators: [
		(Story) => {
			// Installed during render: the sender card fetches on mount.
			patchFetch();
			return <Story />;
		},
	],
} satisfies Meta<typeof Harness>;

export default meta;
type Story = StoryObj<typeof meta>;

/** A zone meroku did not create, publishing a Google sender record. */
export const ExistingZone: Story = {
	args: { env: "existing", initial: { domain: "example.com" } },
};

/** A zone meroku created: the page is the whole truth. */
export const CreatedZone: Story = {
	args: {
		env: "created",
		initial: {
			domain: "example.com",
			txt_records: [
				{
					name: "google._domainkey",
					values: ["v=DKIM1; k=rsa; p=FAKEKEYDONOTUSE"],
				},
			],
		},
	},
};

/** AWS could not be read: editing still works, with a plain note. */
export const LiveReadFailed: Story = {
	args: { env: "unreadable", initial: { domain: "example.com" } },
};

/** No Route53 zone: records are added by hand. */
export const NoZone: Story = {
	args: { env: "no-zone", initial: { domain: "example.com" } },
};

/** Invalid values stay in the editor and never reach the YAML. */
export const InvalidValues: Story = {
	args: {
		env: "created",
		initial: {
			domain: "example.com",
			txt_records: [{ name: "_dmarc", values: ["v=spf1 -all"] }],
		},
	},
};
