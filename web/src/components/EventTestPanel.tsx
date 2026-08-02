import { faker } from "@faker-js/faker";
import { CheckCircle, Clock, Send, Sparkles, XCircle } from "lucide-react";
import { useEffect, useState } from "react";
import {
	infrastructureApi,
	type TestEventRequest,
	type TestEventResponse,
} from "../api/infrastructure";
import type { EventBridgeRule, EventProcessorTask } from "../types/yamlConfig";
import { Alert, AlertDescription } from "./ui/alert";
import { Badge } from "./ui/badge";
import { Button } from "./ui/button";
import {
	Card,
	CardContent,
	CardDescription,
	CardHeader,
	CardTitle,
} from "./ui/card";
import { Label } from "./ui/label";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "./ui/select";
import { Textarea } from "./ui/textarea";

interface EventTestPanelProps {
	eventTask: EventProcessorTask;
}

// Generate realistic fake data based on event type
function generateFakeEvent(detailType: string): object {
	const lowerType = detailType.toLowerCase();

	// Order-related events
	if (lowerType.includes("order") || lowerType.includes("purchase")) {
		return {
			orderId: faker.string.uuid(),
			customerId: faker.string.uuid(),
			timestamp: new Date().toISOString(),
			status: faker.helpers.arrayElement([
				"pending",
				"confirmed",
				"shipped",
				"delivered",
			]),
			totalAmount: Number(faker.commerce.price({ min: 10, max: 500 })),
			currency: "USD",
			items: Array.from(
				{ length: faker.number.int({ min: 1, max: 3 }) },
				() => ({
					productId: faker.string.uuid(),
					name: faker.commerce.productName(),
					quantity: faker.number.int({ min: 1, max: 5 }),
					price: Number(faker.commerce.price({ min: 5, max: 100 })),
				}),
			),
			shippingAddress: {
				street: faker.location.streetAddress(),
				city: faker.location.city(),
				country: faker.location.country(),
				zipCode: faker.location.zipCode(),
			},
		};
	}

	// User-related events
	if (
		lowerType.includes("user") ||
		lowerType.includes("signup") ||
		lowerType.includes("login")
	) {
		return {
			userId: faker.string.uuid(),
			email: faker.internet.email(),
			timestamp: new Date().toISOString(),
			action: lowerType.includes("login")
				? "login"
				: lowerType.includes("signup")
					? "signup"
					: "update",
			profile: {
				firstName: faker.person.firstName(),
				lastName: faker.person.lastName(),
				phone: faker.phone.number(),
			},
			metadata: {
				ipAddress: faker.internet.ip(),
				userAgent: faker.internet.userAgent(),
				location: faker.location.city(),
			},
		};
	}

	// Payment-related events
	if (lowerType.includes("payment") || lowerType.includes("transaction")) {
		return {
			transactionId: faker.string.uuid(),
			orderId: faker.string.uuid(),
			timestamp: new Date().toISOString(),
			amount: Number(faker.finance.amount({ min: 10, max: 1000 })),
			currency: faker.finance.currencyCode(),
			status: faker.helpers.arrayElement([
				"pending",
				"completed",
				"failed",
				"refunded",
			]),
			paymentMethod: faker.helpers.arrayElement([
				"credit_card",
				"debit_card",
				"paypal",
				"bank_transfer",
			]),
			last4: faker.finance.creditCardNumber().slice(-4),
		};
	}

	// Notification-related events
	if (
		lowerType.includes("notification") ||
		lowerType.includes("email") ||
		lowerType.includes("sms")
	) {
		return {
			notificationId: faker.string.uuid(),
			recipientId: faker.string.uuid(),
			timestamp: new Date().toISOString(),
			type: faker.helpers.arrayElement(["email", "sms", "push"]),
			subject: faker.lorem.sentence(),
			content: faker.lorem.paragraph(),
			priority: faker.helpers.arrayElement(["low", "normal", "high"]),
		};
	}

	// Inventory-related events
	if (lowerType.includes("inventory") || lowerType.includes("stock")) {
		return {
			productId: faker.string.uuid(),
			sku: faker.string.alphanumeric(8).toUpperCase(),
			timestamp: new Date().toISOString(),
			action: faker.helpers.arrayElement([
				"restock",
				"sold",
				"reserved",
				"released",
			]),
			quantity: faker.number.int({ min: 1, max: 100 }),
			previousStock: faker.number.int({ min: 0, max: 500 }),
			newStock: faker.number.int({ min: 0, max: 500 }),
			warehouseId: faker.string.uuid(),
		};
	}

	// Job/task-related events
	if (
		lowerType.includes("job") ||
		lowerType.includes("task") ||
		lowerType.includes("process")
	) {
		return {
			jobId: faker.string.uuid(),
			timestamp: new Date().toISOString(),
			type: faker.helpers.arrayElement([
				"sync",
				"export",
				"import",
				"cleanup",
				"report",
			]),
			status: faker.helpers.arrayElement([
				"queued",
				"running",
				"completed",
				"failed",
			]),
			progress: faker.number.int({ min: 0, max: 100 }),
			metadata: {
				startedBy: faker.internet.email(),
				priority: faker.helpers.arrayElement(["low", "normal", "high"]),
				retryCount: faker.number.int({ min: 0, max: 3 }),
			},
		};
	}

	// Default generic event
	return {
		eventId: faker.string.uuid(),
		timestamp: new Date().toISOString(),
		data: {
			id: faker.string.uuid(),
			name: faker.lorem.words(3),
			value: faker.number.int({ min: 1, max: 100 }),
			metadata: {
				source: "test",
				version: "1.0",
			},
		},
	};
}

// Get all rules (either from rules[] array or legacy fields)
function getAllRules(task: EventProcessorTask): EventBridgeRule[] {
	if (task.rules && task.rules.length > 0) {
		return task.rules;
	}
	// Legacy format
	if (task.rule_name && task.sources && task.detail_types) {
		return [
			{
				name: task.rule_name,
				sources: task.sources,
				detail_types: task.detail_types,
			},
		];
	}
	return [];
}

export function EventTestPanel({ eventTask }: EventTestPanelProps) {
	const rules = getAllRules(eventTask);

	// Find first available source and detail type
	const defaultSource = rules[0]?.sources[0] || "meroku.test";
	const defaultDetailType = rules[0]?.detail_types[0] || "";

	const [selectedSource, setSelectedSource] = useState(defaultSource);
	const [selectedDetailType, setSelectedDetailType] =
		useState(defaultDetailType);
	const [detailJson, setDetailJson] = useState("");
	const [sendingEvent, setSendingEvent] = useState(false);
	const [eventResponse, setEventResponse] = useState<TestEventResponse | null>(
		null,
	);
	const [jsonError, setJsonError] = useState<string | null>(null);

	// Get all unique sources and detail types
	const allSources = [...new Set(rules.flatMap((r) => r.sources))];
	const allDetailTypes = [...new Set(rules.flatMap((r) => r.detail_types))];

	// Check if current selection matches any rule
	const matchesRule = rules.some(
		(rule) =>
			rule.sources.includes(selectedSource) &&
			rule.detail_types.includes(selectedDetailType),
	);

	// Generate fake data when detail type changes
	useEffect(() => {
		if (selectedDetailType) {
			const fakeData = generateFakeEvent(selectedDetailType);
			setDetailJson(JSON.stringify(fakeData, null, 2));
		}
	}, [selectedDetailType]);

	const handleRegenerateFake = () => {
		if (selectedDetailType) {
			const fakeData = generateFakeEvent(selectedDetailType);
			setDetailJson(JSON.stringify(fakeData, null, 2));
		}
	};

	const handleDetailJsonChange = (value: string) => {
		setDetailJson(value);
		try {
			JSON.parse(value);
			setJsonError(null);
		} catch (_e) {
			setJsonError("Invalid JSON format");
		}
	};

	const handleSendTestEvent = async () => {
		if (!selectedSource || !selectedDetailType) {
			setEventResponse({
				success: false,
				message: "Source and Detail Type are required",
			});
			return;
		}

		let detail: Record<string, unknown>;
		try {
			detail = JSON.parse(detailJson) as Record<string, unknown>;
		} catch (_e) {
			setEventResponse({
				success: false,
				message: "Invalid JSON in event detail",
			});
			return;
		}

		setSendingEvent(true);
		setEventResponse(null);

		const request: TestEventRequest = {
			source: selectedSource,
			detailType: selectedDetailType,
			detail,
		};

		try {
			const response = await infrastructureApi.sendTestEvent(request);
			setEventResponse(response);
		} catch (error) {
			setEventResponse({
				success: false,
				message:
					error instanceof Error ? error.message : "Failed to send test event",
			});
		} finally {
			setSendingEvent(false);
		}
	};

	return (
		<Card>
			<CardHeader>
				<CardTitle className="flex items-center gap-2">
					<Send className="w-5 h-5" />
					Test Event
				</CardTitle>
				<CardDescription>
					Send a test event to trigger this task
				</CardDescription>
			</CardHeader>
			<CardContent className="space-y-4">
				{/* Source Selection */}
				<div className="space-y-2">
					<Label>Event Source</Label>
					<Select value={selectedSource} onValueChange={setSelectedSource}>
						<SelectTrigger>
							<SelectValue placeholder="Select source" />
						</SelectTrigger>
						<SelectContent>
							{allSources.map((source) => (
								<SelectItem key={source} value={source}>
									{source}
								</SelectItem>
							))}
							<SelectItem value="meroku.test">meroku.test (testing)</SelectItem>
						</SelectContent>
					</Select>
				</div>

				{/* Detail Type Selection */}
				<div className="space-y-2">
					<Label>Detail Type</Label>
					<Select
						value={selectedDetailType}
						onValueChange={setSelectedDetailType}
					>
						<SelectTrigger>
							<SelectValue placeholder="Select detail type" />
						</SelectTrigger>
						<SelectContent>
							{allDetailTypes.map((type) => (
								<SelectItem key={type} value={type}>
									{type}
								</SelectItem>
							))}
						</SelectContent>
					</Select>
				</div>

				{/* Match Warning */}
				{selectedSource && selectedDetailType && !matchesRule && (
					<Alert className="border-yellow-600">
						<AlertDescription className="text-yellow-400 text-sm">
							This source/detail-type combination doesn't match any configured
							rule. The event won't trigger this task.
						</AlertDescription>
					</Alert>
				)}

				{/* Event Detail JSON */}
				<div className="space-y-2">
					<div className="flex items-center justify-between">
						<Label>Event Detail (JSON)</Label>
						<Button
							size="sm"
							variant="ghost"
							onClick={handleRegenerateFake}
							className="h-6 text-xs"
						>
							<Sparkles className="w-3 h-3 mr-1" />
							Regenerate
						</Button>
					</div>
					<Textarea
						value={detailJson}
						onChange={(e) => handleDetailJsonChange(e.target.value)}
						className="font-mono text-xs min-h-[160px]"
						placeholder="{}"
					/>
					{jsonError && <p className="text-xs text-red-400">{jsonError}</p>}
				</div>

				{/* Send Button */}
				<Button
					onClick={handleSendTestEvent}
					disabled={
						sendingEvent ||
						!!jsonError ||
						!selectedSource ||
						!selectedDetailType
					}
					className="w-full"
				>
					{sendingEvent ? (
						<>
							<Clock className="w-4 h-4 mr-2 animate-spin" />
							Sending...
						</>
					) : (
						<>
							<Send className="w-4 h-4 mr-2" />
							Send Test Event
						</>
					)}
				</Button>

				{/* Response */}
				{eventResponse && (
					<Alert
						className={
							eventResponse.success ? "border-green-600" : "border-red-600"
						}
					>
						{eventResponse.success ? (
							<CheckCircle className="h-4 w-4 text-green-600" />
						) : (
							<XCircle className="h-4 w-4 text-red-600" />
						)}
						<AlertDescription>
							{eventResponse.message}
							{eventResponse.eventId && (
								<div className="text-xs mt-1 font-mono">
									Event ID: {eventResponse.eventId}
								</div>
							)}
						</AlertDescription>
					</Alert>
				)}

				{/* Configured Rules Summary */}
				<div className="pt-2 border-t border-gray-700">
					<p className="text-xs text-gray-500 mb-2">Configured Rules:</p>
					<div className="space-y-1">
						{rules.map((rule) => (
							<div key={rule.name} className="flex items-center gap-2 text-xs">
								<Badge variant="outline" className="text-xs">
									{rule.name}
								</Badge>
								<span className="text-gray-600">
									{rule.sources.join(", ")} → {rule.detail_types.join(", ")}
								</span>
							</div>
						))}
					</div>
				</div>
			</CardContent>
		</Card>
	);
}
