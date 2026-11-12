import { Plus, Trash2, Vault } from "lucide-react";
import { useId, useState } from "react";
import type { YamlInfrastructureConfig } from "../types/yamlConfig";
import {
	vaultConnectionsOf,
	vaultNameError,
	vaultNames,
	vaultPath,
} from "../utils/vaults";
import { removeVaultReferences } from "../utils/workloadReferences";
import { Badge } from "./ui/badge";
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

interface VaultsPropertiesProps {
	config: YamlInfrastructureConfig;
	onConfigChange: (config: Partial<YamlInfrastructureConfig>) => void;
}

/**
 * The Vaults node: the environment's shared SSM vaults. Each vault is a path in
 * Parameter Store; workloads connect to it from their own Vaults tab.
 *
 * This component is the single owner of the top-level `vaults` list. Removing a
 * vault also removes every workload's connection to it, in the same update.
 */
export function VaultsProperties({
	config,
	onConfigChange,
}: VaultsPropertiesProps) {
	const uid = useId();
	const [newName, setNewName] = useState("");

	const names = vaultNames(config);
	const newNameError = vaultNameError(newName, names);

	const handleAdd = () => {
		if (newName === "" || newNameError) return;
		onConfigChange({ vaults: [...(config.vaults || []), { name: newName }] });
		setNewName("");
	};

	const handleRemove = (name: string) => {
		const remaining = (config.vaults || []).filter((v) => v.name !== name);
		const vaults = remaining.length > 0 ? remaining : undefined;
		onConfigChange({
			vaults,
			...removeVaultReferences({ ...config, vaults }, name),
		});
	};

	return (
		<div className="space-y-4">
			<Card>
				<CardHeader>
					<CardTitle className="flex items-center gap-2">
						<Vault className="w-5 h-5" />
						Vaults
					</CardTitle>
					<CardDescription>
						A vault holds secrets several workloads share. Every parameter
						directly under its path reaches each connected workload as a secret,
						named after the parameter in upper case. Connect a workload from its
						own Vaults tab.
					</CardDescription>
				</CardHeader>
				<CardContent className="space-y-4">
					{names.length === 0 && (
						<p className="text-sm text-muted-foreground">No vaults yet.</p>
					)}

					{names.map((name) => {
						const connections = vaultConnectionsOf(config, name);
						return (
							<Card key={name} data-testid={`vault-${name}`}>
								<CardHeader>
									<div className="flex items-center justify-between gap-2">
										<div>
											<CardTitle>{name}</CardTitle>
											<CardDescription>
												<code>{vaultPath(config, name)}</code>
											</CardDescription>
										</div>
										<Button
											variant="ghost"
											size="sm"
											aria-label={`Remove vault ${name}`}
											onClick={() => handleRemove(name)}
										>
											<Trash2 className="w-4 h-4" />
										</Button>
									</div>
								</CardHeader>
								<CardContent className="space-y-2">
									{connections.length === 0 ? (
										<p className="text-sm text-muted-foreground">
											No workload is connected yet.
										</p>
									) : (
										<div className="flex flex-wrap gap-2">
											{connections.map((c) => (
												<Badge key={c.workload} variant="secondary">
													{c.workload}
													{c.access === "read_write" ? " · read and write" : ""}
													{c.prefix ? ` · ${c.prefix}_` : ""}
												</Badge>
											))}
										</div>
									)}
								</CardContent>
							</Card>
						);
					})}

					<div className="space-y-2">
						<Label htmlFor={`${uid}-new-vault`}>New vault</Label>
						<div className="flex gap-2">
							<Input
								id={`${uid}-new-vault`}
								placeholder="shared"
								value={newName}
								aria-invalid={newNameError !== null}
								onChange={(e) => setNewName(e.target.value.trim())}
								onKeyDown={(e) => {
									if (e.key === "Enter") handleAdd();
								}}
							/>
							<Button
								onClick={handleAdd}
								disabled={newName === "" || newNameError !== null}
							>
								<Plus className="w-4 h-4" />
								Add
							</Button>
						</div>
						{newNameError && (
							<p className="text-sm text-destructive">{newNameError}</p>
						)}
					</div>
				</CardContent>
			</Card>

			<Card>
				<CardHeader>
					<CardTitle>Setting a secret</CardTitle>
					<CardDescription>
						meroku never stores the values. Put a parameter under the vault's
						path, then apply, so the connected workloads list it.
					</CardDescription>
				</CardHeader>
				<CardContent className="space-y-2">
					<pre className="text-xs font-mono whitespace-pre-wrap break-all">
						{`aws ssm put-parameter --type SecureString \\\n  --name ${vaultPath(config, names[0] || "<vault>")}database_url \\\n  --value <value>`}
					</pre>
					<p className="text-xs text-muted-foreground">
						Adding or removing a parameter changes the connected task
						definitions on the next apply. Changing a value with --overwrite
						does not: redeploy the connected workloads to pick it up.
					</p>
				</CardContent>
			</Card>
		</div>
	);
}
