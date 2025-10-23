import { format } from "date-fns";
import {
	AlertCircle,
	Calendar,
	CheckCircle,
	ChevronDown,
	ChevronUp,
	Clock,
	Code,
	ExternalLink,
	GitBranch,
	Globe,
	Loader2,
	Play,
	RefreshCw,
	XCircle,
} from "lucide-react";
import { useCallback, useEffect, useState } from "react";
import { amplifyApi } from "../api";
import { useToast } from "../hooks/use-toast";
import type { AmplifyAppInfo } from "../types/amplify";
import type { StatusConfig } from "../types/components";
import { Badge } from "./ui/badge";
import { Button } from "./ui/button";
import {
	Card,
	CardContent,
	CardDescription,
	CardHeader,
	CardTitle,
} from "./ui/card";
import {
	Collapsible,
	CollapsibleContent,
	CollapsibleTrigger,
} from "./ui/collapsible";

interface AmplifyAppsProps {
	environment: string;
	profile?: string;
}

const buildStatusConfig: Record<string, StatusConfig> = {
	SUCCEED: { color: "bg-green-500", icon: CheckCircle, text: "Success" },
	FAILED: { color: "bg-red-500", icon: XCircle, text: "Failed" },
	RUNNING: {
		color: "bg-blue-500",
		icon: Loader2,
		text: "Running",
		pulse: true,
	},
	PENDING: { color: "bg-yellow-500", icon: Clock, text: "Pending" },
	PROVISIONING: { color: "bg-blue-500", icon: Loader2, text: "Provisioning" },
	CANCELLING: { color: "bg-orange-500", icon: AlertCircle, text: "Cancelling" },
	CANCELLED: { color: "bg-gray-500", icon: XCircle, text: "Cancelled" },
};

const stageConfig = {
	PRODUCTION: { color: "bg-red-600", text: "PROD" },
	BETA: { color: "bg-orange-600", text: "BETA" },
	DEVELOPMENT: { color: "bg-blue-600", text: "DEV" },
	EXPERIMENTAL: { color: "bg-purple-600", text: "EXP" },
	PULL_REQUEST: { color: "bg-gray-600", text: "PR" },
};

export function AmplifyApps({ environment, profile }: AmplifyAppsProps) {
	const [apps, setApps] = useState<AmplifyAppInfo[]>([]);
	const [loading, setLoading] = useState(true);
	const [refreshing, setRefreshing] = useState(false);
	const [expandedApps, setExpandedApps] = useState<Set<string>>(new Set());
	const [triggeringBuild, setTriggeringBuild] = useState<string | null>(null);
	const { toast } = useToast();

	const fetchApps = useCallback(
		async (isRefresh = false) => {
			if (isRefresh) {
				setRefreshing(true);
			} else {
				setLoading(true);
			}

			try {
				const response = await amplifyApi.getApps(environment, profile);
				if (response?.apps && Array.isArray(response.apps)) {
					setApps(response.apps);
				} else {
					setApps([]);
					console.warn("No apps data available from API");
				}
			} catch (error) {
				console.error("Failed to fetch Amplify apps:", error);
				toast({
					title: "Error",
					description: "Failed to fetch Amplify apps",
					variant: "destructive",
				});
			} finally {
				setLoading(false);
				setRefreshing(false);
			}
		},
		[environment, profile, toast],
	);

	useEffect(() => {
		fetchApps();
		const interval = setInterval(() => fetchApps(true), 30000);
		return () => clearInterval(interval);
	}, [fetchApps]);

	const toggleAppExpansion = (appId: string) => {
		setExpandedApps((prev) => {
			const next = new Set(prev);
			if (next.has(appId)) {
				next.delete(appId);
			} else {
				next.add(appId);
			}
			return next;
		});
	};

	const triggerBuild = async (appId: string, branchName: string) => {
		setTriggeringBuild(`${appId}-${branchName}`);
		try {
			await amplifyApi.triggerBuild({ appId, branchName, profile });
			toast({
				title: "Build Started",
				description: `Build triggered for branch: ${branchName}`,
			});
			setTimeout(() => fetchApps(true), 2000);
		} catch (error) {
			console.error("Failed to trigger build:", error);
			toast({
				title: "Error",
				description: "Failed to trigger build",
				variant: "destructive",
			});
		} finally {
			setTriggeringBuild(null);
		}
	};

	const formatDuration = (seconds: number) => {
		const mins = Math.floor(seconds / 60);
		const secs = seconds % 60;
		return `${mins}m ${secs}s`;
	};

	const renderBuildStatus = (status: string, isAnimated = false) => {
		const config =
			buildStatusConfig[status as keyof typeof buildStatusConfig] ||
			buildStatusConfig.PENDING;
		const Icon = config.icon;

		return (
			<div
				className={`flex items-center gap-1.5 ${isAnimated && config.pulse ? "animate-pulse" : ""}`}
			>
				<div
					className={`w-2 h-2 rounded-full ${config.color} ${config.pulse ? "animate-pulse" : ""}`}
				/>
				<Icon className={`w-4 h-4 ${config.pulse ? "animate-spin" : ""}`} />
				<span className="text-sm font-medium">{config.text}</span>
			</div>
		);
	};

	if (loading) {
		return (
			<div className="flex items-center justify-center p-8">
				<Loader2 className="w-8 h-8 animate-spin" />
			</div>
		);
	}

	return (
		<div className="space-y-4">
			<div className="flex items-center justify-between mb-6">
				<h2 className="text-2xl font-bold">Amplify Applications</h2>
				<Button
					onClick={() => fetchApps(true)}
					disabled={refreshing}
					size="sm"
					variant="outline"
				>
					<RefreshCw
						className={`w-4 h-4 mr-2 ${refreshing ? "animate-spin" : ""}`}
					/>
					Refresh
				</Button>
			</div>

			{apps.length === 0 ? (
				<Card>
					<CardContent className="flex flex-col items-center justify-center py-12">
						<AlertCircle className="w-12 h-12 text-muted-foreground mb-4" />
						<p className="text-muted-foreground">
							No Amplify applications found
						</p>
					</CardContent>
				</Card>
			) : (
				apps.map((app) => (
					<Card key={app.appId} className="overflow-hidden">
						<CardHeader className="pb-4">
							<div className="flex items-start justify-between">
								<div className="space-y-1">
									<CardTitle className="text-xl">{app.name}</CardTitle>
									<CardDescription className="space-y-1">
										<div className="flex items-center gap-4">
											<span className="flex items-center gap-1">
												<Code className="w-4 h-4" />
												{app.repository}
											</span>
										</div>
										<div className="flex flex-col gap-1">
											{app.defaultDomain && (
												<span className="flex items-center gap-1 text-xs">
													<Globe className="w-3 h-3" />
													<span className="text-gray-500">Default:</span>
													<span className="text-gray-400">
														{app.defaultDomain}
													</span>
												</span>
											)}
											{app.customDomain && (
												<span className="flex items-center gap-1 text-xs">
													<Globe className="w-3 h-3" />
													<span className="text-gray-500">Custom:</span>
													<span className="text-gray-400">
														{app.customDomain}
													</span>
												</span>
											)}
										</div>
									</CardDescription>
								</div>
								<div className="flex items-center gap-2">
									<Button size="sm" variant="ghost" asChild>
										<a
											href={`https://console.aws.amazon.com/amplify/home#/${app.appId}`}
											target="_blank"
											rel="noopener noreferrer"
										>
											<ExternalLink className="w-4 h-4" />
										</a>
									</Button>
								</div>
							</div>
						</CardHeader>

						<CardContent>
							<div className="space-y-3">
								{app.branches.map((branch) => (
									<Collapsible
										key={branch.branchName}
										open={expandedApps.has(`${app.appId}-${branch.branchName}`)}
									>
										<div className="border rounded-lg p-3">
											<CollapsibleTrigger
												onClick={() =>
													toggleAppExpansion(
														`${app.appId}-${branch.branchName}`,
													)
												}
												className="w-full"
											>
												<div className="flex items-center justify-between">
													<div className="flex items-center gap-3">
														<GitBranch className="w-4 h-4 text-muted-foreground" />
														<span className="font-medium">
															{branch.branchName}
														</span>
														{branch.stage && (
															<Badge
																variant="secondary"
																className={`${stageConfig[branch.stage as keyof typeof stageConfig]?.color || "bg-gray-600"} text-white`}
															>
																{stageConfig[
																	branch.stage as keyof typeof stageConfig
																]?.text || branch.stage}
															</Badge>
														)}
													</div>
													<div className="flex items-center gap-3">
														{renderBuildStatus(
															branch.lastBuildStatus || "PENDING",
														)}
														{expandedApps.has(
															`${app.appId}-${branch.branchName}`,
														) ? (
															<ChevronUp className="w-4 h-4" />
														) : (
															<ChevronDown className="w-4 h-4" />
														)}
													</div>
												</div>
											</CollapsibleTrigger>

											<CollapsibleContent>
												<div className="mt-3 pt-3 border-t space-y-3">
													<div className="grid grid-cols-2 gap-4 text-sm">
														<div>
															<p className="text-muted-foreground">
																Last Build
															</p>
															<p className="font-medium flex items-center gap-1">
																<Calendar className="w-3 h-3" />
																{branch.lastBuildTime
																	? format(
																			new Date(branch.lastBuildTime as string),
																			"PPp",
																		)
																	: "Never"}
															</p>
														</div>
														<div>
															<p className="text-muted-foreground">Duration</p>
															<p className="font-medium flex items-center gap-1">
																<Clock className="w-3 h-3" />
																{branch.lastBuildDuration
																	? formatDuration(branch.lastBuildDuration)
																	: "-"}
															</p>
														</div>
													</div>

													{branch.lastCommitMessage && (
														<div className="text-sm">
															<p className="text-muted-foreground">
																Last Commit
															</p>
															<p className="font-mono text-xs mt-1 p-2 bg-muted rounded">
																{branch.lastCommitMessage}
															</p>
														</div>
													)}

													{branch.branchUrl && (
														<div className="text-sm">
															<p className="text-muted-foreground">
																Branch URL
															</p>
															<p className="text-xs mt-1">
																<a
																	href={branch.branchUrl}
																	target="_blank"
																	rel="noopener noreferrer"
																	className="text-blue-400 hover:text-blue-300"
																>
																	{branch.branchUrl}
																</a>
															</p>
														</div>
													)}

													<div className="flex items-center gap-2">
														<Button
															size="sm"
															onClick={() =>
																triggerBuild(app.appId, branch.branchName)
															}
															disabled={
																triggeringBuild ===
																`${app.appId}-${branch.branchName}`
															}
														>
															{triggeringBuild ===
															`${app.appId}-${branch.branchName}` ? (
																<Loader2 className="w-4 h-4 mr-2 animate-spin" />
															) : (
																<Play className="w-4 h-4 mr-2" />
															)}
															Trigger Build
														</Button>
														{branch.branchUrl && (
															<Button size="sm" variant="outline" asChild>
																<a
																	href={branch.branchUrl}
																	target="_blank"
																	rel="noopener noreferrer"
																>
																	<Globe className="w-4 h-4 mr-2" />
																	Visit Site
																</a>
															</Button>
														)}
													</div>
												</div>
											</CollapsibleContent>
										</div>
									</Collapsible>
								))}
							</div>
						</CardContent>
					</Card>
				))
			)}
		</div>
	);
}
