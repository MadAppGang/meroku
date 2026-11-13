import { CheckCircle, Clock, Send, XCircle } from "lucide-react";
import { useEffect, useId, useState } from "react";
import {
	infrastructureApi,
	type TestEventRequest,
	type TestEventResponse,
} from "../api/infrastructure";
import type { ComponentNode } from "../types";
import type { YamlInfrastructureConfig } from "../types/yamlConfig";
import { isEventBusEnabled, projectBusName } from "../utils/workloadEvents";
import { Alert, AlertDescription } from "./ui/alert";
import { Button } from "./ui/button";
import { Input } from "./ui/input";
import { Label } from "./ui/label";
import { StyledSection } from "./ui/styled-section";
import { Textarea } from "./ui/textarea";

interface EventTaskTestEventProps {
	config: YamlInfrastructureConfig;
	node: ComponentNode;
}

export function EventTaskTestEvent({ config, node }: EventTaskTestEventProps) {
	const uid = useId();
	const taskName = node.id.replace("event-", "");

	const eventTask = config.event_processor_tasks?.find(
		(task) => task.name === taskName,
	);

	// The task's first rule: the rules form, or the legacy single rule. The test
	// event goes to that rule's bus, so it reaches the rule.
	const busEnabled = isEventBusEnabled(config);
	const firstRule = eventTask?.rules?.[0];
	const ruleSources = firstRule ? firstRule.sources : eventTask?.sources;
	const ruleDetailTypes = firstRule
		? firstRule.detail_types
		: eventTask?.detail_types;
	const ruleBus =
		firstRule?.bus === "default" || (!firstRule?.bus && !busEnabled)
			? "default"
			: projectBusName(config);

	const [testEvent, setTestEvent] = useState<TestEventRequest>({
		env: config.env,
		eventBusName: ruleBus,
		source: ruleSources?.[0] || "meroku.test",
		detailType: ruleDetailTypes?.[0] || "",
		detail: {},
	});
	const [detailJson, setDetailJson] = useState(
		`{\n  "test": true,\n  "timestamp": "${new Date().toISOString()}"\n}`,
	);
	const [sendingEvent, setSendingEvent] = useState(false);
	const [eventResponse, setEventResponse] = useState<TestEventResponse | null>(
		null,
	);
	const [jsonError, setJsonError] = useState<string | null>(null);

	useEffect(() => {
		setTestEvent((prev) => ({
			...prev,
			eventBusName: ruleBus,
			detailType: ruleDetailTypes?.[0] || prev.detailType,
		}));
	}, [ruleBus, ruleDetailTypes]);

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
			const response = await infrastructureApi.sendTestEvent({
				...testEvent,
				env: config.env,
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

	return (
		<div className="space-y-4">
			<StyledSection
				title="Send Test Event"
				description="Trigger this task via EventBridge"
				icon={Send}
			>
				<div className="space-y-4">
					<div className="grid grid-cols-2 gap-4">
						<div className="space-y-2">
							<Label htmlFor={`${uid}-test-source`}>Source</Label>
							<Input
								id={`${uid}-test-source`}
								value={testEvent.source}
								onChange={(e) =>
									setTestEvent((prev) => ({ ...prev, source: e.target.value }))
								}
								placeholder="meroku.test"
								className="font-mono text-sm"
							/>
							{ruleSources &&
								!ruleSources.includes(testEvent.source) &&
								testEvent.source && (
									<p className="text-xs text-warning">
										Doesn't match configured sources
									</p>
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
								placeholder="OrderCreated"
								className="font-mono text-sm"
							/>
							{ruleDetailTypes &&
								!ruleDetailTypes.includes(testEvent.detailType) &&
								testEvent.detailType && (
									<p className="text-xs text-warning">
										Doesn't match configured detail types
									</p>
								)}
						</div>
					</div>

					<div className="space-y-2">
						<Label htmlFor={`${uid}-test-detail`}>Event Detail (JSON)</Label>
						<Textarea
							id={`${uid}-test-detail`}
							value={detailJson}
							onChange={(e) => handleDetailJsonChange(e.target.value)}
							placeholder='{"orderId": "123"}'
							className="font-mono text-sm min-h-32"
							rows={6}
						/>
						{jsonError && <p className="text-xs text-danger">{jsonError}</p>}
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
								Send Test Event
							</>
						)}
					</Button>

					{eventResponse && (
						<Alert variant={eventResponse.success ? "success" : "danger"}>
							{eventResponse.success ? (
								<CheckCircle className="h-4 w-4" />
							) : (
								<XCircle className="h-4 w-4" />
							)}
							<AlertDescription>
								{eventResponse.message}
								{eventResponse.eventId && (
									<span className="text-xs ml-2 font-mono">
										ID: {eventResponse.eventId}
									</span>
								)}
							</AlertDescription>
						</Alert>
					)}
				</div>
			</StyledSection>
		</div>
	);
}
