import type { Node } from "reactflow";

interface NodeBounds {
	id: string;
	x: number;
	y: number;
	width: number;
	height: number;
}

// Default sizes for different node types
const NODE_DIMENSIONS = {
	service: { width: 280, height: 120 }, // Actual rendered size
	group: { width: 800, height: 400 }, // Will be overridden by style
};

const MINIMUM_MARGIN = 30; // Reduced for more compact layout

export function preventNodeOverlap(
	nodes: Node[],
	margin: number = MINIMUM_MARGIN,
): Node[] {
	// Create bounds for all nodes
	const nodeBounds: NodeBounds[] = nodes.map((node) => {
		let width = NODE_DIMENSIONS.service.width;
		let height = NODE_DIMENSIONS.service.height;

		if (node.type === "group" && node.style) {
			width = (node.style.width as number) || NODE_DIMENSIONS.group.width;
			height = (node.style.height as number) || NODE_DIMENSIONS.group.height;
		}

		return {
			id: node.id,
			x: node.position.x,
			y: node.position.y,
			width,
			height,
		};
	});

	// Sort nodes by position (prioritize fixing horizontal overlaps)
	nodeBounds.sort((a, b) => {
		// First by Y position (rows)
		const yDiff = a.y - b.y;
		if (Math.abs(yDiff) > 10) {
			return yDiff;
		}
		// Then by X position (columns)
		return a.x - b.x;
	});

	// Check and fix overlaps
	const adjustedNodes = [...nodes];
	const processedBounds: NodeBounds[] = [];

	for (let i = 0; i < nodeBounds.length; i++) {
		const currentBounds = { ...nodeBounds[i] };

		// Check against all previously processed nodes
		let hasOverlap = true;
		let iterations = 0;
		const maxIterations = 100; // Prevent infinite loops

		while (hasOverlap && iterations < maxIterations) {
			hasOverlap = false;
			iterations++;

			for (const processed of processedBounds) {
				const overlap = checkOverlap(currentBounds, processed, margin);

				if (overlap) {
					hasOverlap = true;
					// Move current node to resolve overlap
					const adjustment = calculateAdjustment(
						currentBounds,
						processed,
						margin,
					);
					currentBounds.x += adjustment.x;
					currentBounds.y += adjustment.y;
					break; // Recheck against all nodes
				}
			}
		}

		// Update node position
		const nodeIndex = adjustedNodes.findIndex((n) => n.id === currentBounds.id);
		if (nodeIndex !== -1) {
			adjustedNodes[nodeIndex] = {
				...adjustedNodes[nodeIndex],
				position: {
					x: currentBounds.x,
					y: currentBounds.y,
				},
			};
		}

		processedBounds.push(currentBounds);
	}

	return adjustedNodes;
}

function checkOverlap(
	bounds1: NodeBounds,
	bounds2: NodeBounds,
	margin: number,
): boolean {
	const left1 = bounds1.x;
	const right1 = bounds1.x + bounds1.width;
	const top1 = bounds1.y;
	const bottom1 = bounds1.y + bounds1.height;

	const left2 = bounds2.x;
	const right2 = bounds2.x + bounds2.width;
	const top2 = bounds2.y;
	const bottom2 = bounds2.y + bounds2.height;

	// Check if rectangles overlap including margin
	const horizontalOverlap = left1 < right2 + margin && right1 + margin > left2;
	const verticalOverlap = top1 < bottom2 + margin && bottom1 + margin > top2;

	return horizontalOverlap && verticalOverlap;
}

function calculateAdjustment(
	bounds1: NodeBounds,
	bounds2: NodeBounds,
	margin: number,
): { x: number; y: number } {
	// Calculate how much nodes need to be separated

	const centerX1 = bounds1.x + bounds1.width / 2;
	const centerY1 = bounds1.y + bounds1.height / 2;
	const centerX2 = bounds2.x + bounds2.width / 2;
	const centerY2 = bounds2.y + bounds2.height / 2;

	// Calculate overlap amounts on each side
	const overlapRight = bounds2.x + bounds2.width + margin - bounds1.x;
	const overlapLeft = bounds1.x + bounds1.width + margin - bounds2.x;
	const overlapBottom = bounds2.y + bounds2.height + margin - bounds1.y;
	const overlapTop = bounds1.y + bounds1.height + margin - bounds2.y;

	// Determine best direction to move
	// Prefer horizontal movement for nodes on same row
	if (Math.abs(centerY1 - centerY2) < bounds1.height / 2) {
		// Nodes are roughly on the same horizontal line
		if (centerX1 > centerX2) {
			return { x: overlapRight, y: 0 };
		} else {
			return { x: -overlapLeft, y: 0 };
		}
	}

	// Otherwise, find minimum movement
	const movements = [
		{ x: overlapRight, y: 0, distance: overlapRight },
		{ x: -overlapLeft, y: 0, distance: overlapLeft },
		{ x: 0, y: overlapBottom, distance: overlapBottom },
		{ x: 0, y: -overlapTop, distance: overlapTop },
	];

	// Choose movement with minimum distance
	return movements.reduce((min, current) =>
		current.distance < min.distance ? current : min,
	);
}

// Group-aware layout that maintains nodes within their groups
export function layoutNodesWithGroups(
	nodes: Node[],
	margin: number = MINIMUM_MARGIN,
): Node[] {
	// First, separate groups and regular nodes
	// Dynamic groups should not be moved as they calculate their own position
	const dynamicGroups = nodes.filter((n) => n.type === "dynamicGroup");
	const groups = nodes.filter((n) => n.type === "group");
	const regularNodes = nodes.filter(
		(n) => n.type !== "group" && n.type !== "dynamicGroup",
	);

	// Apply overlap prevention to groups first
	const adjustedGroups = preventNodeOverlap(groups, margin);

	// Then adjust regular nodes, respecting group boundaries
	const adjustedRegularNodes = regularNodes.map((node) => {
		// Check if node belongs to a group
		const nodeGroup = node.data?.group;
		if (nodeGroup) {
			// Find the group bounds
			const group = adjustedGroups.find((g) =>
				g.data?.label?.includes(nodeGroup),
			);
			if (group?.style) {
				const groupX = group.position.x;
				const groupY = group.position.y;
				const groupWidth = (group.style.width as number) || 800;
				const groupHeight = (group.style.height as number) || 400;

				// Ensure node stays within group bounds with padding
				const padding = 20;
				const minX = groupX + padding;
				const maxX =
					groupX + groupWidth - NODE_DIMENSIONS.service.width - padding;
				const minY = groupY + padding + 40; // Extra space for group label
				const maxY =
					groupY + groupHeight - NODE_DIMENSIONS.service.height - padding;

				return {
					...node,
					position: {
						x: Math.max(minX, Math.min(maxX, node.position.x)),
						y: Math.max(minY, Math.min(maxY, node.position.y)),
					},
				};
			}
		}
		return node;
	});

	// Apply overlap prevention to all nodes except dynamic groups
	const allAdjustedNodes = [
		...dynamicGroups, // Keep dynamic groups at their original position
		...adjustedGroups,
		...preventNodeOverlap(adjustedRegularNodes, margin),
	];

	return allAdjustedNodes;
}
