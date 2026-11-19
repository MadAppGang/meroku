import { CheckCircle, Clock, Info, Send, XCircle, Zap } from "lucide-react";
import { useId, useState } from "react";
import {
	infrastructureApi,
	type TestEventRequest,
	type TestEventResponse,
} from "../api/infrastructure";
import type {
	EventBusChoice,
	YamlInfrastructureConfig,
} from "../types/yamlConfig";
import {
	eventGroupIdError,
	isEnhancedBus,
	isEventBusEnabled,
	projectBusName,
} from "../utils/workloadEvents";
import { Alert, AlertDescription } from "./ui/alert";
import { Button } from "./ui/button";
import {
	Card,
	CardContent,
	CardDescription,
	CardHeader,
	CardTitle,
} from "./ui/card";
import { CodeBlock } from "./ui/code-block";
import { Input } from "./ui/input";
import { Label } from "./ui/label";
import { KeyValue, KeyValueList } from "./ui/recipes/key-value-list";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "./ui/select";
import { NoteCode, NoteDetail, StatusNote } from "./ui/status-note";
import { Textarea } from "./ui/textarea";

/** Example event details, stamped with the current time. */
function exampleEvents(env: string): { title: string; detail: object }[] {
	const timestamp = new Date().toISOString();
	return [
		{
			title: "Basic Test Event",
			detail: {
				test: true,
				timestamp,
				message: "This is a test event from EventBridge",
			},
		},
		{
			title: "Application Event",
			detail: {
				eventType: "user.created",
				userId: "user-123",
				email: "user@example.com",
				timestamp,
				source: "user-service",
			},
		},
		{
			title: "System Event",
			detail: {
				eventType: "deployment.completed",
				serviceName: "backend",
				version: "v1.2.3",
				environment: env,
				status: "success",
				timestamp,
			},
		},
		{
			title: "Business Event",
			detail: {
				eventType: "order.processed",
				orderId: "ORD-123456",
				customerId: "CUST-789",
				amount: 99.99,
				currency: "USD",
				status: "completed",
				timestamp,
			},
		},
	];
}

interface EventBridgeTestEventProps {
	config: YamlInfrastructureConfig;
	/** Sends the event; the server API by default, replaced in stories. */
	sendTestEvent?: (event: TestEventRequest) => Promise<TestEventResponse>;
}

export function EventBridgeTestEvent({
	config,
	sendTestEvent = infrastructureApi.sendTestEvent,
}: EventBridgeTestEventProps) {
	const uid = useId();
	const busEnabled = isEventBusEnabled(config);
	const enhanced = isEnhancedBus(config);
	const projectBus = projectBusName(config);
	// Which bus, not its name, so the choice stays right for the env shown.
	const [busChoice, setBusChoice] = useState<EventBusChoice>(
		busEnabled ? "project" : "default",
	);
	const bus: EventBusChoice =
		busChoice === "project" && busEnabled ? "project" : "default";
	const eventBusName = bus === "project" ? projectBus : "default";
	// Test event state - default source to 'meroku.test'
	const [testEvent, setTestEvent] = useState<
		Omit<TestEventRequest, "eventBusName">
	>({
		env: config.env,
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
	const [eventGroupId, setEventGroupId] = useState("");
	// An event group orders events for ordered subscribers, which only the
	// enhanced project bus has.
	const showEventGroup = enhanced && bus === "project";
	const groupError = showEventGroup ? eventGroupIdError(eventGroupId) : null;

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
			const response = await sendTestEvent({
				...testEvent,
				env: config.env,
				eventBusName,
				...(showEventGroup && eventGroupId !== "" ? { eventGroupId } : {}),
			});
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
						<Label htmlFor={`${uid}-test-bus`}>Bus</Label>
						<Select
							value={bus}
							onValueChange={(value) => setBusChoice(value as EventBusChoice)}
						>
							<SelectTrigger id={`${uid}-test-bus`} aria-label="Test event bus">
								<SelectValue />
							</SelectTrigger>
							<SelectContent>
								<SelectItem value="default">Default bus</SelectItem>
								<SelectItem value="project" disabled={!busEnabled}>
									Project bus ({projectBus}){enhanced ? ", Enhanced" : ""}
								</SelectItem>
							</SelectContent>
						</Select>
					</div>

					{showEventGroup && (
						<div className="space-y-2">
							<Label htmlFor={`${uid}-test-group`}>
								Event group (optional)
							</Label>
							<Input
								id={`${uid}-test-group`}
								variant="code"
								value={eventGroupId}
								aria-invalid={groupError !== null}
								aria-describedby={`${uid}-test-group-hint`}
								onChange={(e) => setEventGroupId(e.target.value.trim())}
								placeholder="e.g., order-1042"
							/>
							<div id={`${uid}-test-group-hint`} aria-live="polite">
								{groupError ? (
									<StatusNote tone="error">{groupError}</StatusNote>
								) : (
									<NoteDetail>
										Ordered subscribers receive events of the same group in the
										order they were sent. Leave it blank for an event without a
										group.
									</NoteDetail>
								)}
							</div>
						</div>
					)}

					<div className="space-y-2">
						<Label htmlFor={`${uid}-test-source`}>Event Source</Label>
						<Input
							id={`${uid}-test-source`}
							variant="code"
							value={testEvent.source}
							onChange={(e) =>
								setTestEvent((prev) => ({ ...prev, source: e.target.value }))
							}
							placeholder="e.g., meroku.test"
						/>
						{allConfiguredSources.length > 0 && (
							<div className="space-y-1">
								<NoteDetail>
									Event processor sources: {allConfiguredSources.join(", ")}
								</NoteDetail>
								{!allConfiguredSources.includes(testEvent.source) &&
									testEvent.source && (
										<StatusNote tone="warning">
											This source doesn't match any configured event processors
										</StatusNote>
									)}
							</div>
						)}
					</div>

					<div className="space-y-2">
						<Label htmlFor={`${uid}-test-detail-type`}>Detail Type</Label>
						<Input
							id={`${uid}-test-detail-type`}
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
								<NoteDetail>
									Event processor detail types:{" "}
									{allConfiguredDetailTypes.join(", ")}
								</NoteDetail>
								{!allConfiguredDetailTypes.includes(testEvent.detailType) &&
									testEvent.detailType && (
										<StatusNote tone="warning">
											This detail type doesn't match any configured event
											processors
										</StatusNote>
									)}
							</div>
						)}
					</div>

					<div className="space-y-2">
						<Label htmlFor={`${uid}-test-detail`}>Event Detail (JSON)</Label>
						<Textarea
							id={`${uid}-test-detail`}
							value={detailJson}
							onChange={(e) => handleDetailJsonChange(e.target.value)}
							placeholder='{"test": true, "message": "EventBridge test event"}'
							variant="code"
							className="min-h-48"
							rows={10}
						/>
						{jsonError && <StatusNote tone="error">{jsonError}</StatusNote>}
					</div>

					<Button
						onClick={handleSendTestEvent}
						disabled={
							sendingEvent ||
							!!jsonError ||
							groupError !== null ||
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
						<Alert variant={eventResponse.success ? "success" : "danger"}>
							{eventResponse.success ? <CheckCircle /> : <XCircle />}
							<AlertDescription>
								{eventResponse.message}
								{eventResponse.eventId && (
									<NoteDetail>
										Event ID: <NoteCode>{eventResponse.eventId}</NoteCode>
									</NoteDetail>
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
					<CardContent className="space-y-3">
						{configuredEventTasks.map((task) => (
							<Card key={task.name} size="inset">
								<CardTitle variant="item">{task.name}</CardTitle>
								<KeyValueList size="compact">
									<KeyValue label="Sources">
										{task.sources?.join(", ") || "None"}
									</KeyValue>
									<KeyValue label="Detail types">
										{task.detail_types?.join(", ") || "None"}
									</KeyValue>
									<KeyValue label="Rule" variant="code">
										{task.rule_name}
									</KeyValue>
								</KeyValueList>
							</Card>
						))}
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
					{exampleEvents(config.env).map((example) => (
						<CodeBlock key={example.title} title={example.title}>
							{JSON.stringify(example.detail, null, 2)}
						</CodeBlock>
					))}
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
				<CardContent className="space-y-2">
					<StatusNote>
						Use <NoteCode>meroku.test</NoteCode> as the source for test events
						to easily identify them
					</StatusNote>
					<StatusNote>
						Include a <NoteCode>test: true</NoteCode> field in your event detail
						for easy filtering
					</StatusNote>
					<StatusNote>
						Always include timestamps in your test events for debugging
					</StatusNote>
					<StatusNote>
						Check CloudWatch Logs for event processor tasks to see if they were
						triggered
					</StatusNote>
					<StatusNote>
						This environment's project bus is <NoteCode>{projectBus}</NoteCode>
						{enhanced ? " (Enhanced)" : ""}; AWS services publish to the default
						bus
					</StatusNote>
					<StatusNote>
						Use EventBridge console to monitor event delivery and rule matches
					</StatusNote>
				</CardContent>
			</Card>
		</div>
	);
}
