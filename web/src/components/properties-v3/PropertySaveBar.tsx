/**
 * properties-v3 — multi-channel save bar (Clarification 1).
 *
 * One save button per DIRTY persistence channel, each with its own
 * channel-scoped worst-impact summary:
 * - "Save configuration"  (configuration channel)
 * - "Apply live changes"  (live channel)
 * - "Save layout"         (layout channel)
 *
 * The single global discard resets ALL draft channels ("Discard all
 * changes"). Saving one channel never touches the others. Saved status is
 * explicitly separate from deployed status; the deploy-information action is
 * a demo (informational) action only.
 */

import { PropertyActionButton } from "../properties/PropertyPanelPrimitives";
import { getImpactMetadata } from "./helpers";
import {
	PropertyImpactBadge,
	SaveBarButton,
	SaveBarChannelRow,
	SaveBarChannelSummary,
	SaveBarDeployStatus,
	SaveBarDiscardIcon,
	SaveBarSurface,
	Stack,
	Text,
} from "./primitives";
import type { PersistenceChannel } from "./types";
import type { PanelDraftController } from "./usePanelDraftState";

export interface PropertySaveBarProps {
	controller: PanelDraftController;
	/** Canvas-only node: no infrastructure deployment semantics. */
	canvasOnly?: boolean;
	/** Whether the currently saved configuration is deployed (adapter/demo). */
	deployed?: boolean | null;
	/** Demo action: show deployment information. Never runs AWS calls. */
	onShowDeployInfo?: () => void;
	onSaveChannel: (channel: PersistenceChannel) => void;
	onDiscardAll: () => void;
}

const CHANNEL_LABELS: Readonly<Record<PersistenceChannel, string>> = {
	configuration: "Save configuration",
	live: "Apply live changes",
	layout: "Save layout",
};

const CHANNEL_ORDER: readonly PersistenceChannel[] = [
	"configuration",
	"live",
	"layout",
];

export function PropertySaveBar({
	controller,
	canvasOnly = false,
	deployed = null,
	onShowDeployInfo,
	onSaveChannel,
	onDiscardAll,
}: PropertySaveBarProps) {
	const { diff, channelStatus, channelError, submitting } = controller;
	const dirty = diff.dirtyChannels;
	const anyDirty = dirty.length > 0;

	return (
		<SaveBarSurface
			state={submitting ? "saving" : anyDirty ? "dirty" : "clean"}
		>
			<Stack gap={6}>
				{anyDirty ? (
					<Stack gap={4}>
						{CHANNEL_ORDER.filter((channel) => dirty.includes(channel)).map(
							(channel) => {
								const summary = diff.channelSummaries[channel];
								const status = channelStatus[channel];
								const error = channelError[channel];
								return (
									<SaveBarChannelRow key={channel}>
										<SaveBarChannelSummary>
											<Text as="span">
												{CHANNEL_LABELS[channel]} — {summary?.changeCount ?? 0}{" "}
												{(summary?.changeCount ?? 0) === 1
													? "change"
													: "changes"}
											</Text>
											{summary?.worstImpact ? (
												<PropertyImpactBadge
													impact={summary.worstImpact}
													compact
												/>
											) : null}
											{summary?.worstImpact ? (
												<Text as="span" variant="muted">
													{getImpactMetadata(summary.worstImpact).summary}
												</Text>
											) : null}
											{status === "error" && error ? (
												<Text as="span" variant="muted">
													{error}
												</Text>
											) : null}
										</SaveBarChannelSummary>
										<SaveBarButton
											disabled={submitting}
											onClick={() => onSaveChannel(channel)}
										>
											{status === "submitting"
												? "Saving…"
												: CHANNEL_LABELS[channel]}
										</SaveBarButton>
									</SaveBarChannelRow>
								);
							},
						)}
					</Stack>
				) : (
					<Text as="span">No unsaved changes</Text>
				)}

				<SaveBarDeployStatus>
					{canvasOnly ? (
						<Text as="span" variant="muted">
							Canvas only
						</Text>
					) : (
						<Text as="span" variant="muted">
							{deployed === null
								? "Deployment status unknown"
								: deployed
									? "Saved configuration is deployed"
									: "Not deployed — configuration pending deployment"}
						</Text>
					)}
					{!canvasOnly && onShowDeployInfo ? (
						<PropertyActionButton onClick={onShowDeployInfo}>
							Deployment info (demo)
						</PropertyActionButton>
					) : null}
					{anyDirty ? (
						<SaveBarButton
							variant="ghost"
							disabled={submitting}
							onClick={onDiscardAll}
						>
							<SaveBarDiscardIcon /> Discard all changes
						</SaveBarButton>
					) : null}
				</SaveBarDeployStatus>
			</Stack>
		</SaveBarSurface>
	);
}
