import {
	Activity,
	AlertCircle,
	ArrowRight,
	BarChart3,
	CheckCircle,
	CheckCircle2,
	ChevronDown,
	Clock,
	Container,
	Copy,
	Database,
	Edit3,
	Eye,
	FileText,
	Globe,
	Grid3X3,
	HardDrive,
	Layers,
	Link2,
	Loader2,
	Mail,
	Maximize2,
	Minimize2,
	Move,
	Package,
	Plus,
	Server,
	Settings,
	Shield,
	Terminal,
	Trash2,
	Users,
	X,
	Zap,
} from "lucide-react";
import React, { useState, useRef, useEffect } from "react";

const DeploymentPlatformUI = () => {
	const [nodes, setNodes] = useState([
		{
			id: "1",
			x: 100,
			y: 200,
			label: "Backend",
			type: "backend",
			status: "running",
			lastDeployed: "just now",
			project: "myapp-backend",
			framework: "Python",
			version: "3.9",
			region: "us-east-1",
			environment: "production",
			scaling: "auto",
			memory: "1GB",
			timeout: "30s",
			monitoring: true,
			replicas: 3,
			minReplicas: 1,
			maxReplicas: 10,
		},
		{
			id: "2",
			x: 450,
			y: 100,
			label: "Frontend",
			type: "frontend",
			status: "running",
			lastDeployed: "2 hours ago",
			project: "myapp-frontend",
			framework: "Next.js",
			version: "14.0.0",
			region: "us-east-1",
			environment: "production",
			features: {
				ssr: true,
				analytics: true,
			},
			performance: 98,
			replicas: 1,
		},
		{
			id: "3",
			x: 450,
			y: 300,
			label: "PostgreSQL",
			type: "database",
			status: "running",
			lastDeployed: "1 week ago",
			project: "pg-data",
			version: "15.3",
			region: "us-east-1",
			environment: "production",
			storage: "100GB",
			connections: 50,
			replicas: 2,
			backup: true,
		},
	]);

	const [connections, setConnections] = useState([
		{ from: "1", to: "2" },
		{ from: "2", to: "3" },
		{ from: "2", to: "4" },
	]);

	const [selectedNode, setSelectedNode] = useState(null);
	const [showActivityPanel, setShowActivityPanel] = useState(false);
	const [showAddComponentModal, setShowAddComponentModal] = useState(false);
	const [activeTab, setActiveTab] = useState("settings");
	const [draggingNode, setDraggingNode] = useState(null);
	const [canvasOffset, setCanvasOffset] = useState({ x: 0, y: 0 });
	const [zoom, setZoom] = useState(1);

	const canvasRef = useRef(null);
	const [canvasSize, setCanvasSize] = useState({ width: 800, height: 600 });

	useEffect(() => {
		const updateCanvasSize = () => {
			if (canvasRef.current) {
				const rect = canvasRef.current.getBoundingClientRect();
				setCanvasSize({ width: rect.width, height: rect.height });
			}
		};
		updateCanvasSize();
		window.addEventListener("resize", updateCanvasSize);
		return () => window.removeEventListener("resize", updateCanvasSize);
	}, []);

	const getStatusColor = (status) => {
		switch (status) {
			case "running":
				return "#10b981";
			case "deploying":
				return "#3b82f6";
			case "stopped":
				return "#6b7280";
			case "error":
				return "#ef4444";
			default:
				return "#6b7280";
		}
	};

	const getServiceIcon = (type) => {
		switch (type) {
			case "backend":
				return Server;
			case "frontend":
				return Layers;
			case "database":
				return Database;
			case "redis":
				return HardDrive;
			case "cognito":
				return Shield;
			case "ses":
				return Mail;
			case "sqs":
				return Container;
			default:
				return Zap;
		}
	};

	const getServiceColor = (type) => {
		switch (type) {
			case "backend":
				return "#3b82f6";
			case "frontend":
				return "#10b981";
			case "database":
				return "#8b5cf6";
			case "redis":
				return "#ef4444";
			case "cognito":
				return "#f59e0b";
			case "ses":
				return "#06b6d4";
			case "sqs":
				return "#ec4899";
			default:
				return "#6b7280";
		}
	};

	const componentTypes = [
		{ type: "backend", label: "Backend Service", icon: Server },
		{ type: "frontend", label: "Frontend Service", icon: Layers },
		{ type: "database", label: "PostgreSQL", icon: Database },
		{ type: "redis", label: "Redis Cache", icon: HardDrive },
		{ type: "cognito", label: "Cognito Auth", icon: Shield },
		{ type: "ses", label: "SES Email", icon: Mail },
		{ type: "sqs", label: "SQS Queue", icon: Container },
	];

	const handleMouseDown = (e, nodeId) => {
		if (e.button === 0) {
			// Left click
			setDraggingNode(nodeId);
			setSelectedNode(nodes.find((n) => n.id === nodeId));
		}
	};

	const handleMouseMove = (e) => {
		if (draggingNode && canvasRef.current) {
			const rect = canvasRef.current.getBoundingClientRect();
			const x = (e.clientX - rect.left - canvasOffset.x) / zoom;
			const y = (e.clientY - rect.top - canvasOffset.y) / zoom;

			setNodes(
				nodes.map((node) =>
					node.id === draggingNode ? { ...node, x, y } : node,
				),
			);
		}
	};

	const handleMouseUp = () => {
		setDraggingNode(null);
	};

	const handleCanvasMouseDown = (e) => {
		if (e.target === e.currentTarget) {
			setSelectedNode(null);
		}
	};

	const addComponent = (type) => {
		const componentType = componentTypes.find((c) => c.type === type);
		const newNode = {
			id: `${Date.now()}`,
			x: 300 + Math.random() * 200,
			y: 200 + Math.random() * 200,
			label: componentType?.label || "New Service",
			type: type,
			status: "deploying",
			lastDeployed: "Just now",
			project: `${type}-${Date.now()}`,
			region: "us-east-1",
			environment: "production",
			replicas: 1,
		};
		setNodes([...nodes, newNode]);
		setShowAddComponentModal(false);
	};

	const deleteNode = (nodeId) => {
		setNodes(nodes.filter((n) => n.id !== nodeId));
		setConnections(
			connections.filter((c) => c.from !== nodeId && c.to !== nodeId),
		);
		setSelectedNode(null);
	};

	const handleZoom = (delta) => {
		setZoom((prev) => Math.max(0.5, Math.min(2, prev + delta)));
	};

	const updateReplicas = (nodeId, change) => {
		setNodes(
			nodes.map((node) => {
				if (node.id === nodeId) {
					const newReplicas = Math.max(
						1,
						Math.min(node.maxReplicas || 10, (node.replicas || 1) + change),
					);
					return { ...node, replicas: newReplicas };
				}
				return node;
			}),
		);
	};

	return (
		<div className="h-screen bg-gray-900 flex flex-col">
			{/* Header */}
			<header className="bg-gray-800 border-b border-gray-700 px-6 py-4">
				<div className="flex items-center justify-between">
					<div className="flex items-center gap-4">
						<div className="flex items-center gap-2">
							<Zap className="w-6 h-6 text-purple-500" />
							<h1 className="text-xl font-semibold text-white">
								Deployment Platform
							</h1>
						</div>
						<div className="flex items-center gap-2 bg-gray-700 px-3 py-1 rounded-lg">
							<span className="text-sm text-gray-300">Environment:</span>
							<select className="bg-transparent text-white text-sm outline-none font-medium">
								<option>production</option>
								<option>staging</option>
								<option>development</option>
							</select>
						</div>
					</div>
					<div className="flex items-center gap-4">
						<button className="text-gray-400 hover:text-white font-medium">
							Architecture
						</button>
						<button className="text-gray-400 hover:text-white font-medium">
							Observability
						</button>
						<button className="text-gray-400 hover:text-white font-medium">
							Logs
						</button>
						<button className="text-gray-400 hover:text-white font-medium">
							Settings
						</button>
						<button className="bg-purple-600 hover:bg-purple-700 text-white px-4 py-2 rounded-lg text-sm flex items-center gap-2 font-medium">
							<Plus className="w-4 h-4" />
							Deploy
						</button>
					</div>
				</div>
			</header>

			<div className="flex-1 flex relative">
				{/* Left Sidebar */}
				<div className="w-16 bg-gray-800 border-r border-gray-700 flex flex-col items-center py-4 gap-4">
					<button className="p-2 text-gray-400 hover:text-white hover:bg-gray-700 rounded">
						<Grid3X3 className="w-5 h-5" />
					</button>
					<button
						onClick={() => setShowAddComponentModal(true)}
						className="p-2 text-gray-400 hover:text-white hover:bg-gray-700 rounded"
					>
						<Plus className="w-5 h-5" />
					</button>
					<button
						onClick={() => handleZoom(-0.1)}
						className="p-2 text-gray-400 hover:text-white hover:bg-gray-700 rounded"
					>
						<Minimize2 className="w-5 h-5" />
					</button>
					<button
						onClick={() => handleZoom(0.1)}
						className="p-2 text-gray-400 hover:text-white hover:bg-gray-700 rounded"
					>
						<Maximize2 className="w-5 h-5" />
					</button>
				</div>

				{/* Main Canvas */}
				<div className="flex-1 relative overflow-hidden bg-gray-900">
					<svg
						ref={canvasRef}
						className="w-full h-full"
						onMouseMove={handleMouseMove}
						onMouseUp={handleMouseUp}
						onMouseDown={handleCanvasMouseDown}
						style={{ cursor: draggingNode ? "grabbing" : "default" }}
					>
						{/* Grid Pattern */}
						<defs>
							<pattern
								id="grid"
								width="20"
								height="20"
								patternUnits="userSpaceOnUse"
							>
								<circle cx="1" cy="1" r="0.5" fill="#374151" />
							</pattern>
						</defs>
						<rect width="100%" height="100%" fill="url(#grid)" />

						<g
							transform={`translate(${canvasOffset.x}, ${canvasOffset.y}) scale(${zoom})`}
						>
							{/* Connections */}
							{connections.map((conn, idx) => {
								const fromNode = nodes.find((n) => n.id === conn.from);
								const toNode = nodes.find((n) => n.id === conn.to);
								if (!fromNode || !toNode) return null;

								const fromY = fromNode.y + 80;
								const toY = toNode.y + 80;

								return (
									<g key={idx}>
										<line
											x1={fromNode.x + 125}
											y1={fromY}
											x2={toNode.x + 125}
											y2={toY}
											stroke="#4b5563"
											strokeWidth="2"
											strokeDasharray="5,5"
										/>
									</g>
								);
							})}

							{/* Nodes */}
							{nodes.map((node) => {
								const Icon = getServiceIcon(node.type);
								const isSelected = selectedNode?.id === node.id;
								const nodeColor = getServiceColor(node.type);

								return (
									<g
										key={node.id}
										transform={`translate(${node.x}, ${node.y})`}
										onMouseDown={(e) => handleMouseDown(e, node.id)}
										style={{ cursor: "pointer" }}
									>
										{/* Card Shadow */}
										<rect
											width="250"
											height="160"
											rx="12"
											fill="rgba(0,0,0,0.1)"
											x="2"
											y="2"
										/>
										{/* Card Background */}
										<rect
											width="250"
											height="160"
											rx="12"
											fill="#1e1e2e"
											stroke={isSelected ? nodeColor : "#2a2a3e"}
											strokeWidth={isSelected ? "2" : "1"}
										/>

										{/* Header with Icon and Name */}
										<g transform="translate(20, 25)">
											{node.type === "backend" ? (
												<g>
													<rect
														x="0"
														y="0"
														width="12"
														height="12"
														rx="2"
														fill="#3776ab"
													/>
													<rect
														x="12"
														y="0"
														width="12"
														height="12"
														rx="2"
														fill="#ffd43b"
													/>
													<rect
														x="0"
														y="12"
														width="12"
														height="12"
														rx="2"
														fill="#ffd43b"
													/>
													<rect
														x="12"
														y="12"
														width="12"
														height="12"
														rx="2"
														fill="#3776ab"
													/>
												</g>
											) : (
												<Icon
													x="0"
													y="0"
													width="24"
													height="24"
													color={nodeColor}
												/>
											)}
											<text
												x="35"
												y="17"
												fill="#ffffff"
												fontSize="16"
												fontWeight="500"
											>
												{node.label}
											</text>
										</g>

										{/* Status and Deployment info */}
										<g transform="translate(20, 70)">
											<CheckCircle
												x="0"
												y="0"
												width="20"
												height="20"
												color="#10b981"
											/>
											<text x="30" y="15" fill="#9ca3af" fontSize="14">
												Deployed {node.lastDeployed}
											</text>
										</g>

										{/* Replicas info */}
										{node.replicas && (
											<g transform="translate(20, 110)">
												<Copy
													x="0"
													y="0"
													width="20"
													height="20"
													color="#9ca3af"
												/>
												<text x="30" y="15" fill="#9ca3af" fontSize="14">
													{node.replicas} Replica{node.replicas > 1 ? "s" : ""}
												</text>
											</g>
										)}
									</g>
								);
							})}
						</g>
					</svg>

					{/* Create Button */}
					<button
						onClick={() => setShowAddComponentModal(true)}
						className="absolute top-4 right-4 bg-purple-600 hover:bg-purple-700 text-white px-4 py-2 rounded-lg flex items-center gap-2 shadow-lg"
					>
						<Plus className="w-4 h-4" />
						Create
					</button>

					{/* Zoom Indicator */}
					<div className="absolute bottom-4 left-4 bg-gray-800 px-3 py-1 rounded-lg text-sm text-gray-400">
						{Math.round(zoom * 100)}%
					</div>
				</div>

				{/* Right Panel */}
				{selectedNode && (
					<div className="w-96 bg-gray-800 border-l border-gray-700 flex flex-col">
						<div className="p-4 border-b border-gray-700 flex items-center justify-between">
							<div className="flex items-center gap-3">
								{(() => {
									const Icon = getServiceIcon(selectedNode.type);
									const color = getServiceColor(selectedNode.type);
									return <Icon className="w-5 h-5" style={{ color }} />;
								})()}
								<div>
									<h2 className="text-white font-medium">
										{selectedNode.label}
									</h2>
									<p className="text-gray-400 text-sm">
										{selectedNode.project}
									</p>
								</div>
							</div>
							<button
								onClick={() => setSelectedNode(null)}
								className="text-gray-400 hover:text-white"
							>
								<X className="w-5 h-5" />
							</button>
						</div>

						{/* Tabs */}
						<div className="flex border-b border-gray-700">
							{[
								{ id: "settings", label: "Settings", icon: Settings },
								{ id: "logs", label: "Logs", icon: FileText },
								{ id: "metrics", label: "Metrics", icon: BarChart3 },
								{ id: "env", label: "Environment", icon: Terminal },
							].map((tab) => (
								<button
									key={tab.id}
									onClick={() => setActiveTab(tab.id)}
									className={`flex-1 px-4 py-3 text-sm flex items-center justify-center gap-2 ${
										activeTab === tab.id
											? "text-purple-400 border-b-2 border-purple-400"
											: "text-gray-400 hover:text-white"
									}`}
								>
									<tab.icon className="w-4 h-4" />
									{tab.label}
								</button>
							))}
						</div>

						{/* Tab Content */}
						<div className="flex-1 overflow-y-auto p-4">
							{activeTab === "settings" && (
								<div className="space-y-4">
									<div>
										<label className="text-gray-400 text-sm">
											Container Image
										</label>
										<input
											className="w-full mt-1 bg-gray-700 text-white px-3 py-2 rounded-lg border border-gray-600"
											value={selectedNode.project}
											readOnly
										/>
									</div>
									<div>
										<label className="text-gray-400 text-sm">Region</label>
										<select className="w-full mt-1 bg-gray-700 text-white px-3 py-2 rounded-lg border border-gray-600">
											<option>us-east-1</option>
											<option>us-west-2</option>
											<option>eu-west-1</option>
											<option>ap-southeast-1</option>
										</select>
									</div>
									{selectedNode.type === "backend" && (
										<>
											<div>
												<label className="text-gray-400 text-sm">
													Replicas
												</label>
												<div className="flex items-center gap-2 mt-1">
													<button
														onClick={() => updateReplicas(selectedNode.id, -1)}
														className="px-3 py-1 bg-gray-600 hover:bg-gray-500 rounded text-white"
													>
														-
													</button>
													<input
														className="w-20 text-center bg-gray-700 text-white px-3 py-2 rounded-lg border border-gray-600"
														value={selectedNode.replicas || 1}
														readOnly
													/>
													<button
														onClick={() => updateReplicas(selectedNode.id, 1)}
														className="px-3 py-1 bg-gray-600 hover:bg-gray-500 rounded text-white"
													>
														+
													</button>
												</div>
											</div>
											<div>
												<label className="text-gray-400 text-sm">Memory</label>
												<select className="w-full mt-1 bg-gray-700 text-white px-3 py-2 rounded-lg border border-gray-600">
													<option>512MB</option>
													<option>1GB</option>
													<option>2GB</option>
													<option>4GB</option>
												</select>
											</div>
										</>
									)}
									<div className="pt-4 space-y-2">
										<button className="w-full bg-purple-600 hover:bg-purple-700 text-white py-2 rounded-lg">
											Update Configuration
										</button>
										<button
											onClick={() => deleteNode(selectedNode.id)}
											className="w-full bg-red-600 hover:bg-red-700 text-white py-2 rounded-lg"
										>
											Delete Component
										</button>
									</div>
								</div>
							)}
							{activeTab === "logs" && (
								<div className="font-mono text-xs space-y-1">
									<div className="text-gray-400">
										[2024-06-25 08:05:50] Starting deployment...
									</div>
									<div className="text-blue-400">
										[2024-06-25 08:06:20] Pulling Docker image...
									</div>
									<div className="text-green-400">
										[2024-06-25 08:06:50] Container started successfully
									</div>
									<div className="text-gray-400">
										[2024-06-25 08:07:00] Health check passed
									</div>
									<div className="text-green-400">
										[2024-06-25 08:07:10] Deployment complete
									</div>
								</div>
							)}
							{activeTab === "metrics" && (
								<div className="space-y-4">
									<div className="bg-gray-700 p-4 rounded-lg">
										<h4 className="text-gray-300 text-sm mb-2">CPU Usage</h4>
										<div className="text-2xl text-purple-400">24%</div>
									</div>
									<div className="bg-gray-700 p-4 rounded-lg">
										<h4 className="text-gray-300 text-sm mb-2">Memory Usage</h4>
										<div className="text-2xl text-purple-400">
											512 MB / 1024 MB
										</div>
									</div>
									<div className="bg-gray-700 p-4 rounded-lg">
										<h4 className="text-gray-300 text-sm mb-2">Request Rate</h4>
										<div className="text-2xl text-purple-400">1.2k req/min</div>
									</div>
								</div>
							)}
							{activeTab === "env" && (
								<div className="space-y-4">
									<div>
										<label className="text-gray-400 text-sm">
											Environment Variables
										</label>
										<div className="mt-2 space-y-2">
											<div className="flex gap-2">
												<input
													className="flex-1 bg-gray-700 text-white px-3 py-2 rounded-lg text-sm border border-gray-600"
													placeholder="KEY"
												/>
												<input
													className="flex-1 bg-gray-700 text-white px-3 py-2 rounded-lg text-sm border border-gray-600"
													placeholder="VALUE"
												/>
												<button className="text-red-400 hover:text-red-300">
													<Trash2 className="w-4 h-4" />
												</button>
											</div>
										</div>
										<button className="mt-2 text-purple-400 text-sm hover:text-purple-300">
											+ Add Variable
										</button>
									</div>
								</div>
							)}
						</div>
					</div>
				)}

				{/* Activity Panel */}
				{showActivityPanel && (
					<div className="w-80 bg-gray-800 border-l border-gray-700 p-4">
						<div className="flex items-center justify-between mb-4">
							<h3 className="text-white font-medium flex items-center gap-2">
								<Activity className="w-4 h-4" />
								Activity
							</h3>
							<button
								onClick={() => setShowActivityPanel(false)}
								className="text-gray-400 hover:text-white"
							>
								<X className="w-4 h-4" />
							</button>
						</div>
						<div className="space-y-3">
							<div className="text-sm">
								<div className="flex items-center gap-2 text-yellow-400 mb-1">
									<Clock className="w-4 h-4" />
									<span>Frontend</span>
								</div>
								<p className="text-gray-400 text-xs">
									Deployment completed • 2 hours ago
								</p>
							</div>
							<div className="text-sm">
								<div className="flex items-center gap-2 text-purple-400 mb-1">
									<Zap className="w-4 h-4" />
									<span>Backend API</span>
								</div>
								<p className="text-gray-400 text-xs">
									Scaled to 3 instances • 1 hour ago
								</p>
							</div>
							<div className="text-sm">
								<div className="flex items-center gap-2 text-purple-400 mb-1">
									<Database className="w-4 h-4" />
									<span>PostgreSQL</span>
								</div>
								<p className="text-gray-400 text-xs">
									Backup completed • 6 hours ago
								</p>
							</div>
						</div>
					</div>
				)}
			</div>

			{/* Add Component Modal */}
			{showAddComponentModal && (
				<div className="absolute inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50">
					<div className="bg-gray-800 rounded-xl p-6 w-96 shadow-2xl">
						<h2 className="text-white text-xl font-semibold mb-4">
							Add Component
						</h2>
						<div className="grid grid-cols-2 gap-3">
							{componentTypes.map((comp) => {
								const color = getServiceColor(comp.type);
								return (
									<button
										key={comp.type}
										onClick={() => addComponent(comp.type)}
										className="bg-gray-700 hover:bg-gray-600 p-4 rounded-lg flex flex-col items-center gap-2 transition-all border border-gray-600 hover:border-gray-500"
									>
										<comp.icon className="w-8 h-8" style={{ color }} />
										<span className="text-gray-200 text-sm">{comp.label}</span>
									</button>
								);
							})}
						</div>
						<button
							onClick={() => setShowAddComponentModal(false)}
							className="mt-4 w-full bg-gray-700 hover:bg-gray-600 text-white py-2 rounded-lg"
						>
							Cancel
						</button>
					</div>
				</div>
			)}

			{/* Activity Toggle Button */}
			<button
				onClick={() => setShowActivityPanel(!showActivityPanel)}
				className="absolute bottom-4 right-4 bg-purple-600 hover:bg-purple-700 text-white p-3 rounded-full shadow-lg"
			>
				<Activity className="w-5 h-5" />
			</button>
		</div>
	);
};

export default DeploymentPlatformUI;
