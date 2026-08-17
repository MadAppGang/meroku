import { Activity, Braces, ShieldCheck, Terminal } from "lucide-react";
import { useId, useState } from "react";
import {
	PropertyGroup as Group,
	PropertyAdvancedSettings,
	PropertyAutoscalingGroup,
	PropertyCapabilities,
	PropertyContainerProcess,
	type PropertyEnvironmentVariable,
	PropertyEnvironmentVariables,
	PropertyFieldRow,
	PropertyPanelShell,
	PropertyReadonlyRows as ReadonlyRows,
} from "./PropertyLayouts";
import {
	PropertyAutoscalingTarget,
	PropertyEditableField,
	type PropertyImageMode,
	PropertySaveBar,
	PropertySelectField,
} from "./PropertyPrimitives";
import "./property-panel.css";

type CompactView = "setup" | "details";
type CompactTheme = "dark" | "light";

const cpuOptions = [
	{ value: "256", label: "256 (0.25 vCPU)" },
	{ value: "512", label: "512 (0.5 vCPU)" },
	{ value: "1024", label: "1024 (1 vCPU)" },
];

const memoryOptions = [
	{ value: "512", label: "512 MB" },
	{ value: "1024", label: "1024 MB" },
	{ value: "2048", label: "2048 MB" },
];

export function ServiceProperties({
	mode = "manage",
	theme = "dark",
}: {
	mode?: "manage" | "create";
	theme?: CompactTheme;
}) {
	const fieldId = useId();
	const [activeView, setActiveView] = useState<CompactView>("setup");
	const [imageMode, setImageMode] = useState<PropertyImageMode>("default");
	const [commandOverride, setCommandOverride] = useState("npm, start");
	const [port, setPort] = useState(8080);
	const [healthPath, setHealthPath] = useState("/health");
	const [cpu, setCpu] = useState("512");
	const [memory, setMemory] = useState("1024");
	const [minimumTasks, setMinimumTasks] = useState(1);
	const [maximumTasks, setMaximumTasks] = useState(5);
	const [desiredTasks, setDesiredTasks] = useState(2);
	const [deployTimeout, setDeployTimeout] = useState(15);
	const [hostPort, setHostPort] = useState(8080);
	const [autoscaling, setAutoscaling] = useState(true);
	const [targetUtilization, setTargetUtilization] = useState(70);
	const [tracing, setTracing] = useState(true);
	const [remoteAccess, setRemoteAccess] = useState(false);
	const [advancedOpen, setAdvancedOpen] = useState(false);
	const [dirty, setDirty] = useState(false);
	const markDirty = () => setDirty(true);
	const isCreate = mode === "create";
	const environmentVariables: PropertyEnvironmentVariable[] = [
		{
			id: "port",
			key: "PORT",
			value: "8080",
			origin: "system",
			badge: "System",
			title: "Assigned automatically by Meroku",
		},
		{
			id: "environment",
			key: "ENVIRONMENT",
			value: "production",
			origin: "system",
			badge: "System",
			title: "Assigned automatically by Meroku",
		},
		{
			id: "log-level",
			key: "LOG_LEVEL",
			value: "info",
			origin: "custom",
			badge: "Manual",
			title: "Configured manually for this service",
		},
		{
			id: "db-host",
			key: "DB_HOST",
			value: "postgres.internal",
			origin: "service",
			badge: "postgres",
			title: "Assigned automatically by the postgres service",
		},
		{
			id: "api-key",
			key: "API_KEY",
			value: "sk_live_example",
			origin: "secret",
			badge: "shared/prod",
			title: "Inherited from the shared/prod secret group",
		},
		{
			id: "cache-url",
			key: "CACHE_URL",
			value: "redis://redis.internal:6379",
			origin: "service",
			badge: "redis",
			title: "Assigned automatically by the redis service",
		},
		{
			id: "feature-flag-beta",
			key: "FEATURE_FLAG_BETA",
			value: "false",
			origin: "custom",
			badge: "Manual",
			title: "Configured manually for this service",
		},
		{
			id: "aws-region",
			key: "AWS_REGION",
			value: "ap-southeast-2",
			origin: "system",
			badge: "System",
			title: "Assigned automatically by Meroku",
		},
	];
	const containerSection = (
		<PropertyContainerProcess
			imageMode={imageMode}
			imageValues={{
				default: "123456789.dkr.ecr.ap-southeast-2.amazonaws.com/terminator",
				custom: "docker.io/library/terminator",
				shared: "shared/auth-service",
			}}
			commandValue={commandOverride}
			onImageModeChange={(mode) => {
				setImageMode(mode);
				markDirty();
			}}
			onCommandChange={(value) => {
				setCommandOverride(value);
				markDirty();
			}}
		/>
	);
	const saveFooter = (
		<PropertySaveBar
			state={dirty ? "dirty" : "clean"}
			lastUpdated="Last updated 2m ago"
			cleanLabel={isCreate ? "Ready to create" : undefined}
			discardLabel={isCreate ? "Cancel" : "Discard"}
			primaryLabel={isCreate ? "Create service" : "Apply Changes"}
			allowCleanActions={isCreate}
			showStatusIcon={false}
			onDiscard={() => setDirty(false)}
			onApply={() => setDirty(false)}
		/>
	);

	return (
		<PropertyPanelShell
			name={isCreate ? "Create service" : "terminator"}
			ariaLabel={
				isCreate ? "create service properties" : "terminator service properties"
			}
			theme={theme}
			mode={mode}
			deletable={!isCreate}
			views={
				isCreate
					? []
					: [
							{ id: "setup", label: "Setup" },
							{ id: "details", label: "Details" },
						]
			}
			activeView={activeView}
			onViewChange={(view) => setActiveView(view as CompactView)}
			footer={saveFooter}
			meta={
				isCreate ? (
					<span>New ECS service · ap-southeast-2</span>
				) : (
					<>
						<span className="mp-compact-header__running">● Running</span>
						<span aria-hidden="true">|</span>
						<span>ap-southeast-2</span>
					</>
				)
			}
		>
			{activeView === "setup" && (
				<>
					{isCreate && containerSection}

					<Group
						title="Runtime & Scaling"
						description="Network, resources, and task count"
						icon={<Activity />}
					>
						<PropertyFieldRow variant="runtime">
							<PropertyEditableField
								id={`${fieldId}-port`}
								label="Port"
								type="number"
								value={port}
								mono
								onChange={(value) => {
									setPort(Number(value));
									markDirty();
								}}
							/>
							<PropertyEditableField
								id={`${fieldId}-health-path`}
								label="Health path"
								value={healthPath}
								mono
								onChange={(value) => {
									setHealthPath(String(value));
									markDirty();
								}}
							/>
						</PropertyFieldRow>
						<PropertyFieldRow variant="runtime">
							<PropertySelectField
								id={`${fieldId}-cpu`}
								label="CPU"
								value={cpu}
								options={cpuOptions}
								theme={theme}
								onChange={(value) => {
									setCpu(value);
									markDirty();
								}}
							/>
							<PropertySelectField
								id={`${fieldId}-memory`}
								label="Memory"
								value={memory}
								options={memoryOptions}
								theme={theme}
								onChange={(value) => {
									setMemory(value);
									markDirty();
								}}
							/>
						</PropertyFieldRow>
						<PropertyAutoscalingGroup
							enabled={autoscaling}
							ariaLabel="Autoscaling grouped"
							onEnabledChange={(enabled) => {
								setAutoscaling(enabled);
								markDirty();
							}}
							target={
								<PropertyAutoscalingTarget
									value={targetUtilization}
									theme={theme}
									onChange={(value) => {
										setTargetUtilization(value);
										markDirty();
									}}
								/>
							}
							enabledFields={
								<>
									<PropertyEditableField
										key="autoscaling-min"
										id={`${fieldId}-autoscaling-min`}
										label="Min"
										type="number"
										value={minimumTasks}
										mono
										onChange={(value) => {
											setMinimumTasks(Number(value));
											markDirty();
										}}
									/>
									<PropertyEditableField
										key="autoscaling-max"
										id={`${fieldId}-autoscaling-max`}
										label="Max"
										type="number"
										value={maximumTasks}
										mono
										onChange={(value) => {
											setMaximumTasks(Number(value));
											markDirty();
										}}
									/>
								</>
							}
							disabledFields={
								<PropertyEditableField
									key="desired-tasks"
									id={`${fieldId}-desired-tasks`}
									label="Desired"
									type="number"
									value={desiredTasks}
									mono
									onChange={(value) => {
										setDesiredTasks(Number(value));
										markDirty();
									}}
								/>
							}
						/>
						<PropertyCapabilities
							items={[
								{
									id: "x-ray",
									label: "X-Ray",
									checked: tracing,
									onCheckedChange: (checked) => {
										setTracing(checked);
										markDirty();
									},
								},
								{
									id: "ssh",
									label: "SSH",
									checked: remoteAccess,
									onCheckedChange: (checked) => {
										setRemoteAccess(checked);
										markDirty();
									},
								},
							]}
						/>
					</Group>

					<Group
						title="Environment"
						description="Injected values and overrides"
						icon={<Braces />}
						action={
							<button type="button" className="mp-compact-link">
								+ Add variable
							</button>
						}
					>
						<PropertyEnvironmentVariables variables={environmentVariables} />
					</Group>

					<PropertyAdvancedSettings
						open={advancedOpen}
						onOpenChange={setAdvancedOpen}
					>
						<PropertyFieldRow>
							<PropertyEditableField
								id={`${fieldId}-deploy-timeout`}
								label="Deploy timeout"
								type="number"
								value={deployTimeout}
								unit="min"
								mono
								onChange={(value) => {
									setDeployTimeout(Number(value));
									markDirty();
								}}
							/>
							<PropertyEditableField
								id={`${fieldId}-host-port`}
								label="Host port"
								type="number"
								value={hostPort}
								mono
								onChange={(value) => {
									setHostPort(Number(value));
									markDirty();
								}}
							/>
						</PropertyFieldRow>
					</PropertyAdvancedSettings>
				</>
			)}

			{activeView === "details" && (
				<>
					{!isCreate && containerSection}
					<Group
						title="Runtime Status"
						description="Current deployment details"
						icon={<Activity />}
					>
						<ReadonlyRows
							rows={[
								["Health", "Healthy · 2/2 tasks"],
								["ECS service", "circl_terminator_dev"],
								["Log group", "/ecs/circl/terminator/dev"],
							]}
						/>
					</Group>
					<Group
						title="IAM & Parameters"
						description="Runtime permissions and secrets"
						icon={<ShieldCheck />}
					>
						<ReadonlyRows
							rows={[
								["Task role", "circl_terminator_task_dev"],
								["Execution role", "circl_terminator_execution_dev"],
								["Parameter prefix", "/dev/circl/terminator/"],
							]}
						/>
					</Group>
					<Group
						title="Remote Shell"
						description="ECS Exec access"
						icon={<Terminal />}
					>
						<ReadonlyRows
							rows={[["Status", remoteAccess ? "Enabled" : "Disabled"]]}
						/>
					</Group>
				</>
			)}
		</PropertyPanelShell>
	);
}

// Backwards-compatible alias for stories that are no longer loaded by Storybook.
export const ServiceCompactGrouped = ServiceProperties;
