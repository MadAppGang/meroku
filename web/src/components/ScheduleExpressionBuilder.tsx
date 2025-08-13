import { Calendar, Clock, X } from "lucide-react";
import React, { useEffect, useState } from "react";
import { Button } from "./ui/button";
import { Input } from "./ui/input";
import { Label } from "./ui/label";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "./ui/select";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "./ui/tabs";

interface ScheduleExpressionBuilderProps {
	value: string;
	onChange: (value: string) => void;
}

// Multi-select component for cron fields
interface MultiSelectProps {
	value: string;
	onChange: (value: string) => void;
	options: { value: string; label: string }[];
	placeholder: string;
}

function MultiSelectField({
	value,
	onChange,
	options,
	placeholder,
}: MultiSelectProps) {
	const [isOpen, setIsOpen] = useState(false);
	const [inputValue, setInputValue] = useState(value);
	const dropdownRef = React.useRef<HTMLDivElement>(null);

	// Parse the current value to determine selected items
	const selectedValues = value === "*" || value === "?" ? [] : value.split(",");

	// Close dropdown when clicking outside
	useEffect(() => {
		const handleClickOutside = (event: MouseEvent) => {
			if (
				dropdownRef.current &&
				!dropdownRef.current.contains(event.target as Node)
			) {
				setIsOpen(false);
			}
		};

		if (isOpen) {
			document.addEventListener("mousedown", handleClickOutside);
			return () => {
				document.removeEventListener("mousedown", handleClickOutside);
			};
		}
	}, [isOpen]);

	const toggleValue = (val: string) => {
		if (val === "*" || val === "?") {
			onChange(val);
			setInputValue(val);
			return;
		}

		const newValues = [...selectedValues];
		const index = newValues.indexOf(val);

		if (index > -1) {
			newValues.splice(index, 1);
		} else {
			newValues.push(val);
		}

		const newValue = newValues.length === 0 ? "*" : newValues.join(",");
		onChange(newValue);
		setInputValue(newValue);
	};

	const handleInputChange = (e: React.ChangeEvent<HTMLInputElement>) => {
		setInputValue(e.target.value);
		onChange(e.target.value);
	};

	return (
		<div className="relative" ref={dropdownRef}>
			<div className="flex gap-2">
				<Input
					value={inputValue}
					onChange={handleInputChange}
					placeholder={placeholder}
					className="bg-gray-800 border-gray-600 flex-1"
				/>
				<Button
					type="button"
					variant="outline"
					size="sm"
					onClick={() => setIsOpen(!isOpen)}
					className="px-2"
				>
					<Calendar className="w-4 h-4" />
				</Button>
			</div>

			{isOpen && (
				<div className="absolute z-10 mt-1 w-full bg-gray-800 border border-gray-600 rounded-md shadow-lg max-h-60 overflow-auto">
					<div className="p-2">
						<Button
							type="button"
							variant="ghost"
							size="sm"
							className="w-full justify-start text-xs"
							onClick={() => {
								toggleValue("*");
								setIsOpen(false);
							}}
						>
							Any/All (*)
						</Button>
						{options.map((option) => (
							<Button
								key={option.value}
								type="button"
								variant="ghost"
								size="sm"
								className={`w-full justify-start text-xs ${
									selectedValues.includes(option.value) ? "bg-blue-900/30" : ""
								}`}
								onClick={() => toggleValue(option.value)}
							>
								{option.label}
								{selectedValues.includes(option.value) && (
									<X className="w-3 h-3 ml-auto" />
								)}
							</Button>
						))}
					</div>
					<div className="border-t border-gray-700 p-2">
						<p className="text-xs text-gray-400 mb-1">Examples:</p>
						<p className="text-xs text-gray-500">• 1,15,30 (specific days)</p>
						<p className="text-xs text-gray-500">• 1-5 (range)</p>
						<p className="text-xs text-gray-500">• MON,WED,FRI</p>
						<p className="text-xs text-gray-500">• MON-FRI</p>
					</div>
				</div>
			)}
		</div>
	);
}

export function ScheduleExpressionBuilder({
	value,
	onChange,
}: ScheduleExpressionBuilderProps) {
	// Parse the initial value to determine type and values
	const isRate = value.startsWith("rate(");
	const isCron = value.startsWith("cron(");

	const [expressionType, setExpressionType] = useState<"rate" | "cron">(
		isRate ? "rate" : "cron",
	);

	// Rate expression state
	const [rateValue, setRateValue] = useState("1");
	const [rateUnit, setRateUnit] = useState<"minute" | "hour" | "day">("day");

	// Cron expression state
	const [cronMinute, setCronMinute] = useState("0");
	const [cronHour, setCronHour] = useState("12");
	const [cronDayOfMonth, setCronDayOfMonth] = useState("*");
	const [cronMonth, setCronMonth] = useState("*");
	const [cronDayOfWeek, setCronDayOfWeek] = useState("?");
	const [cronYear, setCronYear] = useState("*");

	// Common presets
	const [preset, setPreset] = useState<string>("");

	// Options for multi-select fields
	const monthOptions = [
		{ value: "1", label: "January" },
		{ value: "2", label: "February" },
		{ value: "3", label: "March" },
		{ value: "4", label: "April" },
		{ value: "5", label: "May" },
		{ value: "6", label: "June" },
		{ value: "7", label: "July" },
		{ value: "8", label: "August" },
		{ value: "9", label: "September" },
		{ value: "10", label: "October" },
		{ value: "11", label: "November" },
		{ value: "12", label: "December" },
	];

	const dayOfWeekOptions = [
		{ value: "MON", label: "Monday" },
		{ value: "TUE", label: "Tuesday" },
		{ value: "WED", label: "Wednesday" },
		{ value: "THU", label: "Thursday" },
		{ value: "FRI", label: "Friday" },
		{ value: "SAT", label: "Saturday" },
		{ value: "SUN", label: "Sunday" },
	];

	const dayOfMonthOptions = Array.from({ length: 31 }, (_, i) => ({
		value: String(i + 1),
		label: String(i + 1),
	}));

	// Parse initial value
	useEffect(() => {
		if (isRate) {
			const match = value.match(/rate\((\d+)\s+(minute|hour|day)s?\)/);
			if (match) {
				setRateValue(match[1]);
				setRateUnit(match[2] as any);
			}
		} else if (isCron) {
			const match = value.match(
				/cron\((.*?)\s+(.*?)\s+(.*?)\s+(.*?)\s+(.*?)\s+(.*?)\)/,
			);
			if (match) {
				setCronMinute(match[1]);
				setCronHour(match[2]);
				setCronDayOfMonth(match[3]);
				setCronMonth(match[4]);
				setCronDayOfWeek(match[5]);
				setCronYear(match[6]);
			}
		}
	}, [value, isRate, isCron]);

	// Build rate expression
	const buildRateExpression = () => {
		const unit = parseInt(rateValue) === 1 ? rateUnit : `${rateUnit}s`;
		return `rate(${rateValue} ${unit})`;
	};

	// Build cron expression
	const buildCronExpression = () => {
		return `cron(${cronMinute} ${cronHour} ${cronDayOfMonth} ${cronMonth} ${cronDayOfWeek} ${cronYear})`;
	};

	// Update parent when values change
	useEffect(() => {
		if (expressionType === "rate") {
			onChange(buildRateExpression());
		} else {
			onChange(buildCronExpression());
		}
	}, [expressionType, buildCronExpression, buildRateExpression, onChange]);

	// Apply preset
	const applyPreset = (presetValue: string) => {
		setPreset(presetValue);
		switch (presetValue) {
			case "every-5-minutes":
				setExpressionType("rate");
				setRateValue("5");
				setRateUnit("minute");
				break;
			case "every-hour":
				setExpressionType("rate");
				setRateValue("1");
				setRateUnit("hour");
				break;
			case "daily":
				setExpressionType("rate");
				setRateValue("1");
				setRateUnit("day");
				break;
			case "daily-noon":
				setExpressionType("cron");
				setCronMinute("0");
				setCronHour("12");
				setCronDayOfMonth("*");
				setCronMonth("*");
				setCronDayOfWeek("?");
				setCronYear("*");
				break;
			case "weekdays-9am":
				setExpressionType("cron");
				setCronMinute("0");
				setCronHour("9");
				setCronDayOfMonth("?");
				setCronMonth("*");
				setCronDayOfWeek("MON-FRI");
				setCronYear("*");
				break;
			case "first-monday":
				setExpressionType("cron");
				setCronMinute("0");
				setCronHour("9");
				setCronDayOfMonth("?");
				setCronMonth("*");
				setCronDayOfWeek("2#1");
				setCronYear("*");
				break;
		}
	};

	return (
		<div className="space-y-4">
			{/* Presets */}
			<div className="space-y-2">
				<Label>Quick Presets</Label>
				<Select value={preset} onValueChange={applyPreset}>
					<SelectTrigger className="bg-gray-800 border-gray-600">
						<SelectValue placeholder="Select a preset schedule" />
					</SelectTrigger>
					<SelectContent>
						<SelectItem value="every-5-minutes">Every 5 minutes</SelectItem>
						<SelectItem value="every-hour">Every hour</SelectItem>
						<SelectItem value="daily">Daily</SelectItem>
						<SelectItem value="daily-noon">Daily at noon UTC</SelectItem>
						<SelectItem value="weekdays-9am">Weekdays at 9 AM UTC</SelectItem>
						<SelectItem value="first-monday">
							First Monday of month at 9 AM UTC
						</SelectItem>
					</SelectContent>
				</Select>
			</div>

			{/* Expression Type Tabs */}
			<Tabs
				value={expressionType}
				onValueChange={(v) => setExpressionType(v as "rate" | "cron")}
			>
				<TabsList className="grid w-full grid-cols-2">
					<TabsTrigger value="rate" className="flex items-center gap-2">
						<Clock className="w-4 h-4" />
						Rate-based
					</TabsTrigger>
					<TabsTrigger value="cron" className="flex items-center gap-2">
						<Calendar className="w-4 h-4" />
						Cron-based
					</TabsTrigger>
				</TabsList>

				<TabsContent value="rate" className="space-y-4">
					<div className="grid grid-cols-2 gap-4">
						<div className="space-y-2">
							<Label>Value</Label>
							<Input
								type="number"
								min="1"
								value={rateValue}
								onChange={(e) => setRateValue(e.target.value)}
								className="bg-gray-800 border-gray-600"
							/>
						</div>
						<div className="space-y-2">
							<Label>Unit</Label>
							<Select
								value={rateUnit}
								onValueChange={(v) => setRateUnit(v as any)}
							>
								<SelectTrigger className="bg-gray-800 border-gray-600">
									<SelectValue />
								</SelectTrigger>
								<SelectContent>
									<SelectItem value="minute">Minute(s)</SelectItem>
									<SelectItem value="hour">Hour(s)</SelectItem>
									<SelectItem value="day">Day(s)</SelectItem>
								</SelectContent>
							</Select>
						</div>
					</div>
					<div className="text-xs text-gray-400">
						Runs every {rateValue} {rateUnit}
						{parseInt(rateValue) !== 1 ? "s" : ""}
					</div>
				</TabsContent>

				<TabsContent value="cron" className="space-y-4">
					<div className="grid grid-cols-2 gap-4">
						<div className="space-y-2">
							<Label>Minute (0-59)</Label>
							<Input
								value={cronMinute}
								onChange={(e) => setCronMinute(e.target.value)}
								placeholder="0"
								className="bg-gray-800 border-gray-600"
							/>
						</div>
						<div className="space-y-2">
							<Label>Hour (0-23)</Label>
							<Input
								value={cronHour}
								onChange={(e) => setCronHour(e.target.value)}
								placeholder="12"
								className="bg-gray-800 border-gray-600"
							/>
						</div>
						<div className="space-y-2">
							<Label>Day of Month</Label>
							<MultiSelectField
								value={cronDayOfMonth}
								onChange={setCronDayOfMonth}
								options={dayOfMonthOptions}
								placeholder="* or 1,15,30 or 1-5"
							/>
						</div>
						<div className="space-y-2">
							<Label>Month</Label>
							<MultiSelectField
								value={cronMonth}
								onChange={setCronMonth}
								options={monthOptions}
								placeholder="* or 1,6,12 or 3-5"
							/>
						</div>
						<div className="space-y-2">
							<Label>Day of Week</Label>
							<MultiSelectField
								value={cronDayOfWeek}
								onChange={setCronDayOfWeek}
								options={[
									{ value: "?", label: "Any day (?)" },
									...dayOfWeekOptions,
								]}
								placeholder="? or MON,WED,FRI or MON-FRI"
							/>
						</div>
						<div className="space-y-2">
							<Label>Year</Label>
							<Input
								value={cronYear}
								onChange={(e) => setCronYear(e.target.value)}
								placeholder="* or 2024-2030"
								className="bg-gray-800 border-gray-600"
							/>
						</div>
					</div>

					<div className="space-y-2">
						<p className="text-xs text-gray-400">
							<strong>Special characters:</strong>
						</p>
						<ul className="text-xs text-gray-500 space-y-1">
							<li>
								• <code>*</code> - Any value
							</li>
							<li>
								• <code>?</code> - No specific value (day of month/week)
							</li>
							<li>
								• <code>-</code> - Range (e.g., MON-FRI)
							</li>
							<li>
								• <code>,</code> - List (e.g., MON,WED,FRI)
							</li>
							<li>
								• <code>#</code> - Nth occurrence (e.g., 2#1 = first Monday)
							</li>
						</ul>
					</div>
				</TabsContent>
			</Tabs>

			{/* Generated Expression */}
			<div className="p-3 bg-gray-800 rounded-lg">
				<p className="text-xs text-gray-400 mb-1">Generated expression:</p>
				<code className="text-sm text-blue-300 font-mono">
					{expressionType === "rate"
						? buildRateExpression()
						: buildCronExpression()}
				</code>
			</div>
		</div>
	);
}
