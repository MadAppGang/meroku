import {
	Bug,
	Clock,
	Eye,
	EyeOff,
	Globe,
	Grid3x3,
	Maximize,
	Minus,
	MousePointer,
	Move,
	Plus,
	Server,
	Zap,
} from "lucide-react";
import { useEffect, useState } from "react";
import { useReactFlow } from "reactflow";
import { Button } from "./ui/button";

interface CanvasControlsProps {
	showInactive: boolean;
	onToggleInactive: () => void;
	onAddService?: () => void;
	onAddScheduledTask?: () => void;
	onAddEventTask?: () => void;
	onAddAmplify?: () => void;
}

export function CanvasControls({
	showInactive,
	onToggleInactive,
	onAddService,
	onAddScheduledTask,
	onAddEventTask,
	onAddAmplify,
}: CanvasControlsProps) {
	const { zoomIn, zoomOut, fitView, getNodes, getEdges } = useReactFlow();
	const [debugMode, setDebugMode] = useState(false);

	// Initialize debug mode from localStorage
	useEffect(() => {
		const storedDebugMode =
			window.localStorage.getItem("debug_positions") === "true";
		setDebugMode(storedDebugMode);
	}, []);

	const handleDebugPositions = () => {
		// Toggle debug mode
		const newDebugMode = !debugMode;
		window.localStorage.setItem("debug_positions", newDebugMode.toString());
		setDebugMode(newDebugMode);

		console.log(`=== DEBUG MODE: ${newDebugMode ? "ENABLED" : "DISABLED"} ===`);
		console.log(
			"Verbose position logging is now " +
				(newDebugMode ? "enabled" : "disabled"),
		);
		console.log("");

		const nodes = getNodes();
		const edges = getEdges();

		console.log("=== DEBUG: Node Positions ===");
		console.log("Total nodes:", nodes.length);

		// Log all node positions
		nodes.forEach((node) => {
			console.log(`Node: ${node.id}`);
			console.log(`  Position: x=${node.position.x}, y=${node.position.y}`);
			console.log(`  Type: ${node.type}`);
			console.log(`  Data:`, node.data);
		});

		console.log("\n=== DEBUG: Edge Connections ===");
		console.log("Total edges:", edges.length);

		// Log all edge connections
		edges.forEach((edge) => {
			console.log(`Edge: ${edge.id}`);
			console.log(
				`  Source: ${edge.source} (handle: ${edge.sourceHandle || "default"})`,
			);
			console.log(
				`  Target: ${edge.target} (handle: ${edge.targetHandle || "default"})`,
			);
			console.log(`  Type: ${edge.type}`);
			console.log(`  Label: ${edge.label || "none"}`);
		});

		// Create a summary object for easy copying
		const summary = {
			nodes: nodes.map((n) => ({
				id: n.id,
				position: { x: n.position.x, y: n.position.y },
				type: n.type,
			})),
			edges: edges.map((e) => ({
				id: e.id,
				source: e.source,
				target: e.target,
				sourceHandle: e.sourceHandle,
				targetHandle: e.targetHandle,
				label: e.label,
			})),
		};

		console.log("\n=== DEBUG: Summary (JSON) ===");
		console.log(JSON.stringify(summary, null, 2));

		// Helper information
		console.log("\n=== DEBUG: Helper Information ===");
		console.log("To copy a specific node position:");
		console.log(
			'  Right-click on the position object above and select "Copy object"',
		);
		console.log("");
		console.log("To find missing nodes:");
		console.log(
			"  Compare the node IDs above with expected nodes in your configuration",
		);
		console.log("");
		console.log("Debug mode is now:", debugMode ? "ON" : "OFF");
		console.log(
			"When ON, all position save/load operations will be logged in detail",
		);
	};

	return (
		<div className="absolute left-4 top-4 z-40 flex flex-col gap-2">
			{/* Zoom Controls */}
			<div className="bg-gray-800 border border-gray-700 rounded-lg p-1 flex flex-col gap-1">
				<Button
					size="icon"
					variant="ghost"
					onClick={() => zoomIn()}
					className="w-8 h-8 text-gray-400 hover:text-white hover:bg-gray-700"
				>
					<Plus className="w-4 h-4" />
				</Button>
				<Button
					size="icon"
					variant="ghost"
					onClick={() => zoomOut()}
					className="w-8 h-8 text-gray-400 hover:text-white hover:bg-gray-700"
				>
					<Minus className="w-4 h-4" />
				</Button>
				<Button
					size="icon"
					variant="ghost"
					onClick={() => fitView()}
					className="w-8 h-8 text-gray-400 hover:text-white hover:bg-gray-700"
				>
					<Maximize className="w-4 h-4" />
				</Button>
			</div>

			{/* Tool Controls */}
			<div className="bg-gray-800 border border-gray-700 rounded-lg p-1 flex flex-col gap-1">
				<Button
					size="icon"
					variant="ghost"
					className="w-8 h-8 text-gray-400 hover:text-white hover:bg-gray-700"
				>
					<MousePointer className="w-4 h-4" />
				</Button>
				<Button
					size="icon"
					variant="ghost"
					className="w-8 h-8 text-gray-400 hover:text-white hover:bg-gray-700"
				>
					<Move className="w-4 h-4" />
				</Button>
				<Button
					size="icon"
					variant="ghost"
					className="w-8 h-8 text-gray-400 hover:text-white hover:bg-gray-700"
				>
					<Grid3x3 className="w-4 h-4" />
				</Button>
			</div>

			{/* Add Service Controls */}
			<div className="bg-gray-800 border border-gray-700 rounded-lg p-1 flex flex-col gap-1">
				<Button
					size="icon"
					variant="ghost"
					onClick={onAddService}
					className="w-8 h-8 text-gray-400 hover:text-white hover:bg-blue-700"
					title="Add Service"
				>
					<Server className="w-4 h-4" />
				</Button>
				<Button
					size="icon"
					variant="ghost"
					onClick={onAddScheduledTask}
					className="w-8 h-8 text-gray-400 hover:text-white hover:bg-green-700"
					title="Add Scheduled Task"
				>
					<Clock className="w-4 h-4" />
				</Button>
				<Button
					size="icon"
					variant="ghost"
					onClick={onAddEventTask}
					className="w-8 h-8 text-gray-400 hover:text-white hover:bg-purple-700"
					title="Add Event Task"
				>
					<Zap className="w-4 h-4" />
				</Button>
				<Button
					size="icon"
					variant="ghost"
					onClick={onAddAmplify}
					className="w-8 h-8 text-gray-400 hover:text-white hover:bg-orange-700"
					title="Add Amplify App"
				>
					<Globe className="w-4 h-4" />
				</Button>
			</div>

			{/* Toggle Inactive Items */}
			<div className="bg-gray-800 border border-gray-700 rounded-lg p-1">
				<Button
					size="icon"
					variant="ghost"
					onClick={onToggleInactive}
					className="w-8 h-8 text-gray-400 hover:text-white hover:bg-gray-700"
					title={showInactive ? "Hide inactive items" : "Show inactive items"}
				>
					{showInactive ? (
						<Eye className="w-4 h-4" />
					) : (
						<EyeOff className="w-4 h-4" />
					)}
				</Button>
			</div>

			{/* Debug Button */}
			<div className="bg-gray-800 border border-gray-700 rounded-lg p-1">
				<Button
					size="icon"
					variant="ghost"
					onClick={handleDebugPositions}
					className={`w-8 h-8 ${debugMode ? "text-red-400 bg-red-900/50" : "text-gray-400"} hover:text-white hover:bg-red-700`}
					title={
						debugMode
							? "Debug mode is ON - Click to disable and show current positions"
							: "Debug mode is OFF - Click to enable verbose logging and show positions"
					}
				>
					<Bug className="w-4 h-4" />
				</Button>
			</div>
		</div>
	);
}
