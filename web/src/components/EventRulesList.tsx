import { Edit2, Plus, Trash2, X, Zap } from "lucide-react";
import { useState } from "react";
import type { EventBridgeRule } from "../types/yamlConfig";
import { Badge } from "./ui/badge";
import { Button } from "./ui/button";
import {
	Card,
	CardContent,
	CardDescription,
	CardHeader,
	CardTitle,
} from "./ui/card";
import { Input } from "./ui/input";
import { Label } from "./ui/label";

interface EventRulesListProps {
	rules: EventBridgeRule[];
	onRulesChange: (rules: EventBridgeRule[]) => void;
}

export function EventRulesList({ rules, onRulesChange }: EventRulesListProps) {
	const [editingRuleIndex, setEditingRuleIndex] = useState<number | null>(null);
	const [editingRule, setEditingRule] = useState<EventBridgeRule | null>(null);
	const [newSource, setNewSource] = useState("");
	const [newDetailType, setNewDetailType] = useState("");
	const [isAddingNew, setIsAddingNew] = useState(false);

	const handleAddRule = () => {
		setIsAddingNew(true);
		setEditingRule({ name: "", sources: [], detail_types: [] });
		setEditingRuleIndex(null);
	};

	const handleEditRule = (index: number) => {
		setEditingRuleIndex(index);
		setEditingRule({ ...rules[index] });
		setIsAddingNew(false);
	};

	const handleDeleteRule = (index: number) => {
		const updated = rules.filter((_, i) => i !== index);
		onRulesChange(updated);
	};

	const handleSaveRule = () => {
		if (!editingRule || !editingRule.name) return;

		if (isAddingNew) {
			onRulesChange([...rules, editingRule]);
		} else if (editingRuleIndex !== null) {
			const updated = [...rules];
			updated[editingRuleIndex] = editingRule;
			onRulesChange(updated);
		}

		setEditingRule(null);
		setEditingRuleIndex(null);
		setIsAddingNew(false);
		setNewSource("");
		setNewDetailType("");
	};

	const handleCancelEdit = () => {
		setEditingRule(null);
		setEditingRuleIndex(null);
		setIsAddingNew(false);
		setNewSource("");
		setNewDetailType("");
	};

	const handleAddSource = () => {
		if (newSource && editingRule && !editingRule.sources.includes(newSource)) {
			setEditingRule({
				...editingRule,
				sources: [...editingRule.sources, newSource],
			});
			setNewSource("");
		}
	};

	const handleRemoveSource = (source: string) => {
		if (editingRule) {
			setEditingRule({
				...editingRule,
				sources: editingRule.sources.filter((s) => s !== source),
			});
		}
	};

	const handleAddDetailType = () => {
		if (
			newDetailType &&
			editingRule &&
			!editingRule.detail_types.includes(newDetailType)
		) {
			setEditingRule({
				...editingRule,
				detail_types: [...editingRule.detail_types, newDetailType],
			});
			setNewDetailType("");
		}
	};

	const handleRemoveDetailType = (type: string) => {
		if (editingRule) {
			setEditingRule({
				...editingRule,
				detail_types: editingRule.detail_types.filter((t) => t !== type),
			});
		}
	};

	return (
		<Card>
			<CardHeader>
				<div className="flex items-center justify-between">
					<div>
						<CardTitle className="flex items-center gap-2">
							<Zap className="w-5 h-5" />
							EventBridge Rules
						</CardTitle>
						<CardDescription>
							Define which events trigger this task
						</CardDescription>
					</div>
					<Button
						size="sm"
						onClick={handleAddRule}
						disabled={isAddingNew || editingRuleIndex !== null}
					>
						<Plus className="w-4 h-4 mr-1" />
						Add Rule
					</Button>
				</div>
			</CardHeader>
			<CardContent className="space-y-4">
				{/* Rules List */}
				{rules.length === 0 && !isAddingNew && (
					<div className="text-center py-6 text-gray-500 border border-dashed border-gray-700 rounded-lg">
						<Zap className="w-8 h-8 mx-auto mb-2 opacity-50" />
						<p>No rules configured</p>
						<p className="text-xs mt-1">Add a rule to start listening for events</p>
					</div>
				)}

				{rules.map((rule, index) => (
					<div
						key={rule.name}
						className={`border rounded-lg p-3 ${
							editingRuleIndex === index
								? "border-blue-500 bg-blue-950/20"
								: "border-gray-700"
						}`}
					>
						{editingRuleIndex === index ? (
							/* Edit Mode */
							<RuleEditor
								rule={editingRule!}
								onRuleChange={setEditingRule}
								newSource={newSource}
								onNewSourceChange={setNewSource}
								onAddSource={handleAddSource}
								onRemoveSource={handleRemoveSource}
								newDetailType={newDetailType}
								onNewDetailTypeChange={setNewDetailType}
								onAddDetailType={handleAddDetailType}
								onRemoveDetailType={handleRemoveDetailType}
								onSave={handleSaveRule}
								onCancel={handleCancelEdit}
							/>
						) : (
							/* View Mode */
							<div className="flex items-start justify-between">
								<div className="space-y-2 flex-1">
									<div className="flex items-center gap-2">
										<span className="font-medium text-sm">{rule.name}</span>
									</div>
									<div className="flex flex-wrap gap-1">
										{rule.sources.map((source) => (
											<Badge key={source} variant="outline" className="text-xs">
												{source}
											</Badge>
										))}
										<span className="text-gray-500 mx-1">→</span>
										{rule.detail_types.map((type) => (
											<Badge key={type} variant="secondary" className="text-xs">
												{type}
											</Badge>
										))}
									</div>
								</div>
								<div className="flex items-center gap-1">
									<Button
										size="sm"
										variant="ghost"
										onClick={() => handleEditRule(index)}
									>
										<Edit2 className="w-3 h-3" />
									</Button>
									<Button
										size="sm"
										variant="ghost"
										className="text-red-400 hover:text-red-300"
										onClick={() => handleDeleteRule(index)}
									>
										<Trash2 className="w-3 h-3" />
									</Button>
								</div>
							</div>
						)}
					</div>
				))}

				{/* New Rule Form */}
				{isAddingNew && editingRule && (
					<div className="border border-blue-500 rounded-lg p-3 bg-blue-950/20">
						<RuleEditor
							rule={editingRule}
							onRuleChange={setEditingRule}
							newSource={newSource}
							onNewSourceChange={setNewSource}
							onAddSource={handleAddSource}
							onRemoveSource={handleRemoveSource}
							newDetailType={newDetailType}
							onNewDetailTypeChange={setNewDetailType}
							onAddDetailType={handleAddDetailType}
							onRemoveDetailType={handleRemoveDetailType}
							onSave={handleSaveRule}
							onCancel={handleCancelEdit}
							isNew
						/>
					</div>
				)}
			</CardContent>
		</Card>
	);
}

interface RuleEditorProps {
	rule: EventBridgeRule;
	onRuleChange: (rule: EventBridgeRule) => void;
	newSource: string;
	onNewSourceChange: (value: string) => void;
	onAddSource: () => void;
	onRemoveSource: (source: string) => void;
	newDetailType: string;
	onNewDetailTypeChange: (value: string) => void;
	onAddDetailType: () => void;
	onRemoveDetailType: (type: string) => void;
	onSave: () => void;
	onCancel: () => void;
	isNew?: boolean;
}

function RuleEditor({
	rule,
	onRuleChange,
	newSource,
	onNewSourceChange,
	onAddSource,
	onRemoveSource,
	newDetailType,
	onNewDetailTypeChange,
	onAddDetailType,
	onRemoveDetailType,
	onSave,
	onCancel,
	isNew,
}: RuleEditorProps) {
	return (
		<div className="space-y-3">
			<div className="space-y-1">
				<Label className="text-xs">Rule Name</Label>
				<Input
					value={rule.name}
					onChange={(e) => onRuleChange({ ...rule, name: e.target.value })}
					placeholder="e.g., order-events"
					className="font-mono text-sm h-8"
				/>
			</div>

			<div className="space-y-1">
				<Label className="text-xs">Event Sources</Label>
				<div className="flex items-center gap-2">
					<Input
						value={newSource}
						onChange={(e) => onNewSourceChange(e.target.value)}
						placeholder="e.g., backend"
						className="text-sm h-8"
						onKeyPress={(e) => e.key === "Enter" && onAddSource()}
					/>
					<Button
						size="sm"
						variant="outline"
						onClick={onAddSource}
						disabled={!newSource}
						className="h-8"
					>
						<Plus className="w-3 h-3" />
					</Button>
				</div>
				<div className="flex flex-wrap gap-1 mt-1">
					{rule.sources.map((source) => (
						<Badge
							key={source}
							variant="outline"
							className="text-xs pr-1"
						>
							{source}
							<button
								type="button"
								onClick={() => onRemoveSource(source)}
								className="ml-1 hover:bg-gray-600 rounded-sm"
							>
								<X className="w-3 h-3" />
							</button>
						</Badge>
					))}
				</div>
			</div>

			<div className="space-y-1">
				<Label className="text-xs">Detail Types</Label>
				<div className="flex items-center gap-2">
					<Input
						value={newDetailType}
						onChange={(e) => onNewDetailTypeChange(e.target.value)}
						placeholder="e.g., order-created"
						className="text-sm h-8"
						onKeyPress={(e) => e.key === "Enter" && onAddDetailType()}
					/>
					<Button
						size="sm"
						variant="outline"
						onClick={onAddDetailType}
						disabled={!newDetailType}
						className="h-8"
					>
						<Plus className="w-3 h-3" />
					</Button>
				</div>
				<div className="flex flex-wrap gap-1 mt-1">
					{rule.detail_types.map((type) => (
						<Badge
							key={type}
							variant="secondary"
							className="text-xs pr-1"
						>
							{type}
							<button
								type="button"
								onClick={() => onRemoveDetailType(type)}
								className="ml-1 hover:bg-gray-600 rounded-sm"
							>
								<X className="w-3 h-3" />
							</button>
						</Badge>
					))}
				</div>
			</div>

			<div className="flex justify-end gap-2 pt-2 border-t border-gray-700">
				<Button size="sm" variant="ghost" onClick={onCancel}>
					Cancel
				</Button>
				<Button
					size="sm"
					onClick={onSave}
					disabled={!rule.name || rule.sources.length === 0 || rule.detail_types.length === 0}
				>
					{isNew ? "Add Rule" : "Save"}
				</Button>
			</div>
		</div>
	);
}
