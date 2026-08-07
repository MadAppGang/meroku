import { ChevronDown, Eye, EyeOff, Lock, Trash2, X } from "lucide-react";
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

const CPU_OPTIONS = [
	["256", "256"],
	["512", "512"],
	["1024", "1024"],
];

const MEMORY_OPTIONS = [
	["512", "512 MB"],
	["1024", "1024 MB"],
	["2048", "2048 MB"],
];

function CompactShell({
	label,
	view,
	onViewChange,
	dirty,
	onReset,
	children,
}: {
	label: string;
	view: CompactView;
	onViewChange: (view: CompactView) => void;
	dirty: boolean;
	onReset: () => void;
	children: ReactNode;
}) {
	return (
		<aside className="meroku-properties mp-compact-panel" aria-label={label}>
			<header className="mp-compact-header">
				<div>
					<h2>terminator</h2>
					<div className="mp-compact-header__meta">
						<span className="mp-compact-header__running">● Running</span>
						<span aria-hidden="true">|</span>
						<span>ap-southeast-2</span>
					</div>
				</div>
				<div className="mp-compact-header__actions">
					<button
						type="button"
						className="mp-icon-button mp-icon-button--danger"
						aria-label="Delete terminator"
					>
						<Trash2 />
					</button>
					<button
						type="button"
						className="mp-icon-button"
						aria-label="Close properties"
					>
						<X />
					</button>
				</div>
			</header>

			<nav className="mp-tabs" aria-label="Property views">
				{(["setup", "details"] as CompactView[]).map((tab) => (
					<button
						type="button"
						key={tab}
						className="mp-tab"
						data-active={view === tab}
						aria-current={view === tab ? "page" : undefined}
						onClick={() => onViewChange(tab)}
					>
						{tab[0].toUpperCase() + tab.slice(1)}
					</button>
				))}
			</nav>

			{children}

			<footer className="mp-savebar">
				<output
					className="mp-savebar__status"
					data-state={dirty ? "dirty" : "clean"}
				>
					<span>{dirty ? "Unsaved changes" : "Last updated 2m ago"}</span>
				</output>
				<div className="mp-savebar__actions">
					<Button
						type="button"
						variant="ghost"
						className="mp-button mp-button--ghost"
						disabled={!dirty}
						onClick={onReset}
					>
						Discard
					</Button>
					<Button
						type="button"
						className="mp-button mp-button--primary"
						disabled={!dirty}
						onClick={onReset}
					>
						Apply Changes
					</Button>
				</div>
			</footer>
		</aside>
	);
}

function InlineInput({
	label,
	defaultValue,
	type = "text",
	onDirty,
}: {
	label: string;
	defaultValue: string | number;
	type?: "text" | "number";
	onDirty: () => void;
}) {
	return (
		<div className="mp-inline-field">
			<span className="mp-inline-field__label">{label}:</span>
			<Input
				aria-label={label}
				className="mp-inline-control mp-mono"
				type={type}
				defaultValue={defaultValue}
				onChange={onDirty}
			/>
		</div>
	);
}

function InlineSelect({
	label,
	defaultValue,
	options,
	onDirty,
}: {
	label: string;
	defaultValue: string;
	options: string[][];
	onDirty: () => void;
}) {
	return (
		<div className="mp-inline-field">
			<span className="mp-inline-field__label">{label}:</span>
			<Select defaultValue={defaultValue} onValueChange={onDirty}>
				<SelectTrigger aria-label={label} className="mp-inline-control">
					<SelectValue />
				</SelectTrigger>
				<SelectContent className="mp-select-content">
					{options.map(([value, display]) => (
						<SelectItem key={value} value={value}>
							{display}
						</SelectItem>
					))}
				</SelectContent>
			</Select>
		</div>
	);
}

function CompactCheck({
	label,
	checked,
	onCheckedChange,
}: {
	label: string;
	checked: boolean;
	onCheckedChange: (checked: boolean) => void;
}) {
	return (
		<div className="mp-check-option">
			<Checkbox
				aria-label={label}
				checked={checked}
				className="mp-compact-checkbox"
				onCheckedChange={(next) => onCheckedChange(Boolean(next))}
			/>
			<span>{label}</span>
		</div>
	);
}

function SectionHeading({ index, title }: { index: string; title: string }) {
	return (
		<header className="mp-alt-heading">
			<span>{index}</span>
			<h3>{title}</h3>
		</header>
	);
}

function ImageLine() {
	return (
		<div className="mp-alt-image">
			<Lock aria-hidden="true" />
			<span>Image:</span>
			<code>123456789.dkr.ecr.ap-southeast-2.amazonaws.com/terminator</code>
			<code>:latest</code>
		</div>
	);
}

function DetailView({ className }: { className: string }) {
	return (
		<div className={className}>
			<div className="mp-alt-details">
				<div>
					<span>ECS service</span>
					<code>circl_terminator_dev</code>
				</div>
				<div>
					<span>Task role</span>
					<code>circl_terminator_task_dev</code>
				</div>
				<div>
					<span>Log group</span>
					<code>/ecs/circl/terminator/dev</code>
				</div>
			</div>
		</div>
	);
}

function EnvironmentRows({
	secretVisible,
	onToggleSecret,
}: {
	secretVisible: boolean;
	onToggleSecret: () => void;
}) {
	return (
		<table className="mp-alt-env" aria-label="Environment variables">
			<thead>
				<tr>
					<th scope="col">Key</th>
					<th scope="col">Value</th>
					<th scope="col">
						<span className="sr-only">Actions</span>
					</th>
				</tr>
			</thead>
			<tbody>
				<tr>
					<td>
						<code>NODE_ENV</code>
					</td>
					<td>
						<code>production</code>
					</td>
					<td>
						<Lock aria-label="System variable" />
					</td>
				</tr>
				<tr>
					<td>
						<code>DB_HOST</code>
					</td>
					<td>
						<code>postgres.internal</code>
					</td>
					<td>
						<Lock aria-label="System variable" />
					</td>
				</tr>
				<tr>
					<td>
						<code>API_KEY</code>
					</td>
					<td>
						<code>{secretVisible ? "sk_live_example" : "••••••••••••"}</code>
					</td>
					<td>
						<button
							type="button"
							className="mp-icon-button"
							aria-label={secretVisible ? "Hide API key" : "Reveal API key"}
							onClick={onToggleSecret}
						>
							{secretVisible ? <EyeOff /> : <Eye />}
						</button>
					</td>
				</tr>
			</tbody>
		</table>
	);
}

export function ServiceCompactPairs() {
	const [view, setView] = useState<CompactView>("setup");
	const [dirty, setDirty] = useState(false);
	const [autoscaling, setAutoscaling] = useState(true);
	const [tracing, setTracing] = useState(true);
	const [remoteAccess, setRemoteAccess] = useState(false);
	const [advancedOpen, setAdvancedOpen] = useState(false);
	const [secretVisible, setSecretVisible] = useState(false);
	const markDirty = () => setDirty(true);

	return (
		<CompactShell
			label="terminator properties compact inline columns"
			view={view}
			onViewChange={setView}
			dirty={dirty}
			onReset={() => setDirty(false)}
		>
			{view === "details" ? (
				<DetailView className="mp-pairs-content" />
			) : (
				<div className="mp-pairs-content">
					<div className="mp-pairs-sheet">
						<section className="mp-pairs-section">
							<SectionHeading index="01" title="Container" />
							<ImageLine />
							<div className="mp-pairs-row mp-pairs-row--command">
								<InlineInput
									label="Command"
									defaultValue="npm, start"
									onDirty={markDirty}
								/>
							</div>
						</section>

						<section className="mp-pairs-section">
							<SectionHeading index="02" title="Runtime & Scaling" />
							<div className="mp-pairs-row">
								<InlineInput
									label="Port"
									defaultValue={8080}
									type="number"
									onDirty={markDirty}
								/>
								<InlineInput
									label="Health"
									defaultValue="/health"
									onDirty={markDirty}
								/>
							</div>
							<div className="mp-pairs-row">
								<InlineSelect
									label="CPU"
									defaultValue="512"
									options={CPU_OPTIONS}
									onDirty={markDirty}
								/>
								<InlineSelect
									label="Memory"
									defaultValue="1024"
									options={MEMORY_OPTIONS}
									onDirty={markDirty}
								/>
							</div>
							<div
								className="mp-pairs-scaling-row"
								data-autoscaling={autoscaling}
							>
								<div className="mp-pairs-autoscaling">
									<CompactCheck
										label="Autoscaling · 70%"
										checked={autoscaling}
										onCheckedChange={(checked) => {
											setAutoscaling(checked);
											markDirty();
										}}
									/>
								</div>
								{autoscaling ? (
									<>
										<InlineInput
											key="autoscaling-min"
											label="Min"
											defaultValue={1}
											type="number"
											onDirty={markDirty}
										/>
										<InlineInput
											key="autoscaling-max"
											label="Max"
											defaultValue={5}
											type="number"
											onDirty={markDirty}
										/>
									</>
								) : (
									<InlineInput
										key="desired-tasks"
										label="Desired"
										defaultValue={2}
										type="number"
										onDirty={markDirty}
									/>
								)}
							</div>
							<div className="mp-pairs-checks">
								<span>Features:</span>
								<CompactCheck
									label="X-Ray"
									checked={tracing}
									onCheckedChange={(checked) => {
										setTracing(checked);
										markDirty();
									}}
								/>
								<CompactCheck
									label="SSH"
									checked={remoteAccess}
									onCheckedChange={(checked) => {
										setRemoteAccess(checked);
										markDirty();
									}}
								/>
							</div>
						</section>

						<section className="mp-pairs-section">
							<div className="mp-alt-heading mp-alt-heading--action">
								<span>03</span>
								<h3>Environment</h3>
								<button type="button">+ Add variable</button>
							</div>
							<EnvironmentRows
								secretVisible={secretVisible}
								onToggleSecret={() => setSecretVisible((current) => !current)}
							/>
						</section>

						<div className="mp-alt-advanced">
							<button
								type="button"
								aria-expanded={advancedOpen}
								onClick={() => setAdvancedOpen((current) => !current)}
							>
								<span>Advanced</span>
								<ChevronDown data-open={advancedOpen} />
							</button>
							{advancedOpen && (
								<div className="mp-pairs-row">
									<InlineInput
										label="Timeout"
										defaultValue={15}
										type="number"
										onDirty={markDirty}
									/>
									<InlineInput
										label="Host port"
										defaultValue={8080}
										type="number"
										onDirty={markDirty}
									/>
								</div>
							)}
						</div>
					</div>
				</div>
			)}
		</CompactShell>
	);
}

function LedgerCell({
	label,
	children,
}: {
	label: string;
	children: ReactNode;
}) {
	return (
		<div className="mp-ledger-cell">
			<span>{label}:</span>
			{children}
		</div>
	);
}

function LedgerInput({
	label,
	defaultValue,
	type = "text",
	onDirty,
}: {
	label: string;
	defaultValue: string | number;
	type?: "text" | "number";
	onDirty: () => void;
}) {
	return (
		<LedgerCell label={label}>
			<Input
				aria-label={`${label} ledger`}
				className="mp-ledger-control mp-mono"
				type={type}
				defaultValue={defaultValue}
				onChange={onDirty}
			/>
		</LedgerCell>
	);
}

function LedgerSelect({
	label,
	defaultValue,
	options,
	onDirty,
}: {
	label: string;
	defaultValue: string;
	options: string[][];
	onDirty: () => void;
}) {
	return (
		<LedgerCell label={label}>
			<Select defaultValue={defaultValue} onValueChange={onDirty}>
				<SelectTrigger
					aria-label={`${label} ledger`}
					className="mp-ledger-control"
				>
					<SelectValue />
				</SelectTrigger>
				<SelectContent className="mp-select-content">
					{options.map(([value, display]) => (
						<SelectItem key={value} value={value}>
							{display}
						</SelectItem>
					))}
				</SelectContent>
			</Select>
		</LedgerCell>
	);
}

export function ServiceCompactLedger() {
	const [view, setView] = useState<CompactView>("setup");
	const [dirty, setDirty] = useState(false);
	const [autoscaling, setAutoscaling] = useState(true);
	const [tracing, setTracing] = useState(true);
	const [remoteAccess, setRemoteAccess] = useState(false);
	const [advancedOpen, setAdvancedOpen] = useState(false);
	const markDirty = () => setDirty(true);

	return (
		<CompactShell
			label="terminator properties compact ledger"
			view={view}
			onViewChange={setView}
			dirty={dirty}
			onReset={() => setDirty(false)}
		>
			{view === "details" ? (
				<DetailView className="mp-ledger-content" />
			) : (
				<div className="mp-ledger-content">
					<div className="mp-ledger-sheet">
						<div className="mp-ledger-row mp-ledger-row--image">
							<strong>Container</strong>
							<div className="mp-ledger-image">
								<Lock aria-hidden="true" />
								<code>
									123456789.dkr.ecr.ap-southeast-2.amazonaws.com/terminator:latest
								</code>
							</div>
						</div>
						<div className="mp-ledger-row">
							<strong>Process</strong>
							<div className="mp-ledger-values mp-ledger-values--two">
								<LedgerInput
									label="Command"
									defaultValue="npm, start"
									onDirty={markDirty}
								/>
								<LedgerInput
									label="Port"
									defaultValue={8080}
									type="number"
									onDirty={markDirty}
								/>
							</div>
						</div>
						<div className="mp-ledger-row">
							<strong>Network</strong>
							<div className="mp-ledger-values mp-ledger-values--two">
								<LedgerInput
									label="Port"
									defaultValue={8080}
									type="number"
									onDirty={markDirty}
								/>
								<LedgerInput
									label="Health"
									defaultValue="/health"
									onDirty={markDirty}
								/>
							</div>
						</div>
						<div className="mp-ledger-row">
							<strong>Resources</strong>
							<div className="mp-ledger-values mp-ledger-values--two">
								<LedgerSelect
									label="CPU"
									defaultValue="512"
									options={CPU_OPTIONS}
									onDirty={markDirty}
								/>
								<LedgerSelect
									label="Memory"
									defaultValue="1024"
									options={MEMORY_OPTIONS}
									onDirty={markDirty}
								/>
							</div>
						</div>
						<div className="mp-ledger-row">
							<strong>Scaling</strong>
							<div
								className="mp-ledger-scaling-values"
								data-autoscaling={autoscaling}
							>
								<div className="mp-ledger-autoscaling">
									<CompactCheck
										label="Autoscaling 70%"
										checked={autoscaling}
										onCheckedChange={(checked) => {
											setAutoscaling(checked);
											markDirty();
										}}
									/>
								</div>
								{autoscaling ? (
									<>
										<LedgerInput
											key="autoscaling-min"
											label="Min"
											defaultValue={1}
											type="number"
											onDirty={markDirty}
										/>
										<LedgerInput
											key="autoscaling-max"
											label="Max"
											defaultValue={5}
											type="number"
											onDirty={markDirty}
										/>
									</>
								) : (
									<LedgerInput
										key="desired-tasks"
										label="Desired"
										defaultValue={2}
										type="number"
										onDirty={markDirty}
									/>
								)}
							</div>
						</div>
						<div className="mp-ledger-row">
							<strong>Features</strong>
							<div className="mp-ledger-checks">
								<CompactCheck
									label="X-Ray"
									checked={tracing}
									onCheckedChange={(checked) => {
										setTracing(checked);
										markDirty();
									}}
								/>
								<CompactCheck
									label="SSH"
									checked={remoteAccess}
									onCheckedChange={(checked) => {
										setRemoteAccess(checked);
										markDirty();
									}}
								/>
							</div>
						</div>
						<div className="mp-ledger-row mp-ledger-row--env">
							<strong>Environment</strong>
							<div className="mp-ledger-env">
								<span>
									<code>NODE_ENV</code>
									<b>production</b>
								</span>
								<span>
									<code>DB_HOST</code>
									<b>postgres.internal</b>
								</span>
								<span>
									<code>API_KEY</code>
									<b>••••••••••••</b>
								</span>
								<button type="button">+ Add</button>
							</div>
						</div>
						<div className="mp-alt-advanced">
							<button
								type="button"
								aria-expanded={advancedOpen}
								onClick={() => setAdvancedOpen((current) => !current)}
							>
								<span>Advanced</span>
								<ChevronDown data-open={advancedOpen} />
							</button>
							{advancedOpen && (
								<div className="mp-ledger-row">
									<strong>Deployment</strong>
									<div className="mp-ledger-values mp-ledger-values--two">
										<LedgerInput
											label="Timeout"
											defaultValue={15}
											type="number"
											onDirty={markDirty}
										/>
										<LedgerInput
											label="Host port"
											defaultValue={8080}
											type="number"
											onDirty={markDirty}
										/>
									</div>
								</div>
							)}
						</div>
					</div>
				</div>
			)}
		</CompactShell>
	);
}
