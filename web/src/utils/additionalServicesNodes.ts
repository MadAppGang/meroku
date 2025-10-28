import type { Node } from "reactflow";
import type { YamlInfrastructureConfig } from "../types/yamlConfig";
import { getAdditionalServices } from "./nodeStateMapping";

/**
 * Generate nodes for additional services (services, scheduled tasks, event tasks)
 */
export function generateAdditionalServiceNodes(
	config: YamlInfrastructureConfig | null,
	baseX = 500,
	baseY = 459,
): Node[] {
	if (!config) return [];

	const { services, scheduledTasks, eventTasks } =
		getAdditionalServices(config);
	const nodes: Node[] = [];
	const offsetX = 150; // Horizontal spacing between services

	// Regular services (to the right of backend service)
	services.forEach((service, index) => {
		nodes.push({
			id: `service-${service.name}`,
			type: "service",
			position: { x: baseX + (index + 1) * offsetX, y: baseY },
			data: {
				id: `service-${service.name}`,
				type: "service",
				name: service.name,
				status: "running",
				group: "ECS Cluster",
				subgroup: "Services",
				description: getServiceDescription(service.name),
				configProperties: {
					cpu: service.cpu,
					memory: service.memory,
					desiredCount: service.desired_count || 1,
					port: service.container_port,
					xrayEnabled: service.xray_enabled,
					serviceType: getServiceType(service.name),
					purpose: getServicePurpose(service.name),
					domain: undefined,
					healthStatus: {
						critical: true,
						monitored: service.xray_enabled || false,
					},
				},
			},
		});
	});

	// Scheduled tasks (below services)
	const schedTaskY = baseY + 120;
	scheduledTasks.forEach((task, index) => {
		nodes.push({
			id: `scheduled-${task.name}`,
			type: "service",
			position: { x: baseX + index * offsetX, y: schedTaskY },
			data: {
				id: `scheduled-${task.name}`,
				type: "scheduled-task",
				name: task.name,
				description: getScheduledTaskDescription(task.name),
				status: "running",
				group: "ECS Cluster",
				subgroup: "Scheduled Tasks",
				configProperties: {
					schedule: task.schedule,
					publicAccess: true, // Always enabled for internet access
					taskCount: 1, // Scheduled tasks run as single instances
					cpu: task.cpu?.toString() || "256",
					memory: task.memory?.toString() || "512",
					serviceType: "Scheduled Task",
					purpose: getTaskPurpose(task.name, "scheduled"),
					healthStatus: {
						critical: false,
						monitored: true,
					},
				},
			},
		});
	});

	// Event processor tasks (below scheduled tasks)
	const eventTaskY = schedTaskY + 120;
	eventTasks.forEach((task, index) => {
		nodes.push({
			id: `event-${task.name}`,
			type: "service",
			position: { x: baseX + index * offsetX, y: eventTaskY },
			data: {
				id: `event-${task.name}`,
				type: "event-task",
				name: task.name,
				description: getEventTaskDescription(task.name),
				status: "running",
				group: "ECS Cluster",
				subgroup: "Event Tasks",
				configProperties: {
					ruleName: task.rule_name,
					detailTypes: task.detail_types,
					sources: task.sources,
					publicAccess: true, // Always enabled for internet access
					taskCount: 1, // Event tasks run as single instances
					cpu: task.cpu?.toString() || "256",
					memory: task.memory?.toString() || "512",
					serviceType: "Event Processor",
					purpose: getTaskPurpose(task.name, "event"),
					healthStatus: {
						critical: true,
						monitored: true,
					},
				},
			},
		});
	});

	return nodes;
}

/**
 * Update ECS cluster group to include dynamic service IDs
 */
export function updateEcsClusterGroup(
	node: Node,
	additionalServiceIds: string[],
): Node {
	if (node.id !== "ecs-cluster-group") return node;

	const baseNodeIds = [
		"ecs-cluster",
		"backend-service",
		"xray",
		"cloudwatch",
		"alarms",
	];
	const allNodeIds = [...baseNodeIds, ...additionalServiceIds];

	return {
		...node,
		data: {
			...node.data,
			nodeIds: allNodeIds,
		},
	};
}

/**
 * Get service type based on service name patterns
 */
function getServiceType(serviceName: string): string {
	const name = serviceName.toLowerCase();
	if (
		name.includes("monitor") ||
		name.includes("metric") ||
		name.includes("log")
	) {
		return "Monitoring";
	}
	if (name.includes("cache") || name.includes("redis")) {
		return "Cache";
	}
	if (name.includes("auth") || name.includes("notif")) {
		return "Auxiliary";
	}
	if (name.includes("api") || name.includes("core") || name.includes("main")) {
		return "Core Service";
	}
	return "Service";
}

/**
 * Get service description based on common patterns
 */
function getServiceDescription(serviceName: string): string {
	const name = serviceName.toLowerCase();
	if (name.includes("api")) return "REST/GraphQL API";
	if (name.includes("auth")) return "User authentication";
	if (name.includes("notification")) return "Push & email alerts";
	if (name.includes("cache")) return "Redis cache layer";
	if (name.includes("monitor")) return "System monitoring";
	if (name.includes("analytics")) return "Data analytics";
	if (name.includes("worker")) return "Async processing";
	if (name.includes("chat") || name.includes("ai")) return "AI chat service";
	if (name.includes("payment")) return "Payment processing";
	if (name.includes("search")) return "Search engine";
	if (name.includes("media")) return "Media processing";
	return "Container service";
}

/**
 * Get service purpose/function description
 */
function getServicePurpose(serviceName: string): string {
	const name = serviceName.toLowerCase();
	if (name.includes("api")) return "Core API backend";
	if (name.includes("auth")) return "Authentication & authorization";
	if (name.includes("notification")) return "Email & push notifications";
	if (name.includes("cache")) return "High-performance caching";
	if (name.includes("monitor")) return "Metrics collection";
	if (name.includes("analytics")) return "Analytics data processing";
	if (name.includes("worker")) return "Asynchronous job processing";
	if (name.includes("chat") || name.includes("ai"))
		return "AI/Chat functionality";
	return "Application service";
}

/**
 * Get task purpose based on task type and name
 */
function getTaskPurpose(
	taskName: string,
	taskType: "scheduled" | "event",
): string {
	const name = taskName.toLowerCase();
	if (taskType === "scheduled") {
		if (name.includes("backup")) return "Periodic data backup";
		if (name.includes("cleanup")) return "Resource cleanup";
		if (name.includes("report")) return "Report generation";
		if (name.includes("sync")) return "Data synchronization";
		if (name.includes("reconcil")) return "Data reconciliation";
		if (name.includes("fee")) return "Fee calculation";
		return "Scheduled job execution";
	} else {
		if (name.includes("order")) return "Order processing";
		if (name.includes("payment")) return "Payment processing";
		if (name.includes("notification")) return "Event notifications";
		if (name.includes("audit")) return "Audit logging";
		return "Event processing";
	}
}

/**
 * Get scheduled task description
 */
function getScheduledTaskDescription(taskName: string): string {
	const name = taskName.toLowerCase();
	if (name.includes("backup")) return "Database backup";
	if (name.includes("cleanup")) return "Cleanup old data";
	if (name.includes("report")) return "Generate reports";
	if (name.includes("sync")) return "Data synchronization";
	if (name.includes("reconcil")) return "Reconcile records";
	if (name.includes("fee")) return "Calculate fees";
	if (name.includes("export")) return "Export data";
	if (name.includes("import")) return "Import data";
	if (name.includes("notification")) return "Send notifications";
	return "Scheduled task";
}

/**
 * Get event task description
 */
function getEventTaskDescription(taskName: string): string {
	const name = taskName.toLowerCase();
	if (name.includes("order")) return "Process orders";
	if (name.includes("payment")) return "Handle payments";
	if (name.includes("notification")) return "Send notifications";
	if (name.includes("audit")) return "Audit events";
	if (name.includes("webhook")) return "Process webhooks";
	if (name.includes("email")) return "Email processor";
	if (name.includes("sms")) return "SMS processor";
	if (name.includes("analytics")) return "Analytics events";
	return "Event processor";
}
