import {
	ArrowDown,
	ArrowLeft,
	ArrowRight,
	ArrowUp,
	LogIn,
	LogOut,
	X,
} from "lucide-react";
import { useState } from "react";
import type { Edge } from "reactflow";
import { Button } from "./ui/button";
import { Card } from "./ui/card";

interface EdgeHandleSelectorProps {
	edge: Edge;
	position: { x: number; y: number };
	onClose: () => void;
	onHandleChange: (
		edgeId: string,
		sourceHandle?: string,
		targetHandle?: string,
	) => void;
}

const HANDLE_ICONS = {
	top: ArrowUp,
	right: ArrowRight,
	bottom: ArrowDown,
	left: ArrowLeft,
} as const;

export function EdgeHandleSelector({
	edge,
	position,
	onClose,
	onHandleChange,
}: EdgeHandleSelectorProps) {
	const [sourceHandle, setSourceHandle] = useState(
		edge.sourceHandle || "source-right",
	);
	const [targetHandle, setTargetHandle] = useState(
		edge.targetHandle || "target-left",
	);

	// Apply changes immediately when selection changes
	const handleSourceChange = (handle: string) => {
		setSourceHandle(handle);
		onHandleChange(edge.id, handle, targetHandle);
	};

	const handleTargetChange = (handle: string) => {
		setTargetHandle(handle);
		onHandleChange(edge.id, sourceHandle, handle);
	};

	return (
		<div
			className="absolute z-50 pointer-events-none"
			style={{
				left: `${position.x}px`,
				top: `${position.y}px`,
				transform: "translate(-50%, -100%) translateY(-20px)",
			}}
		>
			<Card className="bg-gray-800/95 backdrop-blur-sm border-gray-600 shadow-xl pointer-events-auto">
				<div className="flex items-center gap-3 p-2">
					{/* Source controls */}
					<div className="flex items-center gap-1">
						<LogOut className="w-3 h-3 text-gray-400" />
						<div className="flex gap-1">
							{(["top", "right", "bottom", "left"] as const).map(
								(direction) => {
									const Icon = HANDLE_ICONS[direction];
									const handleId = `source-${direction}`;
									return (
										<Button
											key={handleId}
											size="sm"
											variant={sourceHandle === handleId ? "default" : "ghost"}
											onClick={() => handleSourceChange(handleId)}
											className="h-7 w-7 p-0"
											title={`Source ${direction}`}
										>
											<Icon className="w-3 h-3" />
										</Button>
									);
								},
							)}
						</div>
					</div>

					<div className="w-px h-6 bg-gray-600" />

					{/* Target controls */}
					<div className="flex items-center gap-1">
						<LogIn className="w-3 h-3 text-gray-400" />
						<div className="flex gap-1">
							{(["top", "right", "bottom", "left"] as const).map(
								(direction) => {
									const Icon = HANDLE_ICONS[direction];
									const handleId = `target-${direction}`;
									return (
										<Button
											key={handleId}
											size="sm"
											variant={targetHandle === handleId ? "default" : "ghost"}
											onClick={() => handleTargetChange(handleId)}
											className="h-7 w-7 p-0"
											title={`Target ${direction}`}
										>
											<Icon className="w-3 h-3" />
										</Button>
									);
								},
							)}
						</div>
					</div>

					<div className="w-px h-6 bg-gray-600" />

					<Button
						size="sm"
						variant="ghost"
						onClick={onClose}
						className="h-7 w-7 p-0"
						title="Close"
					>
						<X className="w-3 h-3" />
					</Button>
				</div>
			</Card>
		</div>
	);
}
