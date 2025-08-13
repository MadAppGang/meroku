import type React from "react";
import type { NodeProps } from "reactflow";

interface GroupNodeData {
	label: string;
	style?: React.CSSProperties;
}

export function GroupNode({ data }: NodeProps<GroupNodeData>) {
	return (
		<div
			style={{
				padding: 20,
				borderRadius: 12,
				border: "2px dashed #4b5563",
				backgroundColor: "rgba(75, 85, 99, 0.1)",
				width: 700,
				height: 300,
				...data.style,
			}}
		>
			<div
				style={{
					position: "absolute",
					top: -10,
					left: 20,
					backgroundColor: "#0a0a0a",
					padding: "2px 8px",
					borderRadius: 4,
					fontSize: 12,
					color: "#9ca3af",
					fontWeight: 500,
				}}
			>
				{data.label}
			</div>
		</div>
	);
}
