import type React from "react";
import { useMemo } from "react";
import { type NodeProps, useNodes, useStore } from "reactflow";

interface GroupNodeData {
  label: string;
  style?: React.CSSProperties;
  nodeIds?: string[]; // IDs of nodes that belong to this group
}

export function DynamicGroupNode({
  data,
  id,
  position = { x: 0, y: 0 },
}: NodeProps<GroupNodeData>) {
  const nodes = useNodes();
  const nodeInternals = useStore((state) => state.nodeInternals);

  // Calculate bounds based on child nodes
  const bounds = useMemo(() => {
    const childNodes = nodes.filter((node) => {
      // Skip group nodes themselves
      if (node.type === "dynamicGroup" || node.type === "group") return false;

      // Include nodes explicitly listed in nodeIds
      if (data.nodeIds?.includes(node.id)) return true;

      // For subgroups (Services, Observability), include nodes with matching subgroup
      if (node.data?.subgroup === data.label) return true;

      // For main groups, include nodes with matching group (but not if they have a subgroup)
      if (node.data?.group === data.label && !node.data?.subgroup) return true;

      return false;
    });

    if (childNodes.length === 0) {
      return { x: 0, y: 0, width: 400, height: 300 };
    }

    // Filter out any nodes that might have invalid positions
    const validNodes = childNodes.filter(
      (node) =>
        node.position &&
        typeof node.position.x === "number" &&
        typeof node.position.y === "number"
    );

    if (validNodes.length === 0) {
      return { x: 0, y: 0, width: 400, height: 300 };
    }

    let minX = Number.POSITIVE_INFINITY;
    let minY = Number.POSITIVE_INFINITY;
    let maxX = Number.NEGATIVE_INFINITY;
    let maxY = Number.NEGATIVE_INFINITY;

    for (const node of validNodes) {
      // Get actual node dimensions from React Flow internals
      const nodeInternal = nodeInternals.get(node.id);
      const nodeWidth = nodeInternal?.width || 140;
      const nodeHeight = nodeInternal?.height || 80;

      minX = Math.min(minX, node.position.x);
      minY = Math.min(minY, node.position.y);
      maxX = Math.max(maxX, node.position.x + nodeWidth);
      maxY = Math.max(maxY, node.position.y + nodeHeight);
    }

    // Add padding
    const sidePadding = 20;
    const topPadding = 35; // Space above nodes for label
    const bottomPadding = 15; // Bottom padding

    const bounds = {
      x: minX - sidePadding,
      y: minY - topPadding,
      width: maxX - minX + sidePadding * 2,
      height: maxY - minY + topPadding + bottomPadding,
    };

    console.log(`Group ${data.label} bounds:`, bounds);

    return bounds;
  }, [nodes, nodeInternals, data.nodeIds, data.label, position]);

  return (
    <div
      style={{
        position: "absolute",
        left: 0,
        top: 0,
        width: bounds.width,
        height: bounds.height,
        borderRadius: 12,
        border: "2px dashed #4b5563",
        backgroundColor: "rgba(75, 85, 99, 0.1)",
        pointerEvents: "none",
        transform: `translate(${bounds.x}px, ${bounds.y}px)`,
        ...data.style,
      }}
    >
      <div
        style={{
          position: "absolute",
          top: 8,
          left: 20,
          backgroundColor: "#0a0a0a",
          padding: "4px 12px",
          borderRadius: 6,
          fontSize: 14,
          color: "#9ca3af",
          fontWeight: 500,
        }}
      >
        {data.label}
      </div>
    </div>
  );
}
