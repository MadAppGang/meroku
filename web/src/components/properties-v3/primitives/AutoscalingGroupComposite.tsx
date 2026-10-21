/**
 * properties-v3 — autoscaling group composite with an impact badge slot.
 *
 * The shared library composite (PropertyAutoscalingGroup) has no slot for the
 * per-field impact badge that every editable v3 field must carry, so the v3
 * variant reproduces its exact markup (same classnames, same enabled/disabled
 * fields swap) with a badge header above the toggle. The badge itself is
 * derived by the caller from the schema field's mutability.
 */

import type { ReactNode } from "react";
import { PropertyCheckboxField } from "../../properties/PropertyPanelPrimitives";

export interface AutoscalingGroupCompositeProps {
	enabled: boolean;
	onEnabledChange: (enabled: boolean) => void;
	/** Schema label of the boolean toggle field. */
	ariaLabel: string;
	/** Impact badge for the toggle (mutability-derived, never hardcoded). */
	badge?: ReactNode;
	enabledFields: ReactNode;
	disabledFields: ReactNode;
}

export function AutoscalingGroupComposite({
	enabled,
	onEnabledChange,
	ariaLabel,
	badge,
	enabledFields,
	disabledFields,
}: AutoscalingGroupCompositeProps) {
	return (
		<div className="mp-autoscaling-group" data-enabled={enabled}>
			<div className="mp-v3-field-item">
				{badge ? <div className="mp-v3-field-item__header">{badge}</div> : null}
				<PropertyCheckboxField
					className="mp-autoscaling-group__toggle"
					item={{
						id: "autoscaling",
						label: ariaLabel,
						checked: enabled,
						onCheckedChange: onEnabledChange,
					}}
					label="Autoscaling"
				/>
			</div>
			<div className="mp-autoscaling-group__fields">
				{enabled ? enabledFields : disabledFields}
			</div>
		</div>
	);
}
