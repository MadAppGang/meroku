import type React from "react";

interface StyledSectionProps {
	title: string;
	description?: string;
	icon: React.ElementType;
	iconColor?: string;
	children: React.ReactNode;
	className?: string;
	/** Optional action buttons to display on the right side of the header */
	actions?: React.ReactNode;
}

/**
 * A styled section component with icon, title, and description.
 * Non-collapsible version of CollapsibleSection - content is always visible.
 * Use this for tab content to maintain consistent styling across the app.
 */
export function StyledSection({
	title,
	description,
	icon: Icon,
	iconColor = "text-gray-400",
	children,
	className = "",
	actions,
}: StyledSectionProps) {
	return (
		<div
			className={`border border-gray-700 rounded-xl overflow-hidden bg-gray-800/30 ${className}`}
		>
			<div className="flex items-center justify-between px-4 py-3 border-b border-gray-700/50">
				<div className="flex items-center gap-3">
					<div className={`p-1.5 rounded-lg bg-gray-800 ${iconColor}`}>
						<Icon className="w-4 h-4" />
					</div>
					<div>
						<h3 className="font-medium text-white text-sm">{title}</h3>
						{description && (
							<p className="text-xs text-gray-500">{description}</p>
						)}
					</div>
				</div>
				{actions && <div className="flex items-center gap-2">{actions}</div>}
			</div>
			<div className="px-4 py-4">{children}</div>
		</div>
	);
}
