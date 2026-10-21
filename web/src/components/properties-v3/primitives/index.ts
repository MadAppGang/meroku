/**
 * properties-v3 — UI primitives barrel.
 *
 * Low-level primitives encapsulate all styled DOM. Higher layers (archetypes,
 * PropertyPanel, PropertySaveBar) compose these and the stable `properties/`
 * library components read-only. No imports from properties-v2.
 */

export {
	AutoscalingGroupComposite,
	type AutoscalingGroupCompositeProps,
} from "./AutoscalingGroupComposite";
export {
	type FieldActionState,
	FieldControl,
	type FieldControlProps,
} from "./FieldControl";
export {
	PropertyConfirmDialog,
	type PropertyConfirmDialogProps,
} from "./PropertyConfirmDialog";
export {
	PropertyImpactBadge,
	type PropertyImpactBadgeProps,
} from "./PropertyImpactBadge";
export {
	type KeyValueColumn,
	PropertyKeyValueTable,
	type PropertyKeyValueTableProps,
} from "./PropertyKeyValueTable";
export {
	PropertyLogViewerField,
	type PropertyLogViewerFieldProps,
} from "./PropertyLogViewerField";
export { PropertyNotice, type PropertyNoticeProps } from "./PropertyNotice";
export {
	PropertyTelemetry,
	type PropertyTelemetryProps,
} from "./PropertyTelemetry";
export {
	SaveBarButton,
	type SaveBarButtonProps,
	SaveBarChannelRow,
	SaveBarChannelSummary,
	SaveBarDeployStatus,
	SaveBarDiscardIcon,
	SaveBarSurface,
	type SaveBarSurfaceProps,
} from "./SaveBarParts";
export {
	Grid,
	type GridProps,
	Row,
	type RowProps,
	Section,
	type SectionProps,
	Stack,
	type StackProps,
	Text,
	type TextProps,
} from "./Structural";
