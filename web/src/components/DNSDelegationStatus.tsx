import {
	AlertCircle,
	CheckCircle2,
	Copy,
	Loader2,
	RefreshCw,
	ShieldAlert,
} from "lucide-react";
import { useCallback, useEffect, useState } from "react";
import { type DNSParentCandidate, type DNSStatus, dnsApi } from "../api/dns";
import {
	Card,
	CardContent,
	CardDescription,
	CardHeader,
	CardTitle,
} from "./ui/card";

interface DNSDelegationStatusProps {
	env: string;
}

/**
 * Live delegation status for an environment's hosted zone.
 *
 * This answers the question that used to cost 45 minutes of a stalled apply:
 * does the public internet actually point at our Route53 zone? A certificate
 * cannot be issued until it does, and because the domain module exports its
 * certificate ARNs from `aws_acm_certificate_validation`, an undelegated zone
 * blocks the entire deployment rather than just the domain.
 */
export function DNSDelegationStatus({ env }: DNSDelegationStatusProps) {
	const [status, setStatus] = useState<DNSStatus | null>(null);
	const [loading, setLoading] = useState(true);
	const [error, setError] = useState<string | null>(null);

	const [candidates, setCandidates] = useState<DNSParentCandidate[] | null>(
		null,
	);
	const [scanning, setScanning] = useState(false);
	const [delegating, setDelegating] = useState(false);
	const [result, setResult] = useState<string | null>(null);

	const load = useCallback(async () => {
		setLoading(true);
		setError(null);
		try {
			setStatus(await dnsApi.getStatus(env));
		} catch (e) {
			setError(e instanceof Error ? e.message : String(e));
		} finally {
			setLoading(false);
		}
	}, [env]);

	useEffect(() => {
		void load();
	}, [load]);

	const scan = async () => {
		setScanning(true);
		setError(null);
		try {
			setCandidates(await dnsApi.getParentCandidates(env));
		} catch (e) {
			setError(e instanceof Error ? e.message : String(e));
		} finally {
			setScanning(false);
		}
	};

	const delegate = async (profile: string) => {
		setDelegating(true);
		setError(null);
		setResult(null);
		try {
			const res = await dnsApi.delegate(env, profile);
			setResult(res.message);
			await load();
		} catch (e) {
			setError(e instanceof Error ? e.message : String(e));
		} finally {
			setDelegating(false);
		}
	};

	if (loading) {
		return (
			<Card>
				<CardContent className="flex items-center gap-2 py-6 text-sm text-gray-400">
					<Loader2 className="w-4 h-4 animate-spin" />
					Checking DNS delegation…
				</CardContent>
			</Card>
		);
	}

	if (error && !status) {
		return (
			<Card className="border-red-700 bg-red-900/20">
				<CardContent className="py-6 text-sm text-red-300">
					<div className="flex items-center gap-2">
						<AlertCircle className="w-4 h-4" />
						{error}
					</div>
				</CardContent>
			</Card>
		);
	}

	if (!status) return null;

	const delegated = status.delegated;

	return (
		<Card
			className={
				delegated
					? "border-green-700 bg-green-900/10"
					: "border-yellow-700 bg-yellow-900/10"
			}
		>
			<CardHeader>
				<CardTitle className="flex items-center gap-2 text-base">
					{delegated ? (
						<CheckCircle2 className="w-4 h-4 text-green-400" />
					) : (
						<ShieldAlert className="w-4 h-4 text-yellow-400" />
					)}
					{delegated ? "DNS delegation verified" : "DNS delegation missing"}
					<button
						type="button"
						onClick={() => void load()}
						className="ml-auto text-gray-400 hover:text-gray-200"
						title="Re-check"
					>
						<RefreshCw className="w-3.5 h-3.5" />
					</button>
				</CardTitle>
				<CardDescription>{status.reason}</CardDescription>
			</CardHeader>

			<CardContent className="space-y-4 text-sm">
				<dl className="grid grid-cols-[auto,1fr] gap-x-4 gap-y-1 text-gray-300">
					<dt className="text-gray-500">Zone</dt>
					<dd className="font-mono">{status.zone_name}</dd>
					{status.zone_id && (
						<>
							<dt className="text-gray-500">Zone ID</dt>
							<dd className="font-mono">{status.zone_id}</dd>
						</>
					)}
					{status.parent_domain && (
						<>
							<dt className="text-gray-500">Parent</dt>
							<dd className="font-mono">{status.parent_domain}</dd>
						</>
					)}
					<dt className="text-gray-500">Certificates</dt>
					<dd>
						{status.certificate_ready
							? "can be issued"
							: "blocked until delegation resolves"}
					</dd>
				</dl>

				{/* The real nameservers, read from Route53 — not a guess. */}
				{status.zone_nameservers && status.zone_nameservers.length > 0 && (
					<div className="space-y-1">
						<div className="flex items-center gap-2 text-gray-400">
							<span>
								Add these NS records to{" "}
								<span className="font-mono">{status.parent_domain}</span>
							</span>
							<button
								type="button"
								title="Copy all"
								onClick={() =>
									navigator.clipboard.writeText(
										(status.zone_nameservers ?? []).join("\n"),
									)
								}
								className="text-gray-500 hover:text-gray-300"
							>
								<Copy className="w-3.5 h-3.5" />
							</button>
						</div>
						<ul className="font-mono text-xs bg-black/30 rounded p-2 space-y-0.5">
							{status.zone_nameservers.map((ns) => (
								<li key={ns}>{ns}</li>
							))}
						</ul>
					</div>
				)}

				{!delegated && status.needs_delegation && (
					<div className="space-y-2">
						{status.can_auto_delegate ? (
							<>
								<p className="text-gray-400">
									The parent zone is on Route53. meroku can add the record for
									you, once you say which AWS profile manages it.
								</p>
								{candidates === null ? (
									<button
										type="button"
										onClick={() => void scan()}
										disabled={scanning}
										className="px-3 py-1.5 rounded bg-blue-600 hover:bg-blue-500 disabled:opacity-50 text-white text-xs"
									>
										{scanning ? "Scanning profiles…" : "Find the parent zone"}
									</button>
								) : (
									<ul className="space-y-1">
										{candidates.length === 0 && (
											<li className="text-gray-400">
												No local AWS profile holds a {status.parent_domain}{" "}
												zone. Add the records above manually.
											</li>
										)}
										{candidates.map((c) => (
											<li
												key={c.profile}
												className="flex items-center gap-2 text-xs"
											>
												<span className="font-mono">{c.profile}</span>
												{c.account_id && (
													<span className="text-gray-500">
														account {c.account_id}
													</span>
												)}
												{c.authoritative ? (
													<>
														<span className="text-green-400">
															matches public DNS
														</span>
														<button
															type="button"
															onClick={() => void delegate(c.profile)}
															disabled={delegating}
															className="ml-auto px-2 py-1 rounded bg-blue-600 hover:bg-blue-500 disabled:opacity-50 text-white"
														>
															{delegating ? "Writing…" : "Add NS record"}
														</button>
													</>
												) : (
													/* Not authoritative: writing here would silently
													   do nothing, so no action is offered. */
													<span className="text-yellow-400">
														{c.error ?? "does not match public DNS"}
													</span>
												)}
											</li>
										))}
									</ul>
								)}
							</>
						) : (
							<p className="text-gray-400">
								{status.parent_is_route53
									? "The parent zone is on Route53 but the delegation cannot be automated from here — add the records above manually."
									: `${status.parent_domain} is not hosted on Route53, so the records above must be added wherever it is hosted.`}
							</p>
						)}
					</div>
				)}

				{result && <p className="text-green-400">{result}</p>}
				{error && status && <p className="text-red-400">{error}</p>}
			</CardContent>
		</Card>
	);
}
