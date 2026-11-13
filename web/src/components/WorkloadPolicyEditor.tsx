import { Check, Edit2, Plus, Shield, Trash2, X } from "lucide-react";
import { useId, useState } from "react";
import type { IAMStatement } from "../types/yamlConfig";
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
import { Textarea } from "./ui/textarea";

type Effect = "Allow" | "Deny";

interface WorkloadPolicyEditorProps {
	/** The workload's current statements, straight from the config. */
	statements: IAMStatement[];
	/** Receives the whole new list on every add, edit or delete. */
	onChange: (statements: IAMStatement[]) => void;
	/** The task role the policy is attached to, shown in the description. */
	roleName: string;
}

interface Draft {
	effect: Effect;
	actions: string;
	resources: string;
}

const emptyDraft: Draft = { effect: "Allow", actions: "", resources: "" };

const lines = (text: string) =>
	text
		.split("\n")
		.map((line) => line.trim())
		.filter((line) => line !== "");

/**
 * Applies a draft to a statement, keeping every field the editor does not
 * show (conditions, and anything a newer schema adds). An Allow statement that
 * had no effect key keeps having none, so an untouched YAML statement saves
 * unchanged.
 */
function applyDraft(
	base: IAMStatement | undefined,
	draft: Draft,
): IAMStatement {
	const { effect: _previous, ...rest } = base ?? { actions: [], resources: [] };
	const next: IAMStatement = {
		...rest,
		actions: lines(draft.actions),
		resources: lines(draft.resources),
	};
	if (draft.effect === "Deny" || base?.effect) {
		next.effect = draft.effect;
	}
	return next;
}

/**
 * The `policy` editor shared by the backend, services, scheduled tasks and
 * event tasks: a list of IAM statements, each with an effect, actions and
 * resources. Conditions are shown but edited in YAML only.
 *
 * The list is never copied into state: it is read from `statements` on every
 * render, so switching to another workload's node shows that workload's
 * statements. Only the open form is local; mount with `key` set to the node id
 * so a half-typed draft does not follow the user to another node.
 */
export function WorkloadPolicyEditor({
	statements,
	onChange,
	roleName,
}: WorkloadPolicyEditorProps) {
	const uid = useId();
	// null: no form open; "new": adding; a number: editing that statement.
	const [editing, setEditing] = useState<number | "new" | null>(null);
	const [draft, setDraft] = useState<Draft>(emptyDraft);

	const draftActions = lines(draft.actions);
	const draftResources = lines(draft.resources);
	const draftValid = draftActions.length > 0 && draftResources.length > 0;
	const draftWildcard = draftActions.some((a) => a === "*" || a === "*:*");

	const openNew = () => {
		setDraft(emptyDraft);
		setEditing("new");
	};

	const openEdit = (index: number) => {
		const statement = statements[index];
		setDraft({
			effect: statement.effect ?? "Allow",
			actions: statement.actions.join("\n"),
			resources: statement.resources.join("\n"),
		});
		setEditing(index);
	};

	const close = () => {
		setEditing(null);
		setDraft(emptyDraft);
	};

	const save = () => {
		if (!draftValid || editing === null) return;
		if (editing === "new") {
			onChange([...statements, applyDraft(undefined, draft)]);
		} else {
			onChange(
				statements.map((statement, i) =>
					i === editing ? applyDraft(statement, draft) : statement,
				),
			);
		}
		close();
	};

	const remove = (index: number) => {
		onChange(statements.filter((_, i) => i !== index));
		if (editing === index) close();
	};

	const form = (
		<Card data-testid="policy-statement-form">
			<CardContent inset="standalone" className="space-y-3">
				<div className="space-y-1">
					<Label htmlFor={`${uid}-effect`}>Effect</Label>
					<Select
						value={draft.effect}
						onValueChange={(value: string) =>
							setDraft({ ...draft, effect: value as Effect })
						}
					>
						<SelectTrigger id={`${uid}-effect`}>
							<SelectValue />
						</SelectTrigger>
						<SelectContent>
							<SelectItem value="Allow">Allow</SelectItem>
							<SelectItem value="Deny">Deny</SelectItem>
						</SelectContent>
					</Select>
				</div>
				<div className="space-y-1">
					<Label htmlFor={`${uid}-actions`}>Actions (one per line)</Label>
					<Textarea
						id={`${uid}-actions`}
						placeholder={"dynamodb:GetItem\ndynamodb:PutItem"}
						value={draft.actions}
						onChange={(e) => setDraft({ ...draft, actions: e.target.value })}
						aria-invalid={draft.actions !== "" && draftActions.length === 0}
						className="h-24 font-mono"
					/>
				</div>
				<div className="space-y-1">
					<Label htmlFor={`${uid}-resources`}>Resources (one per line)</Label>
					<Textarea
						id={`${uid}-resources`}
						placeholder="arn:aws:dynamodb:us-east-1:000000000000:table/example"
						value={draft.resources}
						onChange={(e) => setDraft({ ...draft, resources: e.target.value })}
						aria-invalid={draft.resources !== "" && draftResources.length === 0}
						className="h-24 font-mono"
					/>
				</div>
				{!draftValid && (
					<p className="text-sm text-muted-foreground">
						A statement needs at least one action and one resource.
					</p>
				)}
				{draftWildcard && (
					<Alert variant="destructive">
						<AlertDescription>
							"*" allows every action on these resources. Prefer the specific
							actions the workload calls.
						</AlertDescription>
					</Alert>
				)}
				<div className="flex justify-end gap-2">
					<Button size="sm" variant="ghost" onClick={close}>
						<X />
						Cancel
					</Button>
					<Button size="sm" onClick={save} disabled={!draftValid}>
						<Check />
						{editing === "new" ? "Add statement" : "Save"}
					</Button>
				</div>
			</CardContent>
		</Card>
	);

	return (
		<Card data-testid="workload-policy-editor">
			<CardHeader>
				<div className="flex items-center justify-between gap-2">
					<div className="space-y-1.5">
						<CardTitle className="flex items-center gap-2">
							<Shield className="size-5" />
							Custom IAM Policy
						</CardTitle>
						<CardDescription>
							Statements added to <code className="font-mono">{roleName}</code>{" "}
							as one managed policy
						</CardDescription>
					</div>
					<Button size="sm" onClick={openNew} disabled={editing !== null}>
						<Plus />
						Add statement
					</Button>
				</div>
			</CardHeader>
			<CardContent className="space-y-3">
				{editing === "new" && form}

				{statements.map((statement, index) =>
					editing === index ? (
						// biome-ignore lint/suspicious/noArrayIndexKey: statements have no id in the YAML; position is their identity
						<div key={index}>{form}</div>
					) : (
						<Card
							// biome-ignore lint/suspicious/noArrayIndexKey: statements have no id in the YAML; position is their identity
							key={index}
							data-testid="policy-statement"
						>
							<CardContent inset="standalone" className="space-y-2">
								<div className="flex items-center justify-between gap-2">
									<div className="flex items-center gap-2">
										<Badge
											variant={
												statement.effect === "Deny" ? "destructive" : "default"
											}
										>
											{statement.effect ?? "Allow"}
										</Badge>
										{(statement.conditions?.length ?? 0) > 0 && (
											<Badge variant="outline">
												{statement.conditions?.length} condition
												{statement.conditions?.length === 1 ? "" : "s"} (edit in
												YAML)
											</Badge>
										)}
									</div>
									<div className="flex items-center gap-1">
										<Button
											size="icon"
											variant="ghost"
											aria-label={`Edit statement ${index + 1}`}
											onClick={() => openEdit(index)}
											disabled={editing !== null}
										>
											<Edit2 />
										</Button>
										<Button
											size="icon"
											variant="ghost"
											aria-label={`Delete statement ${index + 1}`}
											onClick={() => remove(index)}
										>
											<Trash2 />
										</Button>
									</div>
								</div>
								<div className="flex flex-wrap gap-1">
									{statement.actions.map((action) => (
										<Badge
											key={action}
											variant="secondary"
											className="font-mono"
										>
											{action}
										</Badge>
									))}
								</div>
								<div className="space-y-1">
									{statement.resources.map((resource) => (
										<code
											key={resource}
											className="block break-all font-mono text-sm text-muted-foreground"
										>
											{resource}
										</code>
									))}
								</div>
							</CardContent>
						</Card>
					),
				)}

				{statements.length === 0 && editing === null && (
					<p className="py-4 text-center text-sm text-muted-foreground">
						No custom statements. Add one to grant this workload access to
						another AWS resource.
					</p>
				)}
			</CardContent>
		</Card>
	);
}
