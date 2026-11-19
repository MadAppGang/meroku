import { cva, type VariantProps } from "class-variance-authority";
import type * as React from "react";

import { cn } from "../utils";

const keyValueListVariants = cva("grid grid-cols-label-value gap-x-3", {
	variants: {
		size: {
			// The summary at the top of a card.
			default: "items-center gap-y-2 text-sm",
			// Facts inside a compact or inset card.
			compact: "items-baseline gap-y-1 text-xs",
		},
	},
	defaultVariants: {
		size: "default",
	},
});

const keyValueValueVariants = cva("min-w-0", {
	variants: {
		variant: {
			// Plain copy.
			default: "text-foreground",
			// A name as written: a bus, a rule, an id.
			code: "break-all font-mono text-foreground",
			// A row of badges or tags.
			tags: "flex flex-wrap gap-2",
		},
	},
	defaultVariants: {
		variant: "default",
	},
});

/**
 * Labelled facts, one per row: a label on the left, its value on the right.
 * Compose with `KeyValue` rows.
 */
function KeyValueList({
	size,
	className,
	...props
}: React.ComponentProps<"dl"> & VariantProps<typeof keyValueListVariants>) {
	return (
		<dl
			data-slot="key-value-list"
			className={cn(keyValueListVariants({ size }), className)}
			{...props}
		/>
	);
}

interface KeyValueProps extends VariantProps<typeof keyValueValueVariants> {
	label: React.ReactNode;
	children: React.ReactNode;
}

/** One row of a `KeyValueList`. */
function KeyValue({ label, variant, children }: KeyValueProps) {
	return (
		<>
			<dt className="text-muted-foreground">{label}</dt>
			<dd className={keyValueValueVariants({ variant })}>{children}</dd>
		</>
	);
}

export { KeyValue, KeyValueList };
