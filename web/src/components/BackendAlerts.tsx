import { AlertCircle, Bell, Clock, TrendingUp, Zap } from "lucide-react";
import type { YamlInfrastructureConfig } from "../types/yamlConfig";
import {
	Card,
	CardContent,
	CardDescription,
	CardHeader,
	CardTitle,
} from "./ui/card";

interface BackendAlertsProps {
	config: YamlInfrastructureConfig;
}

export function BackendAlerts({}: BackendAlertsProps) {
	return (
		<div className="space-y-4">
			<Card>
				<CardHeader>
					<CardTitle className="flex items-center gap-2">
						<Bell className="w-5 h-5" />
						CloudWatch Alarms & Alerts
					</CardTitle>
					<CardDescription>
						Monitor and alert on backend service metrics and events
					</CardDescription>
				</CardHeader>
				<CardContent>
					<div className="flex flex-col items-center justify-center py-16 text-center">
						<div className="relative">
							<Bell className="w-16 h-16 text-gray-600 mb-4" />
							<div className="absolute -top-1 -right-1 w-4 h-4 bg-blue-500 rounded-full animate-pulse" />
						</div>

						<h3 className="text-xl font-semibold text-gray-300 mb-2">
							Coming Soon
						</h3>

						<p className="text-sm text-gray-400 max-w-md mb-6">
							Configure CloudWatch alarms for your backend service including
							CPU, memory, error rates, and custom metrics.
						</p>

						<div className="grid grid-cols-1 md:grid-cols-2 gap-4 w-full max-w-2xl mt-8">
							<div className="border border-gray-700 rounded-lg p-4 bg-gray-800/50">
								<AlertCircle className="w-8 h-8 text-yellow-400 mb-2" />
								<h4 className="text-sm font-medium text-gray-300 mb-1">
									Error Rate Alerts
								</h4>
								<p className="text-xs text-gray-500">
									Get notified when error rates exceed thresholds
								</p>
							</div>

							<div className="border border-gray-700 rounded-lg p-4 bg-gray-800/50">
								<TrendingUp className="w-8 h-8 text-green-400 mb-2" />
								<h4 className="text-sm font-medium text-gray-300 mb-1">
									Performance Alerts
								</h4>
								<p className="text-xs text-gray-500">
									Monitor CPU, memory, and response times
								</p>
							</div>

							<div className="border border-gray-700 rounded-lg p-4 bg-gray-800/50">
								<Zap className="w-8 h-8 text-blue-400 mb-2" />
								<h4 className="text-sm font-medium text-gray-300 mb-1">
									Custom Metrics
								</h4>
								<p className="text-xs text-gray-500">
									Create alerts for application-specific metrics
								</p>
							</div>

							<div className="border border-gray-700 rounded-lg p-4 bg-gray-800/50">
								<Clock className="w-8 h-8 text-purple-400 mb-2" />
								<h4 className="text-sm font-medium text-gray-300 mb-1">
									Scheduled Health Checks
								</h4>
								<p className="text-xs text-gray-500">
									Regular endpoint monitoring and alerting
								</p>
							</div>
						</div>
					</div>
				</CardContent>
			</Card>
		</div>
	);
}
