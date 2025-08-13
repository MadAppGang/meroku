import { CheckCircle, Clock, Info, Send, XCircle, Zap } from "lucide-react";
import { useState } from "react";
import {
	infrastructureApi,
	type TestEventRequest,
	type TestEventResponse,
} from "../api/infrastructure";
import type { YamlInfrastructureConfig } from "../types/yamlConfig";
import { Alert, AlertDescription } from "./ui/alert";
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
import { Textarea } from "./ui/textarea";

interface EventBridgeTestEventProps {
	config: YamlInfrastructureConfig;
}

export function EventBridgeTestEvent({ config }: EventBridgeTestEventProps) {
	// Test event state - default source to 'meroku.test'
	const [testEvent, setTestEvent] = useState<TestEventRequest>({
		source: "meroku.test",
		detailType: "Test Event",
		detail: {},
	});
	const [detailJson, setDetailJson] = useState(
		'{\n  "test": true,\n  "timestamp": "' +
			new Date().toISOString() +
			'",\n  "message": "EventBridge test event"\n}',
	);
	const [sendingEvent, setSendingEvent] = useState(false);
	const [eventResponse, setEventResponse] = useState<TestEventResponse | null>(
		null,
	);
	const [jsonError, setJsonError] = useState<string | null>(null);

	const handleDetailJsonChange = (value: string) => {
		setDetailJson(value);
		try {
			const parsed = JSON.parse(value);
			setTestEvent((prev) => ({ ...prev, detail: parsed }));
			setJsonError(null);
		} catch (_e) {
			setJsonError("Invalid JSON format");
		}
	};

	const handleSendTestEvent = async () => {
		if (!testEvent.source || !testEvent.detailType) {
			setEventResponse({
				success: false,
				message: "Source and Detail Type are required",
			});
			return;
		}

		setSendingEvent(true);
		setEventResponse(null);

		try {
			const response = await infrastructureApi.sendTestEvent(testEvent);
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

	// Get all configured event processors for reference
	const configuredEventTasks = config.event_processor_tasks || [];
	const allConfiguredSources = [
		...new Set(configuredEventTasks.flatMap((task) => task.sources || [])),
	];
	const allConfiguredDetailTypes = [
		...new Set(configuredEventTasks.flatMap((task) => task.detail_types || [])),
	];

	return (
		<div className="space-y-4">
			{/* Info Alert */}
			<Alert>
				<Info className="h-4 w-4" />
				<AlertDescription>
					Send test events to EventBridge to test your event-driven
					infrastructure. Events will be published to your custom event bus and
					can trigger any configured event processor tasks.
				</AlertDescription>
			</Alert>

			{/* Test Event Configuration */}
			<Card>
				<CardHeader>
					<CardTitle className="flex items-center gap-2">
						<Send className="w-5 h-5" />
						Test Event Configuration
					</CardTitle>
					<CardDescription>
						Configure and send a test event to EventBridge
					</CardDescription>
				</CardHeader>
				<CardContent className="space-y-4">
					<div className="space-y-2">
						<Label htmlFor="test-source">Event Source</Label>
						<Input
							id="test-source"
							value={testEvent.source}
							onChange={(e) =>
								setTestEvent((prev) => ({ ...prev, source: e.target.value }))
							}
							placeholder="e.g., meroku.test"
							className="font-mono"
						/>
						{allConfiguredSources.length > 0 && (
							<div className="space-y-1">
								<p className="text-xs text-gray-500">
									Event processor sources: {allConfiguredSources.join(", ")}
								</p>
								{!allConfiguredSources.includes(testEvent.source) &&
									testEvent.source && (
										<p className="text-xs text-yellow-500 flex items-center gap-1">
											<Info className="w-3 h-3" />
											This source doesn't match any configured event processors
										</p>
									)}
							</div>
						)}
					</div>

					<div className="space-y-2">
						<Label htmlFor="test-detail-type">Detail Type</Label>
						<Input
							id="test-detail-type"
							value={testEvent.detailType}
							onChange={(e) =>
								setTestEvent((prev) => ({
									...prev,
									detailType: e.target.value,
								}))
							}
							placeholder="e.g., Test Event"
						/>
						{allConfiguredDetailTypes.length > 0 && (
							<div className="space-y-1">
								<p className="text-xs text-gray-500">
									Event processor detail types:{" "}
									{allConfiguredDetailTypes.join(", ")}
								</p>
								{!allConfiguredDetailTypes.includes(testEvent.detailType) &&
									testEvent.detailType && (
										<p className="text-xs text-yellow-500 flex items-center gap-1">
											<Info className="w-3 h-3" />
											This detail type doesn't match any configured event
											processors
										</p>
									)}
							</div>
						)}
					</div>

					<div className="space-y-2">
						<Label htmlFor="test-detail">Event Detail (JSON)</Label>
						<Textarea
							id="test-detail"
							value={detailJson}
							onChange={(e) => handleDetailJsonChange(e.target.value)}
							placeholder='{"test": true, "message": "EventBridge test event"}'
							className="font-mono text-sm min-h-[200px]"
							rows={10}
						/>
						{jsonError && <p className="text-xs text-red-400">{jsonError}</p>}
					</div>

					<Button
						onClick={handleSendTestEvent}
						disabled={
							sendingEvent ||
							!!jsonError ||
							!testEvent.source ||
							!testEvent.detailType
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
								Send Test Event to EventBridge
							</>
						)}
					</Button>

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
				</CardContent>
			</Card>

			{/* Event Processors Info */}
			{configuredEventTasks.length > 0 && (
				<Card>
					<CardHeader>
						<CardTitle className="flex items-center gap-2">
							<Zap className="w-5 h-5" />
							Configured Event Processors
						</CardTitle>
						<CardDescription>
							Event processor tasks that can be triggered by events
						</CardDescription>
					</CardHeader>
					<CardContent>
						<div className="space-y-3">
							{configuredEventTasks.map((task, _index) => (
								<div key={task.name} className="p-3 bg-gray-800 rounded-lg">
									<h4 className="text-sm font-medium text-gray-200 mb-2">
										{task.name}
									</h4>
									<div className="space-y-1 text-xs text-gray-400">
										<div>
											<span className="text-blue-400">Sources:</span>{" "}
											{task.sources?.join(", ") || "None"}
										</div>
										<div>
											<span className="text-blue-400">Detail Types:</span>{" "}
											{task.detail_types?.join(", ") || "None"}
										</div>
										<div>
											<span className="text-blue-400">Rule:</span>{" "}
											{task.rule_name}
										</div>
									</div>
								</div>
							))}
						</div>
					</CardContent>
				</Card>
			)}

			{/* Example Events */}
			<Card>
				<CardHeader>
					<CardTitle className="flex items-center gap-2">
						<Zap className="w-5 h-5" />
						Example Test Events
					</CardTitle>
					<CardDescription>
						Common test event patterns for different scenarios
					</CardDescription>
				</CardHeader>
				<CardContent className="space-y-4">
					<div className="space-y-3">
						<div className="p-3 bg-gray-800 rounded-lg">
							<h4 className="text-sm font-medium text-gray-300 mb-2">
								Basic Test Event
							</h4>
							<pre className="text-xs text-gray-400 overflow-x-auto">{`{
  "test": true,
  "timestamp": "${new Date().toISOString()}",
  "message": "This is a test event from EventBridge"
}`}</pre>
						</div>

						<div className="p-3 bg-gray-800 rounded-lg">
							<h4 className="text-sm font-medium text-gray-300 mb-2">
								Application Event
							</h4>
							<pre className="text-xs text-gray-400 overflow-x-auto">{`{
  "eventType": "user.created",
  "userId": "user-123",
  "email": "user@example.com",
  "timestamp": "${new Date().toISOString()}",
  "source": "user-service"
}`}</pre>
						</div>

						<div className="p-3 bg-gray-800 rounded-lg">
							<h4 className="text-sm font-medium text-gray-300 mb-2">
								System Event
							</h4>
							<pre className="text-xs text-gray-400 overflow-x-auto">{`{
  "eventType": "deployment.completed",
  "serviceName": "backend",
  "version": "v1.2.3",
  "environment": "${config.env}",
  "status": "success",
  "timestamp": "${new Date().toISOString()}"
}`}</pre>
						</div>

						<div className="p-3 bg-gray-800 rounded-lg">
							<h4 className="text-sm font-medium text-gray-300 mb-2">
								Business Event
							</h4>
							<pre className="text-xs text-gray-400 overflow-x-auto">{`{
  "eventType": "order.processed",
  "orderId": "ORD-123456",
  "customerId": "CUST-789",
  "amount": 99.99,
  "currency": "USD",
  "status": "completed",
  "timestamp": "${new Date().toISOString()}"
}`}</pre>
						</div>
					</div>
				</CardContent>
			</Card>

			{/* Testing Tips */}
			<Card>
				<CardHeader>
					<CardTitle>Testing Tips</CardTitle>
					<CardDescription>
						Best practices for testing EventBridge
					</CardDescription>
				</CardHeader>
				<CardContent>
					<ul className="text-sm text-gray-300 space-y-2">
						<li className="flex items-start gap-2">
							<span className="text-blue-400">•</span>
							<span>
								Use <code className="text-blue-400 text-xs">meroku.test</code>{" "}
								as the source for test events to easily identify them
							</span>
						</li>
						<li className="flex items-start gap-2">
							<span className="text-blue-400">•</span>
							<span>
								Include a{" "}
								<code className="text-blue-400 text-xs">test: true</code> field
								in your event detail for easy filtering
							</span>
						</li>
						<li className="flex items-start gap-2">
							<span className="text-blue-400">•</span>
							<span>
								Always include timestamps in your test events for debugging
							</span>
						</li>
						<li className="flex items-start gap-2">
							<span className="text-blue-400">•</span>
							<span>
								Check CloudWatch Logs for event processor tasks to see if they
								were triggered
							</span>
						</li>
						<li className="flex items-start gap-2">
							<span className="text-blue-400">•</span>
							<span>
								Events are published to your custom event bus:{" "}
								<code className="text-blue-400 text-xs">
									{config.project}_{config.env}_eventbus
								</code>
							</span>
						</li>
						<li className="flex items-start gap-2">
							<span className="text-blue-400">•</span>
							<span>
								Use EventBridge console to monitor event delivery and rule
								matches
							</span>
						</li>
					</ul>
				</CardContent>
			</Card>
		</div>
	);
}
