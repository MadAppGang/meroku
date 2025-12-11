import {
	ArrowRight,
	Box,
	ChevronDown,
	ChevronRight,
	Edit2,
	Filter,
	Plus,
	Radio,
	Trash2,
	X,
	Zap,
} from "lucide-react";
import { useId, useState } from "react";
import type { EventBridgeRule } from "../types/yamlConfig";
import { Button } from "./ui/button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "./ui/dialog";
import { Input } from "./ui/input";
import { Label } from "./ui/label";

interface EventRulesListProps {
	rules: EventBridgeRule[];
	onRulesChange: (rules: EventBridgeRule[]) => void;
}

export function EventRulesList({ rules, onRulesChange }: EventRulesListProps) {
	const [editingRule, setEditingRule] = useState<EventBridgeRule | null>(null);
	const [editingIndex, setEditingIndex] = useState<number | null>(null);
	const [isDialogOpen, setIsDialogOpen] = useState(false);
	const [expandedRules, setExpandedRules] = useState<Set<number>>(new Set([0])); // First rule expanded by default

	const handleAddRule = () => {
		setEditingRule({ name: "", sources: [], detail_types: [] });
		setEditingIndex(null);
		setIsDialogOpen(true);
	};

	const handleEditRule = (index: number) => {
		setEditingRule({ ...rules[index] });
		setEditingIndex(index);
		setIsDialogOpen(true);
	};

	const handleDeleteRule = (index: number) => {
		const updated = rules.filter((_, i) => i !== index);
		onRulesChange(updated);
	};

	const handleSaveRule = (rule: EventBridgeRule) => {
		if (editingIndex !== null) {
			const updated = [...rules];
			updated[editingIndex] = rule;
			onRulesChange(updated);
		} else {
			onRulesChange([...rules, rule]);
			// Expand the newly added rule
			setExpandedRules(new Set([...expandedRules, rules.length]));
		}
		setIsDialogOpen(false);
		setEditingRule(null);
		setEditingIndex(null);
	};

	const toggleExpanded = (index: number) => {
		const newExpanded = new Set(expandedRules);
		if (newExpanded.has(index)) {
			newExpanded.delete(index);
		} else {
			newExpanded.add(index);
		}
		setExpandedRules(newExpanded);
	};

	return (
		<>
			{/* Header */}
			<div className="flex items-center justify-between mb-4">
				<div className="flex items-center gap-3">
					<div className="p-2 rounded-lg bg-amber-500/10 border border-amber-500/20">
						<Zap className="w-5 h-5 text-amber-500" />
					</div>
					<div>
						<h2 className="font-semibold text-white">EventBridge Rules</h2>
						<p className="text-sm text-gray-400">
							{rules.length === 0
								? "No rules configured"
								: `${rules.length} rule${rules.length !== 1 ? "s" : ""} will trigger this task`}
						</p>
					</div>
				</div>
				<Button onClick={handleAddRule} size="sm" className="gap-1.5">
					<Plus className="w-4 h-4" />
					Add Rule
				</Button>
			</div>

			{/* Empty State */}
			{rules.length === 0 && (
				<div className="border-2 border-dashed border-gray-700 rounded-xl p-8 text-center">
					<div className="inline-flex p-3 rounded-full bg-gray-800 mb-3">
						<Zap className="w-6 h-6 text-gray-500" />
					</div>
					<h3 className="text-gray-300 font-medium mb-1">
						No EventBridge rules
					</h3>
					<p className="text-gray-500 text-sm mb-4 max-w-md mx-auto">
						Rules define which events trigger this task. Add a rule to start
						listening for events from your services.
					</p>
					<Button
						onClick={handleAddRule}
						variant="outline"
						size="sm"
						className="gap-1.5"
					>
						<Plus className="w-4 h-4" />
						Create your first rule
					</Button>
				</div>
			)}

			{/* Rules List */}
			<div className="space-y-3">
				{rules.map((rule, index) => {
					const isExpanded = expandedRules.has(index);
					return (
						<div
							key={`${rule.name}-${index}`}
							className="bg-gray-800/50 border border-gray-700 rounded-xl overflow-hidden transition-all hover:border-gray-600"
						>
							{/* Rule Header - Always Visible */}
							<div className="flex items-center justify-between px-4 py-3">
								<button
									type="button"
									className="flex items-center gap-3 flex-1 text-left"
									onClick={() => toggleExpanded(index)}
								>
									<span className="p-0.5">
										{isExpanded ? (
											<ChevronDown className="w-4 h-4 text-gray-400" />
										) : (
											<ChevronRight className="w-4 h-4 text-gray-400" />
										)}
									</span>
									<div className="flex items-center gap-2">
										<span className="font-medium text-white">{rule.name}</span>
										<span className="text-xs text-gray-500">
											{rule.sources.length} source
											{rule.sources.length !== 1 ? "s" : ""} →{" "}
											{rule.detail_types.length} event
											{rule.detail_types.length !== 1 ? "s" : ""}
										</span>
									</div>
								</button>
								<div className="flex items-center gap-1">
									<Button
										size="sm"
										variant="ghost"
										className="h-8 w-8 p-0 text-gray-400 hover:text-white"
										onClick={() => handleEditRule(index)}
									>
										<Edit2 className="w-3.5 h-3.5" />
									</Button>
									<Button
										size="sm"
										variant="ghost"
										className="h-8 w-8 p-0 text-gray-400 hover:text-red-400"
										onClick={() => handleDeleteRule(index)}
									>
										<Trash2 className="w-3.5 h-3.5" />
									</Button>
								</div>
							</div>

							{/* Expanded Content */}
							{isExpanded && (
								<div className="px-4 pb-4 pt-1 border-t border-gray-700/50">
									{/* Visual Event Flow */}
									<div className="flex items-stretch gap-3 mt-3">
										{/* Sources Section */}
										<div className="flex-1 min-w-0">
											<div className="flex items-center gap-2 mb-2">
												<Radio className="w-3.5 h-3.5 text-blue-400" />
												<span className="text-xs font-medium text-blue-400 uppercase tracking-wide">
													Sources
												</span>
											</div>
											<div className="space-y-1.5">
												{rule.sources.map((source) => (
													<div
														key={source}
														className="flex items-center gap-2 px-3 py-2 bg-blue-500/10 border border-blue-500/20 rounded-lg"
													>
														<Box className="w-3.5 h-3.5 text-blue-400 shrink-0" />
														<span className="text-sm text-blue-300 font-mono truncate">
															{source}
														</span>
													</div>
												))}
											</div>
										</div>

										{/* Arrow */}
										<div className="flex items-center justify-center px-2">
											<div className="flex flex-col items-center gap-1">
												<ArrowRight className="w-5 h-5 text-gray-500" />
												<span className="text-[10px] text-gray-600 uppercase tracking-wider">
													triggers
												</span>
											</div>
										</div>

										{/* Detail Types Section */}
										<div className="flex-1 min-w-0">
											<div className="flex items-center gap-2 mb-2">
												<Filter className="w-3.5 h-3.5 text-emerald-400" />
												<span className="text-xs font-medium text-emerald-400 uppercase tracking-wide">
													Event Types
												</span>
											</div>
											<div className="space-y-1.5">
												{rule.detail_types.map((type) => (
													<div
														key={type}
														className="flex items-center gap-2 px-3 py-2 bg-emerald-500/10 border border-emerald-500/20 rounded-lg"
													>
														<Zap className="w-3.5 h-3.5 text-emerald-400 shrink-0" />
														<span className="text-sm text-emerald-300 font-mono truncate">
															{type}
														</span>
													</div>
												))}
											</div>
										</div>
									</div>

									{/* Pattern Preview */}
									<div className="mt-4 p-3 bg-gray-900 rounded-lg border border-gray-700">
										<div className="flex items-center gap-2 mb-2">
											<span className="text-[10px] font-medium text-gray-500 uppercase tracking-wide">
												EventBridge Pattern Preview
											</span>
										</div>
										<pre className="text-xs text-gray-400 font-mono overflow-x-auto">
											{`{
  "source": [${rule.sources.map((s) => `"${s}"`).join(", ")}],
  "detail-type": [${rule.detail_types.map((t) => `"${t}"`).join(", ")}]
}`}
										</pre>
									</div>
								</div>
							)}
						</div>
					);
				})}
			</div>

			{/* Edit/Add Dialog */}
			<RuleEditorDialog
				open={isDialogOpen}
				onOpenChange={setIsDialogOpen}
				rule={editingRule}
				onSave={handleSaveRule}
				isNew={editingIndex === null}
			/>
		</>
	);
}

interface RuleEditorDialogProps {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	rule: EventBridgeRule | null;
	onSave: (rule: EventBridgeRule) => void;
	isNew: boolean;
}

function RuleEditorDialog({
	open,
	onOpenChange,
	rule,
	onSave,
	isNew,
}: RuleEditorDialogProps) {
	const [localRule, setLocalRule] = useState<EventBridgeRule>({
		name: "",
		sources: [],
		detail_types: [],
	});
	const [newSource, setNewSource] = useState("");
	const [newDetailType, setNewDetailType] = useState("");
	const ruleNameId = useId();

	// Sync local state when rule prop changes
	useState(() => {
		if (rule) {
			setLocalRule({ ...rule });
		}
	});

	// Reset local state when dialog opens
	const handleOpenChange = (isOpen: boolean) => {
		if (isOpen && rule) {
			setLocalRule({ ...rule });
			setNewSource("");
			setNewDetailType("");
		}
		onOpenChange(isOpen);
	};

	const handleAddSource = () => {
		if (newSource && !localRule.sources.includes(newSource)) {
			setLocalRule({
				...localRule,
				sources: [...localRule.sources, newSource],
			});
			setNewSource("");
		}
	};

	const handleRemoveSource = (source: string) => {
		setLocalRule({
			...localRule,
			sources: localRule.sources.filter((s) => s !== source),
		});
	};

	const handleAddDetailType = () => {
		if (newDetailType && !localRule.detail_types.includes(newDetailType)) {
			setLocalRule({
				...localRule,
				detail_types: [...localRule.detail_types, newDetailType],
			});
			setNewDetailType("");
		}
	};

	const handleRemoveDetailType = (type: string) => {
		setLocalRule({
			...localRule,
			detail_types: localRule.detail_types.filter((t) => t !== type),
		});
	};

	const isValid =
		localRule.name.trim() !== "" &&
		localRule.sources.length > 0 &&
		localRule.detail_types.length > 0;

	return (
		<Dialog open={open} onOpenChange={handleOpenChange}>
			<DialogContent className="sm:max-w-lg">
				<DialogHeader>
					<DialogTitle className="flex items-center gap-2">
						<Zap className="w-5 h-5 text-amber-500" />
						{isNew ? "Create EventBridge Rule" : "Edit Rule"}
					</DialogTitle>
					<DialogDescription>
						Define which events from your services will trigger this task.
					</DialogDescription>
				</DialogHeader>

				<div className="space-y-5 py-4">
					{/* Rule Name */}
					<div className="space-y-2">
						<Label htmlFor={ruleNameId}>Rule Name</Label>
						<Input
							id={ruleNameId}
							value={localRule.name}
							onChange={(e) =>
								setLocalRule({ ...localRule, name: e.target.value })
							}
							placeholder="e.g., order-events, user-notifications"
							className="font-mono"
						/>
						<p className="text-xs text-gray-500">
							A unique identifier for this rule in your infrastructure
						</p>
					</div>

					{/* Sources */}
					<div className="space-y-2">
						<Label className="flex items-center gap-2">
							<Radio className="w-3.5 h-3.5 text-blue-400" />
							Event Sources
						</Label>
						<div className="flex gap-2">
							<Input
								value={newSource}
								onChange={(e) => setNewSource(e.target.value)}
								placeholder="e.g., backend, orders-service"
								className="font-mono"
								onKeyDown={(e) => {
									if (e.key === "Enter") {
										e.preventDefault();
										handleAddSource();
									}
								}}
							/>
							<Button
								type="button"
								variant="outline"
								onClick={handleAddSource}
								disabled={!newSource}
							>
								<Plus className="w-4 h-4" />
							</Button>
						</div>
						{localRule.sources.length > 0 && (
							<div className="flex flex-wrap gap-2 mt-2">
								{localRule.sources.map((source) => (
									<div
										key={source}
										className="group flex items-center gap-1.5 px-3 py-1.5 bg-blue-500/10 border border-blue-500/20 rounded-lg"
									>
										<Box className="w-3.5 h-3.5 text-blue-400" />
										<span className="text-sm text-blue-300 font-mono">
											{source}
										</span>
										<button
											type="button"
											onClick={() => handleRemoveSource(source)}
											className="ml-1 p-0.5 rounded hover:bg-blue-500/20 transition-colors"
										>
											<X className="w-3 h-3 text-blue-400" />
										</button>
									</div>
								))}
							</div>
						)}
						<p className="text-xs text-gray-500">
							Services that emit events (matches EventBridge "source" field)
						</p>
					</div>

					{/* Detail Types */}
					<div className="space-y-2">
						<Label className="flex items-center gap-2">
							<Filter className="w-3.5 h-3.5 text-emerald-400" />
							Event Types
						</Label>
						<div className="flex gap-2">
							<Input
								value={newDetailType}
								onChange={(e) => setNewDetailType(e.target.value)}
								placeholder="e.g., order-created, payment-processed"
								className="font-mono"
								onKeyDown={(e) => {
									if (e.key === "Enter") {
										e.preventDefault();
										handleAddDetailType();
									}
								}}
							/>
							<Button
								type="button"
								variant="outline"
								onClick={handleAddDetailType}
								disabled={!newDetailType}
							>
								<Plus className="w-4 h-4" />
							</Button>
						</div>
						{localRule.detail_types.length > 0 && (
							<div className="flex flex-wrap gap-2 mt-2">
								{localRule.detail_types.map((type) => (
									<div
										key={type}
										className="group flex items-center gap-1.5 px-3 py-1.5 bg-emerald-500/10 border border-emerald-500/20 rounded-lg"
									>
										<Zap className="w-3.5 h-3.5 text-emerald-400" />
										<span className="text-sm text-emerald-300 font-mono">
											{type}
										</span>
										<button
											type="button"
											onClick={() => handleRemoveDetailType(type)}
											className="ml-1 p-0.5 rounded hover:bg-emerald-500/20 transition-colors"
										>
											<X className="w-3 h-3 text-emerald-400" />
										</button>
									</div>
								))}
							</div>
						)}
						<p className="text-xs text-gray-500">
							Specific event types to listen for (matches EventBridge
							"detail-type" field)
						</p>
					</div>

					{/* Live Pattern Preview */}
					{(localRule.sources.length > 0 ||
						localRule.detail_types.length > 0) && (
						<div className="p-3 bg-gray-900 rounded-lg border border-gray-700">
							<div className="flex items-center gap-2 mb-2">
								<span className="text-[10px] font-medium text-gray-500 uppercase tracking-wide">
									EventBridge Pattern
								</span>
							</div>
							<pre className="text-xs text-gray-400 font-mono overflow-x-auto">
								{`{
  "source": [${localRule.sources.map((s) => `"${s}"`).join(", ")}],
  "detail-type": [${localRule.detail_types.map((t) => `"${t}"`).join(", ")}]
}`}
							</pre>
						</div>
					)}
				</div>

				<DialogFooter>
					<Button variant="outline" onClick={() => onOpenChange(false)}>
						Cancel
					</Button>
					<Button onClick={() => onSave(localRule)} disabled={!isValid}>
						{isNew ? "Create Rule" : "Save Changes"}
					</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
}
