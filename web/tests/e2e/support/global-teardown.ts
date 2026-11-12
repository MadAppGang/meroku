import { rmSync } from "node:fs";

/**
 * Removes the run's temporary root. Set MEROKU_E2E_KEEP_ROOT=1 to keep it, for
 * example to inspect the YAML a failing test left behind.
 */
export default function globalTeardown(): void {
	const root = process.env.MEROKU_E2E_ROOT;
	if (!root || process.env.MEROKU_E2E_KEEP_ROOT === "1") {
		if (root) console.log(`Kept the e2e project root: ${root}`);
		return;
	}
	rmSync(root, { recursive: true, force: true });
}
