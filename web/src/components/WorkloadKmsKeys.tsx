import { KeyRound, Plus, Trash2 } from "lucide-react";
import { useId } from "react";
import type { ComponentNode } from "../types";
import type {
	KmsKeyAccess,
	KmsKeyConnection,
	YamlInfrastructureConfig,
} from "../types/yamlConfig";
import {
	activeKmsKeyNames,
	KMS_KEY_ACCESS_LABELS,
	kmsKeyEnvNames,
	kmsKeyOf,
} from "../utils/kmsKeys";
import {
	workloadConnections,
	workloadConnectionsUpdate,
	workloadOf,
} from "../utils/workloadConnections";
import { Alert, AlertDescription } from "./ui/alert";
import { Badge } from "./ui/badge";
import { Button } from "./ui/button";
import {
	Card,
	CardContent,
	CardDescription,
	CardHeader,
	CardTitle,
} from "./ui/card";
import { Label } from "./ui/label";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "./ui/select";
import { NoteDetail } from "./ui/status-note";

const ACCESS_ORDER: KmsKeyAccess[] = ["sign", "verify", "both"];

/** What each access lets the application do with the key. */
const ACCESS_DESCRIPTIONS: Record<KmsKeyAccess, string> = {
	sign: "The application can sign with this key and read its public key.",
	verify:
		"The application can check signatures made with this key and read its public key. It cannot sign.",
	both: "The application can sign, check signatures and read the public key.",
};

/** The access a new connection starts with: the least it can do. */
const DEFAULT_ACCESS: KmsKeyAccess = "verify";

interface KmsKeyConnectionsEditorProps {
	config: YamlInfrastructureConfig;
	connections: KmsKeyConnection[];
	onChange: (index: number, patch: Partial<KmsKeyConnection>) => void;
	onAdd: (connection: KmsKeyConnection) => void;
	onRemove: (index: number) => void;
}

/**
 * A workload's KMS key connections. It never sees the workload or the list the
 * workload lives in: every edit goes to the owner as (index, patch), and the
 * owner applies it to the current config.
 */
export function KmsKeyConnectionsEditor({
	config,
	connections,
	onChange,
	onAdd,
	onRemove,
}: KmsKeyConnectionsEditorProps) {
	const active = activeKmsKeyNames(config);
	const connected = new Set(connections.map((c) => c.name));
	const available = active.filter((n) => !connected.has(n));

	return (
		<div className="space-y-4">
			{active.length === 0 && connections.length === 0 && (
				<NoteDetail>
					This environment has no KMS keys to use. Add one on the KMS keys node.
				</NoteDetail>
			)}

			{connections.map((connection, index) => (
				<KmsKeyConnectionRow
					key={connection.name}
					config={config}
					connection={connection}
					choices={[connection.name, ...available]}
					onChange={(patch) => onChange(index, patch)}
					onRemove={() => onRemove(index)}
				/>
			))}

			{available.length > 0 && (
				<Button
					variant="outline"
					size="sm"
					onClick={() =>
						onAdd({ name: available[0] as string, access: DEFAULT_ACCESS })
					}
				>
					<Plus className="w-4 h-4" />
					Connect a key
				</Button>
			)}
		</div>
	);
}

interface KmsKeyConnectionRowProps {
	config: YamlInfrastructureConfig;
	connection: KmsKeyConnection;
	choices: string[];
	onChange: (patch: Partial<KmsKeyConnection>) => void;
	onRemove: () => void;
}

function KmsKeyConnectionRow({
	config,
	connection,
	choices,
	onChange,
	onRemove,
}: KmsKeyConnectionRowProps) {
	const uid = useId();
	const key = kmsKeyOf(config, connection.name);
	const env = kmsKeyEnvNames(connection.name);
	const access: KmsKeyAccess | undefined = ACCESS_ORDER.includes(
		connection.access,
	)
		? connection.access
		: undefined;

	return (
		<Card data-testid={`kms-key-connection-${connection.name}`}>
			<CardHeader>
				<CardTitle>{connection.name}</CardTitle>
				<CardDescription>
					<code>
						alias/{config.project}-{config.env}-{connection.name}
					</code>
				</CardDescription>
			</CardHeader>
			<CardContent className="space-y-3">
				{!key && (
					<Alert variant="destructive">
						<AlertDescription>
							<span>
								The key "{connection.name}" is not in this environment's KMS
								keys; generate refuses the connection until it is removed.
							</span>
						</AlertDescription>
					</Alert>
				)}
				{key?.retired && (
					<Alert variant="destructive">
						<AlertDescription>
							<span>
								The key "{connection.name}" is retired; generate refuses the
								connection. Remove it, or restore the key on the KMS keys node.
							</span>
						</AlertDescription>
					</Alert>
				)}
				{access === undefined && (
					<Alert variant="destructive">
						<AlertDescription>
							<span>Choose what this workload may do with the key.</span>
						</AlertDescription>
					</Alert>
				)}
				<div className="flex items-end gap-2">
					<div className="flex-1 space-y-1">
						<Label htmlFor={`${uid}-key`}>Key</Label>
						<Select
							value={connection.name}
							onValueChange={(name) => onChange({ name })}
						>
							<SelectTrigger
								id={`${uid}-key`}
								aria-label={`Key of connection ${connection.name}`}
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
							value={access ?? ""}
							onValueChange={(next) =>
								onChange({ access: next as KmsKeyAccess })
							}
						>
							<SelectTrigger
								id={`${uid}-access`}
								aria-label={`Access to key ${connection.name}`}
							>
								<SelectValue placeholder="Choose access" />
							</SelectTrigger>
							<SelectContent>
								{ACCESS_ORDER.map((value) => (
									<SelectItem key={value} value={value}>
										{KMS_KEY_ACCESS_LABELS[value]}
									</SelectItem>
								))}
							</SelectContent>
						</Select>
					</div>
					<Button
						variant="ghost"
						size="sm"
						aria-label={`Disconnect key ${connection.name}`}
						onClick={onRemove}
					>
						<Trash2 className="w-4 h-4" />
					</Button>
				</div>
				{access && <NoteDetail>{ACCESS_DESCRIPTIONS[access]}</NoteDetail>}
				<div className="space-y-1">
					<NoteDetail>This workload gets:</NoteDetail>
					<div className="flex flex-wrap gap-2">
						<Badge variant="code">{env.arn}</Badge>
						<Badge variant="code">{env.publicKey}</Badge>
					</div>
					<NoteDetail>
						The first holds the key's ARN, for calls to AWS; the second its
						public key in PEM form, to check signatures without calling AWS.
					</NoteDetail>
				</div>
			</CardContent>
		</Card>
	);
}

interface WorkloadKmsKeysProps {
	config: YamlInfrastructureConfig;
	node: ComponentNode;
	onConfigChange: (config: Partial<YamlInfrastructureConfig>) => void;
}

/**
 * The KMS keys tab of a workload. This component is the single owner of the
 * workload's `kms_keys` list: the editor hands it (index, patch), and it writes
 * the workload's whole list, computed from the current config, into the one
 * entry of the one list the workload lives in.
 */
export function WorkloadKmsKeys({
	config,
	node,
	onConfigChange,
}: WorkloadKmsKeysProps) {
	const workload = workloadOf(node);
	if (!workload) return null;

	const save = (next: KmsKeyConnection[]) => {
		const update = workloadConnectionsUpdate(
			config,
			workload,
			"kms_keys",
			next,
		);
		if (update) onConfigChange(update);
	};

	const connections = workloadConnections(config, workload, "kms_keys");

	return (
		<Card>
			<CardHeader>
				<CardTitle className="flex items-center gap-2">
					<KeyRound className="w-5 h-5" />
					KMS keys
				</CardTitle>
				<CardDescription>
					Connect a signing key to let this workload sign with it, check
					signatures made with it, or both. The key itself never leaves AWS.
				</CardDescription>
			</CardHeader>
			<CardContent>
				<KmsKeyConnectionsEditor
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
