import type { ReactNode } from "react";

export type PropertyFieldLayoutMode = "two-column" | "vertical";

export function PropertyFieldLayout({
	children,
	mode = "two-column",
	variant = "default",
}: {
	children: ReactNode;
	mode?: PropertyFieldLayoutMode;
	variant?: "default" | "runtime" | "single";
}) {
	const variantClass =
		variant === "runtime"
			? " mp-compact-fields--runtime"
			: variant === "single"
				? " mp-compact-fields--single"
				: "";

	return (
		<div
			className={`mp-compact-fields mp-property-layout${variantClass}`}
			data-layout={mode}
			data-variant={variant}
		>
			{children}
		</div>
	);
}

/** Compatibility adapter for existing compact panels. New compositions use PropertyFieldLayout. */
export function PropertyFieldRow({
	children,
	variant = "default",
}: {
	children: ReactNode;
	variant?: "default" | "runtime" | "single";
}) {
	return (
		<PropertyFieldLayout
			mode={variant === "single" ? "vertical" : "two-column"}
			variant={variant}
		>
			{children}
		</PropertyFieldLayout>
	);
}
