import { ArrowRightLeft, Info, Plus, Trash2 } from "lucide-react";
import { useId } from "react";
import type {
	AmplifyRewriteRule,
	AmplifyRewriteStatus,
	YamlInfrastructureConfig,
} from "../types/yamlConfig";
import { amplifyAppHost, workloadApiHost } from "../utils/envDomain";
import { Alert, AlertDescription } from "./ui/alert";
import { Button } from "./ui/button";
import { Input } from "./ui/input";
import { Label } from "./ui/label";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "./ui/select";
import { StyledSection } from "./ui/styled-section";

const WILDCARD = "<*>";
const BACKEND = "backend";
const KIND_WORKLOAD = "workload";
const KIND_TARGET = "target";

const STATUS_OPTIONS: Array<{ value: AmplifyRewriteStatus; label: string }> = [
	{ value: "200", label: "200 — rewrite (address bar unchanged)" },
	{ value: "301", label: "301 — permanent redirect" },
	{ value: "302", label: "302 — temporary redirect" },
	{ value: "404-200", label: "404-200 — only when the path does not exist" },
];

/**
 * The workloads a rule may proxy to: the backend and every service the API
 * Gateway or ALB routes (network_access public, the default, or full_open).
 */
function routableWorkloads(config: YamlInfrastructureConfig): string[] {
	const services = (config.services ?? [])
		.filter((s) => s.enabled !== false)
		.filter(
			(s) =>
				!s.network_access ||
				s.network_access === "public" ||
				s.network_access === "full_open",
		)
		.map((s) => s.name);
	return [BACKEND, ...services];
}

/**
 * Where a workload rule sends requests, as generate resolves it: the API
 * domain in ALB mode and in API Gateway mode with an API prefix, otherwise the
 * API Gateway endpoint, which is only known after deploy.
 */
function workloadBaseLabel(config: YamlInfrastructureConfig): string {
	const host = workloadApiHost(config);
	return host ? `https://${host}` : "<API Gateway endpoint>";
}

/**
 * The Amplify app whose own host the API host is, if any: generate refuses a
 * workload rule then, because the rewrite would point the app at itself.
 */
function selfHostApp(config: YamlInfrastructureConfig): string | null {
	const apiHost = workloadApiHost(config);
	if (!apiHost) return null;
	const app = (config.amplify_apps ?? []).find(
		(a) => amplifyAppHost(config, a) === apiHost,
	);
	return app ? app.name : null;
}

/** Mirrors amplifyTargetPath in app/amplify_rewrites.go. */
function targetPath(rule: AmplifyRewriteRule): string {
	if (rule.path) return rule.path;
	const workload = rule.workload ?? "";
	if (workload === BACKEND) return rule.source;
	const prefix = `/${workload}`;
	if (rule.source === prefix) return "";
	if (rule.source.startsWith(`${prefix}/`)) {
		return rule.source.slice(prefix.length);
	}
	return rule.source;
}

function ruleErrors(
	rule: AmplifyRewriteRule,
	workloads: string[],
	config: YamlInfrastructureConfig,
): string[] {
	const errors: string[] = [];
	const source = rule.source.trim();
	if (!source) {
		errors.push("Enter the path to match, such as /api/<*>.");
	} else {
		const count = source.split(WILDCARD).length - 1;
		if (count > 1 || (count === 1 && !source.endsWith(WILDCARD))) {
			errors.push(`${WILDCARD} may appear once, at the end of the source.`);
		}
	}
	if (rule.workload !== undefined) {
		if (!rule.workload) {
			errors.push("Pick the workload to send requests to.");
		} else if (!workloads.includes(rule.workload)) {
			errors.push(
				`"${rule.workload}" is not the backend or a public service in this environment.`,
			);
		}
		if (rule.path && !rule.path.startsWith("/")) {
			errors.push("The path on the workload must start with /.");
		}
		if (!rule.path && source && !source.startsWith("/")) {
			errors.push(
				"This source is not a path, so set the path on the workload.",
			);
		}
		if (config.alb?.enabled && !config.domain?.enabled) {
			errors.push(
				"With the ALB on, proxying to a workload needs a domain: turn the domain on first.",
			);
		}
		const loopApp = selfHostApp(config);
		if (loopApp) {
			errors.push(
				`The API host is the host of the Amplify app "${loopApp}", so this rule would send the app to itself. Give the API its own prefix.`,
			);
		}
	} else if (!rule.target?.trim()) {
		errors.push("Enter the destination path or URL.");
	}
	return errors;
}

interface AmplifyRewritesProps {
	config: YamlInfrastructureConfig;
	nodeId: string;
	onConfigChange?: (config: Partial<YamlInfrastructureConfig>) => void;
}

/**
 * The "Rewrites" tab of an Amplify app: amplify_apps[].custom_rules, applied in
 * order before the SPA rule.
 */
export function AmplifyRewrites({
	config,
	nodeId,
	onConfigChange,
}: AmplifyRewritesProps) {
	const uid = useId();
	const appName = nodeId.replace("amplify-", "");
	const appIndex =
		config.amplify_apps?.findIndex((app) => app.name === appName) ?? -1;
	const app = config.amplify_apps?.[appIndex];

	if (!app) {
		return (
			<p className="text-sm text-muted-foreground">
				Amplify app configuration not found.
			</p>
		);
	}

	const rules = app.custom_rules ?? [];
	const workloads = routableWorkloads(config);
	const base = workloadBaseLabel(config);

	const saveRules = (next: AmplifyRewriteRule[]) => {
		if (!onConfigChange || !config.amplify_apps) return;
		const apps = [...config.amplify_apps];
		const { custom_rules: _omit, ...rest } = app;
		apps[appIndex] = next.length > 0 ? { ...rest, custom_rules: next } : rest;
		onConfigChange({ amplify_apps: apps });
	};

	const updateRule = (index: number, rule: AmplifyRewriteRule) => {
		saveRules(rules.map((r, i) => (i === index ? rule : r)));
	};

	const setKind = (index: number, kind: string) => {
		const { source } = rules[index];
		updateRule(
			index,
			kind === KIND_WORKLOAD
				? { source, workload: BACKEND }
				: { source, target: "" },
		);
	};

	const addRule = () => {
		saveRules([...rules, { source: "/api/<*>", workload: BACKEND }]);
	};

	const removeRule = (index: number) => {
		saveRules(rules.filter((_, i) => i !== index));
	};

	const catchAll = rules.findIndex((r) => r.source.trim() === "/<*>");

	return (
		<div className="space-y-4">
			<StyledSection
				title="Rewrites"
				description="Applied in order, before the SPA rule. The first rule that matches wins."
				icon={ArrowRightLeft}
				actions={
					<Button type="button" size="sm" variant="outline" onClick={addRule}>
						<Plus className="w-4 h-4 mr-1" />
						Add rule
					</Button>
				}
			>
				{rules.length === 0 ? (
					<p className="text-sm text-muted-foreground">
						No rules. Add one to send /api requests to the backend from the
						app's own domain, with no CORS setup.
					</p>
				) : (
					<ul className="space-y-4" aria-label="Rewrite rules">
						{rules.map((rule, index) => {
							const isWorkload = rule.workload !== undefined;
							const errors = ruleErrors(rule, workloads, config);
							const id = `${uid}-rule-${index}`;
							const afterCatchAll = catchAll >= 0 && index > catchAll;
							return (
								<li
									// Rules have no identity of their own; their order is their meaning.
									// biome-ignore lint/suspicious/noArrayIndexKey: see above
									key={index}
									className="space-y-3 rounded-lg border border-border p-3"
									data-testid="amplify-rewrite-rule"
								>
									<div className="flex items-end gap-2">
										<div className="flex-1">
											<Label htmlFor={`${id}-source`}>Source</Label>
											<Input
												id={`${id}-source`}
												value={rule.source}
												onChange={(e) =>
													updateRule(index, { ...rule, source: e.target.value })
												}
												placeholder="/api/<*>"
												variant="code"
												className="mt-1"
											/>
										</div>
										<div className="w-40">
											<Label htmlFor={`${id}-kind`}>Send to</Label>
											<Select
												value={isWorkload ? KIND_WORKLOAD : KIND_TARGET}
												onValueChange={(v) => setKind(index, v)}
											>
												<SelectTrigger id={`${id}-kind`} className="mt-1">
													<SelectValue />
												</SelectTrigger>
												<SelectContent>
													<SelectItem value={KIND_WORKLOAD}>
														A workload
													</SelectItem>
													<SelectItem value={KIND_TARGET}>
														A path or URL
													</SelectItem>
												</SelectContent>
											</Select>
										</div>
										<Button
											type="button"
											size="icon"
											variant="ghost"
											onClick={() => removeRule(index)}
											aria-label={`Remove rule ${index + 1}`}
										>
											<Trash2 className="w-4 h-4" />
										</Button>
									</div>

									{isWorkload ? (
										<div className="flex gap-2">
											<div className="flex-1">
												<Label htmlFor={`${id}-workload`}>Workload</Label>
												<Select
													value={rule.workload || undefined}
													onValueChange={(v) =>
														updateRule(index, { ...rule, workload: v })
													}
												>
													<SelectTrigger id={`${id}-workload`} className="mt-1">
														<SelectValue placeholder="Pick a workload" />
													</SelectTrigger>
													<SelectContent>
														{workloads.map((w) => (
															<SelectItem key={w} value={w}>
																{w}
															</SelectItem>
														))}
														{rule.workload &&
															!workloads.includes(rule.workload) && (
																<SelectItem value={rule.workload}>
																	{rule.workload} (not available)
																</SelectItem>
															)}
													</SelectContent>
												</Select>
											</div>
											<div className="flex-1">
												<Label htmlFor={`${id}-path`}>
													Path on the workload
												</Label>
												<Input
													id={`${id}-path`}
													value={rule.path ?? ""}
													onChange={(e) => {
														const { path: _p, ...rest } = rule;
														updateRule(
															index,
															e.target.value
																? { ...rest, path: e.target.value }
																: rest,
														);
													}}
													placeholder="Same as source"
													variant="code"
													className="mt-1"
												/>
											</div>
										</div>
									) : (
										<div className="flex gap-2">
											<div className="flex-1">
												<Label htmlFor={`${id}-target`}>Target</Label>
												<Input
													id={`${id}-target`}
													value={rule.target ?? ""}
													onChange={(e) =>
														updateRule(index, {
															...rule,
															target: e.target.value,
														})
													}
													placeholder="/pricing or https://example.com/<*>"
													variant="code"
													className="mt-1"
												/>
											</div>
											<div className="w-64">
												<Label htmlFor={`${id}-status`}>Status</Label>
												<Select
													value={rule.status ?? "200"}
													onValueChange={(v) => {
														const { status: _s, ...rest } = rule;
														updateRule(
															index,
															v === "200"
																? rest
																: {
																		...rest,
																		status: v as AmplifyRewriteStatus,
																	},
														);
													}}
												>
													<SelectTrigger id={`${id}-status`} className="mt-1">
														<SelectValue />
													</SelectTrigger>
													<SelectContent>
														{STATUS_OPTIONS.map((o) => (
															<SelectItem key={o.value} value={o.value}>
																{o.label}
															</SelectItem>
														))}
													</SelectContent>
												</Select>
											</div>
										</div>
									)}

									{isWorkload && rule.workload && errors.length === 0 && (
										<p className="text-xs text-muted-foreground font-mono">
											→ {base}
											{rule.workload === BACKEND ? "" : `/${rule.workload}`}
											{targetPath(rule)} (status 200)
										</p>
									)}
									{afterCatchAll && (
										<p className="text-xs text-muted-foreground">
											This rule comes after the catch-all /&lt;*&gt; and never
											matches. Move it above.
										</p>
									)}
									{errors.map((e) => (
										<p key={e} className="text-xs text-danger" role="alert">
											{e}
										</p>
									))}
								</li>
							);
						})}
					</ul>
				)}
			</StyledSection>

			<Alert>
				<Info className="w-4 h-4" />
				<AlertDescription>
					A workload rule proxies the request through Amplify, so the browser
					stays on the app's domain. Server-sent events and WebSockets do not
					work through it: call the API domain directly for those.
				</AlertDescription>
			</Alert>
		</div>
	);
}
