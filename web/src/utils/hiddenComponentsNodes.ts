import type { Node } from "reactflow";
import type { YamlInfrastructureConfig } from "../types/yamlConfig";

/**
 * Generate nodes for components not shown in the main diagram
 * These can be displayed in a separate area or panel
 */
export function generateHiddenComponentNodes(
	config: YamlInfrastructureConfig | null,
): Node[] {
	if (!config) return [];

	const nodes: Node[] = [];
	const baseX = -400;
	const baseY = 1000;
	const spacing = 200;

	// PostgreSQL (replaces Aurora)
	if (config.postgres?.enabled) {
		nodes.push({
			id: "postgres",
			type: "service",
			position: { x: baseX, y: baseY },
			data: {
				id: "postgres",
				type: "postgres",
				name: "RDS PostgreSQL",
				status: "running",
				configProperties: {
					dbname: config.postgres.dbname,
					username: config.postgres.username,
					engineVersion: config.postgres.engine_version,
					publicAccess: config.postgres.public_access,
					pgAdminEnabled: config.workload?.install_pg_admin,
				},
			},
		});
	}

	// SQS
	if (config.sqs?.enabled) {
		nodes.push({
			id: "sqs",
			type: "service",
			position: { x: baseX + spacing, y: baseY },
			data: {
				id: "sqs",
				type: "sqs",
				name: "Amazon SQS",
				status: "running",
				configProperties: {
					queueName: config.sqs.name,
				},
			},
		});
	}

	// EFS
	if (config.efs && config.efs.length > 0) {
		nodes.push({
			id: "efs",
			type: "service",
			position: { x: baseX + spacing * 2, y: baseY },
			data: {
				id: "efs",
				type: "efs",
				name: "Amazon EFS",
				status: "running",
				configProperties: {
					volumes: config.efs.map((ef) => ({
						name: ef.name,
						path: ef.path,
					})),
					mounts: config.workload?.efs || [],
				},
			},
		});
	}

	// ALB
	if (config.alb?.enabled) {
		nodes.push({
			id: "alb",
			type: "service",
			position: { x: baseX + spacing * 3, y: baseY },
			data: {
				id: "alb",
				type: "alb",
				name: "Application Load Balancer",
				status: "running",
				configProperties: {
					domainName: config.workload?.backend_alb_domain_name,
				},
			},
		});
	}

	// AppSync
	if (config.pubsub_appsync?.enabled) {
		nodes.push({
			id: "appsync",
			type: "service",
			position: { x: baseX + spacing * 4, y: baseY },
			data: {
				id: "appsync",
				type: "appsync",
				name: "AWS AppSync",
				status: "running",
				configProperties: {
					schema: config.pubsub_appsync.schema,
					authLambda: config.pubsub_appsync.auth_lambda,
					resolvers: config.pubsub_appsync.resolvers,
				},
			},
		});
	}

	return nodes;
}
