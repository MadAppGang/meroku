/**
 * properties-v3 — structural layout primitives (Clarification 6).
 *
 * Archetypes and the panel compose ONLY these plus library components; no
 * raw styled DOM appears above this layer. Layout-only styling lives here.
 */

import type { CSSProperties, ReactNode } from "react";

const gapStyle = (gap: number): CSSProperties =>
	({ "--mp-v3-gap": `${gap}px` }) as CSSProperties;

export interface StackProps {
	children: ReactNode;
	/** Layout gap in pixels (parent owns spacing). */
	gap?: number;
	/** Layout-only class (placement), never appearance. */
	className?: string;
}

export function Stack({ children, gap = 8, className }: StackProps) {
	return (
		<div
			className={className ? `mp-v3-stack ${className}` : "mp-v3-stack"}
			style={gapStyle(gap)}
		>
			{children}
		</div>
	);
}

export interface RowProps {
	children: ReactNode;
	gap?: number;
}

export function Row({ children, gap = 8 }: RowProps) {
	return (
		<div className="mp-v3-row" style={gapStyle(gap)}>
			{children}
		</div>
	);
}

export interface GridProps {
	children: ReactNode;
	gap?: number;
}

export function Grid({ children, gap = 8 }: GridProps) {
	return (
		<div className="mp-v3-grid" style={gapStyle(gap)}>
			{children}
		</div>
	);
}

export interface TextProps {
	children: ReactNode;
	variant?: "default" | "muted" | "mono";
	as?: "p" | "span" | "strong";
}

export function Text({ children, variant = "default", as = "p" }: TextProps) {
	const Tag = as;
	return (
		<Tag className="mp-v3-text" data-variant={variant}>
			{children}
		</Tag>
	);
}

export interface SectionProps {
	title: string;
	description?: string;
	children: ReactNode;
}

export function Section({ title, description, children }: SectionProps) {
	return (
		<div className="mp-v3-stack" style={gapStyle(6)}>
			<Text as="strong">{title}</Text>
			{description ? <Text variant="muted">{description}</Text> : null}
			{children}
		</div>
	);
}
