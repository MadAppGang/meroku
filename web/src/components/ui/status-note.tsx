import { cva } from "class-variance-authority";
import { AlertTriangle, Ban, CheckCircle2, Info, Loader2 } from "lucide-react";
import type * as React from "react";

import { cn } from "./utils";

/**
 * A one-line status with an icon: the result of a check, an inline field
 * error, a hint. Lighter than `Alert`, which is a bordered box; a note sits
 * inside a card next to the field it is about.
 */
export type StatusNoteTone =
	| "success"
	| "warning"
	| "error"
	| "neutral"
	| "pending";

const statusNoteIcon = cva("mt-0.5 size-4 shrink-0", {
	variants: {
		tone: {
			success: "text-tech-success",
			warning: "text-tech-warning",
			error: "text-danger",
			neutral: "text-muted-foreground",
			pending: "text-muted-foreground animate-spin motion-reduce:animate-none",
		},
	},
	defaultVariants: { tone: "neutral" },
});

/** The text takes the tone's colour only for errors; others stay neutral. */
const statusNoteText = cva("min-w-0 flex-1 space-y-1", {
	variants: {
		tone: {
			success: "text-card-foreground",
			warning: "text-card-foreground",
			error: "text-danger",
			neutral: "text-card-foreground",
			pending: "text-card-foreground",
		},
	},
	defaultVariants: { tone: "neutral" },
});

const STATUS_NOTE_ICONS = {
	success: CheckCircle2,
	warning: AlertTriangle,
	error: Ban,
	neutral: Info,
	pending: Loader2,
} as const;

function StatusNote({
	tone = "neutral",
	title,
	id,
	children,
}: {
	tone?: StatusNoteTone;
	/** Technical detail, shown as a native tooltip. */
	title?: string;
	id?: string;
	children: React.ReactNode;
}) {
	const Icon = STATUS_NOTE_ICONS[tone];
	return (
		<div
			id={id}
			data-slot="status-note"
			data-tone={tone}
			className="flex items-start gap-2 text-sm"
			title={title}
		>
			<Icon className={statusNoteIcon({ tone })} aria-hidden="true" />
			<div className={statusNoteText({ tone })}>{children}</div>
		</div>
	);
}

/** Secondary text: an explanation under a field, a choice or a note. */
function NoteDetail({ className, ...props }: React.ComponentProps<"p">) {
	return (
		<p
			data-slot="note-detail"
			className={cn("text-xs text-muted-foreground", className)}
			{...props}
		/>
	);
}

/** A technical value inside plain copy: an id, a range, a command. */
function NoteCode({ className, ...props }: React.ComponentProps<"code">) {
	return (
		<code
			data-slot="note-code"
			className={cn("font-mono text-xs text-muted-foreground", className)}
			{...props}
		/>
	);
}

export { NoteCode, NoteDetail, StatusNote };
