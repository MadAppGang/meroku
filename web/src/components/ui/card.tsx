import { cva, type VariantProps } from "class-variance-authority";
import type * as React from "react";

import { cn } from "./utils";

function Card({ className, ...props }: React.ComponentProps<"div">) {
	return (
		<div
			data-slot="card"
			className={cn(
				"bg-card text-card-foreground flex flex-col gap-6 rounded-xl border",
				className,
			)}
			{...props}
		/>
	);
}

function CardHeader({ className, ...props }: React.ComponentProps<"div">) {
	return (
		<div
			data-slot="card-header"
			className={cn(
				"@container/card-header grid auto-rows-min grid-rows-[auto_auto] items-start gap-1.5 px-6 pt-6 has-data-[slot=card-action]:grid-cols-[1fr_auto] [.border-b]:pb-6",
				className,
			)}
			{...props}
		/>
	);
}

const cardTitleVariants = cva("leading-none", {
	variants: {
		variant: {
			default: "",
			// A title that is a path or a name as written, such as a folder.
			code: "font-mono text-sm",
		},
	},
	defaultVariants: {
		variant: "default",
	},
});

function CardTitle({
	className,
	variant,
	...props
}: React.ComponentProps<"div"> & VariantProps<typeof cardTitleVariants>) {
	return (
		<h4
			data-slot="card-title"
			className={cn(cardTitleVariants({ variant }), className)}
			{...props}
		/>
	);
}

const cardDescriptionVariants = cva("text-muted-foreground", {
	variants: {
		variant: {
			default: "",
			// A description that is an identifier, such as an env var name.
			code: "font-mono",
		},
	},
	defaultVariants: {
		variant: "default",
	},
});

function CardDescription({
	className,
	variant,
	...props
}: React.ComponentProps<"div"> & VariantProps<typeof cardDescriptionVariants>) {
	return (
		<p
			data-slot="card-description"
			className={cn(cardDescriptionVariants({ variant }), className)}
			{...props}
		/>
	);
}

function CardAction({ className, ...props }: React.ComponentProps<"div">) {
	return (
		<div
			data-slot="card-action"
			className={cn(
				"col-start-2 row-span-2 row-start-1 self-start justify-self-end",
				className,
			)}
			{...props}
		/>
	);
}

const cardContentVariants = cva("px-6 [&:last-child]:pb-6", {
	variants: {
		inset: {
			// Below a CardHeader, which supplies the space above.
			default: "",
			// The card's only section, with no header above it.
			standalone: "pt-6",
		},
	},
	defaultVariants: {
		inset: "default",
	},
});

function CardContent({
	className,
	inset,
	...props
}: React.ComponentProps<"div"> & VariantProps<typeof cardContentVariants>) {
	return (
		<div
			data-slot="card-content"
			className={cn(cardContentVariants({ inset }), className)}
			{...props}
		/>
	);
}

function CardFooter({ className, ...props }: React.ComponentProps<"div">) {
	return (
		<div
			data-slot="card-footer"
			className={cn("flex items-center px-6 pb-6 [.border-t]:pt-6", className)}
			{...props}
		/>
	);
}

export {
	Card,
	CardHeader,
	CardFooter,
	CardTitle,
	CardAction,
	CardDescription,
	CardContent,
};
