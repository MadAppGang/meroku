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
				configProperties: {
					cpu: service.cpu,
					memory: service.memory,
					desiredCount: service.desired_count || 1,
					port: service.container_port,
					xrayEnabled: service.xray_enabled,
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
