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
					domain: service.public_access ? `${service.name}.${config.api_domain || `api.${config.project}.com`}` : undefined,
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
				name: `Scheduled: ${task.name}`,
				description: task.schedule,
				status: "running",
				group: "ECS Cluster",
				subgroup: "Scheduled Tasks",
				configProperties: {
					schedule: task.schedule,
					publicAccess: task.allow_public_access,
					taskCount: 1, // Scheduled tasks run as single instances
					cpu: task.cpu || '256',
					memory: task.memory || '512',
					serviceType: 'Scheduled Task',
					purpose: getTaskPurpose(task.name, 'scheduled'),
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
				name: `Event: ${task.name}`,
				description: task.rule_name,
				status: "running",
				group: "ECS Cluster",
				subgroup: "Event Tasks",
				configProperties: {
					ruleName: task.rule_name,
					detailTypes: task.detail_types,
					sources: task.sources,
					publicAccess: task.allow_public_access,
					taskCount: 1, // Event tasks run as single instances
					cpu: task.cpu || '256',
					memory: task.memory || '512',
					serviceType: 'Event Processor',
					purpose: getTaskPurpose(task.name, 'event'),
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
	if (name.includes('monitor') || name.includes('metric') || name.includes('log')) {
		return 'Monitoring';
	}
	if (name.includes('cache') || name.includes('redis')) {
		return 'Cache';
	}
	if (name.includes('auth') || name.includes('notif')) {
		return 'Auxiliary';
	}
	if (name.includes('api') || name.includes('core') || name.includes('main')) {
		return 'Core Service';
	}
	return 'Service';
}

/**
 * Get service description based on common patterns
 */
function getServiceDescription(serviceName: string): string {
	const name = serviceName.toLowerCase();
	if (name.includes('api')) return 'API service';
	if (name.includes('auth')) return 'Authentication service';
	if (name.includes('notification')) return 'Notification service';
	if (name.includes('cache')) return 'Caching service';
	if (name.includes('monitor')) return 'Monitoring service';
	if (name.includes('analytics')) return 'Analytics service';
	if (name.includes('worker')) return 'Background worker';
	return 'Service';
}

/**
 * Get service purpose/function description
 */
function getServicePurpose(serviceName: string): string {
	const name = serviceName.toLowerCase();
	if (name.includes('api')) return 'Core API backend';
	if (name.includes('auth')) return 'Authentication & authorization';
	if (name.includes('notification')) return 'Email & push notifications';
	if (name.includes('cache')) return 'High-performance caching';
	if (name.includes('monitor')) return 'Metrics collection';
	if (name.includes('analytics')) return 'Analytics data processing';
	if (name.includes('worker')) return 'Asynchronous job processing';
	if (name.includes('chat') || name.includes('ai')) return 'AI/Chat functionality';
	return 'Application service';
}

/**
 * Get task purpose based on task type and name
 */
function getTaskPurpose(taskName: string, taskType: 'scheduled' | 'event'): string {
	const name = taskName.toLowerCase();
	if (taskType === 'scheduled') {
		if (name.includes('backup')) return 'Periodic data backup';
		if (name.includes('cleanup')) return 'Resource cleanup';
		if (name.includes('report')) return 'Report generation';
		if (name.includes('sync')) return 'Data synchronization';
		if (name.includes('reconcil')) return 'Data reconciliation';
		if (name.includes('fee')) return 'Fee calculation';
		return 'Scheduled job execution';
	} else {
		if (name.includes('order')) return 'Order processing';
		if (name.includes('payment')) return 'Payment processing';
		if (name.includes('notification')) return 'Event notifications';
		if (name.includes('audit')) return 'Audit logging';
		return 'Event processing';
	}
}
