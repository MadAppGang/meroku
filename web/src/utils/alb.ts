export const DEFAULT_ALB_IDLE_TIMEOUT = 60;
export const MIN_ALB_IDLE_TIMEOUT = 1;
export const MAX_ALB_IDLE_TIMEOUT = 4000;

export function parseAlbIdleTimeout(value: string): number | null {
	const normalized = value.trim();
	if (!/^\d+$/.test(normalized)) {
		return null;
	}

	const seconds = Number(normalized);
	if (
		!Number.isSafeInteger(seconds) ||
		seconds < MIN_ALB_IDLE_TIMEOUT ||
		seconds > MAX_ALB_IDLE_TIMEOUT
	) {
		return null;
	}

	return seconds;
}
