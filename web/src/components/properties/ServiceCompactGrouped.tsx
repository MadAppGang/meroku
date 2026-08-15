import {
	Activity,
	Braces,
	ChevronDown,
	Container,
	Eye,
	EyeOff,
	Lock,
	ShieldCheck,
	Terminal,
	Trash2,
	X,
} from "lucide-react";
import { type ReactNode, useState } from "react";
import { Button } from "../ui/button";
import { Checkbox } from "../ui/checkbox";
import { Input } from "../ui/input";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "../ui/select";
import "./property-panel.css";

type CompactView = "setup" | "details";
type ImageMode = "default" | "custom" | "shared";

const cpuOptions = [
	["256", "256 (0.25 vCPU)"],
	["512", "512 (0.5 vCPU)"],
	["1024", "1024 (1 vCPU)"],
];

const memoryOptions = [
	["512", "512 MB"],
	["1024", "1024 MB"],
	["2048", "2048 MB"],
];

function Group({
	title,
	description,
	icon,
	action,
	children,
}: {
	title: string;
	description: string;
	icon: ReactNode;
	action?: ReactNode;
	children: ReactNode;
}) {
	return (
		<section className="mp-compact-group">
			<header className="mp-compact-group__header">
				<div className="mp-compact-group__identity">
					<span className="mp-compact-group__icon" aria-hidden="true">
						{icon}
					</span>
					<div>
						<h3>{title}</h3>
						<p>{description}</p>
					</div>
				</div>
				{action}
			</header>
			<div className="mp-compact-group__body">{children}</div>
		</section>
	);
}

function Field({ label, children }: { label: string; children: ReactNode }) {
	return (
		<div className="mp-compact-field">
			<div className="mp-compact-field__label">{label}</div>
			{children}
		</div>
	);
}

function ResourceSelect({
	label,
	value,
	options,
	onDirty,
}: {
	label: string;
	value: string;
	options: string[][];
	onDirty: () => void;
}) {
	return (
		<Field label={label}>
			<Select defaultValue={value} onValueChange={onDirty}>
				<SelectTrigger aria-label={label} className="mp-control">
					<SelectValue />
				</SelectTrigger>
				<SelectContent className="mp-select-content">
					{options.map(([optionValue, optionLabel]) => (
						<SelectItem key={optionValue} value={optionValue}>
							{optionLabel}
						</SelectItem>
					))}
				</SelectContent>
			</Select>
		</Field>
	);
}

function ReadonlyRows({ rows }: { rows: string[][] }) {
	return (
		<div className="mp-compact-readonly">
			{rows.map(([label, value]) => (
				<div className="mp-compact-readonly__row" key={label}>
					<span>{label}</span>
					<code>{value}</code>
				</div>
			))}
		</div>
	);
}

export function ServiceCompactGrouped({
	mode = "manage",
}: {
	mode?: "manage" | "create";
}) {
	const [activeView, setActiveView] = useState<CompactView>("setup");
	const [imageMode, setImageMode] = useState<ImageMode>("default");
	const [autoscaling, setAutoscaling] = useState(true);
	const [tracing, setTracing] = useState(true);
	const [remoteAccess, setRemoteAccess] = useState(false);
	const [advancedOpen, setAdvancedOpen] = useState(false);
	const [secretVisible, setSecretVisible] = useState(false);
	const [dirty, setDirty] = useState(false);
	const markDirty = () => setDirty(true);
	const isCreate = mode === "create";
	const containerSection = (
		<Group
			title="Container & Process"
			description="Setup-time image and entrypoint"
			icon={<Container />}
			action={
				<fieldset className="mp-compact-segments" aria-label="Image source">
					{(["default", "custom", "shared"] as ImageMode[]).map(
						(imageSource) => (
							<button
								type="button"
								key={imageSource}
								data-active={imageMode === imageSource}
								aria-pressed={imageMode === imageSource}
								onClick={() => {
									setImageMode(imageSource);
									markDirty();
								}}
							>
								{imageSource[0].toUpperCase() + imageSource.slice(1)}
							</button>
						),
					)}
				</fieldset>
			}
		>
			<div className="mp-compact-image-row">
				<Lock aria-hidden="true" />
				<code>
					{imageMode === "default"
						? "123456789.dkr.ecr.ap-southeast-2.amazonaws.com/terminator"
						: imageMode === "custom"
							? "docker.io/library/terminator"
							: "shared/auth-service"}
				</code>
				<span>: latest</span>
			</div>
			<div className="mp-compact-fields mp-compact-fields--single">
				<Field label="Command override">
					<Input
						aria-label="Command override"
						className="mp-control mp-mono"
						defaultValue="npm, start"
						onChange={markDirty}
					/>
				</Field>
			</div>
		</Group>
	);

	return (
		<aside
			className="meroku-properties mp-compact-panel"
			data-mode={mode}
			aria-label={
				isCreate
					? "create service compact grouped"
					: "terminator properties compact grouped"
			}
		>
			<header className="mp-compact-header">
				<div>
					<h2>{isCreate ? "Create service" : "terminator"}</h2>
					<div className="mp-compact-header__meta">
						{isCreate ? (
							<span>New ECS service · ap-southeast-2</span>
						) : (
							<>
								<span className="mp-compact-header__running">● Running</span>
								<span aria-hidden="true">|</span>
								<span>ap-southeast-2</span>
							</>
						)}
					</div>
				</div>
				<div className="mp-compact-header__actions">
					{!isCreate && (
						<button
							type="button"
							className="mp-icon-button mp-icon-button--danger"
							aria-label="Delete terminator"
						>
							<Trash2 />
						</button>
					)}
					<button
						type="button"
						className="mp-icon-button"
						aria-label="Close properties"
					>
						<X />
					</button>
				</div>
			</header>

			{!isCreate && (
				<nav className="mp-tabs" aria-label="Property views">
					{(["setup", "details"] as CompactView[]).map((view) => (
						<button
							type="button"
							key={view}
							className="mp-tab"
							data-active={activeView === view}
							aria-current={activeView === view ? "page" : undefined}
							onClick={() => setActiveView(view)}
						>
							{view[0].toUpperCase() + view.slice(1)}
						</button>
					))}
				</nav>
			)}

			<div className="mp-compact-content">
				{activeView === "setup" && (
					<div className="mp-compact-sheet">
						{isCreate && containerSection}

						<Group
							title="Runtime & Scaling"
							description="Network, resources, and task count"
							icon={<Activity />}
						>
							<div className="mp-compact-fields">
								<Field label="Port">
									<Input
										aria-label="Port grouped"
										className="mp-control mp-mono"
										type="number"
										defaultValue={8080}
										onChange={markDirty}
									/>
								</Field>
								<Field label="Health path">
									<Input
										aria-label="Health path"
										className="mp-control mp-mono"
										defaultValue="/health"
										onChange={markDirty}
									/>
								</Field>
							</div>
							<div className="mp-compact-fields">
								<ResourceSelect
									label="CPU"
									value="512"
									options={cpuOptions}
									onDirty={markDirty}
								/>
								<ResourceSelect
									label="Memory"
									value="1024"
									options={memoryOptions}
									onDirty={markDirty}
								/>
							</div>
							<div className="mp-compact-scaling-row">
								<div className="mp-compact-scaling-toggle">
									<Checkbox
										aria-label="Autoscaling grouped"
										checked={autoscaling}
										className="mp-compact-checkbox"
										onCheckedChange={(checked) => {
											setAutoscaling(Boolean(checked));
											markDirty();
										}}
									/>
									<span>Autoscaling</span>
									{autoscaling && <small>target 70%</small>}
								</div>
								<div
									className="mp-compact-scaling-fields"
									data-autoscaling={autoscaling}
								>
									{autoscaling ? (
										<>
											<Field key="autoscaling-min" label="Min">
												<Input
													aria-label="Minimum grouped"
													className="mp-control mp-mono"
													type="number"
													defaultValue={1}
													onChange={markDirty}
												/>
											</Field>
											<Field key="autoscaling-max" label="Max">
												<Input
													aria-label="Maximum grouped"
													className="mp-control mp-mono"
													type="number"
													defaultValue={5}
													onChange={markDirty}
												/>
											</Field>
										</>
									) : (
										<Field key="desired-tasks" label="Desired">
											<Input
												aria-label="Desired grouped"
												className="mp-control mp-mono"
												type="number"
												defaultValue={2}
												onChange={markDirty}
											/>
										</Field>
									)}
								</div>
							</div>
							<div className="mp-compact-runtime-row mp-compact-runtime-row--features">
								<div className="mp-compact-capabilities">
									{[
										["X-Ray", tracing, setTracing],
										["SSH", remoteAccess, setRemoteAccess],
									].map(([label, checked, setter]) => (
										<div className="mp-compact-capability" key={String(label)}>
											<Checkbox
												aria-label={String(label)}
												checked={Boolean(checked)}
												className="mp-compact-checkbox"
												onCheckedChange={(next) => {
													(setter as (value: boolean) => void)(Boolean(next));
													markDirty();
												}}
											/>
											<span>{String(label)}</span>
										</div>
									))}
								</div>
							</div>
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
							<table
								className="mp-compact-env"
								aria-label="Environment variables"
							>
								<thead>
									<tr className="mp-compact-env__head">
										<th scope="col">Key</th>
										<th scope="col">Value</th>
										<th scope="col">
											<span className="sr-only">Actions</span>
										</th>
									</tr>
								</thead>
								<tbody>
									{[
										["NODE_ENV", "production", "system"],
										["DB_HOST", "postgres.internal", "system"],
										[
											"API_KEY",
											secretVisible ? "sk_live_example" : "••••••••••••••",
											"secret",
										],
									].map(([key, value, kind]) => (
										<tr className="mp-compact-env__row" key={key}>
											<td>
												<code>{key}</code>
											</td>
											<td>
												<code>{value}</code>
											</td>
											<td>
												{kind === "system" ? (
													<Lock aria-label="System variable" />
												) : (
													<button
														type="button"
														className="mp-icon-button"
														aria-label={
															secretVisible ? "Hide API key" : "Reveal API key"
														}
														onClick={() =>
															setSecretVisible((current) => !current)
														}
													>
														{secretVisible ? <EyeOff /> : <Eye />}
													</button>
												)}
											</td>
										</tr>
									))}
								</tbody>
							</table>
						</Group>

						<div className="mp-compact-advanced">
							<button
								type="button"
								aria-expanded={advancedOpen}
								onClick={() => setAdvancedOpen((current) => !current)}
							>
								<span>Advanced settings</span>
								<ChevronDown data-open={advancedOpen} />
							</button>
							{advancedOpen && (
								<div className="mp-compact-advanced__body">
									<Field label="Deploy timeout">
										<Input
											aria-label="Deploy timeout"
											className="mp-control mp-mono"
											type="number"
											defaultValue={15}
										/>
									</Field>
									<Field label="Host port">
										<Input
											aria-label="Host port"
											className="mp-control mp-mono"
											type="number"
											defaultValue={8080}
										/>
									</Field>
								</div>
							)}
						</div>
					</div>
				)}

				{activeView === "details" && (
					<div className="mp-compact-sheet">
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
					</div>
				)}
			</div>

			<footer className="mp-savebar">
				<output
					className="mp-savebar__status"
					data-state={dirty ? "dirty" : "clean"}
				>
					<span>
						{isCreate
							? "Ready to create"
							: dirty
								? "Unsaved changes"
								: "Last updated 2m ago"}
					</span>
				</output>
				<div className="mp-savebar__actions">
					<Button
						type="button"
						variant="ghost"
						className="mp-button mp-button--ghost"
						disabled={!dirty && !isCreate}
						onClick={() => setDirty(false)}
					>
						{isCreate ? "Cancel" : "Discard"}
					</Button>
					<Button
						type="button"
						className="mp-button mp-button--primary"
						disabled={!dirty && !isCreate}
						onClick={() => setDirty(false)}
					>
						{isCreate ? "Create service" : "Apply Changes"}
					</Button>
				</div>
			</footer>
		</aside>
	);
}
