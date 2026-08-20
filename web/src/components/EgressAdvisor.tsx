import type { EgressAdvice, EgressStrategy } from "../hooks/use-pricing";
import { Badge } from "./ui/badge";
import { Button } from "./ui/button";

/**
 * Visual advisor for the ECS egress strategy.
 *
 * Public IPv4 costs per task and nothing per GB. A NAT Gateway costs a flat
 * hourly rate and nothing per task. They cross at roughly 5 services, or 10 in
 * production where a NAT runs in each AZ. This component shows where the
 * environment currently sits relative to that crossing.
 *
 * Advisory only. Nothing here changes infrastructure by itself; onSelect just
 * writes egress_strategy to the environment YAML, and the operator still has to
 * plan and apply. See ai_docs/EGRESS_COST_MODEL.md.
 */

interface EgressAdvisorProps {
	advice: EgressAdvice | undefined;
	/** Called when the operator picks a strategy. Omit to render read-only. */
	onSelect?: (strategy: EgressStrategy) => void;
	/** True while a selection is being saved. */
	saving?: boolean;
	/** The default VPC has no private subnets, so NAT options are unavailable. */
	usesDefaultVPC?: boolean;
}

const STRATEGY_LABEL: Record<EgressStrategy, string> = {
	public_ip: "Public IPv4",
	nat_gateway: "Single NAT Gateway",
	nat_gateway_ha: "NAT Gateway per AZ",
};

const STRATEGY_BLURB: Record<EgressStrategy, string> = {
	public_ip:
		"Each task gets its own address. No fixed cost, but the bill grows with every task you add.",
	nat_gateway:
		"Tasks move to private subnets behind one NAT. Flat cost, and no task is reachable from the internet.",
	nat_gateway_ha:
		"One NAT in each AZ. Survives losing a zone and avoids cross-AZ transfer charges.",
};

const STRATEGY_ORDER: EgressStrategy[] = [
	"public_ip",
	"nat_gateway",
	"nat_gateway_ha",
];

function formatMoney(value: number): string {
	if (value === 0) return "$0";
	if (value >= 100) return `$${Math.round(value)}`;
	return `$${value.toFixed(2)}`;
}

export function EgressAdvisor({
	advice,
	onSelect,
	saving = false,
	usesDefaultVPC = false,
}: EgressAdvisorProps) {
	if (!advice) return null;

	const { footprint, current, recommended, shouldSwitch, threshold } = advice;

	// Progress toward the switch threshold. Past it the bar is full rather than
	// overflowing — the message at that point is "you are past it", not "how far".
	const progress = Math.min(1, footprint.Services / Math.max(1, threshold));

	return (
		<div className="rounded-lg border bg-card text-card-foreground">
			<div className="flex items-start justify-between gap-3 border-b p-4">
				<div>
					<h3 className="text-sm font-semibold">Task internet access</h3>
					<p className="text-muted-foreground mt-0.5 text-xs">
						How ECS tasks reach ECR, CloudWatch and third-party APIs
					</p>
				</div>
				<Badge variant={shouldSwitch ? "default" : "secondary"}>
					{shouldSwitch ? "Change suggested" : "Good fit"}
				</Badge>
			</div>

			<div className="space-y-4 p-4">
				{/* Where this environment sits relative to the crossing point. */}
				<div>
					<div className="mb-1.5 flex items-baseline justify-between text-xs">
						<span className="text-muted-foreground">
							{footprint.Services} service
							{footprint.Services === 1 ? "" : "s"} · {footprint.Tasks} task
							{footprint.Tasks === 1 ? "" : "s"}
						</span>
						<span className="text-muted-foreground tabular-nums">
							NAT is cheaper from {threshold}
						</span>
					</div>
					<div
						className="bg-muted h-1.5 w-full overflow-hidden rounded-full"
						role="img"
						aria-label={`${footprint.Services} of ${threshold} services toward the NAT threshold`}
					>
						<div
							className={`h-full rounded-full transition-all ${
								progress >= 1 ? "bg-primary" : "bg-muted-foreground/40"
							}`}
							style={{ width: `${Math.max(3, progress * 100)}%` }}
						/>
					</div>
				</div>

				<p className="text-sm leading-relaxed">{advice.summary}</p>

				{/* The three options, priced for this environment. */}
				<div className="grid gap-2">
					{STRATEGY_ORDER.map((strategy) => {
						const isCurrent = strategy === current;
						const isRecommended = strategy === recommended;
						const blocked = usesDefaultVPC && strategy !== "public_ip";

						// Only the two endpoints of the comparison are priced by the
						// backend. Showing a made-up number for the third would be worse
						// than showing none.
						let price: number | null = null;
						if (isCurrent) price = advice.currentMonthlyCost;
						else if (isRecommended) price = advice.switchedMonthlyCost;

						return (
							<button
								key={strategy}
								type="button"
								disabled={isCurrent || blocked || saving || !onSelect}
								onClick={() => onSelect?.(strategy)}
								className={`flex items-start justify-between gap-3 rounded-md border p-3 text-left transition-colors ${
									isCurrent
										? "border-primary bg-primary/5"
										: blocked
											? "opacity-50"
											: "hover:bg-accent"
								} ${onSelect && !isCurrent && !blocked ? "cursor-pointer" : "cursor-default"}`}
							>
								<div className="min-w-0">
									<div className="flex flex-wrap items-center gap-1.5">
										<span className="text-sm font-medium">
											{STRATEGY_LABEL[strategy]}
										</span>
										{isCurrent && (
											<Badge variant="outline" className="text-[10px]">
												Current
											</Badge>
										)}
										{isRecommended && !isCurrent && (
											<Badge className="text-[10px]">Recommended</Badge>
										)}
									</div>
									<p className="text-muted-foreground mt-0.5 text-xs leading-relaxed">
										{blocked
											? "Needs private subnets. Turn off use_default_vpc to enable."
											: STRATEGY_BLURB[strategy]}
									</p>
								</div>
								{price !== null && (
									<div className="shrink-0 text-right">
										<div className="text-sm font-semibold tabular-nums">
											{formatMoney(price)}
										</div>
										<div className="text-muted-foreground text-[10px]">
											per month
										</div>
									</div>
								)}
							</button>
						);
					})}
				</div>

				{shouldSwitch && advice.monthlySaving > 0 && onSelect && (
					<div className="flex items-center justify-between gap-3 border-t pt-3">
						<p className="text-muted-foreground text-xs">
							Switching replaces the tasks' network configuration, so expect a
							rolling redeploy.
						</p>
						<Button
							size="sm"
							disabled={saving}
							onClick={() => onSelect(recommended)}
						>
							{saving ? "Saving…" : `Switch to ${STRATEGY_LABEL[recommended]}`}
						</Button>
					</div>
				)}
			</div>
		</div>
	);
}
