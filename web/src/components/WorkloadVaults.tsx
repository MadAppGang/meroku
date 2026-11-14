import { Plus, Trash2, Vault } from "lucide-react";
import { useId, useState } from "react";
import type { ComponentNode } from "../types";
import type {
	VaultAccess,
	VaultConnection,
	YamlInfrastructureConfig,
} from "../types/yamlConfig";
import {
	vaultNames,
	vaultPath,
	vaultPrefixError,
	vaultSecretName,
} from "../utils/vaults";
import { Alert, AlertDescription } from "./ui/alert";
import { Button } from "./ui/button";
import {
	Card,
	CardContent,
	CardDescription,
	CardHeader,
	CardTitle,
} from "./ui/card";
import { Input } from "./ui/input";
import { Label } from "./ui/label";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "./ui/select";

interface VaultConnectionsEditorProps {
	config: YamlInfrastructureConfig;
	connections: VaultConnection[];
	onChange: (index: number, patch: Partial<VaultConnection>) => void;
	onAdd: (connection: VaultConnection) => void;
	onRemove: (index: number) => void;
}

/**
 * A workload's vault connections. It never sees the workload or the list the
 * workload lives in: every edit goes to the owner as (index, patch), and the
 * owner applies it to the current config.
 */
export function VaultConnectionsEditor({
	config,
	connections,
	onChange,
	onAdd,
	onRemove,
}: VaultConnectionsEditorProps) {
	const declared = vaultNames(config);
	const connected = new Set(connections.map((c) => c.name));
	const available = declared.filter((n) => !connected.has(n));

	return (
		<div className="space-y-4">
			{declared.length === 0 && (
				<p className="text-sm text-muted-foreground">
					This environment has no vaults yet. Add one on the Vaults node.
				</p>
			)}

			{connections.map((connection, index) => (
				<VaultConnectionRow
					key={connection.name}
					config={config}
					connection={connection}
					choices={[connection.name, ...available]}
					declared={declared.includes(connection.name)}
					onChange={(patch) => onChange(index, patch)}
					onRemove={() => onRemove(index)}
				/>
			))}

			{available.length > 0 && (
				<Button
					variant="outline"
					size="sm"
					onClick={() => onAdd({ name: available[0], access: "read" })}
				>
					<Plus className="w-4 h-4" />
					Connect a vault
				</Button>
			)}
		</div>
	);
}

interface VaultConnectionRowProps {
	config: YamlInfrastructureConfig;
	connection: VaultConnection;
	choices: string[];
	declared: boolean;
	onChange: (patch: Partial<VaultConnection>) => void;
	onRemove: () => void;
}

function VaultConnectionRow({
	config,
	connection,
	choices,
	declared,
	onChange,
	onRemove,
}: VaultConnectionRowProps) {
	const uid = useId();
	// The prefix is saved when it is valid and the field is left, so a
	// half-typed prefix never reaches the YAML.
	const [prefixDraft, setPrefixDraft] = useState(connection.prefix || "");
	const prefixError = vaultPrefixError(prefixDraft);

	const commitPrefix = () => {
		if (prefixError || prefixDraft === (connection.prefix || "")) return;
		onChange({ prefix: prefixDraft === "" ? undefined : prefixDraft });
	};

	const savedPrefix = connection.prefix || "";

	return (
		<Card data-testid={`vault-connection-${connection.name}`}>
			<CardHeader>
				<CardTitle>{connection.name}</CardTitle>
				<CardDescription>
					<code>{vaultPath(config, connection.name)}</code>
				</CardDescription>
			</CardHeader>
			<CardContent className="space-y-3">
				{!declared && (
					<Alert variant="destructive">
						<AlertDescription>
							<span>
								The vault "{connection.name}" is not in this environment's
								vaults; generate refuses the connection until it is removed.
							</span>
						</AlertDescription>
					</Alert>
				)}
				<div className="flex items-end gap-2">
					<div className="flex-1 space-y-1">
						<Label htmlFor={`${uid}-vault`}>Vault</Label>
						<Select
							value={connection.name}
							onValueChange={(name) => onChange({ name })}
						>
							<SelectTrigger
								id={`${uid}-vault`}
								aria-label={`Vault of connection ${connection.name}`}
							>
								<SelectValue />
							</SelectTrigger>
							<SelectContent>
								{choices.map((name) => (
									<SelectItem key={name} value={name}>
										{name}
									</SelectItem>
								))}
							</SelectContent>
						</Select>
					</div>
					<div className="flex-1 space-y-1">
						<Label htmlFor={`${uid}-access`}>Access</Label>
						<Select
							value={connection.access || "read"}
							onValueChange={(access) =>
								onChange({ access: access as VaultAccess })
							}
						>
							<SelectTrigger
								id={`${uid}-access`}
								aria-label={`Access to vault ${connection.name}`}
							>
								<SelectValue />
							</SelectTrigger>
							<SelectContent>
								<SelectItem value="read">Read</SelectItem>
								<SelectItem value="read_write">Read and write</SelectItem>
							</SelectContent>
						</Select>
					</div>
					<Button
						variant="ghost"
						size="sm"
						aria-label={`Disconnect vault ${connection.name}`}
						onClick={onRemove}
					>
						<Trash2 className="w-4 h-4" />
					</Button>
				</div>
				<div className="space-y-1">
					<Label htmlFor={`${uid}-prefix`}>Prefix (optional)</Label>
					<Input
						id={`${uid}-prefix`}
						placeholder="PAY"
						value={prefixDraft}
						aria-invalid={prefixError !== null}
						onChange={(e) => setPrefixDraft(e.target.value.trim())}
						onBlur={commitPrefix}
						onKeyDown={(e) => {
							if (e.key === "Enter") commitPrefix();
						}}
					/>
					{prefixError ? (
						<p className="text-sm text-danger">{prefixError}</p>
					) : (
						<p className="text-xs text-muted-foreground">
							{vaultPath(config, connection.name)}database_url becomes{" "}
							<span className="font-mono">
								{vaultSecretName(savedPrefix, "database_url")}
							</span>
							.
							{savedPrefix === "" &&
								" Without a prefix, a parameter named env would become ENV, which meroku reserves; plan refuses it."}
						</p>
					)}
				</div>
				{connection.access === "read_write" && (
					<p className="text-xs text-muted-foreground">
						The application may also create and update this vault's parameters.
						It cannot delete them.
					</p>
				)}
			</CardContent>
		</Card>
	);
}

interface WorkloadVaultsProps {
	config: YamlInfrastructureConfig;
	node: ComponentNode;
	onConfigChange: (config: Partial<YamlInfrastructureConfig>) => void;
}

type WorkloadKind = "backend" | "service" | "scheduled-task" | "event-task";

function workloadOf(node: ComponentNode): {
	kind: WorkloadKind;
	name: string;
} | null {
	switch (node.type) {
		case "backend":
			return { kind: "backend", name: "backend" };
		case "service":
			return { kind: "service", name: node.id.replace(/^service-/, "") };
		case "scheduled-task":
			return {
				kind: "scheduled-task",
				name: node.id.replace(/^scheduled-/, ""),
			};
		case "event-task":
			return { kind: "event-task", name: node.id.replace(/^event-/, "") };
		default:
			return null;
	}
}

/**
 * The Vaults tab of a workload. This component is the single owner of the
 * workload's `vaults` list: the editor hands it (index, patch), and it writes
 * the workload's whole list, computed from the current config, into the one
 * entry of the one list the workload lives in.
 */
export function WorkloadVaults({
	config,
	node,
	onConfigChange,
}: WorkloadVaultsProps) {
	const workload = workloadOf(node);
	if (!workload) return null;

	const current = (): VaultConnection[] => {
		switch (workload.kind) {
			case "backend":
				return config.workload?.vaults || [];
			case "service":
				return (
					config.services?.find((s) => s.name === workload.name)?.vaults || []
				);
			case "scheduled-task":
				return (
					config.scheduled_tasks?.find((t) => t.name === workload.name)
						?.vaults || []
				);
			case "event-task":
				return (
					config.event_processor_tasks?.find((t) => t.name === workload.name)
						?.vaults || []
				);
		}
	};

	const save = (next: VaultConnection[]) => {
		const vaults = next.length > 0 ? next : undefined;
		const withVaults = <T extends { name: string }>(item: T): T =>
			item.name === workload.name ? { ...item, vaults } : item;
		switch (workload.kind) {
			case "backend":
				onConfigChange({ workload: { vaults } });
				return;
			case "service":
				if (!config.services) return;
				onConfigChange({ services: config.services.map(withVaults) });
				return;
			case "scheduled-task":
				if (!config.scheduled_tasks) return;
				onConfigChange({
					scheduled_tasks: config.scheduled_tasks.map(withVaults),
				});
				return;
			case "event-task":
				if (!config.event_processor_tasks) return;
				onConfigChange({
					event_processor_tasks: config.event_processor_tasks.map(withVaults),
				});
				return;
		}
	};

	const connections = current();

	return (
		<Card>
			<CardHeader>
				<CardTitle className="flex items-center gap-2">
					<Vault className="w-5 h-5" />
					Vaults
				</CardTitle>
				<CardDescription>
					Every parameter of a connected vault reaches this workload as a
					secret. Read lets the container receive them; read and write also lets
					the application create and update them.
				</CardDescription>
			</CardHeader>
			<CardContent>
				<VaultConnectionsEditor
					config={config}
					connections={connections}
					onChange={(index, patch) =>
						save(
							connections.map((c, i) => (i === index ? { ...c, ...patch } : c)),
						)
					}
					onAdd={(connection) => save([...connections, connection])}
					onRemove={(index) => save(connections.filter((_, i) => i !== index))}
				/>
			</CardContent>
		</Card>
	);
}
