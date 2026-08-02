import { fetchWithTokenRetry } from "../utils/fetchWithRetry";

/**
 * Client for the /api/dns/* endpoints.
 *
 * Before these existed the DNS subsystem was reachable only from the CLI, so the
 * web UI displayed a hand-written guess at which records would exist — including
 * an `_acme-challenge` CNAME, which is a Let's Encrypt name that ACM never
 * creates. These endpoints return what is actually in Route53 and in public DNS.
 */

export interface DNSStatus {
	environment: string;
	/** skip | normal | bootstrap | blocked | missing-zone */
	plan: string;
	reason: string;

	zone_name: string;
	parent_domain?: string;
	zone_id?: string;

	zone_nameservers?: string[];
	public_nameservers?: string[];

	/** Public DNS points at our zone; certificates can validate. */
	delegated: boolean;
	/** An NS record must be added to the parent zone before deploying. */
	needs_delegation: boolean;
	/** The parent domain is hosted on Route53. */
	parent_is_route53: boolean;
	/** meroku can write the delegation record itself. */
	can_auto_delegate: boolean;
	zone_exists: boolean;
	certificate_ready: boolean;
}

export interface DNSParentCandidate {
	profile: string;
	account_id?: string;
	zone_id?: string;
	nameservers?: string[];
	/**
	 * The zone's nameservers match what the internet returns for the parent
	 * domain, proving it is the live zone rather than a same-named zone in an
	 * unrelated account. Only an authoritative candidate can be delegated to.
	 */
	authoritative: boolean;
	error?: string;
}

export interface DNSDelegateResult {
	written: boolean;
	verified: boolean;
	zone_name: string;
	parent_zone_id: string;
	nameservers: string[];
	message: string;
}

async function getJSON<T>(url: string): Promise<T> {
	const response = await fetchWithTokenRetry(url);
	if (!response.ok) {
		const body = await response.json().catch(() => ({}));
		throw new Error(body.error || `Request failed: ${response.status}`);
	}
	return response.json();
}

export const dnsApi = {
	/** Delegation state of an environment's hosted zone. */
	async getStatus(env: string): Promise<DNSStatus> {
		return getJSON<DNSStatus>(`/api/dns/status?env=${encodeURIComponent(env)}`);
	},

	/**
	 * Local AWS profiles that hold the parent zone. Exactly one should come back
	 * `authoritative: true`; the others are same-named zones elsewhere.
	 */
	async getParentCandidates(env: string): Promise<DNSParentCandidate[]> {
		return getJSON<DNSParentCandidate[]>(
			`/api/dns/parent-candidates?env=${encodeURIComponent(env)}`,
		);
	},

	/**
	 * Write the NS delegation record into the parent zone.
	 *
	 * This changes live DNS, so `confirm` is required by the server and the
	 * caller must have shown the user what will be written.
	 */
	async delegate(env: string, profile: string): Promise<DNSDelegateResult> {
		const response = await fetchWithTokenRetry("/api/dns/delegate", {
			method: "POST",
			headers: { "Content-Type": "application/json" },
			body: JSON.stringify({ env, profile, confirm: true }),
		});
		if (!response.ok) {
			const body = await response.json().catch(() => ({}));
			throw new Error(body.error || `Delegation failed: ${response.status}`);
		}
		return response.json();
	},
};
