export const DEFAULT_SCHEDULE_TIMEZONE = "UTC";
export const DEFAULT_MAX_RETRY_ATTEMPTS = 3;
export const MAX_RETRY_ATTEMPTS = 185;

export type ContainerCommand = string[] | string | undefined;

export function formatContainerCommand(command: ContainerCommand): string {
	if (Array.isArray(command)) {
		return JSON.stringify(command);
	}

	if (!command) {
		return "";
	}

	const trimmed = command.trim();
	if (trimmed.startsWith("[") && trimmed.endsWith("]")) {
		try {
			const parsed = JSON.parse(trimmed);
			if (
				Array.isArray(parsed) &&
				parsed.every((argument) => typeof argument === "string")
			) {
				return JSON.stringify(parsed);
			}
		} catch {
			// Preserve legacy scalar values that only resemble JSON.
		}
	}

	return command;
}

export function parseContainerCommand(value: string): string[] | undefined {
	const trimmed = value.trim();
	if (!trimmed) {
		return undefined;
	}

	if (trimmed.startsWith("[") && trimmed.endsWith("]")) {
		try {
			const parsed = JSON.parse(trimmed);
			if (
				Array.isArray(parsed) &&
				parsed.every((argument) => typeof argument === "string")
			) {
				return parsed;
			}
		} catch {
			// Fall through to the comma-separated editor format.
		}
	}

	return trimmed
		.split(",")
		.map((argument) => argument.trim())
		.filter(Boolean);
}

export function isValidScheduleTimezone(timezone: string): boolean {
	if (!timezone.trim()) {
		return false;
	}

	try {
		new Intl.DateTimeFormat("en-US", { timeZone: timezone.trim() }).format();
		return true;
	} catch {
		return false;
	}
}

export function parseMaxRetryAttempts(
	value: string,
): { value: number } | { error: string } {
	const trimmed = value.trim();
	const parsed = Number(trimmed);
	if (
		!trimmed ||
		!Number.isInteger(parsed) ||
		parsed < 0 ||
		parsed > MAX_RETRY_ATTEMPTS
	) {
		return {
			error: `Retry attempts must be a whole number from 0 to ${MAX_RETRY_ATTEMPTS}`,
		};
	}

	return { value: parsed };
}

export function isValidSqsQueueArn(arn: string): boolean {
	if (!arn.trim()) {
		return true;
	}

	return /^arn:(aws|aws-cn|aws-us-gov|aws-iso|aws-iso-b):sqs:[a-z0-9-]+:\d{12}:[A-Za-z0-9_-]{1,80}$/.test(
		arn.trim(),
	);
}
