/**
 * properties-v3 — inline notice primitive.
 *
 * Used for the visibility-controller reset notice (naming discarded inactive
 * changes), save failures, and action errors. Dismissible.
 */

import type { ReactNode } from "react";
import { PropertyActionButton } from "../../properties/PropertyPanelPrimitives";
import { Stack, Text } from "./Structural";

export interface PropertyNoticeProps {
	title: string;
	items?: readonly string[];
	children?: ReactNode;
	tone?: "warning" | "danger" | "info";
	onDismiss?: () => void;
}

export function PropertyNotice({
	title,
	items,
	children,
	tone = "warning",
	onDismiss,
}: PropertyNoticeProps) {
	return (
		<output className="mp-v3-notice" data-tone={tone}>
			<div className="mp-v3-notice__body">
				<div className="mp-v3-notice__title">{title}</div>
				{items && items.length > 0 ? (
					<ul className="mp-v3-notice__list">
						{items.map((item) => (
							<li key={item}>{item}</li>
						))}
					</ul>
				) : null}
				{children ? (
					<Stack gap={2}>
						<Text variant="muted">{children}</Text>
					</Stack>
				) : null}
			</div>
			{onDismiss ? (
				<div className="mp-v3-notice__dismiss">
					<PropertyActionButton onClick={onDismiss}>
						Dismiss
					</PropertyActionButton>
				</div>
			) : null}
		</output>
	);
}
