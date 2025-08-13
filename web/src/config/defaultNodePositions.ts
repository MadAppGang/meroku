// Default node positions for initial project layout
export const defaultNodePositions = {
	nodes: [
		{
			id: "github",
			position: {
				x: 280,
				y: -60,
			},
			type: "service",
		},
		{
			id: "client-app",
			position: {
				x: -520,
				y: 490,
			},
			type: "service",
		},
		{
			id: "route53",
			position: {
				x: -140,
				y: 370,
			},
			type: "service",
		},
		{
			id: "api-gateway",
			position: {
				x: -130,
				y: 500,
			},
			type: "service",
		},
		{
			id: "ecs-cluster-group",
			position: {
				x: 0,
				y: 0,
			},
			type: "dynamicGroup",
		},
		{
			id: "ecs-cluster",
			position: {
				x: 284,
				y: 283,
			},
			type: "service",
		},
		{
			id: "ecr",
			position: {
				x: 280,
				y: 110,
			},
			type: "service",
		},
		{
			id: "backend-service",
			position: {
				x: 292,
				y: 459,
			},
			type: "service",
		},
		{
			id: "aurora",
			position: {
				x: 700,
				y: 120,
			},
			type: "service",
		},
		{
			id: "eventbridge",
			position: {
				x: 310,
				y: 740,
			},
			type: "service",
		},
		{
			id: "ses",
			position: {
				x: 710,
				y: 620,
			},
			type: "service",
		},
		{
			id: "sns",
			position: {
				x: 700,
				y: 360,
			},
			type: "service",
		},
		{
			id: "sqs",
			position: {
				x: 700,
				y: 240,
			},
			type: "service",
		},
		{
			id: "s3",
			position: {
				x: 710,
				y: 500,
			},
			type: "service",
		},
	],
	edges: [
		{
			id: "github-ecr",
			source: "github",
			target: "ecr",
			sourceHandle: "source-bottom",
			targetHandle: "target-top",
			label: "push",
		},
		{
			id: "ecr-ecs",
			source: "ecr",
			target: "ecs-cluster",
			sourceHandle: "source-bottom",
			targetHandle: "target-top",
			label: "deploy",
		},
		{
			id: "client-api",
			source: "client-app",
			target: "api-gateway",
			sourceHandle: "source-right",
			targetHandle: "target-left",
			label: "HTTPS",
		},
		{
			id: "api-backend",
			source: "api-gateway",
			target: "backend-service",
			sourceHandle: "source-right",
			targetHandle: "target-left",
			label: "route",
		},
		{
			id: "backend-s3",
			source: "backend-service",
			target: "s3",
			sourceHandle: "source-right",
			targetHandle: "target-left",
			label: "S3",
		},
		{
			id: "backend-eventbridge",
			source: "backend-service",
			target: "eventbridge",
			sourceHandle: "source-bottom",
			targetHandle: "target-top",
			label: "events",
		},
	],
};
