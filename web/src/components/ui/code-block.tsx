import { cva, type VariantProps } from "class-variance-authority";
import type * as React from "react";

import { cn } from "./utils";

const codeBlockVariants = cva(
	"overflow-x-auto whitespace-pre font-mono text-xs outline-none focus-visible:ring-2 focus-visible:ring-ring/50",
	{
		variants: {
			surface: {
				// On a card: its own recessed box.
				muted: "rounded-lg bg-muted p-3 text-muted-foreground",
				// Inside a box that is already recessed, such as an inset Card.
				plain: "text-muted-foreground",
			},
		},
		defaultVariants: {
			surface: "muted",
		},
	},
);

interface CodeBlockProps
	extends Omit<React.ComponentProps<"figure">, "children" | "title">,
		VariantProps<typeof codeBlockVariants> {
	/** What the example is, shown above the code. */
	title?: React.ReactNode;
	/** The code, as written: JSON, a command, a policy. */
	children: string;
}

/**
 * A block of code shown as written, such as an example event. Read-only; an
 * editable one is a Textarea with `variant="code"`.
 */
function CodeBlock({
	title,
	surface,
	className,
	children,
	...props
}: CodeBlockProps) {
	return (
		<figure
			data-slot="code-block"
			className={cn("min-w-0 space-y-2", className)}
			{...props}
		>
			{title !== undefined && (
				<figcaption className="text-sm font-medium text-foreground">
					{title}
				</figcaption>
			)}
			{/* biome-ignore lint/a11y/noNoninteractiveTabindex: a scrollable region must be focusable so a keyboard can scroll a line wider than the block (axe scrollable-region-focusable). */}
			<pre tabIndex={0} className={codeBlockVariants({ surface })}>
				<code>{children}</code>
			</pre>
		</figure>
	);
}

export { CodeBlock, codeBlockVariants };
