import {
	Activity,
	AlertCircle,
	CheckCircle,
	Clock,
	GitBranch,
	Loader2,
	RefreshCw,
	XCircle,
} from "lucide-react";
import { useCallback, useEffect, useState } from "react";
import { amplifyApi } from "../api";
import type { AmplifyAppInfo } from "../types/amplify";
import type { BuildStatusConfig } from "../types/components";
import { Badge } from "./ui/badge";
import { Button } from "./ui/button";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "./ui/tooltip";
import { cn } from "./ui/utils";

interface AmplifyStatusWidgetProps {
	appName: string;
	environment: string;
	profile?: string;
	variant?: "minimal" | "compact" | "full";
	showRefresh?: boolean;
	autoRefresh?: boolean;
	refreshInterval?: number;
	className?: string;
}

const buildStatusConfig: Record<string, BuildStatusConfig> = {
	SUCCEED: {
		color: "text-green-500",
		bgColor: "bg-green-500/10",
		icon: CheckCircle,
		text: "Success",
		priority: 1,
	},
	FAILED: {
		color: "text-red-500",
		bgColor: "bg-red-500/10",
		icon: XCircle,
		text: "Failed",
		priority: 4,
	},
	RUNNING: {
		color: "text-blue-500",
		bgColor: "bg-blue-500/10",
		icon: Loader2,
		text: "Running",
		pulse: true,
		priority: 3,
	},
	PENDING: {
		color: "text-yellow-500",
		bgColor: "bg-yellow-500/10",
		icon: Clock,
		text: "Pending",
		priority: 2,
	},
	PROVISIONING: {
		color: "text-blue-500",
		bgColor: "bg-blue-500/10",
		icon: Loader2,
		text: "Provisioning",
		pulse: true,
		priority: 3,
	},
	CANCELLING: {
		color: "text-orange-500",
		bgColor: "bg-orange-500/10",
		icon: AlertCircle,
		text: "Cancelling",
		priority: 3,
	},
	CANCELLED: {
		color: "text-gray-500",
		bgColor: "bg-gray-500/10",
		icon: XCircle,
		text: "Cancelled",
		priority: 2,
	},
};

export function AmplifyStatusWidget({
	appName,
	environment,
	profile,
	variant = "compact",
	showRefresh = true,
	autoRefresh = true,
	refreshInterval = 30000,
	className,
}: AmplifyStatusWidgetProps) {
	const [app, setApp] = useState<AmplifyAppInfo | null>(null);
	const [loading, setLoading] = useState(true);
	const [refreshing, setRefreshing] = useState(false);
	const [error, setError] = useState<string | null>(null);

	const fetchAppStatus = useCallback(
		async (isRefresh = false) => {
			if (isRefresh) {
				setRefreshing(true);
			} else {
				setLoading(true);
			}
			setError(null);

			try {
				const response = await amplifyApi.getApps(environment, profile);
				const foundApp = response.apps.find((a) => a.name === appName);
				if (foundApp) {
					setApp(foundApp);
				} else {
					setError("App not found");
				}
			} catch (err) {
				console.error("Failed to fetch Amplify app status:", err);
				// Don't show error for API unavailability, just show static content
				setError(null);
			} finally {
				setLoading(false);
				setRefreshing(false);
			}
		},
		[environment, profile, appName],
	);

	useEffect(() => {
		fetchAppStatus();
		if (autoRefresh) {
			const interval = setInterval(() => fetchAppStatus(true), refreshInterval);
			return () => clearInterval(interval);
		}
	}, [autoRefresh, refreshInterval, fetchAppStatus]);

	const getOverallStatus = () => {
		if (!app || app.branches.length === 0) return null;

		// Find the branch with the highest priority status (most critical)
		const statuses = app.branches.map((b) => ({
			status: b.lastBuildStatus,
			config:
				buildStatusConfig[
					b.lastBuildStatus as keyof typeof buildStatusConfig
				] || buildStatusConfig.PENDING,
			branchName: b.branchName,
		}));

		statuses.sort(
			(a, b) => (b.config.priority || 0) - (a.config.priority || 0),
		);
		return statuses[0];
	};

	if (loading && !app) {
		return (
			<div className={cn("flex items-center gap-2", className)}>
				<Loader2 className="w-4 h-4 animate-spin text-muted-foreground" />
				{variant !== "minimal" && (
					<span className="text-sm text-muted-foreground">
						Checking status...
					</span>
				)}
			</div>
		);
	}

	if (error || !app) {
		// Show a static display when API is not available
		return (
			<div className={cn("flex items-center gap-2", className)}>
				<div className="flex items-center gap-1.5 px-2 py-1 rounded-md bg-gray-500/10">
					<Clock className="w-4 h-4 text-gray-500" />
					<span className="text-sm font-medium text-gray-500">
						Status Unavailable
					</span>
				</div>
			</div>
		);
	}

	const overallStatus = getOverallStatus();
	if (!overallStatus) {
		return (
			<div className={cn("flex items-center gap-2", className)}>
				<div className="flex items-center gap-1.5 px-2 py-1 rounded-md bg-gray-500/10">
					<AlertCircle className="w-4 h-4 text-gray-500" />
					<span className="text-sm font-medium text-gray-500">No branches</span>
				</div>
			</div>
		);
	}

	const StatusIcon = overallStatus.config.icon;
	const isAnimated = overallStatus.config.pulse;

	if (variant === "minimal") {
		return (
			<TooltipProvider>
				<Tooltip>
					<TooltipTrigger asChild>
						<div className={cn("relative", className)}>
							<div
								className={cn(
									"w-3 h-3 rounded-full",
									overallStatus.config.bgColor.replace("/10", ""),
									isAnimated && "animate-pulse",
								)}
							/>
							{isAnimated && (
								<div
									className={cn(
										"absolute inset-0 w-3 h-3 rounded-full animate-ping",
										overallStatus.config.bgColor.replace("/10", "/50"),
									)}
								/>
							)}
						</div>
					</TooltipTrigger>
					<TooltipContent>
						<div className="text-xs">
							<p className="font-medium">{app.name}</p>
							<p className="text-muted-foreground">
								{overallStatus.branchName}: {overallStatus.config.text}
							</p>
						</div>
					</TooltipContent>
				</Tooltip>
			</TooltipProvider>
		);
	}

	const branchStatuses = app.branches.map((branch) => {
		const config =
			buildStatusConfig[
				branch.lastBuildStatus as keyof typeof buildStatusConfig
			] || buildStatusConfig.PENDING;
		return { branch, config };
	});

	if (variant === "compact") {
		return (
			<div className="space-y-2">
				<div className={cn("flex items-center gap-2", className)}>
					<div
						className={cn(
							"flex items-center gap-1.5 px-2 py-1 rounded-md",
							overallStatus.config.bgColor,
						)}
					>
						<StatusIcon
							className={cn(
								"w-4 h-4",
								overallStatus.config.color,
								isAnimated && "animate-spin",
							)}
						/>
						<span
							className={cn("text-sm font-medium", overallStatus.config.color)}
						>
							{overallStatus.config.text}
						</span>
					</div>

					{app.branches.length > 1 && (
						<Badge variant="secondary" className="text-xs">
							<GitBranch className="w-3 h-3 mr-1" />
							{app.branches.length}
						</Badge>
					)}

					{showRefresh && (
						<Button
							size="icon"
							variant="ghost"
							className="h-6 w-6"
							onClick={() => fetchAppStatus(true)}
							disabled={refreshing}
						>
							<RefreshCw
								className={cn("w-3 h-3", refreshing && "animate-spin")}
							/>
						</Button>
					)}
				</div>

				{/* Show default domain */}
				{app.defaultDomain && (
					<div className="text-xs text-gray-400">
						<span className="text-gray-500">Default:</span> {app.defaultDomain}
					</div>
				)}
			</div>
		);
	}

	// Full variant
	return (
		<div className={cn("space-y-3 p-3 border rounded-lg", className)}>
			<div className="flex items-center justify-between">
				<div className="flex items-center gap-2">
					<Activity className="w-4 h-4 text-muted-foreground" />
					<span className="font-medium text-sm">{app.name}</span>
				</div>
				{showRefresh && (
					<Button
						size="icon"
						variant="ghost"
						className="h-6 w-6"
						onClick={() => fetchAppStatus(true)}
						disabled={refreshing}
					>
						<RefreshCw
							className={cn("w-3 h-3", refreshing && "animate-spin")}
						/>
					</Button>
				)}
			</div>

			<div className="space-y-2">
				{branchStatuses.map(({ branch, config }) => {
					const Icon = config.icon;
					const isRunning = config.pulse;

					return (
						<div
							key={branch.branchName}
							className={cn(
								"flex items-center justify-between px-2 py-1.5 rounded text-sm",
								config.bgColor,
							)}
						>
							<div className="flex items-center gap-2">
								<GitBranch className="w-3 h-3 text-muted-foreground" />
								<span className="font-medium">{branch.branchName}</span>
							</div>
							<div className={cn("flex items-center gap-1", config.color)}>
								<Icon className={cn("w-3 h-3", isRunning && "animate-spin")} />
								<span className="text-xs">{config.text}</span>
							</div>
						</div>
					);
				})}
			</div>

			{autoRefresh && (
				<div className="text-xs text-muted-foreground text-center">
					Auto-refreshes every {refreshInterval / 1000}s
				</div>
			)}
		</div>
	);
}
