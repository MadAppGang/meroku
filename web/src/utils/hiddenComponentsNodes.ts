import type { Node } from "reactflow";
import type { YamlInfrastructureConfig } from "../types/yamlConfig";

/**
 * Generate nodes for components not shown in the main diagram
 * These can be displayed in a separate area or panel
 */
export function generateHiddenComponentNodes(
	config: YamlInfrastructureConfig | null,
	environment?: string,
): Node[] {
	if (!config) return [];

	const nodes: Node[] = [];
	const baseX = -400;
	const baseY = 1000;
	const spacing = 200;

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
				description: "Message queue",
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
				description: "Shared file storage",
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
				description: "GraphQL API",
				status: "running",
				configProperties: {
					schema: config.pubsub_appsync.schema,
					authLambda: config.pubsub_appsync.auth_lambda,
					resolvers: config.pubsub_appsync.resolvers,
				},
			},
		});
	}

	// Amplify Apps
	if (config.amplify_apps && config.amplify_apps.length > 0) {
		config.amplify_apps.forEach((app, index) => {
			// Get branch info
			const branches = app.branches || [];
			const primaryBranch =
				branches.find((b) => b.stage === "PRODUCTION")?.name ||
				branches[0]?.name ||
				"";
			const branchCount = branches.length;

			nodes.push({
				id: `amplify-${app.name}`,
				type: "service",
				position: { x: -639, y: 158 + index * 120 }, // Position near client apps
				data: {
					id: `amplify-${app.name}`,
					type: "amplify",
					name: app.name,
					description: `Amplify app (${branchCount} branch${branchCount !== 1 ? "es" : ""})`,
					status: "running",
					configProperties: {
						repository: app.github_repository,
						branch: primaryBranch,
						branches: branches,
						customDomain: app.custom_domain,
						environment: environment || config.env || "dev",
					},
				},
			});
		});
	}

	return nodes;
}
