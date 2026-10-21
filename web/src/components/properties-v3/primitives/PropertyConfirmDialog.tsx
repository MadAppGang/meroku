/**
 * properties-v3 — destructive-change confirmation dialog.
 *
 * Lists the EXACT reviewed payload: resource names plus field/property paths
 * of every replacement-dirty change. The confirmed payload is immutable —
 * the caller disables edits while submitting and re-arms the dialog whenever
 * the pending operations change. Cancel preserves the draft untouched.
 *
 * Wording: saving persists desired state; the NEXT DEPLOY may recreate
 * resources and lose data. On an uncreated resource, wording frames the same
 * gate as initial creation (no false destruction claim).
 */

import { Button } from "../../ui/button";
import {
	Dialog,
	DialogContent,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "../../ui/dialog";
import type { AffectedResource, PropertyChange } from "../types";
import { Text } from "./Structural";

export interface PropertyConfirmDialogProps {
	open: boolean;
	resourceName: string;
	/** Immutable reviewed operations for one persistence channel. */
	changes: readonly PropertyChange[];
	/** Resources named by destructive changes, with their field paths. */
	resources: readonly AffectedResource[];
	/** Field labels by field id — every listed change shows its exact label. */
	fieldLabels?: Readonly<Record<string, string>>;
	/** True when the node has not been created yet (creation wording). */
	uncreated: boolean;
	onConfirm: () => void;
	onCancel: () => void;
}

/** Sentinel id for changes with no explicit affected resources. */
const PANEL_FALLBACK_RESOURCE_ID = "__panel__";

export function PropertyConfirmDialog({
	open,
	resourceName,
	changes,
	resources,
	fieldLabels,
	uncreated,
	onConfirm,
	onCancel,
}: PropertyConfirmDialogProps) {
	if (!open) return null;

	// Group every destructive change by its EXPLICIT affected resources;
	// changes without any land under the panel-identity fallback so the
	// warning list is never empty and never omits a reviewed change.
	interface ListedChange {
		fieldId: string;
		label: string;
		propertyPath: string;
	}
	const changesByResource = new Map<string, ListedChange[]>();
	for (const change of changes) {
		if (change.impact !== "recreates-data-loss") continue;
		const listed: ListedChange = {
			fieldId: change.fieldId,
			label: fieldLabels?.[change.fieldId] ?? change.fieldId,
			propertyPath: change.propertyPath,
		};
		const targets =
			change.affectedResources.length > 0
				? change.affectedResources.map((resource) => resource.id)
				: [PANEL_FALLBACK_RESOURCE_ID];
		for (const id of targets) {
			const list = changesByResource.get(id) ?? [];
			list.push(listed);
			changesByResource.set(id, list);
		}
	}
	const knownResources = new Map(
		resources.map((resource) => [resource.id, resource]),
	);
	const groups = [...changesByResource.entries()].map(([id, list]) => ({
		resource:
			knownResources.get(id) ??
			(id === PANEL_FALLBACK_RESOURCE_ID
				? { id, name: resourceName }
				: { id, name: id }),
		changes: list,
	}));

	return (
		<Dialog
			open={open}
			onOpenChange={(next) => (next ? undefined : onCancel())}
		>
			<DialogContent>
				<DialogHeader>
					<DialogTitle>
						{uncreated
							? "Confirm initial creation settings"
							: "Confirm destructive configuration"}
					</DialogTitle>
				</DialogHeader>
				<Text>
					{uncreated
						? `${resourceName} has not been created yet — no existing data is affected. These settings configure the new resource; some can never be changed afterwards.`
						: `Saving changes to ${resourceName} introduces replacement changes. Saving itself does not destroy anything, but the next deploy may recreate the resources below and lose their data.`}
				</Text>
				<ul className="mp-v3-confirm__resources">
					{groups.map(({ resource, changes: listed }) => (
						<li key={resource.id} className="mp-v3-confirm__resource">
							<div className="mp-v3-confirm__resource-name">
								{resource.name}
							</div>
							<ul className="mp-v3-confirm__paths">
								{listed.map((change) => (
									<li key={`${change.fieldId}-${change.propertyPath}`}>
										{change.label} ({change.propertyPath})
									</li>
								))}
							</ul>
						</li>
					))}
				</ul>
				<Text variant="muted" as="span">
					Saving persists desired state to project YAML. Cloud infrastructure is
					modified only on the next deployment.
				</Text>
				<DialogFooter className="mp-v3-confirm__footer">
					<Button type="button" variant="secondary" onClick={onCancel}>
						Cancel
					</Button>
					<Button type="button" variant="destructive" onClick={onConfirm}>
						{uncreated ? "Confirm & Save" : "Confirm & Save Configuration"}
					</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
}
