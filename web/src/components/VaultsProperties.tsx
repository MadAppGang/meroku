import { format } from "date-fns";
import { Plus, Trash2, Vault } from "lucide-react";
import { Fragment, useCallback, useEffect, useId, useState } from "react";
import {
	infrastructureApi,
	type VaultSecretStatus,
	VaultSecretsError,
} from "../api/infrastructure";
import type { YamlInfrastructureConfig } from "../types/yamlConfig";
import {
	vaultConnectionsOf,
	vaultNameError,
	vaultNames,
	vaultPath,
	vaultSecretName,
	vaultSecretNameError,
	vaultSecretValueError,
} from "../utils/vaults";
import { removeVaultReferences } from "../utils/workloadReferences";
import { Alert, AlertDescription, AlertTitle } from "./ui/alert";
import {
	AlertDialog,
	AlertDialogAction,
	AlertDialogCancel,
	AlertDialogContent,
	AlertDialogDescription,
	AlertDialogFooter,
	AlertDialogHeader,
	AlertDialogTitle,
} from "./ui/alert-dialog";
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
import { Separator } from "./ui/separator";
import { NoteCode, NoteDetail, StatusNote } from "./ui/status-note";

interface VaultsPropertiesProps {
	config: YamlInfrastructureConfig;
	onConfigChange: (config: Partial<YamlInfrastructureConfig>) => void;
}

/**
 * The Vaults node: the environment's shared SSM vaults. Each vault is a path in
 * Parameter Store with a list of secret names; workloads connect to it from
 * their own Vaults tab.
 *
 * This component is the single owner of the top-level `vaults` list. Removing a
 * vault also removes every workload's connection to it, in the same update; a
 * vault's secret list is written as that vault's whole list, computed from the
 * current config.
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
		onConfigChange({
			vaults: [...(config.vaults || []), { name: newName, secrets: [] }],
		});
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

	const handleSecrets = (name: string, secrets: string[]) => {
		onConfigChange({
			vaults: (config.vaults || []).map((v) =>
				v.name === name ? { ...v, secrets } : v,
			),
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
						A vault holds secrets several workloads share. List each secret's
						name here and set its value: the value goes straight to AWS and is
						never shown again. Every workload connected to the vault gets each
						listed secret, named in upper case. Connect a workload from its own
						Vaults tab.
					</CardDescription>
				</CardHeader>
				<CardContent className="space-y-4">
					{names.length === 0 && <NoteDetail>No vaults yet.</NoteDetail>}

					{(config.vaults || []).map((vault) => {
						const connections = vaultConnectionsOf(config, vault.name);
						return (
							<Card key={vault.name} data-testid={`vault-${vault.name}`}>
								<CardHeader>
									<div className="flex items-center justify-between gap-2">
										<div>
											<CardTitle>{vault.name}</CardTitle>
											<CardDescription>
												<code>{vaultPath(config, vault.name)}</code>
											</CardDescription>
										</div>
										<Button
											variant="ghost"
											size="sm"
											aria-label={`Remove vault ${vault.name}`}
											onClick={() => handleRemove(vault.name)}
										>
											<Trash2 className="w-4 h-4" />
										</Button>
									</div>
								</CardHeader>
								<CardContent className="space-y-4">
									{connections.length === 0 ? (
										<NoteDetail>No workload is connected yet.</NoteDetail>
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
									<Separator />
									<VaultSecrets
										env={config.env}
										vault={vault.name}
										declared={vault.secrets}
										onChange={(secrets) => handleSecrets(vault.name, secrets)}
									/>
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
							<StatusNote tone="error">{newNameError}</StatusNote>
						)}
					</div>
				</CardContent>
			</Card>
		</div>
	);
}

/** What the vault's listing from AWS is, as far as this panel knows. */
type Listing =
	| { state: "loading" }
	| { state: "ready"; secrets: VaultSecretStatus[] }
	| { state: "not-saved" }
	| { state: "error"; message: string };

/** The note shown after the last change, until the next one. */
type Hint = "apply" | "removed" | "redeploy" | null;

const RETRY_DELAY_MS = 250;
// The YAML is saved in the background after every change; these bound how
// long a request waits for the saved file to catch up (about five seconds).
const LIST_ATTEMPTS = 8;
const PUT_ATTEMPTS = 20;

/**
 * Runs `request` again while the server answers `status`, up to `attempts`
 * times, so a request that depends on the saved YAML waits for the save.
 */
async function retryWhileStatus<T>(
	request: () => Promise<T>,
	status: number,
	attempts: number,
): Promise<T> {
	for (let attempt = 1; ; attempt++) {
		try {
			return await request();
		} catch (error) {
			if (
				!(error instanceof VaultSecretsError) ||
				error.status !== status ||
				attempt >= attempts
			) {
				throw error;
			}
			await new Promise((resolve) => setTimeout(resolve, RETRY_DELAY_MS));
		}
	}
}

function messageOf(error: unknown): string {
	if (error instanceof VaultSecretsError && error.status === 403) {
		return `AWS refused the request: ${error.message}. Check that your AWS profile may manage this vault's parameters.`;
	}
	return error instanceof Error ? error.message : String(error);
}

function changedAt(iso?: string): string | null {
	if (!iso) return null;
	const date = new Date(iso);
	return Number.isNaN(date.getTime())
		? null
		: format(date, "d MMM yyyy, HH:mm");
}

interface VaultSecretsProps {
	env: string;
	vault: string;
	/** The vault's `secrets` list; `undefined` when the YAML has no such key. */
	declared: string[] | undefined;
	onChange: (secrets: string[]) => void;
}

/**
 * One vault's secrets: the names its YAML lists, merged with the parameters
 * AWS has under its path. Names are edited through `onChange`; values are only
 * ever sent, never read, and every value field starts empty.
 */
function VaultSecrets({ env, vault, declared, onChange }: VaultSecretsProps) {
	const uid = useId();
	const [listing, setListing] = useState<Listing>({ state: "loading" });
	const [reloadKey, setReloadKey] = useState(0);
	const [hint, setHint] = useState<Hint>(null);
	const [actionError, setActionError] = useState<string | null>(null);
	const [busy, setBusy] = useState(false);
	const [editing, setEditing] = useState<string | null>(null);
	const [confirmDelete, setConfirmDelete] = useState<string | null>(null);
	const [newName, setNewName] = useState("");
	const [newValue, setNewValue] = useState("");

	const reload = useCallback(() => setReloadKey((k) => k + 1), []);

	// biome-ignore lint/correctness/useExhaustiveDependencies: reloadKey re-reads AWS after a change or on "Try again".
	useEffect(() => {
		let cancelled = false;
		retryWhileStatus(
			() => infrastructureApi.listVaultSecrets(env, vault),
			404,
			LIST_ATTEMPTS,
		)
			.then((result) => {
				if (!cancelled) setListing({ state: "ready", secrets: result.secrets });
			})
			.catch((error: unknown) => {
				if (cancelled) return;
				if (error instanceof VaultSecretsError && error.status === 404) {
					setListing({ state: "not-saved" });
				} else {
					setListing({ state: "error", message: messageOf(error) });
				}
			});
		return () => {
			cancelled = true;
		};
	}, [env, vault, reloadKey]);

	const inAws = new Map<string, VaultSecretStatus>();
	if (listing.state === "ready") {
		for (const s of listing.secrets) if (s.set) inAws.set(s.name, s);
	}
	const ready = listing.state === "ready";

	const run = async (action: () => Promise<void>) => {
		setBusy(true);
		setActionError(null);
		try {
			await action();
		} catch (error) {
			setActionError(messageOf(error));
		} finally {
			setBusy(false);
			reload();
		}
	};

	if (declared === undefined) {
		const found = ready ? listing.secrets.map((s) => s.name) : [];
		return (
			<div className="space-y-3">
				<Alert>
					<AlertTitle>meroku needs this vault's secret names</AlertTitle>
					<AlertDescription>
						<p>
							Every connected workload gets the secrets this vault lists. Import
							the names already in AWS, or start with an empty list and add them
							here.
						</p>
						{ready &&
							(found.length > 0 ? (
								<p>Found in AWS: {found.join(", ")}.</p>
							) : (
								<p>AWS has no secrets in this vault yet.</p>
							))}
					</AlertDescription>
				</Alert>
				<ListingNote listing={listing} onRetry={reload} />
				<div className="flex flex-wrap gap-2">
					<Button
						size="sm"
						disabled={!ready || found.length === 0}
						onClick={() => {
							onChange(found);
							setHint("apply");
						}}
					>
						Import names from AWS
					</Button>
					<Button size="sm" variant="outline" onClick={() => onChange([])}>
						Start empty
					</Button>
				</div>
			</div>
		);
	}

	const names = [...new Set([...declared, ...inAws.keys()])].sort();
	const newNameError = vaultSecretNameError(newName, declared);
	const newValueError = vaultSecretValueError(newValue);
	const canAdd =
		ready &&
		!busy &&
		newName !== "" &&
		newNameError === null &&
		newValue !== "" &&
		newValueError === null;

	const setValue = (name: string, value: string) =>
		run(async () => {
			// A name just added is saved to the YAML in the background; the
			// server refuses the value until the saved file lists the name.
			await retryWhileStatus(
				() => infrastructureApi.setVaultSecret(env, vault, name, value),
				409,
				PUT_ATTEMPTS,
			);
		});

	const handleAdd = async () => {
		if (!canAdd) return;
		const name = newName;
		const value = newValue;
		setNewName("");
		setNewValue("");
		onChange([...declared, name]);
		setHint("apply");
		await setValue(name, value);
	};

	return (
		<div className="space-y-3">
			<ListingNote listing={listing} onRetry={reload} />

			{names.length === 0 && (
				<NoteDetail>This vault has no secrets yet.</NoteDetail>
			)}

			{names.map((name, index) => {
				const isDeclared = declared.includes(name);
				const aws = inAws.get(name);
				return (
					<Fragment key={name}>
						{index > 0 && <Separator />}
						<div className="space-y-2" data-testid={`vault-secret-${name}`}>
							<div className="flex flex-wrap items-center justify-between gap-2">
								<div className="flex flex-wrap items-center gap-2">
									<Badge variant="code">{name}</Badge>
									<NoteDetail>becomes</NoteDetail>
									<Badge variant="code">
										{vaultSecretName(undefined, name)}
									</Badge>
								</div>
								<div className="flex flex-wrap gap-2">
									{isDeclared && ready && (
										<Button
											size="sm"
											variant="outline"
											disabled={busy}
											aria-label={`${aws ? "Replace value of" : "Set value of"} ${name}`}
											onClick={() => setEditing(name)}
										>
											{aws ? "Replace value" : "Set value"}
										</Button>
									)}
									{isDeclared && (
										<Button
											size="sm"
											variant="ghost"
											disabled={busy}
											aria-label={`Remove ${name} from the YAML`}
											onClick={() => {
												onChange(declared.filter((n) => n !== name));
												if (editing === name) setEditing(null);
												setHint("removed");
											}}
										>
											Remove
										</Button>
									)}
									{!isDeclared && (
										<>
											<Button
												size="sm"
												variant="outline"
												disabled={busy}
												aria-label={`Add ${name} to the YAML`}
												onClick={() => {
													onChange([...declared, name]);
													setHint("apply");
												}}
											>
												Add to YAML
											</Button>
											<Button
												size="sm"
												variant="ghost"
												disabled={busy}
												aria-label={`Delete ${name} from AWS`}
												onClick={() => setConfirmDelete(name)}
											>
												Delete from AWS
											</Button>
										</>
									)}
								</div>
							</div>

							{ready && isDeclared && aws && (
								<NoteDetail>
									Value set
									{aws.version ? ` · version ${aws.version}` : ""}
									{changedAt(aws.lastModified)
										? ` · changed ${changedAt(aws.lastModified)}`
										: ""}
								</NoteDetail>
							)}
							{ready && isDeclared && !aws && (
								<StatusNote tone="warning">
									No value yet. Connected workloads cannot start until it is
									set.
								</StatusNote>
							)}
							{!isDeclared && (
								<StatusNote tone="neutral">
									Not in the YAML. The value is in AWS, but no workload gets it.
								</StatusNote>
							)}

							{editing === name && (
								<SecretValueForm
									id={`${uid}-${name}`}
									name={name}
									replacing={aws !== undefined}
									busy={busy}
									onCancel={() => setEditing(null)}
									onSave={async (value) => {
										setEditing(null);
										setHint("redeploy");
										await setValue(name, value);
									}}
								/>
							)}
						</div>
					</Fragment>
				);
			})}

			{actionError && <StatusNote tone="error">{actionError}</StatusNote>}
			{hint === "apply" && (
				<StatusNote tone="neutral">
					Apply to give the connected workloads this change.
				</StatusNote>
			)}
			{hint === "removed" && (
				<StatusNote tone="neutral">
					<p>Apply to give the connected workloads this change.</p>
					<NoteDetail>
						Apply, then delete the value, so running tasks never point at a
						missing secret.
					</NoteDetail>
				</StatusNote>
			)}
			{hint === "redeploy" && (
				<StatusNote tone="neutral">
					Redeploy the connected workloads to pick up the new value.
				</StatusNote>
			)}

			<div className="space-y-2">
				<div className="flex flex-wrap items-end gap-2">
					<div className="flex-1 space-y-1">
						<Label htmlFor={`${uid}-new-name`}>New secret name</Label>
						<Input
							id={`${uid}-new-name`}
							placeholder="database_url"
							autoComplete="off"
							value={newName}
							aria-invalid={newNameError !== null}
							onChange={(e) => setNewName(e.target.value.trim())}
						/>
					</div>
					<div className="flex-1 space-y-1">
						<Label htmlFor={`${uid}-new-value`}>Value</Label>
						<Input
							id={`${uid}-new-value`}
							type="password"
							autoComplete="new-password"
							value={newValue}
							aria-invalid={newValueError !== null}
							onChange={(e) => setNewValue(e.target.value)}
							onKeyDown={(e) => {
								if (e.key === "Enter") handleAdd();
							}}
						/>
					</div>
					<Button
						disabled={!canAdd}
						aria-label={`Add secret to ${vault}`}
						onClick={handleAdd}
					>
						<Plus className="w-4 h-4" />
						Add secret
					</Button>
				</div>
				{newNameError && <StatusNote tone="error">{newNameError}</StatusNote>}
				{newValueError && <StatusNote tone="error">{newValueError}</StatusNote>}
			</div>

			<AlertDialog
				open={confirmDelete !== null}
				onOpenChange={(open) => {
					if (!open) setConfirmDelete(null);
				}}
			>
				<AlertDialogContent>
					<AlertDialogHeader>
						<AlertDialogTitle>
							Delete {confirmDelete} from AWS?
						</AlertDialogTitle>
						<AlertDialogDescription>
							The value is deleted from AWS for good and cannot be brought back.
							The YAML does not list it, so no workload gets it now.
						</AlertDialogDescription>
					</AlertDialogHeader>
					<AlertDialogFooter>
						<AlertDialogCancel>Cancel</AlertDialogCancel>
						<AlertDialogAction
							variant="destructive"
							onClick={() => {
								const name = confirmDelete;
								setConfirmDelete(null);
								if (name === null) return;
								run(async () => {
									await infrastructureApi.deleteVaultSecret(env, vault, name);
								});
							}}
						>
							Delete from AWS
						</AlertDialogAction>
					</AlertDialogFooter>
				</AlertDialogContent>
			</AlertDialog>
		</div>
	);
}

/** Where the AWS listing stands, when it is not simply ready. */
function ListingNote({
	listing,
	onRetry,
}: {
	listing: Listing;
	onRetry: () => void;
}) {
	switch (listing.state) {
		case "ready":
			return null;
		case "loading":
			return <StatusNote tone="pending">Checking AWS…</StatusNote>;
		case "not-saved":
			return (
				<div className="flex flex-wrap items-center gap-2">
					<StatusNote tone="neutral">
						Save first: this vault is not in the saved file yet.
					</StatusNote>
					<Button size="sm" variant="outline" onClick={onRetry}>
						Check again
					</Button>
				</div>
			);
		case "error":
			return (
				<div className="flex flex-wrap items-center gap-2">
					<StatusNote tone="error">
						Couldn't read this vault from AWS: {listing.message}
					</StatusNote>
					<Button size="sm" variant="outline" onClick={onRetry}>
						Try again
					</Button>
				</div>
			);
	}
}

/** A write-only value field. It always starts empty. */
function SecretValueForm({
	id,
	name,
	replacing,
	busy,
	onSave,
	onCancel,
}: {
	id: string;
	name: string;
	replacing: boolean;
	busy: boolean;
	onSave: (value: string) => void;
	onCancel: () => void;
}) {
	const [value, setValue] = useState("");
	const error = vaultSecretValueError(value);
	const canSave = !busy && value !== "" && error === null;
	const save = () => {
		if (canSave) onSave(value);
	};
	return (
		<div className="space-y-2">
			<div className="flex flex-wrap items-end gap-2">
				<div className="flex-1 space-y-1">
					<Label htmlFor={id}>
						{replacing ? "New value for" : "Value for"} {name}
					</Label>
					<Input
						id={id}
						type="password"
						autoComplete="new-password"
						value={value}
						aria-invalid={error !== null}
						onChange={(e) => setValue(e.target.value)}
						onKeyDown={(e) => {
							if (e.key === "Enter") save();
						}}
					/>
				</div>
				<Button size="sm" disabled={!canSave} onClick={save}>
					Save value
				</Button>
				<Button size="sm" variant="ghost" onClick={onCancel}>
					Cancel
				</Button>
			</div>
			{error && <StatusNote tone="error">{error}</StatusNote>}
			<NoteDetail>
				The value goes straight to AWS as <NoteCode>SecureString</NoteCode>.
				meroku never shows it again.
			</NoteDetail>
		</div>
	);
}
