import type React from "react";
import { useMemo } from "react";
import { type NodeProps, useNodes, useStore } from "reactflow";
import type { ComponentNode } from "../types";

interface GroupNodeData {
  label: string;
  style?: React.CSSProperties;
  nodeIds?: string[]; // IDs of nodes that belong to this group
}

export function DynamicGroupNode({ data }: NodeProps<GroupNodeData>) {
  const nodes = useNodes();
  const nodeInternals = useStore((state) => state.nodeInternals);

  // Calculate bounds based on child nodes
  const bounds = useMemo(() => {
    const childNodes = nodes.filter((node) => {
      // Skip group nodes themselves
      if (node.type === "dynamicGroup" || node.type === "group") return false;

      // Include nodes explicitly listed in nodeIds
      if (data.nodeIds?.includes(node.id)) return true;

      // Cast node.data to ComponentNode to access group/subgroup properties
      const nodeData = node.data as ComponentNode;

      // For subgroups (Services, Observability), include nodes with matching subgroup
      if (nodeData?.subgroup === data.label) return true;

      // For main groups, include nodes with matching group (but not if they have a subgroup)
      if (nodeData?.group === data.label && !nodeData?.subgroup) return true;

      return false;
    });

    if (childNodes.length === 0) {
      return { x: 0, y: 0, width: 400, height: 300 };
    }

    // Filter out any nodes that might have invalid positions or are group nodes
    const validNodes = childNodes.filter(
      (node) =>
        node.position &&
        typeof node.position.x === "number" &&
        typeof node.position.y === "number" &&
        node.type === "service" // Only include service nodes for bounds calculation
    );

    if (validNodes.length === 0) {
      return { x: 0, y: 0, width: 400, height: 300 };
    }

    let minX = Number.POSITIVE_INFINITY;
    let minY = Number.POSITIVE_INFINITY;
    let maxX = Number.NEGATIVE_INFINITY;
    let maxY = Number.NEGATIVE_INFINITY;

    validNodes.forEach((node) => {
      // Get actual node dimensions from React Flow internals
      const nodeInternal = nodeInternals.get(node.id);
      const nodeWidth = nodeInternal?.width || 140;
      const nodeHeight = nodeInternal?.height || 80;

      minX = Math.min(minX, node.position.x);
      minY = Math.min(minY, node.position.y);
      maxX = Math.max(maxX, node.position.x + nodeWidth);
      maxY = Math.max(maxY, node.position.y + nodeHeight);
    });

    // Add padding
    const sidePadding = 20;
    const topPadding = 40; // Space above nodes for label
    const bottomPadding = 20; // Bottom padding

    const bb = {
      x: minX - sidePadding,
      y: minY - topPadding,
      width: maxX - minX + sidePadding * 2,
      height: maxY - minY + topPadding + bottomPadding,
    };
    console.log(bb);
    return {
      x: minX - sidePadding,
      y: minY - topPadding,
      width: maxX - minX + sidePadding * 2,
      height: maxY - minY + topPadding + bottomPadding,
    };
  }, [nodes, nodeInternals, data.nodeIds, data.label]);

  return (
    <div
      style={{
        position: "absolute",
        left: bounds.x,
        top: bounds.y,
        width: bounds.width,
        height: bounds.height,
        borderRadius: 12,
        border: "2px dashed #4b5563",
        backgroundColor: "rgba(75, 85, 99, 0.1)",
        pointerEvents: "none",
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
