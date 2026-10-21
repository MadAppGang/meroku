/**
 * properties-v3 — save bar layout parts.
 *
 * Encapsulates the styled DOM of the multi-channel save bar so
 * PropertySaveBar composes without raw elements. Buttons reuse the shared
 * mp-button styling through a semantic wrapper.
 */

import { RotateCcw } from "lucide-react";
import type { ReactNode } from "react";
import { Button } from "../../ui/button";

export interface SaveBarSurfaceProps {
	state: "clean" | "dirty" | "saving";
	children: ReactNode;
}

export function SaveBarSurface({ state, children }: SaveBarSurfaceProps) {
	return (
		<footer className="mp-savebar mp-v3-save-bar" data-state={state}>
			{children}
		</footer>
	);
}

export function SaveBarChannelRow({ children }: { children: ReactNode }) {
	return <div className="mp-v3-save-bar__channel">{children}</div>;
}

export function SaveBarChannelSummary({ children }: { children: ReactNode }) {
	return <div className="mp-v3-save-bar__channel-summary">{children}</div>;
}

export function SaveBarDeployStatus({ children }: { children: ReactNode }) {
	return <div className="mp-v3-save-bar__deploy-status">{children}</div>;
}

export function SaveBarActions({ children }: { children: ReactNode }) {
	return <div className="mp-v3-save-bar__actions">{children}</div>;
}

export interface SaveBarButtonProps {
	children: ReactNode;
	variant?: "primary" | "ghost";
	disabled?: boolean;
	onClick?: () => void;
}

export function SaveBarButton({
	children,
	variant = "primary",
	disabled,
	onClick,
}: SaveBarButtonProps) {
	return (
		<Button
			type="button"
			variant={variant === "ghost" ? "ghost" : undefined}
			className={
				variant === "ghost"
					? "mp-button mp-button--ghost"
					: "mp-button mp-button--primary"
			}
			disabled={disabled}
			onClick={onClick}
		>
			{children}
		</Button>
	);
}

export function SaveBarDiscardIcon() {
	return <RotateCcw aria-hidden="true" />;
}
