import {
	BaseEdge,
	EdgeLabelRenderer,
	type EdgeProps,
	getSmoothStepPath,
} from "reactflow";

export function CustomEdge({
	id,
	sourceX,
	sourceY,
	targetX,
	targetY,
	sourcePosition,
	targetPosition,
	label,
	markerEnd,
	markerStart,
	style = {},
}: EdgeProps) {
	const [edgePath, labelX, labelY] = getSmoothStepPath({
		sourceX,
		sourceY,
		sourcePosition,
		targetX,
		targetY,
		targetPosition,
	});

	// Extract stroke color from style to use for label background
	const strokeColor = style.stroke || "#6b7280";
	const isAnimated = (style as any).animated !== false;
	const isDimmed = style.opacity && Number(style.opacity) < 1;

	return (
		<>
			<BaseEdge
				id={id}
				path={edgePath}
				markerEnd={markerEnd}
				markerStart={markerStart}
				style={style}
			/>
			{label && (
				<EdgeLabelRenderer>
					<div
						style={{
							position: "absolute",
							transform: `translate(-50%, -50%) translate(${labelX}px,${labelY}px)`,
							pointerEvents: "all",
						}}
						className="nodrag nopan"
					>
						<div
							className={`
              px-2 py-1 text-xs font-medium rounded-md
              backdrop-blur-sm transition-all duration-200
              ${isDimmed ? "opacity-30" : "opacity-90"}
              ${isAnimated ? "shadow-sm" : ""}
            `}
							style={{
								backgroundColor: `${strokeColor}20`,
								border: `1px solid ${strokeColor}40`,
								color: strokeColor,
								boxShadow:
									isAnimated && !isDimmed
										? `0 0 8px ${strokeColor}30`
										: undefined,
							}}
						>
							<div className="flex items-center gap-1">
								{isAnimated && !isDimmed && (
									<div
										className="w-1.5 h-1.5 rounded-full animate-pulse"
										style={{ backgroundColor: strokeColor }}
									/>
								)}
								<span>{label}</span>
							</div>
						</div>
					</div>
				</EdgeLabelRenderer>
			)}
		</>
	);
}
