import { Activity, AlertTriangle } from "lucide-react";
import type { ComponentNode } from "../types";
import type { YamlInfrastructureConfig } from "../types/yamlConfig";
import { Alert, AlertDescription } from "./ui/alert";
import {
	Card,
	CardContent,
	CardDescription,
	CardHeader,
	CardTitle,
} from "./ui/card";
import { Switch } from "./ui/switch";

interface ServiceXRayConfigurationProps {
	config: YamlInfrastructureConfig;
	onConfigChange: (config: Partial<YamlInfrastructureConfig>) => void;
	node: ComponentNode;
}

export function ServiceXRayConfiguration({
	config,
	onConfigChange,
	node,
}: ServiceXRayConfigurationProps) {
	// Extract service name from node id
	const serviceName = node.id.replace("service-", "");

	// Find the service configuration
	const serviceConfig = config.services?.find(
		(service) => service.name === serviceName,
	);

	const handleServiceChange = (
		updates: Partial<NonNullable<YamlInfrastructureConfig["services"]>[0]>,
	) => {
		if (!config.services) return;

		const updatedServices = config.services.map((service) =>
			service.name === serviceName ? { ...service, ...updates } : service,
		);

		onConfigChange({ services: updatedServices });
	};

	if (!serviceConfig) {
		return (
			<Alert className="border-red-600">
				<AlertTriangle className="h-4 w-4 text-red-600" />
				<AlertDescription>
					Service "{serviceName}" not found in configuration.
				</AlertDescription>
			</Alert>
		);
	}

	return (
		<Card className="w-full">
			<CardHeader>
				<CardTitle className="flex items-center gap-2">
					<Activity className="w-5 h-5" />
					X-Ray Configuration
				</CardTitle>
				<CardDescription>
					Enable AWS X-Ray distributed tracing for {serviceName}
				</CardDescription>
			</CardHeader>
			<CardContent className="space-y-4">
				<div className="flex items-center justify-between">
					<div className="flex-1">
						<h3 className="text-lg font-medium">Enable X-Ray Tracing</h3>
						<p className="text-sm text-gray-500 mt-1">
							Trace requests as they travel through your service
						</p>
					</div>
					<Switch
						checked={serviceConfig.xray_enabled || false}
						onCheckedChange={(checked) =>
							handleServiceChange({ xray_enabled: checked })
						}
						className="data-[state=checked]:bg-blue-500 data-[state=unchecked]:bg-gray-600"
					/>
				</div>

				{serviceConfig.xray_enabled && (
					<Alert>
						<Activity className="h-4 w-4" />
						<AlertDescription>
							X-Ray tracing is enabled. Your service will send trace data to AWS
							X-Ray. Make sure your application is instrumented with the X-Ray
							SDK.
						</AlertDescription>
					</Alert>
				)}
			</CardContent>
		</Card>
	);
}
