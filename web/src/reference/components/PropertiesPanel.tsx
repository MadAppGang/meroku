import React, { useState } from "react";
import {
	X,
	ChevronLeft,
	ChevronRight,
	ChevronDown,
	ChevronUp,
	Copy,
	Check,
	Shield,
	Lock,
	Eye,
	EyeOff,
	Cpu,
	Trash2,
	Plus,
	Settings,
	Terminal,
	Activity,
	Database,
	Globe,
	Share2,
	Container,
	FileText,
	Network,
	BookOpen,
	HelpCircle,
	ExternalLink,
	Loader2,
	Key,
	RefreshCw,
	Download,
} from "lucide-react";
import { TabOption, IAMRole, IAMPolicy, EnvVar, SSMParameter } from "../types";

interface PropertiesPanelProps {
	isOpen: boolean;
	togglePanel: () => void;
}

const MOCK_ROLES: IAMRole[] = [
	{
		id: "role-1",
		name: "TerminatorTaskRole",
		description: "Used by the running container",
		arn: "arn:aws:iam::123456789012:role/circl_terminator_task_dev",
		policies: [
			{
				id: "p1",
				name: "CloudWatchFullAccess",
				type: "Managed",
				accessLevel: "Full",
			},
			{
				id: "p2",
				name: "SSMParameterAccess",
				type: "Inline",
				accessLevel: "Read",
			},
		],
	},
	{
		id: "role-2",
		name: "TerminatorExecutionRole",
		description: "Used by ECS to pull images and start containers",
		arn: "arn:aws:iam::123456789012:role/circl_scheduler_terminator_task_execution_dev",
		policies: [
			{
				id: "p3",
				name: "AmazonECSTaskExecutionRolePolicy",
				type: "Managed",
				accessLevel: "Full",
			},
		],
	},
];

const MOCK_ENV: EnvVar[] = [
	{
		id: "e1",
		key: "DB_HOST",
		value: "postgres-primary.cluster-xyz.us-east-1.rds.amazonaws.com",
		isSecret: false,
		isPredefined: true,
	},
	{
		id: "e2",
		key: "NODE_ENV",
		value: "production",
		isSecret: false,
		isPredefined: true,
	},
	{
		id: "e3",
		key: "API_KEY",
		value: "sk_live_51Mz...",
		isSecret: true,
		isPredefined: false,
	},
	{
		id: "e4",
		key: "FEATURE_FLAGS",
		value: "new-ui,beta-access",
		isSecret: false,
		isPredefined: false,
	},
];

const MOCK_SSM: SSMParameter[] = [
	{ id: "s1", name: "db_password", value: null },
	{ id: "s2", name: "stripe_secret_key", value: null },
	{ id: "s3", name: "sendgrid_api_key", value: null },
];

const MOCK_SERVICES = [
	"auth-service",
	"payment-processor",
	"notification-worker",
	"audit-logger",
	"email-dispatcher",
	"search-indexer",
];

const SSM_PREFIX = "/op/service/dev/";

export const PropertiesPanel: React.FC<PropertiesPanelProps> = ({
	isOpen,
	togglePanel,
}) => {
	const [activeTab, setActiveTab] = useState<TabOption>(TabOption.Settings);
	const [envVars, setEnvVars] = useState<EnvVar[]>(MOCK_ENV);
	const [ssmParams, setSsmParams] = useState<SSMParameter[]>(MOCK_SSM);

	// Env Var Handlers
	const handleAddEnvVar = () => {
		const newVar: EnvVar = {
			id: `e${Date.now()}`,
			key: "",
			value: "",
			isSecret: false,
			isPredefined: false,
		};
		setEnvVars([...envVars, newVar]);
	};

	const handleRemoveEnvVar = (id: string) => {
		setEnvVars(envVars.filter((v) => v.id !== id));
	};

	const handleUpdateEnvVar = (id: string, updates: Partial<EnvVar>) => {
		setEnvVars(envVars.map((v) => (v.id === id ? { ...v, ...updates } : v)));
	};

	// SSM Handlers
	const handleAddSSM = () => {
		const newParam: SSMParameter = {
			id: `s${Date.now()}`,
			name: "",
			value: null,
		};
		setSsmParams([...ssmParams, newParam]);
	};

	const handleRemoveSSM = (id: string) => {
		setSsmParams(ssmParams.filter((p) => p.id !== id));
	};

	const handleUpdateSSM = (id: string, updates: Partial<SSMParameter>) => {
		setSsmParams(
			ssmParams.map((p) => (p.id === id ? { ...p, ...updates } : p)),
		);
	};

	const handleFetchSSM = (id: string) => {
		// Set loading state
		setSsmParams(
			ssmParams.map((p) => (p.id === id ? { ...p, isLoading: true } : p)),
		);

		// Simulate API call
		setTimeout(() => {
			setSsmParams((current) =>
				current.map((p) => {
					if (p.id === id) {
						return {
							...p,
							isLoading: false,
							value: `fetched_secret_${Math.random().toString(36).substring(7)}`, // Mock value
						};
					}
					return p;
				}),
			);
		}, 1500);
	};

	if (!isOpen) return null;

	return (
		<div className="w-[600px] h-full bg-tech-bg/95 backdrop-blur-md border-l border-tech-border flex flex-col shadow-2xl z-30 transform transition-transform duration-300">
			{/* Header */}
			<div className="h-16 flex items-center justify-between px-6 border-b border-tech-border bg-tech-bg shrink-0">
				<div>
					<h2 className="text-white font-medium text-lg tracking-tight">
						terminator
					</h2>
					<div className="flex items-center gap-2 text-xs text-zinc-500 font-mono mt-0.5">
						<span className="text-tech-success">● Running</span>
						<span>|</span>
						<span>ap-southeast-2</span>
					</div>
				</div>
				<div className="flex items-center gap-3">
					<button className="text-zinc-500 hover:text-red-400 transition-colors">
						<Trash2 size={18} />
					</button>
					<button
						onClick={togglePanel}
						className="text-zinc-500 hover:text-white transition-colors"
					>
						<X size={20} />
					</button>
				</div>
			</div>

			{/* Navigation Tabs */}
			<div className="flex px-2 border-b border-tech-border bg-tech-bg/50 shrink-0 overflow-x-auto no-scrollbar">
				{Object.values(TabOption).map((tab) => (
					<button
						key={tab}
						onClick={() => setActiveTab(tab)}
						className={`
              relative px-4 py-3 text-sm font-medium transition-colors whitespace-nowrap
              ${activeTab === tab ? "text-white" : "text-zinc-500 hover:text-zinc-300"}
            `}
					>
						{tab}
						{activeTab === tab && (
							<div className="absolute bottom-0 left-0 w-full h-[2px] bg-tech-accent shadow-[0_0_10px_rgba(59,130,246,0.5)]" />
						)}
					</button>
				))}
			</div>

			{/* Scrollable Content Area */}
			<div className="flex-1 overflow-y-auto p-6 space-y-8 custom-scrollbar">
				{activeTab === TabOption.Settings && <SettingsSection />}
				{activeTab === TabOption.Autoscaling && <AutoscalingSection />}
				{activeTab === TabOption.SSH && <SSHSection />}
				{activeTab === TabOption.CICD && <CICDSection />}
				{activeTab === TabOption.IAM && <IAMSection roles={MOCK_ROLES} />}
				{activeTab === TabOption.EnvVars && (
					<EnvVarsSection
						vars={envVars}
						ssmParams={ssmParams}
						onAdd={handleAddEnvVar}
						onRemove={handleRemoveEnvVar}
						onUpdate={handleUpdateEnvVar}
						onAddSSM={handleAddSSM}
						onRemoveSSM={handleRemoveSSM}
						onUpdateSSM={handleUpdateSSM}
						onFetchSSM={handleFetchSSM}
					/>
				)}
				{activeTab === TabOption.XRay && <XRaySection />}

				{/* Placeholder for other tabs */}
				{![
					TabOption.Settings,
					TabOption.Autoscaling,
					TabOption.SSH,
					TabOption.CICD,
					TabOption.IAM,
					TabOption.EnvVars,
					TabOption.XRay,
				].includes(activeTab) && (
					<div className="flex flex-col items-center justify-center h-64 text-zinc-600">
						<Settings size={48} className="mb-4 opacity-20" />
						<p>Configuration for {activeTab} coming soon.</p>
					</div>
				)}
			</div>

			{/* Footer Actions */}
			<div className="p-4 border-t border-tech-border bg-tech-bg flex justify-between items-center shrink-0">
				<span className="text-xs text-zinc-500">Last updated 2m ago</span>
				<div className="flex gap-3">
					<button className="px-4 py-2 text-sm text-zinc-300 hover:text-white transition-colors">
						Discard
					</button>
					<button className="px-4 py-2 text-sm bg-white text-black font-semibold rounded hover:bg-zinc-200 transition-colors shadow-[0_0_15px_rgba(255,255,255,0.1)]">
						Apply Changes
					</button>
				</div>
			</div>
		</div>
	);
};

/* --- Sub-Components for specific sections --- */

const SSHSection: React.FC = () => {
	const [enabled, setEnabled] = useState(false);
	const [connectionString, setConnectionString] = useState<string | null>(null);
	const [isGenerating, setIsGenerating] = useState(false);

	const handleGenerate = () => {
		setIsGenerating(true);
		// Simulate network request
		setTimeout(() => {
			setConnectionString(
				"aws ecs execute-command --cluster circl_dev --task 8f2a...",
			);
			setIsGenerating(false);
		}, 1500);
	};

	return (
		<div className="flex flex-col gap-6 animate-in fade-in slide-in-from-bottom-4 duration-500">
			<div className="border border-tech-border rounded-xl bg-tech-surface/20 shadow-sm overflow-hidden">
				{/* Header */}
				<div className="flex items-center justify-between p-4 border-b border-tech-border bg-white/[0.01]">
					<div className="flex items-center gap-3">
						<div
							className={`p-1.5 rounded-lg ${enabled ? "bg-emerald-500/10 text-emerald-500 border border-emerald-500/20" : "bg-zinc-800 text-zinc-500"}`}
						>
							<Terminal size={18} />
						</div>
						<div>
							<h3 className="text-sm font-medium text-zinc-200">
								Remote Shell
							</h3>
							<p className="text-xs text-zinc-500">Secure ECS Exec access</p>
						</div>
					</div>
					<ToggleSwitch
						checked={enabled}
						onChange={() => {
							setEnabled(!enabled);
							if (enabled) setConnectionString(null);
						}}
					/>
				</div>

				{enabled && (
					<div className="p-5 space-y-5">
						{/* Status */}
						<div className="flex items-center gap-2 px-3 py-2 rounded-lg bg-emerald-500/5 border border-emerald-500/10">
							<div className="relative flex h-2 w-2">
								<span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75"></span>
								<span className="relative inline-flex rounded-full h-2 w-2 bg-emerald-500"></span>
							</div>
							<span className="text-xs font-medium text-emerald-500">
								Agent Active & Ready
							</span>
						</div>

						{/* Actions */}
						<div className="grid grid-cols-1 gap-3">
							<button className="flex items-center justify-center gap-2 w-full py-2 bg-white text-black text-xs font-bold uppercase tracking-wide rounded hover:bg-zinc-200 transition-colors shadow-[0_0_15px_rgba(255,255,255,0.1)]">
								<Terminal size={14} />
								Launch Web Terminal
							</button>
						</div>

						{/* CLI Command */}
						<div className="space-y-2">
							<div className="flex items-center justify-between">
								<label className="text-[10px] uppercase tracking-wider font-semibold text-zinc-500">
									CLI Access
								</label>
								{connectionString && (
									<a
										href="#"
										className="flex items-center gap-1 text-[10px] text-tech-accent hover:text-blue-400 transition-colors"
									>
										<span>Documentation</span>
										<ExternalLink size={10} />
									</a>
								)}
							</div>

							{!connectionString ? (
								<button
									onClick={handleGenerate}
									disabled={isGenerating}
									className="w-full h-10 border border-dashed border-tech-border rounded-lg bg-black/20 hover:bg-black/40 hover:border-zinc-700 transition-all flex items-center justify-center gap-2 group"
								>
									{isGenerating ? (
										<>
											<div className="w-3 h-3 border-2 border-zinc-600 border-t-zinc-300 rounded-full animate-spin"></div>
											<span className="text-xs text-zinc-500 font-mono">
												Requesting session token...
											</span>
										</>
									) : (
										<>
											<span className="text-zinc-600 group-hover:text-zinc-400 transition-colors font-mono text-xs">
												{">_"}
											</span>
											<span className="text-xs text-zinc-500 group-hover:text-zinc-300 font-medium transition-colors">
												Generate Connection String
											</span>
										</>
									)}
								</button>
							) : (
								<div className="flex items-center bg-black border border-tech-border rounded-lg p-1 group focus-within:border-zinc-700 transition-colors animate-in fade-in slide-in-from-bottom-1 duration-300">
									<div className="px-2 text-zinc-600 select-none">
										<Terminal size={12} />
									</div>
									<code className="flex-1 text-[10px] font-mono text-zinc-400 truncate py-1.5">
										{connectionString}
									</code>
									<CopyButton text={connectionString} />
								</div>
							)}
						</div>
					</div>
				)}
			</div>
		</div>
	);
};

const XRaySection: React.FC = () => {
	const [enabled, setEnabled] = useState(true);

	const ports = [
		{ port: 2000, protocol: "UDP", desc: "X-Ray daemon endpoint" },
		{ port: 4317, protocol: "TCP", desc: "OpenTelemetry gRPC endpoint" },
		{ port: 4318, protocol: "TCP", desc: "OpenTelemetry HTTP endpoint" },
		{ port: 55681, protocol: "TCP", desc: "Legacy Jaeger/Zipkin endpoint" },
	];

	return (
		<div className="flex flex-col gap-6 animate-in fade-in slide-in-from-bottom-4 duration-500">
			{/* Main Toggle Card */}
			<div className="border border-tech-border rounded-xl bg-tech-surface/20 shadow-sm overflow-hidden">
				<div className="flex items-center justify-between p-4 border-b border-tech-border bg-white/[0.01]">
					<div className="flex items-center gap-3">
						<div
							className={`p-1.5 rounded-lg ${enabled ? "bg-purple-500/10 text-purple-400" : "bg-zinc-800 text-zinc-500"}`}
						>
							<Activity size={18} />
						</div>
						<div>
							<h3 className="text-sm font-medium text-zinc-200">
								Distributed Tracing
							</h3>
							<div className="flex items-center gap-2 mt-0.5">
								<p className="text-xs text-zinc-500">
									AWS X-Ray sidecar integration
								</p>
								<span className="text-zinc-700">|</span>
								<a
									href="#"
									className="flex items-center gap-1 text-[10px] text-tech-accent hover:text-blue-400 transition-colors"
								>
									<span>Docs</span>
									<ExternalLink size={10} />
								</a>
							</div>
						</div>
					</div>
					<ToggleSwitch
						checked={enabled}
						onChange={() => setEnabled(!enabled)}
					/>
				</div>

				{enabled && (
					<div className="p-5 space-y-6">
						{/* Sidecar Status Indicator */}
						<div className="flex items-center gap-2 p-3 rounded bg-zinc-950 border border-tech-border">
							<div className="w-2 h-2 rounded-full bg-tech-success shadow-[0_0_8px_rgba(16,185,129,0.5)] animate-pulse"></div>
							<span className="text-xs text-zinc-300 font-medium">
								ADOT Collector Sidecar Active
							</span>
						</div>

						{/* Port Mappings */}
						<div className="space-y-3">
							<label className="text-[10px] uppercase tracking-wider font-semibold text-zinc-500 flex items-center gap-2">
								<Network size={12} /> Port Mappings (localhost)
							</label>

							<div className="border border-tech-border rounded-lg overflow-hidden">
								<table className="w-full text-left">
									<thead className="bg-zinc-950/50 text-[10px] uppercase text-zinc-500 font-semibold border-b border-tech-border">
										<tr>
											<th className="px-3 py-2 w-16">Port</th>
											<th className="px-3 py-2 w-16">Proto</th>
											<th className="px-3 py-2">Service</th>
										</tr>
									</thead>
									<tbody className="divide-y divide-tech-border bg-black/20">
										{ports.map((p) => (
											<tr
												key={p.port}
												className="hover:bg-white/5 transition-colors"
											>
												<td className="px-3 py-2 text-xs font-mono text-tech-accent">
													{p.port}
												</td>
												<td className="px-3 py-2">
													<span
														className={`text-[9px] px-1.5 py-0.5 rounded border ${p.protocol === "UDP" ? "border-blue-500/20 text-blue-400 bg-blue-500/10" : "border-zinc-700 text-zinc-400 bg-zinc-800"}`}
													>
														{p.protocol}
													</span>
												</td>
												<td className="px-3 py-2 text-xs text-zinc-400">
													{p.desc}
												</td>
											</tr>
										))}
									</tbody>
								</table>
							</div>
						</div>

						{/* Help / Integration Reference */}
						<div className="mt-2 pt-4 border-t border-tech-border/50 flex items-center justify-between">
							<div className="flex items-center gap-2 text-zinc-400">
								<BookOpen size={14} />
								<span className="text-xs">Need instrumentation code?</span>
							</div>
							<button className="flex items-center gap-1.5 text-xs text-tech-accent hover:text-blue-300 transition-colors group">
								<span>View Integration Guide</span>
								<ChevronRight
									size={12}
									className="group-hover:translate-x-0.5 transition-transform"
								/>
							</button>
						</div>
					</div>
				)}
			</div>
		</div>
	);
};

const processYamlValue = (text: string) => {
	let result = text;
	// Highlight strings
	result = result.replace(
		/(["'])(.*?)\1/g,
		'<span class="text-emerald-400">$1$2$1</span>',
	);
	// Highlight variables
	result = result.replace(
		/(\$\{\{.*?\}\})/g,
		'<span class="text-amber-400">$1</span>',
	);
	// Highlight booleans
	result = result.replace(
		/\b(true|false)\b/g,
		'<span class="text-orange-400">$1</span>',
	);
	return result;
};

const highlightYaml = (code: string) => {
	return code
		.split("\n")
		.map((line) => {
			// Comments
			if (line.trim().startsWith("#")) {
				return `<span class="text-zinc-600 italic">${line}</span>`;
			}

			// Key-Value pairs (handles "key:", "  key:", "- key:", "  - key:")
			const keyMatch = line.match(/^(\s*)(-\s+)?([\w-]+)(:)(.*)/);
			if (keyMatch) {
				const [_, indent, dash, key, colon, val] = keyMatch;
				const highlightedKey = `<span class="text-blue-400">${key}</span>`;
				const processedVal = processYamlValue(val || "");
				return `${indent}${dash || ""}${highlightedKey}${colon}${processedVal}`;
			}

			// Fallback for other lines
			return processYamlValue(line);
		})
		.join("\n");
};

const CICDSection: React.FC = () => {
	const [expanded, setExpanded] = useState(false);

	const workflowCode = `name: Deploy backend to AWS (dev)

on:
  push:
    branches: [main]
  workflow_dispatch:

concurrency:
  group: deploy-backend-dev-\${{ github.ref }}
  cancel-in-progress: true

jobs:
  deploy:
    name: Build and Deploy to ECS
    runs-on: ubuntu-latest
    permissions:
      id-token: write
      contents: read

    steps:
      - name: Checkout code
        uses: actions/checkout@v4

      - name: Configure AWS credentials
        uses: aws-actions/configure-aws-credentials@v4.0.1
        with:
          role-to-assume: arn:aws:iam::285253872242:role/circl-dev-github-actions-role
          aws-region: ap-southeast-2

      - name: Login to Amazon ECR
        id: login-ecr
        uses: aws-actions/amazon-ecr-login@v2

      - name: Build, tag, and push Docker image
        env:
          ECR_REGISTRY: \${{ steps.login-ecr.outputs.registry }}
          ECR_REPOSITORY: circl_backend
          IMAGE_TAG: \${{ github.sha }}
        run: |
          # Build Docker image (Dockerfile in root)
          docker build -t $ECR_REGISTRY/$ECR_REPOSITORY:$IMAGE_TAG .
          docker tag $ECR_REGISTRY/$ECR_REPOSITORY:$IMAGE_TAG $ECR_REGISTRY/$ECR_REPOSITORY:latest

          # Create ECR repository if it doesn't exist
          aws ecr describe-repositories --repository-names $ECR_REPOSITORY || \\
            aws ecr create-repository --repository-name $ECR_REPOSITORY

          # Push images
          docker push $ECR_REGISTRY/$ECR_REPOSITORY:$IMAGE_TAG
          docker push $ECR_REGISTRY/$ECR_REPOSITORY:latest

          echo "✅ Image pushed: $ECR_REGISTRY/$ECR_REPOSITORY:$IMAGE_TAG"

      - name: Deploy to ECS
        run: |
          echo "Deploying backend to ECS..."
          aws ecs update-service \\
            --cluster circl_cluster_dev \\
            --service circl_service_dev \\
            --force-new-deployment \\
            --region ap-southeast-2

      - name: Wait for service stability
        run: |
          echo "Waiting for service to stabilize..."
          aws ecs wait services-stable \\
            --cluster circl_cluster_dev \\
            --services circl_service_dev \\
            --region ap-southeast-2
          echo "✅ Deployment completed successfully"`;

	return (
		<div className="flex flex-col gap-6 animate-in fade-in slide-in-from-bottom-4 duration-500">
			{/* Workflow File Viewer */}
			<div className="border border-tech-border rounded-xl bg-tech-surface/20 overflow-hidden shadow-sm flex flex-col">
				{/* File Header */}
				<div className="flex items-center justify-between p-4 border-b border-tech-border bg-white/[0.01]">
					<div className="flex items-center gap-3">
						<FileText size={16} className="text-zinc-500" />
						<span className="text-xs font-mono text-zinc-300 tracking-tight">
							.github/workflows/backend-dev.yml
						</span>
					</div>
					<button className="group flex items-center gap-1.5 px-2 py-1 rounded hover:bg-zinc-800 transition-colors">
						<Copy size={12} className="text-zinc-500 group-hover:text-white" />
						<span className="text-[10px] font-medium text-zinc-500 group-hover:text-white uppercase tracking-wider">
							Copy
						</span>
					</button>
				</div>

				{/* Code Block */}
				<div className="relative group/code bg-[#0d0d0f]">
					<div
						className={`
                        p-5 overflow-hidden transition-all duration-500 ease-in-out
                        ${expanded ? "max-h-[800px]" : "max-h-[240px]"}
                    `}
					>
						<pre className="font-mono text-[11px] leading-relaxed text-zinc-300 overflow-x-auto selection:bg-blue-500/30">
							<code
								dangerouslySetInnerHTML={{
									__html: highlightYaml(workflowCode),
								}}
							/>
						</pre>
					</div>

					{/* Gradient Fade Overlay */}
					{!expanded && (
						<div className="absolute inset-x-0 bottom-0 h-24 bg-gradient-to-t from-tech-bg via-tech-bg/80 to-transparent pointer-events-none" />
					)}
				</div>

				{/* Expand Toggle */}
				<button
					onClick={() => setExpanded(!expanded)}
					className="w-full py-2.5 border-t border-tech-border bg-white/[0.02] hover:bg-white/[0.04] text-[10px] font-semibold text-zinc-500 hover:text-zinc-300 uppercase tracking-widest flex items-center justify-center gap-2 transition-colors focus:outline-none"
				>
					{expanded ? "Collapse Workflow" : "View Full Workflow"}
					{expanded ? <ChevronUp size={12} /> : <ChevronDown size={12} />}
				</button>
			</div>
		</div>
	);
};

const SettingsSection: React.FC = () => {
	const [mode, setMode] = useState<"dedicated" | "external" | "shared">(
		"dedicated",
	);
	const [bucketPostfix, setBucketPostfix] = useState("45fcj");
	const [sharedService, setSharedService] = useState("");
	const [isDropdownOpen, setIsDropdownOpen] = useState(false);

	return (
		<div className="flex flex-col gap-6 animate-in fade-in slide-in-from-bottom-4 duration-500">
			{/* Container Config Card */}
			<div className="border border-tech-border rounded-xl bg-tech-surface/20 shadow-sm relative">
				{/* Card Header with Tabs */}
				<div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 p-4 border-b border-tech-border bg-white/[0.01] rounded-t-xl">
					<div className="flex items-center gap-2.5">
						<div className="p-1.5 rounded bg-blue-500/10 text-blue-500 border border-blue-500/20">
							<Container size={16} />
						</div>
						<div>
							<h3 className="text-sm font-medium text-zinc-200">
								Container Image
							</h3>
							<p className="text-xs text-zinc-500">
								Configure the source image for this task
							</p>
						</div>
					</div>

					{/* Compact Tabs */}
					<div className="flex items-center p-1 rounded-lg bg-black border border-tech-border">
						<TabButton
							active={mode === "dedicated"}
							onClick={() => setMode("dedicated")}
							label="Default"
						/>
						<TabButton
							active={mode === "external"}
							onClick={() => setMode("external")}
							label="Custom"
						/>
						<TabButton
							active={mode === "shared"}
							onClick={() => setMode("shared")}
							label="Shared"
						/>
					</div>
				</div>

				<div className="p-5 space-y-5">
					{/* Unified Image & Tag Configuration */}
					<div className="space-y-2">
						<div className="flex justify-between items-center">
							<label className="text-[10px] uppercase tracking-wider font-semibold text-zinc-500">
								{mode === "dedicated"
									? "Managed Repository"
									: mode === "external"
										? "Image URI"
										: "Source Service"}
							</label>
							{mode !== "dedicated" && (
								<span className="text-[10px] text-zinc-600 font-mono">
									image:tag
								</span>
							)}
						</div>

						<div
							className={`
                            relative flex items-center h-10 rounded-lg border bg-black transition-all group focus-within:border-tech-accent focus-within:ring-1 focus-within:ring-tech-accent
                            ${isDropdownOpen ? "border-tech-accent ring-1 ring-tech-accent z-20" : "border-tech-border hover:border-zinc-700"}
                        `}
						>
							{/* Backdrop for dropdown */}
							{isDropdownOpen && (
								<div
									className="fixed inset-0 z-0"
									onClick={() => setIsDropdownOpen(false)}
								/>
							)}

							{/* Main Input Area */}
							<div className="flex-1 min-w-0 relative z-10">
								{mode === "dedicated" && (
									<div className="flex items-center h-full px-3 text-zinc-500">
										<Lock size={14} className="mr-3 text-zinc-600 shrink-0" />
										<span className="font-mono text-sm text-zinc-400 truncate">
											123456789.dkr.ecr.ap-southeast-2.amazonaws.com/circl_backend
										</span>
										<div className="ml-2">
											<span className="text-[10px] bg-tech-success/10 text-tech-success border border-tech-success/20 px-2 py-0.5 rounded font-medium">
												Auto
											</span>
										</div>
									</div>
								)}

								{mode === "external" && (
									<div className="flex items-center h-full px-3">
										<Globe size={14} className="mr-3 text-zinc-500 shrink-0" />
										<input
											placeholder="docker.io/library/nginx"
											className="w-full bg-transparent text-sm font-mono text-white placeholder-zinc-700 focus:outline-none"
										/>
									</div>
								)}

								{mode === "shared" && (
									<>
										<button
											onClick={() => setIsDropdownOpen(!isDropdownOpen)}
											className="w-full h-full text-left flex items-center justify-between px-3 focus:outline-none"
										>
											<div className="flex items-center gap-3 overflow-hidden">
												<Share2 size={14} className="text-zinc-500 shrink-0" />
												<span
													className={`font-mono text-sm truncate ${sharedService ? "text-white" : "text-zinc-500"}`}
												>
													{sharedService || "Select service..."}
												</span>
											</div>
											<ChevronDown
												size={14}
												className={`text-zinc-500 shrink-0 transition-transform ${isDropdownOpen ? "rotate-180" : ""}`}
											/>
										</button>

										{isDropdownOpen && (
											<div className="absolute top-full left-0 right-[-86px] mt-2 bg-zinc-950 border border-tech-border rounded-lg shadow-2xl overflow-hidden p-1 animate-in fade-in zoom-in-95 duration-100 z-50">
												<div className="px-2 py-1.5 text-[10px] uppercase text-zinc-500 font-semibold tracking-wider border-b border-zinc-900 mb-1">
													Available Services
												</div>
												<div className="max-h-60 overflow-y-auto custom-scrollbar">
													{MOCK_SERVICES.map((s) => (
														<button
															key={s}
															onClick={() => {
																setSharedService(s);
																setIsDropdownOpen(false);
															}}
															className={`
                                                                w-full text-left px-3 py-2 text-sm font-mono rounded-md flex items-center justify-between group transition-colors
                                                                ${sharedService === s ? "bg-tech-accent/10 text-tech-accent" : "text-zinc-400 hover:bg-zinc-900 hover:text-white"}
                                                            `}
														>
															<span>{s}</span>
															{sharedService === s && <Check size={14} />}
														</button>
													))}
												</div>
											</div>
										)}
									</>
								)}
							</div>

							{/* Divider */}
							<div className="h-5 w-px bg-tech-border"></div>

							{/* Tag Input */}
							<div className="w-[85px] h-full flex items-center px-3 relative z-10">
								<span className="text-zinc-600 font-mono text-sm mr-1 select-none">
									:
								</span>
								<input
									defaultValue="latest"
									readOnly={mode === "dedicated"}
									className={`w-full bg-transparent text-sm font-mono focus:outline-none p-0 border-none focus:ring-0 ${mode === "dedicated" ? "text-zinc-500 cursor-not-allowed" : "text-tech-accent"}`}
								/>
							</div>
						</div>
					</div>

					{/* Command Row */}
					<div className="space-y-2">
						<label className="text-[10px] uppercase tracking-wider font-semibold text-zinc-500">
							Command Override
						</label>
						<div className="relative group">
							<div className="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none">
								<Terminal
									size={14}
									className="text-zinc-600 group-focus-within:text-tech-accent transition-colors"
								/>
							</div>
							<input
								defaultValue="npm, start"
								className="w-full bg-black/40 border border-tech-border rounded-lg py-2 pl-9 pr-3 text-sm font-mono text-zinc-300 focus:text-white focus:border-tech-accent focus:ring-1 focus:ring-tech-accent transition-all placeholder-zinc-700"
							/>
						</div>
					</div>
				</div>
			</div>

			{/* Storage Config Card */}
			<div className="border border-tech-border rounded-xl bg-tech-surface/20 overflow-hidden shadow-sm">
				<div className="flex items-center gap-2.5 p-4 border-b border-tech-border bg-white/[0.01]">
					<div className="p-1.5 rounded bg-purple-500/10 text-purple-400 border border-purple-500/20">
						<Database size={16} />
					</div>
					<div>
						<h3 className="text-sm font-medium text-zinc-200">
							Storage Resources
						</h3>
						<p className="text-xs text-zinc-500">S3 bucket configuration</p>
					</div>
				</div>

				<div className="p-5 flex items-center gap-6">
					{/* Inline Composite Input for Bucket */}
					<div className="flex-1 space-y-2">
						<label className="text-[10px] uppercase tracking-wider font-semibold text-zinc-500">
							S3 Bucket Identifier
						</label>
						<div className="flex items-baseline gap-0.5 p-2 rounded-lg border border-tech-border bg-black/50 focus-within:border-tech-accent focus-within:ring-1 focus-within:ring-tech-accent/20 transition-all">
							<span className="text-sm text-zinc-500 font-mono select-none pl-1">
								circl-backend-dev-
							</span>
							<input
								value={bucketPostfix}
								onChange={(e) => setBucketPostfix(e.target.value)}
								className="flex-1 bg-transparent text-sm text-white font-mono focus:outline-none min-w-0 placeholder-zinc-700"
								placeholder="suffix"
							/>
						</div>
					</div>

					<div className="w-px h-10 bg-tech-border mx-2"></div>

					{/* Public Access Toggle */}
					<div className="flex flex-col items-center gap-3">
						<span className="text-[10px] uppercase tracking-wider font-semibold text-zinc-500">
							Public Access
						</span>
						<ToggleSwitch />
					</div>
				</div>
			</div>
		</div>
	);
};

const TabButton: React.FC<{
	active: boolean;
	onClick: () => void;
	label: string;
}> = ({ active, onClick, label }) => (
	<button
		onClick={onClick}
		className={`
            px-3 py-1 text-[10px] font-medium rounded transition-all duration-200 select-none
            ${
							active
								? "bg-zinc-800 text-white shadow-sm"
								: "text-zinc-500 hover:text-zinc-300"
						}
        `}
	>
		{label}
	</button>
);

const IAMSection: React.FC<{ roles: IAMRole[] }> = ({ roles }) => {
	return (
		<div className="space-y-6 animate-in fade-in slide-in-from-bottom-4 duration-500">
			<div className="p-4 rounded-lg bg-blue-500/5 border border-blue-500/20 flex gap-4 items-start">
				<Shield className="text-tech-accent mt-1 shrink-0" size={20} />
				<div>
					<h4 className="text-sm font-medium text-blue-100">
						Principle of Least Privilege
					</h4>
					<p className="text-xs text-blue-300/70 mt-1 leading-relaxed">
						Each task uses specific roles automatically created by Terraform.
						Modifying these permissions will trigger a recreation of the IAM
						policies.
					</p>
				</div>
			</div>

			<div className="space-y-4">
				<SectionHeader title="Assigned Roles" count={roles.length} />
				{roles.map((role) => (
					<RoleCard key={role.id} role={role} />
				))}
			</div>
		</div>
	);
};

const RoleCard: React.FC<{ role: IAMRole }> = ({ role }) => {
	const [expanded, setExpanded] = useState(false);

	return (
		<div
			className={`
      rounded-lg border transition-all duration-200 overflow-hidden
      ${expanded ? "bg-tech-surface border-tech-accent/30 ring-1 ring-tech-accent/20" : "bg-tech-bg border-tech-border hover:border-zinc-700"}
    `}
		>
			{/* Card Header */}
			<div
				onClick={() => setExpanded(!expanded)}
				className="flex items-center justify-between p-4 cursor-pointer group"
			>
				<div className="flex items-center gap-3">
					<div
						className={`p-2 rounded-md ${expanded ? "bg-tech-accent/20 text-tech-accent" : "bg-zinc-900 text-zinc-500 group-hover:text-zinc-300"}`}
					>
						<Shield size={18} />
					</div>
					<div>
						<h3 className="text-sm font-medium text-zinc-200">{role.name}</h3>
						<p className="text-xs text-zinc-500">{role.description}</p>
					</div>
				</div>
				<div className="flex items-center gap-3">
					<span className="text-xs font-mono text-zinc-600 px-2 py-1 rounded bg-zinc-900/50 border border-zinc-800">
						IAM Role
					</span>
					{expanded ? (
						<ChevronDown size={16} className="text-zinc-500" />
					) : (
						<ChevronRight size={16} className="text-zinc-500" />
					)}
				</div>
			</div>

			{/* Expanded Content */}
			{expanded && (
				<div className="border-t border-tech-border bg-black/20 p-4 space-y-5">
					{/* ARN Display */}
					<div className="space-y-2">
						<label className="text-[11px] uppercase tracking-wider font-semibold text-zinc-500">
							Resource ARN
						</label>
						<div className="flex items-center justify-between bg-black rounded border border-tech-border p-2 group/arn hover:border-zinc-600 transition-colors">
							<code className="text-xs font-mono text-zinc-300 truncate pr-4">
								{role.arn}
							</code>
							<CopyButton text={role.arn} />
						</div>
					</div>

					{/* Policies List */}
					<div className="space-y-3">
						<div className="flex justify-between items-end">
							<label className="text-[11px] uppercase tracking-wider font-semibold text-zinc-500">
								Attached Policies
							</label>
							<button className="text-xs text-tech-accent hover:text-blue-300 flex items-center gap-1">
								<Plus size={12} /> Add Policy
							</button>
						</div>

						<div className="space-y-2">
							{role.policies.map((policy) => (
								<div
									key={policy.id}
									className="flex items-center justify-between p-2 rounded hover:bg-white/5 transition-colors border border-transparent hover:border-white/5"
								>
									<div className="flex items-center gap-2">
										<Lock
											size={14}
											className={
												policy.type === "Managed"
													? "text-orange-400"
													: "text-purple-400"
											}
										/>
										<span className="text-sm text-zinc-300">{policy.name}</span>
									</div>
									<div className="flex items-center gap-2">
										<span
											className={`text-[10px] px-1.5 py-0.5 rounded border ${
												policy.type === "Managed"
													? "border-orange-500/20 text-orange-400 bg-orange-500/10"
													: "border-purple-500/20 text-purple-400 bg-purple-500/10"
											}`}
										>
											{policy.type}
										</span>
										<span className="text-[10px] px-1.5 py-0.5 rounded border border-zinc-700 text-zinc-500 bg-zinc-800">
											{policy.accessLevel}
										</span>
									</div>
								</div>
							))}
						</div>
					</div>
				</div>
			)}
		</div>
	);
};

const EnvVarsSection: React.FC<{
	vars: EnvVar[];
	ssmParams: SSMParameter[];
	onAdd: () => void;
	onRemove: (id: string) => void;
	onUpdate: (id: string, updates: Partial<EnvVar>) => void;
	onAddSSM: () => void;
	onRemoveSSM: (id: string) => void;
	onUpdateSSM: (id: string, updates: Partial<SSMParameter>) => void;
	onFetchSSM: (id: string) => void;
}> = ({
	vars,
	ssmParams,
	onAdd,
	onRemove,
	onUpdate,
	onAddSSM,
	onRemoveSSM,
	onUpdateSSM,
	onFetchSSM,
}) => {
	const predefined = vars.filter((v) => v.isPredefined);
	const custom = vars.filter((v) => !v.isPredefined);

	return (
		<div className="space-y-8 animate-in fade-in slide-in-from-bottom-4 duration-500">
			{/* Predefined Section */}
			<div className="space-y-3">
				<div className="flex items-center justify-between px-1">
					<div>
						<h3 className="text-xs font-semibold uppercase tracking-wider text-zinc-400">
							System Variables
						</h3>
						<p className="text-[10px] text-zinc-600 mt-0.5">
							Injected automatically by the runtime
						</p>
					</div>
					<span className="text-xs font-mono text-zinc-500 bg-zinc-900 px-2 py-0.5 rounded border border-zinc-800">
						{predefined.length}
					</span>
				</div>
				<div className="bg-tech-surface/50 border border-tech-border rounded-lg overflow-hidden">
					<table className="w-full text-left">
						<thead className="bg-zinc-950/50 text-[10px] uppercase text-zinc-600 font-semibold tracking-wider border-b border-tech-border">
							<tr>
								<th className="px-4 py-2 w-1/3">Key</th>
								<th className="px-4 py-2">Value</th>
								<th className="px-4 py-2 w-10"></th>
							</tr>
						</thead>
						<tbody className="divide-y divide-tech-border/50">
							{predefined.map((v) => (
								<EnvRow
									key={v.id}
									env={v}
									onRemove={onRemove}
									onUpdate={onUpdate}
									readOnly
								/>
							))}
						</tbody>
					</table>
				</div>
			</div>

			{/* Custom Section */}
			<div className="space-y-3">
				<div className="flex items-center justify-between px-1">
					<div>
						<h3 className="text-xs font-semibold uppercase tracking-wider text-zinc-400">
							Custom Variables
						</h3>
						<p className="text-[10px] text-zinc-600 mt-0.5">
							Task-specific overrides and secrets
						</p>
					</div>
					<span className="text-xs font-mono text-zinc-500 bg-zinc-900 px-2 py-0.5 rounded border border-zinc-800">
						{custom.length}
					</span>
				</div>

				<div className="bg-tech-surface border border-tech-border rounded-lg overflow-hidden flex flex-col">
					{custom.length > 0 ? (
						<table className="w-full text-left">
							<thead className="bg-zinc-950/50 text-[10px] uppercase text-zinc-500 font-semibold tracking-wider border-b border-tech-border">
								<tr>
									<th className="px-4 py-2 w-1/3">Key</th>
									<th className="px-4 py-2">Value</th>
									<th className="px-4 py-2 w-10"></th>
								</tr>
							</thead>
							<tbody className="divide-y divide-tech-border">
								{custom.map((v) => (
									<EnvRow
										key={v.id}
										env={v}
										onRemove={onRemove}
										onUpdate={onUpdate}
									/>
								))}
							</tbody>
						</table>
					) : (
						<div className="p-8 flex flex-col items-center justify-center text-zinc-600 border-b border-tech-border border-dashed">
							<Database size={24} className="mb-2 opacity-20" />
							<span className="text-xs">No custom variables defined</span>
						</div>
					)}

					<button
						onClick={onAdd}
						className="w-full py-3 text-xs text-zinc-400 hover:text-white hover:bg-white/5 transition-colors flex items-center justify-center gap-2 font-medium"
					>
						<Plus size={14} /> Add Variable
					</button>
				</div>
			</div>

			{/* SSM Parameter Store Section */}
			<div className="space-y-3">
				<div className="flex items-center justify-between px-1">
					<div>
						<h3 className="text-xs font-semibold uppercase tracking-wider text-zinc-400">
							SSM Parameter Store
						</h3>
						<p className="text-[10px] text-zinc-600 mt-0.5">
							Secure secrets mapped from AWS SSM
						</p>
					</div>
					<span className="text-xs font-mono text-zinc-500 bg-zinc-900 px-2 py-0.5 rounded border border-zinc-800">
						{ssmParams.length}
					</span>
				</div>

				<div className="bg-tech-surface border border-tech-border rounded-lg overflow-hidden flex flex-col">
					<table className="w-full text-left">
						<thead className="bg-zinc-950/50 text-[10px] uppercase text-zinc-500 font-semibold tracking-wider border-b border-tech-border">
							<tr>
								<th className="px-4 py-2 w-1/2">Parameter Name</th>
								<th className="px-4 py-2">Value</th>
								<th className="px-4 py-2 w-10"></th>
							</tr>
						</thead>
						<tbody className="divide-y divide-tech-border">
							{ssmParams.map((p) => (
								<SSMRow
									key={p.id}
									param={p}
									onRemove={onRemoveSSM}
									onUpdate={onUpdateSSM}
									onFetch={onFetchSSM}
								/>
							))}
						</tbody>
					</table>

					<button
						onClick={onAddSSM}
						className="w-full py-3 text-xs text-zinc-400 hover:text-white hover:bg-white/5 transition-colors flex items-center justify-center gap-2 font-medium"
					>
						<Plus size={14} /> Add Parameter
					</button>
				</div>
			</div>
		</div>
	);
};

const EnvRow: React.FC<{
	env: EnvVar;
	onRemove: (id: string) => void;
	onUpdate: (id: string, updates: Partial<EnvVar>) => void;
	readOnly?: boolean;
}> = ({ env, onRemove, onUpdate, readOnly }) => {
	const [showSecret, setShowSecret] = useState(false);

	return (
		<tr
			className={`group transition-colors ${readOnly ? "bg-zinc-900/30" : "hover:bg-white/[0.02]"}`}
		>
			<td className="px-4 py-2">
				<input
					type="text"
					value={env.key}
					onChange={(e) =>
						!readOnly && onUpdate(env.id, { key: e.target.value })
					}
					readOnly={readOnly}
					placeholder="KEY"
					className={`w-full bg-transparent text-sm font-mono focus:outline-none placeholder-zinc-800
                        ${readOnly ? "text-zinc-500 cursor-not-allowed" : "text-zinc-300 focus:text-white"}`}
				/>
			</td>
			<td className="px-4 py-2">
				<div className="flex items-center gap-2">
					{env.isSecret && !showSecret ? (
						<div className="flex-1 text-zinc-600 font-mono text-sm tracking-widest">
							••••••••••••••••
						</div>
					) : (
						<input
							type="text"
							value={env.value}
							onChange={(e) =>
								!readOnly && onUpdate(env.id, { value: e.target.value })
							}
							readOnly={readOnly}
							placeholder="Value"
							className={`w-full bg-transparent text-sm font-mono focus:outline-none placeholder-zinc-800
                                ${env.isSecret ? "text-yellow-500/70" : ""}
                                ${readOnly ? "text-zinc-600 cursor-not-allowed" : env.isSecret ? "text-yellow-500" : "text-zinc-400 focus:text-white"}
                            `}
						/>
					)}
				</div>
			</td>
			<td className="px-4 py-2 text-right">
				<div
					className={`flex items-center justify-end gap-2 transition-opacity ${readOnly ? "" : "opacity-0 group-hover:opacity-100"}`}
				>
					{env.isSecret && (
						<button
							onClick={() => setShowSecret(!showSecret)}
							className="text-zinc-500 hover:text-white"
						>
							{showSecret ? <EyeOff size={14} /> : <Eye size={14} />}
						</button>
					)}
					{readOnly ? (
						<Lock size={12} className="text-zinc-700" />
					) : (
						<button
							onClick={() => onRemove(env.id)}
							className="text-zinc-500 hover:text-red-400"
						>
							<Trash2 size={14} />
						</button>
					)}
				</div>
			</td>
		</tr>
	);
};

const SSMRow: React.FC<{
	param: SSMParameter;
	onRemove: (id: string) => void;
	onUpdate: (id: string, updates: Partial<SSMParameter>) => void;
	onFetch: (id: string) => void;
}> = ({ param, onRemove, onUpdate, onFetch }) => {
	return (
		<tr className="group hover:bg-white/[0.02] transition-colors">
			<td className="px-4 py-2">
				<div className="flex items-center w-full">
					<span className="text-zinc-600 font-mono text-sm select-none mr-0.5 whitespace-nowrap">
						/op/service/dev/
					</span>
					<input
						type="text"
						value={param.name}
						onChange={(e) => onUpdate(param.id, { name: e.target.value })}
						placeholder="param_name"
						className="w-full bg-transparent text-sm font-mono text-zinc-300 focus:outline-none focus:text-white placeholder-zinc-800"
					/>
				</div>
			</td>
			<td className="px-4 py-2">
				<div className="flex items-center gap-2">
					{param.isLoading ? (
						<div className="flex items-center gap-2 text-zinc-500 text-xs font-mono">
							<Loader2 size={12} className="animate-spin" />
							<span>Retrieving...</span>
						</div>
					) : param.value === null ? (
						<button
							onClick={() => onFetch(param.id)}
							className="flex items-center gap-1.5 px-2 py-1 rounded border border-zinc-800 bg-zinc-900/50 hover:bg-zinc-800 hover:text-white text-zinc-500 text-[10px] font-medium transition-colors"
						>
							<Download size={10} />
							Retrieve Value
						</button>
					) : (
						<div className="flex items-center gap-2 w-full">
							<Key size={12} className="text-purple-500 shrink-0" />
							<input
								type="text"
								value={param.value}
								onChange={(e) => onUpdate(param.id, { value: e.target.value })}
								className="w-full bg-transparent text-sm font-mono text-purple-400 focus:text-purple-300 focus:outline-none"
							/>
						</div>
					)}
				</div>
			</td>
			<td className="px-4 py-2 text-right">
				<div className="flex items-center justify-end gap-2 opacity-0 group-hover:opacity-100 transition-opacity">
					<button
						onClick={() => onRemove(param.id)}
						className="text-zinc-500 hover:text-red-400"
					>
						<Trash2 size={14} />
					</button>
				</div>
			</td>
		</tr>
	);
};

const AutoscalingSection: React.FC = () => {
	return (
		<div className="flex flex-col gap-6 animate-in fade-in slide-in-from-bottom-4 duration-500">
			<div className="border border-tech-border rounded-xl bg-tech-surface/20 shadow-sm overflow-hidden p-5 space-y-4">
				<div className="flex items-center gap-3 mb-2">
					<div className="p-1.5 rounded bg-orange-500/10 text-orange-400 border border-orange-500/20">
						<Activity size={18} />
					</div>
					<div>
						<h3 className="text-sm font-medium text-zinc-200">
							Autoscaling Configuration
						</h3>
						<p className="text-xs text-zinc-500">
							Manage task replication based on load
						</p>
					</div>
				</div>

				<div className="grid grid-cols-2 gap-4">
					<div className="space-y-1">
						<label className="text-[10px] uppercase tracking-wider font-semibold text-zinc-500">
							Min Instances
						</label>
						<input
							type="number"
							defaultValue={1}
							className="w-full bg-black border border-tech-border rounded px-3 py-2 text-sm text-zinc-300 focus:border-tech-accent focus:outline-none"
						/>
					</div>
					<div className="space-y-1">
						<label className="text-[10px] uppercase tracking-wider font-semibold text-zinc-500">
							Max Instances
						</label>
						<input
							type="number"
							defaultValue={5}
							className="w-full bg-black border border-tech-border rounded px-3 py-2 text-sm text-zinc-300 focus:border-tech-accent focus:outline-none"
						/>
					</div>
				</div>

				<div className="pt-2">
					<label className="text-[10px] uppercase tracking-wider font-semibold text-zinc-500 mb-2 block">
						Scaling Metrics
					</label>
					<div className="space-y-2">
						<div className="flex items-center justify-between p-2 rounded border border-tech-border bg-black/40">
							<div className="flex items-center gap-2">
								<Cpu size={14} className="text-zinc-500" />
								<span className="text-sm text-zinc-300">CPU Utilization</span>
							</div>
							<span className="text-sm font-mono text-tech-accent">
								{"> 70%"}
							</span>
						</div>
					</div>
				</div>
			</div>
		</div>
	);
};

const ToggleSwitch: React.FC<{
	checked?: boolean;
	onChange?: (checked: boolean) => void;
}> = ({ checked = false, onChange }) => {
	return (
		<button
			type="button"
			onClick={() => onChange && onChange(!checked)}
			className={`relative inline-flex h-5 w-9 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none ${checked ? "bg-tech-accent" : "bg-zinc-700"}`}
		>
			<span
				className={`pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out ${checked ? "translate-x-4" : "translate-x-0"}`}
			/>
		</button>
	);
};

const CopyButton: React.FC<{ text: string | null }> = ({ text }) => {
	const [copied, setCopied] = useState(false);

	const handleCopy = () => {
		if (!text) return;
		navigator.clipboard.writeText(text);
		setCopied(true);
		setTimeout(() => setCopied(false), 2000);
	};

	return (
		<button
			onClick={handleCopy}
			className="p-1.5 hover:bg-white/10 rounded text-zinc-500 hover:text-white transition-colors"
		>
			{copied ? (
				<Check size={14} className="text-emerald-500" />
			) : (
				<Copy size={14} />
			)}
		</button>
	);
};

const SectionHeader: React.FC<{ title: string; count?: number }> = ({
	title,
	count,
}) => (
	<div className="flex items-center justify-between pb-2 border-b border-tech-border">
		<h3 className="text-sm font-medium text-white">{title}</h3>
		{count !== undefined && (
			<span className="text-xs font-mono text-zinc-500 bg-zinc-900 px-2 py-0.5 rounded border border-zinc-800">
				{count}
			</span>
		)}
	</div>
);
