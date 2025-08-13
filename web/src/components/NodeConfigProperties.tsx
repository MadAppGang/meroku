import type React from "react";
import type { ComponentNode } from "../types";
import { Badge } from "./ui/badge";
import {
	Card,
	CardContent,
	CardDescription,
	CardHeader,
	CardTitle,
} from "./ui/card";

interface NodeConfigPropertiesProps {
	node: ComponentNode;
}

export function NodeConfigProperties({ node }: NodeConfigPropertiesProps) {
	const { configProperties } = node;

	if (!configProperties || Object.keys(configProperties).length === 0) {
		return (
			<Card>
				<CardHeader>
					<CardTitle>Configuration</CardTitle>
					<CardDescription>
						No configuration properties available
					</CardDescription>
				</CardHeader>
			</Card>
		);
	}

	const renderValue = (value: any): React.ReactNode => {
		if (typeof value === "boolean") {
			return (
				<Badge variant={value ? "default" : "secondary"}>
					{value ? "Enabled" : "Disabled"}
				</Badge>
			);
		}
		if (Array.isArray(value)) {
			if (value.length === 0)
				return <span className="text-gray-500">None</span>;
			return (
				<div className="space-y-1">
					{value.map((item, index) => (
						<div key={index} className="text-sm">
							{typeof item === "object" ? JSON.stringify(item, null, 2) : item}
						</div>
					))}
				</div>
			);
		}
		if (typeof value === "object" && value !== null) {
			return (
				<pre className="text-xs bg-gray-800 p-2 rounded">
					{JSON.stringify(value, null, 2)}
				</pre>
			);
		}
		return value?.toString() || <span className="text-gray-500">Not set</span>;
	};

	const formatKey = (key: string): string => {
		return key
			.split(/(?=[A-Z])/)
			.join(" ")
			.replace(/_/g, " ")
			.replace(/\b\w/g, (l) => l.toUpperCase());
	};

	return (
		<Card>
			<CardHeader>
				<CardTitle>Configuration Properties</CardTitle>
				<CardDescription>Based on YAML configuration</CardDescription>
			</CardHeader>
			<CardContent>
				<div className="space-y-3">
					{Object.entries(configProperties).map(([key, value]) => (
						<div
							key={key}
							className="border-b border-gray-800 pb-2 last:border-0"
						>
							<div className="text-sm font-medium text-gray-400 mb-1">
								{formatKey(key)}
							</div>
							<div className="text-sm text-white">{renderValue(value)}</div>
						</div>
					))}
				</div>
			</CardContent>
		</Card>
	);
}
