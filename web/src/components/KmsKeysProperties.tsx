import { KeyRound, Plus } from "lucide-react";
import { useId, useState } from "react";
import type {
	KmsKeyDefinition,
	KmsKeySpec,
	YamlInfrastructureConfig,
} from "../types/yamlConfig";
import {
	DEFAULT_KMS_KEY_SPEC,
	KMS_KEY_ACCESS_LABELS,
	KMS_KEY_SPEC_CHOICES,
	kmsKeyConnectionsOf,
	kmsKeyNameError,
	kmsKeyNames,
	kmsKeySpecChoice,
	nextKmsKeyName,
} from "../utils/kmsKeys";
import { removeKmsKeyReferences } from "../utils/workloadReferences";
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
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "./ui/select";
import { NoteCode, NoteDetail, StatusNote } from "./ui/status-note";

interface KmsKeysPropertiesProps {
	config: YamlInfrastructureConfig;
	onConfigChange: (config: Partial<YamlInfrastructureConfig>) => void;
}

/** The text of a key type option: its name and a one-line hint. */
function specOptionText(spec: KmsKeySpec): string {
	const choice = kmsKeySpecChoice(spec);
	return `${choice.label} — ${choice.hint}`;
}

function KeySpecSelect({
	id,
	label,
	value,
	onChange,
}: {
	id: string;
	label: string;
	value: KmsKeySpec;
	onChange: (spec: KmsKeySpec) => void;
}) {
	return (
		<Select
			value={value}
			onValueChange={(spec) => onChange(spec as KmsKeySpec)}
		>
			<SelectTrigger id={id} aria-label={label}>
				<SelectValue />
			</SelectTrigger>
			<SelectContent>
				{KMS_KEY_SPEC_CHOICES.map((choice) => (
					<SelectItem key={choice.spec} value={choice.spec}>
						{specOptionText(choice.spec)}
					</SelectItem>
				))}
			</SelectContent>
		</Select>
	);
}

/**
 * The KMS keys node: the environment's signing keys. Each key is an asymmetric
 * KMS key; workloads connect to it from their own KMS keys tab to sign, verify
 * or both.
 *
 * This component is the single owner of the top-level `kms_keys` list.
 * Retiring a key removes every workload's connection to it in the same update.
 * A key's type can change only while the key is new to this panel: a key that
 * was in the YAML when the panel opened may exist in AWS, which cannot change
 * the type of a key.
 */
export function KmsKeysProperties({
	config,
	onConfigChange,
}: KmsKeysPropertiesProps) {
	const uid = useId();
	// Keys the YAML held when the panel opened. Their type is fixed.
	const [savedKeys] = useState<ReadonlySet<string>>(
		() => new Set(kmsKeyNames(config)),
	);
	// `null` until the name field is edited: the field shows the next key<n>.
	const [nameDraft, setNameDraft] = useState<string | null>(null);
	const [newSpec, setNewSpec] = useState<KmsKeySpec>(DEFAULT_KMS_KEY_SPEC);
	const [confirmDelete, setConfirmDelete] = useState<string | null>(null);

	const keys = config.kms_keys || [];
	const names = kmsKeyNames(config);
	const newName = nameDraft ?? nextKmsKeyName(names);
	const newNameError = kmsKeyNameError(newName, names);

	const writeKeys = (next: KmsKeyDefinition[]) =>
		next.length > 0 ? next : undefined;

	const handleAdd = () => {
		if (newNameError) return;
		onConfigChange({
			kms_keys: [...keys, { name: newName, key_spec: newSpec }],
		});
		setNameDraft(null);
		setNewSpec(DEFAULT_KMS_KEY_SPEC);
	};

	const handleSpec = (name: string, key_spec: KmsKeySpec) => {
		onConfigChange({
			kms_keys: keys.map((k) => (k.name === name ? { ...k, key_spec } : k)),
		});
	};

	const handleRetire = (name: string) => {
		const kms_keys = keys.map((k) =>
			k.name === name ? { ...k, retired: true } : k,
		);
		onConfigChange({
			kms_keys,
			...removeKmsKeyReferences({ ...config, kms_keys }, name),
		});
	};

	const handleRestore = (name: string) => {
		onConfigChange({
			kms_keys: keys.map((k) => {
				if (k.name !== name) return k;
				const { retired: _retired, ...rest } = k;
				return rest;
			}),
		});
	};

	const handleDelete = (name: string) => {
		const kms_keys = writeKeys(keys.filter((k) => k.name !== name));
		onConfigChange({
			kms_keys,
			...removeKmsKeyReferences({ ...config, kms_keys }, name),
		});
	};

	const aliasOf = (name: string) =>
		`alias/${config.project}-${config.env}-${name}`;

	return (
		<div className="space-y-4">
			<Card>
				<CardHeader>
					<CardTitle className="flex items-center gap-2">
						<KeyRound className="w-5 h-5" />
						KMS keys
					</CardTitle>
					<CardDescription>
						A signing key lives in AWS and never leaves it: workloads ask AWS to
						sign or check a signature, for example on a JWT. Add a key here,
						then connect a workload from its own KMS keys tab.
					</CardDescription>
				</CardHeader>
				<CardContent className="space-y-4">
					{keys.length === 0 && <NoteDetail>No keys yet.</NoteDetail>}

					{keys.map((key) => {
						const connections = kmsKeyConnectionsOf(config, key.name);
						const spec = key.key_spec ?? DEFAULT_KMS_KEY_SPEC;
						const editable = !savedKeys.has(key.name);
						return (
							<Card key={key.name} data-testid={`kms-key-${key.name}`}>
								<CardHeader>
									<div className="flex flex-wrap items-center justify-between gap-2">
										<div className="space-y-1">
											<div className="flex flex-wrap items-center gap-2">
												<CardTitle>{key.name}</CardTitle>
												<Badge variant="code">
													{kmsKeySpecChoice(spec).label}
												</Badge>
												{key.retired && (
													<Badge variant="outline">Retired</Badge>
												)}
											</div>
											<CardDescription>
												<code>{aliasOf(key.name)}</code>
											</CardDescription>
										</div>
										<div className="flex flex-wrap gap-2">
											{key.retired ? (
												<>
													<Button
														size="sm"
														variant="outline"
														aria-label={`Restore key ${key.name}`}
														onClick={() => handleRestore(key.name)}
													>
														Restore
													</Button>
													<Button
														size="sm"
														variant="destructive"
														aria-label={`Delete key ${key.name}`}
														onClick={() => setConfirmDelete(key.name)}
													>
														Delete
													</Button>
												</>
											) : (
												<Button
													size="sm"
													variant="outline"
													aria-label={`Retire key ${key.name}`}
													onClick={() => handleRetire(key.name)}
												>
													Retire
												</Button>
											)}
										</div>
									</div>
								</CardHeader>
								<CardContent className="space-y-3">
									{key.retired ? (
										<StatusNote tone="neutral">
											<p>
												Retired: no workload uses this key any more, but it
												still works in AWS.
											</p>
											<NoteDetail>Apply, then delete the key.</NoteDetail>
										</StatusNote>
									) : connections.length === 0 ? (
										<NoteDetail>No workload uses this key yet.</NoteDetail>
									) : (
										<div className="flex flex-wrap gap-2">
											{connections.map((c) => (
												<Badge key={c.workload} variant="secondary">
													{c.workload} · {KMS_KEY_ACCESS_LABELS[c.access]}
												</Badge>
											))}
										</div>
									)}

									{editable ? (
										<div className="space-y-1">
											<Label htmlFor={`${uid}-${key.name}-spec`}>
												Key type
											</Label>
											<KeySpecSelect
												id={`${uid}-${key.name}-spec`}
												label={`Key type of ${key.name}`}
												value={spec}
												onChange={(next) => handleSpec(key.name, next)}
											/>
											<NoteDetail>
												You can change the type until the key is saved. After
												that it is fixed.
											</NoteDetail>
										</div>
									) : (
										<NoteDetail>
											Key type: {specOptionText(spec)}. AWS cannot change the
											type of an existing key: to use another type, add a new
											key, move the workloads to it, then retire this one.
										</NoteDetail>
									)}
								</CardContent>
							</Card>
						);
					})}

					<div className="space-y-2">
						<div className="flex flex-wrap items-end gap-2">
							<div className="flex-1 space-y-1">
								<Label htmlFor={`${uid}-new-name`}>New key name</Label>
								<Input
									id={`${uid}-new-name`}
									autoComplete="off"
									value={newName}
									aria-invalid={newNameError !== null}
									onChange={(e) => setNameDraft(e.target.value.trim())}
									onKeyDown={(e) => {
										if (e.key === "Enter") handleAdd();
									}}
								/>
							</div>
							<div className="flex-1 space-y-1">
								<Label htmlFor={`${uid}-new-spec`}>Key type</Label>
								<KeySpecSelect
									id={`${uid}-new-spec`}
									label="Type of the new key"
									value={newSpec}
									onChange={setNewSpec}
								/>
							</div>
							<Button onClick={handleAdd} disabled={newNameError !== null}>
								<Plus className="w-4 h-4" />
								Add key
							</Button>
						</div>
						{newNameError && (
							<StatusNote tone="error">{newNameError}</StatusNote>
						)}
						<NoteDetail>
							Each connected workload gets <NoteCode>KMS_NAME_KEY_ARN</NoteCode>{" "}
							and <NoteCode>KMS_NAME_PUBLIC_KEY</NoteCode>, with the key's name
							in upper case.
						</NoteDetail>
					</div>
				</CardContent>
			</Card>

			<AlertDialog
				open={confirmDelete !== null}
				onOpenChange={(open) => {
					if (!open) setConfirmDelete(null);
				}}
			>
				<AlertDialogContent>
					<AlertDialogHeader>
						<AlertDialogTitle>Delete key {confirmDelete}?</AlertDialogTitle>
						<AlertDialogDescription>
							The next apply asks AWS to delete the key. It stops working at
							once, and AWS deletes it for good after 7 days. Until then the
							deletion can be cancelled in the AWS console.
						</AlertDialogDescription>
					</AlertDialogHeader>
					<StatusNote tone="warning">
						<p>
							Anything signed with this key can no longer be checked with it.
						</p>
						<NoteDetail>
							Apply the retirement first, so no running workload still uses it.
						</NoteDetail>
					</StatusNote>
					<AlertDialogFooter>
						<AlertDialogCancel>Cancel</AlertDialogCancel>
						<AlertDialogAction
							variant="destructive"
							onClick={() => {
								const name = confirmDelete;
								setConfirmDelete(null);
								if (name !== null) handleDelete(name);
							}}
						>
							Delete key
						</AlertDialogAction>
					</AlertDialogFooter>
				</AlertDialogContent>
			</AlertDialog>
		</div>
	);
}
